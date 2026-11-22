param(
    [Parameter(Mandatory = $true)][string]$BootstrapPath,
    [Parameter(Mandatory = $true)][string]$ScratchRoot
)

$ErrorActionPreference = 'Stop'
$env:TEMP = $ScratchRoot
$env:PROCESSOR_ARCHITECTURE = 'AMD64'
$AnxArchitecture = 'amd64'
$DestDir = Join-Path $ScratchRoot 'bin'
New-Item -ItemType Directory -Path $DestDir | Out-Null
$existingBinary = Join-Path $DestDir 'anx.exe'
[IO.File]::WriteAllText($existingBinary, 'existing installation')

# Load only the release function. Dot-sourcing the installer itself would run
# its main dispatch, including toolchain installation and user PATH updates.
$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile(
    $BootstrapPath, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count -gt 0) { throw ($parseErrors -join [Environment]::NewLine) }
$definition = $ast.Find({
    param($node)
    $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
        $node.Name -eq 'Try-DownloadReleaseBinary'
}, $false)
if (-not $definition) { throw 'Windows release bootstrap function not found.' }
$releaseBootstrap = $definition.Body.GetScriptBlock()

function Write-Step { }
function Write-WarnMsg { }
function Write-Success { }

$archiveName = 'anx_windows_amd64.zip'
$fixture = Join-Path $ScratchRoot 'fixture.zip'
[IO.File]::WriteAllText($fixture, 'deterministic release fixture')
$digest = (Get-FileHash -LiteralPath $fixture -Algorithm SHA256).Hash

function Invoke-WebRequest {
    param([string]$Uri, [string]$OutFile, [switch]$UseBasicParsing, [int]$TimeoutSec)
    switch -Wildcard ($Uri) {
        '*/SHA256SUMS' { [IO.File]::WriteAllLines($OutFile, $script:manifestLines) }
        "*/$archiveName" { Copy-Item -LiteralPath $fixture -Destination $OutFile }
        default { throw "Unexpected download in installer test: $Uri" }
    }
}

function Get-AnxSHA256([string]$path) {
    (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash
}

function Expand-AnxZip([string]$archive, [string]$destination) {
    $script:extractCount++
    # Stop at the extraction boundary; never stage or run an executable.
}

$cases = @(
    @{ Name = 'valid'; Lines = @("$digest *$archiveName"); Extract = 1 }
    @{ Name = 'wrong filename'; Lines = @("$digest other-$archiveName"); Extract = 0 }
    @{ Name = 'missing'; Lines = @(); Extract = 0 }
    @{ Name = 'malformed'; Lines = @("not-a-checksum $archiveName"); Extract = 0 }
    @{ Name = 'duplicate'; Lines = @("$digest $archiveName", "$digest $archiveName"); Extract = 0 }
    @{ Name = 'mismatch'; Lines = @(('0' * 64) + " $archiveName"); Extract = 0 }
)
foreach ($case in $cases) {
    $script:manifestLines = [string[]]$case.Lines
    $script:extractCount = 0
    $result = & $releaseBootstrap
    if ($result -ne $false -or $script:extractCount -ne $case.Extract) {
        throw "$($case.Name): unexpected install result or extraction count ($script:extractCount)."
    }
    if ([IO.File]::ReadAllText($existingBinary) -ne 'existing installation') {
        throw "$($case.Name): changed the existing installation."
    }
    if (Get-ChildItem -LiteralPath $ScratchRoot -Directory -Filter 'anx-release-*') {
        throw "$($case.Name): left release staging files behind."
    }
}
