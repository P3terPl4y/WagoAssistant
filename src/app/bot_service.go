package app

import (
	"App/src/adapters/notifications"
	"App/src/config"
	"App/src/domain"
	"App/src/pkg/concurrency"
	"App/src/pkg/logger"
	"App/src/ports"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// BotService handles bot lifecycle, messaging, and WhatsApp integration.
type BotService struct {
	bots        ports.BotRepository
	prompts     ports.PromptRepository
	subs        ports.SubscriptionRepository
	users       ports.UserRepository
	chat        *ChatService
	ai          ports.AIService
	botMgr      *concurrency.BotManager
	promptCache *concurrency.PromptCache
	dedup       *concurrency.MessageDedup
	userSem     *concurrency.UserSemaphore
	cache       ports.CacheService
	logger      logger.Logger
	cfg         *config.Config
	gNotifier   *notifications.GmailNotifier

	// WhatsApp session containers
	containersMu sync.Mutex
	containers   map[int]*sqlstore.Container

	// Blocked senders (per-JID bot pause)
	blockedMu sync.RWMutex
	adminMu   sync.RWMutex
	blocked   map[types.JID]bool
	// Admin bot
	AdminClient *whatsmeow.Client
	AdminJID    types.JID
	AdminBotID  int
}

// NewBotService creates a new BotService with all dependencies.
func NewBotService(
	bots ports.BotRepository,
	prompts ports.PromptRepository,
	subs ports.SubscriptionRepository,
	users ports.UserRepository,
	chat *ChatService,
	ai ports.AIService,
	botMgr *concurrency.BotManager,
	promptCache *concurrency.PromptCache,
	dedup *concurrency.MessageDedup,
	userSem *concurrency.UserSemaphore,
	cache ports.CacheService,
	log logger.Logger,
	cfg *config.Config,
	gNotifier *notifications.GmailNotifier,
) *BotService {
	return &BotService{
		bots: bots, prompts: prompts, subs: subs, users: users,
		chat: chat, ai: ai, botMgr: botMgr, promptCache: promptCache,
		dedup: dedup, userSem: userSem, cache: cache, logger: log.WithComponent("bot_service"),
		cfg: cfg, gNotifier: gNotifier, containers: make(map[int]*sqlstore.Container),
		blocked: make(map[types.JID]bool),
	}
}

// GetContainer returns or creates a WhatsApp session container for a bot.
func (s *BotService) GetContainer(botID int) *sqlstore.Container {
	s.containersMu.Lock()
	defer s.containersMu.Unlock()
	if c, ok := s.containers[botID]; ok {
		return c
	}

	// Crear directorio si no existe
	dir := "./src/db"
	if err := os.MkdirAll(dir, 0755); err != nil {
		s.logger.Fatal().Err(err).Str("dir", dir).Msg("Failed to create session db directory")
	}

	ctx := context.Background()
	dbLog := waLog.Stdout("Database", "WARN", true)

	// DSN correcta: _fk=true habilita foreign keys, _busy_timeout=10000, _journal_mode=WAL
	dsn := fmt.Sprintf("file:%s?_fk=true&_busy_timeout=10000&_journal_mode=WAL",
		fmt.Sprintf("%s/whatsapp_bot%d.db", dir, botID))

	container, err := sqlstore.New(ctx, "sqlite3", dsn, dbLog)
	if err != nil {
		s.logger.Fatal().Err(err).Int("bot_id", botID).Msg("Failed to init session container")
	}
	s.containers[botID] = container
	s.logger.Info().Int("bot_id", botID).Msg("Session DB initialized")
	return container
}

// ConnectWithRetry connects a client with exponential backoff.
func (s *BotService) ConnectWithRetry(client *whatsmeow.Client) error {
	var lastErr error
	for attempt := 1; attempt <= s.cfg.MaxConnectRetries; attempt++ {
		if err := client.Connect(); err != nil {
			lastErr = err
			s.logger.Warn().Int("attempt", attempt).Err(err).Msg("Connection attempt failed")
			if attempt < s.cfg.MaxConnectRetries {
				time.Sleep(time.Duration(attempt*2) * time.Second)
			}
		} else {
			return nil
		}
	}
	return fmt.Errorf("failed after %d attempts: %w", s.cfg.MaxConnectRetries, lastErr)
}

// InitBot starts a bot instance (QR flow or session restore).
func (s *BotService) InitBot(botID int, qrResult chan<- string) {
	log := s.logger.WithBotID(botID)
	ctx, cancel := context.WithCancel(context.Background())

	sendQR := func(val string) {
		defer func() { recover() }()
		if qrResult != nil {
			qrResult <- val
		}
	}
	defer func() {
		cancel()
		s.botMgr.Unregister(botID)
		if qrResult != nil {
			close(qrResult)
		}
		log.Info().Msg("Bot finalized")
	}()

	bot, err := s.bots.GetByID(ctx, botID)
	if err != nil || bot == nil {
		log.Error().Err(err).Msg("Bot not found")
		return
	}
	if bot.Blocked {
		log.Warn().Msg("Bot is blocked, not starting")
		return
	}
	if bot.PaymentStatus != "free" && bot.PaymentStatus != "paid" {
		log.Warn().Str("status", bot.PaymentStatus).Msg("Payment not confirmed")
		return
	}

	prompt, _ := s.prompts.Get(ctx, botID)
	if prompt == "" {
		prompt = "Eres un asistente útil."
	}

	sub, err := s.subs.Get(ctx, botID)
	if err != nil || sub == nil {
		sub = &domain.Subscription{
			BotID:     botID,
			Tier:      "free",
			MsgLimit:  10,
			ExpiresAt: time.Now().Add(s.cfg.SubscriptionDuration),
		}
		if saveErr := s.subs.Save(ctx, sub); saveErr != nil {
			log.Error().Err(saveErr).Msg("Failed to save subscription")
			return
		}
	}
	log.Info().Time("expires_at", sub.ExpiresAt).Str("tier", sub.Tier).Msg("Subscription info")

	container := s.GetContainer(botID)
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get device")
		return
	}

	clientLog := waLog.Stdout("Client", "WARN", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	s.botMgr.Register(botID, client, cancel)

	// Event handler
	client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			s.handleMessage(client, botID, v, ctx)
		case *events.Disconnected:
			log.Warn().Msg("Disconnected, attempting reconnect")
			go func() {
				time.Sleep(3 * time.Second)
				if !s.botMgr.IsActive(botID) {
					return
				}
				if err := s.ConnectWithRetry(client); err != nil {
					log.Error().Err(err).Msg("Reconnect failed")
					cancel()
				} else {
					log.Info().Msg("Reconnected successfully")
				}
			}()
		case *events.StreamReplaced:
			log.Warn().Msg("Session replaced by another device")
			cancel()
		}
	})

	// Session restore or QR
	if client.Store.ID != nil {
		log.Info().Msg("Session restored")
		if err := s.ConnectWithRetry(client); err != nil {
			log.Error().Err(err).Msg("Connection failed")
			return
		}
		sendQR("SESSION_EXISTS")
		s.runLifecycle(botID, client, ctx, cancel)
		return
	}

	log.Info().Msg("Generating QR")
	qrChan, err := client.GetQRChannel(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get QR channel")
		return
	}

	go func() {
		defer func() { recover() }()
		for evt := range qrChan {
			select {
			case <-ctx.Done():
				return
			default:
				if evt.Event == "code" {
					sendQR(evt.Code)
					log.Info().Msg("QR generated")
				} else if evt.Event == "timeout" {
					log.Warn().Msg("QR timed out")
					sendQR("TIMEOUT")
					cancel()
					return
				}
			}
		}
	}()

	if err := s.ConnectWithRetry(client); err != nil {
		log.Error().Err(err).Msg("Connection failed")
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			client.Disconnect()
			return
		case <-ticker.C:
			if client.Store.ID != nil {
				log.Info().Msg("Authentication successful, bot active")
				s.runLifecycle(botID, client, ctx, cancel)
				return
			}
		}
	}
}

