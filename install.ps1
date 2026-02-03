# Dalang CLI Installer for Windows
# Usage: irm https://dalang.io/install.ps1 | iex
# Or: Invoke-WebRequest -Uri https://dalang.io/install.ps1 -UseBasicParsing | Invoke-Expression

$ErrorActionPreference = "Stop"

$DownloadBase = "https://dalang.io/cli"
$BinaryName = "dalang.exe"

function Write-Info { param($Message) Write-Host "[INFO] " -ForegroundColor Blue -NoNewline; Write-Host $Message }
function Write-Success { param($Message) Write-Host "[OK] " -ForegroundColor Green -NoNewline; Write-Host $Message }
function Write-Warn { param($Message) Write-Host "[WARN] " -ForegroundColor Yellow -NoNewline; Write-Host $Message }
function Write-Err { param($Message) Write-Host "[ERROR] " -ForegroundColor Red -NoNewline; Write-Host $Message }

function Get-InstallDir {
    # Use user's local bin directory
    $localBin = Join-Path $env:LOCALAPPDATA "Dalang"
    if (-not (Test-Path $localBin)) {
        New-Item -ItemType Directory -Path $localBin -Force | Out-Null
    }
    return $localBin
}

function Add-ToPath {
    param($Dir)

    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($currentPath -notlike "*$Dir*") {
        $newPath = "$currentPath;$Dir"
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        $env:Path = "$env:Path;$Dir"
        return $true
    }
    return $false
}

function Main {
    Write-Host ""
    Write-Host "  ____        _                    "
    Write-Host " |  _ \  __ _| | __ _ _ __   __ _  "
    Write-Host " | | | |/ _`` | |/ _`` | '_ \ / _`` | "
    Write-Host " | |_| | (_| | | (_| | | | | (_| | "
    Write-Host " |____/ \__,_|_|\__,_|_| |_|\__, | "
    Write-Host "                           |___/   CLI Installer"
    Write-Host ""

    # Detect architecture
    $arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
    Write-Info "Detected: windows/$arch"

    # Only amd64 is supported for now
    if ($arch -ne "amd64") {
        Write-Err "Only 64-bit Windows is supported"
        exit 1
    }

    # Construct download URL
    $binary = "dalang-windows-$arch.exe"
    $downloadUrl = "$DownloadBase/$binary"

    Write-Info "Downloading from: $downloadUrl"

    # Get install directory
    $installDir = Get-InstallDir
    $installPath = Join-Path $installDir $BinaryName

    # Download binary
    try {
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $downloadUrl -OutFile $installPath -UseBasicParsing
        Write-Success "Downloaded successfully"
    } catch {
        Write-Err "Failed to download binary: $_"
        exit 1
    }

    Write-Info "Installing to $installDir"

    # Add to PATH if needed
    $pathAdded = Add-ToPath $installDir
    if ($pathAdded) {
        Write-Info "Added $installDir to PATH"
    }

    # Verify installation
    if (Test-Path $installPath) {
        Write-Success "Dalang CLI installed successfully!"
        Write-Host ""
        Write-Info "Run 'dalang auth' to get started"
        Write-Host ""

        if ($pathAdded) {
            Write-Warn "Please restart your terminal for PATH changes to take effect"
            Write-Host ""
        }
    } else {
        Write-Err "Installation failed"
        exit 1
    }
}

Main
