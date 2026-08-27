<#
.SYNOPSIS
  Builds every shipping binary in the repository from the current source.

.DESCRIPTION
  There are two independent things called mCollaborator, and until now only one
  of them was ever rebuilt:

    backend\mCollaborator.exe     the web app, run directly and served on
                                  http://localhost:9900
    desktop\dist\*.exe            the desktop app, the server it spawns, and
                                  the NSIS installer

  desktop\build.ps1 rebuilds the second set only. A change under backend\
  therefore reached the desktop app as soon as anyone packaged it, while the web
  binary stayed at whatever revision it was last built at - for three days, in
  the case that prompted this script. Nothing reported the drift, because a
  stale binary runs perfectly well; it just renders the previous version of the
  report.

  This script builds both in one invocation, then compares the web binary
  against the server binary the desktop app ships. They are the same package
  built with the same flags, so their hashes must match. A mismatch means the
  two halves have diverged again, and is treated as a failure.

  The web binary is compiled to a temporary path and swapped in only once it has
  built, so a failed build never leaves you without a working server. The binary
  it replaces is kept beside it as mCollaborator.prev.exe.

.PARAMETER WebOnly
  Build backend\mCollaborator.exe and skip the desktop bundle. Useful while
  iterating on the backend, when no installer is needed.

.PARAMETER DesktopOnly
  Build the desktop bundle only, exactly as desktop\build.ps1 does today.

.PARAMETER NoRestart
  A running web server is stopped for the swap either way. This leaves it
  stopped instead of starting it again afterwards.

.PARAMETER NoInstaller
  Passed through to desktop\build.ps1: build the app without the installer.

.EXAMPLE
  .\build.ps1
  Rebuild everything, and restart the web server if it was up.

.EXAMPLE
  .\build.ps1 -WebOnly
  Rebuild just the web binary.
#>
[CmdletBinding()]
param(
    [switch]$WebOnly,
    [switch]$DesktopOnly,
    [switch]$NoRestart,
    [switch]$NoInstaller
)

# Native tools report failure through their exit code, which every call site
# checks. Windows PowerShell otherwise treats anything a native command writes
# to stderr as a terminating error, which would abort the build on ordinary
# progress output.
$ErrorActionPreference = 'Continue'

$root    = $PSScriptRoot
$backend = Join-Path $root 'backend'
$desktop = Join-Path $root 'desktop'
$dist    = Join-Path $desktop 'dist'

$webExe    = Join-Path $backend 'mCollaborator.exe'
$prevExe   = Join-Path $backend 'mCollaborator.prev.exe'
$tmpExe    = Join-Path $backend 'mCollaborator.build.exe'
$distApp   = Join-Path $dist 'mCollaborator.exe'
$serverExe = Join-Path $dist 'mCollaborator-server.exe'
$installer = Join-Path $dist 'mCollaborator-amd64-installer.exe'

function Step($msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }
function Note($msg) { Write-Host "    $msg" -ForegroundColor DarkGray }
function Warn($msg) { Write-Host "    $msg" -ForegroundColor Yellow }

if ($WebOnly -and $DesktopOnly) { throw '-WebOnly and -DesktopOnly are mutually exclusive' }
if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'go is not on PATH' }

# Processes are matched on their image path, never on their name: the web app
# and the desktop app are both called mCollaborator.exe, and stopping the wrong
# one would be both incorrect and hard to work out from the output. The name is
# derived from the path only to narrow the Get-Process call.
function Get-RunningFrom($path) {
    $name = [System.IO.Path]::GetFileNameWithoutExtension($path)
    Get-Process -Name $name -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -eq $path }
}

function Stop-RunningFrom($path, $label) {
    $procs = @(Get-RunningFrom $path)
    if ($procs.Count -eq 0) { return $false }
    Step "Stopping $label so its binary can be replaced"
    foreach ($p in $procs) {
        Note "PID $($p.Id)  $($p.Path)"
        Stop-Process -Id $p.Id -Force
    }
    Start-Sleep -Seconds 2
    return $true
}

$webPort = if ($env:PORT) { $env:PORT } else { '9900' }
$restartWeb = $false

