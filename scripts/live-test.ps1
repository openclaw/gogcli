#!/usr/bin/env pwsh
$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$bashScript = Join-Path $scriptDir "live-test.sh"

if (Get-Command bash -ErrorAction SilentlyContinue) {
  & bash $bashScript @args
  exit $LASTEXITCODE
}

throw "bash is required to run scripts/live-test.sh on Windows"
