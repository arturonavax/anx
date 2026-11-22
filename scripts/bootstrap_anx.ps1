# Compatibility entry point for callers using the local bootstrap script.
[CmdletBinding()]
param(
    [string]$BinName = $env:ANX_BIN_NAME,
    [switch]$FromSource,
    [switch]$GoInstall,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$ForwardArgs
)

$ErrorActionPreference = 'Stop'
$installer = Join-Path (Split-Path -Parent $PSScriptRoot) 'install.ps1'
& $installer -BinName $BinName -FromSource:$FromSource -GoInstall:$GoInstall -ForwardArgs $ForwardArgs
