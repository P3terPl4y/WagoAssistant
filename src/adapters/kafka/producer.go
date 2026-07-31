package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"App/src/pkg/logger"

	"github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafka.Writer
	config Config
	logger logger.Logger
	mu     sync.Mutex
}

func NewProducer(config Config, log logger.Logger) *Producer {
	if !config.Enabled {
		return nil
	}
	p := &Producer{
		config: config,
		logger: log.WithComponent("kafka_producer"),
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(config.Brokers...),
			Topic:                  config.TopicIncoming,
			Balancer:               &kafka.Hash{},
			WriteTimeout:           10 * time.Second,
			ReadTimeout:            5 * time.Second,
			RequiredAcks:           kafka.RequireOne,
			AllowAutoTopicCreation: true,
		},
	}
	p.logger.Info().
		Str("topic", config.TopicIncoming).
		Int("brokers", len(config.Brokers)).
		Msg("Kafka producer initialized")
	return p
}

func (p *Producer) Publish(ctx context.Context, msg *IncomingMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.writer == nil {
		return fmt.Errorf("kafka producer not initialized")
	}

	data, err := msg.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	key := []byte(fmt.Sprintf("%d", msg.BotID))
	kafkaMsg := kafka.Message{
		Key:   key,
		Value: data,
		Time:  time.Now(),
	}

	err = p.writer.WriteMessages(ctx, kafkaMsg)
	if err != nil {
		p.logger.Error().
			Err(err).
			Int("bot_id", msg.BotID).
			Msg("Failed to publish message to Kafka")
		return fmt.Errorf("kafka write error: %w", err)
	}

	p.logger.Debug().
		Int("bot_id", msg.BotID).
		Str("topic", p.config.TopicIncoming).
		Msg("Message published to Kafka")
	return nil
}

func (p *Producer) Close() error {
	if p.writer != nil {
		return p.writer.Close()
	}
	return nil
}
