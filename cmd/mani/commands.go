package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/Federicoand98/mani/app"
)

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, args []string) error
}

var commands = []command{
	{"run", "run an agent from manifest (single task or trigger deamon)", runFromManifest},
	{"serve", "expose an agent over HTTP/websocket", runServer},
	{"init", "scaffold a new agent manifest in the current directory", runInit},
	{"validate", "check a manifest without running it", runValidate},
	{"tui", "start the interactive terminal chat", runTUICommand},
}

func lookupCommand(name string) (command, bool) {
	for _, cmd := range commands {
		if cmd.name == name {
			return cmd, true
		}
	}
	return command{}, false
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `mani - a declarative agent runtime

Usage:
  mani [command] [flags]
  mani						start the interactive terminal chat

Commands:
`)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, c := range commands {
		fmt.Fprintf(tw, "  %s\t%s\n", c.name, c.summary)
	}
	tw.Flush()

	fmt.Fprint(w, `
Flags:
  -h, --help                show this help
  -v, --version             show the version

Run "mani <command> --help" for the flags of a single command.
`)
}

// commands
//

//go:embed templates/*.yaml
var templatesFS embed.FS

func templateNames() []string {
	entries, _ := templatesFS.ReadDir("templates")
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	sort.Strings(names)
	return names
}

func runInit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	tmpl := fs.String("template", "agent", "template to scaffold: "+strings.Join(templateNames(), ", "))
	out := fs.String("o", "agent.yaml", "output file")
	force := fs.Bool("force", false, "overwrite the output file if it already exists")
	_ = fs.Parse(args)

	data, err := templatesFS.ReadFile("templates/" + *tmpl + ".yaml")
	if err != nil {
		return usagef("unknown template %q (available: %s)", *tmpl, strings.Join(templateNames(), ", "))
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if *force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(*out, flags, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return usagef("%s already exists (use --force to overwrite)", *out)
		}
		return err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return err
	}

	abs, _ := filepath.Abs(*out)
	fmt.Printf("created %s\n\nnext:\n  mani validate --config %s\n  mani run --config %s --task \"hello\"\n",
		abs, *out, *out)
	return nil
}

func runValidate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	configPath := fs.String("config", "", "path to the YAML manifest")
	_ = fs.Parse(args)

	if *configPath == "" {
		return usagef("--config is required")
	}

	spec, err := app.LoadManifest(*configPath)
	if err != nil {
		return usagef("%v", err)
	}

	fmt.Printf("%s: ok\n", *configPath)
	fmt.Printf("  identity:     %s (%s / %s)\n", spec.Identity.Name, spec.Identity.Provider, spec.Identity.Model)
	fmt.Printf("  tools:        %d\n", len(spec.Capabilities.Tools))
	fmt.Printf("  subagents:    %d\n", len(spec.Capabilities.Subagents))
	fmt.Printf("  triggers:     %d\n", len(spec.Run.Triggers))
	if spec.Output.Schema.Type != "" {
		fmt.Printf("  output:       structured\n")
	}
	return nil
}
