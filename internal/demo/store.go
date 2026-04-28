package demo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Store struct {
	mutex       sync.RWMutex
	path        string
	model       readModel
	subscribers map[chan struct{}]struct{}
}

type readModel struct {
	Events map[string]EventRecord `json:"events"`
	Traces map[string]TraceRecord `json:"traces"`
	DLQ    []DLQRecord            `json:"dlq"`
}

func NewStore(path string) (*Store, error) {
	if path == "" {
		path = DefaultDataPath
	}

	store := &Store{
		path: path,
		model: readModel{
			Events: make(map[string]EventRecord),
			Traces: make(map[string]TraceRecord),
			DLQ:    make([]DLQRecord, 0),
		},
		subscribers: make(map[chan struct{}]struct{}),
	}

	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *Store) RecordPublished(event MediaDeliveryEvent) error {
	event.ApplyDefaults()
	now := time.Now().UTC()

	store.mutex.Lock()
	defer store.mutex.Unlock()

	traceID := TraceIDForEvent(event)
	store.model.Events[event.EventID] = EventRecord{
		EventID:       event.EventID,
		TraceID:       traceID,
		AssetID:       event.AssetID,
		Topic:         event.Topic,
		EventType:     event.EventType,
		Status:        TraceStatusQueued,
		CorrelationID: event.CorrelationID,
		CreatedAt:     event.CreatedAt,
		UpdatedAt:     now,
	}
	if _, ok := store.model.Traces[traceID]; !ok {
		store.model.Traces[traceID] = newTraceRecord(event, TraceStatusQueued, StagePending)
	}

	return store.persistAndBroadcastLocked()
}

func (store *Store) RecordEventAccepted(event MediaDeliveryEvent) error {
	event.ApplyDefaults()
	now := time.Now().UTC()

	store.mutex.Lock()
	defer store.mutex.Unlock()

	traceID := TraceIDForEvent(event)
	trace := store.traceLocked(event)
	trace.Status = TraceStatusRunning
	trace.CurrentStage = StagePending
	trace.UpdatedAt = now
	trace = setStep(trace, "event-accepted", StepStatusCompleted, "Kafka-like topic message validated", "", now)

	eventRecord := store.model.Events[event.EventID]
	eventRecord.Status = "consumed"
	eventRecord.UpdatedAt = now
	store.model.Events[event.EventID] = eventRecord
	store.model.Traces[traceID] = trace

	return store.persistAndBroadcastLocked()
}

func (store *Store) MarkWorkflowStarted(event MediaDeliveryEvent, workflowID string, runID string) error {
	now := time.Now().UTC()

	store.mutex.Lock()
	defer store.mutex.Unlock()

	traceID := TraceIDForEvent(event)
	trace := store.traceLocked(event)
	trace.WorkflowID = workflowID
	trace.RunID = runID
	trace.Status = TraceStatusRunning
	trace.CurrentStage = StagePending
	trace.UpdatedAt = now
	trace = setStep(trace, "workflow-started", StepStatusCompleted, ImageProcessingWorkflowName, "", now)
	trace.ActivityLog = append(trace.ActivityLog, ActivityLogEntry{
		Time:         now,
		ActivityName: "StartWorkflow",
		Status:       StepStatusCompleted,
		Message:      workflowID,
	})
	store.model.Traces[traceID] = trace

	return store.persistAndBroadcastLocked()
}

func (store *Store) MarkDuplicate(event MediaDeliveryEvent, workflowID string) error {
	now := time.Now().UTC()

	store.mutex.Lock()
	defer store.mutex.Unlock()

	traceID := TraceIDForEvent(event)
	trace := store.traceLocked(event)
	trace.WorkflowID = workflowID
	trace.Status = TraceStatusRunning
	trace.CurrentStage = "Duplicate"
	trace.UpdatedAt = now
	trace.ActivityLog = append(trace.ActivityLog, ActivityLogEntry{
		Time:         now,
		ActivityName: "StartWorkflow",
		Status:       "duplicate",
		Message:      "WorkflowIdReusePolicy rejected duplicate start",
	})
	store.model.Traces[traceID] = trace

	eventRecord := store.model.Events[event.EventID]
	eventRecord.Status = "duplicate"
	eventRecord.UpdatedAt = now
	store.model.Events[event.EventID] = eventRecord

	return store.persistAndBroadcastLocked()
}

