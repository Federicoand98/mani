package main

import (
	"context"
	"flag"
	"os"

	"github.com/Federicoand98/mani/app"
)

func runBatch(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("batch", flag.ExitOnError)
	configPath := fs.String("config", "", "path to the YAML manifest")
	in := fs.String("in", "", `JSONL file with one task per line or "-" for stdin`)
	out := fs.String("out", "", "directory for the results, which makes the batch resumable; without it they stream to sdout")
	jobs := fs.Int("jobs", 1, "tasks to run at the same time")
	limit := fs.Int("limit", 0, "stop after this many new tasks (0 = all of them)")
	_ = fs.Bool("verbose", false, "print logs to stderr (default: quiet)")
	_ = fs.Parse(args)

	switch {
	case *configPath == "":
		return usagef("--config is required")
	case *in == "":
		return usagef("--in is required")
	case *jobs < 1:
		return usagef("--jobs must be at least 1")
	}

	if _, err := app.LoadManifest(*configPath); err != nil {
		return usagef("%v", err)
	}

	input, err := readRecordsFrom(*in)
	if err != nil {
		return usagef("%v", err)
	}

	f := app.FlowSpec{
		Flow:  "batch",
		About: "runs one agent over a file of tasks",
		Input: "one task per line",
		Steps: []app.StepSpec{{
			Step:    "tasks",
			Does:    "runs the agent on each task",
			Agent:   *configPath,
			ForEach: app.FlowInput,
			Jobs:    *jobs,
		}},
		Result: "tasks",
	}

	r, cleanut, err := newFlowRun(f, ".", *out, "batch")
	if err != nil {
		return err
	}
	defer cleanut()

	r.flat, r.input, r.limit = true, input, *limit
	if *out == "" {
		r.stream = newRecordStream(os.Stdout)
	}
	err = r.run(ctx)
	r.report()
	return err
}