// handleMessage processes incoming WhatsApp messages.
func (s *BotService) handleMessage(client *whatsmeow.Client, botID int, v *events.Message, ctx context.Context) {
	if v.Info.IsFromMe || v.Message.GetProtocolMessage() != nil || v.Info.IsGroup {
		return
	}
	if s.dedup.IsDuplicate(v.Info.ID) {
		return
	}
	text := v.Message.GetConversation()
	if text == "" {
		if ext := v.Message.GetExtendedTextMessage(); ext != nil {
			text = ext.GetText()
		}
	}
	if text == "" {
		return
	}
	if len(text) > s.cfg.MaxMsgLength {
		text = text[:s.cfg.MaxMsgLength] + "..."
	}

	senderJID := v.Info.Sender.ToNonAD()
	userKey := fmt.Sprintf("%d:%s", botID, senderJID.String())
	s.logger.Info().Int("bot_id", botID).Str("sender", senderJID.String()).Str("text", text).Msg("Message received")
	go s.switchHandler(client, userKey, botID, senderJID, text)
}

// switchHandler routes messages based on bot block state.
func (s *BotService) switchHandler(client *whatsmeow.Client, userKey string, botID int, recipient types.JID, txt string) {
	s.blockedMu.Lock()
	defer s.blockedMu.Unlock()

	blocked := s.blocked[recipient]
	if blocked {
		switch {
		case txt == "-start" || txt == "Hola bot" || txt == "Activate":
			delete(s.blocked, recipient)
			res := "🤖 Hola de nuevo"
			_, err := client.SendMessage(context.Background(), recipient, &waE2E.Message{Conversation: &res})
			if err != nil {
				s.logger.Error().Err(err).Str("recipient", recipient.String()).Msg("Failed to send message")
			} else {
				s.logger.Info().Str("recipient", recipient.String()).Msg("Response sent")
			}
			s.logger.Info().Int("bot_id", botID).Str("recipient", recipient.String()).Msg("Bot resumed")
		case strings.Contains(txt, "@Bot"):
			go s.respond(client, userKey, botID, recipient, txt)
		}
		return
	}

	switch {
	case txt == "-stop" || txt == "Adios bot" || txt == "Desactivate":
		s.blocked[recipient] = true
		res := "🤖 Hasta luego"
		_, err := client.SendMessage(context.Background(), recipient, &waE2E.Message{Conversation: &res})
		if err != nil {
			s.logger.Error().Err(err).Str("recipient", recipient.String()).Msg("Failed to send message")
		} else {
			s.logger.Info().Str("recipient", recipient.String()).Msg("Response sent")
		}
		s.logger.Info().Int("bot_id", botID).Str("recipient", recipient.String()).Msg("Bot paused")
	case strings.Contains(txt, "Pedido:") || strings.Contains(txt, "Agendar Cita:"):

		go func() {

			ctcName, ctcPn := s.getContact(client, recipient)
			fmt.Println(ctcName)
			fmt.Println(ctcPn)
			err := s.RegisterHistoryal(botID, recipient, txt, ctcName, ctcPn)
			if err != nil {
				s.logger.Error().Msg(err.Error())
			}
			s.notifyAdmin(botID, recipient, txt)
			if err != nil {
				s.logger.Error().Msg(err.Error())
			}
			err = s.gNotifier.SendNotification(botID, fmt.Sprintf("Un cliente quiere hablar contigo (%s:%s)", ctcName, ctcPn), txt)
			if err != nil {
				s.logger.Error().Msg(err.Error())
				return
			}
		}()
	default:

		go s.respond(client, userKey, botID, recipient, txt)
	}
}

