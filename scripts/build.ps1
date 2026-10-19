<#
.SYNOPSIS
    Compila o OpenIDM e gera o instalador de release.

.DESCRIPTION
    1. Compila o frontend (React + Vite)
    2. Gera os recursos do Windows (ícone e versão) com go-winres
    3. Executa os testes
    4. Compila openidm.exe e openidm-native-host.exe em build/bin
    5. Embute os binários e a extensão no instalador (OpenIDM-Setup.exe)
    6. Copia o instalador versionado e o SHA256 para release/v<versão>

    A versão é lida de wails.json (info.productVersion).

.EXAMPLE
    ./scripts/build.ps1

.EXAMPLE
    ./scripts/build.ps1 -SkipTests
#>
[CmdletBinding()]
param(
    [switch]$SkipFrontend,
    [switch]$SkipTests
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

$Version    = (Get-Content (Join-Path $Root 'wails.json') -Raw | ConvertFrom-Json).info.productVersion
$BinDir     = Join-Path $Root 'build\bin'
$PayloadDir = Join-Path $Root 'build\payload'
$PayloadZip = Join-Path $Root 'cmd\installer\payload.zip'
$ReleaseDir = Join-Path $Root "release\v$Version"
$LdFlags    = '-s -w -H=windowsgui'

function Write-Step([string]$Message) {
    Write-Host ""
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Invoke-Checked([string]$File, [string[]]$Arguments) {
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Comando falhou ($LASTEXITCODE): $File $($Arguments -join ' ')"
    }
}

Write-Host "OpenIDM v$Version — build de release" -ForegroundColor Green

# ─── 1. Frontend ────────────────────────────────────────────────────
if (-not $SkipFrontend) {
    Write-Step 'Compilando o frontend'
    Push-Location (Join-Path $Root 'frontend')
    try {
        if (-not (Test-Path 'node_modules')) {
            Invoke-Checked 'npm' @('ci', '--no-audit', '--no-fund')
        }
        Invoke-Checked 'npm' @('run', 'build')
    }
    finally {
        Pop-Location
    }
}
if (-not (Test-Path (Join-Path $Root 'frontend\dist\index.html'))) {
    throw 'frontend/dist/index.html não encontrado. Rode sem -SkipFrontend.'
}

# ─── 2. Recursos do Windows (ícone, manifesto e versão) ─────────────
Write-Step 'Gerando recursos do Windows (go-winres)'
if (-not (Get-Command 'go-winres' -ErrorAction SilentlyContinue)) {
    Invoke-Checked 'go' @('install', 'github.com/tc-hib/go-winres@latest')
}
$WinresJson = Join-Path $Root 'build\windows\winres.json'
foreach ($Target in @('rsrc', 'cmd\native-host\rsrc', 'cmd\installer\rsrc')) {
    Invoke-Checked 'go-winres' @(
        'make', '--in', $WinresJson, '--arch', 'amd64', '--out', (Join-Path $Root $Target),
        '--product-version', $Version, '--file-version', "$Version.0"
    )
}

# ─── 3. Testes ──────────────────────────────────────────────────────
if (-not $SkipTests) {
    Write-Step 'Executando os testes'
    Invoke-Checked 'go' @('vet', './...')
    Invoke-Checked 'go' @('test', './...')
}

# ─── 4. Aplicativo e Native Host ────────────────────────────────────
Write-Step 'Compilando openidm.exe e openidm-native-host.exe'
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
Invoke-Checked 'go' @('build', '-trimpath', '-tags', 'desktop,production', '-ldflags', $LdFlags, '-o', (Join-Path $BinDir 'openidm.exe'), '.')
Invoke-Checked 'go' @('build', '-trimpath', '-ldflags', $LdFlags, '-o', (Join-Path $BinDir 'openidm-native-host.exe'), './cmd/native-host')

# ─── 5. Instalador com os arquivos embutidos ────────────────────────
Write-Step 'Empacotando e compilando o instalador'
if (Test-Path $PayloadDir) { Remove-Item -Recurse -Force $PayloadDir }
New-Item -ItemType Directory -Force -Path $PayloadDir | Out-Null
Copy-Item (Join-Path $BinDir 'openidm.exe') $PayloadDir
Copy-Item (Join-Path $BinDir 'openidm-native-host.exe') $PayloadDir
Copy-Item -Recurse (Join-Path $Root 'browser-extension') $PayloadDir

try {
    if (Test-Path $PayloadZip) { Remove-Item -Force $PayloadZip }
    Compress-Archive -Path (Join-Path $PayloadDir '*') -DestinationPath $PayloadZip -CompressionLevel Optimal
    Invoke-Checked 'go' @('build', '-trimpath', '-tags', 'embedpayload', '-ldflags', $LdFlags, '-o', (Join-Path $BinDir 'OpenIDM-Setup.exe'), './cmd/installer')
}
finally {
    if (Test-Path $PayloadZip) { Remove-Item -Force $PayloadZip }
    if (Test-Path $PayloadDir) { Remove-Item -Recurse -Force $PayloadDir }
}

# ─── 6. Release ─────────────────────────────────────────────────────
Write-Step "Publicando em release/v$Version"
New-Item -ItemType Directory -Force -Path $ReleaseDir | Out-Null
$SetupName = "OpenIDM-Setup-$Version-x64.exe"
$SetupPath = Join-Path $ReleaseDir $SetupName
Copy-Item (Join-Path $BinDir 'OpenIDM-Setup.exe') $SetupPath -Force

$Hash = (Get-FileHash $SetupPath -Algorithm SHA256).Hash.ToLower()
Set-Content -Path (Join-Path $ReleaseDir 'SHA256SUMS.txt') -Value "$Hash  $SetupName" -Encoding ascii

$SizeMB = [math]::Round((Get-Item $SetupPath).Length / 1MB, 1)
Write-Host ""
Write-Host "Pronto!" -ForegroundColor Green
Write-Host "  Instalador: release/v$Version/$SetupName ($SizeMB MB)"
Write-Host "  SHA256:     $Hash"
