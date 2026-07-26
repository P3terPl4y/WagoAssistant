package handlers

import (
	"App/src/domain"
	"App/src/pkg/logger"
	"App/src/ports"
	"time"

	"github.com/gofiber/fiber/v3"
)

// PaymentHandler handles subscription checkout and webhook events.
type PaymentHandler struct {
	subs   ports.SubscriptionRepository
	bots   ports.BotRepository
	logger logger.Logger
}

func NewPaymentHandler(subs ports.SubscriptionRepository, bots ports.BotRepository, log logger.Logger) *PaymentHandler {
	return &PaymentHandler{
		subs:   subs,
		bots:   bots,
		logger: log.WithComponent("payment_handler"),
	}
}

// Checkout generates a placeholder checkout session URL.
func (h *PaymentHandler) Checkout(c fiber.Ctx) error {
	var req struct {
		BotID int    `json:"bot_id"`
		Tier  string `json:"tier"` // 'pro' or 'enterprise'
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// TODO: Replace with Stripe or PayPal SDK call to create a Checkout Session
	// return stripe.Checkout.New(&stripe.CheckoutSessionParams{ ... })

	// Mock response
	mockURL := "https://checkout.stripe.com/c/pay/cs_test_mock..."
	return c.JSON(fiber.Map{
		"checkout_url": mockURL,
		"tier":         req.Tier,
		"status":       "pending",
	})
}

// Webhook receives asynchronous payment confirmations from gateways.
func (h *PaymentHandler) Webhook(c fiber.Ctx) error {
	// TODO: Validate webhook signature using Stripe/PayPal secret
	// sig := c.Get("Stripe-Signature")
	// event, err := webhook.ConstructEvent(c.Body(), sig, endpointSecret)

	// For demonstration, we'll parse a mock payload
	var req struct {
		BotID  int    `json:"bot_id"`
		Tier   string `json:"tier"`
		Status string `json:"status"` // e.g. "succeeded"
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).SendString("Bad Request")
	}

	if req.Status != "succeeded" {
		return c.SendStatus(200) // Ignore pending/failed events
	}
	
	limit := 1000 // Pro limit
	if req.Tier == "enterprise" {
		limit = -1
	}

	sub := &domain.Subscription{
		BotID:     req.BotID,
		Tier:      req.Tier,
		MsgLimit:  limit,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour), // 30 days
	}

	if err := h.subs.Save(c, sub); err != nil {
		h.logger.Error().Err(err).Int("bot_id", req.BotID).Msg("Failed to update subscription via webhook")
		return c.Status(500).SendString("Internal Error")
	}

	// Also update bots table to mark payment status
	if err := h.bots.UpdatePaymentStatus(c, req.BotID, "paid"); err != nil {
		h.logger.Error().Err(err).Msg("Failed to update payment status")
	}

	h.logger.Info().Int("bot_id", req.BotID).Str("tier", req.Tier).Msg("Subscription upgraded via webhook")
	return c.SendStatus(200)
}
