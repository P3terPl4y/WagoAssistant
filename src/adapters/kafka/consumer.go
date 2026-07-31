package kafka

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"App/src/pkg/logger"

	"github.com/segmentio/kafka-go"
	"golang.org/x/time/rate"
)

type Consumer struct {
	reader    *kafka.Reader
	config    Config
	logger    logger.Logger
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	limiter   *rate.Limiter
	workerSem chan struct{} // semáforo para controlar número de workers activos
}

type ProcessMessageFunc func(botID int, senderJID string, text string, userKey string) error

func NewConsumer(config Config, log logger.Logger, handler ProcessMessageFunc) (*Consumer, error) {
	if !config.Enabled {
		return nil, nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Configurar rate limiter: llamadas por segundo
	limit := rate.Limit(config.RateLimit) // ej: 0.5 => 1 cada 2 segundos
	limiter := rate.NewLimiter(limit, 1)

	// Número máximo de workers concurrentes
	workerLimit := config.WorkerCount
	if workerLimit <= 0 {
		workerLimit = 5 // valor por defecto
	}
	workerSem := make(chan struct{}, workerLimit)

	consumer := &Consumer{
		config:    config,
		logger:    log.WithComponent("kafka_consumer"),
		ctx:       ctx,
		cancel:    cancel,
		limiter:   limiter,
		workerSem: workerSem,
	}

	consumer.reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        config.Brokers,
		GroupID:        config.ConsumerGroup,
		Topic:          config.TopicIncoming,
		MinBytes:       10e3,
		MaxBytes:       10e6,
		MaxWait:        1 * time.Second,
		CommitInterval: 0, // commits manuales
		StartOffset:    kafka.LastOffset,
	})

	consumer.logger.Info().
		Str("topic", config.TopicIncoming).
		Str("group", config.ConsumerGroup).
		Int("workers", workerLimit).
		Float64("rate_limit", float64(limit)).
		Msg("Kafka consumer initialized with async workers")

	// Iniciar el lector y despachador de mensajes
	consumer.wg.Add(1)
	go consumer.dispatch(handler)

	return consumer, nil
}

func (c *Consumer) dispatch(handler ProcessMessageFunc) {
	defer c.wg.Done()
	c.logger.Info().Msg("Kafka dispatcher started")

	for {
		select {
		case <-c.ctx.Done():
			c.logger.Info().Msg("Dispatcher stopping...")
			return
		default:
			// Leer mensaje (bloqueante)
			msg, err := c.reader.ReadMessage(c.ctx)
			if err != nil {
				if err == context.Canceled {
					return
				}
				c.logger.Error().Err(err).Msg("Failed to read message from Kafka")
				time.Sleep(500 * time.Millisecond)
				continue
			}

			// Ocupar un slot del semáforo (limita workers concurrentes)
			select {
			case c.workerSem <- struct{}{}:
				// Slot disponible, lanzar worker
			case <-c.ctx.Done():
				return
			}

			c.wg.Add(1)
			go c.processMessage(msg, handler)
		}
	}
}

func (c *Consumer) processMessage(msg kafka.Message, handler ProcessMessageFunc) {
	defer c.wg.Done()
	defer func() { <-c.workerSem }() // liberar slot al terminar

	// 1. Esperar a que el rate limiter permita la llamada a IA
	if err := c.limiter.Wait(c.ctx); err != nil {
		if err == context.Canceled {
			return
		}
		c.logger.Error().Err(err).Msg("Rate limiter wait failed")
		return
	}

	// 2. Deserializar mensaje
	var incoming IncomingMessage
	if err := json.Unmarshal(msg.Value, &incoming); err != nil {
		c.logger.Error().
			Err(err).
			Bytes("raw", msg.Value).
			Msg("Failed to unmarshal Kafka message")
		// No se commitea, se perderá el mensaje (puede ir a DLQ)
		return
	}

	// 3. Ejecutar handler (llama a la IA y procesa)
	err := handler(incoming.BotID, incoming.SenderJID, incoming.Text, incoming.UserKey)
	if err != nil {
		c.logger.Error().
			Err(err).
			Int("bot_id", incoming.BotID).
			Msg("Handler failed, message will be retried (no commit)")
		// No se commitea, Kafka lo reintentará automáticamente
		return
	}

	// 4. Commit del offset solo si fue exitoso
	if err := c.reader.CommitMessages(c.ctx, msg); err != nil {
		c.logger.Warn().
			Err(err).
			Int64("offset", msg.Offset).
			Msg("Failed to commit offset, will retry")
		// Si falla el commit, el mensaje se reprocesará (puede causar duplicados)
		return
	}

	c.logger.Debug().
		Int("bot_id", incoming.BotID).
		Int64("offset", msg.Offset).
		Msg("Message processed and committed")
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

/* Configuración dinámica basada en el proveedor
func (c *Consumer) getRateLimit() float64 {
	// Leer de una variable de entorno o de un endpoint de configuración
	if os.Getenv("KAFKA_RATE_LIMIT") != "" {
		limit, _ := strconv.ParseFloat(os.Getenv("KAFKA_RATE_LIMIT"), 64)
		return limit
	}
	// Valor por defecto para OpenRouter free: 0.5 req/s
	return 0.5
}
*/
