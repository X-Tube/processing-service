# Architecture

This document describes the architecture of the X Tube Go `processing-service` only. The service is part of X Tube and is responsible for asynchronous media processing.

## High-Level Architecture

```mermaid
flowchart TB
    subgraph External["External runtime dependencies"]
        S3In[(S3 input buckets)]
        S3Out[(S3 output buckets)]
        SQSVideo[SQS video processing queue]
        SQSThumb[SQS thumbnail processing queue]
        Kafka[(Kafka topic: xtube.video.progress)]
    end

    subgraph Service["X Tube processing-service"]
        Main[cmd/processing-service]
        WorkerVideo[video SQS worker]
        WorkerThumb[thumbnail SQS worker]
        Video[internal/video]
        Thumb[internal/thumbnail]
        Store[internal/storage]
        Progress[internal/progress]
        Obs[internal/observability]
    end

    SQSVideo --> WorkerVideo
    SQSThumb --> WorkerThumb
    WorkerVideo --> Video
    WorkerThumb --> Thumb
    Video --> Store
    Thumb --> Store
    Store --> S3In
    Store --> S3Out
    Video --> Progress
    Progress --> Kafka
    Main --> Obs
```

The service starts two independent SQS workers:

- A video worker bound to `SQS_VIDEO_PROCESSING_URL`.
- A thumbnail worker bound to `SQS_THUMBNAIL_PROCESSING_URL`.

Both workers use the same S3 object store implementation.

## Internal Package Responsibilities

```mermaid
flowchart LR
    Cmd["cmd/processing-service<br/>composition root"]
    Config["internal/config<br/>environment loading and validation"]
    AWS["internal/awsclient<br/>AWS SDK clients"]
    Events["internal/events<br/>S3/SNS message parsing"]
    Messaging["internal/messaging<br/>SQS client interface helpers"]
    Worker["internal/worker<br/>SQS polling, visibility renewal, deletion"]
    Processor["internal/processor<br/>common processor interface"]
    Storage["internal/storage<br/>S3 download/upload abstraction"]
    Video["internal/video<br/>video extraction, ffmpeg segmentation, chunk upload"]
    Thumb["internal/thumbnail<br/>thumbnail extraction, resize, upload"]
    Progress["internal/progress<br/>Kafka progress publisher"]
    Obs["internal/observability<br/>logs, metrics, health"]

    Cmd --> Config
    Cmd --> AWS
    Cmd --> Worker
    Cmd --> Video
    Cmd --> Thumb
    Cmd --> Progress
    Worker --> Processor
    Worker --> Events
    Worker --> Messaging
    Video --> Events
    Video --> Storage
    Video --> Progress
    Thumb --> Events
    Thumb --> Storage
    AWS --> Storage
    Cmd --> Obs
    Worker --> Obs
    Video --> Obs
    Thumb --> Obs
```

| Package | Responsibility |
| --- | --- |
| `cmd/processing-service` | Loads config, creates logger, AWS clients, S3 store, Kafka publisher, processors, and workers. |
| `internal/config` | Reads environment variables, applies defaults, and validates required values. |
| `internal/awsclient` | Builds AWS SDK S3 and SQS clients, including optional custom endpoint overrides. |
| `internal/events` | Parses S3 event messages, SNS-wrapped S3 messages, and ignores S3 test events. |
| `internal/messaging` | Defines the SQS client interface and masks receipt handles for logs. |
| `internal/worker` | Polls SQS, renews message visibility, invokes processors, records metrics, and deletes messages after success. |
| `internal/processor` | Defines the common `Name` and `Process` interface used by workers. |
| `internal/storage` | Downloads S3 objects to files and uploads files to S3 with inferred content type. |
| `internal/video` | Validates video events, derives `video_id`, runs ffmpeg, uploads chunks, and publishes Kafka progress. |
| `internal/thumbnail` | Validates thumbnail events, resizes images, uploads original and resized thumbnails. |
| `internal/progress` | Defines the video progress event contract and Kafka writer implementation. |
| `internal/observability` | Provides JSON logger setup, Prometheus metrics, `/health`, and `/metrics`. |

## Runtime Model

```mermaid
sequenceDiagram
    participant Main as main()
    participant Config as config.Load
    participant AWS as AWS SDK clients
    participant Kafka as progress publisher
    participant VW as Video worker
    participant TW as Thumbnail worker
    participant HTTP as Observability server

    Main->>Config: load env
    Main->>HTTP: start :9090
    Main->>AWS: create SQS and S3 clients
    Main->>Kafka: create publisher or no-op
    Main->>VW: start goroutine
    Main->>TW: start goroutine
    Main->>Main: wait for SIGINT/SIGTERM
```

## AWS SDK Integration

The service uses AWS SDK for Go v2.

- `AWS_ENDPOINT_URL` is optional. When set, both S3 and SQS use the custom endpoint.
- S3 uses path-style addressing when `AWS_ENDPOINT_URL` is set.
- Credentials are loaded by the AWS SDK default chain.
- The service expects the configured S3 buckets and SQS queues to exist. Provisioning those resources is outside this service documentation.

## Kafka Integration

Kafka progress publishing is implemented in `internal/progress`.

- The default topic is `xtube.video.progress`.
- `KAFKA_BROKERS` accepts comma-separated brokers.
- `KAFKA_ENABLED=false` replaces Kafka with a no-op publisher.
- If Kafka is enabled and a progress event cannot be published, video processing fails and the SQS message is not deleted.
- The service expects the configured topic to exist. Kafka provisioning is outside this service documentation.

## Observability

The service starts an HTTP server on `:9090` with:

| Endpoint | Behavior |
| --- | --- |
| `/health` | Returns `200 OK` and body `ok`. |
| `/metrics` | Prometheus metrics via `promhttp.Handler`. |

Metrics currently cover SQS processing:

| Metric | Labels |
| --- | --- |
| `sqs_messages_received_total` | `queue` |
| `sqs_messages_processed_total` | `queue`, `status` |
| `sqs_messages_deleted_total` | `queue` |
| `sqs_processing_duration_seconds` | `queue` |

## Explicit Non-Responsibilities

This repository does not implement:

- Upload API.
- Playback API.
- Authentication or authorization.
- Catalog or recommendation logic.
- Frontend behavior.
- Any HTTP API beyond health and metrics.
