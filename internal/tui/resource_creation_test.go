package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestResourceCreationReviewShowsManifestContextAndNamespace(t *testing.T) {
	manifest := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: demo\n"
	model := newResourceCreationModel(manifest, "production-eu", "payments")
	view := model.View()

	for _, expected := range []string{
		"apiVersion: v1",
		"kind: ConfigMap",
		"production-eu",
		"payments",
		"Are you sure you want to create these resources in k8s cluster?",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("review is missing %q:\n%s", expected, view)
		}
	}
	if *model.answer {
		t.Fatal("resource creation must default to No")
	}
}

func TestResourceCreationReviewRequiresExplicitYes(t *testing.T) {
	model := newResourceCreationModel("kind: ConfigMap\n", "dev", "default")

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(resourceCreationModel)
	if cmd == nil || !model.done {
		t.Fatalf("default confirmation command = %v, done = %v", cmd, model.done)
	}
	if model.confirmed {
		t.Fatal("default Enter confirmed resource creation")
	}

	model = newResourceCreationModel("kind: ConfigMap\n", "dev", "default")
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = updated.(resourceCreationModel)
	if cmd == nil || !model.done || !model.confirmed {
		t.Fatalf("yes confirmation command = %v, done = %v, confirmed = %v", cmd, model.done, model.confirmed)
	}
}

func TestResourceCreationReviewScrollsLongYAML(t *testing.T) {
	lines := make([]string, 30)
	for index := range lines {
		lines[index] = fmt.Sprintf("line-%02d", index)
	}
	model := newResourceCreationModel(strings.Join(lines, "\n"), "dev", "default")
	model.viewport.Height = 5

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(resourceCreationModel)
	if model.viewport.YOffset != 1 {
		t.Fatalf("viewport offset = %d, want 1", model.viewport.YOffset)
	}
	if !strings.Contains(model.View(), "YAML scroll:") {
		t.Fatalf("scroll status is missing:\n%s", model.View())
	}
}
