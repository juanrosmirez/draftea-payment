package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// EventHandler función para manejar eventos
type EventHandler func(ctx context.Context, event interface{}) error

// Subscriber interfaz para suscribirse a eventos
type Subscriber interface {
	Subscribe(ctx context.Context, eventType EventType, handler EventHandler) error
	Start(ctx context.Context) error
	Stop() error
}

// KafkaSubscriber implementación de Subscriber usando Kafka
type KafkaSubscriber struct {
	reader   *kafka.Reader
	handlers map[EventType][]EventHandler
	logger   *zap.Logger
	done     chan struct{}
}

// NewKafkaSubscriber crea un nuevo subscriber de Kafka
func NewKafkaSubscriber(brokers []string, topic, groupID string, logger *zap.Logger) *KafkaSubscriber {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		GroupID:     groupID,
		MinBytes:    10e3, // 10KB
		MaxBytes:    10e6, // 10MB
		MaxWait:     1 * time.Second,
		StartOffset: kafka.LastOffset,
	})

	return &KafkaSubscriber{
		reader:   reader,
		handlers: make(map[EventType][]EventHandler),
		logger:   logger,
		done:     make(chan struct{}),
	}
}

// Subscribe registra un handler para un tipo de evento
func (s *KafkaSubscriber) Subscribe(ctx context.Context, eventType EventType, handler EventHandler) error {
	s.handlers[eventType] = append(s.handlers[eventType], handler)
	s.logger.Info("handler registered for event type", zap.String("event_type", string(eventType)))
	return nil
}

// Start inicia el procesamiento de eventos
func (s *KafkaSubscriber) Start(ctx context.Context) error {
	s.logger.Info("starting event subscriber")

	go func() {
		for {
			select {
			case <-ctx.Done():
				s.logger.Info("subscriber context cancelled")
				return
			case <-s.done:
				s.logger.Info("subscriber stopped")
				return
			default:
				message, err := s.reader.ReadMessage(ctx)
				if err != nil {
					s.logger.Error("failed to read message", zap.Error(err))
					continue
				}

				if err := s.processMessage(ctx, message); err != nil {
					s.logger.Error("failed to process message", 
						zap.Error(err),
						zap.String("partition", fmt.Sprintf("%d", message.Partition)),
						zap.String("offset", fmt.Sprintf("%d", message.Offset)))
				}
			}
		}
	}()

	return nil
}

// processMessage procesa un mensaje individual
func (s *KafkaSubscriber) processMessage(ctx context.Context, message kafka.Message) error {
	// Extraer tipo de evento de los headers
	var eventType EventType
	for _, header := range message.Headers {
		if header.Key == "event_type" {
			eventType = EventType(header.Value)
			break
		}
	}

	if eventType == "" {
		return fmt.Errorf("event_type header not found")
	}

	// Buscar handlers para este tipo de evento
	handlers, exists := s.handlers[eventType]
	if !exists || len(handlers) == 0 {
		s.logger.Debug("no handlers found for event type", zap.String("event_type", string(eventType)))
		return nil
	}

	// Deserializar evento basado en su tipo
	event, err := s.deserializeEvent(eventType, message.Value)
	if err != nil {
		return fmt.Errorf("failed to deserialize event: %w", err)
	}

	// Ejecutar todos los handlers para este evento
	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			s.logger.Error("handler failed to process event",
				zap.String("event_type", string(eventType)),
				zap.Error(err))
			// Continuar con otros handlers incluso si uno falla
		}
	}

	return nil
}

// deserializeEvent deserializa un evento basado en su tipo
func (s *KafkaSubscriber) deserializeEvent(eventType EventType, data []byte) (interface{}, error) {
	switch eventType {
	case PaymentInitiated:
		var event PaymentInitiatedEvent
		err := json.Unmarshal(data, &event)
		return event, err
	case PaymentCompleted:
		var event PaymentCompletedEvent
		err := json.Unmarshal(data, &event)
		return event, err
	case PaymentFailed:
		var event PaymentFailedEvent
		err := json.Unmarshal(data, &event)
		return event, err
	case WalletDeducted:
		var event WalletDeductedEvent
		err := json.Unmarshal(data, &event)
		return event, err
	case WalletRefunded:
		var event WalletRefundedEvent
		err := json.Unmarshal(data, &event)
		return event, err
	case GatewayResponseReceived:
		var event GatewayResponseReceivedEvent
		err := json.Unmarshal(data, &event)
		return event, err
	case MetricsCollected:
		var event MetricsCollectedEvent
		err := json.Unmarshal(data, &event)
		return event, err
	default:
		return nil, fmt.Errorf("unknown event type: %s", eventType)
	}
}

// Stop detiene el subscriber
func (s *KafkaSubscriber) Stop() error {
	close(s.done)
	return s.reader.Close()
}
