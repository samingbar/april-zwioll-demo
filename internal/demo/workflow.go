package demo

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	projectWorkflowStateActivityName = "ProjectWorkflowStateActivity"
	downloadImageActivityName        = "DownloadImageActivity"
	processImageActivityName         = "ProcessImageActivity"
	uploadImageActivityName          = "UploadImageActivity"
)

func ImageProcessingWorkflow(ctx workflow.Context, input ImageProcessingInput) (ImageProcessingOutput, error) {
	if err := input.Validate(); err != nil {
		return ImageProcessingOutput{}, temporal.NewNonRetryableApplicationError(
			fmt.Sprintf("invalid image processing input: %v", err),
			"InvalidImageProcessingInput",
			err,
		)
	}

	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 20 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    3,
		},
	})

	info := workflow.GetInfo(ctx)
	workflowID := info.WorkflowExecution.ID
	runID := info.WorkflowExecution.RunID

	if err := project(ctx, input, workflowID, runID, "downloading", StepStatusRunning, StageDownloading, "DownloadImage activity started", downloadImageActivityName, "", nil, ""); err != nil {
		return ImageProcessingOutput{}, err
	}

	var downloaded DownloadImageOutput
	if err := workflow.ExecuteActivity(ctx, downloadImageActivityName, DownloadImageInput{
		TraceID:   input.TraceID,
		AssetID:   input.AssetID,
		SourceURL: input.SourceURL,
		SizeBytes: input.SizeBytes,
	}).Get(ctx, &downloaded); err != nil {
		_ = project(ctx, input, workflowID, runID, "downloading", StepStatusFailed, StageFailed, "DownloadImage activity failed", downloadImageActivityName, err.Error(), nil, "")
		return ImageProcessingOutput{}, err
	}

	if err := project(ctx, input, workflowID, runID, "downloading", StepStatusCompleted, StageDownloading, "Downloaded source asset and verified checksum", downloadImageActivityName, "", nil, ""); err != nil {
		return ImageProcessingOutput{}, err
	}
	if err := project(ctx, input, workflowID, runID, "processing", StepStatusRunning, StageProcessing, "ProcessImage activity started", processImageActivityName, "", nil, ""); err != nil {
		return ImageProcessingOutput{}, err
	}

	var processed ProcessImageOutput
	if err := workflow.ExecuteActivity(ctx, processImageActivityName, ProcessImageInput{
		TraceID:             input.TraceID,
		AssetID:             input.AssetID,
		ListingID:           input.ListingID,
		LocalURI:            downloaded.LocalURI,
		AssetChecksum:       downloaded.AssetChecksum,
		RequestedRenditions: input.RequestedRenditions,
	}).Get(ctx, &processed); err != nil {
		_ = project(ctx, input, workflowID, runID, "processing", StepStatusFailed, StageFailed, "ProcessImage activity failed", processImageActivityName, err.Error(), nil, "")
		return ImageProcessingOutput{}, err
	}

	if err := project(ctx, input, workflowID, runID, "processing", StepStatusCompleted, StageProcessing, "Generated rendition plan", processImageActivityName, "", processed.Renditions, ""); err != nil {
		return ImageProcessingOutput{}, err
	}
	if err := project(ctx, input, workflowID, runID, "uploading", StepStatusRunning, StageUploading, "UploadImage activity started", uploadImageActivityName, "", processed.Renditions, ""); err != nil {
		return ImageProcessingOutput{}, err
	}

	var uploaded UploadImageOutput
	if err := workflow.ExecuteActivity(ctx, uploadImageActivityName, UploadImageInput{
		TraceID:        input.TraceID,
		AssetID:        input.AssetID,
		ListingID:      input.ListingID,
		DestinationURL: input.DestinationURL,
		Renditions:     processed.Renditions,
	}).Get(ctx, &uploaded); err != nil {
		_ = project(ctx, input, workflowID, runID, "uploading", StepStatusFailed, StageFailed, "UploadImage activity failed", uploadImageActivityName, err.Error(), processed.Renditions, "")
		return ImageProcessingOutput{}, err
	}

	if err := project(ctx, input, workflowID, runID, "uploading", StepStatusCompleted, StageUploading, "Uploaded processed renditions", uploadImageActivityName, "", uploaded.Renditions, uploaded.PublishedURL); err != nil {
		return ImageProcessingOutput{}, err
	}
	if err := project(ctx, input, workflowID, runID, "projected", StepStatusCompleted, StageProjected, "Read model updated through workflow activity", projectWorkflowStateActivityName, "", uploaded.Renditions, uploaded.PublishedURL); err != nil {
		return ImageProcessingOutput{}, err
	}
	if err := project(ctx, input, workflowID, runID, "visible", StepStatusCompleted, StageCompleted, "Sparkle API can serve the completed trace", projectWorkflowStateActivityName, "", uploaded.Renditions, uploaded.PublishedURL); err != nil {
		return ImageProcessingOutput{}, err
	}

	return ImageProcessingOutput{
		TraceID:       input.TraceID,
		WorkflowID:    workflowID,
		AssetID:       input.AssetID,
		PublishedURL:  uploaded.PublishedURL,
		Renditions:    uploaded.Renditions,
		AssetChecksum: downloaded.AssetChecksum,
		Status:        TraceStatusCompleted,
	}, nil
}

func project(
	ctx workflow.Context,
	input ImageProcessingInput,
	workflowID string,
	runID string,
	stepKey string,
	stepStatus string,
	stage string,
	detail string,
	activityName string,
	errorMessage string,
	renditions []Rendition,
	publishedURL string,
) error {
	var output ProjectionOutput
	return workflow.ExecuteActivity(ctx, projectWorkflowStateActivityName, ProjectionInput{
		TraceID:      input.TraceID,
		WorkflowID:   workflowID,
		RunID:        runID,
		StepKey:      stepKey,
		StepStatus:   stepStatus,
		Stage:        stage,
		Detail:       detail,
		ActivityName: activityName,
		ErrorMessage: errorMessage,
		Renditions:   renditions,
		PublishedURL: publishedURL,
		OccurredAt:   workflow.Now(ctx),
	}).Get(ctx, &output)
}