// respond generates and sends an AI response.
func (s *BotService) respond(client *whatsmeow.Client, userKey string, botID int, recipient types.JID, txt string) {
	defer s.userSem.Unlock(userKey)
	log := s.logger.WithBotID(botID)

	if !s.botMgr.IsActive(botID) {
		return
	}

	ctx := context.Background()

	// Enforce daily rate limit based on tier
	if s.cache != nil && s.cache.Available() {
		sub, err := s.subs.Get(ctx, botID)
		if err == nil && sub != nil && sub.MsgLimit != -1 {
			usage, err := s.cache.IncrementUsage(ctx, botID)
			if err == nil && usage > sub.MsgLimit && botID != s.AdminBotID {
				log.Warn().Int("usage", usage).Int("limit", sub.MsgLimit).Msg("Rate limit exceeded")
				limitMsg := "🤖 Has superado el límite diario de mensajes para tu plan de suscripción."
				_, _ = client.SendMessage(ctx, recipient, &waE2E.Message{Conversation: &limitMsg})
				return
			}
		}
	}

	if err := s.chat.SaveMessage(ctx, botID, recipient.String(), "user", txt); err != nil {
		log.Error().Err(err).Msg("Failed to save user message")
	}

	history, err := s.chat.GetHistory(ctx, botID, recipient.String(), int(s.cfg.MaxHistory))
	if err != nil {
		history = []domain.ChatMessage{}
	}
	history = s.chat.TruncateHistory(history)

	contexto, ok := s.promptCache.Get(botID)
	if !ok {
		contexto, _ = s.prompts.Get(ctx, botID)
		s.promptCache.Set(botID, contexto)
	}
	if contexto == "" {
		contexto = "Eres un asistente útil de WhatsApp. Responde de forma concisa."
	}

	var pb strings.Builder
	pb.WriteString(contexto + "\n\n")
	for _, m := range history {
		switch m.Role {
		case "user":
			pb.WriteString("U: " + m.Content + "\n")
		case "assistant":
			pb.WriteString("A: " + m.Content + "\n")
		}
	}
	pb.WriteString("U: " + txt + "\nA:")

	type aiResult struct {
		resp string
		err  error
	}
	aiCh := make(chan aiResult, 1)
	go func() {
		r, e := s.ai.Call(ctx, pb.String())
		aiCh <- aiResult{r, e}
	}()

	var respuestaIA string
	select {
	case res := <-aiCh:
		if res.err != nil {
			log.Error().Err(res.err).Msg("AI error")
			respuestaIA = "🤖 Lo siento, no pude procesar tu mensaje. Inténtalo de nuevo en un momento."
			if s.cache != nil {
				s.cache.RecordGlobalMetric(ctx, "errors")
			}
		} else {
			respuestaIA = res.resp
			if s.cache != nil {
				s.cache.RecordGlobalMetric(ctx, "messages")
			}
		}
	case <-time.After(s.cfg.AITimeoutTotal):
		log.Warn().Str("recipient", recipient.String()).Msg("AI timeout")
		respuestaIA = "🤖 Estoy tardando más de lo esperado. Inténtalo de nuevo."
		if s.cache != nil {
			s.cache.RecordGlobalMetric(ctx, "timeouts")
		}
	}

	if err := s.chat.SaveMessage(ctx, botID, recipient.String(), "assistant", respuestaIA); err != nil {
		log.Error().Err(err).Msg("Failed to save AI response")
	}

	_, err = client.SendMessage(context.Background(), recipient, &waE2E.Message{Conversation: &respuestaIA})
	if err != nil {
		log.Error().Err(err).Str("recipient", recipient.String()).Msg("Failed to send message")
	} else {
		log.Info().Str("recipient", recipient.String()).Msg("Response sent")
	}
}

