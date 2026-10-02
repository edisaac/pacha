
### 1. Levantar el Solver (Kotlin)
En la primera terminal, navega a la carpeta `kotlin` y levanta el servicio usando el script o Maven:

```bash
cd kotlin

# Opción A: Usando el script provisto
./build-and-run.sh

# Opción B: Usando Maven directamente en modo desarrollo
mvn quarkus:dev
```

### 2. Levantar la API Web (Go)
En la segunda terminal, navega a la carpeta `golang` y ejecuta la API de Go definiendo el entorno `local`:

```bash
cd golang
export APP_ENV=local
go run cmd/api/main.go
```

> **Nota sobre variables de entorno:**
> Por defecto al correr sin Docker, Go buscará la base de datos en la carpeta `../shared-data` relativa al directorio `golang` y se comunicará con Kotlin usando `http://localhost:8082`. 
> Si deseas cambiarlos, puedes usar las variables `SHARED_DATA_DIR` y `KOTLIN_SERVICE_URL`.

### 3. Compilación de Estilos (Tailwind CSS Estático)
El proyecto está configurado para utilizar un paquete CSS estático de Tailwind en lugar de depender del Play CDN en tiempo de ejecución (`cdn.tailwindcss.com`). Esto garantiza compatibilidad, elimina destellos en mutaciones de HTMX y permite que el cliente de escritorio (Wails) funcione sin conexión. El archivo compilado ya se incluye en `golang/static/css/tailwind.css`.

Si modificas o agregas nuevas clases en las plantillas HTML de `golang/internal/presentation/`, puedes re-generar el archivo de estilos ejecutando el siguiente comando desde la carpeta `golang`:

```bash
cd golang
npm --registry=https://registry.npmjs.org exec --yes tailwindcss@3.4.17 -- -c ./tailwind.config.js -i ./static/css/input.css -o ./static/css/tailwind.css --minify
```

> **Nota:** Al empaquetar la aplicación con `wails-desktop/scripts/build-linux.sh`, la compilación de estilos se ejecuta automáticamente antes del build del backend si dispones de `npm` y `npx` en tu sistema.

