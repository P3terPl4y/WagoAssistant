package main

import (
	"App/src/adapters/ai"
	"App/src/adapters/encryption"
	"App/src/adapters/kafka"
	"App/src/adapters/notifications"
	adapterRedis "App/src/adapters/redis"
 "App/src/worker"
	//"App/src/adapters/sqlite"
	"App/src/adapters/postgre"
	"App/src/app"
	"App/src/config"
	"App/src/handlers"
	"App/src/pkg/apperror"
	"App/src/pkg/concurrency"
	"App/src/pkg/logger"
	"App/src/router"
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/gofiber/fiber/v3"
	fiberLogger "github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/session"
	redisStorage "github.com/gofiber/storage/redis/v3"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func main() {
	// ============================================================
	// 1. CONFIGURATION
	// ============================================================
	cfg := config.Load()
	log := logger.New(cfg.Environment)
	log.Info().Str("env", cfg.Environment).Msg("Starting Wago")

	// ============================================================
	// 2. DATABASE (sqlite)
	// ============================================================
	/*db := sqlite.Connect("./src/db/db", log)
	defer db.Close()
	sqlite.EnsureAdmin(db, cfg.AdminUsername, cfg.AdminEmail, cfg.AdminPhone, cfg.AdminPass, log)

	// ============================================================
	// 3. REPOSITORIES
	// ============================================================
	userRepo := sqlite.NewUserRepo(db)
	botRepo := sqlite.NewBotRepo(db)
	chatRepo := sqlite.NewChatRepo(db)
	promptRepo := sqlite.NewPromptRepo(db)
	subRepo := sqlite.NewSubscriptionRepo(db)
	oauthRepo := sqlite.NewOAuthRepo(db)
	*/
	// ============================================================
	// 2. DATABASE (Postgre)
	// ============================================================
	db, ctx := postgre.Connect(cfg.DatabaseURL, log)
	defer db.Close()
	postgre.EnsureAdmin(ctx, db, cfg.AdminUsername, cfg.AdminEmail, cfg.AdminPhone, cfg.AdminPass, log)

	// ============================================================
	// 3. REPOSITORIES
	// ============================================================
	userRepo := postgre.NewUserRepo(db)
	botRepo := postgre.NewBotRepo(db)
	chatRepo := postgre.NewChatRepo(db)
	promptRepo := postgre.NewPromptRepo(db)
	subRepo := postgre.NewSubscriptionRepo(db)
	oauthRepo := postgre.NewOAuthRepo(db)

	// ============================================================
	// 4. REDIS (optional)
	// ============================================================
	redisCache := adapterRedis.Connect(cfg.RedisURL, log)

	// ============================================================
	// 5. SERVICES
	// ============================================================
	kafkaConfig := kafka.LoadKafkaConfig()
	encSvc := encryption.NewAESGCM(cfg.EncryptionKey)
	aiSvc := ai.NewMultiProvider(cfg.AI, log)
	botMgr := concurrency.NewBotManager(log)
	promptCache := concurrency.NewPromptCache(5 * time.Minute)
	dedup := concurrency.NewMessageDedup(cfg.DedupWindow)
	userSem := concurrency.NewUserSemaphore(redisCache)
	gNotifier := notifications.NewGmailNotifier(userRepo, log)
	userSvc := app.NewUserService(userRepo, log)
	chatSvc := app.NewChatService(chatRepo, encSvc, redisCache, log, cfg.MaxHistory, cfg.MaxHistoryChars)
	botSvc := app.NewBotService(
		botRepo, promptRepo, subRepo, userRepo, chatSvc, aiSvc,
		botMgr, promptCache, dedup, userSem, redisCache, log, cfg, gNotifier, kafkaConfig)
	// Iniciar consumidor de Kafka (si está habilitado)
	if kafkaConfig.Enabled {
		if err := botSvc.StartKafkaConsumer(botSvc.ProcessKafkaMessage); err != nil {
			log.Fatal().Err(err).Msg("Failed to start Kafka consumer")
		}
		log.Info().Msg("Kafka consumer started")
	}
 // ============================================================
    //  WORKER: MONITOREO Y REINICIO DE IA LOCAL
    // ============================================================
    // Solo si LOCAL_AI_ENABLED es true y se ha configurado el binario
    if os.Getenv("LOCAL_AI_ENABLED") == "true" {
        aiCfg := worker.AIHealthConfig{
            BinPath:       os.Getenv("AI_BIN_PATH"),
            ModelPath:     os.Getenv("AI_MODEL_PATH"),
            ListenAddr:    os.Getenv("AI_LISTEN_ADDR"),
            HealthURL:     os.Getenv("AI_HEALTH_URL"),
            CheckInterval: 5 * time.Minute, // cada 5 min (configurable)
            RestartHour:   3,               // 3 AM
            ExtraArgs:     []string{"-threads", "2", "-batch-size", "64", "-ctx-size", "2048"},
        }

        // Si no hay valores por defecto, usar defaults
        if aiCfg.BinPath == "" {
            aiCfg.BinPath = "/var/www/lucifer/pia/go-pherence/llmserver"
        }
        if aiCfg.ModelPath == "" {
            aiCfg.ModelPath = "/var/www/lucifer/pia/go-pherence/models/qwen2.5-0.5b.Q4_K_M.gguf"
        }
        if aiCfg.ListenAddr == "" {
            aiCfg.ListenAddr = ":8080"
        }
        if aiCfg.HealthURL == "" {
            aiCfg.HealthURL = "http://localhost:8080/health"
        }

        healthWorker := worker.NewAIHealthWorker(aiCfg, log)
        healthWorker.Start(context.Background())
        defer healthWorker.Stop()

        log.Info().Msg("AI Health Worker iniciado (monitoreo y reinicio automático)")
    } else {
        log.Info().Msg("AI Health Worker deshabilitado (LOCAL_AI_ENABLED=false)")
    }
	// ============================================================
	// 7. OAUTH CONFIG
	// ============================================================
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURL:  cfg.GoogleRedirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/calendar",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}

	// ============================================================
	// 8. FIBER APP
	// ============================================================
	fiberApp := fiber.New(fiber.Config{
		TrustProxy:   true,
		ErrorHandler: apperror.FiberErrorHandler,
	})

	// Middleware
	fiberApp.Use(fiberLogger.New(fiberLogger.Config{
		Format: "${time} - ${method} ${path} ${status}\n",
	}))
	fiberApp.Use(handlers.SecurityHeaders)

	// Session middleware
	var storage fiber.Storage
	if redisCache != nil && redisCache.Available() && cfg.RedisURL != "" {
		storage = redisStorage.New(redisStorage.Config{URL: cfg.RedisURL})
		log.Info().Msg("Sessions stored in Redis")
	} else {
		log.Warn().Msg("Sessions stored in memory (no Redis)")
	}
	sessionMW := session.New(session.Config{
		CookieSecure:   cfg.CookieSecure,
		CookieHTTPOnly: true,
		CookieSameSite: "Lax",
		IdleTimeout:    cfg.SessionExpiration,
		Storage:        storage,
	})
	fiberApp.Use(sessionMW)

	// ============================================================
	// 9. HANDLERS
	// ============================================================
	authH := handlers.NewAuthHandler(userSvc, log)
	botH := handlers.NewBotHandler(botSvc, userSvc, botRepo, promptRepo, promptCache, botMgr, log, cfg.MaxBots, gNotifier)
	adminH := handlers.NewAdminHandler(userSvc, botSvc, userRepo, botRepo, promptRepo, botMgr, db, redisCache, log, cfg.MaxBots, gNotifier)
	dashH := handlers.NewDashboardHandler(userSvc, botRepo, promptRepo, subRepo, redisCache, log)
	googleH := handlers.NewGoogleHandler(oauthCfg, userRepo, oauthRepo, log)
	paymentH := handlers.NewPaymentHandler(subRepo, botRepo, log)

	// ============================================================
	// 10. ROUTING
	// ============================================================
	router.Setup(fiberApp, authH, botH, adminH, dashH, googleH, paymentH, cfg.RateLimitPerMinute, cfg.CookieSecure)

	// ============================================================
	// 11. GRACEFUL SHUTDOWN
	// ============================================================
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info().Str("addr", cfg.ServerAddr).Msg("Server starting")
		if err := fiberApp.Listen(cfg.ServerAddr); err != nil {
			log.Fatal().Err(err).Msg("Server failed")
		}
	}()

	<-quit
	log.Info().Msg("Shutdown signal received")

	// Stop accepting new requests
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := fiberApp.ShutdownWithContext(ctx); err != nil {
		log.Error().Err(err).Msg("Server shutdown error")
	}

	// Disconnect all bots
	botMgr.ShutdownAll(15 * time.Second)

	log.Info().Msg("Clean shutdown complete")
}
