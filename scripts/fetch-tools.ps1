param(
    [string]$Dest = (Join-Path $PSScriptRoot "..\dist\windows\bin")
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
New-Item -ItemType Directory -Force -Path $Dest | Out-Null
$Tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("grimoire-tools-" + [guid]::NewGuid())
New-Item -ItemType Directory -Force -Path $Tmp | Out-Null

function Get-Zip([string]$Url, [string]$Name) {
    $zip = Join-Path $Tmp "$Name.zip"
    Write-Host "Downloading $Name from $Url"
    Invoke-WebRequest -Uri $Url -OutFile $zip -UseBasicParsing
    $out = Join-Path $Tmp $Name
    Expand-Archive -Path $zip -DestinationPath $out -Force
    return $out
}

function Get-LatestAsset([string]$Repo, [string]$Pattern) {
    $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
    $asset = $rel.assets | Where-Object { $_.name -match $Pattern } | Select-Object -First 1
    if (-not $asset) { throw "No asset matches $Pattern in $Repo" }
    return $asset.browser_download_url
}

$vips = Get-Zip (Get-LatestAsset "libvips/build-win64-mxe" "^vips-dev-w64-all-.*\.zip$") "vips"
$vipsBin = Get-ChildItem -Path $vips -Recurse -Directory -Filter bin | Select-Object -First 1
Copy-Item -Path (Join-Path $vipsBin.FullName "*") -Destination $Dest -Recurse -Force

$ff = Get-Zip "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip" "ffmpeg"
Get-ChildItem -Path $ff -Recurse -Include ffmpeg.exe, ffprobe.exe | Copy-Item -Destination $Dest -Force

$pop = Get-Zip (Get-LatestAsset "oschwartz10612/poppler-windows" "^Release-.*\.zip$") "poppler"
$popBin = Get-ChildItem -Path $pop -Recurse -Directory -Filter bin | Where-Object { Test-Path (Join-Path $_.FullName "pdftoppm.exe") } | Select-Object -First 1
$popDest = Join-Path $Dest "pdftoppm"
New-Item -ItemType Directory -Force -Path $popDest | Out-Null
Copy-Item -Path (Join-Path $popBin.FullName "*") -Destination $popDest -Recurse -Force

$ver = (Invoke-WebRequest -Uri "https://exiftool.org/ver.txt" -UseBasicParsing).Content.Trim()
$exif = Get-Zip "https://exiftool.org/exiftool-${ver}_64.zip" "exiftool"
$exifDir = Get-ChildItem -Path $exif -Directory | Select-Object -First 1
$exifDest = Join-Path $Dest "exiftool"
New-Item -ItemType Directory -Force -Path $exifDest | Out-Null
Copy-Item -Path (Join-Path $exifDir.FullName "*") -Destination $exifDest -Recurse -Force
$exe = Get-ChildItem -Path $exifDest -Filter "exiftool*.exe" | Select-Object -First 1
Rename-Item -Path $exe.FullName -NewName "exiftool.exe" -Force -ErrorAction SilentlyContinue

Remove-Item -Recurse -Force $Tmp
Write-Host "Tools are in $Dest"
