<#
.SYNOPSIS
  Downloads and verifies the local media tools used by the production
  pipeline into .local/ (gitignored). Safe to run repeatedly.

  - FFmpeg 8.1 GPL build (BtbN/FFmpeg-Builds), verified against the
    release's checksums.sha256 (the build is refreshed daily upstream).
  - sherpa-onnx v2.0.1 (Apache-2.0) offline text-to-speech tools, pinned SHA-256.
  - Kokoro v1.0 full-precision voice model (Apache-2.0) with its English
    lexicons, pinned SHA-256.
  - Parakeet TDT 110M word-timing model (CC-BY-4.0, NVIDIA), pinned SHA-256.
  - Inter 4.1 font (SIL OFL 1.1), pinned SHA-256 of the first verified download.
#>
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
# Windows bsdtar (handles .tar.bz2 and drive-letter paths); a GNU tar
# earlier on PATH, e.g. from Git Bash, does not.
$tar = Join-Path $env:SystemRoot 'System32\tar.exe'

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
  # GitHub serves this as application/octet-stream, which PowerShell 5.1
  # returns as bytes; save it and read it as text instead.
  $sumsFile = Join-Path $dl 'ffmpeg-checksums.sha256'
  Invoke-WebRequest -Uri "$base/checksums.sha256" -OutFile $sumsFile -UseBasicParsing
  $sums = Get-Content $sumsFile -Raw
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
  & $tar -xjf $arc -C $tmp
  $inner = Get-ChildItem $tmp -Directory | Select-Object -First 1
  if (Test-Path $ttsDir) { Remove-Item $ttsDir -Recurse -Force }
  Move-Item $inner.FullName $ttsDir
  Remove-Item $tmp -Recurse -Force
}

# --- Kokoro voice model ---------------------------------------------------
# Full-precision v1.0 weights: the int8 v0.19 build adds audible hiss.
$modelDir = Join-Path $local 'models\kokoro-multi-lang-v1_0'
if (-not (Test-Path (Join-Path $modelDir 'voices.bin'))) {
  $name = 'kokoro-multi-lang-v1_0.tar.bz2'
  $arc = Get-Verified "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/$name" $name `
    '60703707bc05ccd48d60e1eb723f38b07d6e474795a0b6a588d7220400e0f7a4'
  $models = Join-Path $local 'models'
  New-Item -ItemType Directory -Force $models | Out-Null
  & $tar -xjf $arc -C $models
}

# --- Fallback English lexicon ----------------------------------------------
# The v1.0 lexicon lacks some common words; the backend converts the missing
# ones from this larger lexicon into a supplement (internal/production/lexicon.go).
$lexicon = Join-Path $modelDir 'lexicon-en-us.txt'
if (-not (Test-Path $lexicon)) {
  $lex = Get-Verified 'https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/lexicon-en-us.txt' 'lexicon-en-us.txt' `
    '8b67ae549a465eee78f92ecb74644ad90c3c58ce65f19c68baf85878a10ef73a'
  Copy-Item $lex $lexicon -Force
}

# --- Word timing model ------------------------------------------------------
# NVIDIA Parakeet TDT 110M (CC-BY-4.0) converted for sherpa-onnx: transcribes
# the narration back so captions follow the voice word by word.
$asrDir = Join-Path $local 'models\sherpa-onnx-nemo-parakeet_tdt_transducer_110m-en-36000-int8'
if (-not (Test-Path (Join-Path $asrDir 'encoder.int8.onnx'))) {
  $name = 'sherpa-onnx-nemo-parakeet_tdt_transducer_110m-en-36000-int8.tar.bz2'
  $arc = Get-Verified "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/$name" $name `
    'f628312e9fdf8686374cb01a69425c41732529d540860311f16f37cbc32cfe9b'
  $models = Join-Path $local 'models'
  New-Item -ItemType Directory -Force $models | Out-Null
  & $tar -xjf $arc -C $models
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
