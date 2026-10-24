# Install dot on Windows.
#
#   irm https://raw.githubusercontent.com/neur0manc/dot/main/install.ps1 | iex
#
# Environment:
#   DOT_VERSION      release tag to install (default: the latest release)
#   DOT_INSTALL_DIR  target directory (default: %LOCALAPPDATA%\Programs\dot)
#   DOT_BASE_URL     where release files are served (for mirrors and tests)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$releases = 'https://github.com/neur0manc/dot/releases'

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "unsupported architecture $($env:PROCESSOR_ARCHITECTURE)" }
}

$version = if ($env:DOT_VERSION) { $env:DOT_VERSION } else { 'latest' }
$base = if ($env:DOT_BASE_URL) { $env:DOT_BASE_URL }
        elseif ($version -eq 'latest') { "$releases/latest/download" }
        else { "$releases/download/$version" }
$name = "dot-windows-$arch.zip"

Write-Host "Installing dot $version (windows/$arch)"

$tmp = Join-Path ([IO.Path]::GetTempPath()) "dot-install-$([guid]::NewGuid())"
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

    $dir = if ($env:DOT_INSTALL_DIR) { $env:DOT_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\dot' }
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    Expand-Archive -Path $binary -DestinationPath $tmp -Force
    Move-Item -Force (Join-Path $tmp 'dot.exe') (Join-Path $dir 'dot.exe')

    Write-Host "Installed $(Join-Path $dir 'dot.exe')"

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (($userPath -split ';') -notcontains $dir) {
        [Environment]::SetEnvironmentVariable('Path', "$userPath;$dir", 'User')
        Write-Host "Added $dir to your user PATH; open a new terminal to use it."
    }
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
