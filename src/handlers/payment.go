package handlers

import (
	"App/src/config"
	"App/src/pkg/logger"
	"App/src/ports"
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
)

// PaymentHandler handles subscription checkout and webhook events.
type PaymentHandler struct {
	subs   ports.SubscriptionRepository
	bots   ports.BotRepository
	logger logger.Logger
	config *config.Config
}

func NewPaymentHandler(subs ports.SubscriptionRepository, bots ports.BotRepository, log logger.Logger, config *config.Config) *PaymentHandler {
	return &PaymentHandler{
		subs:   subs,
		bots:   bots,
		logger: log.WithComponent("payment_handler"),
		config: config,
	}
}

// generateSignature genera la firma MD5 requerida por Cryptomus.
// La firma es: md5(base64_encode(jsonBody) + API_KEY)
func generateSignature(jsonBody []byte, apiKey string) string {
	// Codificar el body en Base64
	base64Body := base64.StdEncoding.EncodeToString(jsonBody)
	// Concatenar con la API Key y calcular MD5
	data := base64Body + apiKey
	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%x", hash)
}
func (h *PaymentHandler) Checkout(c fiber.Ctx) error {
	var req struct {
		BotID int    `json:"bot_id"`
		Tier  string `json:"tier"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Precio según tier (como float64)
	var amount float64
	switch req.Tier {
	case "pro":
		amount = 10.00
	case "enterprise":
		amount = 30.00
	default:
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tier"})
	}

	// ID único
	orderID := fmt.Sprintf("bot_%d_%d", req.BotID, time.Now().UnixNano())

	// Payload IDÉNTICO al curl que funciona
	payload := map[string]interface{}{
		"amount":       fmt.Sprintf("%.2f", amount), // "10.00" como string
		"external_ref": orderID,
		"network":      "TRC20",
		"token":        "USDT",
	}

	// Marshal UNA SOLA VEZ
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to marshal payload")
		return c.Status(500).JSON(fiber.Map{"error": "Internal error"})
	}

	h.logger.Info().Str("payload", string(jsonPayload)).Msg("Sending to PayzCore")

	// Llamada a la API
	url := "https://api.payzcore.com/v1/payments"
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Internal error"})
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", h.config.PayzCore_Api_Key)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		h.logger.Error().Err(err).Msg("PayzCore request failed")
		return c.Status(500).JSON(fiber.Map{"error": "Payment gateway unavailable"})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Internal error"})
	}

	h.logger.Info().Int("status", resp.StatusCode).Str("response", string(body)).Msg("PayzCore response")

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		h.logger.Error().Err(err).Msg("Failed to parse response")
		return c.Status(500).JSON(fiber.Map{"error": "Invalid gateway response"})
	}

	// Verificar si la respuesta fue exitosa
	// PayzCore devuelve "success": true en lugar de "state": 0
	if success, ok := result["success"].(bool); !ok || !success {
		errMsg := "Unknown error"
		if msg, ok := result["error"].(string); ok {
			errMsg = msg
		}
		h.logger.Error().Msgf("PayzCore error: %s", errMsg)
		return c.Status(400).JSON(fiber.Map{"error": errMsg})
	}

	// Extraer datos de pago
	paymentData, ok := result["payment"].(map[string]interface{})
	if !ok {
		return c.Status(500).JSON(fiber.Map{"error": "Unexpected response format"})
	}

	address, _ := paymentData["address"].(string)
	qrCode, _ := paymentData["qr_code"].(string) // PayzCore devuelve "qr_code"
	paymentURL, _ := paymentData["payment_url"].(string)

	return c.JSON(fiber.Map{
		"checkout_address": address,
		"qr_code":          qrCode,
		"payment_url":      paymentURL,
		"order_id":         orderID,
		"tier":             req.Tier,
		"status":           "pending",
		"instructions":     "Envía el monto exacto en USDT (TRC-20) a esta dirección.",
	})
}
func (h *PaymentHandler) Webhook(c fiber.Ctx) error {
	body := c.Body()
	if len(body) == 0 {
		return c.Status(400).SendString("Empty body")
	}

	signature := c.Get("x-payzcore-signature")
	timestamp := c.Get("x-payzcore-timestamp")
	if signature == "" || timestamp == "" {
		h.logger.Warn().Msg("Webhook missing signature or timestamp")
		return c.Status(400).SendString("Missing signature or timestamp")
	}

	// Replay protection: max 5 minutos de diferencia
	ts, _ := strconv.ParseInt(timestamp, 10, 64)
	if time.Now().Unix()-ts > 300 {
		h.logger.Warn().Msg("Webhook too old")
		return c.Status(401).SendString("Webhook too old")
	}

	// Construir el mensaje a verificar
	message := fmt.Sprintf("%s.%s", timestamp, string(body))

	// Calcular HMAC-SHA256
	mac := hmac.New(sha256.New, []byte(h.config.PayzCore_Webhook_Secret))
	mac.Write([]byte(message))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	// Comparar (en tiempo constante)
	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		h.logger.Warn().Msg("Webhook invalid signature")
		return c.Status(401).SendString("Invalid signature")
	}

	// Parsear payload
	var payload struct {
		ExternalRef string `json:"externalRef"`
		Status      string `json:"status"`
		Amount      string `json:"amount"`
		TxID        string `json:"txId"`
	}
	json.Unmarshal(body, &payload)

	if payload.Status == "paid" {
		// Extraer botID y tier del externalRef
		var botID int
		var tier string
		fmt.Sscanf(payload.ExternalRef, "bot_%d_%s", &botID, &tier)

		// Guardar suscripción en BD
		// ... (tu lógica actual)
		h.logger.Info().Str("order", payload.ExternalRef).Msg("Payment confirmed")
	}

	return c.SendStatus(200)
}

/* CODIGO FUNCIONAL
// Checkout genera una factura en Cryptomus y devuelve la URL de pago.
func (h *PaymentHandler) Checkout(c fiber.Ctx) error {
	var req struct {
		BotID int    `json:"bot_id"`
		Tier  string `json:"tier"` // 'pro' or 'enterprise'
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Define el precio según el tier
	var amount string
	switch req.Tier {
	case "pro":
		amount = "10.00"
	case "enterprise":
		amount = "30.00"
	default:
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tier"})
	}

	// Genera un order_id único (alfanumérico, guiones y guiones bajos permitidos)
	orderID := fmt.Sprintf("bot_%d_%d", req.BotID, time.Now().UnixNano())

	// Prepara el payload para Cryptomus
	payload := map[string]interface{}{
		"amount":       amount,
		"currency":     "USD",
		"order_id":     orderID,
		"url_callback": h.config.CryptomusWebhookURL,
		"lifetime":     3600, // 1 hora para pagar
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to marshal payment payload")
		return c.Status(500).JSON(fiber.Map{"error": "Internal error"})
	}

	// Generar la firma correcta: md5(base64_encode(body) + API_KEY)
	signature := generateSignature(jsonPayload, h.config.CryptomusAPIKey)

	// Llama a la API de Cryptomus
	url := "https://api.cryptomus.com/v1/payment"
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Internal error"})
	}

	// Headers correctos según documentación
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("userId", h.config.CryptomusMerchantID) // ¡userId, no merchant!
	httpReq.Header.Set("sign", signature)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		h.logger.Error().Err(err).Msg("Cryptomus API request failed")
		return c.Status(500).JSON(fiber.Map{"error": "Payment gateway unavailable"})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Internal error"})
	}

	// Procesa la respuesta
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		h.logger.Error().Err(err).Msg("Failed to parse Cryptomus response")
		return c.Status(500).JSON(fiber.Map{"error": "Invalid gateway response"})
	}

	// Verifica que la respuesta sea exitosa (state == 0)
	if state, ok := result["state"].(float64); !ok || state != 0 {
		errMsg := "Unknown error"
		if msg, ok := result["message"].(string); ok {
			errMsg = msg
		}
		h.logger.Error().Msgf("Cryptomus error: %s", errMsg)
		return c.Status(400).JSON(fiber.Map{"error": errMsg})
	}

	// Extrae la URL de pago del campo "result.url"
	resultData, ok := result["result"].(map[string]interface{})
	if !ok {
		return c.Status(500).JSON(fiber.Map{"error": "Unexpected gateway response"})
	}
	checkoutURL, ok := resultData["url"].(string)
	if !ok {
		return c.Status(500).JSON(fiber.Map{"error": "Missing payment URL"})
	}

	return c.JSON(fiber.Map{
		"checkout_url": checkoutURL,
		"order_id":     orderID,
		"tier":         req.Tier,
		"status":       "pending",
	})
}
*
// Webhook recibe notificaciones de pago de Cryptomus.
func (h *PaymentHandler) Webhook(c fiber.Ctx) error {
	// 1. Leer el cuerpo
	body := c.Body()
	if len(body) == 0 {
		return c.Status(400).SendString("Empty body")
	}

	// 2. Obtener la firma del header
	sigHeader := c.Get("sign")
	if sigHeader == "" {
		h.logger.Warn().Msg("Webhook missing signature")
		return c.Status(400).SendString("Missing signature")
	}

	// 3. Validar la firma (usando el mismo método que en Checkout)
	expectedSignature := generateSignature(body, h.config.CryptomusAPIKey)
	if sigHeader != expectedSignature {
		h.logger.Warn().Msg("Webhook invalid signature")
		return c.Status(401).SendString("Invalid signature")
	}

	// 4. Parsear el payload
	var payload struct {
		OrderID  string `json:"order_id"`
		Status   string `json:"status"`
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		h.logger.Error().Err(err).Msg("Failed to parse webhook payload")
		return c.Status(400).SendString("Bad payload")
	}

	// 5. Solo procesar si el pago fue exitoso
	// Estados posibles: process, check, paid, paid_over, fail, wrong_amount, cancel, system_fail, refund_process, refund_fail, refund_paid
	if payload.Status != "paid" && payload.Status != "paid_over" {
		// paid_over significa que se pagó más de lo debido
		return c.SendStatus(200)
	}

	// 6. Extraer bot_id y tier del order_id
	// Formato: "bot_{botID}_{timestamp}"
	var botID int
	var tier string
	n, err := fmt.Sscanf(payload.OrderID, "bot_%d_%s", &botID, &tier)
	if err != nil || n != 2 {
		h.logger.Error().Str("order_id", payload.OrderID).Msg("Invalid order_id format")
		return c.Status(400).SendString("Invalid order_id")
	}

	// Determinar límite según tier
	limit := 1000
	if tier == "enterprise" {
		limit = -1 // ilimitado
	}

	// 7. Guardar la suscripción
	sub := &domain.Subscription{
		BotID:     botID,
		Tier:      tier,
		MsgLimit:  limit,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}

	if err := h.subs.Save(c, sub); err != nil {
		h.logger.Error().Err(err).Int("bot_id", botID).Msg("Failed to save subscription")
		return c.Status(500).SendString("Internal error")
	}

	// Actualizar estado de pago en la tabla bots
	if err := h.bots.UpdatePaymentStatus(c, botID, "paid"); err != nil {
		h.logger.Error().Err(err).Msg("Failed to update payment status")
	}

	h.logger.Info().
		Int("bot_id", botID).
		Str("tier", tier).
		Str("order_id", payload.OrderID).
		Str("status", payload.Status).
		Msg("Subscription paid via Cryptomus webhook")

	return c.SendStatus(200)
}
*/
