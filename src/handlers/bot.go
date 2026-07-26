package handlers

import (
	"App/src/adapters/notifications"
	"App/src/app"
	"App/src/pkg/concurrency"
	"App/src/pkg/logger"
	"App/src/ports"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/skip2/go-qrcode"
)

// BotHandler handles bot-related HTTP endpoints.
type BotHandler struct {
	botSvc      *app.BotService
	botRepo     ports.BotRepository
	promptRepo  ports.PromptRepository
	promptCache *concurrency.PromptCache
	botMgr      *concurrency.BotManager
	logger      logger.Logger
	maxBots     int
	gNotifier   *notifications.GmailNotifier
}

func NewBotHandler(botSvc *app.BotService, botRepo ports.BotRepository, promptRepo ports.PromptRepository, promptCache *concurrency.PromptCache, botMgr *concurrency.BotManager, log logger.Logger, maxBots int, gNotifier *notifications.GmailNotifier) *BotHandler {
	return &BotHandler{botSvc: botSvc, botRepo: botRepo, promptRepo: promptRepo, promptCache: promptCache, botMgr: botMgr, logger: log.WithComponent("bot_handler"), maxBots: maxBots, gNotifier: gNotifier}
}

func (h *BotHandler) StartBot(c fiber.Ctx) error {
	userID := c.Locals("user_id").(int)
	role := c.Locals("role").(string)
	ctx := c
	h.logger.Info().Msg("Enviando notificacion al admin")
	msg := fmt.Sprintf("Tienes pagos por confirmar")
	err := h.gNotifier.SendAdminNotification(msg, "Ve a confirmar el pago")
	if err != nil {
		h.logger.Error().Msg(err.Error())
	}
	h.logger.Info().Msg("Se debio enviar notificacion a admin")
	bots, err := h.botRepo.GetByUser(ctx, userID)
	if err != nil {
		return c.JSON(fiber.Map{"status": "error", "message": "Error al verificar bots"})
	}

	if len(bots) > 0 {
		bot := bots[0]
		if bot.Blocked {
			return c.JSON(fiber.Map{"status": "error", "message": "El bot está bloqueado. Contacta al administrador."})
		}
		if role != "admin" {

			if bot.PaymentStatus == "pending" {
				msg := fmt.Sprintf("Tienes pagos por confirmar")
				err := h.gNotifier.SendAdminNotification(msg, "Ve a confirmar el pago")
				if err != nil {
					h.logger.Error().Msg(err.Error())
				}
				fmt.Println("Enviando notificacion a admin")
				return c.JSON(fiber.Map{"status": "pending_payment", "id": bot.ID, "payment_status": "pending", "message": "Pago pendiente. Espera confirmación del administrador."})
			}
			if bot.PaymentStatus != "paid" && bot.PaymentStatus != "free" {
				return c.JSON(fiber.Map{"status": "error", "message": "Estado de pago inválido."})
			}
		}
		if h.botMgr.IsActive(bot.ID) {
			return c.JSON(fiber.Map{"status": "session_exists", "id": bot.ID})
		}

		return h.LaunchBot(c, bot.ID)
	}

	// Create new bot
	status := "pending"
	if role == "admin" {
		status = "free"
	}
	sessionFile := fmt.Sprintf("whatsapp_bot%d.db", userID)
	newID, err := h.botRepo.Create(ctx, userID, sessionFile, status)
	if err != nil {
		return c.JSON(fiber.Map{"status": "error", "message": "Error al crear bot"})
	}
	newSessionFile := fmt.Sprintf("whatsapp_bot%d.db", newID)
	_ = h.botRepo.UpdateSessionFile(ctx, newID, newSessionFile)

	if role == "admin" {
		return h.LaunchBot(c, newID)
	}
	return c.JSON(fiber.Map{"status": "pending_payment", "id": newID, "payment_status": "pending", "message": "Bot creado. Se requiere confirmación de pago por parte del administrador."})
}

func (h *BotHandler) LaunchBot(c fiber.Ctx, botID int) error {
	qrResult := make(chan string, 1)
	go h.botSvc.InitBot(botID, qrResult)
	select {
	case result := <-qrResult:
		switch result {
		case "SESSION_EXISTS":
			return c.JSON(fiber.Map{"status": "session_exists", "id": botID})
		case "TIMEOUT":
			return c.JSON(fiber.Map{"status": "timeout", "id": botID})
		default:
			png, err := qrcode.Encode(result, qrcode.Medium, 256)
			if err != nil {
				return c.JSON(fiber.Map{"status": "error", "message": "Error generando QR"})
			}
			return c.JSON(fiber.Map{"status": "qr", "qr": base64.StdEncoding.EncodeToString(png), "id": botID})
		}
	case <-time.After(35 * time.Second):
		return c.JSON(fiber.Map{"status": "error", "message": "Tiempo de espera agotado"})
	}
}

func (h *BotHandler) GetBotIDStatus(c fiber.Ctx) error {
	botID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"status": "error", "message": "ID inválido"})
	}
	userID := c.Locals("user_id").(int)
	role := c.Locals("role").(string)

	bot, err := h.botRepo.GetByID(c, botID)
	if err != nil || bot == nil {
		return c.JSON(fiber.Map{"status": "error", "message": "Bot no encontrado"})
	}
	if role != "admin" && bot.UserID != userID {
		return c.Status(403).JSON(fiber.Map{"status": "error", "message": "No autorizado"})
	}
	if h.botMgr.IsActive(botID) {
		return c.JSON(fiber.Map{"status": "active"})
	}
	if bot.PaymentStatus == "pending" {
		return c.JSON(fiber.Map{"status": "pending_payment", "id": botID})
	}
	return c.JSON(fiber.Map{"status": "inactive"})
}

func (h *BotHandler) UpdatePrompt(c fiber.Ctx) error {
	botID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID inválido"})
	}
	userID := c.Locals("user_id").(int)
	role := c.Locals("role").(string)
	bot, err := h.botRepo.GetByID(c, botID)
	if err != nil || bot == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Bot no encontrado"})
	}
	if role != "admin" && bot.UserID != userID {
		return c.Status(403).JSON(fiber.Map{"error": "No autorizado"})
	}
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(c.Body(), &req); err != nil || req.Prompt == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Prompt requerido"})
	}
	if err := h.promptRepo.Save(c, botID, req.Prompt); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al guardar prompt"})
	}
	h.promptCache.Invalidate(botID)
	return c.JSON(fiber.Map{"status": "ok"})
}

func (h *BotHandler) ActiveBots(c fiber.Ctx) error {
	userID := c.Locals("user_id").(int)
	role := c.Locals("role").(string)

	if role == "admin" {
		return c.JSON(fiber.Map{"bots": h.botMgr.ActiveIDs()})
	}

	bots, err := h.botRepo.GetByUser(c, userID)
	if err != nil {
		return c.JSON(fiber.Map{"bots": []int{}})
	}
	ids := []int{}
	for _, b := range bots {
		if h.botMgr.IsActive(b.ID) {
			ids = append(ids, b.ID)
		}
	}
	return c.JSON(fiber.Map{"bots": ids})
}
