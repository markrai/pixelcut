[CmdletBinding()]
param(
    [switch]$FormatOnly,
    [switch]$Paired,
    [switch]$VerboseOutput
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$script:stage = 'Initialization'
$script:results = [ordered]@{}
$script:details = [ordered]@{}
$script:failureExcerpt = @()
$script:stageStarted = $false

function Require-Tool {
    param([string]$Name)
    $command = Get-Command $Name -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -eq $command) {
        throw "Required tool not found on PATH: $Name"
    }
    return $command.Source
}

function Invoke-Logged {
    param(
        [string]$Tool,
        [string[]]$Arguments,
        [string]$Log,
        [switch]$Append,
        [switch]$ReportOnly
    )
    $exe = Require-Tool $Tool
    $writer = [System.IO.StreamWriter]::new($Log, [bool]$Append, [System.Text.UTF8Encoding]::new($false))
    $previous = $ErrorActionPreference
    $tail = [System.Collections.Generic.Queue[string]]::new()
    $errors = [System.Collections.Generic.List[string]]::new()
    try {
        $ErrorActionPreference = 'Continue'
        & $exe @Arguments 2>&1 | ForEach-Object {
            $line = $_.ToString()
            $writer.WriteLine($line)
            if ($VerboseOutput -and -not $ReportOnly) {
                Write-Host $line
            }
            if ($tail.Count -ge 8) { $null = $tail.Dequeue() }
            $short = if ($line.Length -gt 400) { $line.Substring(0, 400) + ' ...' } else { $line }
            $tail.Enqueue($short)
            if ($errors.Count -lt 12 -and $line -match '(^\s*--- FAIL:|^FAIL\b|^#\s|\.go:\d+:|\b(error|fatal|panic):|WARNING: DATA RACE)') {
                $errors.Add($short)
            }
        }
        $code = $LASTEXITCODE
        if ($code -ne 0) {
            if (-not $ReportOnly -and -not $VerboseOutput) {
                $script:failureExcerpt = if ($errors.Count -gt 0) { @($errors.ToArray()) } else { @($tail.ToArray()) }
            }
            throw "$Tool exited with code $code. See $Log"
        }
    } finally {
        $ErrorActionPreference = $previous
        $writer.Dispose()
    }
}

function Invoke-Checked {
    param([string]$Tool, [string[]]$Arguments)
    $exe = Require-Tool $Tool
    & $exe @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Tool exited with code $LASTEXITCODE"
    }
}

function Complete-Stage {
    if ($script:stageStarted) {
        $script:results[$script:stage] = 'PASS'
        $script:stageStarted = $false
        Write-Host "PASS: $script:stage" -ForegroundColor Green
    }
}

function Begin-Stage {
    param([string]$Name)
    Complete-Stage
    $script:stage = $Name
    $script:stageStarted = $true
    $script:results[$Name] = 'RUNNING'
    Write-Host "RUN:  $Name"
}

function Write-FormatLine {
    param([string]$Log, [string]$Message, [switch]$ReportOnly)
    [System.IO.File]::AppendAllText($Log, $Message + [Environment]::NewLine, [System.Text.UTF8Encoding]::new($false))
    if (-not $ReportOnly) { Write-Host $Message }
}

function Scan-FormatChunk {
    param([string[]]$Files, [string]$Log)
    $exe = Require-Tool gofmt
    $previous = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $lines = @(& $exe -l @Files 2>&1)
        $code = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previous
    }
    if ($code -ne 0) {
        Write-FormatLine $Log "gofmt could not parse or read source files (exit $code):"
        foreach ($line in $lines) {
            Write-FormatLine $Log ($line.ToString()) -ReportOnly
        }
        $script:failureExcerpt = @($lines | Select-Object -First 12 | ForEach-Object { $_.ToString() })
        throw "gofmt scan failed (exit $code). See $Log"
    }
    foreach ($line in $lines) {
        if (-not [string]::IsNullOrWhiteSpace($line.ToString())) {
            $line.ToString()
        }
    }
}

