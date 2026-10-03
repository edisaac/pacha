#!/bin/bash

# Script de compilación y empaquetado para Linux (Wails Desktop + Go API + Kotlin Quarkus Native)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WAILS_DIR="$(dirname "$SCRIPT_DIR")"
REPO_ROOT="$(dirname "$WAILS_DIR")"

echo "🐧 --- Iniciando Compilación de Escritorio para Linux (x86_64) ---"

# 1. Preparar estructura de directorios bin
mkdir -p "$WAILS_DIR/bin"
mkdir -p "$WAILS_DIR/build/bin"

# 2. Compilar Go API nativo y estilos CSS para Linux
echo "📦 [1/3] Compilando estilos CSS y Backend Go nativo para Linux..."
cd "$REPO_ROOT/golang"
if command -v npm &> /dev/null && command -v npx &> /dev/null; then
    echo "🎨 Generando CSS estático de Tailwind..."
    npm --registry=https://registry.npmjs.org exec --yes tailwindcss@3.4.17 -- -c ./tailwind.config.js -i ./static/css/input.css -o ./static/css/tailwind.css --minify 2>/dev/null || true
fi
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$WAILS_DIR/bin/api" ./cmd/api/main.go
chmod +x "$WAILS_DIR/bin/api"
echo "✅ Backend Go compilado con éxito en: $WAILS_DIR/bin/api"

# 3. Compilar Kotlin Quarkus en modo JVM (Uber-JAR)
echo "📦 [2/3] Compilando Solver Kotlin con Quarkus en modo JVM (Uber-JAR)..."
cd "$REPO_ROOT/kotlin"
MVN_CMD="mvn"
if [ -f "./mvnw" ]; then
    MVN_CMD="./mvnw"
fi

$MVN_CMD package -Dquarkus.package.jar.type=uber-jar -DskipTests

# Copiar el JAR generado (target/*-runner.jar) al directorio bin del empaquetado
RUNNER_JAR=$(find target/ -maxdepth 1 -name "*-runner.jar" | head -n 1)
if [ -f "$RUNNER_JAR" ]; then
    rm -f "$WAILS_DIR/bin/solver"* 2>/dev/null || true
    cp "$RUNNER_JAR" "$WAILS_DIR/bin/solver.jar"
    chmod +x "$WAILS_DIR/bin/solver.jar" 2>/dev/null || true
    echo "✅ Solver Kotlin (JVM Uber-JAR) copiado a: $WAILS_DIR/bin/solver.jar"
else
    echo "⚠️ Aviso: No se encontró el archivo *-runner.jar en target/."
    echo "Asegúrate de que la compilación de Quarkus haya finalizado correctamente."
fi

# 4. Generar JRE privado con jlink
echo "☕ Generando JRE privado con jlink para Linux..."
if command -v jlink &> /dev/null; then
    rm -rf "$WAILS_DIR/bin/jre" 2>/dev/null || true
    jlink --add-modules java.se,jdk.unsupported,jdk.management,jdk.crypto.ec,jdk.naming.dns,jdk.charsets,jdk.zipfs \
          --strip-debug \
          --no-man-pages \
          --no-header-files \
          --compress=2 \
          --output "$WAILS_DIR/bin/jre"
    echo "✅ JRE privado generado en: $WAILS_DIR/bin/jre"
else
    echo "⚠️ Aviso: No se encontró 'jlink' en el PATH. No se empaquetará el JRE privado."
    echo "Asegúrate de usar JDK 11+ (JRE minimal fallará sin él)."
fi

# 5. Copiar recursos compartidos
echo "📂 Copiando recursos compartidos y plantillas (shared-data, internal, static)..."
cp -r "$REPO_ROOT/shared-data" "$WAILS_DIR/build/bin/" 2>/dev/null || true
cp -r "$REPO_ROOT/shared-data" "$WAILS_DIR/" 2>/dev/null || true
cp -r "$REPO_ROOT/golang/internal" "$WAILS_DIR/build/bin/" 2>/dev/null || true
cp -r "$REPO_ROOT/golang/internal" "$WAILS_DIR/" 2>/dev/null || true
cp -r "$REPO_ROOT/golang/static" "$WAILS_DIR/build/bin/" 2>/dev/null || true
cp -r "$REPO_ROOT/golang/static" "$WAILS_DIR/" 2>/dev/null || true

cp -r "$REPO_ROOT/golang/static" "$WAILS_DIR/" 2>/dev/null || true

# Copiar JRE
if [ -d "$WAILS_DIR/bin/jre" ]; then
    cp -r "$WAILS_DIR/bin/jre" "$WAILS_DIR/build/bin/" 2>/dev/null || true
fi

# 6. Compilar aplicación de escritorio Wails
echo "🖥️ [3/3] Compilando aplicación de escritorio con Wails para Linux..."
cd "$WAILS_DIR"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
if command -v wails &> /dev/null; then
    wails build -platform linux/amd64 -tags webkit2_41 -clean
    rm -rf "$WAILS_DIR/build/bin/bin" 2>/dev/null || true
    cp -r "$WAILS_DIR/bin" "$WAILS_DIR/build/bin/"
    echo "🎉 ¡Empaquetado para Linux completado exitosamente!"
    echo "📍 Ejecutable disponible en: $WAILS_DIR/build/bin/HorariosDesktop"
else
    echo "⚠️ El CLI de 'wails' no se encuentra en el PATH. Compilando binario de ventana con 'go build -tags desktop,production,webkit2_41'..."
    GOOS=linux GOARCH=amd64 go build -tags desktop,production,webkit2_41 -ldflags="-s -w" -o "$WAILS_DIR/build/bin/HorariosDesktop" .
    rm -rf "$WAILS_DIR/build/bin/bin" 2>/dev/null || true
    cp -r "$WAILS_DIR/bin" "$WAILS_DIR/build/bin/"
    echo "🎉 ¡Compilación terminada!"
    echo "📍 Ejecutable disponible en: $WAILS_DIR/build/bin/HorariosDesktop"
fi
