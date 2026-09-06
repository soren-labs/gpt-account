# Windows from-zero installer. Does not require WSL or Python.
# Put this file next to gpa.exe (release zip) and run:
#   powershell -ExecutionPolicy Bypass -File .\install.ps1
$ErrorActionPreference = 'Stop'

function Find-GpaExe {
    $here = $PSScriptRoot
    $root = Split-Path -Parent $here
    $candidates = @(
        (Join-Path $here 'gpa.exe'),
        (Join-Path (Get-Location) 'gpa.exe'),
        (Join-Path $root 'dist\gpa.exe'),
        (Join-Path $here '..\dist\gpa.exe')
    )
    foreach ($p in $candidates) {
        if ($p -and (Test-Path $p)) { return (Resolve-Path $p).Path }
    }
    $release = $env:GPA_RELEASE_URL
    if (-not $release) {
        $release = 'https://github.com/soren-labs/gpt-account/releases/latest/download/gpa.exe'
    }
    $tmp = Join-Path $env:TEMP 'gpa-release.exe'
    try {
        Write-Host "Downloading gpa.exe ..."
        Invoke-WebRequest -UseBasicParsing -Uri $release -OutFile $tmp
        if (Test-Path $tmp) { return $tmp }
    } catch {
        throw "gpa.exe not found next to this script, and download failed. Place gpa.exe beside install.ps1 or set GPA_RELEASE_URL."
    }
}

function Same-Path([string]$a, [string]$b) {
    if (-not $a -or -not $b) { return $false }
    $fa = [IO.Path]::GetFullPath($a)
    $fb = [IO.Path]::GetFullPath($b)
    return $fa.ToLowerInvariant() -eq $fb.ToLowerInvariant()
}

$exe = Find-GpaExe
$destDir = Join-Path $env:LOCALAPPDATA 'gpa\bin'
$bin = Join-Path $destDir 'gpa.exe'
New-Item -ItemType Directory -Force -Path $destDir | Out-Null
if (Same-Path $exe $bin) {
    Write-Host "gpa.exe already in $destDir"
} else {
    Copy-Item -Force $exe $bin
}

$cur = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not $cur) { $cur = '' }
$parts = @($cur -split ';' | Where-Object { $_ -and $_.Trim() -ne '' -and $_.Trim() -ne '.' })
if ($parts -notcontains $destDir) { $parts += $destDir }
[Environment]::SetEnvironmentVariable('Path', ($parts -join ';'), 'User')

$Wsh = New-Object -ComObject WScript.Shell
$lnk = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\GPA.lnk'
$sc = $Wsh.CreateShortcut($lnk)
$sc.TargetPath = $bin
$sc.WorkingDirectory = $env:USERPROFILE
$sc.Save()

Write-Host "Installed $bin"
& $bin install
if ($LASTEXITCODE -ne 0) {
    throw "gpa install failed: $LASTEXITCODE"
}
Write-Host "Open a new PowerShell and run: gpa"
