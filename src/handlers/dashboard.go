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
	metrics := `<!-- SECCIÓN: ESTADO DE IA -->
<div class="glass-card" style="margin-top:1.5rem;">
    <h3><i class="fas fa-brain"></i> Estado del Servidor IA</h3>
    <div id="aiHealthContainer">
        <div style="display:flex; flex-wrap:wrap; gap:1.5rem; margin-top:0.75rem;">
            <div style="flex:1; min-width:150px;">
                <span style="color:var(--color-text-muted); font-size:0.9rem;">Estado</span><br>
                <span id="aiHealthStatus" class="status-badge inactive">Cargando...</span>
            </div>
            <div style="flex:1; min-width:150px;">
                <span style="color:var(--color-text-muted); font-size:0.9rem;">PID</span><br>
                <span id="aiHealthPid" style="font-weight:600;">-</span>
            </div>
            <div style="flex:1; min-width:150px;">
                <span style="color:var(--color-text-muted); font-size:0.9rem;">Reinicios</span><br>
                <span id="aiHealthRestarts" style="font-weight:600;">0</span>
            </div>
            <div style="flex:1; min-width:150px;">
                <span style="color:var(--color-text-muted); font-size:0.9rem;">Última verificación</span><br>
                <span id="aiHealthLastCheck" style="font-weight:600;">-</span>
            </div>
            <div style="flex:1; min-width:150px;">
                <span style="color:var(--color-text-muted); font-size:0.9rem;">Tiempo activo</span><br>
                <span id="aiHealthUptime" style="font-weight:600;">-</span>
            </div>
        </div>
        <div style="margin-top:0.75rem; font-size:0.85rem; color:var(--color-text-muted);">
            <span>Modelo: </span><code id="aiHealthModel" style="background:var(--color-bg-input); padding:0.1rem 0.4rem; border-radius:4px;">-</code>
            &nbsp;|&nbsp; <span>Binario: </span><code id="aiHealthBin" style="background:var(--color-bg-input); padding:0.1rem 0.4rem; border-radius:4px;">-</code>
            &nbsp;|&nbsp; <span>Último error: </span><span id="aiHealthLastError" style="color:#ef4444;">-</span>
        </div>
    </div>
</div>

<script>
function fetchAIHealth() {
    fetch('/admin/ai-health', { headers: { 'X-CSRF-Token': getCsrfToken() } })
        .then(res => res.ok ? res.json() : Promise.reject('Error'))
        .then(data => {
            document.getElementById('aiHealthStatus').textContent = data.is_healthy ? '🟢 Activo' : '🔴 Inactivo';
            document.getElementById('aiHealthStatus').className = 'status-badge ' + (data.is_healthy ? 'active' : 'inactive');
            document.getElementById('aiHealthPid').textContent = data.pid || '-';
            document.getElementById('aiHealthRestarts').textContent = data.restart_count || 0;
            document.getElementById('aiHealthLastCheck').textContent = data.last_check ? new Date(data.last_check).toLocaleString() : '-';
            if (data.uptime) {
                const s = Math.floor(data.uptime / 1000);
                const h = Math.floor(s / 3600);
                const m = Math.floor((s % 3600) / 60);
                document.getElementById('aiHealthUptime').textContent = '${h}h ${m}m';
            } else {
                document.getElementById('aiHealthUptime').textContent = '-';
            }
            document.getElementById('aiHealthModel').textContent = data.model_path || '-';
            document.getElementById('aiHealthBin').textContent = data.bin_path || '-';
            document.getElementById('aiHealthLastError').textContent = data.last_error || 'Sin errores';
        })
        .catch(() => {
            document.getElementById('aiHealthStatus').textContent = '⚠️ Error';
            document.getElementById('aiHealthStatus').className = 'status-badge warning';
        });
}
document.addEventListener('DOMContentLoaded', fetchAIHealth);
setInterval(fetchAIHealth, 10000);
</script>`
	// Inject JS variables and serve the existing dashboard template
	html := fmt.Sprintf(`<script>window.userID=%d;window.botID=%d;window.userDisplay=%q;window.userEmail=%q;window.userPhone=%q;window.userRole=%q;window.paymentStatus=%q;window.currentPrompt=%q;window.tier=%q;window.msgLimit=%d;window.usage=%d;</script>`,
		userID, botID, user.Username, user.Email, user.Phone, role, paymentStatus, currentPrompt, tier, msgLimit, usage)

	// Read and serve the dashboard HTML file with injected variables
	content, err := os.ReadFile("./src/static/dashboard.html")
	if err != nil {
		return c.Status(500).SendString("Error loading dashboard template")
	}
	c.Set("Content-Type", "text/html")
	if role == "admin" {
		return c.SendString(html + string(content) + metrics)
	}
	return c.SendString(html + string(content))
}
func (h *DashboardHandler) ListPedidos(c fiber.Ctx) error {
	limit := c.Query("limit", "100")
	offset := c.Query("offset", "0")
	userID := c.Locals("user_id").(int)
	user, err := h.userSvc.ListAllHistory(c, userID, limit, offset)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Error al obtener usuarios"})
	}
	return c.JSON(user)
}
