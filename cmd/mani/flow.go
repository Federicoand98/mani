package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/session"
)

type flowRun struct {
	flow   app.FlowSpec
	base   string        // dir that agent paths and run commands are relative to
	out    string        // where the records live
	flat   bool          // a single step writing straight to the output file
	input  []record      // the records given with --in
	limit  int           // at most this many new runs per for_each step; 0 means no limit
	source string        // journal source of every run: "flow" or "batch"
	stream *recordStream // the result step's records go here; nil = not printed
	tokens tokenBudget
}

type tokenBudget struct {
	cap     int64
	in, out atomic.Int64
}

func (b *tokenBudget) add(in, out int) {
	b.in.Add(int64(in))
	b.out.Add(int64(out))
}

func (b *tokenBudget) spent() bool {
	return b.cap > 0 && b.in.Load()+b.out.Load() >= b.cap
}

var errBudget = errors.New("the flow's token budget (limits.tokens) is spent; raise it and run again to go on")

// newFlowRun prepares the state directory.
func newFlowRun(f app.FlowSpec, base, out, source string) (*flowRun, func(), error) {
	cleanup := func() {}
	if out == "" {
		tmp, err := os.MkdirTemp("", "mani-"+f.Flow+"-")
		if err != nil {
			return nil, nil, err
		}
		out, cleanup = tmp, func() { _ = os.RemoveAll(tmp) }
	}

	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, nil, fmt.Errorf("--out: %w", err)
	}

	r := &flowRun{flow: f, base: base, out: out, source: source}
	r.tokens.cap = int64(f.Limits.Tokens)
	return r, cleanup, nil
}

func (r *flowRun) run(ctx context.Context) error {
	outs := map[string]stepOutput{app.FlowInput: {records: r.input, field: "task"}}

	for _, st := range r.flow.Steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		store := &stepStore{dir: r.dir(st.Step)}
		if err := os.MkdirAll(store.dir, 0o755); err != nil {
			return err
		}
		up := outs[st.Upstream()]

		var out stepOutput
		var err error

		switch {
		case st.Agent != "" && st.ForEach != "":
			out, err = r.agentEach(ctx, st, store, up)
		case st.Agent != "":
			out, err = r.agentAll(ctx, st, store, up)
		default:
			out, err = r.command(ctx, st, store, up)
		}
		if err != nil {
			return fmt.Errorf("step %s: %w", st.Step, err)
		}
		outs[st.Step] = out
	}
	return nil
}

func (r *flowRun) dir(step string) string {
	if r.flat {
		return r.out
	}
	return filepath.Join(r.out, step)
}

// agentEach runs the agent once per upstream record, jobs at a time, on the records that are missing or older than the record they come from
func (r *flowRun) agentEach(ctx context.Context, st app.StepSpec, store *stepStore, up stepOutput) (stepOutput, error) {
	ids := make([]string, 0, len(up.records))
	var todo []record

	for _, in := range up.records {
		ids = append(ids, in.id())
		if store.freshSince(in.id(), up.stamp[in.id()]) {
			continue
		}
		if _, err := taskText(in[up.field]); err != nil {
			return stepOutput{}, fmt.Errorf("record %s: %q: %w", in.id(), up.field, err)
		}
		todo = append(todo, in)
	}

	later := 0
	if r.limit > 0 && len(todo) > r.limit {
		later = len(todo) - r.limit
		todo = todo[:r.limit]
	}

	r.logf("%s: %d to run, %d already done", st.Step, len(todo), len(ids)-len(todo)-later)

	var made sync.Map
	if len(todo) > 0 {
		manifest := app.AgentPath(r.base, st.Agent)
		rt, err := r.build(ctx, manifest)
		if err != nil {
			return stepOutput{}, err
		}
		defer rt.Close()

		var n atomic.Int64
		started, failed := r.mapRecords(ctx, todo, st.Jobs, func(ctx context.Context, in record) error {
			rec, err := r.runOne(ctx, rt, manifest, in, in[up.field])
			if err == nil {
				err = store.put(rec)
			}
			i := n.Add(1)
			if err != nil {
				r.logf("  [%d/%d] %s  FAILED: %s", i, len(todo), in.id(), firstLine(err.Error()))
				if ctx.Err() == nil { // Ctrl-C is not a failure of the record
					_ = store.failed(in.id(), err)
				}
				return err
			}
			r.logf("  [%d/%d] %s  ok", i, len(todo), in.id())
			made.Store(in.id(), true)
			r.emit(st, rec)
			return nil
		})

		switch {
		case ctx.Err() != nil:
			return stepOutput{}, ctx.Err()
		case failed > 0:
			return stepOutput{}, fmt.Errorf("%d of %d failed (see %s); run again to retry only those",
				failed, started, filepath.Join(store.dir, "errors.jsonl"))
		case started < len(todo):
			return stepOutput{}, errBudget
		}
	}
	if later > 0 {
		r.logf("%s: %d left for later (--limit)", st.Step, later)
	}

	out, err := store.output(ids, "result")
	if err != nil {
		return stepOutput{}, err
	}
	for _, rec := range out.records {
		if _, now := made.Load(rec.id()); !now {
			r.emit(st, rec) // done in an earlier run: printed too, the result is the whole step
		}
	}
	return out, nil
}

