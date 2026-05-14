# Operations

This guide covers runtime and operational usage for the X Tube Go `processing-service` only. It intentionally does not explain how to provision S3, SQS, Kafka, local cloud emulators, buckets, queues, notifications, or Kafka topics.

## Runtime Prerequisites

| Requirement           | Purpose                                                                          |
| --------------------- | -------------------------------------------------------------------------------- |
| Go                    | Build, test, and run the service directly. The module declares Go `1.24.4`.      |
| S3-compatible storage | The service downloads input media and uploads processed outputs.                 |
| SQS-compatible queues | The service receives asynchronous video and thumbnail jobs.                      |
| Kafka                 | The service publishes video progress events when `KAFKA_ENABLED=true`.           |
| FFmpeg                | The service uses the `ffmpeg` executable for video transcoding and segmentation. |

External resources must already exist and be reachable through the configured environment variables.

## FFmpeg Runtime Requirement

The Go code invokes `ffmpeg` as an external process from `internal/video/transcoder.go`.

- Docker execution: the provided Dockerfile installs `ffmpeg` in the runtime image.
- Host execution: `ffmpeg` must be installed and available in the host `PATH`.
- If `ffmpeg` is missing or fails, video processing returns an error and the SQS message is not deleted.

Verify host availability:

```bash
ffmpeg -version
```

## Run The Service Locally

Create or load an environment file for this service. The file should contain the variables documented in [configuration.md](configuration.md). Then run:

```bash
set -a
source .env
set +a

go run ./cmd/processing-service
```

To run without Kafka progress publishing:

```bash
export KAFKA_ENABLED=false
go run ./cmd/processing-service
```

## Build And Run With Docker

Build the service image:

```bash
docker build -t xtube-processing-service .
```

Run it with an environment file:

```bash
docker run --rm \
  --env-file .env \
  -p 9090:9090 \
  xtube-processing-service
```

When running in Docker, ensure the container can reach the configured S3, SQS, and Kafka endpoints. For Kafka, `KAFKA_BROKERS` must be resolvable from inside the container.

## Run Tests

```bash
go test ./...
```

## Health And Metrics

The service starts an observability server on `:9090`.

```bash
curl http://localhost:9090/health
curl http://localhost:9090/metrics
```

Expected health response:

```text
ok
```

## Service-Level Integration Checks

The service expects:

| Dependency             | Service contract                                                                              |
| ---------------------- | --------------------------------------------------------------------------------------------- |
| S3 input video bucket  | `S3_BUCKET_INPUT` exists and S3 event messages point to objects in that bucket.               |
| S3 output video bucket | `S3_BUCKET_OUTPUT` exists and accepts processed chunk uploads.                                |
| S3 thumbnail bucket    | `S3_BUCKET_THUMBNAILS` exists if thumbnail bucket validation is enabled by configuration.     |
| SQS video queue        | `SQS_VIDEO_PROCESSING_URL` points to a queue containing S3 event messages for videos.         |
| SQS thumbnail queue    | `SQS_THUMBNAIL_PROCESSING_URL` points to a queue containing S3 event messages for thumbnails. |
| Kafka topic            | `KAFKA_VIDEO_PROGRESS_TOPIC` exists when Kafka is enabled.                                    |

Provisioning these dependencies is outside the scope of this service documentation.

## Expected S3 Keys

Video input examples:

```text
uploads/music.mp4
uploads/video-123/original.mp4
```

Processed video output examples:

```text
music/360p/video-1.mp4
music/480p/video-1.mp4
music/720p/video-1.mp4
```

Thumbnail input example:

```text
uploads/maxresdefault.jpg
```

Processed thumbnail output examples:

```text
processed/maxresdefault/original.jpg
processed/maxresdefault/3x.jpg
```

## Kafka Progress Consumption

Kafka client tooling is outside this service. Any Kafka consumer can read the configured topic. The service publishes messages with key `video_id` and this JSON payload:

```json
{
  "video_id": "video-id",
  "progress_percent": 37
}
```

Messages appear only after each processed video chunk is successfully uploaded to S3.

## Logging

Logs are JSON objects emitted through `slog`.

Typical fields include:

| Field           | Meaning                                                                                            |
| --------------- | -------------------------------------------------------------------------------------------------- |
| `component`     | Logical component such as `service`, `worker`, `processor`, `ffmpeg`, `kafka`, or `observability`. |
| `worker`        | Worker name, usually `video` or `thumbnail`.                                                       |
| `queue`         | Worker queue label.                                                                                |
| `bucket`, `key` | S3 location being processed.                                                                       |
| `video_id`      | Derived video identifier for video jobs.                                                           |
| `thumbnail_id`  | Derived thumbnail identifier for thumbnail jobs.                                                   |
| `duration_ms`   | Operation duration in milliseconds.                                                                |
| `error`         | Error details for failed operations.                                                               |

Important lifecycle logs:

- `processing service started`
- `observability server started`
- `sqs worker started`
- `sqs message received`
- `video download started` / `video download finished`
- `video segmentation started` / `video segmentation finished`
- `video upload finished`
- `thumbnail processing started` / `thumbnail processing finished`
- `sqs message deleted`

`LOG_FFMPEG_PROGRESS=true` emits ffmpeg progress lines. `LOG_CHUNK_DETAILS=true` enables debug logs for individual segment discovery and chunk upload start/finish.

## Troubleshooting

| Symptom                            | Check                                                                                                                                                                                 |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Service exits at startup           | Required env vars are missing: `SQS_VIDEO_PROCESSING_URL`, `SQS_THUMBNAIL_PROCESSING_URL`, `S3_BUCKET_INPUT`, `S3_BUCKET_OUTPUT`.                                                     |
| S3 download or upload fails        | Confirm `AWS_ENDPOINT_URL`, AWS credentials, bucket names, and object keys.                                                                                                           |
| SQS polling fails                  | Confirm queue URLs, AWS credentials, region, and network access.                                                                                                                      |
| Video processing fails immediately | Confirm `ffmpeg` is installed and available in `PATH`, or run through the provided Docker image.                                                                                      |
| Kafka progress publish fails       | Confirm `KAFKA_BROKERS`, `KAFKA_VIDEO_PROGRESS_TOPIC`, network access, and topic availability. Use `KAFKA_ENABLED=false` only when progress events are optional for that environment. |
| Thumbnail outputs trigger new jobs | The processor rejects non-`uploads/` keys; ensure only intended S3 events are delivered to the thumbnail queue.                                                                       |
| SQS messages keep reappearing      | Processing is failing or Kafka publish is failing. Check worker error logs and the external SQS redrive policy.                                                                       |

## Recommendations

- Keep video and thumbnail SQS queues configured with a redrive policy outside the application.
- Keep `SQS_VIDEO_VISIBILITY_TIMEOUT_SECONDS` longer than expected video processing time; the worker renews visibility, but very short values increase operational risk.
- Create and manage Kafka topics outside the application. The service expects the configured topic to exist.
