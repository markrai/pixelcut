function Get-CIFailureExcerpt {
    param([Parameter(Mandatory = $true)][string]$Log)

    $lines = [System.IO.File]::ReadAllLines($Log, [System.Text.Encoding]::UTF8)
    $testFailures = [System.Collections.Generic.List[int]]::new()
    $packageFailures = [System.Collections.Generic.List[int]]::new()
    $otherFailures = [System.Collections.Generic.List[int]]::new()
    for ($i = 0; $i -lt $lines.Length; $i++) {
        $line = $lines[$i]
        if ($line -match '^\s*--- FAIL:\s') {
            $testFailures.Add($i)
        } elseif ($line -match '^FAIL(?:\s|$)') {
            $packageFailures.Add($i)
        } elseif ($line -match '^(?:panic:|fatal error:|WARNING: DATA RACE|#\s+\S|(?:[A-Za-z]:)?[^\s].*\.go:\d+:\d+:|go:\s|(?:error|fatal):)') {
            $otherFailures.Add($i)
        }
    }

    $selected = [System.Collections.Generic.SortedSet[int]]::new()
    foreach ($i in $testFailures) {
        if ($selected.Count -ge 24) { break }
        $null = $selected.Add($i)
        for ($j = $i - 1; $j -ge [Math]::Max(0, $i - 5); $j--) {
            if ($selected.Count -ge 24) { break }
            if ($lines[$j] -match '^\s*(?:=== (?:RUN|PAUSE|CONT|NAME)|--- (?:PASS|SKIP|FAIL):)|^(?:ok\s|PASS$|FAIL(?:\s|$))') { break }
            if ($lines[$j].Trim().Length -gt 0) { $null = $selected.Add($j) }
        }
    }
    foreach ($i in $otherFailures) {
        if ($selected.Count -ge 30) { break }
        $null = $selected.Add($i)
        if ($lines[$i] -match '^(?:panic:|fatal error:|WARNING: DATA RACE|#\s)') {
            for ($j = $i + 1; $j -lt [Math]::Min($lines.Length, $i + 5); $j++) {
                if ($selected.Count -ge 30) { break }
                if ($lines[$j] -match '^\s*(?:=== (?:RUN|PAUSE|CONT|NAME)|--- (?:PASS|SKIP|FAIL):)|^(?:ok\s|PASS$|FAIL(?:\s|$))') { break }
                if ($lines[$j].Trim().Length -gt 0) { $null = $selected.Add($j) }
            }
        }
    }
    foreach ($i in $packageFailures) {
        if ($selected.Count -ge 36) { break }
        $null = $selected.Add($i)
    }
    if ($selected.Count -eq 0) {
        'No explicit failure marker found; last nonblank output lines:'
        for ($i = $lines.Length - 1; $i -ge 0 -and $selected.Count -lt 8; $i--) {
            if ($lines[$i].Trim().Length -gt 0) { $null = $selected.Add($i) }
        }
    }
    foreach ($i in $selected) {
        $line = $lines[$i]
        if ($line.Length -gt 400) { $line = $line.Substring(0, 400) + ' ...' }
        $line
    }
    $omitted = @($testFailures | Where-Object { -not $selected.Contains($_) }).Count
    $omitted += @($packageFailures | Where-Object { -not $selected.Contains($_) }).Count
    $omitted += @($otherFailures | Where-Object { -not $selected.Contains($_) }).Count
    if ($omitted -gt 0) {
        "$omitted additional failure markers omitted; see full log."
    }
}
