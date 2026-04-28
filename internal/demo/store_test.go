package demo

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreProjectsWorkflowState(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "read-model.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	event := NewSampleEvent(1)
	if err := store.RecordPublished(event); err != nil {
		t.Fatalf("record published: %v", err)
	}
	if err := store.RecordEventAccepted(event); err != nil {
		t.Fatalf("record accepted: %v", err)
	}
	if err := store.MarkWorkflowStarted(event, WorkflowIDForEvent(event), "run-1"); err != nil {
		t.Fatalf("mark workflow started: %v", err)
	}

	_, err = store.ProjectWorkflowState(ProjectionInput{
		TraceID:      TraceIDForEvent(event),
		WorkflowID:   WorkflowIDForEvent(event),
		RunID:        "run-1",
		StepKey:      "visible",
		StepStatus:   StepStatusCompleted,
		Stage:        StageCompleted,
		Detail:       "visible in sparkle",
		ActivityName: projectWorkflowStateActivityName,
		OccurredAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("project state: %v", err)
	}

	snapshot := store.Snapshot(SystemStatus{})
	if snapshot.Metrics.Completed != 1 {
		t.Fatalf("completed metrics = %d, want 1", snapshot.Metrics.Completed)
	}
	if len(snapshot.Traces) != 1 {
		t.Fatalf("trace count = %d, want 1", len(snapshot.Traces))
	}
	if snapshot.Traces[0].Status != TraceStatusCompleted {
		t.Fatalf("trace status = %q, want completed", snapshot.Traces[0].Status)
	}
}
