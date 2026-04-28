package demo

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"
)

const (
	DefaultTemporalAddress = "localhost:7233"
	DefaultNamespace       = "default"
	DefaultTaskQueue       = "sparkle-image-processing"
	DefaultAPIAddress      = ":8080"
	DefaultDataPath        = "data/read-model.json"

	MediaDeliveredTopic     = "sparkle.media.delivered"
	MediaDeliveredEventType = "richmedia.media.delivered"

	ImageProcessingWorkflowName = "ImageProcessingWorkflow"

	TraceStatusQueued    = "queued"
	TraceStatusRunning   = "running"
	TraceStatusCompleted = "completed"
	TraceStatusFailed    = "failed"
	TraceStatusDLQ       = "dlq"

	StepStatusPending   = "pending"
	StepStatusRunning   = "running"
	StepStatusCompleted = "completed"
	StepStatusFailed    = "failed"

	StagePending     = "Pending"
	StageDownloading = "Downloading"
	StageProcessing  = "Processing"
	StageUploading   = "Uploading"
	StageProjected   = "Projected"
	StageCompleted   = "Completed"
	StageFailed      = "Failed"
)

var DefaultTraceSteps = []TraceStep{
	{Key: "event-accepted", Order: 1, Label: "Event Accepted", Detail: "Kafka-like topic message validated", Status: StepStatusPending},
	{Key: "workflow-started", Order: 2, Label: "Workflow Started", Detail: ImageProcessingWorkflowName, Status: StepStatusPending},
	{Key: "downloading", Order: 3, Label: "Downloading", Detail: "DownloadImage activity", ActivityName: "DownloadImageActivity", Status: StepStatusPending},
	{Key: "processing", Order: 4, Label: "Processing", Detail: "ProcessImage activity", ActivityName: "ProcessImageActivity", Status: StepStatusPending},
	{Key: "uploading", Order: 5, Label: "Uploading", Detail: "UploadImage activity", ActivityName: "UploadImageActivity", Status: StepStatusPending},
	{Key: "projected", Order: 6, Label: "Projected", Detail: "State projected to read model", ActivityName: "ProjectWorkflowStateActivity", Status: StepStatusPending},
	{Key: "visible", Order: 7, Label: "Visible in Sparkle", Detail: "Sparkle API can serve this state", Status: StepStatusPending},
}

type RuntimeConfig struct {
	TemporalAddress string
	Namespace       string
	TaskQueue       string
	APIAddress      string
	DataPath        string
}

type MediaDeliveryEvent struct {
	EventID             string    `json:"eventId"`
	EventType           string    `json:"eventType"`
	Topic               string    `json:"topic"`
	CorrelationID       string    `json:"correlationId"`
	AssetID             string    `json:"assetId"`
	ListingID           string    `json:"listingId"`
	SourceURL           string    `json:"sourceUrl"`
	DestinationURL      string    `json:"destinationUrl"`
	ContentType         string    `json:"contentType"`
	SizeBytes           int64     `json:"sizeBytes"`
	RequestedRenditions []string  `json:"requestedRenditions"`
	CreatedAt           time.Time `json:"createdAt"`
}

type ImageProcessingInput struct {
	TraceID             string    `json:"traceId"`
	EventID             string    `json:"eventId"`
	EventType           string    `json:"eventType"`
	Topic               string    `json:"topic"`
	CorrelationID       string    `json:"correlationId"`
	AssetID             string    `json:"assetId"`
	ListingID           string    `json:"listingId"`
	SourceURL           string    `json:"sourceUrl"`
	DestinationURL      string    `json:"destinationUrl"`
	ContentType         string    `json:"contentType"`
	SizeBytes           int64     `json:"sizeBytes"`
	RequestedRenditions []string  `json:"requestedRenditions"`
	StartedAt           time.Time `json:"startedAt"`
}

