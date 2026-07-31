package kafka

import (
	"encoding/json"
	"time"
)

type IncomingMessage struct {
	BotID      int       `json:"bot_id"`
	SenderJID  string    `json:"sender_jid"`
	Text       string    `json:"text"`
	UserKey    string    `json:"user_key"`
	ReceivedAt time.Time `json:"received_at"`
	RetryCount int       `json:"retry_count"`
}

func (m *IncomingMessage) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

func (m *IncomingMessage) FromJSON(data []byte) error {
	return json.Unmarshal(data, m)
}
