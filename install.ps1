# Self-contained Windows installer for ANX. No elevation or policy changes.
# powershell.exe -NoProfile -Command "irm https://sh.arturonavax.dev/install.ps1 | iex"
[CmdletBinding()]
param(
    [string]$ConfigRepo = "",
    [string]$ConfigRef = "",
    [string]$BinName = $env:ANX_BIN_NAME,
    [switch]$FromSource,
    [switch]$GoInstall,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$ForwardArgs
)

$ErrorActionPreference = 'Stop'
$localSource = $PSScriptRoot
if (-not $env:LOCALAPPDATA) { throw 'LOCALAPPDATA is not set for the current user.' }
if ($BinName -and $BinName -notmatch '^[a-zA-Z0-9][a-zA-Z0-9._-]*$') { throw 'Invalid ANX executable alias.' }
if ($ConfigRepo) { $env:ANX_CONFIG_REPO = $ConfigRepo }
if ($ConfigRef) { $env:ANX_CONFIG_REF = $ConfigRef }

$arch = $null
try { $arch = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString() } catch { }
if (-not $arch) {
    $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
}
$AnxArchitecture = switch ($arch) {
    { $_ -in @('X64', 'AMD64') } { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "Unsupported Windows architecture: $arch. ANX supports x64 and ARM64." }
}
$DestDir = Join-Path $env:LOCALAPPDATA 'Programs/anx/bin'
$minimumGo = [version]'1.27.3'

function Write-Step([string]$msg) { Write-Host "  [STEP] $msg" -ForegroundColor White }
function Write-Success([string]$msg) { Write-Host "  [OK]   $msg" -ForegroundColor Green }
function Write-WarnMsg([string]$msg) { Write-Warning $msg }

# Use compiled .NET APIs rather than importing script modules from the one-liner.
function Get-AnxSHA256([string]$path) {
    $stream = [IO.File]::OpenRead($path)
    $sha256 = [Security.Cryptography.SHA256]::Create()
    try {
        return [BitConverter]::ToString($sha256.ComputeHash($stream)).Replace('-', '')
    } finally {
        $sha256.Dispose()
        $stream.Dispose()
    }
}

function Expand-AnxZip([string]$archive, [string]$destination) {
    if (-not ('System.IO.Compression.ZipFile' -as [type])) {
        Add-Type -AssemblyName System.IO.Compression.FileSystem
    }
    [IO.Compression.ZipFile]::ExtractToDirectory($archive, $destination)
}

function Add-AnxPath([string]$directory) {
    $separator = [IO.Path]::PathSeparator
    if (($env:Path -split [regex]::Escape($separator)) -notcontains $directory) {
        $env:Path = "$directory$separator$env:Path"
    }
    if ($env:ANX_NO_PATH -eq '1') { return }
    try {
        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        if (($userPath -split [regex]::Escape($separator)) -notcontains $directory) {
            $nextPath = if ($userPath) { "$directory$separator$userPath" } else { $directory }
            [Environment]::SetEnvironmentVariable('Path', $nextPath, 'User')
        }
    } catch {
        Write-WarnMsg "Could not update the user PATH. Run ANX directly from $directory."
    }
}

function Install-AnxBinary([string]$binary) {
    & $binary version | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "The staged ANX executable did not pass its version check (exit $LASTEXITCODE)." }
    New-Item -ItemType Directory -Path $DestDir -Force | Out-Null
    $target = Join-Path $DestDir 'anx.exe'
    $staged = Join-Path $DestDir ("anx-" + [Guid]::NewGuid().ToString('N') + '.tmp')
    $backup = "$staged.backup"
    $replaced = $false
    try {
        Copy-Item -LiteralPath $binary -Destination $staged
        if (Test-Path -LiteralPath $target) {
            [IO.File]::Replace($staged, $target, $backup)
            $replaced = $true
        } else {
            [IO.File]::Move($staged, $target)
        }
    } finally {
        if (Test-Path -LiteralPath $staged) { Remove-Item -LiteralPath $staged -Force }
        if ($replaced -and (Test-Path -LiteralPath $backup)) { Remove-Item -LiteralPath $backup -Force }
    }
    Copy-Item -LiteralPath $target -Destination (Join-Path $DestDir 'safe-suspend.exe') -Force
    if ($BinName -and $BinName -notin @('anx', 'safe-suspend')) {
        Copy-Item -LiteralPath $target -Destination (Join-Path $DestDir "$BinName.exe") -Force
    }
    Write-Success "Installed ANX at $target"
}

function Get-UsableGo {
    $command = Get-Command 'go.exe' -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $command) { return }
    $version = & $command.Source version
    if ($LASTEXITCODE -eq 0 -and "$version" -match '\bgo(\d+\.\d+(?:\.\d+)?)\b') {
        if ([version]$Matches[1] -ge $minimumGo) { return $command.Source }
    }
}