function Check-Formatting {
    param([string]$Reports)
    Begin-Stage 'Check Go formatting'
    $log = Join-Path $Reports 'formatting.txt'
    $files = @(Invoke-Checked git @('-c', 'core.quotepath=false', 'ls-files', '--cached', '--others', '--exclude-standard', '--', '*.go') | Sort-Object -Unique)
    $bad = @()
    $chunk = @()
    $checked = 0
    foreach ($file in $files) {
        if ($file.StartsWith('"')) {
            throw "Unsupported quoted Git path: $file"
        }
        if (-not (Test-Path -LiteralPath $file -PathType Leaf)) {
            continue
        }
        $checked++
        $chunk += $file
        if ($chunk.Count -ge 32) {
            $bad += @(Scan-FormatChunk $chunk $log)
            $chunk = @()
        }
    }
    if ($chunk.Count -gt 0) {
        $bad += @(Scan-FormatChunk $chunk $log)
    }
    $crlfOnly = 0
    $realBad = [System.Collections.Generic.List[string]]::new()
    $utf8 = [System.Text.UTF8Encoding]::new($false, $true)
    $temp = Join-Path $Reports 'format-check.go.tmp'
    try {
        foreach ($file in $bad) {
            $path = (Resolve-Path -LiteralPath $file).ProviderPath
            $source = $utf8.GetString([System.IO.File]::ReadAllBytes($path))
            $normalized = $source.Replace("`r`n", "`n")
            $target = $file
            if ($normalized -cne $source) {
                [System.IO.File]::WriteAllText($temp, $normalized, $utf8)
                $remaining = @(Scan-FormatChunk @($temp) $log)
                if ($remaining.Count -eq 0) {
                    $crlfOnly++
                    Write-FormatLine $log "CRLF only (ignored): $file" -ReportOnly
                    continue
                }
                $target = $temp
            }
            $realBad.Add($file)
            Write-FormatLine $log "=== Formatting diff: $file ===" -ReportOnly
            Invoke-Logged gofmt @('-d', $target) $log -Append -ReportOnly
        }
    } finally {
        if (Test-Path -LiteralPath $temp) {
            Remove-Item -LiteralPath $temp -Force
        }
    }
    $detail = "Checked $checked Go files; $($realBad.Count) require formatting; $crlfOnly CRLF-only differences ignored."
    $script:details[$script:stage] = $detail
    $script:formatFailures = @($realBad | Select-Object -First 10)
    $script:formatFailureCount = $realBad.Count
    Write-FormatLine $log $detail -ReportOnly
    if ($realBad.Count -gt 0) {
        Write-FormatLine $log 'Files requiring formatting:' -ReportOnly
        foreach ($file in $realBad) {
            Write-FormatLine $log ('  gofmt -w "{0}"' -f $file) -ReportOnly
        }
        throw "Go formatting check failed: $($realBad.Count) file(s). No files were modified."
    }
    Write-FormatLine $log 'Go formatting passed.' -ReportOnly
}

function Run-Benchmark {
    param([string]$Package, [string]$Pattern, [string]$Log)
    Invoke-Logged go @('test', $Package, '-run', '^$', '-bench', $Pattern, '-benchmem', '-benchtime=200ms', '-count=3') $Log
}

function Run-Paired {
    param([string]$Python, [string]$Reports)
    Begin-Stage 'Benchmark report validation'
    Invoke-Logged $Python @('scripts/test-compare-refactor-benchmarks.py') (Join-Path $Reports 'benchmark-validation.txt')
    Begin-Stage 'Paired benchmark qualification'
    $baseline = 'eff81c55b65ffee528bd7ad636b3fb2937cbe642'
    $candidate = Invoke-Checked git @('rev-parse', 'HEAD')
    Invoke-Checked git @('cat-file', '-e', "$baseline^{commit}")
    Invoke-Checked git @('diff', '--exit-code', $baseline, $candidate, '--', 'internal/project/baseline_test.go')
    $work = Join-Path ([System.IO.Path]::GetTempPath()) ("pixelcut-ci-" + [guid]::NewGuid().ToString('N'))
    $created = $false
    $oldProcs = $env:GOMAXPROCS
    $oldCGO = $env:CGO_ENABLED
    try {
        Invoke-Checked git @('worktree', 'add', '--detach', $work, $baseline)
        $created = $true
        $env:GOMAXPROCS = '2'
        $env:CGO_ENABLED = '0'
        $pairLog = Join-Path $Reports 'paired-environment.txt'
        "baseline=$baseline" | Set-Content -Encoding UTF8 $pairLog
        "candidate=$candidate" | Add-Content -Encoding UTF8 $pairLog
        Invoke-Logged go @('version') $pairLog -Append
        for ($round = 1; $round -le 5; $round++) {
            $order = @('baseline', 'candidate')
            if ($round % 2 -eq 0) {
                $order = @('candidate', 'baseline')
            }
            foreach ($label in $order) {
                $revision = if ($label -eq 'baseline') { $baseline } else { $candidate }
                Invoke-Checked git @('-C', $work, 'checkout', '--detach', $revision)
                Push-Location -LiteralPath $work
                try {
                    Invoke-Logged go @('test', './internal/project', '-run', '^$', '-bench', '^BenchmarkProjectBaseline$', '-benchmem', '-benchtime=300ms', '-count=1', '-cpu=2') (Join-Path $Reports "$label.txt") -Append
                } finally {
                    Pop-Location
                }
            }
        }
        Invoke-Logged $Python @('scripts/compare-refactor-benchmarks.py', (Join-Path $Reports 'baseline.txt'), (Join-Path $Reports 'candidate.txt')) (Join-Path $Reports 'comparison.md')
    } finally {
        $env:GOMAXPROCS = $oldProcs
        $env:CGO_ENABLED = $oldCGO
        if ($created) {
            Invoke-Checked git @('worktree', 'remove', '--force', $work)
        }
    }
}

