package handlers

import (
	"App/src/adapters/notifications"
	"App/src/app"
	"App/src/pkg/concurrency"
	"App/src/pkg/logger"
	"App/src/ports"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
)

// AdminHandler handles admin-only HTTP endpoints.
type AdminHandler struct {
	userSvc    *app.UserService
	botSvc     *app.BotService
	userRepo   ports.UserRepository
	botRepo    ports.BotRepository
	promptRepo ports.PromptRepository
	botMgr     *concurrency.BotManager
	db         *sql.DB
	cache      ports.CacheService
	logger     logger.Logger
	maxBots    int
	gNotifier  *notifications.GmailNotifier
}

func NewAdminHandler(userSvc *app.UserService, botSvc *app.BotService, userRepo ports.UserRepository, botRepo ports.BotRepository, promptRepo ports.PromptRepository, botMgr *concurrency.BotManager, db *sql.DB, cache ports.CacheService, log logger.Logger, maxBots int, gNotifier *notifications.GmailNotifier) *AdminHandler {
	return &AdminHandler{userSvc: userSvc, botSvc: botSvc, userRepo: userRepo, botRepo: botRepo, promptRepo: promptRepo, botMgr: botMgr, db: db, cache: cache, logger: log.WithComponent("admin_handler"), maxBots: maxBots, gNotifier: gNotifier}
}

func (h *AdminHandler) ListUsers(c fiber.Ctx) error {
	limit := c.Query("limit", "100")
	offset := c.Query("offset", "0")
	users, err := h.userSvc.ListAll(c, limit, offset)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al obtener usuarios"})
	}
	result := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		result = append(result, map[string]interface{}{
			"id": u.ID, "username": u.Username, "email": u.Email,
			"phone": u.Phone, "role": u.Role, "created_at": u.CreatedAt,
		})
	}
	return c.JSON(fiber.Map{"users": result})
}

func (h *AdminHandler) GetAllBotsStatus(c fiber.Ctx) error {
	bots, err := h.botRepo.GetAll(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al obtener bots"})
	}
	result := []map[string]interface{}{}
	for _, b := range bots {
		user, err := h.userSvc.GetByID(c, b.UserID)
		username := ""
		if err == nil && user != nil {
			username = user.Username
		}
		prompt, _ := h.promptRepo.Get(c, b.ID)
		result = append(result, map[string]interface{}{
			"id": b.ID, "user_id": b.UserID, "username": username,
			"blocked": b.Blocked, "active": h.botMgr.IsActive(b.ID),
			"payment_status": b.PaymentStatus, "session_file": b.SessionFile,
			"prompt": prompt, "created_at": b.CreatedAt,
		})
	}
	return c.JSON(fiber.Map{"bots": result})
}
func (h *AdminHandler) CreateBot(c fiber.Ctx) error {
	var req struct {
		UserID int `json:"user_id"`
	}
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Datos inválidos"})
	}
	if req.UserID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "ID de usuario inválido"})
	}
	user, err := h.userSvc.GetByID(c, req.UserID)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Usuario no encontrado"})
	}
	count, err := h.botRepo.CountByUser(c, req.UserID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al contar bots"})
	}
	if count >= h.maxBots {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("El usuario ya tiene %d bots (límite %d)", count, h.maxBots)})
	}
	sessionFile := fmt.Sprintf("./src/db/whatsapp_bot%d.db", req.UserID)
	botID, err := h.botRepo.Create(c, req.UserID, sessionFile, "free")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al crear bot"})
	}
	newSessionFile := fmt.Sprintf("./src/db/whatsapp_bot%d.db", botID)
	err = h.botRepo.UpdateSessionFile(c, botID, newSessionFile)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al actualizar la session"})
	}
	return c.JSON(fiber.Map{"status": "ok", "bot_id": botID})
}

func (h *AdminHandler) ConfirmPayment(c fiber.Ctx) error {
	botID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID inválido"})
	}
	bot, err := h.botRepo.GetByID(c, botID)
	if err != nil || bot == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Bot no encontrado"})
	}
	if bot.PaymentStatus != "pending" {
		return c.Status(400).JSON(fiber.Map{"error": "El bot no está pendiente de pago"})
	}
	if err := h.botRepo.UpdatePaymentStatus(c, botID, "paid"); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al actualizar estado de pago"})
	}
	err = h.gNotifier.SendNotification(bot.ID, "Ya puedes activar tu Asistente", fmt.Sprintf("Hola soy tu asistente personal Wago.\n Estoy listo para que me actives "))
	if err != nil {
		h.logger.Error().Msg(err.Error())
	}
	return c.JSON(fiber.Map{"status": "ok", "message": "Pago confirmado."})
}

func (h *AdminHandler) BlockBot(c fiber.Ctx) error {
	botID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID inválido"})
	}
	bot, err := h.botRepo.GetByID(c, botID)
	if err != nil || bot == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Bot no encontrado"})
	}
	if err := h.botRepo.UpdateBlocked(c, botID, !bot.Blocked); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al actualizar estado"})
	}
	return c.JSON(fiber.Map{"status": "ok", "blocked": !bot.Blocked})
}

func (h *AdminHandler) DeleteBot(c fiber.Ctx) error {
	botID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID inválido"})
	}
	if err := h.botRepo.Delete(c, botID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al eliminar bot"})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func (h *AdminHandler) UpdateUserPassword(c fiber.Ctx) error {
	userID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID inválido"})
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.Unmarshal(c.Body(), &req); err != nil || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Contraseña requerida"})
	}
	if err := h.userSvc.UpdatePassword(c, userID, req.Password); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al actualizar contraseña"})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func (h *AdminHandler) DeleteUser(c fiber.Ctx) error {
	userID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID inválido"})
	}
	if err := h.userSvc.Delete(c, userID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al eliminar usuario"})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func (h *AdminHandler) GetMetrics(c fiber.Ctx) error {
	if h.cache == nil || !h.cache.Available() {
		return c.Status(503).JSON(fiber.Map{"error": "Metrics unavailable"})
	}

	messages, err := h.cache.GetGlobalMetrics(c, "messages", 7)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error reading metrics"})
	}

	errors, err := h.cache.GetGlobalMetrics(c, "errors", 7)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error reading metrics"})
	}

	// Calculate the last 7 days for labels
	var labels []string
	now := time.Now()
	for i := 0; i < 7; i++ {
		date := now.AddDate(0, 0, -(6 - i)).Format("02 Jan")
		labels = append(labels, date)
	}

	return c.JSON(fiber.Map{
		"labels":   labels,
		"messages": messages,
		"errors":   errors,
	})
}
