#!/bin/bash

# Script de compilación y empaquetado cruzado para Windows (Wails Desktop + Go API + Kotlin Quarkus Native)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WAILS_DIR="$(dirname "$SCRIPT_DIR")"
REPO_ROOT="$(dirname "$WAILS_DIR")"

echo "🪟 --- Iniciando Compilación para Windows (x86_64) ---"

mkdir -p "$WAILS_DIR/bin"
mkdir -p "$WAILS_DIR/build/bin"

# 1. Compilar Go API nativo para Windows
echo "📦 [1/3] Compilando Backend Go para Windows (.exe)..."
cd "$REPO_ROOT/golang"
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o "$WAILS_DIR/bin/api.exe" ./cmd/api/main.go
echo "✅ Backend Go compilado en: $WAILS_DIR/bin/api.exe"

# 2. Compilar Kotlin Quarkus en modo JVM (Uber-JAR)
echo "📦 [2/3] Compilando Solver Kotlin con Quarkus en modo JVM (Uber-JAR)..."
cd "$REPO_ROOT/kotlin"
MVN_CMD="mvn"
if [ -f "./mvnw" ]; then
    MVN_CMD="./mvnw"
fi

$MVN_CMD package -Dquarkus.package.jar.type=uber-jar -DskipTests

RUNNER_JAR=$(find target/ -maxdepth 1 -name "*-runner.jar" | head -n 1)
if [ -f "$RUNNER_JAR" ]; then
    rm -f "$WAILS_DIR/bin/solver"* 2>/dev/null || true
    cp "$RUNNER_JAR" "$WAILS_DIR/bin/solver.jar"
    echo "✅ Solver Kotlin (JVM Uber-JAR) copiado a: $WAILS_DIR/bin/solver.jar"
else
    echo "⚠️ Aviso: No se encontró *-runner.jar en target/."
fi

# 4. Generar JRE privado con jlink
echo "☕ Generando JRE privado con jlink para Windows (requiere jlink en PATH)..."
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

# 6. Compilar aplicación de escritorio Wails para Windows
echo "🖥️ [3/3] Compilando aplicación Wails para Windows (.exe)..."
cd "$WAILS_DIR"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
if command -v wails &> /dev/null; then
    wails build -platform windows/amd64 -clean || {
        echo "⚠️ Falló wails build cruzado. Compilando directamente con go build -tags desktop,production para Windows..."
        GOOS=windows GOARCH=amd64 go build -tags desktop,production -ldflags="-s -w -H windowsgui" -o "$WAILS_DIR/build/bin/HorariosDesktop.exe" .
    }
else
    echo "⚠️ Wails CLI no detectado en PATH. Compilando binario con go build -tags desktop,production para Windows..."
    GOOS=windows GOARCH=amd64 go build -tags desktop,production -ldflags="-s -w -H windowsgui" -o "$WAILS_DIR/build/bin/HorariosDesktop.exe" .
fi
cp -r "$WAILS_DIR/bin" "$WAILS_DIR/build/bin/" 2>/dev/null || true

echo "🎉 ¡Empaquetado para Windows completado exitosamente!"
echo "📍 Ejecutable listo en: $WAILS_DIR/build/bin/HorariosDesktop.exe"
