# 查看器打开性能实机基准。
# 启动带 debug 端点的临时实例，打开一个目录并逐张打开查看器，
# 记录 Decode / Scale / Encode / Bitmap 分项、首帧占位、满分辨率与内存。
#
# 示例：
#   pwsh -File tools/bench_viewer.ps1 -Runs 5
#   pwsh -File tools/bench_viewer.ps1 -Corpus testdata\thumb-corpus\viewer -Runs 3 -FullRes
#   pwsh -File tools/bench_viewer.ps1 -Build -Runs 10 -Out docs\viewer-bench-app.json
#
# 冷缓存通过临时把 %LOCALAPPDATA%\FlowVision\thumbs 下的 *-preview-*.jpg
# 移入备份目录实现；脚本结束会恢复。不会删除原图、设置或缩略图网格缓存。
param(
    [string]$App = "",
    [string]$Corpus = "testdata\thumb-corpus\dirs\24",
    [int]$Runs = 5,
    [int]$StartIndex = 0,
    [string]$Out = "docs\viewer-bench-app.json",
    [string]$Go = "go",
    [switch]$Build,
    [switch]$FullRes,
    [switch]$Preload,
    [switch]$Warm,
    [switch]$KeepPreviewCache,
    [switch]$KeepAppRunning
)

$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $root

if (-not $App) {
    $App = Join-Path $root "flowvision-debug.exe"
}
if ($Build -or -not (Test-Path -LiteralPath $App)) {
    if (-not (Get-Command $Go -ErrorAction SilentlyContinue)) {
        throw "找不到 go：请用 -Go 指定 go.exe 的绝对路径，或先安装 Go。"
    }
    Write-Host "building debug viewer..."
    & $Go build -tags "debug mygo_nowebview" -o $App .
    if ($LASTEXITCODE -ne 0) { throw "go build failed: $LASTEXITCODE" }
}

if (-not (Test-Path -LiteralPath $Corpus)) {
    throw "corpus missing: $Corpus"
}
$corpusItems = @(Get-ChildItem -LiteralPath $Corpus -File |
    Where-Object { $_.Extension -match '^\.(jpg|jpeg|png|webp|bmp|gif|tif|tiff)$' } |
    Sort-Object Name)
if ($corpusItems.Count -eq 0) {
    throw "corpus has no image: $Corpus"
}
if ($Runs -lt 1) { throw "Runs must be >= 1" }

$port = 47831
$base = "http://127.0.0.1:$port"
$previewCache = Join-Path $env:LOCALAPPDATA "FlowVision\thumbs"
$backupDir = Join-Path $env:TEMP ("flowvision-viewer-bench-" + [guid]::NewGuid().ToString("N"))
$movedCache = @()
$started = $null

function Wait-Ready([int]$timeoutMs = 20000) {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.ElapsedMilliseconds -lt $timeoutMs) {
        try {
            Invoke-RestMethod "$base/ready" -TimeoutSec 1 | Out-Null
            return
        } catch {
            Start-Sleep -Milliseconds 300
        }
    }
    throw "debug endpoint not ready after $timeoutMs ms"
}

function Invoke-Command([string]$name, [string]$arg = "") {
    $url = "$base/command?name=$([uri]::EscapeDataString($name))"
    if ($arg -ne "") { $url += "&arg=$([uri]::EscapeDataString($arg))" }
    Invoke-RestMethod $url -TimeoutSec 5 | Out-Null
}

function Get-AppProcess {
    if ($script:started -and -not $script:started.HasExited) { return $script:started }
    return (Get-Process -Id $script:started.Id)
}

function Stop-BenchApp {
    if ($script:started -and -not $script:started.HasExited) {
        try { Invoke-Command "quit" } catch {}
        Start-Sleep -Milliseconds 500
        if (-not $script:started.HasExited) {
            Stop-Process -Id $script:started.Id -Force -ErrorAction SilentlyContinue
        }
    }
}

