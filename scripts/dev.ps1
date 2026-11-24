$ErrorActionPreference = 'Stop'

$projectRoot = Split-Path -Parent $PSScriptRoot
$backendPath = Join-Path $projectRoot 'backend'
$frontendPath = Join-Path $projectRoot 'frontend'

$serverProcess = Start-Process -FilePath 'go' `
    -ArgumentList @('run', './cmd/server', '-addr', ':8080', '-db', './data/nash-load-lab.db') `
    -WorkingDirectory $backendPath `
    -PassThru `
    -WindowStyle Hidden

try {
    Set-Location $frontendPath
    npm run dev
}
finally {
    if (-not $serverProcess.HasExited) {
        Stop-Process -Id $serverProcess.Id -Force
    }
}
