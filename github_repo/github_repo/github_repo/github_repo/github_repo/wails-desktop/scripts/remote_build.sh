#!/bin/bash

# Configuración de conexión remota a la VM de Windows
WINDOWS_HOST="localhost"
WINDOWS_PORT="2222"
WINDOWS_USER="edmejia"
WINDOWS_PASS="temporal"

echo "=================================================="
echo "🚀 Conectando a la VM Windows para compilar..."
echo "=================================================="

# Instalar Wails CLI en Windows si no existe
echo "1. Asegurando que Wails CLI esté instalado en Windows..."
sshpass -p "$WINDOWS_PASS" ssh -o StrictHostKeyChecking=no -p $WINDOWS_PORT $WINDOWS_USER@$WINDOWS_HOST 'powershell -c "if (!(Get-Command wails -ErrorAction SilentlyContinue)) { echo \"Instalando Wails...\"; go install github.com/wailsapp/wails/v2/cmd/wails@latest } else { echo \"Wails ya está instalado.\" }"'

# Ejecutar el build
echo "2. Ejecutando build-windows.ps1 en la unidad compartida (E:\horarios)..."
sshpass -p "$WINDOWS_PASS" ssh -o StrictHostKeyChecking=no -p $WINDOWS_PORT $WINDOWS_USER@$WINDOWS_HOST 'powershell -c "cd E:\horarios\wails-desktop\scripts; Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope Process; .\build-windows.ps1"'

# Generar el instalador NSIS
echo "3. Generando el instalador NSIS..."
sshpass -p "$WINDOWS_PASS" ssh -o StrictHostKeyChecking=no -p $WINDOWS_PORT $WINDOWS_USER@$WINDOWS_HOST 'powershell -c "cd E:\horarios\wails-desktop; wails build -platform windows/amd64 -nsis"'

echo "✅ Compilación remota finalizada. El instalador debería estar en E:\horarios\wails-desktop\build\bin"
