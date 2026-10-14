# Installs the latest skillset release into %USERPROFILE%\.local\bin and puts that folder
# on the user PATH. Needs only PowerShell.
#   irm https://raw.githubusercontent.com/dhruvsaxena1998/skills/main/install.ps1 | iex
$ErrorActionPreference = 'Stop'
# Windows PowerShell 5.1 still offers TLS 1.0 by default, which github.com refuses.
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$release = Invoke-RestMethod 'https://api.github.com/repos/dhruvsaxena1998/skills/releases/latest'
# Match on the platform, not the whole file name, which may or may not carry .exe.
$asset = $release.assets | Where-Object { $_.name -like "*windows_$arch*" } | Select-Object -First 1
if (-not $asset) { throw "release $($release.tag_name) has no build for windows_$arch" }

$dir = Join-Path $env:USERPROFILE '.local\bin'
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$target = Join-Path $dir 'skillset.exe'
# Download beside the target first, so a cut-off download leaves the old binary in place.
# shortcut: this still fails if skillset is running; close it and re-run, or use `skillset update`.
Invoke-WebRequest -Uri $asset.browser_download_url -OutFile "$target.new" -UseBasicParsing
Move-Item -Path "$target.new" -Destination $target -Force

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $dir) {
  $userPath = if ([string]::IsNullOrEmpty($userPath)) { $dir } else { "$userPath;$dir" }
  [Environment]::SetEnvironmentVariable('Path', $userPath, 'User')
  Write-Host "added $dir to your PATH; open a new terminal to pick it up"
}
Write-Host "skillset $($release.tag_name) installed in $target"
