package manifest

import "testing"

func TestSummaryCompactionGrammar(t *testing.T) {
	data := []byte("identity:\n  name: example\ncontext:\n  window: 131072\n  compaction:\n    enabled: true\n    mode: summary\n    keep_turns: 2\nlimits:\n  max_output_tokens: 4096\n")
	spec, ps := Check(data, t.TempDir())
	if len(ps) != 0 {
		t.Fatalf("problems=%v", ps)
	}
	if spec.Context.Compaction.Mode != "summary" || spec.Context.Compaction.KeepTurns != 2 || spec.Limits.MaxOutputTokens != 4096 {
		t.Fatal("summary configuration lost")
	}
	for _, bad := range []string{"context:\n  compaction:\n    mode: guessed\n", "context:\n  compaction:\n    keep_turns: -1\n", "limits:\n  max_output_tokens: -1\n"} {
		_, ps := Check([]byte("identity:\n  name: example\n"+bad), t.TempDir())
		if len(ps) == 0 {
			t.Fatalf("invalid configuration accepted: %s", bad)
		}
	}
}