// mapRecords runs fn over recs, at most jobs at a time, and says how many it
// started and how many failed. It stops starting new ones when ctx ends or the
// budget is spent; the ones already running finish.
func (r *flowRun) mapRecords(ctx context.Context, recs []record, jobs int, fn func(context.Context, record) error) (started, failed int) {
	var nFailed atomic.Int64
	sem := make(chan struct{}, max(jobs, 1))
	var wg sync.WaitGroup

	for _, rec := range recs {
		sem <- struct{}{}
		// Checked after a slot frees up, so the budget seen is the latest one.
		if ctx.Err() != nil || r.tokens.spent() {
			<-sem
			break
		}
		started++
		wg.Go(func() {
			defer func() { <-sem }() // first: a panic must not keep the slot
			if err := fn(ctx, rec); err != nil {
				nFailed.Add(1)
			}
		})
	}
	wg.Wait()
	return started, int(nFailed.Load())
}

// build loads the step's manifest and makes its Runtime
func (r *flowRun) build(ctx context.Context, manifest string) (*app.Runtime, error) {
	spec, err := app.LoadManifest(manifest)
	if err != nil {
		return nil, err
	}
	return app.Build(ctx, spec)
}

// runOne executes one task on a fresh session: a task is a function, not a
// conversation. The Runtime is shared — the trigger daemon already runs tasks
// concurrently on one Runtime, so reentrancy is not new here.
func (r *flowRun) runOne(ctx context.Context, rt *app.Runtime, manifest string, in record, payload any) (record, error) {
	task, err := taskText(payload)
	if err != nil {
		return nil, err
	}

	started := time.Now()
	ch, cancel := rt.ExecuteIn(app.WithSource(ctx, r.source), session.New(rt.ModelName()), task)
	defer cancel()

	res := consume(ch)
	r.tokens.add(res.InTokens, res.OutTokens)
	switch {
	case res.Cancelled:
		return nil, errors.New("cancelled")
	case res.Err != nil:
		return nil, res.Err
	}

	// Extras first, then the fields mani owns: an input line with a field
	// called "result" must not be able to fake one.
	out := in.carry()
	out["id"] = in.id()
	out["result"] = res.payload()
	out["run"] = res.envelope(rt, r.source, manifest, started, time.Now())
	return out, nil
}

func (r *flowRun) emit(st app.StepSpec, rec record) {
	if r.stream != nil && st.Step == r.flow.Result {
		r.stream.write(rec)
	}
}

// logf writes progress to stderr: stdout belongs to the result.
func (r *flowRun) logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", r.source, fmt.Sprintf(format, args...))
}

func (r *flowRun) report() {
	r.logf("tokens: %d in, %d out", r.tokens.in.Load(), r.tokens.out.Load())
}

