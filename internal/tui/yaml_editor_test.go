package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestYAMLEditorUsesTextareaWithLineNumbers(t *testing.T) {
	value := "apiVersion: v1\nkind: ConfigMap\n"
	model := newYAMLEditorModel(value)

	if !model.editor.ShowLineNumbers {
		t.Fatal("YAML editor line numbers are disabled")
	}
	if !model.editor.Focused() {
		t.Fatal("YAML textarea is not focused")
	}
	if model.editor.Value() != value {
		t.Fatalf("editor value = %q, want %q", model.editor.Value(), value)
	}
	if !strings.Contains(model.View(), "Ctrl+S save and continue") {
		t.Fatalf("editor help is missing:\n%s", model.View())
	}
}

func TestYAMLEditorSavesValidEditedYAML(t *testing.T) {
	model := newYAMLEditorModel("kind: ConfigMap")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(yamlEditorModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("metadata:")})
	model = updated.(yamlEditorModel)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(yamlEditorModel)
	if cmd == nil || !model.done {
		t.Fatalf("save command = %v, done = %v", cmd, model.done)
	}
	if got := model.editor.Value(); got != "kind: ConfigMap\nmetadata:" {
		t.Fatalf("edited YAML = %q", got)
	}
}

func TestYAMLEditorRejectsInvalidYAMLOnSave(t *testing.T) {
	model := newYAMLEditorModel("kind: [")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(yamlEditorModel)

	if cmd != nil || model.done {
		t.Fatalf("save command = %v, done = %v", cmd, model.done)
	}
	if !strings.Contains(model.err, "invalid YAML") {
		t.Fatalf("error = %q", model.err)
	}
}

func TestValidateYAMLAcceptsMultipleDocuments(t *testing.T) {
	value := "kind: ConfigMap\n---\nkind: Secret\n"
	if err := validateYAML(value); err != nil {
		t.Fatalf("validateYAML() error = %v", err)
	}
}
