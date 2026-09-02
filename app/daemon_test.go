package app

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// parseClock
// ---------------------------------------------------------------------------

func TestParseClock(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		h, m    int
		wantErr bool
	}{
		{name: "padded", in: "07:30", h: 7, m: 30},
		{name: "single digits", in: "7:5", h: 7, m: 5},
		{name: "midnight", in: "00:00", h: 0, m: 0},
		{name: "last minute of the day", in: "23:59", h: 23, m: 59},

		// A timezone suffix is the misconfiguration this strictness exists for:
		// fmt.Sscanf used to accept it and silently schedule 09:00 local.
		{name: "timezone suffix", in: "09:00 UTC", wantErr: true},
		{name: "trailing junk", in: "07:30abc", wantErr: true},
		{name: "seconds", in: "07:30:45", wantErr: true},
		{name: "leading space", in: " 7:30", wantErr: true},
		{name: "explicit sign", in: "+7:30", wantErr: true},
		{name: "no separator", in: "0730", wantErr: true},
		{name: "hour only", in: "7", wantErr: true},
		{name: "empty", in: "", wantErr: true},
		{name: "missing minutes", in: "07:", wantErr: true},
		{name: "missing hour", in: ":30", wantErr: true},
		{name: "not a time at all", in: "banana", wantErr: true},

		{name: "hour out of range", in: "24:00", wantErr: true},
		{name: "minute out of range", in: "07:60", wantErr: true},
		{name: "negative hour", in: "-1:00", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, m, err := parseClock(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseClock(%q) = %02d:%02d, want an error", tc.in, h, m)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseClock(%q): %v", tc.in, err)
			}
			if h != tc.h || m != tc.m {
				t.Errorf("parseClock(%q) = %02d:%02d, want %02d:%02d", tc.in, h, m, tc.h, tc.m)
			}
		})
	}
}

// The error names the value and says the time is local: a trigger that refuses
// to start must explain what to write instead.
func TestParseClock_ErrorMentionsInput(t *testing.T) {
	_, _, err := parseClock("09:00 UTC")
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{`"09:00 UTC"`, "local"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// ---------------------------------------------------------------------------
// nextOccurrence / previousOccurrence
// ---------------------------------------------------------------------------

// rome is the reference timezone for the DST tests: in 2026 it springs forward
// on 29 March and falls back on 25 October, both at 03:00 local.
func rome(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		t.Skipf("timezone database unavailable: %v", err)
	}
	return loc
}

func TestNextOccurrence(t *testing.T) {
	loc := rome(t)

	cases := []struct {
		name string
		now  time.Time
		h, m int
		want time.Time
	}{
		{
			name: "later today",
			now:  time.Date(2026, 6, 10, 8, 0, 0, 0, loc),
			h:    9, m: 0,
			want: time.Date(2026, 6, 10, 9, 0, 0, 0, loc),
		},
		{
			name: "already past, so tomorrow",
			now:  time.Date(2026, 6, 10, 10, 0, 0, 0, loc),
			h:    9, m: 0,
			want: time.Date(2026, 6, 11, 9, 0, 0, 0, loc),
		},
		{
			// Exactly now does not count: otherwise the timer would fire with a
			// zero wait and the trigger would run twice in the same minute.
			name: "exactly now counts as past",
			now:  time.Date(2026, 6, 10, 9, 0, 0, 0, loc),
			h:    9, m: 0,
			want: time.Date(2026, 6, 11, 9, 0, 0, 0, loc),
		},
		{
			name: "rolls over the end of the month",
			now:  time.Date(2026, 6, 30, 23, 0, 0, 0, loc),
			h:    9, m: 0,
			want: time.Date(2026, 7, 1, 9, 0, 0, 0, loc),
		},
		{
			name: "rolls over the end of the year",
			now:  time.Date(2026, 12, 31, 23, 0, 0, 0, loc),
			h:    9, m: 0,
			want: time.Date(2027, 1, 1, 9, 0, 0, 0, loc),
		},
		{
			// The regression: 28 March is 23 hours long in Rome, so adding a
			// fixed 24 hours used to land at 10:00 and stay wrong afterwards.
			name: "spring forward keeps the wall clock",
			now:  time.Date(2026, 3, 28, 10, 0, 0, 0, loc),
			h:    9, m: 0,
			want: time.Date(2026, 3, 29, 9, 0, 0, 0, loc),
		},
		{
			// Mirror case: 25 October lasts 25 hours, so a fixed 24 would fire
			// an hour early.
			name: "fall back keeps the wall clock",
			now:  time.Date(2026, 10, 24, 10, 0, 0, 0, loc),
			h:    9, m: 0,
			want: time.Date(2026, 10, 25, 9, 0, 0, 0, loc),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nextOccurrence(tc.now, tc.h, tc.m)
			if !got.Equal(tc.want) {
				t.Errorf("nextOccurrence(%s, %02d:%02d) = %s, want %s", tc.now, tc.h, tc.m, got, tc.want)
			}
			// Whatever the offset, the agent must wake at the time written in
			// the manifest.
			if hh, mm, _ := got.Clock(); hh != tc.h || mm != tc.m {
				t.Errorf("wall clock = %02d:%02d, want %02d:%02d", hh, mm, tc.h, tc.m)
			}
		})
	}
}

