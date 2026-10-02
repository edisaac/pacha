package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"wails-desktop/pkg/manager"
)

// App estructura principal de la aplicación de escritorio Wails
type App struct {
	ctx            context.Context
	processManager *manager.ProcessManager
}

type safeLogWriter struct {
	file *os.File
}

func (w *safeLogWriter) Write(p []byte) (n int, err error) {
	if w.file != nil {
		_, _ = w.file.Write(p)
		_ = w.file.Sync()
	}
	_, _ = os.Stdout.Write(p)
	return len(p), nil
}

// NewApp crea una nueva instancia de la aplicación
func NewApp() *App {
	var writer io.Writer
	exePath, err := os.Executable()
	if err == nil {
		logFilePath := filepath.Join(filepath.Dir(exePath), "horarios-desktop.log")
		logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			writer = &safeLogWriter{file: logFile}
		}
	}
	if writer == nil {
		writer = &safeLogWriter{file: nil}
	}
	log.SetOutput(writer)
	log.Println("[HorariosDesktop] ========================================")
	log.Println("[HorariosDesktop] 🌟 Iniciando Horarios Desktop...")
	log.Println("[HorariosDesktop] ========================================")
	return &App{
		processManager: manager.NewProcessManager(writer),
	}
}

// startup es invocado en el momento en que Wails inicializa la ventana nativa
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	log.Println("[HorariosDesktop] 🌟 Iniciando ciclo de vida de la aplicación de escritorio...")

	// 1. Configurar variables de entorno y localización de datos compartidos
	exePath, err := os.Executable()
	var sharedDir string
	if err == nil {
		exeDir := filepath.Dir(exePath)
		// Si estamos en la distribución final (build/bin/), shared-data está junto al ejecutable
		candidate := filepath.Join(exeDir, "shared-data")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			sharedDir, _ = filepath.Abs(candidate)
		} else {
			// Si estamos en desarrollo dentro de wails-desktop/ (wails dev)
			candidate = filepath.Join(".", "shared-data")
			if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
				sharedDir, _ = filepath.Abs(candidate)
			}
		}
	}
	if sharedDir != "" {
		os.Setenv("SHARED_DATA_DIR", sharedDir)
		log.Printf("[HorariosDesktop] 📁 Carpeta compartida detectada: %s", sharedDir)
	}
	os.Setenv("APP_ENV", "local")
	os.Setenv("API_PORT", "8080")

	// 2. Verificación de JVM (Java) si el solver está en formato JAR
	solverPath, err := manager.ResolveBinaryPath("solver")
	if err == nil && strings.HasSuffix(strings.ToLower(solverPath), ".jar") {
		javaExe := "java"
		if stdruntime.GOOS == "windows" {
			javaExe = "java.exe"
		}
		bundledJava := filepath.Join(filepath.Dir(solverPath), "jre", "bin", javaExe)
		if info, err := os.Stat(bundledJava); err != nil || info.IsDir() {
			if _, err := exec.LookPath("java"); err != nil {
				log.Println("[HorariosDesktop] ❌ Java (JVM) no se encuentra en el PATH ni empaquetado. No se puede ejecutar el solver en modo JAR.")
				runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
					Type:          runtime.ErrorDialog,
					Title:         "Java (JVM) No Encontrado",
					Message:       "No se encontró el JRE empaquetado y tampoco hay Java instalado en el sistema.\n\nPor favor, instala OpenJDK 17 o 21 (asegúrate de agregar 'java' al PATH) o reconstruye el instalador incluyendo el JRE.",
					Buttons:       []string{"Aceptar"},
				})
			} else {
				log.Println("[HorariosDesktop] ☕ JRE privado no encontrado. Usando Java del PATH del sistema.")
			}
		} else {
			log.Println("[HorariosDesktop] ☕ Usando JRE privado empaquetado (jlink).")
		}
	}

	// 3. Arrastrar y ejecutar el solver (Quarkus Uber-JAR o nativo)
	solverEnv := []string{
		"QUARKUS_HTTP_PORT=8082",
		"QUARKUS_LOG_LEVEL=INFO",
	}
	if sharedDir != "" {
		solverEnv = append(solverEnv, "SHARED_DATA_DIR="+sharedDir)
	}
	if err := a.processManager.StartProcess("solver", solverEnv); err != nil {
		log.Printf("[HorariosDesktop] ⚠️ Aviso: No se pudo iniciar el solver: %v", err)
	}

	// 3. Arrastrar y ejecutar el backend Go nativo (api / api.exe)
	apiEnv := []string{
		"APP_ENV=local",
		"API_PORT=8080",
	}
	if sharedDir != "" {
		apiEnv = append(apiEnv, "SHARED_DATA_DIR="+sharedDir)
	}
	if err := a.processManager.StartProcess("api", apiEnv); err != nil {
		log.Printf("[HorariosDesktop] ⚠️ Aviso: No se pudo iniciar el backend Go nativo: %v", err)
	}

	// 4. Chequeo de salud (Healthcheck): monitorea en segundo plano el puerto 8080
	go func() {
		log.Println("[HorariosDesktop] ⏳ Esperando a que el backend Go responda en http://localhost:8080/horarios...")
		if manager.WaitForURL("http://localhost:8080/horarios", 30*time.Second) {
			log.Println("[HorariosDesktop] ✅ Backend Go en línea. El Webview cambiará a la interfaz en breves segundos.")
		} else {
			log.Println("[HorariosDesktop] ❌ Tiempo de espera agotado conectando con http://localhost:8080/horarios.")
		}
	}()
}

// shutdown se ejecuta antes de que se cierre la aplicación de escritorio Wails
func (a *App) shutdown(ctx context.Context) {
	log.Println("[HorariosDesktop] 🛑 Cerrando aplicación y limpiando subprocesos...")
	if a.processManager != nil {
		a.processManager.StopAll()
	}
}