# --- 1. the web binary -----------------------------------------------------
if (-not $DesktopOnly) {
    Step 'Building the web app (backend\mCollaborator.exe)'
    Push-Location $backend
    try {
        & go build -trimpath -ldflags '-s -w' -o $tmpExe .
        if ($LASTEXITCODE -ne 0) { throw 'backend build failed' }
    } finally { Pop-Location }
    Note ('compiled {0:N0} KB' -f ((Get-Item $tmpExe).Length / 1KB))

    # Only now is the running server worth disturbing: the replacement already
    # exists, so the window in which there is no working server is the swap.
    if (Stop-RunningFrom $webExe 'the web server') {
        $restartWeb = -not $NoRestart
    }

    Step 'Swapping the new web binary into place'
    if (Test-Path $webExe) { Move-Item $webExe $prevExe -Force }
    Move-Item $tmpExe $webExe -Force
    Note "the binary it replaced is kept as $(Split-Path $prevExe -Leaf)"
}

# --- 2. the desktop bundle -------------------------------------------------
$desktopOK = $false
if (-not $WebOnly) {
    # desktop\build.ps1 stages into build\bin and copies over dist\ as its last
    # step, so anything holding a file in dist\ open fails the build after the
    # slow part has already run. Two processes can be doing that, and the second
    # is easy to miss: the desktop shell runs the server as a child process, and
    # killing the shell orphans that child rather than taking it down, leaving
    # dist\mCollaborator-server.exe locked by a process with no window.
    Stop-RunningFrom $distApp   'the desktop app'         | Out-Null
    Stop-RunningFrom $serverExe 'its server child' | Out-Null

    # Whether the bundle was really rebuilt is judged on the artefacts, not on
    # an exit code: wails only warns when it cannot find NSIS, and a script
    # invocation does not reliably set $LASTEXITCODE either way.
    $before = if (Test-Path $serverExe) { (Get-Item $serverExe).LastWriteTime } else { [datetime]::MinValue }

    Step 'Building the desktop bundle'
    $buildArgs = @()
    if ($NoInstaller) { $buildArgs += '-NoInstaller' }
    try {
        & (Join-Path $desktop 'build.ps1') @buildArgs
    } catch {
        Warn "the desktop build failed: $($_.Exception.Message)"
    }

    $desktopOK = (Test-Path $serverExe) -and ((Get-Item $serverExe).LastWriteTime -gt $before)
    if (-not $desktopOK) { Warn 'the desktop bundle was not refreshed' }
}

# --- 3. restart ------------------------------------------------------------
if ($restartWeb) {
    Step 'Restarting the web server'
    Start-Process -FilePath $webExe -WorkingDirectory $backend -WindowStyle Minimized
    Start-Sleep -Seconds 4
    try {
        $r = Invoke-WebRequest -Uri "http://localhost:$webPort/health" -UseBasicParsing -TimeoutSec 10
        Note "http://localhost:$webPort/health -> $($r.StatusCode) $($r.Content)"
    } catch {
        Warn "the server did not answer /health on port ${webPort}: $($_.Exception.Message)"
    }
} elseif ($NoRestart -and -not $DesktopOnly) {
    Note 'the web server was left stopped, as asked'
}

# --- 4. the drift check ----------------------------------------------------
# The point of the script. Both binaries are the same package built from
# backend\ with identical flags, so identical hashes are the proof that the web
# app and the desktop app carry the same code. Anything else is precisely the
# bug this script exists to prevent, so it fails rather than warns.
if (-not $WebOnly -and -not $DesktopOnly -and (Test-Path $webExe) -and (Test-Path $serverExe)) {
    Step 'Checking the two halves agree'
    $a = (Get-FileHash $webExe -Algorithm SHA256).Hash
    $b = (Get-FileHash $serverExe -Algorithm SHA256).Hash
    if ($a -eq $b) {
        Note "match  $($a.Substring(0, 16))..."
    } else {
        Write-Host "    web    $a" -ForegroundColor Red
        Write-Host "    server $b" -ForegroundColor Red
        throw 'the web binary and the desktop server binary differ, though they are built from the same source'
    }
}

# --- summary ---------------------------------------------------------------
Step 'Built'
@(
    @{ Name = 'backend\mCollaborator.exe';                      Path = $webExe }
    @{ Name = 'desktop\dist\mCollaborator.exe';                 Path = $distApp }
    @{ Name = 'desktop\dist\mCollaborator-server.exe';          Path = $serverExe }
    @{ Name = 'desktop\dist\mCollaborator-amd64-installer.exe'; Path = $installer }
) | ForEach-Object {
    $line = if (Test-Path $_.Path) {
        $f = Get-Item $_.Path
        '{0,-48} {1,9:N0} KB   {2}' -f $_.Name, ($f.Length / 1KB), $f.LastWriteTime
    } else {
        '{0,-48} {1}' -f $_.Name, '(not built)'
    }
    Write-Host "    $line" -ForegroundColor DarkGray
}

Write-Host "`nDone." -ForegroundColor Green
