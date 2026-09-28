# Measures Jusplay on this machine and writes bench\<date>-<time>.md (not
# tracked: it names the hardware). Speed and memory in the same tables:
#
#   1. the machine
#   2. command-line cold start (process start to exit) and peak memory
#   3. the window's memory (jusplay.exe + its WebView2 processes): the
#      library idle, a video playing, and the peak of a stress run
#      (random seeks, drags, pauses, rate changes) with what it found
#
#   ./tools/bench.ps1                         the committed test video
#   ./tools/bench.ps1 -Video D:\copy\ep.mkv   a real episode (use a copy)
param([string]$Video = "", [int]$StressSeconds = 60)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot
Set-Location $root
$go = (Get-Command go -ErrorAction SilentlyContinue).Source
if (-not $go) { foreach ($p in @("C:\Applications\go\bin\go.exe", "$env:ProgramFiles\Go\bin\go.exe")) { if (Test-Path $p) { $go = $p; break } } }
if (-not $go) { throw 'go not found' }
$env:PATH = (Split-Path $go) + ";$env:PATH"

$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
New-Item -ItemType Directory -Force bench | Out-Null
$out = Join-Path $root "bench\$stamp.md"
$work = Join-Path ([IO.Path]::GetTempPath()) "jusplay-bench-$stamp"
New-Item -ItemType Directory -Force $work | Out-Null
$lines = [System.Collections.Generic.List[string]]::new()
function Say([string]$s) { $lines.Add($s); Write-Host $s }
$mb = { param($b) [math]::Round($b / 1MB, 1) }

& "$root\build.ps1" | Out-Null
$exe = "$root\bin\jusplay.exe"
$version = (Get-Content VERSION -Raw).Trim()

# 1. The machine.
$cpu = Get-CimInstance Win32_Processor | Select-Object -First 1
$cs = Get-CimInstance Win32_ComputerSystem
$os = Get-CimInstance Win32_OperatingSystem
Say "# Jusplay $version — measurements $stamp"
Say ""
Say "| machine | |"
Say "|---|---|"
Say "| CPU | $($cpu.Name.Trim()) ($($cpu.NumberOfCores) cores / $($cpu.NumberOfLogicalProcessors) threads) |"
Say "| memory | $([math]::Round($cs.TotalPhysicalMemory / 1GB, 1)) GB |"
Say "| OS | $($os.Caption) $($os.Version) |"
Say ""

# A library of its own (never the usual data folder): the test media, or a copy of -Video.
$data = Join-Path $work 'data'; $media = Join-Path $work 'media'
New-Item -ItemType Directory -Force $data, $media | Out-Null
if ($Video) { Copy-Item -LiteralPath $Video (Join-Path $media 'video.mkv') } else { Copy-Item testdata\media\*.mkv $media }
$play = (Get-ChildItem $media -Filter *.mkv | Sort-Object Length -Descending | Select-Object -First 1).FullName
$env:JUSPLAY_DATA = $data

# 2. Command-line cold start: a console build, so stdout is a pipe we can time.
$cli = Join-Path $work 'jusplay-cli.exe'
& $go build -trimpath -ldflags "-s -w -X main.version=$version" -o $cli ./cmd/jusplay
& $cli library add $media -json | Out-Null
function TimeCli([string]$label, [string[]]$cliArgs, [int]$n = 10) {
    $ms = @(); $peak = 0; $code = 0
    for ($i = 0; $i -lt $n; $i++) {
        $psi = [Diagnostics.ProcessStartInfo]::new($cli)
        foreach ($a in $cliArgs) { $psi.ArgumentList.Add($a) }
        $psi.RedirectStandardOutput = $true; $psi.RedirectStandardError = $true; $psi.UseShellExecute = $false
        $psi.Environment['JUSPLAY_DATA'] = $data; $psi.Environment['JUSPLAY_STATS'] = '1'
        $sw = [Diagnostics.Stopwatch]::StartNew()
        $p = [Diagnostics.Process]::Start($psi)
        $null = $p.StandardOutput.ReadToEnd(); $err = $p.StandardError.ReadToEnd()
        $p.WaitForExit(); $sw.Stop()
        $ms += $sw.Elapsed.TotalMilliseconds
        if ($err -match 'jusplay-stats peak-working-set (\d+)') { $peak = [math]::Max($peak, [long]$Matches[1]) }
        $code = $p.ExitCode
    }
    $s = $ms | Sort-Object
    Say "| $label | $n | $([math]::Round($s[[int][math]::Floor(($s.Count - 1) / 2)], 1)) ms | $([math]::Round($s[[int][math]::Floor(($s.Count - 1) * 0.95)], 1)) ms | $(& $mb $peak) MB | $code |"
}
Say "## Command line (process start to exit, 10 runs each)"
Say ""
Say "| command | runs | p50 | p95 | peak memory | exit |"
Say "|---|---:|---:|---:|---:|---:|"
TimeCli 'version' @('version')
TimeCli 'help -json' @('help', '-json')
TimeCli 'library list -json' @('library', 'list', '-json')
TimeCli 'entry info -json' @('entry', 'info', $play, '-json')
TimeCli 'link make' @('link', 'make', $play, '-at', '1')
TimeCli 'mkv verify' @('mkv', 'verify', $play)
Say ""

