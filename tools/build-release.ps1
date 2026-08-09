$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$releaseRoot = Join-Path $root 'release'
$guiPath = Join-Path $releaseRoot 'UtauTTS'
$serverPath = Join-Path $releaseRoot 'UtauTTS-Server'
$guiZip = Join-Path $releaseRoot 'UtauTTS-win-x64.zip'
$serverZip = Join-Path $releaseRoot 'UtauTTS-Server-win-x64.zip'
$bundledVoicebankDirectory = Join-Path $root 'voice'
$bundledVoicebankSHA256 = 'B96D1B21145F22E573AFD9EC8AEAAD0EC9CBAEE581C2623C64ADDEB31DE46B3D'

function Invoke-Checked([string]$Command, [string[]]$Arguments) {
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Command failed with exit code $LASTEXITCODE"
    }
}

function Reset-Directory([string]$Path) {
    if (-not $Path.StartsWith($releaseRoot + [IO.Path]::DirectorySeparatorChar)) {
        throw "Unsafe output path: $Path"
    }
    if (Test-Path -LiteralPath $Path) {
        Remove-Item -Recurse -Force -LiteralPath $Path
    }
    New-Item -ItemType Directory -Force -Path $Path | Out-Null
}

function Expand-BundledVoicebank([string]$Destination) {
    $archives = @(Get-ChildItem -LiteralPath $bundledVoicebankDirectory -Filter '*ver3.5.0.zip' -File)
    if ($archives.Count -ne 1) {
        throw "Expected exactly one bundled voicebank archive, found $($archives.Count) in $bundledVoicebankDirectory"
    }
    $bundledVoicebankArchive = $archives[0].FullName
    $actualHash = (Get-FileHash -LiteralPath $bundledVoicebankArchive -Algorithm SHA256).Hash
    if ($actualHash -ne $bundledVoicebankSHA256) {
        throw "Bundled voicebank hash mismatch: expected $bundledVoicebankSHA256, got $actualHash"
    }
    Expand-Archive -LiteralPath $bundledVoicebankArchive -DestinationPath $Destination
}

Reset-Directory $guiPath
Reset-Directory $serverPath
foreach ($zip in @($guiZip, $serverZip)) {
    if (Test-Path -LiteralPath $zip) {
        Remove-Item -Force -LiteralPath $zip
    }
}

$env:GOCACHE = Join-Path $root '.tmp-go-cache'
Push-Location $root
try {
    Write-Host '=== Test ==='
    Invoke-Checked 'go' @('test', './...')

    Write-Host '=== Build GUI package ==='
    Invoke-Checked 'go' @('build', '-trimpath', '-ldflags', '-H windowsgui', '-o', (Join-Path $guiPath 'utautts.exe'), './cmd/utautts-gui')
    $guiCommands = @(
        @('utautts-cli.exe', './cmd/utautts-cli'),
        @('oto-inspect.exe', './cmd/oto-inspect'),
        @('connection-eval.exe', './cmd/connection-eval'),
        @('connection-compare.exe', './cmd/connection-compare'),
        @('connection-dataset.exe', './cmd/connection-dataset'),
        @('connection-train.exe', './cmd/connection-train'),
        @('connection-lattice.exe', './cmd/connection-lattice'),
        @('connection-benchmark.exe', './cmd/connection-benchmark'),
        @('listening-test.exe', './cmd/listening-test'),
        @('listening-score.exe', './cmd/listening-score'),
        @('prosody-dataset.exe', './cmd/prosody-dataset'),
        @('prosody-train.exe', './cmd/prosody-train')
    )
    foreach ($item in $guiCommands) {
        Invoke-Checked 'go' @('build', '-trimpath', '-o', (Join-Path $guiPath $item[0]), $item[1])
    }

    Write-Host '=== Build server package ==='
    Invoke-Checked 'go' @('build', '-trimpath', '-o', (Join-Path $serverPath 'utautts-server.exe'), './cmd/utautts-server')

    if (-not (Get-Command dotnet -ErrorAction SilentlyContinue)) {
        throw '.NET 8 SDK is required'
    }
    Invoke-Checked 'dotnet' @(
        'publish', 'tools/worldline-bridge/worldline-bridge.csproj',
        '-c', 'Release', '-r', 'win-x64', '--self-contained', 'false', '-o', $guiPath
    )
    & (Join-Path $PSScriptRoot 'fetch-worldline.ps1') -OutputPath (Join-Path $guiPath 'worldline.dll')

    Get-ChildItem -LiteralPath $guiPath -Filter 'utautts-worldline-bridge*' | Copy-Item -Destination $serverPath
    Copy-Item -LiteralPath (Join-Path $guiPath 'worldline.dll') -Destination $serverPath

    Copy-Item -LiteralPath 'README.md', 'THIRD_PARTY_NOTICES.txt' -Destination $guiPath
    $guiDocs = Join-Path $guiPath 'docs'
    New-Item -ItemType Directory -Force -Path $guiDocs | Out-Null
    Copy-Item -LiteralPath 'docs/architecture.md', 'docs/training.md', 'docs/evaluation.md', 'docs/connection-dataset.md', 'docs/listening-test.md', 'docs/voicebank.md' -Destination $guiDocs

    $guiVoiceDirectory = Join-Path $guiPath 'voice'
    New-Item -ItemType Directory -Force -Path $guiVoiceDirectory | Out-Null
    Expand-BundledVoicebank $guiVoiceDirectory

    $serverVoiceDirectory = Join-Path $serverPath 'voice'
    New-Item -ItemType Directory -Force -Path $serverVoiceDirectory | Out-Null
    Set-Content -LiteralPath (Join-Path $serverVoiceDirectory 'PUT_VOICEBANKS_HERE.txt') -Encoding UTF8 -Value 'Place each UTAU voicebank in its own folder here.'

    Copy-Item -LiteralPath 'docs/server.md' -Destination (Join-Path $serverPath 'README.md')
    Copy-Item -LiteralPath 'THIRD_PARTY_NOTICES.txt' -Destination $serverPath

    Write-Host '=== Package ==='
    Compress-Archive -Path (Join-Path $guiPath '*') -DestinationPath $guiZip -CompressionLevel Optimal
    Compress-Archive -Path (Join-Path $serverPath '*') -DestinationPath $serverZip -CompressionLevel Optimal

    Write-Host 'GUI:'
    Get-ChildItem -LiteralPath $guiPath | Select-Object Name, Length
    Write-Host 'Server:'
    Get-ChildItem -LiteralPath $serverPath | Select-Object Name, Length
    Get-Item -LiteralPath $guiZip, $serverZip | Select-Object FullName, Length
} finally {
    Pop-Location
}
