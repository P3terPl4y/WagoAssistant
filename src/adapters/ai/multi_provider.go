package ai

import (
	"App/src/config"
	"App/src/pkg/logger"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3/client"
)

// Sanitization patterns (same as original)
var (
	reChatmlTokens  = regexp.MustCompile(`<\|[^|>]+\|>`)
	reThinkBlocks   = regexp.MustCompile(`(?s)<think>.*?</think>`)
	reRoleLines     = regexp.MustCompile(`(?m)^\s*(assistant|user|system)\s*$`)
	reMultiNewline  = regexp.MustCompile(`\n{3,}`)
)

func sanitizeResponse(text string) string {
	text = reThinkBlocks.ReplaceAllString(text, "")
	text = reChatmlTokens.ReplaceAllString(text, "")
	text = reRoleLines.ReplaceAllString(text, "")
	text = strings.TrimSpace(text)
	text = reMultiNewline.ReplaceAllString(text, "\n\n")
	return text
}

// MultiProvider implements ports.AIService with automatic failover across providers.
type MultiProvider struct {
	cfg         config.AIConfig
	logger      logger.Logger
	modelIndex  uint32
	sourceIndex uint32
}

// NewMultiProvider creates a new multi-source AI service.
func NewMultiProvider(cfg config.AIConfig, log logger.Logger) *MultiProvider {
	return &MultiProvider{
		cfg:    cfg,
		logger: log.WithComponent("ai"),
	}
}

type aiSource struct {
	name string
	fn   func(ctx context.Context, prompt string) (string, error)
}

// Call sends a prompt and returns the response, with automatic failover.
func (m *MultiProvider) Call(ctx context.Context, prompt string) (string, error) {
	turn := atomic.AddUint32(&m.sourceIndex, 1) % 3

	sources := []aiSource{
		{"OpenRouter", m.callOpenRouter},
		{"Legacy", m.callLegacy},
	}
	if m.cfg.LocalEnabled {
		sources = append(sources, aiSource{"Local", m.callLocal})
	}

	for i := 0; i < len(sources); i++ {
		idx := (int(turn) + i) % len(sources)
		s := sources[idx]
		resp, err := s.fn(ctx, prompt)
		if err == nil && resp != "" {
			if i > 0 {
				m.logger.Warn().Str("source", s.name).Msg("Primary source failed, responded via fallback")
			}
			return resp, nil
		}
		m.logger.Warn().Str("source", s.name).Int("attempt", i+1).Err(err).Msg("AI source failed")
	}
	return "", fmt.Errorf("all AI providers failed")
}

// --- OpenRouter ---

func (m *MultiProvider) callOpenRouter(ctx context.Context, prompt string) (string, error) {
	var lastErr error
	for i := 0; i < len(m.cfg.FreeModels); i++ {
		idx := atomic.AddUint32(&m.modelIndex, 1) % uint32(len(m.cfg.FreeModels))
		model := m.cfg.FreeModels[idx]
		resp, err := m.callOpenRouterWithModel(ctx, prompt, model)
		if err == nil {
			return resp, nil
		}
		lastErr = fmt.Errorf("model %s: %w", model, err)
	}
	return "", fmt.Errorf("all OpenRouter models failed: %w", lastErr)
}

func (m *MultiProvider) callOpenRouterWithModel(_ context.Context, prompt, model string) (string, error) {
	cc := client.New()
	cc.SetTimeout(30 * time.Second)

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + m.cfg.OpenRouterKey,
		"HTTP-Referer":  "https://wago.app",
		"X-Title":       "Wago WhatsApp Bot",
	}

	resp, err := cc.Post(m.cfg.OpenRouterURL, client.Config{Header: headers, Body: payload})
	if err != nil {
		return "", fmt.Errorf("request failed: %v", err)
	}
	defer resp.Close()

	if resp.StatusCode() == 429 {
		return "", fmt.Errorf("rate limit (429) for model %s", model)
	}
	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return "", fmt.Errorf("JSON parse error: %v", err)
	}
	if errObj, ok := result["error"].(map[string]interface{}); ok {
		if msg, ok := errObj["message"].(string); ok {
			return "", fmt.Errorf("OpenRouter error: %s", msg)
		}
	}
	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok {
					return sanitizeResponse(content), nil
				}
			}
		}
	}
	return "", fmt.Errorf("could not extract response")
}

// --- Legacy ---

func (m *MultiProvider) callLegacy(_ context.Context, prompt string) (string, error) {
	cc := client.New()
	cc.SetTimeout(15 * time.Second)

	payload := map[string]string{"message": prompt}
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + m.cfg.LegacyKey,
	}

	resp, err := cc.Post(m.cfg.LegacyURL, client.Config{Header: headers, Body: payload})
	if err != nil {
		return "", fmt.Errorf("Legacy request error: %v", err)
	}
	defer resp.Close()
	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("Legacy error %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return "", fmt.Errorf("Legacy parse error: %v", err)
	}
	for _, key := range []string{"response", "message", "text"} {
		if respText, ok := result[key].(string); ok && respText != "" {
			return sanitizeResponse(respText), nil
		}
	}
	return "", fmt.Errorf("Legacy returned no valid text")
}

// --- Local AI ---

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens"`
	Stream    bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (m *MultiProvider) callLocal(_ context.Context, prompt string) (string, error) {
	reqBody := chatRequest{
		Model: "qwen3-0.6b",
		Messages: []chatMessage{
			{Role: "system", Content: "Eres un asistente de WhatsApp. Responde de forma concisa, clara y en el mismo idioma del usuario. No incluyas etiquetas, tokens ni marcadores internos en tu respuesta."},
			{Role: "user", Content: prompt},
		},
		MaxTokens: 300,
		Stream:    false,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal error: %v", err)
	}

	httpClient := &http.Client{Timeout: 20 * time.Second}
	resp, err := httpClient.Post(m.cfg.LocalURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("local server error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("local server returned status %d", resp.StatusCode)
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode error: %v", err)
	}
	if len(chatResp.Choices) == 0 || chatResp.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("no response from local server")
	}
	return sanitizeResponse(chatResp.Choices[0].Message.Content), nil
}