function Install-PortableGo([string]$scratch) {
    Write-Step 'Finding a verified Go archive for this user...'
    $releases = Invoke-RestMethod -Uri 'https://go.dev/dl/?mode=json' -UseBasicParsing -TimeoutSec 30
    $file = $releases | Where-Object {
        $_.stable -and $_.version -match '^go\d+\.\d+(\.\d+)?$' -and
        [version]$_.version.Substring(2) -ge $minimumGo
    } | ForEach-Object { $_.files } | Where-Object {
        $_.os -eq 'windows' -and $_.arch -eq $AnxArchitecture -and $_.kind -eq 'archive' -and
        $_.filename -match "^go\d+\.\d+(\.\d+)?\.windows-$AnxArchitecture\.zip$"
    } | Select-Object -First 1
    if (-not $file -or $file.sha256 -notmatch '^[0-9a-fA-F]{64}$') { throw 'No compatible verified Go archive was found.' }

    $goRoot = Join-Path (Join-Path $env:LOCALAPPDATA 'anx/toolchains') ([IO.Path]::GetFileNameWithoutExtension($file.filename))
    $goExe = Join-Path $goRoot 'bin/go.exe'
    if (-not (Test-Path -LiteralPath $goExe -PathType Leaf)) {
        $zip = Join-Path $scratch 'go.zip'
        Invoke-WebRequest -Uri "https://go.dev/dl/$($file.filename)" -OutFile $zip -UseBasicParsing -TimeoutSec 180
        if ((Get-AnxSHA256 $zip) -ne $file.sha256) {
            throw 'Go archive checksum verification failed.'
        }
        $extracted = Join-Path $scratch 'go-extract'
        Expand-AnxZip $zip $extracted
        $portable = Join-Path $extracted 'go'
        if (-not (Test-Path -LiteralPath (Join-Path $portable 'bin/go.exe') -PathType Leaf)) {
            throw 'The verified Go archive did not contain go.exe.'
        }
        if (Test-Path -LiteralPath $goRoot) { throw "The portable Go directory is incomplete: $goRoot" }
        New-Item -ItemType Directory -Path (Split-Path -Parent $goRoot) -Force | Out-Null
        Move-Item -LiteralPath $portable -Destination $goRoot
    }
    $version = & $goExe version
    if ($LASTEXITCODE -ne 0) { throw 'The portable Go executable could not run.' }
    Write-Success "$version"
    return $goExe
}

function Test-AnxSource([string]$path) {
    if (-not $path) { return $false }
    $module = Join-Path $path 'go.mod'
    if (-not (Test-Path -LiteralPath $module -PathType Leaf)) { return $false }
    return [bool]((Get-Content -LiteralPath $module | Select-Object -First 1) -match
        '^module github\.com/arturonavax/anx\s*$' -and
        (Test-Path -LiteralPath (Join-Path $path 'cmd/anx/main.go') -PathType Leaf))
}

function Get-AnxSource([string]$scratch) {
    if (Test-AnxSource $localSource) { return $localSource }
    $repo = if ($env:ANX_REPO_DIR) { $env:ANX_REPO_DIR } else { Join-Path $env:LOCALAPPDATA 'anx/engine' }
    if (Test-AnxSource $repo) { return $repo }
    if (Test-Path -LiteralPath $repo) { throw "The ANX source directory is occupied: $repo. Set ANX_REPO_DIR to an unused path." }

    if ($env:ANX_GIT_URL) {
        $source = Join-Path $scratch 'engine'
        $git = Get-Command 'git.exe' -CommandType Application -ErrorAction Stop
        & $git.Source clone --depth 1 -- $env:ANX_GIT_URL $source | Out-Host
        if ($LASTEXITCODE -ne 0) { throw "Git could not clone the ANX source (exit $LASTEXITCODE)." }
    } else {
        $zip = Join-Path $scratch 'anx-source.zip'
        $sourceURL = 'https://github.com/arturonavax/anx/archive/refs/heads/main.zip'
        Invoke-WebRequest -Uri $sourceURL -OutFile $zip -UseBasicParsing -TimeoutSec 120
        Expand-AnxZip $zip $scratch
        $source = Join-Path $scratch 'anx-main'
    }
    if (-not (Test-AnxSource $source)) { throw 'The ANX source checkout is incomplete.' }
    $repo = [IO.Path]::GetFullPath($repo)
    New-Item -ItemType Directory -Path (Split-Path -Parent $repo) -Force | Out-Null
    Move-Item -LiteralPath $source -Destination $repo
    return $repo
}