// notifyAdmin envía una notificación al dueño del bot usando el bot administrador.
// Si el admin bot no está conectado, envía un correo electrónico como fallback.
// Logs detallados para facilitar la depuración.
// notifyAdmin envía una notificación al dueño del bot usando el bot administrador.
// Si el admin bot no está conectado, envía un correo electrónico como fallback.
// Logs detallados para facilitar la depuración.
func (s *BotService) notifyAdmin(botID int, clientJID types.JID, msg string) {
	// Log de inicio
	s.logger.Info().
		Int("bot_id", botID).
		Str("client_jid", clientJID.String()).
		Str("message", msg).
		Msg("📨 Iniciando notificación al administrador")

	// 1. Obtener el cliente administrador con mutex
	s.adminMu.RLock()
	client := s.AdminClient
	s.adminMu.RUnlock()

	if client == nil {
		s.logger.Warn().
			Int("bot_id", botID).
			Msg("⚠️ AdminClient es nil (no existe)")
	} else {
		s.logger.Info().
			Int("bot_id", botID).
			Bool("is_connected", client.IsConnected()).
			Str("admin_jid", s.AdminJID.String()).
			Msg("🔍 AdminClient obtenido")
	}

	// 2. Verificar si el admin bot existe y está conectado
	if client != nil && client.IsConnected() {
		s.logger.Info().
			Int("bot_id", botID).
			Msg("✅ Admin bot está CONECTADO. Intentando enviar por WhatsApp...")

		// Obtener el usuario dueño del bot (destinatario de la notificación)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		user, err := s.users.GetUserByBotID(ctx, botID)
		if err != nil || user == nil {
			s.logger.Error().
				Err(err).
				Int("bot_id", botID).
				Msg("❌ No se pudo obtener el usuario del bot. Enviando correo de fallback.")
			goto sendEmailFallback
		}

		phone := strings.TrimPrefix(user.Phone, "+")
		if phone == "" {
			s.logger.Warn().
				Int("user_id", user.ID).
				Str("username", user.Username).
				Msg("⚠️ El usuario no tiene número de teléfono. Enviando correo de fallback.")
			goto sendEmailFallback
		}

		userJID, err := types.ParseJID(phone + "@s.whatsapp.net")
		if err != nil {
			s.logger.Error().
				Err(err).
				Str("phone", phone).
				Msg("❌ JID inválido. Enviando correo de fallback.")
			goto sendEmailFallback
		}

		// Construir mensaje
		notif := fmt.Sprintf("📦 Nuevo pedido/cita de %s:\n%s", clientJID, msg)
		s.logger.Debug().
			Str("whatsapp_message", notif).
			Msg("📝 Mensaje preparado para WhatsApp")

		// Enviar por WhatsApp con timeout
		sendCtx, sendCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer sendCancel()

		s.logger.Info().
			Str("to", userJID.String()).
			Str("phone", user.Phone).
			Msg("📤 Enviando mensaje por WhatsApp...")

		_, err = client.SendMessage(sendCtx, userJID, &waE2E.Message{Conversation: &notif})
		if err == nil {
			s.logger.Info().
				Str("phone", user.Phone).
				Int("bot_id", botID).
				Msg("✅ Notificación enviada EXITOSAMENTE por WhatsApp")
			return
		}

		s.logger.Error().
			Err(err).
			Str("phone", user.Phone).
			Msg("❌ Falló el envío por WhatsApp. Intentando correo de fallback...")
	} else {
		s.logger.Warn().
			Int("bot_id", botID).
			Msg("⚠️ Admin bot NO ESTÁ CONECTADO. Usando correo electrónico como fallback.")
	}

sendEmailFallback:
	// 3. Fallback a correo electrónico (siempre se intenta)
	if s.gNotifier == nil {
		s.logger.Error().
			Int("bot_id", botID).
			Msg("❌ No hay notificador de correo configurado (gNotifier es nil).")
		return
	}

	s.logger.Info().
		Int("bot_id", botID).
		Msg("📧 Enviando notificación por correo electrónico...")

	// Construir cuerpo del correo
	emailSubject := "📬 Nuevo pedido/cita - FALLBACK"
	emailBody := fmt.Sprintf(
		"🔔 Se ha recibido un nuevo pedido o solicitud de cita.\n\n"+
			"📌 Cliente: %s\n"+
			"📝 Mensaje: %s\n"+
			"🤖 Bot ID: %d\n"+
			"📅 Fecha: %s\n\n"+
			"Este es un mensaje automático enviado porque el bot administrador no estaba disponible.",
		clientJID.String(),
		msg,
		botID,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	err := s.gNotifier.SendAdminNotification(emailSubject, emailBody)
	if err != nil {
		s.logger.Error().
			Err(err).
			Int("bot_id", botID).
			Msg("❌ Error al enviar correo electrónico de fallback.")
		return
	}

	s.logger.Info().
		Int("bot_id", botID).
		Str("admin_email", os.Getenv("ADMIN_EMAIL")).
		Msg("✅ Notificación enviada EXITOSAMENTE por correo electrónico (fallback)")
}

// runLifecycle monitors subscription and blocked status, disconnecting when needed.
func (s *BotService) runLifecycle(botID int, client *whatsmeow.Client, ctx context.Context, cancel context.CancelFunc) {
	log := s.logger.WithBotID(botID)
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Lifecycle ending")
			client.Disconnect()
			return
		case <-ticker.C:
			sub, err := s.subs.Get(ctx, botID)
			if err != nil || sub == nil {
				continue
			}
			if !sub.ExpiresAt.IsZero() && time.Now().After(sub.ExpiresAt) {
				log.Warn().Msg("Subscription expired")
				cancel()
				client.Disconnect()
				return
			}
			bot, err := s.bots.GetByID(ctx, botID)
			if err != nil || bot == nil || bot.Blocked {
				log.Warn().Msg("Bot blocked or deleted")
				cancel()
				client.Disconnect()
				return
			}
		}
	}
}

