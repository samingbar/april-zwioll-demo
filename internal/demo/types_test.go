package demo

import "testing"

func TestWorkflowIDForEventIsDeterministic(t *testing.T) {
	event := MediaDeliveryEvent{
		EventID:        "evt-1",
		EventType:      MediaDeliveredEventType,
		Topic:          MediaDeliveredTopic,
		CorrelationID:  "Corr 123/ABC",
		AssetID:        "asset-1",
		SourceURL:      "s3://raw/asset-1.jpg",
		DestinationURL: "s3://processed/asset-1",
	}
	event.ApplyDefaults()

	got := WorkflowIDForEvent(event)
	want := "sparkle-image-processing-corr-123-abc"
	if got != want {
		t.Fatalf("workflow id = %q, want %q", got, want)
	}
}

func TestMediaDeliveryEventDefaultsAndValidation(t *testing.T) {
	event := MediaDeliveryEvent{CorrelationID: "corr-1"}
	event.ApplyDefaults()

	if err := event.Validate(); err != nil {
		t.Fatalf("expected defaulted event to validate: %v", err)
	}
	if event.Topic != MediaDeliveredTopic {
		t.Fatalf("topic = %q, want %q", event.Topic, MediaDeliveredTopic)
	}
	if len(event.RequestedRenditions) != 3 {
		t.Fatalf("expected default renditions, got %v", event.RequestedRenditions)
	}
}
