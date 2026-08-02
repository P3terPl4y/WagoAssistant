package worker

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"App/src/pkg/logger"
)

// Configuración del worker
type AIHealthConfig struct {
	BinPath       string
	ModelPath     string
	ListenAddr    string
	HealthURL     string
	CheckInterval time.Duration
	RestartHour   int
	ExtraArgs     []string
}

// Estado de la IA (compartido)
type AIHealthStatus struct {
	mu           sync.RWMutex
	IsHealthy    bool          `json:"is_healthy"`
	LastCheck    time.Time     `json:"last_check"`
	LastRestart  time.Time     `json:"last_restart"`
	RestartCount int           `json:"restart_count"`
	Uptime       time.Duration `json:"uptime"`
	LastError    string        `json:"last_error,omitempty"`
	PID          int           `json:"pid"`
	ModelPath    string        `json:"model_path"`
	BinPath      string        `json:"bin_path"`
	StartedAt    time.Time     `json:"started_at"`
}

// AIHealthWorker gestiona el monitoreo y reinicio del servicio local de IA.
type AIHealthWorker struct {
	cfg    AIHealthConfig
	logger logger.Logger
	stopCh chan struct{}
	status *AIHealthStatus
}

// NewAIHealthWorker crea un nuevo worker.
func NewAIHealthWorker(cfg AIHealthConfig, log logger.Logger) *AIHealthWorker {
	return &AIHealthWorker{
		cfg:    cfg,
		logger: log.WithComponent("ai_health_worker"),
		stopCh: make(chan struct{}),
		status: &AIHealthStatus{
			ModelPath: cfg.ModelPath,
			BinPath:   cfg.BinPath,
			StartedAt: time.Now(),
		},
	}
}

// GetStatus devuelve una copia del estado actual (segura para concurrencia).
func (w *AIHealthWorker) GetStatus() AIHealthStatus {
	w.status.mu.RLock()
	defer w.status.mu.RUnlock()
	return *w.status
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

	w.checkAndRestart()

	ticker := time.NewTicker(w.cfg.CheckInterval)
	defer ticker.Stop()

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
			restartTicker.Stop()
			restartTicker = w.getDailyRestartTicker()
			defer restartTicker.Stop()
		}
	}
}

// checkAndRestart verifica la salud y reinicia si falla.
func (w *AIHealthWorker) checkAndRestart() {
	healthy := w.isAIHealthy()

	w.status.mu.Lock()
	w.status.IsHealthy = healthy
	w.status.LastCheck = time.Now()
	if !healthy {
		w.status.LastError = "Health check failed"
	} else {
		w.status.LastError = ""
	}
	w.status.mu.Unlock()

	if !healthy {
		w.restartAI("health check fallido")
	}
}

// isAIHealthy comprueba el endpoint /health de la IA local.
func (w *AIHealthWorker) isAIHealthy() bool {
	if w.cfg.HealthURL == "" {
		return true
	}
	cmd := exec.Command("curl", "-s", "--max-time", "3", w.cfg.HealthURL)
	output, err := cmd.Output()
	if err != nil {
		w.logger.Error().Err(err).Msg("Health check request falló")
		return false
	}
	return strings.Contains(string(output), "ok") || strings.Contains(string(output), `"status":"ok"`)
}

// restartAI ejecuta el reinicio del proceso de IA.
func (w *AIHealthWorker) restartAI(reason string) {
	w.logger.Info().Str("reason", reason).Msg("Reiniciando IA local...")

	// Matar cualquier proceso previo
	killCmd := exec.Command("pkill", "-f", "llmserver")
	if err := killCmd.Run(); err != nil {
		w.logger.Warn().Err(err).Msg("pkill no encontró proceso o falló")
	}

	// Construir comando de inicio
	args := []string{
		"-model", w.cfg.ModelPath,
		"-listen", w.cfg.ListenAddr,
	}
	if len(w.cfg.ExtraArgs) > 0 {
		args = append(args, w.cfg.ExtraArgs...)
	}
	cmd := exec.Command(w.cfg.BinPath, args...)
	cmd.Dir = "/var/www/lucifer/pia/go-pherence"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		w.logger.Error().Err(err).Msg("Fallo al iniciar el servidor de IA local")
		return
	}

	// Actualizar estado con el nuevo PID
	w.status.mu.Lock()
	w.status.LastRestart = time.Now()
	w.status.RestartCount++
	w.status.PID = cmd.Process.Pid
	w.status.StartedAt = time.Now()
	w.status.IsHealthy = true
	w.status.LastError = ""
	w.status.mu.Unlock()

	w.logger.Info().
		Int("pid", cmd.Process.Pid).
		Str("bin", w.cfg.BinPath).
		Msg("Servidor de IA local reiniciado exitosamente")
}

// getDailyRestartTicker devuelve un ticker para el reinicio programado.
func (w *AIHealthWorker) getDailyRestartTicker() *time.Ticker {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), w.cfg.RestartHour, 0, 0, 0, now.Location())
	if now.After(next) {
		next = next.Add(24 * time.Hour)
	}
	wait := next.Sub(now)
	ticker := time.NewTicker(wait)
	w.logger.Info().
		Time("next_restart", next).
		Dur("wait", wait).
		Msg("Programado reinicio preventivo")
	return ticker
}