func (store *Store) RecordDLQ(event MediaDeliveryEvent, rawMessage []byte, reason string) error {
	now := time.Now().UTC()
	eventID := event.EventID
	traceID := ""
	if eventID != "" {
		traceID = TraceIDForEvent(event)
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.model.DLQ = append(store.model.DLQ, DLQRecord{
		ID:         fmt.Sprintf("dlq_%d", now.UnixNano()),
		EventID:    eventID,
		TraceID:    traceID,
		AssetID:    event.AssetID,
		Topic:      event.Topic,
		Reason:     reason,
		RawMessage: string(rawMessage),
		CreatedAt:  now,
	})

	if eventID != "" {
		eventRecord := store.model.Events[eventID]
		eventRecord.Status = TraceStatusDLQ
		eventRecord.UpdatedAt = now
		store.model.Events[eventID] = eventRecord

		trace := store.traceLocked(event)
		trace.Status = TraceStatusDLQ
		trace.CurrentStage = StageFailed
		trace.ErrorMessage = reason
		trace.UpdatedAt = now
		trace.CompletedAt = &now
		store.model.Traces[traceID] = trace
	}

	return store.persistAndBroadcastLocked()
}

func (store *Store) ProjectWorkflowState(input ProjectionInput) (ProjectionOutput, error) {
	if input.TraceID == "" {
		return ProjectionOutput{}, fmt.Errorf("traceId is required")
	}
	if input.StepKey == "" {
		return ProjectionOutput{}, fmt.Errorf("stepKey is required")
	}
	if input.StepStatus == "" {
		input.StepStatus = StepStatusCompleted
	}
	if input.OccurredAt.IsZero() {
		input.OccurredAt = time.Now().UTC()
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()

	trace, ok := store.model.Traces[input.TraceID]
	if !ok {
		return ProjectionOutput{}, fmt.Errorf("trace %q not found", input.TraceID)
	}

	trace.WorkflowID = firstNonEmpty(input.WorkflowID, trace.WorkflowID)
	trace.RunID = firstNonEmpty(input.RunID, trace.RunID)
	trace.CurrentStage = firstNonEmpty(input.Stage, trace.CurrentStage)
	trace.UpdatedAt = input.OccurredAt
	trace.Renditions = append([]Rendition(nil), input.Renditions...)
	trace.PublishedURL = firstNonEmpty(input.PublishedURL, trace.PublishedURL)
	trace = setStep(trace, input.StepKey, input.StepStatus, input.Detail, input.ErrorMessage, input.OccurredAt)

	if input.ActivityName != "" {
		trace.ActivityLog = append(trace.ActivityLog, ActivityLogEntry{
			Time:         input.OccurredAt,
			ActivityName: input.ActivityName,
			Status:       input.StepStatus,
			Message:      firstNonEmpty(input.Detail, input.ErrorMessage),
		})
	}

	switch input.StepStatus {
	case StepStatusFailed:
		trace.Status = TraceStatusFailed
		trace.CurrentStage = StageFailed
		trace.ErrorMessage = input.ErrorMessage
		completedAt := input.OccurredAt
		trace.CompletedAt = &completedAt
	case StepStatusRunning:
		trace.Status = TraceStatusRunning
	case StepStatusCompleted:
		if input.StepKey == "visible" {
			trace.Status = TraceStatusCompleted
			trace.CurrentStage = StageCompleted
			completedAt := input.OccurredAt
			trace.CompletedAt = &completedAt
		} else {
			trace.Status = TraceStatusRunning
		}
	}

	store.model.Traces[input.TraceID] = trace
	return ProjectionOutput{TraceID: trace.TraceID, Stage: trace.CurrentStage, Status: trace.Status}, store.persistAndBroadcastLocked()
}

func (store *Store) Snapshot(system SystemStatus) UISnapshot {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	traces := make([]TraceRecord, 0, len(store.model.Traces))
	for _, trace := range store.model.Traces {
		traces = append(traces, cloneTrace(trace))
	}
	sort.Slice(traces, func(i, j int) bool {
		return traces[i].UpdatedAt.After(traces[j].UpdatedAt)
	})

	events := make([]EventRecord, 0, len(store.model.Events))
	for _, event := range store.model.Events {
		events = append(events, event)
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].UpdatedAt.After(events[j].UpdatedAt)
	})

	dlq := make([]DLQRecord, 0, len(store.model.DLQ))
	dlq = append(dlq, store.model.DLQ...)
	sort.Slice(dlq, func(i, j int) bool {
		return dlq[i].CreatedAt.After(dlq[j].CreatedAt)
	})

	return UISnapshot{
		System:  system,
		Metrics: metricsFor(traces, dlq),
		Events:  events,
		Traces:  traces,
		DLQ:     dlq,
	}
}

func (store *Store) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	store.mutex.Lock()
	store.subscribers[ch] = struct{}{}
	store.mutex.Unlock()

	cancel := func() {
		store.mutex.Lock()
		delete(store.subscribers, ch)
		close(ch)
		store.mutex.Unlock()
	}
	return ch, cancel
}

func (store *Store) Reset() error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.model = readModel{
		Events: make(map[string]EventRecord),
		Traces: make(map[string]TraceRecord),
		DLQ:    make([]DLQRecord, 0),
	}
	return store.persistAndBroadcastLocked()
}

