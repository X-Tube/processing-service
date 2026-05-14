package progress

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
)

type KafkaPublisherConfig struct {
	Brokers  []string
	Topic    string
	ClientID string
}

type KafkaPublisher struct {
	config KafkaPublisherConfig
	writer *kafka.Writer
	logger *slog.Logger
}

func NewKafkaPublisher(config KafkaPublisherConfig, logger *slog.Logger) (*KafkaPublisher, error) {
	if len(config.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers are required")
	}
	if config.Topic == "" {
		return nil, fmt.Errorf("kafka video progress topic is required")
	}
	if config.ClientID == "" {
		config.ClientID = "xtube-processing-service"
	}
	if logger == nil {
		logger = slog.Default()
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(config.Brokers...),
		Topic:        config.Topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		MaxAttempts:  3,
		BatchTimeout: 10 * time.Millisecond,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Transport: &kafka.Transport{
			ClientID: config.ClientID,
		},
	}

	logger.Info(
		"kafka progress publisher configured",
		"component", "kafka",
		"brokers", config.Brokers,
		"topic", config.Topic,
		"client_id", config.ClientID,
	)

	return &KafkaPublisher{
		config: config,
		writer: writer,
		logger: logger,
	}, nil
}

func (p *KafkaPublisher) PublishVideoProgress(ctx context.Context, event VideoProgressEvent) error {
	value, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal video progress event: %w", err)
	}

	message := kafka.Message{
		Key:   []byte(event.VideoID),
		Value: value,
		Time:  time.Now(),
	}

	if err := p.writer.WriteMessages(ctx, message); err != nil {
		p.logger.Error(
			"kafka video progress publish failed",
			"component", "kafka",
			"topic", p.config.Topic,
			"video_id", event.VideoID,
			"progress_percent", event.ProgressPercent,
			"error", err,
		)
		return fmt.Errorf("publish video progress event: %w", err)
	}

	return nil
}

func (p *KafkaPublisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}

	return p.writer.Close()
}
