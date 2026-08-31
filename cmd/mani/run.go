package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/core"
)

func runFromManifest(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", "", "path to the YAML manifest")
	task := fs.String("task", "", "run a single task headlessly; without it, the manifest tringgers are started")
	insecure := fs.Bool("insecure", false, "start webhook triggers without authentication (dev only)")
	_ = fs.Bool("verbose", false, "print logs to the terminal (default: quiet)")
	_ = fs.Bool("debug", false, "alias for --verbose")
	_ = fs.Parse(args)

	var images stringList
	fs.Var(&images, "image", "attach an image to the task (repeatable)")

	if *configPath == "" {
		return usagef("--config is required")
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
			opts = append(opts, app.AllowInsercureWebhook())
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
	for ev := range rt.Execute(ctx, *task, attachments...) {
		switch ev.Type {
		case app.EventPermissionRequest:
			ev.Payload.(app.PermissionRequestPayload).Respond <- app.Deny // fail-closed
		case app.EventDone:
			p := ev.Payload.(app.DonePayload)
			if p.Result != nil {
				b, _ := json.MarshalIndent(p.Result, "", "\t")
				fmt.Println(string(b))
			} else {
				fmt.Println(p.Text)
			}
		case app.EventError:
			if p, ok := ev.Payload.(app.ErrorPayload); ok {
				return p.Err
			}
		}
	}
	return nil
}
