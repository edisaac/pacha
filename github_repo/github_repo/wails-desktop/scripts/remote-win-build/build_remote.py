#!/usr/bin/env python3
import pty
import os
import sys

WINDOWS_HOST = "localhost"
WINDOWS_PORT = "2222"
WINDOWS_USER = "edmejia"
WINDOWS_PASS = "temporal"
WINDOWS_SHARED_DIR = "E:\\horarios"

def run_ssh_command(description, command):
    print(f"\n==================================================")
    print(f"🚀 {description}")
    print(f"==================================================")
    
    pid, fd = pty.fork()
    if pid == 0:
        os.execlp("ssh", "ssh", "-o", "StrictHostKeyChecking=no", "-p", WINDOWS_PORT, f"{WINDOWS_USER}@{WINDOWS_HOST}", f"powershell -c \"{command}\"")
    else:
        output = b""
        while True:
            try:
                data = os.read(fd, 1024)
                if not data:
                    break
                sys.stdout.buffer.write(data)
                sys.stdout.buffer.flush()
                output += data
                # Detect SSH password prompt
                if b"assword:" in output:
                    os.write(fd, WINDOWS_PASS.encode() + b"\n")
                    output = b""
            except OSError:
                break
        os.waitpid(pid, 0)

if __name__ == "__main__":
    print("Iniciando compilación remota hacia la máquina virtual Windows...")
    
    # 1. Instalar Wails si no está
    run_ssh_command(
        "Instalando Wails CLI en Windows (si es necesario)...",
        "if (!(Get-Command wails -ErrorAction SilentlyContinue)) { echo 'Instalando Wails...'; go install github.com/wailsapp/wails/v2/cmd/wails@latest } else { echo 'Wails ya esta instalado.' }"
    )
    
    # 2. Copiar a C:\ para evitar bugs de VirtualBox Shared Folders
    run_ssh_command(
        "Cerrando procesos residuales y copiando proyecto al disco C:\\ para evitar errores de red (Shared Folders)...",
        f"Stop-Process -Name java,api,HorariosDesktop -Force -ErrorAction SilentlyContinue; if (Test-Path C:\\horarios_build) {{ Remove-Item -Recurse -Force C:\\horarios_build }}; New-Item -ItemType Directory -Force C:\\horarios_build; Write-Host 'Copiando archivos con xcopy, esto puede tardar un poco...'; xcopy {WINDOWS_SHARED_DIR} C:\\horarios_build /E /I /Y /H | Out-Null"
    )
    
    # 3. Ejecutar el script principal build-windows.ps1 en C:
    run_ssh_command(
        "Ejecutando build-windows.ps1 en C:\\...",
        "cd C:\\horarios_build\\wails-desktop\\scripts; Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope Process; .\\build-windows.ps1"
    )
    
    # 4. Empaquetar con NSIS
    run_ssh_command(
        "Generando el instalador NSIS (.exe)...",
        "cd C:\\horarios_build\\wails-desktop; wails build -platform windows/amd64 -nsis"
    )
    
    # 5. Comprimir el resultado y copiar el ZIP de vuelta a la unidad compartida
    run_ssh_command(
        "Comprimiendo la aplicacion completa y enviándola de vuelta a Linux (E:\\horarios)...",
        f"if (Test-Path C:\\horarios_build\\HorariosDesktop-Windows.zip) {{ Remove-Item -Force C:\\horarios_build\\HorariosDesktop-Windows.zip }}; Compress-Archive -Path C:\\horarios_build\\wails-desktop\\build\\bin\\* -DestinationPath C:\\horarios_build\\HorariosDesktop-Windows.zip -Force; Copy-Item -Path C:\\horarios_build\\HorariosDesktop-Windows.zip -Destination {WINDOWS_SHARED_DIR}\\ -Force"
    )
    
    print("\n✅ Compilación remota finalizada.")
    print(f"La aplicación portátil está lista y comprimida en tu Linux en: /mnt/code/repos/timefold/horarios/HorariosDesktop-Windows.zip")
    print("Descomprímelo directamente en tu Windows (por ejemplo, en el Escritorio) y ejecuta HorariosDesktop.exe")
