# Sparkle Trace Demo

A local Rich Media workflow tracing demo that connects a Kafka-like event stream, Temporal workflow execution, a file-backed read model, and a Sparkle-style React UI.

## Prerequisites

- Go 1.22+
- Node 20+
- Temporal CLI

## Run Locally

Terminal 1:

```bash
temporal server start-dev
```

Terminal 2:

```bash
go run ./cmd/sparkle-demo
```

Terminal 3:

```bash
npm install
npm run dev
```

Open `http://127.0.0.1:5173`.

## What The Demo Shows

- A local in-process Kafka-like topic accepts `richmedia.media.delivered` events.
- A bridge consumes the topic and starts `ImageProcessingWorkflow` with deterministic workflow IDs.
- Temporal runs `DownloadImage`, `ProcessImage`, `UploadImage`, and read-model projection activities.
- The Sparkle Trace UI streams the read model over Server-Sent Events and shows the event, workflow timeline, activity log, DLQ, metrics, and pizza tracker.

For the stakeholder-level walkthrough and source-question mapping, see [`docs/demo-breakdown.md`](docs/demo-breakdown.md).

## API Shortcuts

```bash
curl -X POST http://127.0.0.1:8080/api/demo/sample
curl -X POST 'http://127.0.0.1:8080/api/demo/burst?count=5'
curl -X POST http://127.0.0.1:8080/api/demo/failure
curl http://127.0.0.1:8080/api/snapshot
```

The read model persists to `data/read-model.json`.
# april-zwioll-demo
