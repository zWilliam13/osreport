param(
    [string]$Port = "8080"
)

$envFile = Join-Path $PSScriptRoot ".env"
if (-not (Test-Path $envFile)) {
    Write-Error "No se encontro .env en $PSScriptRoot. Copia .env.example a .env y completa tus credenciales."
    exit 1
}

Get-Content $envFile | ForEach-Object {
    $line = $_.Trim()
    if ($line -eq "" -or $line.StartsWith("#")) { return }
    $parts = $line -split "=", 2
    if ($parts.Length -eq 2) {
        [Environment]::SetEnvironmentVariable($parts[0], $parts[1], "Process")
    }
}

Write-Host "Iniciando dashboard en http://localhost:$Port ..."
Start-Process "http://localhost:$Port"

& (Join-Path $PSScriptRoot "osreport.exe") serve --port $Port
