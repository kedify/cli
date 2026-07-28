package tui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRunWithSpinnerFallsBackToPlainProgressForNonInteractiveOutput(t *testing.T) {
	output := &bytes.Buffer{}
	called := false

	result, err := RunWithSpinner(output, "Discovering Prometheus…", func() (string, error) {
		called = true
		return "done", nil
	})
	if err != nil {
		t.Fatalf("RunWithSpinner() error = %v", err)
	}
	if !called {
		t.Fatal("operation was not called")
	}
	if result != "done" {
		t.Fatalf("result = %q, want done", result)
	}
	if output.String() != "Discovering Prometheus…\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRunWithSpinnerReturnsOperationError(t *testing.T) {
	wantErr := errors.New("discovery failed")
	_, err := RunWithSpinner(&bytes.Buffer{}, "Discovering Prometheus…", func() (string, error) {
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunWithSpinner() error = %v, want %v", err, wantErr)
	}
}

func TestOperationSpinnerModelAnimatesAndClearsWhenDone(t *testing.T) {
	model := newOperationSpinnerModel("Discovering Prometheus…", func() (string, error) {
		return "done", nil
	})
	initialView := model.View()
	if !strings.Contains(initialView, "Discovering Prometheus…") {
		t.Fatalf("initial view = %q", initialView)
	}

	updated, tickCmd := model.Update(model.spinner.Tick())
	model = updated.(operationSpinnerModel[string])
	if tickCmd == nil {
		t.Fatal("spinner did not schedule its next frame")
	}
	if model.View() == initialView {
		t.Fatalf("spinner frame did not change: %q", model.View())
	}

	updated, quitCmd := model.Update(operationDoneMsg[string]{result: "done"})
	model = updated.(operationSpinnerModel[string])
	if quitCmd == nil {
		t.Fatal("completed spinner did not quit")
	}
	if model.View() != "" {
		t.Fatalf("completed spinner view = %q, want empty", model.View())
	}
	if model.result != "done" {
		t.Fatalf("result = %q, want done", model.result)
	}
}