func (store *Store) traceLocked(event MediaDeliveryEvent) TraceRecord {
	traceID := TraceIDForEvent(event)
	trace, ok := store.model.Traces[traceID]
	if ok {
		return trace
	}
	trace = newTraceRecord(event, TraceStatusQueued, StagePending)
	store.model.Traces[traceID] = trace
	return trace
}

func (store *Store) load() error {
	raw, err := os.ReadFile(store.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read store: %w", err)
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, &store.model); err != nil {
		return fmt.Errorf("decode store: %w", err)
	}
	if store.model.Events == nil {
		store.model.Events = make(map[string]EventRecord)
	}
	if store.model.Traces == nil {
		store.model.Traces = make(map[string]TraceRecord)
	}
	if store.model.DLQ == nil {
		store.model.DLQ = make([]DLQRecord, 0)
	}
	return nil
}

func (store *Store) persistAndBroadcastLocked() error {
	if err := os.MkdirAll(filepath.Dir(store.path), 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	raw, err := json.MarshalIndent(store.model, "", "  ")
	if err != nil {
		return fmt.Errorf("encode store: %w", err)
	}
	if err := os.WriteFile(store.path, raw, 0o644); err != nil {
		return fmt.Errorf("write store: %w", err)
	}
	for ch := range store.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	return nil
}

func newTraceRecord(event MediaDeliveryEvent, status string, stage string) TraceRecord {
	now := time.Now().UTC()
	return TraceRecord{
		TraceID:             TraceIDForEvent(event),
		EventID:             event.EventID,
		EventType:           event.EventType,
		Topic:               event.Topic,
		CorrelationID:       event.CorrelationID,
		AssetID:             event.AssetID,
		ListingID:           event.ListingID,
		SourceURL:           event.SourceURL,
		DestinationURL:      event.DestinationURL,
		ContentType:         event.ContentType,
		SizeBytes:           event.SizeBytes,
		RequestedRenditions: append([]string(nil), event.RequestedRenditions...),
		Status:              status,
		CurrentStage:        stage,
		Steps:               cloneSteps(DefaultTraceSteps),
		ActivityLog:         make([]ActivityLogEntry, 0),
		CreatedAt:           firstTime(event.CreatedAt, now),
		UpdatedAt:           now,
	}
}

func setStep(trace TraceRecord, key string, status string, detail string, errorMessage string, timestamp time.Time) TraceRecord {
	for index := range trace.Steps {
		if trace.Steps[index].Key != key {
			continue
		}
		trace.Steps[index].Status = status
		if detail != "" {
			trace.Steps[index].Detail = detail
		}
		trace.Steps[index].ErrorMessage = errorMessage
		if !timestamp.IsZero() {
			occurredAt := timestamp
			trace.Steps[index].Timestamp = &occurredAt
		}
		if status == StepStatusCompleted || status == StepStatusFailed {
			trace.Steps[index].DurationMs = defaultStepDurationMs(key)
		}
		break
	}
	return trace
}

func defaultStepDurationMs(key string) int64 {
	switch key {
	case "event-accepted":
		return 42
	case "workflow-started":
		return 91
	case "downloading":
		return 900
	case "processing":
		return 1200
	case "uploading":
		return 720
	case "projected":
		return 80
	case "visible":
		return 38
	default:
		return 0
	}
}

func metricsFor(traces []TraceRecord, dlq []DLQRecord) MetricsSnapshot {
	var metrics MetricsSnapshot
	var lagTotal float64
	var lagCount int
	for _, trace := range traces {
		if trace.Status != TraceStatusQueued {
			metrics.EventsConsumed++
		}
		if trace.WorkflowID != "" {
			metrics.WorkflowsStarted++
		}
		switch trace.Status {
		case TraceStatusCompleted:
			metrics.Completed++
		case TraceStatusFailed:
			metrics.Failed++
		}
		if trace.CompletedAt != nil {
			lagTotal += trace.CompletedAt.Sub(trace.CreatedAt).Seconds()
			lagCount++
		}
	}
	metrics.DLQCount = len(dlq)
	if lagCount > 0 {
		metrics.AverageLagSeconds = lagTotal / float64(lagCount)
	}
	return metrics
}

func cloneTrace(trace TraceRecord) TraceRecord {
	trace.RequestedRenditions = append([]string(nil), trace.RequestedRenditions...)
	trace.Steps = cloneSteps(trace.Steps)
	trace.ActivityLog = append([]ActivityLogEntry(nil), trace.ActivityLog...)
	trace.Renditions = append([]Rendition(nil), trace.Renditions...)
	return trace
}

func cloneSteps(steps []TraceStep) []TraceStep {
	return append([]TraceStep(nil), steps...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}
