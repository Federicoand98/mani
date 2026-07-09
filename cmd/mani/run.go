package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"

	"github.com/Federicoand98/mani/app"
)

func runFromManifest(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", "", "path al manifest YAML")
	task := fs.String("task", "", "task headless; se assente avvia i trigger del manifest")
	_ = fs.Bool("verbose", false, "mostra i log sul terminale (default: silenzioso)")
	_ = fs.Bool("debug", false, "alias di --verbose")
	_ = fs.Parse(args)

	if *configPath == "" {
		return fmt.Errorf("run: --config richiesto")
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
		if len(spec.Triggers) == 0 {
			return fmt.Errorf("run: nessun --task e nessun trigger nel manifest")
		}
		d, err := app.BuildDaemon(rt, spec.Triggers)
		if err != nil {
			return err
		}
		slog.Info("avvio trigger daemon", "triggers", len(spec.Triggers))
		d.Run(ctx) // bloccante finché ctx non è cancellato
		return nil
	}

	// turno singolo headless
	for ev := range rt.Execute(ctx, *task) {
		switch ev.Type {
		case app.EventPermissionRequest:
			ev.Payload.(app.PermissionRequestPayload).Respond <- app.Deny // fail-closed
		case app.EventDone:
			if rt.HasStructure() {
				b, _ := json.MarshalIndent(rt.StructuredResult(), "", "	")
				fmt.Println(string(b))
			} else {
				fmt.Println(rt.LastResponse())
			}
		case app.EventError:
			if p, ok := ev.Payload.(app.ErrorPayload); ok {
				return p.Err
			}
		}
	}
	return nil
}
