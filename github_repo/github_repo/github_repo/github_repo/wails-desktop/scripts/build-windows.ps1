# Script de compilacion y empaquetado para Windows (PowerShell)

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$WailsDir = Split-Path -Parent $ScriptDir
$RepoRoot = Split-Path -Parent $WailsDir

Write-Host "--- Iniciando Compilacion para Windows (x86_64) ---" -ForegroundColor Cyan

Stop-Process -Name HorariosDesktop, api -Force -ErrorAction SilentlyContinue

New-Item -ItemType Directory -Force -Path "$WailsDir\bin" | Out-Null
New-Item -ItemType Directory -Force -Path "$WailsDir\build\bin" | Out-Null

# 1. Compilar Go API nativo
Write-Host "[1/3] Sincronizando dependencias y compilando Backend Go para Windows (.exe)..." -ForegroundColor Yellow
Set-Location "$RepoRoot\golang"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go get modernc.org/sqlite
go mod tidy
go build -ldflags="-s -w" -o "$WailsDir\bin\api.exe" ./cmd/api/main.go
Write-Host "Backend Go compilado en: $WailsDir\bin\api.exe" -ForegroundColor Green

# 2. Compilar Kotlin Quarkus en modo JVM (Uber-JAR)
Write-Host "[2/3] Compilando Solver Kotlin con Quarkus en modo JVM (Uber-JAR)..." -ForegroundColor Yellow
Set-Location "$RepoRoot\kotlin"
$MvnCmd = "mvn"
if (Test-Path ".\mvnw.cmd") {
    $MvnCmd = ".\mvnw.cmd"
}
& $MvnCmd package '-Dquarkus.package.jar.type=uber-jar' '-DskipTests'

$RunnerJar = Get-ChildItem -Path "target" -Filter "*-runner.jar" | Select-Object -First 1
if ($RunnerJar) {
    Remove-Item -Path "$WailsDir\bin\solver*" -Force -ErrorAction SilentlyContinue
    Copy-Item $RunnerJar.FullName "$WailsDir\bin\solver.jar" -Force
    Write-Host "Solver Kotlin (JVM Uber-JAR) copiado a: $WailsDir\bin\solver.jar" -ForegroundColor Green
} else {
    Write-Host "Aviso: No se encontro *-runner.jar en target/." -ForegroundColor Yellow
}

# 3. Generar JRE privado con jlink
Write-Host "Generando JRE privado con jlink para Windows..." -ForegroundColor Yellow
if (Get-Command jlink -ErrorAction SilentlyContinue) {
    Remove-Item -Path "$WailsDir\bin\jre" -Recurse -Force -ErrorAction SilentlyContinue
    & jlink --add-modules java.se,jdk.unsupported,jdk.management,jdk.crypto.ec,jdk.naming.dns,jdk.charsets,jdk.zipfs `
            --strip-debug `
            --no-man-pages `
            --no-header-files `
            --compress=2 `
            --output "$WailsDir\bin\jre"
    Write-Host "JRE privado generado en: $WailsDir\bin\jre" -ForegroundColor Green
} else {
    Write-Host "Aviso: No se encontro 'jlink' en PATH. No se empaquetara el JRE privado." -ForegroundColor Yellow
}

# 4. Compilar Wails Desktop
Write-Host "Sincronizando dependencias de Wails para evitar pantalla en blanco..." -ForegroundColor Yellow
Set-Location $WailsDir
go get github.com/wailsapp/wails/v2@latest
go mod tidy

Write-Host "Compilando aplicacion de escritorio con Wails..." -ForegroundColor Yellow
if (Get-Command wails -ErrorAction SilentlyContinue) {
    wails build -platform windows/amd64 -clean
} else {
    Write-Host "Wails CLI no encontrado en PATH. Compilando con go build -tags desktop,production..." -ForegroundColor Yellow
    go build -tags desktop,production -ldflags="-s -w -H windowsgui" -o "$WailsDir\build\bin\HorariosDesktop.exe" .
}
New-Item -ItemType Directory -Force -Path "$WailsDir\build\bin\bin" | Out-Null
Copy-Item "$WailsDir\bin\*" "$WailsDir\build\bin\bin" -Recurse -Force

# 5. Copiar recursos compartidos (DESPUES DE WAILS BUILD PARA QUE NO SE BORREN CON -clean)
Write-Host "Copiando recursos compartidos y plantillas (shared-data, internal, static)..." -ForegroundColor Yellow
if (Test-Path "$RepoRoot\shared-data") {
    xcopy "$RepoRoot\shared-data" "$WailsDir\build\bin\shared-data" /E /I /Y /H | Out-Null
    xcopy "$RepoRoot\shared-data" "$WailsDir\shared-data" /E /I /Y /H | Out-Null
}
if (Test-Path "$RepoRoot\golang\internal") {
    xcopy "$RepoRoot\golang\internal" "$WailsDir\build\bin\internal" /E /I /Y /H | Out-Null
    xcopy "$RepoRoot\golang\internal" "$WailsDir\internal" /E /I /Y /H | Out-Null
}
if (Test-Path "$RepoRoot\golang\static") {
    xcopy "$RepoRoot\golang\static" "$WailsDir\build\bin\static" /E /I /Y /H | Out-Null
    xcopy "$RepoRoot\golang\static" "$WailsDir\static" /E /I /Y /H | Out-Null
}

# Copiar JRE
if (Test-Path "$WailsDir\bin\jre") {
    xcopy "$WailsDir\bin\jre" "$WailsDir\build\bin\jre" /E /I /Y /H | Out-Null
}

Write-Host "Empaquetado para Windows completado con exito!" -ForegroundColor Green
Write-Host "Ejecutable listo en: $WailsDir\build\bin\HorariosDesktop.exe" -ForegroundColor Green
