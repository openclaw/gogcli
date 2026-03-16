param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$Args
)

$ErrorActionPreference = "Stop"

$scriptPath = Join-Path $PSScriptRoot "live-test.sh"
if (-not (Test-Path $scriptPath)) {
    Write-Error "Could not find live-test.sh at $scriptPath"
    exit 1
}

$bashCandidates = @()

$bashCommand = Get-Command bash -ErrorAction SilentlyContinue
if ($bashCommand) {
    $bashCandidates += $bashCommand.Source
}

$gitBash = "C:\Program Files\Git\bin\bash.exe"
if (Test-Path $gitBash) {
    $bashCandidates += $gitBash
}

$bashCandidates = $bashCandidates | Select-Object -Unique

if ($bashCandidates.Count -eq 0) {
    Write-Error "No bash executable found. Install Git for Windows (Git Bash) or run scripts/live-test.sh in WSL."
    exit 1
}

$bash = $bashCandidates[0]
& $bash $scriptPath @Args
$exitCode = $LASTEXITCODE
if ($null -eq $exitCode) {
    $exitCode = 0
}
exit $exitCode