$exitCode = 0
$reports = $null
$script:formatFailures = @()
$script:formatFailureCount = 0
$planned = @('Check Go formatting')
if (-not $FormatOnly) {
    $planned += @('Check prerequisites', 'Environment', 'Go tests and coverage (all native Windows packages)', 'Visual crop Lua tests', 'Go vet', 'Portable race checks (native Windows)', 'Project baseline benchmarks', 'Composition benchmarks', 'Composite preview allocation benchmarks', 'HLG linear-light blend benchmark', 'Windows CLI and GUI builds (CGO disabled)')
    if ($Paired) { $planned += @('Benchmark report validation', 'Paired benchmark qualification') }
}
Push-Location -LiteralPath $root
try {
    if ($FormatOnly -and $Paired) {
        throw 'Choose either -FormatOnly or -Paired.'
    }
    $null = Require-Tool git
    $null = Require-Tool go
    $null = Require-Tool gofmt
    $id = (Get-Date -Format 'yyyyMMdd-HHmmss') + '-' + [guid]::NewGuid().ToString('N').Substring(0, 8)
    $reports = Join-Path $root "ci-reports\local-$id"
    New-Item -ItemType Directory -Path $reports -Force | Out-Null
    Write-Host "Reports: $reports"
    Check-Formatting $reports
    if (-not $FormatOnly) {
        Begin-Stage 'Check prerequisites'
        $null = Require-Tool ffmpeg
        $null = Require-Tool ffprobe
        $lua = $null
        foreach ($name in @('lua5.4', 'lua54', 'lua')) {
            $found = Get-Command $name -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($null -ne $found) {
                $version = & $found.Source -v
                if ($LASTEXITCODE -eq 0 -and "$version" -match 'Lua 5\.4') {
                    $lua = $found.Source
                    break
                }
            }
        }
        if ($null -eq $lua) {
            throw 'Lua 5.4 is required (lua5.4, lua54 or lua on PATH).'
        }
        $hostOS = Invoke-Checked go @('env', 'GOHOSTOS')
        $hostArch = Invoke-Checked go @('env', 'GOHOSTARCH')
        if ($hostOS -ne 'windows' -or $hostArch -ne 'amd64') {
            throw 'Local CI requires native Windows amd64 Go.'
        }
        $env:GOOS = 'windows'
        $env:GOARCH = 'amd64'
        $env:CGO_ENABLED = '1'
        $cc = Invoke-Checked go @('env', 'CC')
        $null = Require-Tool $cc
        $python = $null
        if ($Paired) {
            $python = Require-Tool python
            if (@(Invoke-Checked git @('status', '--porcelain', '--untracked-files=normal')).Count -ne 0) {
                throw '-Paired compares committed revisions; commit or stash changes first.'
            }
        }
        Get-ChildItem Env:PIXELCUT_QUALIFY_* | ForEach-Object {
            Remove-Item -LiteralPath ("Env:" + $_.Name)
        }
        Begin-Stage 'Environment'
        $environment = Join-Path $reports 'environment.txt'
        Invoke-Logged go @('version') $environment
        Invoke-Logged go @('env', 'GOOS', 'GOARCH', 'CGO_ENABLED', 'CC') $environment -Append
        Invoke-Logged ffmpeg @('-version') $environment -Append
        Invoke-Logged ffprobe @('-version') $environment -Append
        Invoke-Logged $lua @('-v') $environment -Append
        Begin-Stage 'Go tests and coverage (all native Windows packages)'
        $coverage = Join-Path $reports 'coverage-windows.out'
        Invoke-Logged go @('test', '-count=1', '-v', "-coverprofile=$coverage", './...') (Join-Path $reports 'test-windows.txt')
        Invoke-Logged go @('tool', 'cover', "-func=$coverage") (Join-Path $reports 'coverage-windows.txt')
        Begin-Stage 'Visual crop Lua tests'
        Invoke-Logged $lua @('cmd/pixelcut-gui/crop_test.lua') (Join-Path $reports 'crop-lua.txt')
        Begin-Stage 'Go vet'
        Invoke-Logged go @('vet', './...') (Join-Path $reports 'vet-windows.txt')
        Begin-Stage 'Portable race checks (native Windows)'
        Invoke-Logged go @('test', '-race', '-count=1', './internal/...') (Join-Path $reports 'race-windows.txt')
        Begin-Stage 'Project baseline benchmarks'
        Run-Benchmark './internal/project' '^BenchmarkProjectBaseline$' (Join-Path $reports 'benchmarks-windows.txt')
        Begin-Stage 'Composition benchmarks'
        Run-Benchmark './internal/composition' '^BenchmarkComposition$' (Join-Path $reports 'benchmarks-composition.txt')
        Begin-Stage 'Composite preview allocation benchmarks'
        Run-Benchmark './internal/composition' '^BenchmarkCompositePreview' (Join-Path $reports 'benchmarks-composite-preview.txt')
        Begin-Stage 'HLG linear-light blend benchmark'
        Run-Benchmark './internal/projectexport' '^BenchmarkHLGBlend' (Join-Path $reports 'benchmarks-hlg-blend.txt')
        Begin-Stage 'Windows CLI and GUI builds (CGO disabled)'
        $env:CGO_ENABLED = '0'
        Invoke-Logged go @('build', '-o', (Join-Path $reports 'pixelcut.exe'), './cmd/pixelcut') (Join-Path $reports 'build-cli.txt')
        Invoke-Logged go @('build', '-o', (Join-Path $reports 'pixelcut-gui.exe'), './cmd/pixelcut-gui') (Join-Path $reports 'build-gui.txt')
        if ($Paired) {
            Run-Paired $python $reports
        }
    }
    Complete-Stage
} catch {
    $exitCode = 1
    $script:results[$script:stage] = 'FAIL'
    $script:details[$script:stage] = if ($script:details.Contains($script:stage)) { $script:details[$script:stage] + ' ' + $_.Exception.Message } else { $_.Exception.Message }
} finally {
    Write-Host ""
    $summary = [System.Collections.Generic.List[string]]::new()
    $summary.Add('=== LOCAL CI SUMMARY ===')
    Write-Host '=== LOCAL CI SUMMARY ==='
    if ($script:results.Contains('Initialization')) { $planned = @('Initialization') + $planned }
    foreach ($name in $planned) {
        $status = if ($script:results.Contains($name)) { $script:results[$name] } else { 'NOT RUN' }
        $color = if ($status -eq 'PASS') { 'Green' } elseif ($status -eq 'FAIL') { 'Red' } else { 'DarkGray' }
        $line = '{0,-8} {1}' -f $status, $name
        $summary.Add($line)
        Write-Host $line -ForegroundColor $color
        if ($script:details.Contains($name)) {
            $line = '         ' + $script:details[$name]
            $summary.Add($line)
            Write-Host $line
        }
    }
    if ($script:failureExcerpt.Count -gt 0) {
        $summary.Add('Failure excerpt:')
        Write-Host 'Failure excerpt:'
        foreach ($line in $script:failureExcerpt) {
            $summary.Add([string]$line)
            Write-Host $line
        }
    }
    if ($script:formatFailureCount -gt 0) {
        $summary.Add('Formatting fixes (first 10):')
        Write-Host 'Formatting fixes (first 10):'
        foreach ($file in $script:formatFailures) {
            $line = '  gofmt -w "{0}"' -f $file
            $summary.Add($line)
            Write-Host $line
        }
        if ($script:formatFailureCount -gt 10) {
            $line = "  $($script:formatFailureCount - 10) more; see formatting.txt."
            $summary.Add($line)
            Write-Host $line
        }
    }
    $verdict = if ($exitCode -eq 0) { 'LOCAL CI PASSED' } else { "LOCAL CI FAILED: $script:stage" }
    $summary.Add($verdict)
    Write-Host $verdict -ForegroundColor $(if ($exitCode -eq 0) { 'Green' } else { 'Red' })
    if ($null -ne $reports) {
        $summaryPath = Join-Path $reports 'summary.txt'
        [System.IO.File]::WriteAllLines($summaryPath, $summary.ToArray(), [System.Text.UTF8Encoding]::new($false))
        Write-Host "Summary: $summaryPath"
        Write-Host "Reports: $reports"
    }
    Pop-Location
}
exit $exitCode