func TestNextOccurrence_AlwaysInTheFuture(t *testing.T) {
	loc := rome(t)

	// A whole year, hour by hour, across both transitions: the next occurrence
	// must always be strictly ahead and never more than a day away.
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	end := time.Date(2027, 1, 1, 0, 0, 0, 0, loc)
	for now.Before(end) {
		next := nextOccurrence(now, 9, 0)
		if !next.After(now) {
			t.Fatalf("nextOccurrence(%s) = %s, not in the future", now, next)
		}
		if d := next.Sub(now); d > 25*time.Hour {
			t.Fatalf("nextOccurrence(%s) = %s, %s away: too far", now, next, d)
		}
		now = now.Add(time.Hour)
	}
}

func TestPreviousOccurrence(t *testing.T) {
	loc := rome(t)

	cases := []struct {
		name string
		now  time.Time
		h, m int
		want time.Time
	}{
		{
			name: "earlier today",
			now:  time.Date(2026, 6, 10, 10, 0, 0, 0, loc),
			h:    9, m: 0,
			want: time.Date(2026, 6, 10, 9, 0, 0, 0, loc),
		},
		{
			name: "not yet today, so yesterday",
			now:  time.Date(2026, 6, 10, 8, 0, 0, 0, loc),
			h:    9, m: 0,
			want: time.Date(2026, 6, 9, 9, 0, 0, 0, loc),
		},
		{
			name: "exactly now counts as already happened",
			now:  time.Date(2026, 6, 10, 9, 0, 0, 0, loc),
			h:    9, m: 0,
			want: time.Date(2026, 6, 10, 9, 0, 0, 0, loc),
		},
		{
			name: "rolls back over the start of the month",
			now:  time.Date(2026, 7, 1, 8, 0, 0, 0, loc),
			h:    9, m: 0,
			want: time.Date(2026, 6, 30, 9, 0, 0, 0, loc),
		},
		{
			// catch_up compares this instant with the last recorded run: an
			// hour of drift here means a missed run is replayed, or a run that
			// already happened is replayed a second time.
			name: "spring forward keeps the wall clock",
			now:  time.Date(2026, 3, 29, 8, 0, 0, 0, loc),
			h:    9, m: 0,
			want: time.Date(2026, 3, 28, 9, 0, 0, 0, loc),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := previousOccurrence(tc.now, tc.h, tc.m)
			if !got.Equal(tc.want) {
				t.Errorf("previousOccurrence(%s, %02d:%02d) = %s, want %s", tc.now, tc.h, tc.m, got, tc.want)
			}
		})
	}
}

