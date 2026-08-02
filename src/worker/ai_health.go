package worker

import (
	"context"
	
	"os"
	"os/exec"
	"strings"
	"time"

	"App/src/pkg/logger"
)

// Configuración del worker
type AIHealthConfig struct {
	BinPath       string        // Ruta al binario llmserver
	ModelPath     string        // Ruta al modelo
	ListenAddr    string        // Ej: ":8080"
	HealthURL     string        // URL para health check (ej: http://localhost:8080/health)
	CheckInterval time.Duration // Cada cuánto verificar salud
	RestartHour   int           // Hora del día para reinicio preventivo (0-23)
	ExtraArgs     []string      // Argumentos extra (ej: -threads 2 -batch-size 64)
}

// AIHealthWorker gestiona el monitoreo y reinicio del servicio local de IA.
type AIHealthWorker struct {
	cfg    AIHealthConfig
	logger logger.Logger
	stopCh chan struct{}
}

// NewAIHealthWorker crea un nuevo worker.
func NewAIHealthWorker(cfg AIHealthConfig, log logger.Logger) *AIHealthWorker {
	return &AIHealthWorker{
		cfg:    cfg,
		logger: log.WithComponent("ai_health_worker"),
		stopCh: make(chan struct{}),
	}
}

// Start inicia el worker en una goroutine.
func (w *AIHealthWorker) Start(ctx context.Context) {
	go w.run(ctx)
}

// Stop detiene el worker.
func (w *AIHealthWorker) Stop() {
	close(w.stopCh)
}

// run es el bucle principal del worker.
func (w *AIHealthWorker) run(ctx context.Context) {
	w.logger.Info().
		Dur("check_interval", w.cfg.CheckInterval).
		Int("restart_hour", w.cfg.RestartHour).
		Msg("AI Health Worker iniciado")

	// Primera ejecución inmediata para asegurar que la IA está funcionando
	w.checkAndRestart()

	// Temporizador para health checks periódicos
	ticker := time.NewTicker(w.cfg.CheckInterval)
	defer ticker.Stop()

	// Temporizador para reinicio programado (cada 24h)
	restartTicker := w.getDailyRestartTicker()
	defer restartTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info().Msg("Worker detenido por contexto")
			return
		case <-w.stopCh:
			w.logger.Info().Msg("Worker detenido por señal")
			return
		case <-ticker.C:
			w.checkAndRestart()
		case <-restartTicker.C:
			w.restartAI("reinicio programado (24h)")
			// Reiniciar el ticker para el próximo día
			restartTicker.Stop()
			restartTicker = w.getDailyRestartTicker()
			defer restartTicker.Stop()
		}
	}
}

// checkAndRestart verifica la salud y reinicia si falla.
func (w *AIHealthWorker) checkAndRestart() {
	if w.isAIHealthy() {
		w.logger.Debug().Msg("Health check OK")
		return
	}
	w.logger.Warn().Msg("Health check falló, reiniciando IA local...")
	w.restartAI("health check fallido")
}

// isAIHealthy comprueba el endpoint /health de la IA local.
func (w *AIHealthWorker) isAIHealthy() bool {
	// Si no hay URL configurada, asumir que no hay IA local que monitorear
	if w.cfg.HealthURL == "" {
		return true
	}
	// Usar curl para hacer la petición (más fiable en entornos con restricciones de red)
	cmd := exec.Command("curl", "-s", "--max-time", "3", w.cfg.HealthURL)
	output, err := cmd.Output()
	if err != nil {
		w.logger.Error().Err(err).Msg("Health check request falló")
		return false
	}
	// Buscar "status":"ok" o "ok" en la respuesta
	return strings.Contains(string(output), "ok") || strings.Contains(string(output), `"status":"ok"`)
}

// restartAI ejecuta el reinicio del proceso de IA.
func (w *AIHealthWorker) restartAI(reason string) {
	w.logger.Info().Str("reason", reason).Msg("Reiniciando IA local...")

	// 1. Matar cualquier proceso llmserver existente
	killCmd := exec.Command("pkill", "-f", "llmserver")
	if err := killCmd.Run(); err != nil {
		w.logger.Warn().Err(err).Msg("pkill no encontró proceso o falló (puede que no estuviera corriendo)")
	}

	// 2. Construir comando de inicio
	args := []string{
		"-model", w.cfg.ModelPath,
		"-listen", w.cfg.ListenAddr,
	}
	// Agregar argumentos extra
	if len(w.cfg.ExtraArgs) > 0 {
		args = append(args, w.cfg.ExtraArgs...)
	}
	cmd := exec.Command(w.cfg.BinPath, args...)
	cmd.Dir = "/var/www/lucifer/pia/go-pherence" // o el directorio adecuado
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Ejecutar en segundo plano (sin esperar)
	if err := cmd.Start(); err != nil {
		w.logger.Error().Err(err).Msg("Fallo al iniciar el servidor de IA local")
		return
	}
	w.logger.Info().
		Int("pid", cmd.Process.Pid).
		Str("bin", w.cfg.BinPath).
		Msg("Servidor de IA local reiniciado exitosamente")
}

// getDailyRestartTicker devuelve un ticker que dispara a la hora especificada cada día.
func (w *AIHealthWorker) getDailyRestartTicker() *time.Ticker {
	now := time.Now()
	// Calcular la próxima ocurrencia de la hora configurada
	next := time.Date(now.Year(), now.Month(), now.Day(), w.cfg.RestartHour, 0, 0, 0, now.Location())
	if now.After(next) {
		next = next.Add(24 * time.Hour)
	}
	wait := next.Sub(now)
	ticker := time.NewTicker(wait)
	w.logger.Info().
		Time("next_restart", next).
		Dur("wait", wait).
		Msg("Programado reinicio preventivo para las 3 AM")
	return ticker
}
