# FlowVision 缩略图基准入口
#
# 用法：
#   pwsh tools/bench_thumb.ps1 -Go "C:\path\to\go.exe" -Backend wic
#   pwsh tools/bench_thumb.ps1 -Go "C:\path\to\go.exe" -Backend auto -Suite all
#
# 语料需先用 tools/make_thumb_corpus.py --full 生成；脚本不下载、不删除语料。

param(
    [string]$Go = "go",
    [ValidateSet("auto", "go", "wic", "shell", "embedded", "dct")]
    [string]$Backend = "auto",
    [ValidateSet("baseline", "formats", "quality", "real", "embedded", "dct", "all")]
    [string]$Suite = "baseline",
    [string]$Corpus = "testdata\thumb-corpus",
    [string]$Out = ""
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root

if (-not (Get-Command $Go -ErrorAction SilentlyContinue)) {
    throw "找不到 Go：$Go。请用 -Go 指向 go.exe。"
}
if (-not (Test-Path -LiteralPath $Corpus)) {
    throw "找不到语料目录：$Corpus。先运行 python tools/make_thumb_corpus.py --full"
}

$corpusAbs = (Resolve-Path -LiteralPath $Corpus).Path
$env:FLOWVISION_THUMB_CORPUS = $corpusAbs
$env:FLOWVISION_THUMB_BACKEND = $Backend

$patterns = switch ($Suite) {
    "baseline" { "TestThumbBaseline$" }
    "formats" { "TestFormatRender|TestCorpusOrientation" }
    "quality" { "TestBackendQuality" }
    "real" { "TestRealSamples" }
    "embedded" { "TestEmbeddedExtract" }
    "dct" { "TestDCTOne" }
    "all" { "TestThumbBaseline|TestFormatRender|TestCorpusOrientation|TestBackendQuality|TestRealSamples|TestEmbeddedExtract" }
}

if ($Suite -eq "baseline") {
    if (-not $Out) { $Out = "docs\thumb-bench-$Backend.json" }
    $Out = [System.IO.Path]::GetFullPath((Join-Path $root $Out))
    $dir = Split-Path -Parent $Out
    if (-not (Test-Path -LiteralPath $dir)) { New-Item -ItemType Directory -Path $dir | Out-Null }
    $env:FLOWVISION_BENCH_OUT = $Out
} else {
    Remove-Item Env:FLOWVISION_BENCH_OUT -ErrorAction SilentlyContinue
}

Write-Host "corpus : $corpusAbs"
Write-Host "backend: $Backend"
Write-Host "suite  : $Suite"
if ($Suite -eq "baseline") { Write-Host "out    : $Out" }

& $Go test -count=1 -tags mygo_nowebview -run $patterns -v .
$code = $LASTEXITCODE

if ($code -eq 0 -and $Suite -eq "baseline" -and (Test-Path -LiteralPath $Out)) {
    $rows = Get-Content -LiteralPath $Out -Raw | ConvertFrom-Json
    $rows | Select-Object name,
        @{n = "files"; e = { $_.files } },
        @{n = "wallMs"; e = { [math]::Round($_.wallMs) } },
        @{n = "perFileMs"; e = { [math]::Round($_.perFileMs, 1) } },
        @{n = "maxHeapMB"; e = { [math]::Round($_.maxHeapBytes / 1MB, 1) } } |
        Format-Table -AutoSize
}

exit $code
