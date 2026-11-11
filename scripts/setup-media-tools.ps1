<#
.SYNOPSIS
  Downloads and verifies the local media tools used by the production
  pipeline into .local/ (gitignored). Safe to run repeatedly.

  - FFmpeg 8.1 GPL build (BtbN/FFmpeg-Builds), verified against the
    release's checksums.sha256 (the build is refreshed daily upstream).
  - sherpa-onnx v2.0.1 (Apache-2.0) offline text-to-speech tools, pinned SHA-256.
  - Kokoro int8 English v0.19 voice model (Apache-2.0), pinned SHA-256.
  - Inter 4.1 font (SIL OFL 1.1), pinned SHA-256 of the first verified download.
#>
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$root = Resolve-Path (Join-Path $PSScriptRoot '..')
$local = Join-Path $root '.local'
$dl = Join-Path $local 'downloads'
New-Item -ItemType Directory -Force $dl | Out-Null

function Get-Verified([string]$Url, [string]$File, [string]$Sha256) {
  $path = Join-Path $dl $File
  if (-not (Test-Path $path)) {
    Write-Host "Downloading $File"
    Invoke-WebRequest -Uri $Url -OutFile "$path.part" -UseBasicParsing
    Move-Item "$path.part" $path -Force
  }
  $actual = (Get-FileHash $path -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($actual -ne $Sha256.ToLowerInvariant()) {
    Remove-Item $path -Force
    throw "Checksum mismatch for ${File}: expected $Sha256, got $actual"
  }
  Write-Host "Verified $File"
  return $path
}

# --- FFmpeg ---------------------------------------------------------------
$ffDir = Join-Path $local 'ffmpeg'
if (-not (Test-Path (Join-Path $ffDir 'bin\ffmpeg.exe'))) {
  $base = 'https://github.com/BtbN/FFmpeg-Builds/releases/download/latest'
  $zipName = 'ffmpeg-n8.1-latest-win64-gpl-8.1.zip'
  $sums = (Invoke-WebRequest -Uri "$base/checksums.sha256" -UseBasicParsing).Content
  $line = ($sums -split "`n") | Where-Object { $_ -match [regex]::Escape($zipName) } | Select-Object -First 1
  if (-not $line) { throw "No checksum listed for $zipName" }
  $sha = ($line -split '\s+')[0]
  $zip = Get-Verified "$base/$zipName" $zipName $sha
  $tmp = Join-Path $dl 'ffmpeg-extract'
  if (Test-Path $tmp) { Remove-Item $tmp -Recurse -Force }
  Expand-Archive $zip -DestinationPath $tmp
  $inner = Get-ChildItem $tmp -Directory | Select-Object -First 1
  if (Test-Path $ffDir) { Remove-Item $ffDir -Recurse -Force }
  Move-Item $inner.FullName $ffDir
  Remove-Item $tmp -Recurse -Force
}

# --- sherpa-onnx TTS ------------------------------------------------------
$ttsDir = Join-Path $local 'sherpa-onnx'
if (-not (Test-Path (Join-Path $ttsDir 'bin\sherpa-onnx-offline-tts.exe'))) {
  $name = 'sherpa-onnx-v2.0.1-win-x64-shared-MD-MinSizeRel.tar.bz2'
  $arc = Get-Verified "https://github.com/k2-fsa/sherpa-onnx/releases/download/v2.0.1/$name" $name `
    '6c4f9235d86daa9b7f38db1bea127304f6e7f8fa5cc4447b09f2cab2b0a19d53'
  $tmp = Join-Path $dl 'sherpa-extract'
  if (Test-Path $tmp) { Remove-Item $tmp -Recurse -Force }
  New-Item -ItemType Directory $tmp | Out-Null
  tar -xjf $arc -C $tmp
  $inner = Get-ChildItem $tmp -Directory | Select-Object -First 1
  if (Test-Path $ttsDir) { Remove-Item $ttsDir -Recurse -Force }
  Move-Item $inner.FullName $ttsDir
  Remove-Item $tmp -Recurse -Force
}

# --- Kokoro voice model ---------------------------------------------------
$modelDir = Join-Path $local 'models\kokoro-int8-en-v0_19'
if (-not (Test-Path (Join-Path $modelDir 'voices.bin'))) {
  $name = 'kokoro-int8-en-v0_19.tar.bz2'
  $arc = Get-Verified "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/$name" $name `
    'c9f0dd393615805b0bab050c340834d5e684e732aec91c0e860cd30e982c08bd'
  $models = Join-Path $local 'models'
  New-Item -ItemType Directory -Force $models | Out-Null
  tar -xjf $arc -C $models
}

# --- Inter font -----------------------------------------------------------
$fontDir = Join-Path $local 'fonts'
if (-not (Test-Path (Join-Path $fontDir 'Inter-Bold.ttf'))) {
  $zip = Get-Verified 'https://github.com/rsms/inter/releases/download/v4.1/Inter-4.1.zip' 'Inter-4.1.zip' `
    '9883fdd4a49d4fb66bd8177ba6625ef9a64aa45899767dde3d36aa425756b11e'
  $tmp = Join-Path $dl 'inter-extract'
  if (Test-Path $tmp) { Remove-Item $tmp -Recurse -Force }
  Expand-Archive $zip -DestinationPath $tmp
  New-Item -ItemType Directory -Force $fontDir | Out-Null
  foreach ($f in 'Inter-Regular.ttf', 'Inter-SemiBold.ttf', 'Inter-Bold.ttf') {
    $src = Get-ChildItem $tmp -Recurse -Filter $f | Select-Object -First 1
    if (-not $src) { throw "$f not found in Inter-4.1.zip" }
    Copy-Item $src.FullName (Join-Path $fontDir $f) -Force
  }
  Copy-Item (Get-ChildItem $tmp -Recurse -Filter 'LICENSE.txt' | Select-Object -First 1).FullName (Join-Path $fontDir 'LICENSE-Inter.txt') -Force
  Remove-Item $tmp -Recurse -Force
}

Write-Host ''
Write-Host 'Media tools ready:'
& (Join-Path $ffDir 'bin\ffmpeg.exe') -hide_banner -version | Select-Object -First 1
Write-Host "TTS:   $(Join-Path $ttsDir 'bin\sherpa-onnx-offline-tts.exe')"
Write-Host "Model: $modelDir"
Write-Host "Fonts: $fontDir"
