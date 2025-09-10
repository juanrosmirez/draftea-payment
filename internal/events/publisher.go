package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// Publisher interfaz para publicar eventos
type Publisher interface {
	Publish(ctx context.Context, event interface{}) error
	PublishBatch(ctx context.Context, events []interface{}) error
	Close() error
}

// KafkaPublisher implementación de Publisher usando Kafka
type KafkaPublisher struct {
	writer *kafka.Writer
	logger *zap.Logger
}

// NewKafkaPublisher crea un nuevo publisher de Kafka
func NewKafkaPublisher(brokers []string, topic string, logger *zap.Logger) *KafkaPublisher {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireAll,
		Compression:  kafka.Snappy,
		BatchTimeout: 10 * time.Millisecond,
		BatchSize:    100,
	}

	return &KafkaPublisher{
		writer: writer,
		logger: logger,
	}
}

// Publish publica un evento individual
func (p *KafkaPublisher) Publish(ctx context.Context, event interface{}) error {
	baseEvent, ok := event.(BaseEvent)
	if !ok {
		return fmt.Errorf("event must embed BaseEvent")
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		p.logger.Error("failed to marshal event", 
			zap.String("event_id", baseEvent.ID.String()),
			zap.String("event_type", string(baseEvent.Type)),
			zap.Error(err))
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	message := kafka.Message{
		Key:   []byte(baseEvent.AggregateID.String()),
		Value: eventData,
		Headers: []kafka.Header{
			{Key: "event_type", Value: []byte(baseEvent.Type)},
			{Key: "correlation_id", Value: []byte(baseEvent.CorrelationID.String())},
			{Key: "timestamp", Value: []byte(baseEvent.Timestamp.Format(time.RFC3339))},
		},
	}

	err = p.writer.WriteMessages(ctx, message)
	if err != nil {
		p.logger.Error("failed to publish event",
			zap.String("event_id", baseEvent.ID.String()),
			zap.String("event_type", string(baseEvent.Type)),
			zap.Error(err))
		return fmt.Errorf("failed to publish event: %w", err)
	}

	p.logger.Info("event published successfully",
		zap.String("event_id", baseEvent.ID.String()),
		zap.String("event_type", string(baseEvent.Type)),
		zap.String("aggregate_id", baseEvent.AggregateID.String()))

	return nil
}

// PublishBatch publica múltiples eventos en lote
func (p *KafkaPublisher) PublishBatch(ctx context.Context, events []interface{}) error {
	messages := make([]kafka.Message, 0, len(events))

	for _, event := range events {
		baseEvent, ok := event.(BaseEvent)
		if !ok {
			return fmt.Errorf("all events must embed BaseEvent")
		}

		eventData, err := json.Marshal(event)
		if err != nil {
			p.logger.Error("failed to marshal event in batch",
				zap.String("event_id", baseEvent.ID.String()),
				zap.Error(err))
			continue
		}

		message := kafka.Message{
			Key:   []byte(baseEvent.AggregateID.String()),
			Value: eventData,
			Headers: []kafka.Header{
				{Key: "event_type", Value: []byte(baseEvent.Type)},
				{Key: "correlation_id", Value: []byte(baseEvent.CorrelationID.String())},
				{Key: "timestamp", Value: []byte(baseEvent.Timestamp.Format(time.RFC3339))},
			},
		}

		messages = append(messages, message)
	}

	err := p.writer.WriteMessages(ctx, messages...)
	if err != nil {
		p.logger.Error("failed to publish event batch", zap.Error(err))
		return fmt.Errorf("failed to publish event batch: %w", err)
	}

	p.logger.Info("event batch published successfully", zap.Int("count", len(messages)))
	return nil
}

// Close cierra el publisher
func (p *KafkaPublisher) Close() error {
	return p.writer.Close()
}
