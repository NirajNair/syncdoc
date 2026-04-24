$ErrorActionPreference = "Stop"

$Repo = "NirajNair/syncdoc"
$Binary = "syncdoc"
$InstallDir = "${env:ProgramFiles}\syncdoc"

# ── Helpers ──────────────────────────────────────────────────────────

function Write-Info($Message)  { Write-Host "[INFO]  $Message" }
function Write-Warn($Message)  { Write-Host "[WARN]  $Message" -ForegroundColor Yellow }
function Write-Err($Message)   { Write-Host "[ERROR] $Message" -ForegroundColor Red }

# ── Detect architecture ──────────────────────────────────────────────
# goreleaser maps amd64 → x86_64 in archive names.

if ([Environment]::Is64BitOperatingSystem) {
    # goreleaser uses x86_64 in archive names, not amd64
    $Arch = "x86_64"
} else {
    Write-Err "Unsupported architecture: 32-bit operating system is not supported."
    exit 1
}

# ── Get latest version from GitHub ───────────────────────────────────

Write-Info "Installing $Binary..."

try {
    $LatestRelease = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -ErrorAction Stop
} catch {
    Write-Err "Failed to fetch latest release from GitHub API."
    Write-Err "  Check your internet connection and that https://github.com/$Repo/releases is accessible."
    Write-Err "  Error: $_"
    exit 1
}

$Version = $LatestRelease.tag_name

if (-not $Version) {
    Write-Err "Could not determine latest version from GitHub release."
    exit 1
}

# Archive naming must match goreleaser name_template:
#   {{ .ProjectName }}_{{ title .Os }}_{{ if eq .Arch "amd64" }}x86_64{{ else }}arm64{{ end }}
# No version in the archive name. OS is title-cased (Windows).
$ArchiveName = "${Binary}_Windows_${Arch}.zip"
$DownloadUrl = "https://github.com/$Repo/releases/download/$Version/$ArchiveName"

# ── Download ─────────────────────────────────────────────────────────

Write-Info "Downloading $Binary $Version for Windows/$Arch..."
Write-Info "  URL: $DownloadUrl"

$TempDir = Join-Path $env:TEMP "syncdoc-install"
if (Test-Path $TempDir) { Remove-Item -Recurse -Force $TempDir }
New-Item -ItemType Directory -Force -Path $TempDir | Out-Null
$ZipPath = Join-Path $TempDir $ArchiveName

try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $ZipPath -ErrorAction Stop
} catch {
    Write-Err "Download failed."
    Write-Err "  URL: $DownloadUrl"
    Write-Err "  This may mean the release asset '$ArchiveName' does not exist yet."
    Write-Err "  Check available assets at: https://github.com/$Repo/releases/tag/$Version"
    Write-Err "  Error: $_"
    exit 1
}

$Zipsize = (Get-Item $ZipPath).Length
if ($Zipsize -lt 100) {
    Write-Err "Downloaded file is suspiciously small ($Zipsize bytes). The download may have failed."
    Write-Err "  File: $ZipPath"
    exit 1
}

Write-Info "Downloaded $([math]::Round($Zipsize / 1KB, 1)) KB"

# ── Extract ──────────────────────────────────────────────────────────

Write-Info "Extracting..."
$OutDir = Join-Path $TempDir "out"

try {
    Expand-Archive -Path $ZipPath -DestinationPath $OutDir -Force -ErrorAction Stop
} catch {
    Write-Err "Failed to extract zip archive."
    Write-Err "  Error: $_"
    exit 1
}

# ── Locate binary ─────────────────────────────────────────────────────

$BinaryPath = Join-Path $OutDir "$Binary.exe"
if (-not (Test-Path $BinaryPath)) {
    # Search subdirectories (goreleaser may nest it)
    $BinaryPath = Get-ChildItem -Path $OutDir -Filter "$Binary.exe" -Recurse -ErrorAction SilentlyContinue |
        Select-Object -First 1 -ExpandProperty FullName

    if (-not $BinaryPath) {
        # Try without .exe
        $BinaryPath = Get-ChildItem -Path $OutDir -Filter $Binary -Recurse -ErrorAction SilentlyContinue |
            Select-Object -First 1 -ExpandProperty FullName

        if (-not $BinaryPath) {
            Write-Err "Binary '$Binary' not found in archive after extraction."
            Write-Err "Archive contents:"
            Get-ChildItem -Path $OutDir -Recurse | ForEach-Object { Write-Err "  $($_.FullName)" }
            Remove-Item -Recurse -Force $TempDir
            exit 1
        }
    }
}

# ── Install ──────────────────────────────────────────────────────────

Write-Info "Installing to $InstallDir..."

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item $BinaryPath -Destination "$InstallDir\$Binary.exe" -Force

# ── Add to PATH ──────────────────────────────────────────────────────

$PathEntry = "$InstallDir"
$CurrentPath = [Environment]::GetEnvironmentVariable("Path", "Machine")
if ($CurrentPath -notlike "*$PathEntry*") {
    Write-Info "Adding $InstallDir to system PATH..."
    try {
        [Environment]::SetEnvironmentVariable(
            "Path",
            "$CurrentPath;$PathEntry",
            "Machine"
        )
        $env:Path = "$env:Path;$PathEntry"
    } catch {
        Write-Warn "Could not add $InstallDir to PATH. You may need to add it manually."
        Write-Warn "  Error: $_"
    }
}

# ── Cleanup ──────────────────────────────────────────────────────────

Remove-Item -Recurse -Force $TempDir

# ── Done ─────────────────────────────────────────────────────────────

Write-Host ""
Write-Info "Installed $Binary $Version to $InstallDir\$Binary.exe"
Write-Host "Run '$Binary --help' to get started."
Write-Host ""
Write-Warn "NOTE: You may need to restart your terminal for PATH changes to take effect."