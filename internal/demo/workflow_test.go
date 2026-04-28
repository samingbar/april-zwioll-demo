package demo

import (
	"path/filepath"
	"testing"

	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

func TestImageProcessingWorkflowCompletesAndProjectsState(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "read-model.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	event := NewSampleEvent(3)
	if err := store.RecordPublished(event); err != nil {
		t.Fatalf("record published: %v", err)
	}
	if err := store.RecordEventAccepted(event); err != nil {
		t.Fatalf("record accepted: %v", err)
	}

	var suite testsuite.WorkflowTestSuite
	environment := suite.NewTestWorkflowEnvironment()
	environment.RegisterWorkflowWithOptions(ImageProcessingWorkflow, workflow.RegisterOptions{Name: ImageProcessingWorkflowName})
	environment.RegisterActivity(&Activities{Store: store})

	environment.ExecuteWorkflow(ImageProcessingWorkflow, NewImageProcessingInput(event))
	if err := environment.GetWorkflowError(); err != nil {
		t.Fatalf("workflow failed: %v", err)
	}

	var output ImageProcessingOutput
	if err := environment.GetWorkflowResult(&output); err != nil {
		t.Fatalf("workflow result: %v", err)
	}
	if output.Status != TraceStatusCompleted {
		t.Fatalf("output status = %q, want completed", output.Status)
	}

	snapshot := store.Snapshot(SystemStatus{})
	if len(snapshot.Traces) != 1 {
		t.Fatalf("trace count = %d, want 1", len(snapshot.Traces))
	}
	if snapshot.Traces[0].CurrentStage != StageCompleted {
		t.Fatalf("stage = %q, want %q", snapshot.Traces[0].CurrentStage, StageCompleted)
	}
}
