# Horarios Desktop (Wails v2)

Este módulo contiene la aplicación de escritorio nativa e independiente para **Horarios**, construida sobre [Wails v2](https://wails.io/).

La aplicación funciona como un contenedor gráfico (*Embedded Webview*) que orquesta y gestiona el ciclo de vida de los dos motores locales del sistema en segundo plano:
1. **Backend Go (`api` / `api.exe`):** Servidor HTTP y API HTMX (Puerto 8080).
2. **Solver Kotlin (`solver.jar`):** Motor de optimización Timefold compilado como **Uber-JAR en modo JVM con Quarkus**. Requiere tener **Java 17 o superior instalado** en el equipo de destino para alcanzar el máximo rendimiento con el compilador JIT de HotSpot.

---

## 📋 Requisitos Previos (Para Desarrolladores)

Para construir la aplicación desde el código fuente, tu entorno debe contar con:
- **Go 1.23 o superior:** [https://go.dev/dl/](https://go.dev/dl/)
- **Wails CLI v2:**
  ```bash
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
  ```
- **Docker o Podman (Recomendado para Quarkus Native):**
  Los scripts de construcción utilizan la compilación nativa en contenedor (`-Dquarkus.native.container-build=true`), evitando tener que instalar GraalVM en tu máquina local.
- **Dependencias del sistema (Solo en Linux):**
  Librerías de desarrollo de WebKit y GTK para la ventana nativa:
  ```bash
  sudo apt install build-essential libgtk-3-dev libwebkit2gtk-4.1-dev
  ```

---

## 🚀 Cómo Ejecutar la Aplicación

Tienes dos formas de ejecutar la aplicación: ejecutando la distribución final ya compilada o en modo desarrollo en vivo.

### Opción 1: Ejecutar el Binario Final Compilado (Modo Producción)
Una vez que hayas compilado la aplicación (ver sección *Cómo Compilar y Empaquetar*), simplemente lanza el ejecutable generado:

- **En Linux:**
  ```bash
  ./wails-desktop/build/bin/HorariosDesktop
  ```
- **En Windows (PowerShell / CMD / Doble Clic):**
  ```powershell
  .\wails-desktop\build\bin\HorariosDesktop.exe
  ```
  *(Al ejecutarlo, la ventana iniciará los subprocesos en segundo plano automáticamente y cargará el sistema en cuanto estén en línea).*




### Opción 2: Ejecución en Modo Desarrollo (Live Reload / Debug)
Para desarrollar y probar cambios rápidamente en la ventana de Wails:
1. Asegúrate de haber generado al menos una vez los binarios de backend en la carpeta `wails-desktop/bin/` (ejecutando previamente los scripts de build).
2. Inicia Wails en modo desarrollo desde la carpeta `wails-desktop/`:
   ```bash
   cd wails-desktop
   wails dev # (O con flag en Linux si se requiere: wails dev -tags webkit2_41)
   # Alternativamente, sin CLI de Wails: go run -tags desktop,dev,webkit2_41 .
   ```

---

## 📦 Cómo Compilar y Empaquetar para Distribución

El directorio `scripts/` contiene scripts automatizados para compilar todo el stack en binarios nativos autocontenidos y empaquetarlos en un directorio de distribución listo para entregarse a usuarios finales.

### 🐧 1. Empaquetar para Linux (x86_64)

Ejecuta el script de construcción de Linux:
```bash
cd wails-desktop/scripts
./build-linux.sh
```

**Crear archivo comprimido redistribuible (`.tar.gz`):**
Una vez finalizado el build, puedes comprimir la carpeta resultante para enviarla a clientes con Linux (requiere Java 17+ instalado en el equipo de destino):
```bash
cd ../build/bin
tar -czvf HorariosDesktop-Linux-x86_64.tar.gz HorariosDesktop shared-data/ bin/
```

---

### 🪟 2. Empaquetar para Windows (x86_64)

#### Desde PowerShell (Windows):
```powershell
cd wails-desktop\scripts
.\build-windows.ps1
```
#### Desde VIRTUAL MACHINE HOST A GUEST 
```bash

cd /mnt/code/repos/timefold/horarios/wails-desktop/scripts/remote-win-build
chmod +x build_remote.py
python3 build_remote.py
```

#### Desde Git Bash / WSL:
```bash
cd wails-desktop/scripts
./build-windows.sh
```

**Crear archivo comprimido redistribuible (`.zip`):**
Para distribuir la aplicación en computadoras con Windows 10/11 (requiere Java 17+ o JRE en el PATH del usuario):
```powershell
# En PowerShell (desde la carpeta wails-desktop):
Compress-Archive -Path "build\bin\*" -DestinationPath "HorariosDesktop-Windows-x64.zip" -Force
```
*(Opcional: Si tienes las utilidades NSIS configuradas en Wails, puedes ejecutar `wails build -nsis` en Windows para generar un instalador automático `.exe` de instalación).*

---

## 🏗️ Estructura del Paquete Distribuible

La distribución final en `build/bin/` (o al descomprimir el `.tar.gz` / `.zip`) tiene la siguiente estructura:

```text
build/bin/
├── HorariosDesktop (.exe)    <-- Binario principal (Interfaz de escritorio Wails)
├── shared-data/              <-- Plantillas HTML y recursos compartidos
└── bin/
    ├── api (.exe)            <-- Backend Go (Subproceso nativo de la API HTMX)
    └── solver.jar            <-- Solver Kotlin (Subproceso JVM de Quarkus / Timefold)
```

### 🔄 Autonomía y Ciclo de Vida (`ProcessManager`)
- **Verificación Inteligente:** Al iniciar, Wails verifica si `bin/solver` es un JAR (`solver.jar`). Si es así, verifica si `java` se encuentra en las variables de entorno PATH; si no se detecta, muestra una alerta nativa al usuario.
- **Arranque Automático:** Al ejecutar `HorariosDesktop`, este levanta automáticamente el backend Go (`bin/api`) y el solver con JVM (`java -jar bin/solver.jar`) en segundo plano.
- **Healthcheck Integrado:** La interfaz muestra una pantalla de espera elegante mientras monitorea el puerto 8080 y cambia automáticamente a `http://localhost:8080/horarios` apenas el backend está listo.
- **Cierre Limpio:** Al cerrar la ventana, se envían señales `SIGTERM` en Linux o comandos `taskkill /F /T` en Windows para finalizar el árbol de procesos, garantizando que los servidores se apaguen por completo sin dejar procesos fantasma.
