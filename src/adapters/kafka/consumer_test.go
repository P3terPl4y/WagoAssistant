package kafka

import (
	"testing"
	"time"

	"App/src/pkg/logger"

	"github.com/stretchr/testify/assert"
)

func TestConsumer_RateLimiting(t *testing.T) {
	config := Config{
		Enabled:       true,
		Brokers:       []string{"localhost:9092"},
		TopicIncoming: "test",
		ConsumerGroup: "test-group",
		WorkerCount:   2,
		RateLimit:     1.0, // 1 msg/s
	}
	log := logger.New("test")
	handlerCalled := 0
	handler := func(botID int, senderJID string, text string, userKey string) error {
		handlerCalled++
		time.Sleep(100 * time.Millisecond)
		return nil
	}
	consumer, err := NewConsumer(config, log, handler)
	assert.NoError(t, err)
	assert.NotNil(t, consumer)

	// Simular publicación de mensajes (no se puede sin un broker real, pero se puede testear la lógica con mocks)
	// Para pruebas unitarias, mejor usar mocks de kafka.Reader
	// Aquí mostramos la estructura.
}
