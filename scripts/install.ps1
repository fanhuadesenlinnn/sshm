[CmdletBinding()]
param(
    [string]$Version = $env:SSHM_VERSION,
    [string]$InstallDir = $env:SSHM_INSTALL_DIR,
    [switch]$NoPathUpdate
)

$ErrorActionPreference = 'Stop'
$Repository = 'fanhuadesenlinnn/sshm'

if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = 'latest'
}
if ($Version -ne 'latest' -and $Version -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$') {
    throw 'Version must be latest or a tag such as v6.0.10.'
}
if ($env:OS -ne 'Windows_NT') {
    throw 'This installer only supports Windows.'
}
if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
        $InstallDir = Join-Path $HOME '.local\bin'
    }
    else {
        $InstallDir = Join-Path $env:LOCALAPPDATA 'Programs\sshm'
    }
}

$RawArchitecture = if (-not [string]::IsNullOrWhiteSpace($env:PROCESSOR_ARCHITEW6432)) {
    $env:PROCESSOR_ARCHITEW6432
}
else {
    $env:PROCESSOR_ARCHITECTURE
}
$Architecture = switch -Regex ($RawArchitecture) {
    '^(AMD64|x86_64)$' { 'amd64'; break }
    '^(ARM64|aarch64)$' { 'arm64'; break }
    default { throw "Unsupported Windows architecture: $RawArchitecture" }
}

$Asset = "sshm_windows_$Architecture.zip"
if (-not [string]::IsNullOrWhiteSpace($env:SSHM_RELEASE_BASE_URL)) {
    $ReleaseBase = $env:SSHM_RELEASE_BASE_URL.TrimEnd('/')
}
elseif ($Version -eq 'latest') {
    $ReleaseBase = "https://github.com/$Repository/releases/latest/download"
}
else {
    $ReleaseBase = "https://github.com/$Repository/releases/download/$Version"
}

try {
    [Net.ServicePointManager]::SecurityProtocol =
        [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
}
catch {
    # PowerShell 7 uses the operating system TLS stack and needs no override.
}

$TempDir = Join-Path ([IO.Path]::GetTempPath()) ("sshm-install-" + [Guid]::NewGuid().ToString('N'))
$Archive = Join-Path $TempDir $Asset
$Checksums = Join-Path $TempDir 'checksums.txt'
$ExtractDir = Join-Path $TempDir 'extracted'

New-Item -ItemType Directory -Path $TempDir -Force | Out-Null
try {
    Write-Host "Downloading $Version (windows/$Architecture)..."
    Invoke-WebRequest -UseBasicParsing -Uri "$ReleaseBase/$Asset" -OutFile $Archive
    Invoke-WebRequest -UseBasicParsing -Uri "$ReleaseBase/checksums.txt" -OutFile $Checksums

    $Pattern = '^(?<hash>[0-9A-Fa-f]{64})\s+\*?' + [Regex]::Escape($Asset) + '$'
    $Expected = $null
    foreach ($Line in Get-Content -LiteralPath $Checksums) {
        if ($Line.Trim() -match $Pattern) {
            $Expected = $Matches.hash.ToLowerInvariant()
            break
        }
    }
    if ([string]::IsNullOrWhiteSpace($Expected)) {
        throw "checksums.txt does not contain $Asset."
    }
    $Actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $Archive).Hash.ToLowerInvariant()
    if ($Actual -ne $Expected) {
        throw "SHA-256 verification failed for $Asset."
    }
    Write-Host "Verified SHA-256: $Actual"

    Expand-Archive -LiteralPath $Archive -DestinationPath $ExtractDir -Force
    $Source = Join-Path $ExtractDir 'sshm.exe'
    if (-not (Test-Path -LiteralPath $Source -PathType Leaf)) {
        throw "$Asset does not contain sshm.exe."
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $Destination = Join-Path $InstallDir 'sshm.exe'
    Copy-Item -LiteralPath $Source -Destination $Destination -Force

    if (-not $NoPathUpdate) {
        $UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        $UserEntries = @($UserPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
        if (-not ($UserEntries | Where-Object { $_.TrimEnd('\') -ieq $InstallDir.TrimEnd('\') })) {
            $NewUserPath = if ([string]::IsNullOrWhiteSpace($UserPath)) {
                $InstallDir
            }
            else {
                "$UserPath;$InstallDir"
            }
            [Environment]::SetEnvironmentVariable('Path', $NewUserPath, 'User')
            Write-Host "Added to user PATH: $InstallDir"
        }
        $CurrentEntries = @($env:Path -split ';')
        if (-not ($CurrentEntries | Where-Object { $_.TrimEnd('\') -ieq $InstallDir.TrimEnd('\') })) {
            $env:Path = "$env:Path;$InstallDir"
        }
    }

    Write-Host "Installed: $Destination"
    & $Destination --version
}
finally {
    if (Test-Path -LiteralPath $TempDir) {
        Remove-Item -LiteralPath $TempDir -Recurse -Force
    }
}
