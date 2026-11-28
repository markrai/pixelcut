$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'ci-diagnostics.ps1')
$work = Join-Path ([System.IO.Path]::GetTempPath()) ("pixelcut-ci-diagnostics-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
$log = Join-Path $work 'test.txt'

function Read-Excerpt {
    param([string[]]$Lines)
    [System.IO.File]::WriteAllLines($log, $Lines, [System.Text.UTF8Encoding]::new($false))
    return @(Get-CIFailureExcerpt -Log $log)
}

function Assert-Contains {
    param([string]$Actual, [string]$Expected)
    if (-not $Actual.Contains($Expected)) { throw "Missing diagnostic: $Expected" }
}

try {
    $lines = @()
    for ($i = 0; $i -lt 30; $i++) {
        $lines += @("=== RUN   TestPass$i", "    passing_test.go:42: noise $i", "--- PASS: TestPass$i (0.00s)")
    }
    $lines += @('=== RUN   TestSkip', '    skipped_test.go:16: shell fixture requires Unix', '--- SKIP: TestSkip (0.00s)', '=== RUN   TestActualFailure', '    actual_test.go:99: expected 3, got 4', '--- FAIL: TestActualFailure (0.02s)', 'FAIL', 'FAIL github.com/markrai/pixelcut/internal/media 0.01s', '=== RUN   TestLaterPass', '    later_test.go:42: another successful diagnostic', '--- PASS: TestLaterPass (0.00s)', 'ok github.com/markrai/pixelcut/internal/project 0.01s', 'FAIL')
    $excerpt = (Read-Excerpt $lines) -join [Environment]::NewLine
    Assert-Contains $excerpt '--- FAIL: TestActualFailure'
    Assert-Contains $excerpt 'actual_test.go:99: expected 3, got 4'
    Assert-Contains $excerpt 'github.com/markrai/pixelcut/internal/media'
    foreach ($noise in @('passing_test.go', 'skipped_test.go', 'later_test.go')) {
        if ($excerpt.Contains($noise)) { throw "Unrelated log leaked: $noise" }
    }
    $excerpt = (Read-Excerpt @('# example.test', 'internal/media/example.go:42:17: undefined: broken', 'FAIL example.test [build failed]')) -join [Environment]::NewLine
    Assert-Contains $excerpt 'undefined: broken'
    Assert-Contains $excerpt '[build failed]'
    $excerpt = (Read-Excerpt @('=== RUN   TestRace', 'WARNING: DATA RACE', 'Read at 0x123', '--- FAIL: TestRace (0.01s)', 'FAIL')) -join [Environment]::NewLine
    Assert-Contains $excerpt 'WARNING: DATA RACE'
    $excerpt = (Read-Excerpt @('ordinary tool output', 'unexpected final message')) -join [Environment]::NewLine
    Assert-Contains $excerpt 'No explicit failure marker found'
    Assert-Contains $excerpt 'unexpected final message'
    $lines = @(1..100 | ForEach-Object { "--- FAIL: TestFailure$_ (0.01s)" })
    $excerpt = @(Read-Excerpt $lines)
    if ($excerpt.Count -gt 37) { throw 'Excerpt is not bounded' }
    Assert-Contains ($excerpt -join ' ') 'additional failure markers omitted'
    $VerboseOutput = $false
    $script:failureExcerpt = @()
    $nativeLog = Join-Path $work 'native-command.txt'
    $fixture = Join-Path $work 'fail.cmd'
    [System.IO.File]::WriteAllLines($fixture, @('@echo off', 'echo === RUN   TestNativeFailure', 'echo     native_test.go:42: expected 1, got 2', 'echo --- FAIL: TestNativeFailure', 'echo FAIL example.test', 'exit /b 7'), [System.Text.Encoding]::ASCII)
    Push-Location -LiteralPath $work
    try {
        $caught = $null
        try {
            Invoke-Logged cmd.exe @('/d', '/c', 'fail.cmd') $nativeLog
        } catch {
            $caught = $_.Exception.Message
        }
        Assert-Contains $caught 'cmd.exe exited with code 7.'
        $excerpt = $script:failureExcerpt -join [Environment]::NewLine
        Assert-Contains $excerpt '--- FAIL: TestNativeFailure'
        Assert-Contains $excerpt 'native_test.go:42: expected 1, got 2'
        Assert-Contains ([System.IO.File]::ReadAllText($nativeLog)) 'FAIL example.test'
        $handle = [System.IO.File]::Open($nativeLog, [System.IO.FileMode]::Open, [System.IO.FileAccess]::ReadWrite, [System.IO.FileShare]::None)
        $handle.Dispose()

        function Get-CIFailureExcerpt {
            param([string]$Log)
            throw 'Injected diagnostic failure'
        }
        $caught = $null
        try {
            Invoke-Logged cmd.exe @('/d', '/c', 'fail.cmd') $nativeLog
        } catch {
            $caught = $_.Exception.Message
        }
        Assert-Contains $caught 'cmd.exe exited with code 7.'
        Assert-Contains ($script:failureExcerpt -join [Environment]::NewLine) 'Injected diagnostic failure'
        Assert-Contains ([System.IO.File]::ReadAllText($nativeLog)) 'FAIL example.test'
    } finally {
        Pop-Location
    }
    Write-Host 'CI failure excerpt and native logging regression passed.'
} finally {
    Remove-Item -LiteralPath $work -Recurse -Force
}
exit 0
