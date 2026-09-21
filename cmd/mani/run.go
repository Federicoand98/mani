package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"time"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/core"
)

func runFromManifest(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", "", "path to the YAML manifest")
	task := fs.String("task", "", "run a single task headlessly; without it, the manifest triggers are started")
	insecure := fs.Bool("insecure", false, "start webhook triggers without authentication (dev only)")
	provenance := fs.Bool("provenance", false, "wrap the results with the run that produced it")
	in := fs.String("in", "", `flow only: JSONL records for the flow's input, or "-" for stdin`)
	out := fs.String("out", "", `flow only: directory that keeps every step's records, which makes the flow resumable`)
	limit := fs.Int("limit", 0, "flow only: at most this many new agent runs per step (0=all)")
	_ = fs.Bool("verbose", false, "print logs to the terminal (default: quiet)")
	_ = fs.Bool("debug", false, "alias for --verbose")
	var images stringList
	fs.Var(&images, "image", "attach an image to the task (repeatable)")
	_ = fs.Parse(args)

	if *configPath == "" {
		return usagef("--config is required")
	}

	isFlow, err := app.IsFlowFile(*configPath)
	if err != nil {
		return usagef("%v", err)
	}

	if isFlow {
		if bad := setFlag(fs, "task", "image", "provenance", "insecure"); bad != "" {
			return usagef("--%s does not apply to a flow: it takes its records from --in or from its first step", bad)
		}

		return runFlow(ctx, *configPath, *in, *out, *limit)
	}

	if bad := setFlag(fs, "in", "out", "limit"); bad != "" {
		return usagef("--%s does not apply to a manifest: it is only for flows", bad)
	}

	spec, err := app.LoadManifest(*configPath)
	if err != nil {
		return err
	}

	rt, err := app.Build(ctx, spec)
	if err != nil {
		return err
	}
	defer rt.Close()

	// nessun task → daemon dei trigger
	if *task == "" {
		if len(spec.Run.Triggers) == 0 {
			return usagef("no --task given and no triggers in the manifest")
		}

		var opts []app.DaemonOption
		if *insecure {
			opts = append(opts, app.AllowInsecureWebhook())
		}

		d, err := app.BuildDaemon(rt, spec, opts...)
		if err != nil {
			return err
		}
		slog.Info("[daemon]: starting", "triggers", len(spec.Run.Triggers))
		d.Run(ctx) // bloccante finché ctx non è cancellato
		return nil
	}

	var attachments []core.ContentBlock
	for _, p := range images {
		img, err := app.LoadImage(p)
		if err != nil {
			return usagef("%v", err)
		}
		attachments = append(attachments, img)
	}

	// turno singolo headless

	started := time.Now()
	ch, cancel := rt.ExecuteIn(app.WithSource(ctx, "cli"), rt.CurrentSession(), *task, attachments...)
	defer cancel()

	res := consume(ch)
	if res.Err != nil {
		return res.Err
	}

	if *provenance {
		out := map[string]any{
			"result": res.payload(),
			"run":    res.envelope(rt, "cli", *configPath, started, time.Now()),
		}
		b, _ := json.MarshalIndent(out, "", "\t")
		fmt.Println(string(b))
		return nil
	}

	if res.Result != nil {
		b, _ := json.MarshalIndent(res.Result, "", "\t")
		fmt.Println(string(b))
	} else {
		fmt.Println(res.Text)
	}

	return nil
}