# 3. The window's memory: jusplay.exe and every process under it.
function TreeOf([int]$rootPid) {
    $all = Get-CimInstance Win32_Process -Property ProcessId, ParentProcessId, WorkingSetSize, PrivatePageCount
    $tree = @{ [uint32]$rootPid = $true }
    do {
        $grew = $false
        foreach ($p in $all) { if ($tree.ContainsKey([uint32]$p.ParentProcessId) -and -not $tree.ContainsKey([uint32]$p.ProcessId)) { $tree[[uint32]$p.ProcessId] = $true; $grew = $true } }
    } while ($grew)
    @($all | Where-Object { $tree.ContainsKey([uint32]$_.ProcessId) })
}
function Sample($procs, [int]$appPid) {
    [pscustomobject]@{
        n = $procs.Count
        ws = ($procs | Measure-Object WorkingSetSize -Sum).Sum
        priv = ($procs | Measure-Object PrivatePageCount -Sum).Sum
        app = ($procs | Where-Object ProcessId -eq $appPid | Measure-Object WorkingSetSize -Sum).Sum
    }
}
$env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS = "--autoplay-policy=no-user-gesture-required --mute-audio"
$lib = Start-Process $exe -ArgumentList 'library', '-data', "`"$data`"" -PassThru
Start-Sleep 10
$idle = Sample (TreeOf $lib.Id) $lib.Id
foreach ($p in (TreeOf $lib.Id)) { Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue }
Start-Sleep 2
$pl = Start-Process $exe -ArgumentList 'play', "`"$play`"", '-data', "`"$data`"" -PassThru
Start-Sleep 20
$playing = Sample (TreeOf $pl.Id) $pl.Id
foreach ($p in (TreeOf $pl.Id)) { Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue }
Start-Sleep 2
$report = Join-Path $work 'stress.json'
$st = Start-Process $exe -ArgumentList 'play', "`"$play`"", '-selftest', '-stress', "${StressSeconds}s", '-data', "`"$data`"" -RedirectStandardOutput $report -RedirectStandardError (Join-Path $work 'stress.err') -PassThru
$peak = [pscustomobject]@{ n = 0; ws = 0; priv = 0; app = 0 }; $samples = 0
while (-not $st.HasExited) {
    $s = Sample (TreeOf $st.Id) $st.Id
    if ($s.ws -gt $peak.ws) { $peak.ws = $s.ws; $peak.n = $s.n }
    if ($s.priv -gt $peak.priv) { $peak.priv = $s.priv }
    if ($s.app -gt $peak.app) { $peak.app = $s.app }
    $samples++
    Start-Sleep -Milliseconds 500
}
$j = Get-Content $report -Raw | ConvertFrom-Json
$last = @($j.samples)[-1]
Say "## The window (jusplay.exe + WebView2)"
Say ""
Say "Video: $(if ($Video) { 'a copy of the given file' } else { 'the test media' }), $([math]::Round((Get-Item $play).Length / 1MB)) MB."
Say ""
Say "| | library idle ($($idle.n) processes) | playing, 20 s in ($($playing.n)) | stress run peak ($($peak.n), $samples samples) |"
Say "|---|---:|---:|---:|"
Say "| working set | $(& $mb $idle.ws) MB | $(& $mb $playing.ws) MB | $(& $mb $peak.ws) MB |"
Say "| private bytes | $(& $mb $idle.priv) MB | $(& $mb $playing.priv) MB | $(& $mb $peak.priv) MB |"
Say "| jusplay.exe alone | $(& $mb $idle.app) MB | $(& $mb $playing.app) MB | $(& $mb $peak.app) MB |"
Say ""
Say "| stress run ($($j.stressSeconds) s) | |"
Say "|---|---:|"
Say "| stalls | $(@($j.stalls).Count) |"
Say "| errors | $(@($j.errors).Count) |"
Say "| stream restarts | $($last.restarts) |"
Say "| frames shown / dropped | $($last.frames) / $($last.dropped) |"
Say "| JS heap at the end | $($last.jsHeapMB) MB |"
Say "| Go heap / goroutines at the end | $($last.go.heapMB) MB / $($last.go.goroutines) |"

[IO.File]::WriteAllLines($out, $lines)
Write-Host "`nwrote $out"