function Try-DownloadReleaseBinary {
    if ($FromSource) { return $false }

    $archiveName = "anx_windows_$AnxArchitecture.zip"
    $releaseUrl = "https://github.com/arturonavax/anx/releases/latest/download/$archiveName"
    $sumsUrl = "https://github.com/arturonavax/anx/releases/latest/download/SHA256SUMS"

    Write-Step "Checking for verified pre-compiled ANX release ($archiveName)..."
    $tempDir = $null
    $deploying = $false
    try {
        $tempDir = Join-Path $env:TEMP ("anx-release-" + [System.Guid]::NewGuid().ToString("N"))
        New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
        $zipPath = Join-Path $tempDir $archiveName
        $sumsPath = Join-Path $tempDir "SHA256SUMS"

        Invoke-WebRequest -Uri $releaseUrl -OutFile $zipPath -UseBasicParsing -TimeoutSec 30
        Invoke-WebRequest -Uri $sumsUrl -OutFile $sumsPath -UseBasicParsing -TimeoutSec 15

        $expectedHash = $null
        foreach ($line in (Get-Content -LiteralPath $sumsPath)) {
            $fields = $line.Trim() -split '\s+'
            if ($fields.Count -lt 2 -or ($fields[1] -cne $archiveName -and $fields[1] -cne "*$archiveName")) {
                continue
            }
            if ($expectedHash -or $fields.Count -ne 2 -or $fields[0] -notmatch '^[a-fA-F0-9]{64}$') {
                Write-WarnMsg "Invalid or ambiguous checksum for $archiveName."
                return $false
            }
            $expectedHash = $fields[0]
        }
        if (-not $expectedHash) {
            Write-WarnMsg "Missing checksum for $archiveName."
            return $false
        }

        $actualHash = Get-AnxSHA256 $zipPath
        if ($actualHash -ne $expectedHash) {
            Write-WarnMsg "Release checksum mismatch for $archiveName."
            return $false
        }
        Write-Success "Release checksum verified: $actualHash"

        $extractDir = Join-Path $tempDir "extract"
        Expand-AnxZip $zipPath $extractDir
        $anxStaged = Join-Path $extractDir "anx.exe"
        if (Test-Path -LiteralPath $anxStaged -PathType Leaf) {
            $deploying = $true
            Install-AnxBinary $anxStaged
            return $true
        }
    } catch {
        if ($deploying) { throw }
        Write-WarnMsg "Could not verify $archiveName; trying a user-scope source build."
    } finally {
        if ($tempDir) {
            Remove-Item -LiteralPath $tempDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
    return $false
}


Write-Step 'Installing ANX for the current user...'
$installed = $false
if (-not $GoInstall -and -not $FromSource) { $installed = Try-DownloadReleaseBinary }

if (-not $installed) {
    $scratch = Join-Path $env:TEMP ("anx-build-" + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $scratch | Out-Null
    $previousGobin = $env:GOBIN
    $previousGoarch = $env:GOARCH
    $previousGoos = $env:GOOS
    $previousCgo = $env:CGO_ENABLED
    try {
        $go = Get-UsableGo
        if (-not $go) { $go = Install-PortableGo $scratch }
        $env:GOOS = 'windows'
        $env:GOARCH = $AnxArchitecture
        $env:CGO_ENABLED = '0'
        $binary = Join-Path $scratch 'anx.exe'
        if ($GoInstall) {
            $env:GOBIN = $scratch
            & $go install -trimpath github.com/arturonavax/anx/cmd/anx@latest
            if ($LASTEXITCODE -ne 0) { throw "go install failed (exit $LASTEXITCODE)." }
        } else {
            $repoRoot = Get-AnxSource $scratch
            Push-Location $repoRoot
            try {
                $ldflags = & $go run ./internal/buildversion/cmd -ldflags
                if ($LASTEXITCODE -ne 0 -or -not $ldflags) { throw 'Could not resolve ANX build version.' }
                & $go build -ldflags "$ldflags" -o $binary ./cmd/anx
                if ($LASTEXITCODE -ne 0) { throw "ANX source build failed (exit $LASTEXITCODE)." }
            } finally {
                Pop-Location
            }
        }
        Install-AnxBinary $binary
    } finally {
        $env:GOBIN = $previousGobin
        $env:GOARCH = $previousGoarch
        $env:GOOS = $previousGoos
        $env:CGO_ENABLED = $previousCgo
        Remove-Item -LiteralPath $scratch -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Add-AnxPath $DestDir
if ($ForwardArgs.Count -gt 0) {
    $previousInstallEnv = $env:ANX_INSTALL_ENV
    try {
        $env:ANX_INSTALL_ENV = '1'
        & (Join-Path $DestDir 'anx.exe') @ForwardArgs
        if ($LASTEXITCODE -ne 0) { throw "ANX exited with code $LASTEXITCODE." }
    } finally {
        $env:ANX_INSTALL_ENV = $previousInstallEnv
    }
} else {
    Write-Success 'Open a new terminal to run anx version or anx wizard.'
}
