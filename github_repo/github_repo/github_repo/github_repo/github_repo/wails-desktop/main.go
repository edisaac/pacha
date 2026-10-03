package main

import (
	"embed"
	"log"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/*
var assets embed.FS

func main() {
	// En Linux (especialmente con NVIDIA o sesiones sin permisos DRM directos), el renderizador
	// DMA-BUF de WebKit 4.1 falla con 'DRM_IOCTL_MODE_CREATE_DUMB failed' y deja la ventana en blanco.
	// Desactivamos DMA-BUF para forzar el renderizado por memoria compartida (shm/X11), 100% estable.
	if runtime.GOOS == "linux" {
		os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
	}

	// Crear instancia de la aplicación
	app := NewApp()

	// Iniciar la ventana y motor nativo de Wails
	err := wails.Run(&options.App{
		Title:            "Horarios Desktop - Sistema Integrado de Resolución",
		Width:            1280,
		Height:           800,
		MinWidth:         900,
		MinHeight:        600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			Theme:                windows.SystemDefault,
		},
		Linux: &linux.Options{
			WindowIsTranslucent: false,
		},
	})

	if err != nil {
		log.Fatal("Error crítico al ejecutar la aplicación de escritorio Wails:", err)
	}
}
