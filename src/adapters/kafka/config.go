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
	BatchSize     int
	BatchTimeout  time.Duration
}

func LoadKafkaConfig() Config {
	enabled, _ := strconv.ParseBool(os.Getenv("KAFKA_ENABLED"))
	return Config{
		Brokers:       []string{os.Getenv("KAFKA_BROKERS")},
		TopicIncoming: os.Getenv("KAFKA_TOPIC_INCOMING"),
		TopicOutgoing: os.Getenv("KAFKA_TOPIC_OUTGOING"),
		ConsumerGroup: os.Getenv("KAFKA_CONSUMER_GROUP"),
		Enabled:       enabled,
		BatchSize:     10,
		BatchTimeout:  100 * time.Millisecond,
	}
}
