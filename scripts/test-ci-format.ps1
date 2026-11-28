$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$work = Join-Path ([System.IO.Path]::GetTempPath()) ("pixelcut-ci-format-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path (Join-Path $work 'scripts') -Force | Out-Null
$utf8 = [System.Text.UTF8Encoding]::new($false)

function Run-FormatCheck {
    param([int]$Expected)
    $previous = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $lines = @(& cmd.exe /d /c 'ci.bat -FormatOnly' 2>&1)
        $code = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previous
    }
    $output = ($lines | ForEach-Object { $_.ToString() }) -join "`n"
    Write-Host $output
    if ($code -ne $Expected) {
        throw "Expected exit $Expected, got $code"
    }
    return $output
}

function Latest-FormatReport {
    $directories = @(Get-ChildItem -LiteralPath 'ci-reports' -Directory)
    if ($directories.Count -ne 1) {
        throw "Expected one report directory, got $($directories.Count)"
    }
    return Get-Content -LiteralPath (Join-Path $directories[0].FullName 'formatting.txt') -Raw
}

function Assert-Contains {
    param([string]$Actual, [string]$Expected)
    if (-not $Actual.Contains($Expected)) {
        throw "Missing diagnostic: $Expected"
    }
}

Push-Location -LiteralPath $work
try {
    Copy-Item -LiteralPath (Join-Path $root 'ci.bat') -Destination $work
    Copy-Item -LiteralPath (Join-Path $root 'scripts/ci.ps1') -Destination (Join-Path $work 'scripts')
    & git init -q
    if ($LASTEXITCODE -ne 0) { throw 'git init failed' }
    [System.IO.File]::WriteAllText((Join-Path $work '.gitignore'), "/ci-reports/" + [Environment]::NewLine, $utf8)
    $source = "package fixture" + [Environment]::NewLine + "func bad(){println(1)}" + [Environment]::NewLine
    $tracked = Join-Path $work 'tracked.go'
    $untracked = Join-Path $work 'untracked space.go'
    [System.IO.File]::WriteAllText($tracked, $source, $utf8)
    [System.IO.File]::WriteAllText($untracked, $source, $utf8)
    & git add -- tracked.go
    if ($LASTEXITCODE -ne 0) { throw 'git add failed' }

    $output = Run-FormatCheck 1
    $report = Latest-FormatReport
    foreach ($value in @('Checked 2 Go files; 2 require formatting.', 'gofmt -w "tracked.go"', 'gofmt -w "untracked space.go"', 'Formatting differences (read-only):', 'func bad() { println(1) }')) {
        Assert-Contains $output $value
        Assert-Contains $report $value
    }
    Assert-Contains $output 'LOCAL CI FAILED: Check Go formatting'
    if ([System.IO.File]::ReadAllText($tracked) -ne $source -or [System.IO.File]::ReadAllText($untracked) -ne $source) {
        throw 'Formatting check modified source files'
    }

    Remove-Item -LiteralPath 'ci-reports' -Recurse -Force
    & gofmt -w tracked.go 'untracked space.go'
    if ($LASTEXITCODE -ne 0) { throw 'Fixture formatting failed' }
    $output = Run-FormatCheck 0
    Assert-Contains $output 'LOCAL CI PASSED'
    Assert-Contains (Latest-FormatReport) 'Checked 2 Go files; 0 require formatting.'

    Remove-Item -LiteralPath 'ci-reports' -Recurse -Force
    [System.IO.File]::WriteAllText($tracked, "package fixture`nfunc broken(", $utf8)
    $output = Run-FormatCheck 1
    Assert-Contains $output 'gofmt scan failed'
    Assert-Contains $output 'tracked.go:2:'
    Assert-Contains (Latest-FormatReport) 'tracked.go:2:'
    Write-Host 'Local CI formatting diagnostics regression passed.'
} finally {
    Pop-Location
    Remove-Item -LiteralPath $work -Recurse -Force
}
