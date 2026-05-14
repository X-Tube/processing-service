# Arquitetura

Este documento descreve apenas a arquitetura do `processing-service` Go do X Tube. O serviço faz parte do X Tube e é responsável pelo processamento assíncrono de mídia.

## Arquitetura De Alto Nível

```mermaid
flowchart TB
    subgraph External["Dependências externas de runtime"]
        S3In[(Buckets S3 de entrada)]
        S3Out[(Buckets S3 de saída)]
        SQSVideo[Fila SQS de processamento de vídeo]
        SQSThumb[Fila SQS de processamento de thumbnail]
        Kafka[(Tópico Kafka: xtube.video.progress)]
    end

    subgraph Service["X Tube processing-service"]
        Main[cmd/processing-service]
        WorkerVideo[worker SQS de vídeo]
        WorkerThumb[worker SQS de thumbnail]
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

O serviço inicia dois workers SQS independentes:

- Um worker de vídeo ligado a `SQS_VIDEO_PROCESSING_URL`.
- Um worker de thumbnail ligado a `SQS_THUMBNAIL_PROCESSING_URL`.

Ambos usam a mesma implementação de object store S3.

## Responsabilidades Dos Pacotes Internos

```mermaid
flowchart LR
    Cmd["cmd/processing-service<br/>raiz de composição"]
    Config["internal/config<br/>leitura e validação de ambiente"]
    AWS["internal/awsclient<br/>clientes AWS SDK"]
    Events["internal/events<br/>parse de mensagens S3/SNS"]
    Messaging["internal/messaging<br/>interfaces e helpers SQS"]
    Worker["internal/worker<br/>polling, renovação de visibilidade e deleção SQS"]
    Processor["internal/processor<br/>interface comum de processador"]
    Storage["internal/storage<br/>abstração de download/upload S3"]
    Video["internal/video<br/>extração, ffmpeg, upload de chunks"]
    Thumb["internal/thumbnail<br/>extração, resize e upload"]
    Progress["internal/progress<br/>publisher Kafka de progresso"]
    Obs["internal/observability<br/>logs, métricas e health"]

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

| Pacote | Responsabilidade |
| --- | --- |
| `cmd/processing-service` | Carrega configuração, cria logger, clientes AWS, store S3, publisher Kafka, processadores e workers. |
| `internal/config` | Lê variáveis de ambiente, aplica defaults e valida valores obrigatórios. |
| `internal/awsclient` | Cria clientes S3 e SQS do AWS SDK, incluindo endpoint customizado opcional. |
| `internal/events` | Faz parse de eventos S3, mensagens S3 encapsuladas em SNS e ignora eventos de teste do S3. |
| `internal/messaging` | Define a interface SQS e mascara receipt handles em logs. |
| `internal/worker` | Faz polling SQS, renova visibilidade, chama processadores, registra métricas e deleta mensagens após sucesso. |
| `internal/processor` | Define a interface comum `Name` e `Process` usada pelos workers. |
| `internal/storage` | Baixa objetos S3 para arquivos e faz upload de arquivos com content type inferido. |
| `internal/video` | Valida eventos de vídeo, deriva `video_id`, executa ffmpeg, faz upload de chunks e publica progresso Kafka. |
| `internal/thumbnail` | Valida eventos de thumbnail, redimensiona imagens e envia original e versão reduzida. |
| `internal/progress` | Define o contrato do evento de progresso e a implementação Kafka. |
| `internal/observability` | Fornece logger JSON, métricas Prometheus, `/health` e `/metrics`. |

## Modelo De Runtime

```mermaid
sequenceDiagram
    participant Main as main()
    participant Config as config.Load
    participant AWS as clientes AWS SDK
    participant Kafka as publisher de progresso
    participant VW as worker de vídeo
    participant TW as worker de thumbnail
    participant HTTP as servidor de observabilidade

    Main->>Config: carregar env
    Main->>HTTP: iniciar :9090
    Main->>AWS: criar clientes SQS e S3
    Main->>Kafka: criar publisher ou no-op
    Main->>VW: iniciar goroutine
    Main->>TW: iniciar goroutine
    Main->>Main: aguardar SIGINT/SIGTERM
```

## Integração Com AWS SDK

O serviço usa AWS SDK for Go v2.

- `AWS_ENDPOINT_URL` é opcional. Quando definido, S3 e SQS usam o endpoint customizado.
- S3 usa path-style quando `AWS_ENDPOINT_URL` está definido.
- Credenciais são carregadas pela cadeia padrão do AWS SDK.
- O serviço espera que os buckets S3 e filas SQS configurados já existam. Provisionar esses recursos está fora do escopo desta documentação.

## Integração Kafka

A publicação de progresso Kafka está implementada em `internal/progress`.

- O tópico padrão é `xtube.video.progress`.
- `KAFKA_BROKERS` aceita brokers separados por vírgula.
- `KAFKA_ENABLED=false` substitui Kafka por um publisher no-op.
- Se Kafka estiver habilitado e um evento de progresso não puder ser publicado, o processamento de vídeo falha e a mensagem SQS não é deletada.
- O serviço espera que o tópico configurado já exista. Provisionar Kafka está fora do escopo desta documentação.

## Observabilidade

O serviço inicia um servidor HTTP em `:9090` com:

| Endpoint | Comportamento |
| --- | --- |
| `/health` | Retorna `200 OK` e corpo `ok`. |
| `/metrics` | Métricas Prometheus via `promhttp.Handler`. |

As métricas atuais cobrem processamento SQS:

| Métrica | Labels |
| --- | --- |
| `sqs_messages_received_total` | `queue` |
| `sqs_messages_processed_total` | `queue`, `status` |
| `sqs_messages_deleted_total` | `queue` |
| `sqs_processing_duration_seconds` | `queue` |

## Não Responsabilidades

Este repositório não implementa:

- API de upload.
- API de playback.
- Autenticação ou autorização.
- Catálogo ou recomendação.
- Frontend.
- Qualquer API HTTP além de health e métricas.
