<#
.SYNOPSIS
  Builds the mCollaborator desktop app and, when NSIS is present, its Windows
  installer.

.DESCRIPTION
  Three steps, in order:

    1. Build the backend server from ../backend and stage it beside the shell as
       mCollaborator-server.exe. The desktop app runs this as a child process;
       it is not a fork or a copy of the backend, it is the same binary the
       server release ships.
    2. Regenerate the application icon from the brand assets.
    3. Build the Wails app, and package an installer if NSIS is installed.

  Everything lands in dist/. Nothing outside desktop/ is written to.

.PARAMETER SkipServer
  Reuse the staged server binary instead of rebuilding it.

.PARAMETER NoInstaller
  Build the .exe only, even if NSIS is available.

.EXAMPLE
  .\build.ps1
#>
[CmdletBinding()]
param(
    [switch]$SkipServer,
    [switch]$NoInstaller
)

# Native tools here report failure through their exit code, which every call
# site checks. Windows PowerShell otherwise treats anything a native command
# writes to stderr as a terminating error, which would abort the build on
# ordinary progress output.
$ErrorActionPreference = 'Continue'
$root = $PSScriptRoot
$backend = Join-Path (Split-Path $root -Parent) 'backend'
$dist = Join-Path $root 'dist'
$binDir = Join-Path $root 'build\bin'

function Step($msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }
function Note($msg) { Write-Host "    $msg" -ForegroundColor DarkGray }

# Wails installs to GOPATH\bin, which is not always on PATH.
$goBin = Join-Path (& go env GOPATH) 'bin'
if ($env:Path -notlike "*$goBin*") { $env:Path = "$env:Path;$goBin" }

New-Item -ItemType Directory -Force $dist | Out-Null

# --- 1. the server ---------------------------------------------------------
if (-not $SkipServer) {
    Step 'Building the mCollaborator server'
    Push-Location $backend
    try {
        & go build -trimpath -ldflags '-s -w' -o (Join-Path $binDir 'mCollaborator-server.exe') .
        if ($LASTEXITCODE -ne 0) { throw 'server build failed' }
    } finally { Pop-Location }
    Note "staged $(Join-Path $binDir 'mCollaborator-server.exe')"
} else {
    Step 'Reusing the staged server binary'
}

# --- 2. the icon -----------------------------------------------------------
Step 'Generating the application icon'
Push-Location $root
try {
    & go run ./tools/mkicon `
        -mark ../backend/static/images/cyberteq-mark.png `
        -reference ../backend/static/apple-touch-icon.png `
        -out build/windows/icon.ico `
        -png build/appicon.png
    if ($LASTEXITCODE -ne 0) { throw 'icon generation failed' }
} finally { Pop-Location }

# --- 3. the app, and the installer -----------------------------------------
# NSIS installs outside PATH by default, and `wails build -nsis` only warns when
# it cannot find makensis - it still exits 0. The collect step would then copy
# whatever installer the previous run left in build\bin, so dist/ ends up with a
# fresh app beside a stale installer and nothing said about it.
$nsisDirs = @("${env:ProgramFiles(x86)}\NSIS", "$env:ProgramFiles\NSIS")
if (-not (Get-Command makensis -ErrorAction SilentlyContinue)) {
    foreach ($dir in $nsisDirs) {
        if (Test-Path (Join-Path $dir 'makensis.exe')) { $env:Path = "$env:Path;$dir"; break }
    }
}
$haveNsis = $null -ne (Get-Command makensis -ErrorAction SilentlyContinue)

Push-Location $root
try {
    if ($haveNsis -and -not $NoInstaller) {
        Step 'Building the app and the NSIS installer'
        & wails build -platform windows/amd64 -skipbindings -nsis
    } else {
        Step 'Building the app'
        if (-not $haveNsis -and -not $NoInstaller) {
            Note 'NSIS not found, so no installer is being produced.'
            Note 'Install it with:  winget install NSIS.NSIS'
        }
        & wails build -platform windows/amd64 -skipbindings
    }
    if ($LASTEXITCODE -ne 0) { throw 'wails build failed' }

    # A missing makensis is a warning, not a failure, so the only proof the
    # installer was really rebuilt is that it is newer than the app beside it.
    if ($haveNsis -and -not $NoInstaller) {
        $app = Get-Item (Join-Path $binDir 'mCollaborator.exe')
        $installer = Get-ChildItem $binDir -Filter '*installer.exe' -ErrorAction SilentlyContinue |
                     Sort-Object LastWriteTime -Descending | Select-Object -First 1
        if ($null -eq $installer -or $installer.LastWriteTime -lt $app.LastWriteTime) {
            throw 'the installer was not rebuilt, so dist/ would have taken a stale one'
        }
    }
} finally { Pop-Location }

# --- collect ---------------------------------------------------------------
Step 'Collecting artefacts into dist/'
Get-ChildItem $binDir -File | ForEach-Object {
    Copy-Item $_.FullName (Join-Path $dist $_.Name) -Force
    Note ('{0,-32} {1,8:N0} KB' -f $_.Name, ($_.Length / 1KB))
}

Write-Host "`nDone. Artefacts are in $dist" -ForegroundColor Green
Write-Host @'

  mCollaborator.exe          the desktop app
  mCollaborator-server.exe   the server it runs; both must sit together
  mCollaborator-*-installer.exe  the installer, when NSIS produced one

'@ -ForegroundColor DarkGray
