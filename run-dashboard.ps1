param(
    [string]$Port = "8081",
    [switch]$Team
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

if ($Team) {
    $virtualKeywords = "VirtualBox", "VMware", "VPN", "WireGuard", "TAP", "Tunnel", "Hyper-V", "Virtual"

    $addresses = Get-NetIPAddress -AddressFamily IPv4 | Where-Object {
        $_.IPAddress -notlike "127.*" -and $_.IPAddress -notlike "169.254.*"
    } | ForEach-Object {
        $adapter = Get-NetAdapter -InterfaceIndex $_.InterfaceIndex -ErrorAction SilentlyContinue
        $desc = if ($adapter) { $adapter.InterfaceDescription } else { "" }
        $isVirtual = $false
        foreach ($kw in $virtualKeywords) {
            if ($desc -like "*$kw*" -or $_.InterfaceAlias -like "*$kw*") { $isVirtual = $true }
        }
        [PSCustomObject]@{ IPAddress = $_.IPAddress; Alias = $_.InterfaceAlias; Description = $desc; IsVirtual = $isVirtual }
    }

    Write-Host ""
    Write-Host "=== Dashboard en modo EQUIPO (sin autenticacion, red interna) ==="
    $real = $addresses | Where-Object { -not $_.IsVirtual }
    $virtual = $addresses | Where-Object { $_.IsVirtual }

    if ($real) {
        Write-Host "Comparte una de estas URLs con el equipo:"
        foreach ($a in $real) {
            Write-Host ("  http://{0}:{1}   ({2})" -f $a.IPAddress, $Port, $a.Alias)
        }
    }
    if ($virtual) {
        Write-Host ""
        Write-Host "No compartas estas (VPN/adaptador virtual, nadie mas las puede alcanzar):"
        foreach ($a in $virtual) {
            Write-Host ("  http://{0}:{1}   ({2}: {3})" -f $a.IPAddress, $Port, $a.Alias, $a.Description)
        }
    }
    Write-Host ""
    Write-Host "Deja esta ventana abierta - si la cierras, el dashboard deja de responder para todos."
    Write-Host ""
} else {
    Write-Host "Iniciando dashboard en http://localhost:$Port ..."
}

Start-Process "http://localhost:$Port"

$serveArgs = @("serve", "--port", $Port)
if ($Team) { $serveArgs += "--bind-all" }
& (Join-Path $PSScriptRoot "osreport.exe") @serveArgs
