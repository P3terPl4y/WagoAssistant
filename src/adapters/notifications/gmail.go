package notifications

import (
	"App/src/pkg/logger"
	"App/src/ports"
	"context"
	"fmt"
	"net/smtp"
	"os"
)

// GmailNotifier se encarga de enviar correos usando SMTP
type GmailNotifier struct {
	from     string
	password string
	smtpHost string
	smtpPort string
	userRepo ports.UserRepository
	log      logger.Logger
}

// NewGmailNotifier crea un nuevo notificador con variables de entorno
func NewGmailNotifier(userRepo ports.UserRepository, log logger.Logger) *GmailNotifier {
	return &GmailNotifier{
		from:     os.Getenv("GMAIL_FROM"),     // tu-email@gmail.com
		password: os.Getenv("GMAIL_PASSWORD"), // Contraseña de aplicación
		smtpHost: "smtp.gmail.com",
		smtpPort: "587",
		userRepo: userRepo,
		log:      log,
	}
}

// SendNotification envía un correo electrónico al destinatario
func (g *GmailNotifier) SendNotification(toBotID int, subject, body string) error {

	// Autenticación para el servidor SMTP de Gmail
	auth := smtp.PlainAuth("", g.from, g.password, g.smtpHost)
	user, err := g.userRepo.GetUserByBotID(context.Background(), toBotID)
	if err != nil {
		g.log.Error().Msg(err.Error())
		return fmt.Errorf("error al obtener el correo del destinatario: %w", err)
	}
	// Construir el mensaje
	msg := []byte(fmt.Sprintf("To: %s\r\n"+
		"Subject: %s\r\n"+
		"\r\n"+
		"%s\r\n", user.Email, subject, body))

	// Enviar el correo
	addr := fmt.Sprintf("%s:%s", g.smtpHost, g.smtpPort)
	err = smtp.SendMail(addr, auth, g.from, []string{user.Email}, msg)
	if err != nil {
		g.log.Error().Msg(err.Error())
		return fmt.Errorf("error al enviar correo: %w", err)
	}
	return nil
}
func (g *GmailNotifier) SendAdminNotification(subject, body string) error {

	// Autenticación para el servidor SMTP de Gmail
	auth := smtp.PlainAuth("", g.from, g.password, g.smtpHost)
	to := "elvinfelipetorres@gmail.com"
	// Construir el mensaje
	msg := []byte(fmt.Sprintf("To: %s\r\n"+
		"Subject: %s\r\n"+
		"\r\n"+
		"%s\r\n", to, subject, body))

	// Enviar el correo
	addr := fmt.Sprintf("%s:%s", g.smtpHost, g.smtpPort)
	err := smtp.SendMail(addr, auth, g.from, []string{to}, msg)
	if err != nil {
		g.log.Error().Msg(fmt.Sprintf("Error :%s", err.Error()))
		return fmt.Errorf("error al enviar correo: %w", err)
	}
	return nil
}
