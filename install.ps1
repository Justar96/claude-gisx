#requires -version 5
<#
.SYNOPSIS
    claude-gisx installer for Windows.

.DESCRIPTION
    Downloads the matching Windows binary from GitHub Releases and wires it
    into ~/.claude/settings.json so Claude Code uses it as the statusline.

.EXAMPLE
    irm https://raw.githubusercontent.com/Justar96/claude-gisx/main/install.ps1 | iex

.PARAMETER Version
    Release tag to install (default: "latest").

.PARAMETER InstallDir
    Where to drop the binary. Default: $env:LOCALAPPDATA\Programs\claude-gisx
#>

[CmdletBinding()]
param(
    [string]$Version = $env:CLAUDE_GISX_VERSION,
    [string]$InstallDir = $env:CLAUDE_GISX_INSTALL_DIR,
    [string]$Repo = $env:CLAUDE_GISX_REPO,
    [switch]$SkipSetup
)

$ErrorActionPreference = "Stop"

if (-not $Version)    { $Version    = "latest" }
if (-not $Repo)       { $Repo       = "Justar96/claude-gisx" }
if (-not $InstallDir) { $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\claude-gisx" }

function Write-Step($msg) { Write-Host "  $msg" }
function Write-Ok($msg)   { Write-Host "  $([char]0x2713) $msg" -ForegroundColor Green }
function Write-Err($msg)  { Write-Host "  $([char]0x2717) $msg" -ForegroundColor Red }

Write-Host ""
Write-Host "  claude-gisx installer" -ForegroundColor Cyan
Write-Host "  github.com/$Repo" -ForegroundColor DarkGray
Write-Host ""

# ── detect arch ───────────────────────────────────────────────────────────
# PROCESSOR_ARCHITEW6432 wins when 32-bit PowerShell is hosted on 64-bit Windows;
# fall back to PROCESSOR_ARCHITECTURE otherwise. Both are always set on Windows.
$rawArch = $env:PROCESSOR_ARCHITEW6432
if (-not $rawArch) { $rawArch = $env:PROCESSOR_ARCHITECTURE }
$arch = switch ($rawArch.ToUpperInvariant()) {
    'AMD64' { 'x64' }
    'ARM64' { 'arm64' }
    'X86'   { throw "32-bit Windows is not supported. Build from source: https://github.com/$Repo#build-from-source" }
    default { throw "unsupported architecture: '$rawArch'" }
}
$target = "windows-$arch"
Write-Step "platform   $target"

# ── resolve tag ───────────────────────────────────────────────────────────
if ($Version -eq "latest") {
    $api = "https://api.github.com/repos/$Repo/releases/latest"
    try {
        $rel = Invoke-RestMethod -Uri $api -UseBasicParsing
        $Version = $rel.tag_name
    } catch {
        Write-Err "could not resolve latest release tag from $api"
        throw
    }
}
Write-Step "version    $Version"

$asset = "claude-gisx-$target.exe"
$url   = "https://github.com/$Repo/releases/download/$Version/$asset"
Write-Step "source     $url"
Write-Step "install to $InstallDir"
Write-Host ""

# ── download ──────────────────────────────────────────────────────────────
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("claude-gisx-" + [guid]::NewGuid())
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
$tmpFile = Join-Path $tmp $asset

Write-Step "downloading..."
try {
    Invoke-WebRequest -Uri $url -OutFile $tmpFile -UseBasicParsing
} catch {
    Write-Err "download failed: $($_.Exception.Message)"
    throw
}

# Optional checksum verification
$sumsUrl  = "https://github.com/$Repo/releases/download/$Version/SHA256SUMS"
$sumsFile = Join-Path $tmp "SHA256SUMS"
try {
    Invoke-WebRequest -Uri $sumsUrl -OutFile $sumsFile -UseBasicParsing -ErrorAction Stop
    $line = Get-Content $sumsFile | Where-Object { $_ -match "  $asset$" }
    if ($line) {
        $expected = ($line -split '\s+')[0].ToLower()
        $actual   = (Get-FileHash -Algorithm SHA256 $tmpFile).Hash.ToLower()
        if ($expected -ne $actual) { throw "checksum mismatch for $asset" }
        Write-Ok "checksum verified"
    }
} catch {
    # SHA256SUMS may not be published yet — skip silently.
}

# ── install ───────────────────────────────────────────────────────────────
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$dest = Join-Path $InstallDir "claude-gisx.exe"
Move-Item -Force -Path $tmpFile -Destination $dest
Write-Ok "installed $dest"

Remove-Item -Recurse -Force $tmp

# ── PATH hint ─────────────────────────────────────────────────────────────
$onPath = ($env:PATH -split ';') -contains $InstallDir
if (-not $onPath) {
    Write-Host ""
    Write-Host "  $InstallDir is not in your PATH." -ForegroundColor DarkGray
    Write-Host "  Add it for the current user with:" -ForegroundColor DarkGray
    Write-Host "    [Environment]::SetEnvironmentVariable('Path', `"`$env:PATH;$InstallDir`", 'User')"
}

# ── wire into Claude Code ─────────────────────────────────────────────────
if (-not $SkipSetup) {
    Write-Host ""
    Write-Step "running 'claude-gisx setup'..."
    # --force so reinstalling refreshes settings.json with the current binary's
    # command path (e.g. when upgrading from a version that wrote a bare name).
    & $dest setup --force
    if ($LASTEXITCODE -ne 0) {
        Write-Err "setup failed — run '$dest setup' manually"
        exit 4
    }
}

Write-Host ""
Write-Ok "done. Restart Claude Code to see the new statusline."
Write-Host ""
