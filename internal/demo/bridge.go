package demo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
)

type Bridge struct {
	Broker    *Broker
	Store     *Store
	Client    client.Client
	TaskQueue string
	Logger    *slog.Logger
}

func (bridge *Bridge) Run(ctx context.Context) error {
	if bridge.Broker == nil {
		return fmt.Errorf("broker is required")
	}
	if bridge.Store == nil {
		return fmt.Errorf("store is required")
	}
	if bridge.Client == nil {
		return fmt.Errorf("temporal client is required")
	}
	if bridge.TaskQueue == "" {
		bridge.TaskQueue = DefaultTaskQueue
	}
	if bridge.Logger == nil {
		bridge.Logger = slog.Default()
	}

	messages := bridge.Broker.Subscribe(ctx)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case message, ok := <-messages:
			if !ok {
				return nil
			}
			if err := bridge.processMessage(ctx, message); err != nil {
				bridge.Logger.Error("bridge message failed", "messageId", message.ID, "error", err)
			}
		}
	}
}

func (bridge *Bridge) processMessage(ctx context.Context, message BrokerMessage) error {
	event := message.Event
	if err := event.Validate(); err != nil {
		if dlqErr := bridge.Store.RecordDLQ(event, message.RawMessage, err.Error()); dlqErr != nil {
			return fmt.Errorf("record dlq: %w", dlqErr)
		}
		return nil
	}

	if err := bridge.Store.RecordEventAccepted(event); err != nil {
		return fmt.Errorf("record event accepted: %w", err)
	}

	workflowID := WorkflowIDForEvent(event)
	run, err := bridge.Client.ExecuteWorkflow(
		ctx,
		client.StartWorkflowOptions{
			ID:                    workflowID,
			TaskQueue:             bridge.TaskQueue,
			WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		},
		ImageProcessingWorkflow,
		NewImageProcessingInput(event),
	)
	if err != nil {
		var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &alreadyStarted) {
			return bridge.Store.MarkDuplicate(event, workflowID)
		}
		if dlqErr := bridge.Store.RecordDLQ(event, message.RawMessage, fmt.Sprintf("start workflow: %v", err)); dlqErr != nil {
			return fmt.Errorf("start workflow failed and dlq failed: %v: %w", err, dlqErr)
		}
		return nil
	}

	return bridge.Store.MarkWorkflowStarted(event, workflowID, run.GetRunID())
}