type ImageProcessingOutput struct {
	TraceID       string      `json:"traceId"`
	WorkflowID    string      `json:"workflowId"`
	AssetID       string      `json:"assetId"`
	PublishedURL  string      `json:"publishedUrl"`
	Renditions    []Rendition `json:"renditions"`
	AssetChecksum string      `json:"assetChecksum"`
	Status        string      `json:"status"`
}

type DownloadImageInput struct {
	TraceID   string `json:"traceId"`
	AssetID   string `json:"assetId"`
	SourceURL string `json:"sourceUrl"`
	SizeBytes int64  `json:"sizeBytes"`
}

type DownloadImageOutput struct {
	AssetID       string `json:"assetId"`
	LocalURI      string `json:"localUri"`
	AssetChecksum string `json:"assetChecksum"`
}

type ProcessImageInput struct {
	TraceID             string   `json:"traceId"`
	AssetID             string   `json:"assetId"`
	ListingID           string   `json:"listingId"`
	LocalURI            string   `json:"localUri"`
	AssetChecksum       string   `json:"assetChecksum"`
	RequestedRenditions []string `json:"requestedRenditions"`
}

type ProcessImageOutput struct {
	Renditions []Rendition `json:"renditions"`
}

type UploadImageInput struct {
	TraceID        string      `json:"traceId"`
	AssetID        string      `json:"assetId"`
	ListingID      string      `json:"listingId"`
	DestinationURL string      `json:"destinationUrl"`
	Renditions     []Rendition `json:"renditions"`
}

type UploadImageOutput struct {
	PublishedURL string      `json:"publishedUrl"`
	Renditions   []Rendition `json:"renditions"`
}

type ProjectionInput struct {
	TraceID      string      `json:"traceId"`
	WorkflowID   string      `json:"workflowId"`
	RunID        string      `json:"runId"`
	StepKey      string      `json:"stepKey"`
	StepStatus   string      `json:"stepStatus"`
	Stage        string      `json:"stage"`
	Detail       string      `json:"detail"`
	ActivityName string      `json:"activityName"`
	ErrorMessage string      `json:"errorMessage"`
	PublishedURL string      `json:"publishedUrl"`
	Renditions   []Rendition `json:"renditions"`
	OccurredAt   time.Time   `json:"occurredAt"`
}

type ProjectionOutput struct {
	TraceID string `json:"traceId"`
	Stage   string `json:"stage"`
	Status  string `json:"status"`
}

type Rendition struct {
	Name   string `json:"name"`
	URI    string `json:"uri"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type TraceStep struct {
	Key          string     `json:"key"`
	Order        int        `json:"order"`
	Label        string     `json:"label"`
	Detail       string     `json:"detail"`
	ActivityName string     `json:"activityName,omitempty"`
	Status       string     `json:"status"`
	Timestamp    *time.Time `json:"timestamp,omitempty"`
	DurationMs   int64      `json:"durationMs,omitempty"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
}

type ActivityLogEntry struct {
	Time         time.Time `json:"time"`
	ActivityName string    `json:"activityName"`
	Status       string    `json:"status"`
	DurationMs   int64     `json:"durationMs,omitempty"`
	Message      string    `json:"message,omitempty"`
}

type TraceRecord struct {
	TraceID             string             `json:"traceId"`
	EventID             string             `json:"eventId"`
	EventType           string             `json:"eventType"`
	Topic               string             `json:"topic"`
	WorkflowID          string             `json:"workflowId"`
	RunID               string             `json:"runId"`
	CorrelationID       string             `json:"correlationId"`
	AssetID             string             `json:"assetId"`
	ListingID           string             `json:"listingId"`
	SourceURL           string             `json:"sourceUrl"`
	DestinationURL      string             `json:"destinationUrl"`
	ContentType         string             `json:"contentType"`
	SizeBytes           int64              `json:"sizeBytes"`
	RequestedRenditions []string           `json:"requestedRenditions"`
	Status              string             `json:"status"`
	CurrentStage        string             `json:"currentStage"`
	Steps               []TraceStep        `json:"steps"`
	ActivityLog         []ActivityLogEntry `json:"activityLog"`
	Renditions          []Rendition        `json:"renditions"`
	PublishedURL        string             `json:"publishedUrl"`
	ErrorMessage        string             `json:"errorMessage,omitempty"`
	CreatedAt           time.Time          `json:"createdAt"`
	UpdatedAt           time.Time          `json:"updatedAt"`
	CompletedAt         *time.Time         `json:"completedAt,omitempty"`
}

