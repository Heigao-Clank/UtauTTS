$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$releaseRoot = Join-Path $root 'release'
$guiPath = Join-Path $releaseRoot 'UtauTTS'
$serverPath = Join-Path $releaseRoot 'UtauTTS-Server'
$guiToolsPath = Join-Path $guiPath 'tools'
$guiRuntimePath = Join-Path $guiPath 'runtime'
$guiModelsPath = Join-Path $guiPath 'models'
$serverRuntimePath = Join-Path $serverPath 'runtime'
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
New-Item -ItemType Directory -Force -Path $guiToolsPath, $guiRuntimePath, $guiModelsPath, $serverRuntimePath | Out-Null
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
        Invoke-Checked 'go' @('build', '-trimpath', '-o', (Join-Path $guiToolsPath $item[0]), $item[1])
    }

    Write-Host '=== Build server package ==='
    Invoke-Checked 'go' @('build', '-trimpath', '-o', (Join-Path $serverPath 'utautts-server.exe'), './cmd/utautts-server')

    Write-Host '=== Build Open JTalk frontend helper ==='
    & (Join-Path $PSScriptRoot 'build-openjtalk-feature-bridge.ps1')
    if ($LASTEXITCODE -ne 0) {
        throw "Open JTalk frontend helper build failed with exit code $LASTEXITCODE"
    }

    if (-not (Get-Command dotnet -ErrorAction SilentlyContinue)) {
        throw '.NET 8 SDK is required'
    }
    Invoke-Checked 'dotnet' @(
        'publish', 'tools/worldline-bridge/worldline-bridge.csproj',
        '-c', 'Release', '-r', 'win-x64', '--self-contained', 'false', '-o', $guiRuntimePath
    )
    & (Join-Path $PSScriptRoot 'fetch-worldline.ps1') -OutputPath (Join-Path $guiRuntimePath 'worldline.dll')

    Get-ChildItem -LiteralPath $guiRuntimePath -Filter 'utautts-worldline-bridge*' | Copy-Item -Destination $serverRuntimePath
    Copy-Item -LiteralPath (Join-Path $guiRuntimePath 'worldline.dll') -Destination $serverRuntimePath

    $openJTalkHelper = Join-Path $root 'tools/openjtalk-feature-bridge/bin/utautts-openjtalk-features.exe'
    $openJTalkDictionary = Join-Path $root '.tmp-openjtalk/pyopenjtalk/open_jtalk_dic_utf_8-1.11'
    foreach ($runtimePath in @($guiRuntimePath, $serverRuntimePath)) {
        Copy-Item -LiteralPath $openJTalkHelper -Destination $runtimePath
        Copy-Item -LiteralPath $openJTalkDictionary -Destination $runtimePath -Recurse
        $licensePath = Join-Path $runtimePath 'licenses'
        New-Item -ItemType Directory -Force -Path $licensePath | Out-Null
        $pythonCommand = Get-Command python -ErrorAction Stop
        Copy-Item -LiteralPath (Join-Path (Split-Path $pythonCommand.Source) 'LICENSE.txt') -Destination (Join-Path $licensePath 'PYTHON_LICENSE.txt')
        $pyInstallerLicense = @(Get-ChildItem -LiteralPath (Join-Path $root '.tmp-pyinstaller') -Recurse -Filter 'COPYING.txt' -File | Where-Object { $_.FullName -like '*pyinstaller-*.dist-info*' })
        if ($pyInstallerLicense.Count -ne 1) { throw 'Expected exactly one PyInstaller COPYING.txt' }
        Copy-Item -LiteralPath $pyInstallerLicense[0].FullName -Destination (Join-Path $licensePath 'PYINSTALLER_COPYING.txt')
    }

    Copy-Item -LiteralPath 'README.md', 'THIRD_PARTY_NOTICES.txt' -Destination $guiPath

    $sourceModels = Join-Path $root 'models'
    if (Test-Path -LiteralPath $sourceModels) {
        Get-ChildItem -LiteralPath $sourceModels -Filter '*.json' -File | Copy-Item -Destination $guiModelsPath
    }
    $v8Model = Join-Path $root 'out/prosody/jsut-1000-frame-tcn-v8.json'
    if (Test-Path -LiteralPath $v8Model) {
        Copy-Item -LiteralPath $v8Model -Destination (Join-Path $guiModelsPath 'frame-intonation-v8.json')
    } else {
        Write-Warning 'The v8 prosody model was not found under out/prosody; GUI package will require a model under models/.'
    }
    $guiDocs = Join-Path $guiPath 'docs'
    New-Item -ItemType Directory -Force -Path $guiDocs | Out-Null
    Copy-Item -Path 'docs/*' -Destination $guiDocs -Recurse

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
