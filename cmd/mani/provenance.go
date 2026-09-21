package main

import (
	"time"

	"github.com/Federicoand98/mani/app"
)

// runEnvelope is what a result carries with it once it leaves mani: enough to
// find the run in the journal, and to know which agent produced the value.
type runEnvelope struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Manifest  string    `json:"manifest"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	InTokens  int       `json:"in_tokens"`
	OutTokens int       `json:"out_tokens"`
}

// turnResult collects, from one pass over the event channel, everything both
// `run --provenance` and `batch` need.
type turnResult struct {
	RunID     string
	Result    map[string]any
	Text      string
	InTokens  int
	OutTokens int
	Err       error
	Cancelled bool
}

func consume(ch <-chan app.Event) turnResult {
	var t turnResult
	for ev := range ch {
		switch ev.Type {
		case app.EventPermissionRequest:
			ev.Payload.(app.PermissionRequestPayload).Respond <- app.Deny
		case app.EventUsage:
			p := ev.Payload.(app.UsagePayload)
			t.InTokens += p.Input
			t.OutTokens += p.Output
		case app.EventDone:
			p := ev.Payload.(app.DonePayload)
			t.RunID, t.Result, t.Text = p.RunID, p.Result, p.Text
		case app.EventCancelled:
			t.Cancelled = true
		case app.EventError:
			if p, ok := ev.Payload.(app.ErrorPayload); ok {
				t.RunID, t.Err = p.RunID, p.Err
			}
		}
	}
	return t
}

func (t turnResult) payload() map[string]any {
	if t.Result != nil {
		return t.Result
	}
	return map[string]any{"response": t.Text}
}

func (t turnResult) envelope(rt *app.Runtime, manifest string, started, ended time.Time) runEnvelope {
	return runEnvelope{
		ID:        t.RunID,
		Source:    "cli",
		Provider:  rt.Provider(),
		Model:     rt.ModelName(),
		Manifest:  manifest,
		StartedAt: started,
		EndedAt:   ended,
		InTokens:  t.InTokens,
		OutTokens: t.OutTokens,
	}
}
