package manager

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ProcessManager administra el ciclo de vida de los subprocesos nativos (Go y Kotlin)
type ProcessManager struct {
	mu        sync.Mutex
	cmds      []*exec.Cmd
	ctx       context.Context
	cancel    context.CancelFunc
	logWriter io.Writer
}

// NewProcessManager crea una nueva instancia del administrador de procesos
func NewProcessManager(logWriter io.Writer) *ProcessManager {
	if logWriter == nil {
		logWriter = os.Stdout
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ProcessManager{
		cmds:      make([]*exec.Cmd, 0),
		ctx:       ctx,
		cancel:    cancel,
		logWriter: logWriter,
	}
}

// ResolveBinaryPath busca el binario o JAR en las posibles rutas de instalación/desarrollo
func ResolveBinaryPath(binaryName string) (string, error) {
	var names []string
	if runtime.GOOS == "windows" {
		if strings.HasSuffix(binaryName, ".exe") || strings.HasSuffix(binaryName, ".jar") {
			names = []string{binaryName}
		} else {
			names = []string{binaryName + ".exe", binaryName + ".jar", binaryName}
		}
	} else {
		if strings.HasSuffix(binaryName, ".jar") {
			names = []string{binaryName}
		} else {
			names = []string{binaryName, binaryName + ".jar"}
		}
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("error obteniendo ruta del ejecutable actual: %v", err)
	}
	exeDir := filepath.Dir(exePath)

	var allCandidates []string
	for _, name := range names {
		candidates := []string{
			filepath.Join(exeDir, "bin", name),
			filepath.Join(exeDir, "..", "bin", name),
			filepath.Join(".", "bin", name),
			filepath.Join("..", "bin", name),
			filepath.Join(exeDir, name),
		}
		allCandidates = append(allCandidates, candidates...)

		for _, c := range candidates {
			if abs, err := filepath.Abs(c); err == nil {
				if info, err := os.Stat(abs); err == nil && !info.IsDir() {
					return abs, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no se encontró el ejecutable ni JAR '%s' en las rutas de búsqueda: %v", binaryName, allCandidates)
}

// resolveJavaPath busca el binario de Java empaquetado (JRE privado) o hace fallback al sistema
func resolveJavaPath(baseDir string) string {
	javaExe := "java"
	if runtime.GOOS == "windows" {
		javaExe = "java.exe"
	}
	bundledJava := filepath.Join(baseDir, "jre", "bin", javaExe)
	if info, err := os.Stat(bundledJava); err == nil && !info.IsDir() {
		return bundledJava
	}
	return "java" // Fallback al PATH del sistema operativo
}

// StartProcess inicia un ejecutable nativo o JAR en segundo plano
func (pm *ProcessManager) StartProcess(name string, env []string, args ...string) error {
	binPath, err := ResolveBinaryPath(name)
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	if strings.HasSuffix(strings.ToLower(binPath), ".jar") {
		javaCmd := resolveJavaPath(filepath.Dir(binPath))
		log.Printf("[ProcessManager] ☕ Iniciando proceso con JVM: %s (%s -jar %s)", name, javaCmd, binPath)
		cmdArgs := append([]string{"-jar", binPath}, args...)
		cmd = exec.CommandContext(pm.ctx, javaCmd, cmdArgs...)
	} else {
		log.Printf("[ProcessManager] 🚀 Iniciando proceso nativo: %s (%s)", name, binPath)
		cmd = exec.CommandContext(pm.ctx, binPath, args...)
	}
	dir := filepath.Dir(binPath)
	if filepath.Base(dir) == "bin" {
		dir = filepath.Dir(dir) // Subir de bin/ a la raíz de la app donde están las plantillas y estáticos
	}
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	// Configurar atributos específicos del sistema operativo (p. ej. ocultar ventana de consola en Windows)
	configureSysProcAttr(cmd)

	// Redirigir stdout y stderr al log del escritorio para monitoreo
	cmd.Stdout = pm.logWriter
	cmd.Stderr = pm.logWriter

	// Inject SHARED_DATA_DIR so it finds shared-data in the current bin directory
	sharedDataPath := filepath.Join(dir, "shared-data")
	cmdEnv := append(os.Environ(), env...)
	cmd.Env = append(cmdEnv, "SHARED_DATA_DIR="+sharedDataPath)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error al arrancar %s: %v", name, err)
	}

	pm.mu.Lock()
	pm.cmds = append(pm.cmds, cmd)
	pm.mu.Unlock()

	// Monitorear asincrónicamente la finalización del proceso
	go func() {
		err := cmd.Wait()
		if err != nil && pm.ctx.Err() == nil {
			log.Printf("[ProcessManager] ⚠️ El proceso %s (PID: %d) terminó inesperadamente: %v", name, cmd.Process.Pid, err)
		} else {
			log.Printf("[ProcessManager] El proceso %s (PID: %d) finalizó", name, cmd.Process.Pid)
		}
	}()

	return nil
}

// StopAll detiene todos los procesos administrados de forma segura (SIGTERM o TASKKILL)
func (pm *ProcessManager) StopAll() {
	log.Println("[ProcessManager] 🛑 Deteniendo subprocesos nativos...")
	pm.cancel() // Cancela el contexto de ejecución

	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, cmd := range pm.cmds {
		if cmd.Process == nil {
			continue
		}
		log.Printf("[ProcessManager] Deteniendo proceso PID %d...", cmd.Process.Pid)
		if runtime.GOOS == "windows" {
			// En Windows, utilizar taskkill /F /T para terminar el árbol de procesos limpiamente
			_ = exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", cmd.Process.Pid)).Run()
		} else {
			// En Linux / Unix, enviar señal de interrupción primero
			_ = cmd.Process.Signal(os.Interrupt)
		}
	}

	// Tiempo de espera para cierre elegante antes de que Wails finalice
	time.Sleep(1500 * time.Millisecond)
	log.Println("[ProcessManager] ✅ Subprocesos finalizados limpiamente.")
}

// WaitForURL realiza sondeos periódicos hasta que la URL destino responda (Healthcheck)
func WaitForURL(targetURL string, timeout time.Duration) bool {
	client := http.Client{Timeout: 1 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(targetURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}
