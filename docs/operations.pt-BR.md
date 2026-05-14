# Operações

Este guia cobre runtime e uso operacional apenas do `processing-service` Go do X Tube. Ele intencionalmente não explica como provisionar S3, SQS, Kafka, emuladores locais, buckets, filas, notificações ou tópicos Kafka.

## Pré-Requisitos De Runtime

| Requisito                 | Finalidade                                                                        |
| ------------------------- | --------------------------------------------------------------------------------- |
| Go                        | Compilar, testar e executar o serviço diretamente. O módulo declara Go `1.24.4`.  |
| Storage compatível com S3 | O serviço baixa mídia de entrada e envia saídas processadas.                      |
| Filas compatíveis com SQS | O serviço recebe jobs assíncronos de vídeo e thumbnail.                           |
| Kafka                     | O serviço publica eventos de progresso de vídeo quando `KAFKA_ENABLED=true`.      |
| FFmpeg                    | O serviço usa o executável `ffmpeg` para transcodificação e segmentação de vídeo. |

Recursos externos devem existir previamente e estar acessíveis pelas variáveis de ambiente configuradas.

## Requisito De Runtime Do FFmpeg

O código Go invoca `ffmpeg` como um processo externo a partir de `internal/video/transcoder.go`.

- Execução em Docker: o Dockerfile fornecido instala `ffmpeg` na imagem de runtime.
- Execução no host: `ffmpeg` deve estar instalado e disponível no `PATH` do host.
- Se `ffmpeg` estiver ausente ou falhar, o processamento de vídeo retorna erro e a mensagem SQS não é deletada.

Verificar disponibilidade no host:

```bash
ffmpeg -version
```

## Rodar O Serviço Localmente

Crie ou carregue um arquivo de ambiente para este serviço. O arquivo deve conter as variáveis documentadas em [configuration.pt-BR.md](configuration.pt-BR.md). Depois execute:

```bash
set -a
source .env
set +a

go run ./cmd/processing-service
```

Para rodar sem publicação de progresso Kafka:

```bash
export KAFKA_ENABLED=false
go run ./cmd/processing-service
```

## Build E Execução Com Docker

Construir a imagem do serviço:

```bash
docker build -t xtube-processing-service .
```

Executar com arquivo de ambiente:

```bash
docker run --rm \
  --env-file .env \
  -p 9090:9090 \
  xtube-processing-service
```

Ao rodar em Docker, garanta que o container consiga acessar os endpoints configurados de S3, SQS e Kafka. Para Kafka, `KAFKA_BROKERS` deve ser resolvível de dentro do container.

## Rodar Testes

```bash
go test ./...
```

## Health E Métricas

O serviço inicia um servidor de observabilidade em `:9090`.

```bash
curl http://localhost:9090/health
curl http://localhost:9090/metrics
```

Resposta esperada do health:

```text
ok
```

## Checagens De Integração Do Serviço

O serviço espera:

| Dependência                   | Contrato do serviço                                                                                       |
| ----------------------------- | --------------------------------------------------------------------------------------------------------- |
| Bucket S3 de entrada de vídeo | `S3_BUCKET_INPUT` existe e mensagens de evento S3 apontam para objetos nesse bucket.                      |
| Bucket S3 de saída de vídeo   | `S3_BUCKET_OUTPUT` existe e aceita uploads de chunks processados.                                         |
| Bucket S3 de thumbnails       | `S3_BUCKET_THUMBNAILS` existe se a validação de bucket de thumbnail estiver habilitada pela configuração. |
| Fila SQS de vídeo             | `SQS_VIDEO_PROCESSING_URL` aponta para uma fila com mensagens de evento S3 de vídeo.                      |
| Fila SQS de thumbnail         | `SQS_THUMBNAIL_PROCESSING_URL` aponta para uma fila com mensagens de evento S3 de thumbnail.              |
| Tópico Kafka                  | `KAFKA_VIDEO_PROGRESS_TOPIC` existe quando Kafka está habilitado.                                         |

Provisionar essas dependências está fora do escopo desta documentação do serviço.

## Chaves S3 Esperadas

Exemplos de entrada de vídeo:

```text
uploads/music.mp4
uploads/video-123/original.mp4
```

