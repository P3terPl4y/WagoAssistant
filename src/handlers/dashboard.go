package handlers

import (
	"App/src/app"
	"App/src/pkg/logger"
	"App/src/ports"
	"fmt"
	"os"

	"github.com/gofiber/fiber/v3"
)

// DashboardHandler serves the dashboard HTML page.
type DashboardHandler struct {
	userSvc    *app.UserService
	botRepo    ports.BotRepository
	promptRepo ports.PromptRepository
	subRepo    ports.SubscriptionRepository
	cache      ports.CacheService
	logger     logger.Logger
}

func NewDashboardHandler(userSvc *app.UserService, botRepo ports.BotRepository, promptRepo ports.PromptRepository, subRepo ports.SubscriptionRepository, cache ports.CacheService, log logger.Logger) *DashboardHandler {
	return &DashboardHandler{userSvc: userSvc, botRepo: botRepo, promptRepo: promptRepo, subRepo: subRepo, cache: cache, logger: log.WithComponent("dashboard")}
}

func (h *DashboardHandler) Render(c fiber.Ctx) error {
	userID := c.Locals("user_id").(int)
	role := c.Locals("role").(string)
	user, err := h.userSvc.GetByID(c, userID)
	if err != nil || user == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Usuario no encontrado"})
	}
	bots, err := h.botRepo.GetByUser(c, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al obtener bots"})
	}
	var botID int
	var botInfo string
	var currentPrompt string
	var paymentStatus string
	var tier string = "free"
	var msgLimit int = 10
	var usage int = 0

	if len(bots) == 0 {
		botInfo = "No tienes ningún bot. Crea uno desde aquí."
	} else {
		bot := bots[0]
		botID = bot.ID
		paymentStatus = bot.PaymentStatus
		botInfo = fmt.Sprintf("Bot ID: %d | Bloqueado: %v | Pago: %s", bot.ID, bot.Blocked, bot.PaymentStatus)
		prompt, _ := h.promptRepo.Get(c, bot.ID)
		currentPrompt = prompt

		sub, err := h.subRepo.Get(c, bot.ID)
		if err == nil && sub != nil {
			tier = sub.Tier
			msgLimit = sub.MsgLimit
		}
		if h.cache != nil && h.cache.Available() {
			usage, _ = h.cache.GetUsage(c, bot.ID)
		}
	}
	_ = botInfo

	// Inject JS variables and serve the existing dashboard template
	html := fmt.Sprintf(`<script>window.userID=%d;window.botID=%d;window.userDisplay=%q;window.userEmail=%q;window.userPhone=%q;window.userRole=%q;window.paymentStatus=%q;window.currentPrompt=%q;window.tier=%q;window.msgLimit=%d;window.usage=%d;</script>`,
		userID, botID, user.Username, user.Email, user.Phone, role, paymentStatus, currentPrompt, tier, msgLimit, usage)

	// Read and serve the dashboard HTML file with injected variables
	content, err := os.ReadFile("./src/static/dashboard.html")
	if err != nil {
		return c.Status(500).SendString("Error loading dashboard template")
	}
	c.Set("Content-Type", "text/html")
	return c.SendString(html + string(content))
}