type EventRecord struct {
	EventID       string    `json:"eventId"`
	TraceID       string    `json:"traceId"`
	AssetID       string    `json:"assetId"`
	Topic         string    `json:"topic"`
	EventType     string    `json:"eventType"`
	Status        string    `json:"status"`
	CorrelationID string    `json:"correlationId"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type DLQRecord struct {
	ID         string    `json:"id"`
	EventID    string    `json:"eventId,omitempty"`
	TraceID    string    `json:"traceId,omitempty"`
	AssetID    string    `json:"assetId,omitempty"`
	Topic      string    `json:"topic,omitempty"`
	Reason     string    `json:"reason"`
	RawMessage string    `json:"rawMessage"`
	CreatedAt  time.Time `json:"createdAt"`
}

type MetricsSnapshot struct {
	EventsConsumed    int     `json:"eventsConsumed"`
	WorkflowsStarted  int     `json:"workflowsStarted"`
	Completed         int     `json:"completed"`
	Failed            int     `json:"failed"`
	DLQCount          int     `json:"dlqCount"`
	AverageLagSeconds float64 `json:"averageLagSeconds"`
}

type UISnapshot struct {
	System  SystemStatus    `json:"system"`
	Metrics MetricsSnapshot `json:"metrics"`
	Events  []EventRecord   `json:"events"`
	Traces  []TraceRecord   `json:"traces"`
	DLQ     []DLQRecord     `json:"dlq"`
}

type SystemStatus struct {
	Environment string `json:"environment"`
	KafkaSim    string `json:"kafkaSim"`
	TemporalDev string `json:"temporalDev"`
	Worker      string `json:"worker"`
	API         string `json:"api"`
}

func (event *MediaDeliveryEvent) ApplyDefaults() {
	now := time.Now().UTC()
	if event.EventID == "" {
		event.EventID = fmt.Sprintf("evt_%d", now.UnixNano())
	}
	if event.EventType == "" {
		event.EventType = MediaDeliveredEventType
	}
	if event.Topic == "" {
		event.Topic = MediaDeliveredTopic
	}
	if event.CorrelationID == "" {
		event.CorrelationID = event.EventID
	}
	if event.AssetID == "" {
		event.AssetID = fmt.Sprintf("asset_%s", shortHash(event.EventID))
	}
	if event.ListingID == "" {
		event.ListingID = "listing_1001"
	}
	if event.ContentType == "" {
		event.ContentType = "image/jpeg"
	}
	if event.SourceURL == "" {
		event.SourceURL = fmt.Sprintf("s3://sparkle-local/raw/%s/original.jpg", event.AssetID)
	}
	if event.DestinationURL == "" {
		event.DestinationURL = fmt.Sprintf("s3://sparkle-local/processed/%s", event.AssetID)
	}
	if event.SizeBytes == 0 {
		event.SizeBytes = 2_349_875
	}
	if len(event.RequestedRenditions) == 0 {
		event.RequestedRenditions = []string{"hero", "gallery", "thumbnail"}
	}
	event.RequestedRenditions = sortedUnique(event.RequestedRenditions)
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
}

func (event MediaDeliveryEvent) Validate() error {
	if strings.TrimSpace(event.EventID) == "" {
		return fmt.Errorf("eventId is required")
	}
	if strings.TrimSpace(event.EventType) != MediaDeliveredEventType {
		return fmt.Errorf("unsupported eventType %q", event.EventType)
	}
	if strings.TrimSpace(event.Topic) == "" {
		return fmt.Errorf("topic is required")
	}
	if strings.TrimSpace(event.CorrelationID) == "" {
		return fmt.Errorf("correlationId is required")
	}
	if strings.TrimSpace(event.AssetID) == "" {
		return fmt.Errorf("assetId is required")
	}
	if strings.TrimSpace(event.SourceURL) == "" {
		return fmt.Errorf("sourceUrl is required")
	}
	if strings.TrimSpace(event.DestinationURL) == "" {
		return fmt.Errorf("destinationUrl is required")
	}
	if len(sortedUnique(event.RequestedRenditions)) == 0 {
		return fmt.Errorf("requestedRenditions must include at least one rendition")
	}
	return nil
}

func (input ImageProcessingInput) Validate() error {
	event := MediaDeliveryEvent{
		EventID:             input.EventID,
		EventType:           input.EventType,
		Topic:               input.Topic,
		CorrelationID:       input.CorrelationID,
		AssetID:             input.AssetID,
		ListingID:           input.ListingID,
		SourceURL:           input.SourceURL,
		DestinationURL:      input.DestinationURL,
		ContentType:         input.ContentType,
		SizeBytes:           input.SizeBytes,
		RequestedRenditions: input.RequestedRenditions,
		CreatedAt:           input.StartedAt,
	}
	if err := event.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(input.TraceID) == "" {
		return fmt.Errorf("traceId is required")
	}
	return nil
}

func WorkflowIDForEvent(event MediaDeliveryEvent) string {
	return "sparkle-image-processing-" + sanitize(event.CorrelationID)
}

func TraceIDForEvent(event MediaDeliveryEvent) string {
	return "trace-" + sanitize(event.CorrelationID)
}

func NewImageProcessingInput(event MediaDeliveryEvent) ImageProcessingInput {
	return ImageProcessingInput{
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
		StartedAt:           event.CreatedAt,
	}
}

func NewSampleEvent(index int) MediaDeliveryEvent {
	now := time.Now().UTC()
	event := MediaDeliveryEvent{
		EventID:             fmt.Sprintf("evt_%d_%02d", now.UnixNano(), index),
		EventType:           MediaDeliveredEventType,
		Topic:               MediaDeliveredTopic,
		CorrelationID:       fmt.Sprintf("corr_%d_%02d", now.UnixNano(), index),
		AssetID:             fmt.Sprintf("asset_%04d", 1000+index),
		ListingID:           fmt.Sprintf("listing_%04d", 7000+index),
		ContentType:         "image/jpeg",
		SizeBytes:           int64(2_100_000 + index*74_321),
		RequestedRenditions: []string{"hero", "gallery", "thumbnail"},
		CreatedAt:           now,
	}
	event.ApplyDefaults()
	return event
}

func sanitize(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	for _, current := range value {
		switch {
		case current >= 'a' && current <= 'z':
			builder.WriteRune(current)
		case current >= '0' && current <= '9':
			builder.WriteRune(current)
		case current == '-' || current == '_':
			builder.WriteRune(current)
		default:
			builder.WriteByte('-')
		}
	}
	normalized := strings.Trim(builder.String(), "-")
	if normalized == "" {
		return "unknown"
	}
	return normalized
}

func sortedUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(strings.ToLower(value))
		if trimmed == "" {
			continue
		}
		seen[trimmed] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func StableHash(values ...string) string {
	hasher := fnv.New64a()
	for _, value := range values {
		_, _ = hasher.Write([]byte(value))
		_, _ = hasher.Write([]byte{0})
	}
	return fmt.Sprintf("%x", hasher.Sum64())
}

func shortHash(value string) string {
	hash := StableHash(value)
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
