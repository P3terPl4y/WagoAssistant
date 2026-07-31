package kafka

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Brokers       []string
	TopicIncoming string
	TopicOutgoing string
	ConsumerGroup string
	Enabled       bool
	WorkerCount   int     // número de workers concurrentes
	RateLimit     float64 // llamadas a IA por segundo (ej: 0.5 para 1 cada 2s)
	BatchSize     int
	BatchTimeout  time.Duration
}

func LoadKafkaConfig() Config {
	enabled, _ := strconv.ParseBool(os.Getenv("KAFKA_ENABLED"))
	workers, _ := strconv.Atoi(os.Getenv("KAFKA_WORKERS"))
	if workers <= 0 {
		workers = 5
	}
	rateLimit, _ := strconv.ParseFloat(os.Getenv("KAFKA_RATE_LIMIT"), 64)
	if rateLimit <= 0 {
		rateLimit = 0.5 // por defecto 1 cada 2 segundos
	}
	return Config{
		Brokers:       []string{os.Getenv("KAFKA_BROKERS")},
		TopicIncoming: os.Getenv("KAFKA_TOPIC_INCOMING"),
		TopicOutgoing: os.Getenv("KAFKA_TOPIC_OUTGOING"),
		ConsumerGroup: os.Getenv("KAFKA_CONSUMER_GROUP"),
		Enabled:       enabled,
		WorkerCount:   workers,
		RateLimit:     rateLimit,
		BatchSize:     10,
		BatchTimeout:  100 * time.Millisecond,
	}
}
