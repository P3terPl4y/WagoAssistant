package handlers

import (
	"App/src/pkg/logger"
	"App/src/ports"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"golang.org/x/oauth2"
)

// GoogleHandler handles Google OAuth2 flow.
type GoogleHandler struct {
	oauthCfg  *oauth2.Config
	userRepo  ports.UserRepository
	oauthRepo ports.OAuthRepository
	logger    logger.Logger
}

func NewGoogleHandler(oauthCfg *oauth2.Config, userRepo ports.UserRepository, oauthRepo ports.OAuthRepository, log logger.Logger) *GoogleHandler {
	return &GoogleHandler{oauthCfg: oauthCfg, userRepo: userRepo, oauthRepo: oauthRepo, logger: log.WithComponent("google_oauth")}
}

func (h *GoogleHandler) Login(c fiber.Ctx) error {
	stateBytes := make([]byte, 16)
	rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)
	c.Cookie(&fiber.Cookie{Name: "oauth_state", Value: state, HTTPOnly: true, Secure: false, SameSite: "Lax", MaxAge: 600})
	url := h.oauthCfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	return c.Redirect().To(url)
}

func (h *GoogleHandler) Callback(c fiber.Ctx) error {
	state := c.Query("state")
	cookieState := c.Cookies("oauth_state")
	if state == "" || state != cookieState {
		return c.Status(400).JSON(fiber.Map{"error": "Estado inválido"})
	}
	code := c.Query("code")
	token, err := h.oauthCfg.Exchange(c, code)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al obtener token"})
	}
	client := h.oauthCfg.Client(c, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al obtener datos"})
	}
	defer resp.Body.Close()

	var userInfo struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al decodificar"})
	}

	// Try to find existing user by email
	user, err := h.userRepo.GetByEmail(c, userInfo.Email)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error al buscar usuario"})
	}

	var userID int
	var role string

	if user != nil {
		userID = user.ID
		role = user.Role
	} else {
		// Create new user via repository
		dummyHash := "$2a$10$dummyhash"
		newUser, err := h.userRepo.Create(c, userInfo.Email, userInfo.Email, "", dummyHash)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Error al crear usuario"})
		}
		userID = newUser.ID
		role = newUser.Role
	}

	_ = h.oauthRepo.SaveRefreshToken(c, userID, "google", token.RefreshToken)

	sess := session.FromContext(c)
	sess.Set("user_id", userID)
	sess.Set("role", role)
	return c.Redirect().To("/dashboard")
}