// agentAll runs the agent once, on every upstream record at once: the task is
// the JSON list of those records, so the agent can tell them apart by id. The
// run bookkeeping is left out — it is not the agent's business, and it costs
// tokens.
func (r *flowRun) agentAll(ctx context.Context, st app.StepSpec, store *stepStore, up stepOutput) (stepOutput, error) {
	if store.done(up.newest) {
		r.logf("%s: already done", st.Step)
	} else {
		r.logf("%s: 1 to run, on %d records", st.Step, len(up.records))
		all := make([]record, 0, len(up.records))
		for _, in := range up.records {
			rec := make(record, len(in))
			for k, v := range in {
				if k != "run" {
					rec[k] = v
				}
			}
			all = append(all, rec)
		}

		if r.tokens.spent() {
			return stepOutput{}, errBudget
		}
		manifest := app.AgentPath(r.base, st.Agent)
		rt, err := r.build(ctx, manifest)
		if err != nil {
			return stepOutput{}, err
		}
		defer rt.Close()

		rec, err := r.runOne(ctx, rt, manifest, record{"id": st.Step}, all)
		if err != nil {
			if ctx.Err() == nil {
				_ = store.failed(st.Step, err)
			}
			return stepOutput{}, err
		}
		if err := store.replace([]record{rec}); err != nil {
			return stepOutput{}, err
		}
	}

	out, err := store.output(nil, "result")
	if err == nil {
		r.emitAll(st, out)
	}
	return out, err
}

func (r *flowRun) command(ctx context.Context, st app.StepSpec, store *stepStore, up stepOutput) (stepOutput, error) {
	if store.done(up.newest) {
		r.logf("%s: already done", st.Step)
	} else {
		r.logf("%s: running %s", st.Step, strings.Join(st.Run, " "))

		var groups [][]record
		if st.ForEach != "" {
			for _, in := range up.records {
				groups = append(groups, []record{in})
			}
		} else {
			groups = [][]record{up.records}
		}

		var all []record
		seen := map[string]bool{}
		for _, g := range groups {
			recs, err := r.exec(ctx, st, g)
			if err != nil {
				return stepOutput{}, err
			}
			for _, rec := range recs {
				if seen[rec.id()] {
					return stepOutput{}, fmt.Errorf("printed id %q twice", rec.id())
				}
				seen[rec.id()] = true
			}
			all = append(all, recs...)
		}
		if err := store.replace(all); err != nil {
			return stepOutput{}, err
		}
	}

	out, err := store.output(nil, "task")
	if err == nil {
		r.emitAll(st, out)
	}
	return out, err
}

func (r *flowRun) exec(ctx context.Context, st app.StepSpec, in []record) ([]record, error) {
	var stdin bytes.Buffer
	enc := json.NewEncoder(&stdin)
	for _, rec := range in {
		if err := enc.Encode(rec); err != nil {
			return nil, err
		}
	}

	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, st.Run[0], st.Run[1:]...)
	cmd.Dir = r.base // a relative command path is resolved against Dir as well
	cmd.Stdin, cmd.Stdout, cmd.Stderr = &stdin, &stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %w", strings.Join(st.Run, " "), err)
	}
	return parseRecords(&stdout, st.Step+" (stdout)")
}

func (r *flowRun) emitAll(st app.StepSpec, out stepOutput) {
	for _, rec := range out.records {
		r.emit(st, rec)
	}
}

// runFlow is `mani run` on a flow file.
func runFlow(ctx context.Context, path, in, out string, limit int) error {
	f, err := app.LoadFlow(path)
	if err != nil {
		return usagef("%v", err)
	}

	var input []record
	switch {
	case f.Input != "" && in == "":
		return usagef("this flow reads its records from --in: %s", f.Input)
	case f.Input == "" && in != "":
		return usagef("--in: this flow declares no input, its first step makes the records")
	case in != "":
		if input, err = readRecordsFrom(in); err != nil {
			return usagef("%v", err)
		}
	}

	r, cleanup, err := newFlowRun(f, filepath.Dir(path), out, "flow")
	if err != nil {
		return err
	}
	defer cleanup()

	if out == "" && len(f.Steps) > 1 {
		r.logf("no --out: the records live in a temporary directory, removed at the end — nothing to resume from")
	}
	r.input, r.limit = input, limit
	r.stream = newRecordStream(os.Stdout)

	err = r.run(ctx)
	r.report()
	return err
}
