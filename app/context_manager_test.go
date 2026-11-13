package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/session"
	"github.com/Federicoand98/mani/tool"
)

type contextFake struct {
	calls     int
	summaries int
	primary   int
	infoCalls int
	info      core.ModelLimits
	infoErr   error
	send      func(context.Context, []core.Message, []core.ToolDefinition, core.TokenHandler) (core.LLMResponse, error)
}

func (f *contextFake) ModelInfo(context.Context) (core.ModelLimits, error) {
	f.infoCalls++
	return f.info, f.infoErr
}

func isSummary(messages []core.Message) bool {
	if len(messages) == 0 || len(messages[0].Content) == 0 {
		return false
	}
	b, ok := messages[0].Content[0].(core.TextBlock)
	return ok && b.Text == summaryInstructions
}

func (f *contextFake) Send(ctx context.Context, messages []core.Message, tools []core.ToolDefinition, h core.TokenHandler) (core.LLMResponse, error) {
	f.calls++
	if isSummary(messages) {
		f.summaries++
	} else {
		f.primary++
	}
	if f.send != nil {
		return f.send(ctx, messages, tools, h)
	}
	return contextReply("Goal and decisions retained."), nil
}

func contextReply(text string) core.LLMResponse {
	return core.LLMResponse{Content: []core.ContentBlock{core.TextBlock{Text: text}}, StopReason: core.StopReasonEndTurn,
		Usage: core.TokenUsage{InputTokens: 100, OutputTokens: 10}}
}

func historyWithTurns(turns, size int) []core.Message {
	var messages []core.Message
	for i := range turns {
		messages = append(messages,
			core.Message{Role: core.RoleUser, Content: []core.ContentBlock{core.TextBlock{Text: fmt.Sprintf("task %d", i)}}},
			core.Message{Role: core.RoleAssistant, Content: []core.ContentBlock{core.TextBlock{Text: strings.Repeat("x", size)}}})
	}
	return append(messages, core.Message{Role: core.RoleUser, Content: []core.ContentBlock{core.TextBlock{Text: "current task"}}})
}

func managedFixture(f *contextFake, input int, history []core.Message) (*Runtime, *contextRun, context.Context) {
	cfg := config.Config{Provider: "test", MaxOutputTokens: 256, Providers: map[string]config.ProviderConfig{
		"test": {Model: "model", ModelLimits: map[string]core.ModelLimits{"model": {MaxInputTokens: input}}},
	}}
	rt := &Runtime{agent: core.NewAgent(f), cfg: cfg, store: session.NewInMemoryStore()}
	RegisterContextManagement(rt, true, 2, 0)
	st := &contextRun{session: session.NewWithMemory("model", core.NewInMemoryFrom(history))}
	ctx := context.WithValue(context.Background(), contextRunKey{}, st)
	return rt, st, ctx
}

func TestResolveContextLimits(t *testing.T) {
	tests := []struct {
		name                 string
		manual, endpoint     core.ModelLimits
		output, window, want int
		fail                 bool
	}{
		{"input is not shared window", core.ModelLimits{MaxInputTokens: 245000, MaxOutputTokens: 11000}, core.ModelLimits{}, 4096, 0, 245000, false},
		{"reserve shared output", core.ModelLimits{ContextWindow: 131072}, core.ModelLimits{}, 4096, 0, 126976, false},
		{"smallest known ceiling", core.ModelLimits{MaxInputTokens: 200000}, core.ModelLimits{ContextWindow: 131072}, 4096, 0, 126976, false},
		{"manifest only", core.ModelLimits{}, core.ModelLimits{}, 4096, 32000, 27904, false},
		{"manifest cannot enlarge", core.ModelLimits{ContextWindow: 32000}, core.ModelLimits{}, 4096, 64000, 27904, false},
		{"unknown", core.ModelLimits{}, core.ModelLimits{}, 4096, 0, 0, true},
		{"output beyond capacity", core.ModelLimits{MaxInputTokens: 32000, MaxOutputTokens: 1000}, core.ModelLimits{}, 4096, 0, 0, true},
		{"output consumes window", core.ModelLimits{ContextWindow: 2048}, core.ModelLimits{}, 4096, 0, 0, true},
		{"negative metadata", core.ModelLimits{}, core.ModelLimits{MaxInputTokens: -1}, 4096, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, budget, _, err := resolveLimits(tt.manual, tt.endpoint, tt.output, tt.window)
			if (err != nil) != tt.fail || (!tt.fail && budget != tt.want) {
				t.Fatalf("budget=%d err=%v", budget, err)
			}
		})
	}
}

