package manifest

import (
	"strings"
	"testing"
)

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
			h, m, err := ParseClock(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseClock(%q) = %02d:%02d, want an error", tc.in, h, m)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseClock(%q): %v", tc.in, err)
			}
			if h != tc.h || m != tc.m {
				t.Errorf("ParseClock(%q) = %02d:%02d, want %02d:%02d", tc.in, h, m, tc.h, tc.m)
			}
		})
	}
}

// The error names the value and says the time is local: a trigger that refuses
// to start must explain what to write instead.
func TestParseClock_ErrorMentionsInput(t *testing.T) {
	_, _, err := ParseClock("09:00 UTC")
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{`"09:00 UTC"`, "local"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
