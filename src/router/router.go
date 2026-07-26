package router

import (
	"App/src/handlers"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/static"
)

// Setup configures all routes with the given handler instances.
func Setup(
	app *fiber.App,
	authH *handlers.AuthHandler,
	botH *handlers.BotHandler,
	adminH *handlers.AdminHandler,
	dashH *handlers.DashboardHandler,
	googleH *handlers.GoogleHandler,
	paymentH *handlers.PaymentHandler,
	rateLimitPerMinute int,
	cookieSecure bool,
) {
	// CORS: allow everything so nothing gets blocked behind reverse proxies
	app.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"https://wago.redcliente.cl", "http://localhost:3000"},
		AllowCredentials: true,
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "X-CSRF-Token", "Authorization"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
	}))

	// Rate limiter
	lim := limiter.New(limiter.Config{
		Max:          rateLimitPerMinute,
		Expiration:   1 * time.Minute,
		KeyGenerator: func(c fiber.Ctx) string { return c.IP() },
		LimitReached: func(c fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "Demasiadas peticiones. Intenta de nuevo en un minuto."})
		},
	})

	// Static assets (CSS, JS, images)
	app.Use(static.New("./src/static/"))

	// ──── Pages ────
	app.Get("/", func(c fiber.Ctx) error { return c.SendFile("./src/static/index.html") })
	app.Get("/sing", func(c fiber.Ctx) error { return c.SendFile("./src/static/sing.html") })
	app.Get("/register", func(c fiber.Ctx) error { return c.SendFile("./src/static/register.html") })

	// ──── Google OAuth ────
	app.Get("/auth/google/login", googleH.Login)
	app.Get("/auth/google/callback", googleH.Callback)

	// ──── Auth (public) ────
	app.Post("/register", lim, authH.Register)
	app.Post("/login", lim, authH.Login)
	app.Post("/logout", handlers.AuthRequired, authH.Logout)

	// ──── User (authenticated) ────
	app.Put("/user/password", handlers.AuthRequired, authH.UpdatePassword)
	app.Put("/user/phone", handlers.AuthRequired, authH.UpdatePhone)

	// ──── Dashboard ────
	app.Get("/dashboard", handlers.AuthRequired, dashH.Render)

	// ──── Bot ────
	app.Post("/start-bot", handlers.AuthRequired, lim, botH.StartBot)
	app.Get("/bot/:id/status", handlers.AuthRequired, botH.GetBotIDStatus)
	app.Put("/bot/:id/prompt", handlers.AuthRequired, botH.UpdatePrompt)
	app.Get("/active-bots", handlers.AuthRequired, botH.ActiveBots)

	// ──── Admin ────
	admin := app.Group("/admin", handlers.AuthRequired, handlers.AdminRequired)
	admin.Get("/", func(c fiber.Ctx) error { return c.SendFile("./src/static/admin.html") })
	admin.Get("/users", adminH.ListUsers)
	admin.Get("/bots/status", adminH.GetAllBotsStatus)
	admin.Post("/bots/create", adminH.CreateBot)
	admin.Post("/payments/confirm/:id", adminH.ConfirmPayment)
	admin.Post("/bots/:id/block", adminH.BlockBot)
	admin.Delete("/bots/:id", adminH.DeleteBot)
	admin.Put("/users/:id/password", adminH.UpdateUserPassword)
	admin.Delete("/users/:id", adminH.DeleteUser)
	admin.Get("/metrics", adminH.GetMetrics)

	// ──── Payments ────
	app.Post("/api/payments/checkout", handlers.AuthRequired, paymentH.Checkout)
	app.Post("/api/payments/webhook", paymentH.Webhook)
}
