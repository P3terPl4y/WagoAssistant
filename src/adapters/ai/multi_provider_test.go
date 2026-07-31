package ai

import (
	"context"
	"testing"

	"App/src/config"
	"App/src/pkg/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockSource struct{ mock.Mock }

func (m *mockSource) call(ctx context.Context, prompt string) (string, error) {
    args := m.Called(ctx, prompt)
    return args.String(0), args.Error(1)
}

func TestMultiProvider_CircuitBreaker(t *testing.T) {
    cfg := config.AIConfig{
        FreeModels:    []string{"model1"},
        OpenRouterURL: "http://test",
        OpenRouterKey: "key",
        LegacyURL:     "http://legacy",
        LegacyKey:     "key",
        LocalEnabled:  false,
    }
    log := logger.New("test")
    mp := NewMultiProvider(cfg, log)

    // Forzar fallos para abrir el circuito
    ctx := context.Background()
    for i := 0; i < 5; i++ {
        _, err := mp.Call(ctx, "test")
        assert.Error(t, err)
    }
    // Después de varios fallos, el circuito debería estar abierto y devolver error sin llamar a la IA
    _, err := mp.Call(ctx, "test")
    assert.Error(t, err)
    // Verificar que el mensaje de error contiene "circuit breaker"
    assert.Contains(t, err.Error(), "circuit breaker")
}