try {
    if (-not $KeepPreviewCache -and -not $Warm -and (Test-Path -LiteralPath $previewCache)) {
        New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
        Get-ChildItem -LiteralPath $previewCache -Filter "*-preview-*.jpg" -File | ForEach-Object {
            $dest = Join-Path $backupDir $_.Name
            Move-Item -LiteralPath $_.FullName -Destination $dest
            $script:movedCache += ,@($_.FullName, $dest)
        }
    }

    try {
        Invoke-RestMethod "$base/ready" -TimeoutSec 1 | Out-Null
        throw "port $port already has a FlowVision debug instance; close it first"
    } catch {
        if ($_.Exception.Message -like "port $port already*") { throw }
    }

    Write-Host "starting $App"
    $env:FLOWVISION_VIEWER_HUD = "debug"
    if ($Warm) {
        $env:FLOWVISION_VIEWER_CACHE = "on"
    } else {
        $env:FLOWVISION_VIEWER_CACHE = "off"
    }
    if (-not $Preload) { $env:FLOWVISION_VIEWER_PRELOAD = "off" }
    $script:started = Start-Process -FilePath $App -WorkingDirectory $root -PassThru
    Wait-Ready
    Start-Sleep -Milliseconds 800

    Invoke-Command "openpath" $Corpus
    Start-Sleep -Milliseconds 800
    $state = Invoke-RestMethod "$base/state" -TimeoutSec 5
    if ($state.total -lt 1) {
        try { Invoke-Command "reload" } catch {}
        Start-Sleep -Milliseconds 800
    }
    # 等缩略图队列排空，避免 14 个缩略图 worker 与查看器解码抢 CPU。
    $deadline = (Get-Date).AddSeconds(40)
    while ((Get-Date) -lt $deadline) {
        $state = Invoke-RestMethod "$base/state" -TimeoutSec 5
        if ([int]$state.thumbPending -eq 0 -and [int]$state.thumbRunning -eq 0) { break }
        Start-Sleep -Milliseconds 100
    }
    $state = Invoke-RestMethod "$base/state" -TimeoutSec 5
    if ($state.total -lt 1) {
        throw "corpus loaded 0 items; check the path: $Corpus"
    }
    Write-Host "corpus: $Corpus ($($state.total) items), runs=$Runs"

    $results = New-Object System.Collections.Generic.List[object]
    $indices = for ($i = 0; $i -lt $Runs; $i++) {
        ($StartIndex + $i) % [int]$state.total
    }

    foreach ($idx in $indices) {
        try { Invoke-Command "closeviewer" } catch {}
        # Let the grid repaint and the close-time collection settle before the
        # next measurement; otherwise the next open samples a transient heap.
        Start-Sleep -Milliseconds 600
        $sw = [Diagnostics.Stopwatch]::StartNew()
        Invoke-Command "viewer" "$idx"
        $first = Invoke-RestMethod "$base/state" -TimeoutSec 5
        $firstMs = [math]::Round($sw.Elapsed.TotalMilliseconds, 1)
        $placeholderAtOpen = -not [bool]$first.hasBitmap

        $deadline = (Get-Date).AddSeconds(15)
        $last = $first
        while ((Get-Date) -lt $deadline) {
            $last = Invoke-RestMethod "$base/state" -TimeoutSec 5
            if ($last.viewerOpen -and [double]$last.viewerLoadMs -gt 0) { break }
            Start-Sleep -Milliseconds 20
        }
        if (-not $last.viewerOpen -or [double]$last.viewerLoadMs -le 0) {
            throw "viewer item $idx did not finish loading"
        }

        $proc = Get-Process -Id $script:started.Id
        $row = [ordered]@{
            index        = [int]$idx
            name         = [string]$last.viewerName
            bytes        = [int64]$last.viewerBytes
            loadMs       = [double]$last.viewerLoadMs
            decodeMs     = [double]$last.viewerDecodeMs
            scaleMs      = [double]$last.viewerScaleMs
            encodeMs     = [double]$last.viewerEncodeMs
            bitmapMs     = [double]$last.viewerBitmapMs
            cacheHit     = [bool]$last.viewerCacheHit
            placeholder  = $placeholderAtOpen
            placeholderMs = $firstMs
            preloadMB    = [math]::Round([double]$last.viewerPreloadMB, 2)
            privateMB    = [math]::Round($proc.PrivateMemorySize64 / 1MB, 1)
            wsMB         = [math]::Round($proc.WorkingSet64 / 1MB, 1)
        }

        if ($FullRes) {
            $t0 = Get-Date
            try { Invoke-Command "vieweractual" } catch {}
            $deadline = (Get-Date).AddSeconds(20)
            $full = Invoke-RestMethod "$base/state" -TimeoutSec 5
            while (-not $full.viewerFullRes -and -not $full.viewerFullResUnsupported -and (Get-Date) -lt $deadline) {
                Start-Sleep -Milliseconds 20
                $full = Invoke-RestMethod "$base/state" -TimeoutSec 5
            }
            $proc = Get-Process -Id $script:started.Id
            $row.fullResMs = [math]::Round(((Get-Date) - $t0).TotalMilliseconds, 1)
            $row.fullResReady = [bool]$full.viewerFullRes
            $row.fullResUnsupported = [bool]$full.viewerFullResUnsupported
            $row.fullResSkipped = (-not $full.viewerFullRes -and -not $full.viewerFullResUnsupported)
            $row.fullResMB = [math]::Round([double]$full.viewerFullResMB, 2)
            $row.privateMBAfterFullRes = [math]::Round($proc.PrivateMemorySize64 / 1MB, 1)
            $row.wsMBAfterFullRes = [math]::Round($proc.WorkingSet64 / 1MB, 1)
        }

        $results.Add([pscustomobject]$row)
        Write-Host ("[{0}] {1} load={2}ms decode={3} scale={4} encode={5} bitmap={6} private={7}MB" -f `
            $idx, $row.name, $row.loadMs, $row.decodeMs, $row.scaleMs, $row.encodeMs, $row.bitmapMs, $row.privateMB)
    }

    Invoke-Command "closeviewer"
    Start-Sleep -Milliseconds 1000
    $closeProc = Get-Process -Id $script:started.Id
    $closeState = Invoke-RestMethod "$base/state" -TimeoutSec 5

    $loads = @($results | ForEach-Object { [double]$_.loadMs } | Sort-Object)
    $summary = [ordered]@{
        runs        = $results.Count
        loadMin     = $loads[0]
        loadMedian  = $loads[[int][math]::Floor($loads.Count / 2)]
        loadP95     = $loads[[math]::Min($loads.Count - 1, [int][math]::Ceiling($loads.Count * 0.95) - 1)]
        loadMax     = $loads[$loads.Count - 1]
        placeholderCount = @($results | Where-Object { $_.placeholder }).Count
        privateMBAfterClose = [math]::Round($closeProc.PrivateMemorySize64 / 1MB, 1)
        wsMBAfterClose = [math]::Round($closeProc.WorkingSet64 / 1MB, 1)
        preloadMBAfterClose = [math]::Round([double]$closeState.viewerPreloadMB, 2)
    }

    $cacheMode = "cold"
    if ($Warm -or $KeepPreviewCache) { $cacheMode = "warm" }
    $doc = [ordered]@{}
    $doc["generatedAt"] = (Get-Date).ToString("o")
    $doc["app"] = $App
    $doc["corpus"] = $Corpus
    $doc["corpusCount"] = [int]$state.total
    $doc["startIndex"] = $StartIndex
    $doc["cacheMode"] = $cacheMode
    $doc["hudMode"] = "debug"
    $doc["preloadEnabled"] = [bool]$Preload
    $doc["fullRes"] = [bool]$FullRes
    $doc["summary"] = $summary
    $doc["results"] = $results.ToArray()
    $outPath = Join-Path $root $Out
    $outDir = Split-Path -Parent $outPath
    if ($outDir -and -not (Test-Path -LiteralPath $outDir)) {
        New-Item -ItemType Directory -Force -Path $outDir | Out-Null
    }
    $json = $doc | ConvertTo-Json -Depth 6
    [IO.File]::WriteAllText($outPath, $json, [Text.UTF8Encoding]::new($false))
    Write-Host "wrote $outPath"
} finally {
    if (-not $KeepAppRunning) { Stop-BenchApp }
    foreach ($pair in $script:movedCache) {
        if (Test-Path -LiteralPath $pair[1]) {
            Move-Item -LiteralPath $pair[1] -Destination $pair[0] -Force
        }
    }
    if (Test-Path -LiteralPath $backupDir) {
        Remove-Item -LiteralPath $backupDir -Recurse -Force -ErrorAction SilentlyContinue
    }
    Remove-Item Env:\FLOWVISION_VIEWER_HUD -ErrorAction SilentlyContinue
    Remove-Item Env:\FLOWVISION_VIEWER_CACHE -ErrorAction SilentlyContinue
    Remove-Item Env:\FLOWVISION_VIEWER_PRELOAD -ErrorAction SilentlyContinue
}
