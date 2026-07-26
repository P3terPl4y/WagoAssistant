package handlers

import (
	"App/src/app"
	"App/src/pkg/logger"
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
)

// AuthHandler handles authentication-related HTTP endpoints.
type AuthHandler struct {
	userSvc *app.UserService
	logger  logger.Logger
}

func NewAuthHandler(userSvc *app.UserService, log logger.Logger) *AuthHandler {
	return &AuthHandler{userSvc: userSvc, logger: log.WithComponent("auth_handler")}
}

func (h *AuthHandler) Login(c fiber.Ctx) error {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Datos inválidos"})
	}
	if (req.Username == "" && req.Email == "") || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Usuario/email y contraseña requeridos"})
	}

	user, err := h.userSvc.Authenticate(c, req.Username, req.Email, req.Password)
	if err != nil {
		h.logger.Warn().Str("ip", c.IP()).Str("username", req.Username).Msg("Failed login attempt")
		return c.Status(401).JSON(fiber.Map{"error": "Credenciales incorrectas"})
	}

	sess := session.FromContext(c)
	sess.Set("user_id", user.ID)
	sess.Set("role", user.Role)
	return c.JSON(fiber.Map{"status": "ok"})
}

func (h *AuthHandler) Register(c fiber.Ctx) error {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Datos inválidos"})
	}
	if req.Username == "" || req.Email == "" || req.Phone == "" || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Todos los campos son obligatorios"})
	}
	if len(req.Password) < 6 {
		return c.Status(400).JSON(fiber.Map{"error": "La contraseña debe tener al menos 6 caracteres"})
	}

	_, err := h.userSvc.Register(c, req.Username, req.Email, req.Phone, req.Password)
	if err != nil {
		h.logger.Error().Err(err).Msg("Registration failed")
		return c.Status(400).JSON(fiber.Map{"error": "Usuario, email o teléfono ya registrados"})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func (h *AuthHandler) Logout(c fiber.Ctx) error {
	sess := session.FromContext(c)
	if err := sess.Destroy(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al cerrar sesión"})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func (h *AuthHandler) UpdatePassword(c fiber.Ctx) error {
	userID := c.Locals("user_id").(int)
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

func (h *AuthHandler) UpdatePhone(c fiber.Ctx) error {
	userID := c.Locals("user_id").(int)
	var req struct {
		Phone string `json:"phone"`
	}
	if err := json.Unmarshal(c.Body(), &req); err != nil || req.Phone == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Teléfono requerido"})
	}
	if err := h.userSvc.UpdatePhone(c, userID, req.Phone); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

// LogFailedLogin is a utility for logging failed login attempts.
func LogFailedLogin(log logger.Logger, ip, reason string) {
	log.Warn().Str("ip", ip).Str("reason", reason).Str("time", time.Now().Format(time.RFC3339)).Msg("Failed login attempt")
}
