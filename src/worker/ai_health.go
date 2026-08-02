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

// AIHealthConfig configuración del worker
type AIHealthConfig struct {
	BinPath       string
	ModelPath     string
	ListenAddr    string
	HealthURL     string
	CheckInterval time.Duration
	RestartHour   int
	ExtraArgs     []string
}

// AIHealthStatus estado de la IA (sin mutex, solo datos)
type AIHealthStatus struct {
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

// AIHealthWorker gestiona el monitoreo y reinicio
type AIHealthWorker struct {
	cfg      AIHealthConfig
	logger   logger.Logger
	stopCh   chan struct{}
	status   AIHealthStatus
	statusMu sync.RWMutex
	isOllama bool // true si es Ollama
}

// NewAIHealthWorker crea un nuevo worker
func NewAIHealthWorker(cfg AIHealthConfig, log logger.Logger) *AIHealthWorker {
	// Detectar si es Ollama (BinPath vacío o contiene "ollama")
	isOllama := strings.Contains(cfg.BinPath, "ollama") || cfg.BinPath == ""

	w := &AIHealthWorker{
		cfg:      cfg,
		logger:   log.WithComponent("ai_health_worker"),
		stopCh:   make(chan struct{}),
		isOllama: isOllama,
		status: AIHealthStatus{
			ModelPath: cfg.ModelPath,
			BinPath:   cfg.BinPath,
			StartedAt: time.Now(),
		},
	}
	return w
}

// GetStatus devuelve una copia del estado actual (seguro para concurrencia)
func (w *AIHealthWorker) GetStatus() AIHealthStatus {
	w.statusMu.RLock()
	defer w.statusMu.RUnlock()
	// Devolver una copia sin el mutex (que no existe en la estructura)
	return w.status
}

// Start inicia el worker en goroutine
func (w *AIHealthWorker) Start(ctx context.Context) {
	go w.run(ctx)
}

// Stop detiene el worker
func (w *AIHealthWorker) Stop() {
	close(w.stopCh)
}

// run es el bucle principal
func (w *AIHealthWorker) run(ctx context.Context) {
	w.logger.Info().
		Dur("check_interval", w.cfg.CheckInterval).
		Int("restart_hour", w.cfg.RestartHour).
		Bool("is_ollama", w.isOllama).
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

// checkAndRestart verifica salud y reinicia si falla
func (w *AIHealthWorker) checkAndRestart() {
	healthy := w.isAIHealthy()

	w.statusMu.Lock()
	w.status.IsHealthy = healthy
	w.status.LastCheck = time.Now()
	if !healthy {
		w.status.LastError = "Health check failed"
	} else {
		w.status.LastError = ""
	}
	w.statusMu.Unlock()

	if !healthy {
		w.restartAI("health check fallido")
	}
}

// isAIHealthy comprueba el health del servidor de IA
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
	// Para Ollama, busca "models" en la respuesta; para otros, busca "status":"ok"
	return strings.Contains(string(output), `"models":`) || strings.Contains(string(output), `"status":"ok"`)
}

// restartAI ejecuta el reinicio del proceso de IA
func (w *AIHealthWorker) restartAI(reason string) {
	w.logger.Info().Str("reason", reason).Msg("Reiniciando IA local...")

	if w.isOllama {
		// Modo Ollama: usar systemctl para reiniciar el servicio
		cmd := exec.Command("systemctl", "restart", "ollama")
		if err := cmd.Run(); err != nil {
			w.logger.Error().Err(err).Msg("Fallo al reiniciar el servicio de Ollama")
			w.statusMu.Lock()
			w.status.LastError = "Failed to restart Ollama: " + err.Error()
			w.statusMu.Unlock()
			return
		}
		// Esperar a que el servicio esté listo
		time.Sleep(3 * time.Second)

		w.statusMu.Lock()
		w.status.LastRestart = time.Now()
		w.status.RestartCount++
		w.status.PID = 0
		w.status.StartedAt = time.Now()
		w.status.IsHealthy = true
		w.status.LastError = ""
		w.statusMu.Unlock()

		w.logger.Info().Msg("Servicio de Ollama reiniciado exitosamente")
		return
	}

	// Modo tradicional (llmserver / llama-server)
	killCmd := exec.Command("pkill", "-f", "llmserver")
	if err := killCmd.Run(); err != nil {
		w.logger.Warn().Err(err).Msg("pkill no encontró proceso o falló")
	}

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

	w.statusMu.Lock()
	w.status.LastRestart = time.Now()
	w.status.RestartCount++
	w.status.PID = cmd.Process.Pid
	w.status.StartedAt = time.Now()
	w.status.IsHealthy = true
	w.status.LastError = ""
	w.statusMu.Unlock()

	w.logger.Info().Int("pid", cmd.Process.Pid).Msg("Servidor de IA local reiniciado exitosamente")
}

// getDailyRestartTicker devuelve ticker para reinicio programado
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
