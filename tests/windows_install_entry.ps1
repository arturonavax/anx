param(
    [Parameter(Mandatory = $true)][string]$InstallerPath,
    [Parameter(Mandatory = $true)][string]$ScratchRoot,
    [Parameter(Mandatory = $true)][string]$FixtureExecutable,
    [switch]$SourceFallback
)

$ErrorActionPreference = 'Stop'
$env:LOCALAPPDATA = $ScratchRoot
$env:TEMP = $ScratchRoot
$env:PROCESSOR_ARCHITECTURE = 'AMD64'
$env:ANX_REPO_DIR = Join-Path $ScratchRoot 'legacy-engine'
$env:ANX_GIT_URL = $null
$script:gitCalls = 0
$script:downloads = @()
$script:originalPolicy = Get-ExecutionPolicy
$testArchitecture = if ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') { 'arm64' } else { 'amd64' }
$releaseName = "anx_windows_$testArchitecture.zip"
$goName = "go1.27.3.windows-$testArchitecture.zip"

$oldScripts = Join-Path $env:ANX_REPO_DIR 'scripts'
New-Item -ItemType Directory -Path $oldScripts | Out-Null
New-Item -ItemType Directory -Path (Join-Path $env:ANX_REPO_DIR '.git') | Out-Null
[IO.File]::WriteAllText((Join-Path $oldScripts 'bootstrap_anx.ps1'),
    "throw 'PSSecurityException: running scripts is disabled on this system.'")
if ($SourceFallback) { $env:ANX_REPO_DIR = Join-Path $ScratchRoot 'fresh-engine' }
if (-not ('System.IO.Compression.ZipFile' -as [type])) {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
}
$releaseTree = Join-Path $ScratchRoot 'fixture-release'
New-Item -ItemType Directory -Path $releaseTree | Out-Null
Copy-Item -LiteralPath $FixtureExecutable -Destination (Join-Path $releaseTree 'anx.exe')
$archive = Join-Path $ScratchRoot 'fixture-release.zip'
[IO.Compression.ZipFile]::CreateFromDirectory($releaseTree, $archive)
$digest = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash

if ($SourceFallback) {
    $goTree = Join-Path $ScratchRoot 'fixture-go'
    $goBin = Join-Path $goTree 'go/bin'
    New-Item -ItemType Directory -Path $goBin -Force | Out-Null
    Copy-Item -LiteralPath $FixtureExecutable -Destination (Join-Path $goBin 'go.exe')
    $goArchive = Join-Path $ScratchRoot 'fixture-go.zip'
    [IO.Compression.ZipFile]::CreateFromDirectory($goTree, $goArchive)
    $goDigest = (Get-FileHash -LiteralPath $goArchive -Algorithm SHA256).Hash

    $sourceTree = Join-Path $ScratchRoot 'fixture-source'
    $source = Join-Path $sourceTree 'anx-main'
    New-Item -ItemType Directory -Path (Join-Path $source 'cmd/anx') -Force | Out-Null
    [IO.File]::WriteAllText((Join-Path $source 'go.mod'), 'module github.com/arturonavax/anx')
    [IO.File]::WriteAllText((Join-Path $source 'cmd/anx/main.go'), 'fixture')
    $sourceArchive = Join-Path $ScratchRoot 'fixture-source.zip'
    [IO.Compression.ZipFile]::CreateFromDirectory($sourceTree, $sourceArchive)
}

function git {
    $script:gitCalls++
    Write-Output 'Already up to date.'
}

function Invoke-RestMethod {
    param([string]$Uri, [switch]$UseBasicParsing, [int]$TimeoutSec)
    switch ($Uri) {
        'https://sh.arturonavax.dev/install.ps1' { [IO.File]::ReadAllText($InstallerPath) }
        'https://go.dev/dl/?mode=json' {
            if (-not $SourceFallback) { throw 'Unexpected Go metadata request' }
            [pscustomobject]@{
                stable = $true
                version = 'go1.27.3'
                files = @([pscustomobject]@{
                    os = 'windows'; arch = $testArchitecture; kind = 'archive'; filename = $goName; sha256 = $goDigest
                })
            }
        }
        default { throw "Unexpected request: $Uri" }
    }
}

function Invoke-WebRequest {
    param([string]$Uri, [string]$OutFile, [switch]$UseBasicParsing, [int]$TimeoutSec)
    $script:downloads += $Uri
    switch -Wildcard ($Uri) {
        '*/SHA256SUMS' { [IO.File]::WriteAllText($OutFile, "$digest *$releaseName") }
        "*/$releaseName" {
            if ($SourceFallback) { throw 'Fixture: no precompiled release available.' }
            Copy-Item -LiteralPath $archive -Destination $OutFile
        }
        "*/$goName" { Copy-Item -LiteralPath $goArchive -Destination $OutFile }
        '*/main.zip' { Copy-Item -LiteralPath $sourceArchive -Destination $OutFile }
        default { throw "Unexpected download: $Uri" }
    }
}

function Expand-Archive { throw 'Script-based archive module must not be called.' }

$dest = Join-Path $ScratchRoot 'Programs/anx/bin'
if (-not $SourceFallback) {
    New-Item -ItemType Directory -Path $dest -Force | Out-Null
    [IO.File]::WriteAllText((Join-Path $dest 'anx.exe'), 'previous installation')
}
Push-Location $ScratchRoot
try {
    Invoke-RestMethod https://sh.arturonavax.dev/install.ps1 | Invoke-Expression
} finally {
    Pop-Location
}

foreach ($name in @('anx.exe', 'safe-suspend.exe')) {
    if (-not (Test-Path -LiteralPath (Join-Path $dest $name) -PathType Leaf)) {
        throw "Missing installed binary $name"
    }
    if ((Get-FileHash -LiteralPath (Join-Path $dest $name)).Hash -ne
        (Get-FileHash -LiteralPath $FixtureExecutable).Hash) {
        throw "Installed binary $name does not match the fixture."
    }
}
if ($SourceFallback) {
    if (-not (Test-Path -LiteralPath (Join-Path $env:ANX_REPO_DIR 'go.mod')) -or
        -not (Test-Path -LiteralPath (Join-Path $ScratchRoot "anx/toolchains/go1.27.3.windows-$testArchitecture/bin/go.exe"))) {
        throw 'Source fallback did not use the expected per-user paths.'
    }
} elseif ($script:gitCalls -ne 0 -or $script:downloads.Count -ne 2) {
    throw 'The verified release path must not fetch or update engine source.'
}
if ((Get-ExecutionPolicy) -ne $script:originalPolicy) { throw 'PowerShell execution policy changed.' }
if ($env:Path.Split([IO.Path]::PathSeparator) -notcontains $dest) { throw 'Process PATH not updated.' }
