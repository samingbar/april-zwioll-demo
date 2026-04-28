package demo

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Activities struct {
	Store *Store
}

func (activities *Activities) ProjectWorkflowStateActivity(_ context.Context, input ProjectionInput) (ProjectionOutput, error) {
	if activities == nil || activities.Store == nil {
		return ProjectionOutput{}, fmt.Errorf("read model store is required")
	}
	return activities.Store.ProjectWorkflowState(input)
}

func (activities *Activities) DownloadImageActivity(ctx context.Context, input DownloadImageInput) (DownloadImageOutput, error) {
	if strings.TrimSpace(input.AssetID) == "" {
		return DownloadImageOutput{}, fmt.Errorf("assetId is required")
	}
	if strings.TrimSpace(input.SourceURL) == "" {
		return DownloadImageOutput{}, fmt.Errorf("sourceUrl is required")
	}
	if strings.Contains(strings.ToLower(input.SourceURL), "fail-download") {
		return DownloadImageOutput{}, fmt.Errorf("simulated download failure for %s", input.SourceURL)
	}
	if err := sleepWithContext(ctx, 700*time.Millisecond); err != nil {
		return DownloadImageOutput{}, err
	}
	checksum := StableHash(input.AssetID, input.SourceURL, fmt.Sprintf("%d", input.SizeBytes))
	return DownloadImageOutput{
		AssetID:       input.AssetID,
		LocalURI:      fmt.Sprintf("file://local-cache/%s/%s-original.jpg", input.TraceID, input.AssetID),
		AssetChecksum: checksum,
	}, nil
}

func (activities *Activities) ProcessImageActivity(ctx context.Context, input ProcessImageInput) (ProcessImageOutput, error) {
	if strings.TrimSpace(input.AssetID) == "" {
		return ProcessImageOutput{}, fmt.Errorf("assetId is required")
	}
	if strings.TrimSpace(input.LocalURI) == "" {
		return ProcessImageOutput{}, fmt.Errorf("localUri is required")
	}
	if strings.Contains(strings.ToLower(input.AssetID), "fail-process") {
		return ProcessImageOutput{}, fmt.Errorf("simulated processing failure for %s", input.AssetID)
	}
	if err := sleepWithContext(ctx, 900*time.Millisecond); err != nil {
		return ProcessImageOutput{}, err
	}

	names := sortedUnique(input.RequestedRenditions)
	if len(names) == 0 {
		names = []string{"hero", "gallery", "thumbnail"}
	}

	renditions := make([]Rendition, 0, len(names))
	for _, name := range names {
		width, height := renditionDimensions(name)
		renditions = append(renditions, Rendition{
			Name:   name,
			URI:    fmt.Sprintf("s3://sparkle-local/processed/%s/%s/%s-%s.webp", input.ListingID, input.AssetID, input.AssetChecksum, name),
			Width:  width,
			Height: height,
		})
	}
	return ProcessImageOutput{Renditions: renditions}, nil
}

func (activities *Activities) UploadImageActivity(ctx context.Context, input UploadImageInput) (UploadImageOutput, error) {
	if strings.TrimSpace(input.AssetID) == "" {
		return UploadImageOutput{}, fmt.Errorf("assetId is required")
	}
	if strings.TrimSpace(input.DestinationURL) == "" {
		return UploadImageOutput{}, fmt.Errorf("destinationUrl is required")
	}
	if len(input.Renditions) == 0 {
		return UploadImageOutput{}, fmt.Errorf("renditions are required")
	}
	if strings.Contains(strings.ToLower(input.DestinationURL), "fail-upload") {
		return UploadImageOutput{}, fmt.Errorf("simulated upload failure for %s", input.DestinationURL)
	}
	if err := sleepWithContext(ctx, 650*time.Millisecond); err != nil {
		return UploadImageOutput{}, err
	}
	return UploadImageOutput{
		PublishedURL: fmt.Sprintf("https://sparkle.local/listings/%s/assets/%s", input.ListingID, input.AssetID),
		Renditions:   append([]Rendition(nil), input.Renditions...),
	}, nil
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func renditionDimensions(name string) (int, int) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "hero":
		return 2400, 1600
	case "gallery":
		return 1800, 1200
	case "thumbnail":
		return 640, 480
	default:
		return 1600, 1200
	}
}
