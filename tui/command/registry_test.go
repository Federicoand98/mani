package command

import (
	"errors"
	"testing"
)

type stubCommand struct {
	name     string
	gotArgs  []string
	result   Result
	err      error
	executed int
}

func (s *stubCommand) Name() string        { return s.name }
func (s *stubCommand) Description() string { return "stub" }
func (s *stubCommand) Execute(args []string) (Result, error) {
	s.executed++
	s.gotArgs = args
	return s.result, s.err
}

func TestRegistry_Dispatch_NoPrefix_ReturnsFalse(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubCommand{name: "/quit"})

	_, ok, err := r.Dispatch("ciao mondo")
	if err != nil {
		t.Errorf("err inattesa: %v", err)
	}
	if ok {
		t.Error("input senza '/' non deve essere riconosciuto come comando")
	}
}

func TestRegistry_Dispatch_RegisteredCommand(t *testing.T) {
	cmd := &stubCommand{name: "/quit", result: Result{Output: "bye", Quit: true}}
	r := NewRegistry()
	r.Register(cmd)

	res, ok, err := r.Dispatch("/quit")
	if err != nil {
		t.Fatalf("err inattesa: %v", err)
	}
	if !ok {
		t.Fatal("dispatch deve riconoscere /quit")
	}
	if res.Output != "bye" || !res.Quit {
		t.Errorf("result inatteso: %+v", res)
	}
	if cmd.executed != 1 {
		t.Errorf("Execute chiamato %d volte, atteso 1", cmd.executed)
	}
}

func TestRegistry_Dispatch_UnknownCommand_ReturnsFalse(t *testing.T) {
	r := NewRegistry()
	_, ok, err := r.Dispatch("/sconosciuto")
	if err != nil {
		t.Errorf("err inattesa: %v", err)
	}
	if ok {
		t.Error("comando sconosciuto non deve risultare 'handled'")
	}
}

func TestRegistry_Dispatch_PassesArgs(t *testing.T) {
	cmd := &stubCommand{name: "/echo"}
	r := NewRegistry()
	r.Register(cmd)

	if _, _, err := r.Dispatch("/echo  uno   due"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(cmd.gotArgs) != 2 || cmd.gotArgs[0] != "uno" || cmd.gotArgs[1] != "due" {
		t.Errorf("args attesi [uno due], ottenuti %+v", cmd.gotArgs)
	}
}

func TestRegistry_Dispatch_PropagatesCommandError(t *testing.T) {
	want := errors.New("kaboom")
	cmd := &stubCommand{name: "/fail", err: want}
	r := NewRegistry()
	r.Register(cmd)

	_, ok, err := r.Dispatch("/fail")
	if !ok {
		t.Error("dispatch deve risultare 'handled' anche su errore")
	}
	if !errors.Is(err, want) {
		t.Errorf("errore atteso %v, ottenuto %v", want, err)
	}
}

func TestRegistry_Dispatch_OnlyWhitespace_DoesNotPanic(t *testing.T) {
	// "   " inizia con '/'?  no -> early return.
	// Ma "/  " sì: strings.Fields lo splitta in ["/"] (non vuoto),
	// quindi parts[0] non panica. Documentiamo entrambi i casi.
	r := NewRegistry()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic inatteso: %v", r)
		}
	}()
	r.Dispatch("   ")
	r.Dispatch("/  ")
	r.Dispatch("/")
}