// The two are each other's mirror: between one occurrence and the next there is
// exactly one calendar day, whatever the offset does.
func TestOccurrences_AreConsistent(t *testing.T) {
	loc := rome(t)

	now := time.Date(2026, 3, 28, 0, 0, 0, 0, loc)
	end := time.Date(2026, 4, 2, 0, 0, 0, 0, loc)
	for now.Before(end) {
		prev := previousOccurrence(now, 9, 0)
		next := nextOccurrence(now, 9, 0)
		if !prev.Before(next) {
			t.Fatalf("at %s: previous %s is not before next %s", now, prev, next)
		}
		if got := nextOccurrence(prev, 9, 0); !got.Equal(next) {
			t.Fatalf("at %s: next(previous) = %s, want %s", now, got, next)
		}
		now = now.Add(30 * time.Minute)
	}
}

// ---------------------------------------------------------------------------
// triggerID
// ---------------------------------------------------------------------------

func TestTriggerID(t *testing.T) {
	t.Run("an explicit name wins", func(t *testing.T) {
		got := triggerID(TriggerSpec{Type: "daily", At: "02:00", Name: "nightly-report"})
		if got != "nightly-report" {
			t.Errorf("triggerID = %q, want the declared name", got)
		}
	})

	t.Run("without a name it is derived and stable", func(t *testing.T) {
		spec := TriggerSpec{Type: "daily", At: "02:00", Prompt: "check the logs"}
		first := triggerID(spec)
		if first == "" {
			t.Fatal("triggerID is empty")
		}
		if second := triggerID(spec); second != first {
			t.Errorf("triggerID is not stable: %q then %q", first, second)
		}
		// The identity is what catch_up keys on across restarts: if it changed
		// between two runs of the same binary, every restart would look like a
		// brand new trigger that has never run.
		if len(first) != 12 {
			t.Errorf("triggerID = %q, want 12 hex characters", first)
		}
	})

	t.Run("different specs get different ids", func(t *testing.T) {
		base := TriggerSpec{Type: "daily", At: "02:00", Prompt: "check the logs"}
		others := []TriggerSpec{
			{Type: "every", At: "02:00", Prompt: "check the logs"},
			{Type: "daily", At: "03:00", Prompt: "check the logs"},
			{Type: "daily", At: "02:00", Prompt: "check the metrics"},
			{Type: "daily", At: "02:00", Prompt: "check the logs", Every: "1h"},
		}
		id := triggerID(base)
		for _, o := range others {
			if got := triggerID(o); got == id {
				t.Errorf("%+v collides with the base spec on id %q", o, got)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// renderWebhookPrompt
// ---------------------------------------------------------------------------

func TestRenderWebhookPrompt(t *testing.T) {
	cases := []struct {
		name     string
		template string
		body     string
		want     string
	}{
		{
			name:     "no template: the body is the prompt",
			template: "",
			body:     `{"alert":"disk full"}`,
			want:     `{"alert":"disk full"}`,
		},
		{
			name:     "the placeholder is substituted",
			template: "Triage this alert:\n{{body}}",
			body:     `{"alert":"disk full"}`,
			want:     "Triage this alert:\n" + `{"alert":"disk full"}`,
		},
		{
			name:     "every occurrence is substituted",
			template: "{{body}} — again: {{body}}",
			body:     "x",
			want:     "x — again: x",
		},
		{
			// Without a placeholder the body is appended: dropping it would
			// throw away the only input the trigger received.
			name:     "no placeholder: the body is appended",
			template: "Triage this alert.",
			body:     `{"alert":"disk full"}`,
			want:     "Triage this alert.\n\n" + `{"alert":"disk full"}`,
		},
		{
			name:     "empty body: the template is left alone",
			template: "Run the nightly check.",
			body:     "",
			want:     "Run the nightly check.",
		},
		{
			name:     "empty body with a placeholder leaves a gap",
			template: "Alert: {{body}}",
			body:     "",
			want:     "Alert: ",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderWebhookPrompt(tc.template, tc.body); got != tc.want {
				t.Errorf("renderWebhookPrompt(%q, %q) = %q, want %q", tc.template, tc.body, got, tc.want)
			}
		})
	}
}