Exemplos de saída de vídeo processado:

```text
music/360p/video-1.mp4
music/480p/video-1.mp4
music/720p/video-1.mp4
```

Exemplo de entrada de thumbnail:

```text
uploads/maxresdefault.jpg
```

Exemplos de saída de thumbnail processada:

```text
processed/maxresdefault/original.jpg
processed/maxresdefault/3x.jpg
```

## Consumo De Progresso Kafka

Ferramentas cliente Kafka ficam fora deste serviço. Qualquer consumidor Kafka pode ler o tópico configurado. O serviço publica mensagens com chave `video_id` e este payload JSON:

```json
{
  "video_id": "video-id",
  "progress_percent": 37
}
```

Mensagens aparecem apenas depois que cada chunk de vídeo processado é enviado com sucesso ao S3.

## Logs

Logs são objetos JSON emitidos com `slog`.

Campos comuns:

| Campo           | Significado                                                                                    |
| --------------- | ---------------------------------------------------------------------------------------------- |
| `component`     | Componente lógico como `service`, `worker`, `processor`, `ffmpeg`, `kafka` ou `observability`. |
| `worker`        | Nome do worker, normalmente `video` ou `thumbnail`.                                            |
| `queue`         | Label da fila do worker.                                                                       |
| `bucket`, `key` | Localização S3 em processamento.                                                               |
| `video_id`      | Identificador derivado para jobs de vídeo.                                                     |
| `thumbnail_id`  | Identificador derivado para jobs de thumbnail.                                                 |
| `duration_ms`   | Duração da operação em milissegundos.                                                          |
| `error`         | Detalhes de erro em operações com falha.                                                       |

Logs importantes de ciclo de vida:

- `processing service started`
- `observability server started`
- `sqs worker started`
- `sqs message received`
- `video download started` / `video download finished`
- `video segmentation started` / `video segmentation finished`
- `video upload finished`
- `thumbnail processing started` / `thumbnail processing finished`
- `sqs message deleted`

`LOG_FFMPEG_PROGRESS=true` emite linhas de progresso do ffmpeg. `LOG_CHUNK_DETAILS=true` habilita logs debug de descoberta de segmentos e início/fim de upload de chunks.

## Troubleshooting

| Sintoma                                 | Verificação                                                                                                                                                                                      |
| --------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Serviço encerra no startup              | Variáveis obrigatórias ausentes: `SQS_VIDEO_PROCESSING_URL`, `SQS_THUMBNAIL_PROCESSING_URL`, `S3_BUCKET_INPUT`, `S3_BUCKET_OUTPUT`.                                                              |
| Download ou upload S3 falha             | Confirme `AWS_ENDPOINT_URL`, credenciais AWS, nomes de buckets e chaves de objetos.                                                                                                              |
| Polling SQS falha                       | Confirme URLs das filas, credenciais AWS, região e acesso de rede.                                                                                                                               |
| Vídeo falha imediatamente               | Confirme que `ffmpeg` está instalado e disponível no `PATH`, ou rode pela imagem Docker fornecida.                                                                                               |
| Publicação Kafka falha                  | Confirme `KAFKA_BROKERS`, `KAFKA_VIDEO_PROGRESS_TOPIC`, acesso de rede e disponibilidade do tópico. Use `KAFKA_ENABLED=false` apenas quando eventos de progresso forem opcionais nesse ambiente. |
| Saídas de thumbnail disparam novos jobs | O processador rejeita chaves fora de `uploads/`; garanta que apenas eventos S3 desejados sejam entregues à fila de thumbnail.                                                                    |
| Mensagens SQS reaparecem                | O processamento está falhando ou a publicação Kafka está falhando. Verifique logs de erro do worker e a redrive policy externa do SQS.                                                           |

## Recomendações

- Mantenha filas SQS de vídeo e thumbnail com redrive policy configurada fora da aplicação.
- Mantenha `SQS_VIDEO_VISIBILITY_TIMEOUT_SECONDS` maior que o tempo esperado de processamento de vídeo; o worker renova visibilidade, mas valores muito curtos aumentam risco operacional.
- Crie e gerencie tópicos Kafka fora da aplicação. O serviço espera que o tópico configurado exista.
