param(
    [string]$From   = (Get-Date).AddDays(-7).ToString("yyyy-MM-dd"),
    [string]$To     = (Get-Date).ToString("yyyy-MM-dd"),
    [string]$Output = "informe.xlsx"
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

Write-Host "Generando reporte: $From -> $To ($Output)"
& (Join-Path $PSScriptRoot "osreport.exe") --from $From --to $To --output $Output

Write-Host ""
Write-Host "Presiona una tecla para cerrar..."
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
