# Configuração

O `processing-service` do X Tube é configurado por variáveis de ambiente carregadas por `internal/config`.

## Variáveis Obrigatórias

| Variável | Descrição | Obrigatória | Exemplo |
| --- | --- | --- | --- |
| `SQS_VIDEO_PROCESSING_URL` | URL da fila SQS consumida pelo worker de vídeo. | Sim | `https://sqs.example/xtube-video-processing` |
| `SQS_THUMBNAIL_PROCESSING_URL` | URL da fila SQS consumida pelo worker de thumbnail. | Sim | `https://sqs.example/xtube-thumbnail-processing` |
| `S3_BUCKET_INPUT` | Bucket S3 esperado para eventos de vídeo original. | Sim | `xtube-videos-input` |
| `S3_BUCKET_OUTPUT` | Bucket S3 onde chunks de vídeo processado são enviados. | Sim | `xtube-videos-output` |

## Configurações AWS

| Variável | Descrição | Default / Exemplo | Obrigatória |
| --- | --- | --- | --- |
| `AWS_ENDPOINT_URL` | Endpoint compatível com AWS usado por S3 e SQS. | `https://aws-compatible-endpoint.example` | Opcional |
| `AWS_REGION` | Região AWS. Tem precedência sobre `AWS_DEFAULT_REGION`. | `us-east-1` | Opcional |
| `AWS_DEFAULT_REGION` | Região AWS de fallback. | `us-east-1` | Opcional |
| `AWS_ACCESS_KEY_ID` | Access key usada pela cadeia de credenciais do AWS SDK. | específico do ambiente | Opcional, conforme a cadeia de credenciais do SDK |
| `AWS_SECRET_ACCESS_KEY` | Secret key usada pela cadeia de credenciais do AWS SDK. | específico do ambiente | Opcional, conforme a cadeia de credenciais do SDK |

Quando `AWS_ENDPOINT_URL` está definido, o cliente S3 usa path-style. O serviço espera que os endpoints compatíveis com S3/SQS configurados estejam acessíveis; provisioná-los está fora do escopo desta documentação.

## Buckets S3

| Variável | Descrição | Default / Exemplo | Obrigatória |
| --- | --- | --- | --- |
| `S3_BUCKET_INPUT` | Bucket de origem de vídeos. Eventos S3 de vídeo devem vir deste bucket. | `xtube-videos-input` | Sim |
| `S3_BUCKET_OUTPUT` | Bucket de saída de vídeo processado. | `xtube-videos-output` | Sim |
| `S3_BUCKET_THUMBNAILS` | Bucket de thumbnails. Se configurado, eventos de thumbnail devem vir deste bucket. | `xtube-thumbnails` | Opcional |
| `S3_BUCKET_TEMP` | Carregado pela configuração. | `xtube-temp` | Opcional |

`S3_BUCKET_TEMP` não é usado pela implementação atual. Arquivos temporários são criados no filesystem local usando `VIDEO_TEMP_DIR` e `THUMBNAIL_TEMP_DIR`.

## Configurações SQS

| Variável | Descrição | Default / Exemplo | Obrigatória |
| --- | --- | --- | --- |
| `SQS_VIDEO_PROCESSING_URL` | URL da fila do worker de vídeo. | URL da fila | Sim |
| `SQS_VIDEO_PROCESSING_DLQ_URL` | Carregada pela configuração, mas não usada diretamente pela lógica da aplicação. O worker detecta DLQ via `RedrivePolicy` da fila. | URL da DLQ | Opcional |
| `SQS_THUMBNAIL_PROCESSING_URL` | URL da fila do worker de thumbnail. | URL da fila | Sim |
| `SQS_WAIT_TIME_SECONDS` | Tempo de long polling SQS. | `20` | Opcional |
| `SQS_MAX_MESSAGES` | Máximo de mensagens solicitadas por polling. | `10` | Opcional |
| `SQS_ERROR_DELAY_SECONDS` | Delay após erros de polling. | `2` | Opcional |
| `SQS_VIDEO_VISIBILITY_TIMEOUT_SECONDS` | Timeout de visibilidade para mensagens de vídeo. | `3600` | Opcional |
| `SQS_THUMBNAIL_VISIBILITY_TIMEOUT_SECONDS` | Timeout de visibilidade para mensagens de thumbnail. | `600` | Opcional |

A aplicação não configura políticas de redrive. Retry e DLQ são controlados pela configuração da fila SQS.

## Configurações De Vídeo

| Variável | Descrição | Default / Exemplo | Obrigatória |
| --- | --- | --- | --- |
| `VIDEO_PROCESSOR_NAME` | Nome lógico do worker de vídeo em logs e métricas. | `video` | Opcional |
| `VIDEO_TEMP_DIR` | Diretório pai para temporários de vídeo. Vazio usa o temporário padrão do sistema operacional. | vazio | Opcional |
| `VIDEO_SEGMENT_SECONDS` | Duração dos segmentos ffmpeg em segundos. Valores menores ou iguais a zero voltam para `10`. | `10` | Opcional |

Perfis padrão fixos no código:

| Perfil | Altura |
| --- | --- |
| `360p` | `360` |
| `480p` | `480` |
| `720p` | `720` |

Não implementado: não há variável de ambiente para alterar a lista de perfis.

## Configurações De Thumbnail

| Variável | Descrição | Default / Exemplo | Obrigatória |
| --- | --- | --- | --- |
| `THUMBNAIL_PROCESSOR_NAME` | Nome lógico do worker de thumbnail em logs e métricas. | `thumbnail` | Opcional |
| `THUMBNAIL_TEMP_DIR` | Diretório pai para temporários de thumbnail. Vazio usa o temporário padrão do sistema operacional. | vazio | Opcional |
| `THUMBNAIL_RESIZE_FACTOR` | Divisor de redimensionamento das dimensões da thumbnail. | `3` | Opcional |

## Configurações Kafka

| Variável | Descrição | Default / Exemplo | Obrigatória |
| --- | --- | --- | --- |
| `KAFKA_ENABLED` | Habilita publicação Kafka de progresso. Quando false, usa publisher no-op. | `true` | Opcional |
| `KAFKA_BROKERS` | Lista de brokers Kafka separada por vírgula. | `localhost:9092` | Opcional |
| `KAFKA_VIDEO_PROGRESS_TOPIC` | Tópico Kafka para eventos de progresso. | `xtube.video.progress` | Opcional |
| `KAFKA_CLIENT_ID` | Client ID Kafka usado pelo writer transport. | `xtube-processing-service` | Opcional |

Execução na máquina host:

```env
KAFKA_ENABLED=true
KAFKA_BROKERS=localhost:9092
KAFKA_VIDEO_PROGRESS_TOPIC=xtube.video.progress
KAFKA_CLIENT_ID=xtube-processing-service
```

Execução em container quando o serviço consegue resolver o broker via DNS:

```env
KAFKA_BROKERS=kafka:9092
```

## Configurações De Log

| Variável | Descrição | Default | Obrigatória |
| --- | --- | --- | --- |
| `LOG_LEVEL` | Nível do logger. Suporta `debug`, `info`, `warn`, `warning`, `error`. | `info` | Opcional |
| `LOG_CHUNK_DETAILS` | Habilita logs debug de descoberta de segmentos e início/fim de upload de chunks. | `false` | Opcional |
| `LOG_FFMPEG_PROGRESS` | Habilita logs de progresso do ffmpeg. | `true` | Opcional |

## Exemplo De Ambiente Do Serviço

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