func TestManagedContextCompactsAndPreservesHistory(t *testing.T) {
	f := &contextFake{}
	history := historyWithTurns(8, 1200)
	rt, st, ctx := managedFixture(f, 1600, history)
	f.send = func(ctx context.Context, m []core.Message, tools []core.ToolDefinition, h core.TokenHandler) (core.LLMResponse, error) {
		n, err := core.EstimateRequestTokens(m, tools)
		if err != nil || n > percent(st.budget, 80) {
			t.Fatalf("request=%d budget=%d err=%v", n, st.budget, err)
		}
		limits := core.TokenLimitsFrom(ctx)
		if limits.OutputTokens <= 0 || limits.OutputTokens > 256 {
			t.Fatal("generation limit missing")
		}
		if isSummary(m) && (len(tools) != 0 || h != nil) {
			t.Fatal("summary is not an isolated text call")
		}
		return contextReply("Goal and decisions retained."), nil
	}
	var saved int
	var compacted bool
	st.persist = func() error { saved++; return nil }
	st.emit = func(ev Event) {
		if ev.Type == EventContext && ev.Payload.(core.ContextUsage).Compacted {
			compacted = true
		}
	}
	_, err := rt.agent.Client.Send(ctx, history, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.summaries < 1 || f.summaries > 4 || f.primary != 1 || saved != 1 || !compacted {
		t.Fatalf("calls=%+v saved=%d compacted=%v", f, saved, compacted)
	}
	if len(st.session.Memory().Messages()) != len(history) || st.session.Compaction.CoveredMessages != 12 {
		t.Fatal("history changed or recent turns were summarized")
	}
	if st.session.Context.EstimatedInput > percent(st.budget, 60) || st.session.Context.LastInput == nil {
		t.Fatal("invalid context meter")
	}
	summaries := f.summaries
	if _, err := rt.agent.Client.Send(ctx, history, nil, nil); err != nil {
		t.Fatal(err)
	}
	if f.summaries != summaries {
		t.Fatal("unchanged history was summarized again")
	}
}

func TestManagedContextPersistsCheckpointAcrossReload(t *testing.T) {
	store, err := session.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &contextFake{}
	rt, st, ctx := managedFixture(f, 1600, historyWithTurns(8, 1200))
	st.persist = func() error { return store.Save(st.session) }
	if _, err := rt.agent.Client.Send(ctx, st.session.Memory().Messages(), nil, nil); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(st.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Compaction == nil || loaded.Context == nil || loaded.Context.State != "ready" {
		t.Fatal("checkpoint/status not persisted")
	}
	before := f.summaries
	st2 := &contextRun{session: loaded}
	if _, err := rt.agent.Client.Send(context.WithValue(context.Background(), contextRunKey{}, st2), loaded.Memory().Messages(), nil, nil); err != nil {
		t.Fatal(err)
	}
	if f.summaries != before {
		t.Fatal("reload repeated compaction")
	}
}

func TestManagedContextFailurePreservesCheckpoint(t *testing.T) {
	for _, scenario := range []string{"summary error", "empty", "save error", "four calls", "large turn", "budget"} {
		t.Run(scenario, func(t *testing.T) {
			f := &contextFake{}
			history := historyWithTurns(8, 1200)
			input := 1600
			if scenario == "four calls" {
				history = historyWithTurns(40, 600)
				input = 900
			}
			if scenario == "large turn" {
				history = historyWithTurns(4, 50)
				history[1].Content = []core.ContentBlock{core.TextBlock{Text: strings.Repeat("x", 12000)}}
			}
			rt, st, ctx := managedFixture(f, input, history)
			switch scenario {
			case "summary error":
				f.send = func(context.Context, []core.Message, []core.ToolDefinition, core.TokenHandler) (core.LLMResponse, error) {
					return core.LLMResponse{}, errors.New("summary unavailable")
				}
			case "empty":
				f.send = func(context.Context, []core.Message, []core.ToolDefinition, core.TokenHandler) (core.LLMResponse, error) {
					return contextReply(""), nil
				}
			case "save error":
				st.persist = func() error { return errors.New("disk full") }
			case "budget":
				rt.maxTokens = 10
				ctx = context.WithValue(ctx, budgetKey{}, &budgetState{tokens: 10})
			}
			_, err := rt.agent.Client.Send(ctx, history, nil, nil)
			if err == nil {
				t.Fatal("expected explicit failure")
			}
			if st.session.Compaction != nil || len(st.session.Memory().Messages()) != len(history) {
				t.Fatal("failed compaction modified checkpoint/history")
			}
			if f.summaries > 4 || f.primary != 0 {
				t.Fatalf("unbounded compaction or primary sent: %+v", f)
			}
			if scenario == "four calls" && f.summaries != 4 {
				t.Fatalf("did not exercise the four-call ceiling: %d", f.summaries)
			}
		})
	}
}

func TestManagedContextProtectedInputFailsBeforeSend(t *testing.T) {
	f := &contextFake{}
	history := historyWithTurns(1, 12000)
	rt, _, ctx := managedFixture(f, 1000, history)
	if _, err := rt.agent.Client.Send(ctx, history, nil, nil); err == nil {
		t.Fatal("oversized protected input accepted")
	}
	if f.calls != 0 {
		t.Fatal("sent request despite protected input overflow")
	}
}

func TestManagedContextOverflowRecoveryIsBounded(t *testing.T) {
	for _, scenario := range []string{"recover", "second overflow", "generic 400", "streamed"} {
		t.Run(scenario, func(t *testing.T) {
			f := &contextFake{}
			f.send = func(ctx context.Context, m []core.Message, tools []core.ToolDefinition, h core.TokenHandler) (core.LLMResponse, error) {
				if isSummary(m) {
					if len(tools) != 0 || h != nil {
						t.Fatal("summary can execute tools or stream")
					}
					return contextReply("Retained goal."), nil
				}
				if f.primary == 1 || scenario == "second overflow" {
					if scenario == "streamed" {
						h("partial", false)
					}
					return core.LLMResponse{}, &core.ProviderError{StatusCode: 400, ContextOverflow: scenario != "generic 400", Body: "too long"}
				}
				if core.TokenLimitsFrom(ctx).OutputTokens != 256 {
					t.Fatal("output cap missing")
				}
				return contextReply("done"), nil
			}
			history := historyWithTurns(5, 200)
			rt, _, ctx := managedFixture(f, 2000, history)
			_, err := rt.agent.Client.Send(ctx, history, nil, func(string, bool) {})
			if scenario == "recover" && err != nil {
				t.Fatal(err)
			}
			if scenario != "recover" && err == nil {
				t.Fatal("expected failure")
			}
			want := 2
			if scenario == "generic 400" || scenario == "streamed" {
				want = 1
				if f.summaries != 0 {
					t.Fatal("unsafe recovery")
				}
			}
			if f.primary != want {
				t.Fatalf("primary requests=%d want=%d", f.primary, want)
			}
		})
	}
}

func TestManagedOverflowDoesNotRepeatToolEffects(t *testing.T) {
	f := &contextFake{}
	f.send = func(_ context.Context, m []core.Message, _ []core.ToolDefinition, _ core.TokenHandler) (core.LLMResponse, error) {
		if isSummary(m) {
			return contextReply("Goal retained."), nil
		}
		switch f.primary {
		case 1:
			return core.LLMResponse{Content: []core.ContentBlock{core.ToolUseBlock{ID: "write-1", Name: "write", Input: map[string]any{}}}, StopReason: core.StopReasonToolUse}, nil
		case 2:
			return core.LLMResponse{}, &core.ProviderError{StatusCode: 400, ContextOverflow: true}
		default:
			return contextReply("done"), nil
		}
	}
	history := historyWithTurns(5, 200)
	history = history[:len(history)-1]
	rt, st, _ := managedFixture(f, 3000, history)
	effects := 0
	rt.WithTool(tool.New(tool.ToolSchema{Name: "write"}, core.RiskNone, func(context.Context, map[string]any) (string, error) { effects++; return "written", nil }))
	events, _ := rt.ExecuteIn(context.Background(), st.session, "write once")
	for ev := range events {
		if ev.Type == EventError {
			t.Fatal(ev.Payload.(ErrorPayload).Err)
		}
	}
	if effects != 1 || f.primary != 3 {
		t.Fatalf("effects=%d primary=%d", effects, f.primary)
	}
}

func TestManagedContextMetadataErrorsAreExplicit(t *testing.T) {
	f := &contextFake{infoErr: errors.New("authentication rejected")}
	rt, st, ctx := managedFixture(f, 0, historyWithTurns(0, 0))
	if _, err := rt.agent.Client.Send(ctx, st.session.Memory().Messages(), nil, nil); err == nil || !strings.Contains(err.Error(), "authentication") {
		t.Fatalf("err=%v", err)
	}
	if f.calls != 0 {
		t.Fatal("metadata failure silently ignored")
	}
}

func TestTurnBoundariesRejectOrphanResults(t *testing.T) {
	messages := []core.Message{{Role: core.RoleUser}, {Role: core.RoleUser, Content: []core.ContentBlock{core.ToolResultBlock{ToolUseID: "missing"}}}}
	if _, err := turnBoundaries(messages); err == nil {
		t.Fatal("orphan accepted")
	}
	messages = []core.Message{
		{Role: core.RoleUser},
		{Role: core.RoleAssistant, Content: []core.ContentBlock{core.ToolUseBlock{ID: "one"}}},
		{Role: core.RoleUser, Content: []core.ContentBlock{core.ToolResultBlock{ToolUseID: "one"}}},
		{Role: core.RoleAssistant},
		{Role: core.RoleUser},
	}
	starts, err := turnBoundaries(messages)
	if err != nil || len(starts) != 2 || starts[1] != 4 {
		t.Fatalf("starts=%v err=%v", starts, err)
	}
}

func TestManagedContextPreservesPreCallEditsAndSystemMessages(t *testing.T) {
	history := historyWithTurns(1, 20)
	history = append(history[:2], append([]core.Message{{Role: core.RoleSystem, Content: []core.ContentBlock{core.TextBlock{Text: "late instruction"}}}}, history[2:]...)...)
	f := &contextFake{}
	f.send = func(_ context.Context, m []core.Message, _ []core.ToolDefinition, _ core.TokenHandler) (core.LLMResponse, error) {
		if m[0].Role != core.RoleSystem || m[0].Content[0].(core.TextBlock).Text != "late instruction" {
			t.Fatal("system instruction not preserved")
		}
		if m[len(m)-1].Content[0].(core.TextBlock).Text != "edited task" {
			t.Fatal("pre-call edit lost")
		}
		return contextReply("done"), nil
	}
	rt, _, ctx := managedFixture(f, 2000, history)
	incoming := append([]core.Message(nil), history...)
	incoming[len(incoming)-1].Content = []core.ContentBlock{core.TextBlock{Text: "edited task"}}
	if _, err := rt.agent.Client.Send(ctx, incoming, nil, nil); err != nil {
		t.Fatal(err)
	}
	if history[len(history)-1].Content[0].(core.TextBlock).Text != "current task" {
		t.Fatal("archive mutated")
	}
}

func TestRetryDoesNotReplayPartialResponse(t *testing.T) {
	f := &contextFake{send: func(_ context.Context, _ []core.Message, _ []core.ToolDefinition, h core.TokenHandler) (core.LLMResponse, error) {
		h("partial thinking", true)
		return core.LLMResponse{}, &core.ProviderError{StatusCode: 503}
	}}
	client := NewRetryClient(f, 3, 0)
	if _, err := client.Send(context.Background(), nil, nil, func(string, bool) {}); err == nil {
		t.Fatal("expected error")
	}
	if f.calls != 1 {
		t.Fatal("partial response replayed")
	}
}

func TestProtectedTurnsSkipFailedQuestionsAndKeepRunFeedback(t *testing.T) {
	history := historyWithTurns(4, 20)
	history = history[:len(history)-1]
	history = append(history,
		core.Message{Role: core.RoleUser, Content: []core.ContentBlock{core.TextBlock{Text: "failed question"}}},
		core.Message{Role: core.RoleUser, Content: []core.ContentBlock{core.TextBlock{Text: "real current task"}}},
		core.Message{Role: core.RoleAssistant, Content: []core.ContentBlock{core.TextBlock{Text: "schema retry"}}},
		core.Message{Role: core.RoleUser, Content: []core.ContentBlock{core.TextBlock{Text: "internal schema feedback"}}})
	starts, err := turnBoundaries(history)
	if err != nil {
		t.Fatal(err)
	}
	active := 9
	start := protectedStart(history, starts, 2, &active)
	if start != 4 {
		t.Fatalf("protected start=%d, want two completed turns plus failed/current questions", start)
	}
}

func TestLegacyCounterTracksWithoutCompaction(t *testing.T) {
	f := &contextFake{}
	history := historyWithTurns(8, 1200)
	rt, st, ctx := managedFixture(f, 1600, history)
	rt.contextPolicy = nil
	if _, err := rt.agent.Client.Send(ctx, history, nil, nil); err != nil {
		t.Fatal(err)
	}
	if f.primary != 1 || f.summaries != 0 || st.session.Compaction != nil {
		t.Fatal("legacy mode introduced summarization")
	}
	if st.session.Context == nil || st.session.Context.EstimatedInput <= percent(st.budget, 80) || st.session.Context.LastInput == nil {
		t.Fatal("legacy counter omitted actual large request")
	}
}

func TestLegacyUnknownWindowIsVisibleWithoutChangingSend(t *testing.T) {
	f := &contextFake{}
	rt, st, ctx := managedFixture(f, 0, historyWithTurns(0, 0))
	rt.contextPolicy = nil
	if _, err := rt.agent.Client.Send(ctx, st.session.Memory().Messages(), nil, nil); err != nil {
		t.Fatal(err)
	}
	if f.primary != 1 || st.session.Context.State != "unknown" || st.session.Context.InputBudget != 0 || st.session.Context.EstimatedInput == 0 {
		t.Fatal("unknown window hidden or legacy send blocked")
	}
}

func TestLegacyCounterUsesPostTrimInputAndPersistsFinalHistory(t *testing.T) {
	f := &contextFake{}
	history := historyWithTurns(8, 1200)
	history = history[:len(history)-1]
	rt, st, _ := managedFixture(f, 1600, history)
	rt.contextPolicy = nil
	rt.agent.Hooks().OnPreLLMCall(func(_ context.Context, p *core.PreLLMCallPayload) error {
		p.Messages = p.Messages[len(p.Messages)-3:]
		return nil
	})
	var sent []core.Message
	f.send = func(ctx context.Context, messages []core.Message, _ []core.ToolDefinition, _ core.TokenHandler) (core.LLMResponse, error) {
		sent = append([]core.Message(nil), messages...)
		if core.TokenLimitsFrom(ctx).OutputTokens != 256 {
			t.Fatal("legacy output cap missing")
		}
		return contextReply("done"), nil
	}
	events, cancel := rt.ExecuteIn(context.Background(), st.session, "new question")
	defer cancel()
	for ev := range events {
		if ev.Type == EventError {
			t.Fatal(ev.Payload.(ErrorPayload).Err)
		}
	}
	want, err := core.EstimateRequestTokens(append(sent, core.Message{Role: core.RoleAssistant, Content: []core.ContentBlock{core.TextBlock{Text: "done"}}}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.session.Context.EstimatedInput != want || len(st.session.Memory().Messages()) != 18 || f.summaries != 0 {
		t.Fatalf("counter=%+v want=%d history=%d", st.session.Context, want, len(st.session.Memory().Messages()))
	}
	saved, err := rt.store.Load(st.session.ID)
	if err != nil || saved.Context == nil || saved.Context.EstimatedInput != want {
		t.Fatalf("saved=%+v err=%v", saved, err)
	}
}

func TestLegacyCounterDoesNotIntroduceMetadataGuards(t *testing.T) {
	for _, f := range []*contextFake{{infoErr: errors.New("metadata unavailable")}, {info: core.ModelLimits{ContextWindow: 128}}} {
		rt, st, ctx := managedFixture(f, 0, historyWithTurns(0, 0))
		rt.contextPolicy = nil
		if _, err := rt.agent.Client.Send(ctx, st.session.Memory().Messages(), nil, nil); err != nil {
			t.Fatal(err)
		}
		if f.primary != 1 || st.session.Context.InputBudget != 0 {
			t.Fatal("passive metadata changed legacy execution")
		}
	}
}