// StartAdminBot starts the admin bot in the background.
func (s *BotService) StartAdminBot() {
	adminUser, err := s.users.GetByUsername(context.Background(), s.cfg.AdminUsername)
	if err != nil || adminUser == nil {
		s.logger.Warn().Msg("Admin user not found")
		return
	}
	bots, err := s.bots.GetByUser(context.Background(), adminUser.ID)
	if err != nil || len(bots) == 0 {
		s.logger.Warn().Msg("Admin has no bots")
		return
	}
	adminBot := bots[0]
	s.adminMu.Lock()
	s.AdminBotID = adminBot.ID
	s.adminMu.Unlock()
	go func() {
		backoff := 5 * time.Second
		const maxBackoff = 2 * time.Minute
		for {
			ctx := context.Background()
			container := s.GetContainer(adminBot.ID)
			deviceStore, err := container.GetFirstDevice(ctx)
			if err != nil || deviceStore == nil {
				s.logger.Error().Err(err).Msg("Admin bot: device error")
				time.Sleep(backoff)
				if backoff < maxBackoff {
					backoff *= 2
				}
				continue
			}
			clientLog := waLog.Stdout("AdminClient", "WARN", true)
			client := whatsmeow.NewClient(deviceStore, clientLog)
			if err := client.Connect(); err != nil {
				s.logger.Error().Err(err).Msg("Admin bot: connect error")
				time.Sleep(backoff)
				if backoff < maxBackoff {
					backoff *= 2
				}
				continue
			}
			if client.Store.ID == nil {
				s.logger.Warn().Msg("Admin bot: invalid session")
				time.Sleep(60 * time.Second)
				continue
			}
			s.adminMu.Lock()
			s.AdminClient = client
			s.AdminJID = *client.Store.ID
			s.adminMu.Unlock()

			s.logger.Info().Str("jid", s.AdminJID.String()).Msg("Admin bot active")

			disconnected := make(chan bool)
			client.AddEventHandler(func(evt interface{}) {
				if _, ok := evt.(*events.Disconnected); ok {
					s.logger.Warn().Msg("Admin bot disconnected")
					close(disconnected)
				}
			})
			<-disconnected
			time.Sleep(2 * time.Second)
		}
	}()
}

