package core

import "testing"

func TestTextFrom_SingleTextBlock(t *testing.T) {
	got := TextFrom([]ContentBlock{TextBlock{Text: "ciao"}})
	if got != "ciao" {
		t.Errorf("atteso 'ciao', ottenuto %q", got)
	}
}

func TestTextFrom_MultipleTextBlocks_Concatenated(t *testing.T) {
	got := TextFrom([]ContentBlock{
		TextBlock{Text: "prima "},
		TextBlock{Text: "seconda"},
	})
	if got != "prima seconda" {
		t.Errorf("atteso 'prima seconda', ottenuto %q", got)
	}
}

func TestTextFrom_MixedBlocks_OnlyTextExtracted(t *testing.T) {
	got := TextFrom([]ContentBlock{
		TextBlock{Text: "testo"},
		ToolUseBlock{ID: "x", Name: "read_file", Input: map[string]any{}},
	})
	if got != "testo" {
		t.Errorf("atteso solo 'testo', ottenuto %q", got)
	}
}

func TestTextFrom_ToolResultBlock_Ignored(t *testing.T) {
	got := TextFrom([]ContentBlock{
		ToolResultBlock{ToolUseID: "x", Content: "contenuto tool"},
		TextBlock{Text: "risposta"},
	})
	if got != "risposta" {
		t.Errorf("atteso solo 'risposta', ottenuto %q", got)
	}
}

func TestTextFrom_NoTextBlocks_ReturnsEmpty(t *testing.T) {
	got := TextFrom([]ContentBlock{
		ToolUseBlock{ID: "x", Name: "read_file", Input: map[string]any{}},
	})
	if got != "" {
		t.Errorf("atteso stringa vuota, ottenuto %q", got)
	}
}

func TestTextFrom_EmptySlice(t *testing.T) {
	got := TextFrom([]ContentBlock{})
	if got != "" {
		t.Errorf("atteso stringa vuota con slice vuota, ottenuto %q", got)
	}
}

func TestTextFrom_NilSlice(t *testing.T) {
	got := TextFrom(nil)
	if got != "" {
		t.Errorf("atteso stringa vuota con nil, ottenuto %q", got)
	}
}

func TestTextFrom_MultipleBlocks_OrderPreserved(t *testing.T) {
	got := TextFrom([]ContentBlock{
		TextBlock{Text: "a"},
		ToolUseBlock{ID: "x", Name: "f", Input: map[string]any{}},
		TextBlock{Text: "b"},
		TextBlock{Text: "c"},
	})
	if got != "abc" {
		t.Errorf("atteso 'abc', ottenuto %q", got)
	}
}
