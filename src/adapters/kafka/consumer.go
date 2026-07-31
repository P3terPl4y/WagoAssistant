package kafka

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"App/src/pkg/logger"

	"github.com/segmentio/kafka-go"
)

type Consumer struct {
	reader  *kafka.Reader
	config  Config
	logger  logger.Logger
	handler MessageHandler
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

type MessageHandler interface {
	HandleMessage(ctx context.Context, msg *IncomingMessage) error
}

// Función de callback para procesar mensajes (se inyecta desde BotService)
type ProcessMessageFunc func(botID int, senderJID string, text string, userKey string) error

func NewConsumer(config Config, log logger.Logger, handler ProcessMessageFunc) (*Consumer, error) {
	if !config.Enabled {
		return nil, nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	consumer := &Consumer{
		config: config,
		logger: log.WithComponent("kafka_consumer"),
		ctx:    ctx,
		cancel: cancel,
	}
	consumer.reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        config.Brokers,
		GroupID:        config.ConsumerGroup,
		Topic:          config.TopicIncoming,
		MinBytes:       10e3,
		MaxBytes:       10e6,
		MaxWait:        1 * time.Second,
		CommitInterval: 2 * time.Second,
		StartOffset:    kafka.LastOffset,
		// Remove: AllowAutoTopicCreation: true,
	})

	consumer.logger.Info().
		Str("topic", config.TopicIncoming).
		Str("group", config.ConsumerGroup).
		Msg("Kafka consumer initialized")

	// Iniciar el loop de consumo en goroutine
	consumer.wg.Add(1)
	go consumer.start(handler)

	return consumer, nil
}

func (c *Consumer) start(handler ProcessMessageFunc) {
	defer c.wg.Done()
	c.logger.Info().
		Dur("interval", c.config.ProcessInterval).
		Msg("Kafka consumer loop started with processing interval")

	for {
		select {
		case <-c.ctx.Done():
			c.logger.Info().Msg("Kafka consumer stopping...")
			return
		default:
			msg, err := c.reader.ReadMessage(c.ctx)
			if err != nil {
				if err == context.Canceled {
					return
				}
				c.logger.Error().Err(err).Msg("Failed to read message from Kafka")
				time.Sleep(1 * time.Second)
				continue
			}

			var incoming IncomingMessage
			if err := json.Unmarshal(msg.Value, &incoming); err != nil {
				c.logger.Error().
					Err(err).
					Bytes("raw", msg.Value).
					Msg("Failed to unmarshal Kafka message")
				continue
			}

			c.logger.Debug().
				Int("bot_id", incoming.BotID).
				Str("sender", incoming.SenderJID).
				Int64("offset", msg.Offset).
				Msg("Processing message from Kafka")

			// Procesar mensaje (llamada a la IA)
			err = handler(incoming.BotID, incoming.SenderJID, incoming.Text, incoming.UserKey)
			if err != nil {
				c.logger.Error().
					Err(err).
					Int("bot_id", incoming.BotID).
					Msg("Handler failed, will retry (no commit)")
				// No se hace commit, se reintentará en la siguiente lectura
				continue
			}

			// Commit del offset solo después de procesar exitosamente
			if err := c.reader.CommitMessages(c.ctx, msg); err != nil {
				c.logger.Warn().
					Err(err).
					Int64("offset", msg.Offset).
					Msg("Failed to commit offset, will retry")
				continue
			}

			c.logger.Debug().
				Int("bot_id", incoming.BotID).
				Msg("Message processed and committed, waiting before next...")

			// ⏳ ESPERA OBLIGATORIA: evita saturar la IA
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(c.config.ProcessInterval):
				// Continuar al siguiente mensaje
			}
		}
	}
}

func (c *Consumer) Close() error {
	c.logger.Info().Msg("Closing Kafka consumer...")
	c.cancel()
	c.wg.Wait()
	if c.reader != nil {
		return c.reader.Close()
	}
	return nil
}
