# Wrapper en la raíz para ejecutar el script de empaquetado de Windows
$ScriptPath = Join-Path $PSScriptRoot "wails-desktop\scripts\build-windows.ps1"
if (Test-Path $ScriptPath) {
    & $ScriptPath @args
} else {
    Write-Error "No se encontró el script de compilación en: $ScriptPath"
}
