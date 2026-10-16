# Install dotmgr on Windows.
#
#   irm https://git.sr.ht/~stephan/dotmgr/blob/main/install.ps1 | iex
#
# Environment:
#   DOTMGR_VERSION      release tag to install (default: the latest release)
#   DOTMGR_INSTALL_DIR  target directory (default: %LOCALAPPDATA%\Programs\dotmgr)
#   DOTMGR_BASE_URL     where release files are served (for mirrors and tests)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$repo = 'https://git.sr.ht/~stephan/dotmgr'

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "unsupported architecture $($env:PROCESSOR_ARCHITECTURE)" }
}

$version = $env:DOTMGR_VERSION
if (-not $version) {
    $version = (Invoke-RestMethod "$repo/blob/main/RELEASE").Trim()
}

$base = if ($env:DOTMGR_BASE_URL) { $env:DOTMGR_BASE_URL } else { "$repo/refs/download/$version" }
$name = "dotmgr-windows-$arch.zip"

Write-Host "Installing dotmgr $version (windows/$arch)"

$tmp = Join-Path ([IO.Path]::GetTempPath()) "dotmgr-install-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    $binary = Join-Path $tmp $name
    Invoke-WebRequest "$base/$name" -OutFile $binary
    $sums = (Invoke-RestMethod "$base/SHA256SUMS") -split "`n"

    $line = $sums | Where-Object { ($_ -split '\s+')[1] -eq $name } | Select-Object -First 1
    if (-not $line) { throw "no checksum listed for $name" }

    $expected = ($line -split '\s+')[0]
    $actual = (Get-FileHash $binary -Algorithm SHA256).Hash
    if ($actual -ne $expected.ToUpper()) { throw "checksum mismatch for $name" }

    $dir = if ($env:DOTMGR_INSTALL_DIR) { $env:DOTMGR_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\dotmgr' }
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    Expand-Archive -Path $binary -DestinationPath $tmp -Force
    Move-Item -Force (Join-Path $tmp 'dotmgr.exe') (Join-Path $dir 'dotmgr.exe')

    Write-Host "Installed $(Join-Path $dir 'dotmgr.exe')"

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (($userPath -split ';') -notcontains $dir) {
        [Environment]::SetEnvironmentVariable('Path', "$userPath;$dir", 'User')
        Write-Host "Added $dir to your user PATH; open a new terminal to use it."
    }
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
