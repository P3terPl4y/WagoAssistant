package logger

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type Logger struct {
	zl  zerolog.Logger
	ctx context.Context
}

// New crea un nuevo Logger
func New(environment string) Logger {
	var w io.Writer
	if environment == "production" {
		w = os.Stdout
	} else {
		w = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.DateTime,
		}
	}
	zl := zerolog.New(w).With().Timestamp().Logger()
	return Logger{zl: zl, ctx: context.Background()}
}

// WithTraceID agrega un trace_id al contexto del logger
func (l Logger) WithTraceID(traceID string) Logger {
	return Logger{
		zl:  l.zl.With().Str("trace_id", traceID).Logger(),
		ctx: context.WithValue(l.ctx, "trace_id", traceID),
	}
}

// WithUserID agrega un user_id al contexto del logger
func (l Logger) WithUserID(userID int) Logger {
	return Logger{
		zl:  l.zl.With().Int("user_id", userID).Logger(),
		ctx: context.WithValue(l.ctx, "user_id", userID),
	}
}

// WithBotID agrega un Bot_id al contexto del logger
func (l Logger) WithBotID(botID int) Logger {
	parent := l.ctx
	if parent == nil {
		parent = context.Background()
	}
	newCtx := context.WithValue(parent, "bot_id", botID)
	return Logger{
		zl:  l.zl.With().Int("bot_id", botID).Logger(),
		ctx: newCtx,
	}
}
func (l Logger) WithComponent(name string) Logger {
	return Logger{zl: l.zl.With().Str("component", name).Logger()}
}

// WithSpanID agrega un span_id al contexto del logger
func (l Logger) WithSpanID(spanID string) Logger {
	return Logger{
		zl:  l.zl.With().Str("span_id", spanID).Logger(),
		ctx: context.WithValue(l.ctx, "span_id", spanID),
	}
}

// NewTrace crea un nuevo trace con IDs generados automáticamente
func (l Logger) NewTrace() Logger {
	traceID := uuid.New().String()
	spanID := uuid.New().String()[:8]
	return l.WithTraceID(traceID).WithSpanID(spanID)
}

// Resto de métodos igual...
func (l Logger) Info() *zerolog.Event  { return l.zl.Info() }
func (l Logger) Warn() *zerolog.Event  { return l.zl.Warn() }
func (l Logger) Error() *zerolog.Event { return l.zl.Error() }
func (l Logger) Fatal() *zerolog.Event { return l.zl.Fatal() }
func (l Logger) Debug() *zerolog.Event { return l.zl.Debug() }