// getPhoneNumber intenta obtener el número de teléfono a partir de un JID.
// Si el JID es un LID (@lid), lo resuelve usando el mapeo local.
// Si es un número de teléfono (@s.whatsapp.net), devuelve el User directamente.

// Dentro de una función que tenga acceso al cliente *whatsmeow.Client
func (s *BotService) getContact(client *whatsmeow.Client, jid types.JID) (string, string) {
	contact, err := client.Store.Contacts.GetContact(context.Background(), jid)
	if err != nil {
		return contact.PushName, "" // o devolver el número si no se encuentra
	}
	if jid.Server == types.DefaultUserServer { // "s.whatsapp.net"
		return contact.PushName, jid.User
	}

	// Si es un LID (@lid), intentar resolverlo a un número de teléfono
	if jid.Server == types.HiddenUserServer { // "lid"
		pnJID, err := client.Store.LIDs.GetPNForLID(context.Background(), jid)
		if err == nil {
			// pnJID.User contiene el número de teléfono
			return contact.PushName, pnJID.User
		}
		// Si no se puede resolver, se puede devolver el User del LID como fallback
		return contact.PushName, jid.User
	}

	// Para otros casos (grupos, etc.), devolver el User
	return contact.PushName, jid.User
}
func (s *BotService) RegisterHistoryal(botID int, recipient types.JID, txt string, userName string, phone string) error {
	u, err := s.users.GetUserByBotID(context.Background(), botID)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error al obtener usuario dueño del bot")
		return err
	}

	// Construir el campo "client": número (nombre) o solo número si nombre vacío
	clientStr := recipient.User
	if userName != "" {
		clientStr = fmt.Sprintf("%s (%s)", userName, phone)
	}

	p, err := s.users.CreateHistoryPedidos(context.Background(), u.ID, clientStr, txt)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error al guardar pedido")
		return err
	}

	s.logger.Info().Msg(fmt.Sprintf("Pedido guardado para %d: %s", p.ID, clientStr))
	return nil
}

// SetAdminClientByBotID asigna el cliente del bot dado como AdminClient si el bot pertenece a un admin.
func (s *BotService) SetAdminClientByBotID(botID int) error {
	ctx := context.Background()

	// Verificar que el bot existe
	bot, err := s.bots.GetByID(ctx, botID)
	if err != nil || bot == nil {
		return fmt.Errorf("bot not found")
	}

	// Verificar que el usuario es admin
	user, err := s.users.GetByID(ctx, bot.UserID)
	if err != nil || user == nil {
		return fmt.Errorf("user not found")
	}
	if user.Role != "admin" {
		return fmt.Errorf("user is not admin")
	}

	// Obtener el cliente activo del bot desde BotManager
	client := s.botMgr.GetClient(botID)
	if client == nil {
		return fmt.Errorf("client not active for bot %d", botID)
	}

	// Asignar como AdminClient con mutex
	s.adminMu.Lock()
	s.AdminClient = client
	s.AdminJID = *client.Store.ID
	s.AdminBotID = botID
	s.adminMu.Unlock()

	s.logger.Info().
		Int("bot_id", botID).
		Str("jid", s.AdminJID.String()).
		Msg("✅ Admin client set successfully")

	return nil
}
