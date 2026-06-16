package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestClusterPickerEnterSelectsCurrentOption(t *testing.T) {
	model := clusterPickerModel{
		options: []clusterOption{
			{Name: "alpha", Data: map[string]any{"name": "alpha"}},
			{Name: "beta", Data: map[string]any{"name": "beta"}},
		},
		cursor: 1,
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(clusterPickerModel)
	if got.choice == nil || got.choice.Name != "beta" {
		t.Fatalf("choice = %#v, want beta", got.choice)
	}
}

func TestClusterNameFallsBackToID(t *testing.T) {
	got := clusterName(map[string]any{"id": "cluster-1"})
	if got != "cluster-1" {
		t.Fatalf("clusterName() = %q, want %q", got, "cluster-1")
	}
}
