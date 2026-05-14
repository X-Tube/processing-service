# Configuration

The X Tube `processing-service` is configured through environment variables loaded by `internal/config`.

## Required Variables

| Variable | Description | Required | Example |
| --- | --- | --- | --- |
| `SQS_VIDEO_PROCESSING_URL` | SQS queue URL consumed by the video worker. | Yes | `https://sqs.example/xtube-video-processing` |
| `SQS_THUMBNAIL_PROCESSING_URL` | SQS queue URL consumed by the thumbnail worker. | Yes | `https://sqs.example/xtube-thumbnail-processing` |
| `S3_BUCKET_INPUT` | S3 bucket expected for source video events. | Yes | `xtube-videos-input` |
| `S3_BUCKET_OUTPUT` | S3 bucket where processed video chunks are uploaded. | Yes | `xtube-videos-output` |

## AWS Settings

| Variable | Description | Default / Example | Required |
| --- | --- | --- | --- |
| `AWS_ENDPOINT_URL` | Custom AWS-compatible endpoint used by S3 and SQS clients. | `https://aws-compatible-endpoint.example` | Optional |
| `AWS_REGION` | AWS region. Takes precedence over `AWS_DEFAULT_REGION`. | `us-east-1` | Optional |
| `AWS_DEFAULT_REGION` | Fallback AWS region. | `us-east-1` | Optional |
| `AWS_ACCESS_KEY_ID` | AWS access key used by the AWS SDK credential chain. | environment-specific | Optional, depending on SDK credential chain |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key used by the AWS SDK credential chain. | environment-specific | Optional, depending on SDK credential chain |

When `AWS_ENDPOINT_URL` is set, the S3 client uses path-style addressing. The service expects the configured S3/SQS-compatible endpoints to be reachable; provisioning them is outside the scope of this documentation.

## S3 Buckets

| Variable | Description | Default / Example | Required |
| --- | --- | --- | --- |
| `S3_BUCKET_INPUT` | Source video bucket. Video S3 events must come from this bucket. | `xtube-videos-input` | Yes |
| `S3_BUCKET_OUTPUT` | Processed video output bucket. | `xtube-videos-output` | Yes |
| `S3_BUCKET_THUMBNAILS` | Thumbnail bucket. If configured, thumbnail events must come from this bucket. | `xtube-thumbnails` | Optional |
| `S3_BUCKET_TEMP` | Loaded by configuration. | `xtube-temp` | Optional |

`S3_BUCKET_TEMP` is not used by the current implementation. Temporary files are created on the local filesystem using `VIDEO_TEMP_DIR` and `THUMBNAIL_TEMP_DIR`.

## SQS Settings

| Variable | Description | Default / Example | Required |
| --- | --- | --- | --- |
| `SQS_VIDEO_PROCESSING_URL` | Video worker queue URL. | queue URL | Yes |
| `SQS_VIDEO_PROCESSING_DLQ_URL` | Loaded by configuration but not used directly by application logic. The worker detects DLQ through queue `RedrivePolicy`. | DLQ URL | Optional |
| `SQS_THUMBNAIL_PROCESSING_URL` | Thumbnail worker queue URL. | queue URL | Yes |
| `SQS_WAIT_TIME_SECONDS` | SQS long polling wait time. | `20` | Optional |
| `SQS_MAX_MESSAGES` | Maximum messages requested per poll. | `10` | Optional |
| `SQS_ERROR_DELAY_SECONDS` | Delay after polling errors. | `2` | Optional |
| `SQS_VIDEO_VISIBILITY_TIMEOUT_SECONDS` | Visibility timeout for video messages. | `3600` | Optional |
| `SQS_THUMBNAIL_VISIBILITY_TIMEOUT_SECONDS` | Visibility timeout for thumbnail messages. | `600` | Optional |

The application does not configure queue redrive policies. Retry and DLQ behavior are controlled by SQS queue configuration.

## Video Settings

| Variable | Description | Default / Example | Required |
| --- | --- | --- | --- |
| `VIDEO_PROCESSOR_NAME` | Logical worker name for video logs and metrics. | `video` | Optional |
| `VIDEO_TEMP_DIR` | Parent directory for temporary video processing directories. Empty means the OS default temp directory. | empty | Optional |
| `VIDEO_SEGMENT_SECONDS` | ffmpeg segment duration in seconds. Values less than or equal to zero fall back to `10`. | `10` | Optional |

Default profiles are fixed in code:

| Profile | Height |
| --- | --- |
| `360p` | `360` |
| `480p` | `480` |
| `720p` | `720` |

Not implemented: there is no environment variable for changing the profile list.

## Thumbnail Settings

| Variable | Description | Default / Example | Required |
| --- | --- | --- | --- |
| `THUMBNAIL_PROCESSOR_NAME` | Logical worker name for thumbnail logs and metrics. | `thumbnail` | Optional |
| `THUMBNAIL_TEMP_DIR` | Parent directory for temporary thumbnail processing directories. Empty means the OS default temp directory. | empty | Optional |
| `THUMBNAIL_RESIZE_FACTOR` | Resize divisor for thumbnail output dimensions. | `3` | Optional |

## Kafka Settings

| Variable | Description | Default / Example | Required |
| --- | --- | --- | --- |
| `KAFKA_ENABLED` | Enables Kafka progress publishing. When false, a no-op publisher is used. | `true` | Optional |
| `KAFKA_BROKERS` | Comma-separated Kafka broker list. | `localhost:9092` | Optional |
| `KAFKA_VIDEO_PROGRESS_TOPIC` | Kafka topic for video progress events. | `xtube.video.progress` | Optional |
| `KAFKA_CLIENT_ID` | Kafka client ID used by the writer transport. | `xtube-processing-service` | Optional |

Host execution:

```env
KAFKA_ENABLED=true
KAFKA_BROKERS=localhost:9092
KAFKA_VIDEO_PROGRESS_TOPIC=xtube.video.progress
KAFKA_CLIENT_ID=xtube-processing-service
```

Container execution when the service can resolve the broker by service DNS:

```env
KAFKA_BROKERS=kafka:9092
```

## Logging Settings

| Variable | Description | Default | Required |
| --- | --- | --- | --- |
| `LOG_LEVEL` | Logger level. Supports `debug`, `info`, `warn`, `warning`, `error`. | `info` | Optional |
| `LOG_CHUNK_DETAILS` | Enables debug logs for video segment discovery and chunk upload start/finish. | `false` | Optional |
| `LOG_FFMPEG_PROGRESS` | Enables ffmpeg progress logs. | `true` | Optional |

## Example Service Environment

```env
AWS_ENDPOINT_URL=http://s3-sqs-endpoint.example:4566
AWS_REGION=us-east-1
AWS_DEFAULT_REGION=us-east-1
AWS_ACCESS_KEY_ID=example
AWS_SECRET_ACCESS_KEY=example

S3_BUCKET_INPUT=xtube-videos-input
S3_BUCKET_OUTPUT=xtube-videos-output
S3_BUCKET_THUMBNAILS=xtube-thumbnails
S3_BUCKET_TEMP=xtube-temp

SQS_VIDEO_PROCESSING_URL=https://sqs.example/xtube-video-processing
SQS_VIDEO_PROCESSING_DLQ_URL=https://sqs.example/xtube-video-processing-dlq
SQS_THUMBNAIL_PROCESSING_URL=https://sqs.example/xtube-thumbnail-processing

KAFKA_ENABLED=true
KAFKA_BROKERS=localhost:9092
KAFKA_VIDEO_PROGRESS_TOPIC=xtube.video.progress
KAFKA_CLIENT_ID=xtube-processing-service
```
