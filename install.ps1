$ErrorActionPreference = "Stop"

$Repo = "NirajNair/syncdoc"
$Binary = "syncdoc"
$InstallDir = "${env:ProgramFiles}\syncdoc"

# Detect architecture
if ([Environment]::Is64BitOperatingSystem) {
    if ([Environment]::Is64BitProcess) {
        $Arch = "amd64"
    } else {
        $Arch = "amd64"
    }
} else {
    $Arch = "amd64"
}

# Get latest version
Write-Host "Installing $Binary..."
$LatestRelease = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
$Version = $LatestRelease.tag_name

if (-not $Version) {
    Write-Error "Could not determine latest version"
    exit 1
}

$VersionClean = $Version.TrimStart("v")
$ArchiveName = "${Binary}_${VersionClean}_windows_${Arch}.zip"
$DownloadUrl = "https://github.com/$Repo/releases/download/$Version/$ArchiveName"

Write-Host "Downloading $Binary $Version for windows/$Arch..."

# Download
$TempDir = Join-Path $env:TEMP "syncdoc-install"
New-Item -ItemType Directory -Force -Path $TempDir | Out-Null
$ZipPath = Join-Path $TempDir $ArchiveName
Invoke-WebRequest -Uri $DownloadUrl -OutFile $ZipPath

# Extract
Write-Host "Extracting..."
Expand-Archive -Path $ZipPath -DestinationPath $TempDir\out -Force

# Install
$BinaryPath = Join-Path "$TempDir\out" "$Binary.exe"
if (-not (Test-Path $BinaryPath)) {
    # Try without .exe extension in archive
    $BinaryPath = Join-Path "$TempDir\out" $Binary
    if (-not (Test-Path $BinaryPath)) {
        Write-Error "Binary not found in archive"
        Remove-Item -Recurse -Force $TempDir
        exit 1
    }
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item $BinaryPath -Destination "$InstallDir\$Binary.exe" -Force

# Add to PATH if not present
$PathEntry = "$InstallDir"
$CurrentPath = [Environment]::GetEnvironmentVariable("Path", "Machine")
if ($CurrentPath -notlike "*$PathEntry*") {
    Write-Host "Adding $InstallDir to system PATH..."
    [Environment]::SetEnvironmentVariable(
        "Path",
        "$CurrentPath;$PathEntry",
        "Machine"
    )
    $env:Path = "$env:Path;$PathEntry"
}

# Cleanup
Remove-Item -Recurse -Force $TempDir

Write-Host ""
Write-Host "Installed $Binary $Version to $InstallDir\$Binary.exe"
Write-Host "Run '$Binary --help' to get started."
Write-Host ""
Write-Host "NOTE: You may need to restart your terminal for PATH changes to take effect."