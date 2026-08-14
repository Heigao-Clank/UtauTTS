$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$releaseRoot = Join-Path $root 'release'
$guiPath = Join-Path $releaseRoot 'UtauTTS'
$serverPath = Join-Path $releaseRoot 'UtauTTS-Server'
$guiToolsPath = Join-Path $guiPath 'tools'
$guiRuntimePath = Join-Path $guiPath 'runtime'
$guiModelsPath = Join-Path $guiPath 'models'
$guiPluginsPath = Join-Path $guiPath 'plugins'
$serverRuntimePath = Join-Path $serverPath 'runtime'
$serverModelsPath = Join-Path $serverPath 'models'
$serverPluginsPath = Join-Path $serverPath 'plugins'
$guiZip = Join-Path $releaseRoot 'UtauTTS-win-x64.zip'
$serverZip = Join-Path $releaseRoot 'UtauTTS-Server-win-x64.zip'
$bundledVoicebankDirectory = Join-Path $root 'voice'
$bundledVoicebankSHA256 = 'B96D1B21145F22E573AFD9EC8AEAAD0EC9CBAEE581C2623C64ADDEB31DE46B3D'
$cudaAvailable = $null -ne (Get-Command nvcc -ErrorAction SilentlyContinue)

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
New-Item -ItemType Directory -Force -Path $guiToolsPath, $guiRuntimePath, $guiModelsPath, $guiPluginsPath, $serverRuntimePath, $serverModelsPath, $serverPluginsPath | Out-Null
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
    & (Join-Path $PSScriptRoot 'build-qt.ps1') -OutputDirectory $guiPath
    if ($LASTEXITCODE -ne 0) { throw "Qt GUI build failed with exit code $LASTEXITCODE" }
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
    Copy-Item -LiteralPath (Join-Path $serverPath 'utautts-server.exe') -Destination (Join-Path $guiToolsPath 'utautts-server.exe')

    Write-Host '=== Build Open JTalk frontend helper ==='
    & (Join-Path $PSScriptRoot 'build-openjtalk-feature-bridge.ps1')
    if ($LASTEXITCODE -ne 0) {
        throw "Open JTalk frontend helper build failed with exit code $LASTEXITCODE"
    }

    if (-not (Get-Command dotnet -ErrorAction SilentlyContinue)) {
        throw '.NET 8 SDK is required'
    }
    $previousNugetPackages = $env:NUGET_PACKAGES
    $env:NUGET_PACKAGES = Join-Path $env:USERPROFILE '.nuget/packages'
    try {
        Invoke-Checked 'dotnet' @(
            'publish', 'tools/worldline-bridge/worldline-bridge.csproj',
            '-c', 'Release', '-r', 'win-x64', '--self-contained', 'false', '--ignore-failed-sources', '-o', $guiRuntimePath
        )
    } finally {
        $env:NUGET_PACKAGES = $previousNugetPackages
    }
    & (Join-Path $PSScriptRoot 'fetch-worldline.ps1') -OutputPath (Join-Path $guiRuntimePath 'worldline.dll')
    & (Join-Path $PSScriptRoot 'fetch-worldline-r2-mel.ps1') -OutputPath (Join-Path $guiRuntimePath 'worldline-r2-mel.onnx')

    if ($cudaAvailable) {
        Write-Host '=== Build optional CUDA waveform renderer ==='
        & (Join-Path $PSScriptRoot 'build-waveform-gpu.ps1') -OutputDirectory $guiRuntimePath
        if ($LASTEXITCODE -ne 0) { throw "CUDA waveform renderer build failed with exit code $LASTEXITCODE" }
        Copy-Item -LiteralPath (Join-Path $guiRuntimePath 'utautts-waveform-gpu.dll') -Destination $serverRuntimePath -Force
    } else {
        Write-Warning 'CUDA Toolkit was not found; waveform-gpu will not be included'
    }

    Get-ChildItem -LiteralPath $guiRuntimePath -Filter 'utautts-worldline-bridge*' | Copy-Item -Destination $serverRuntimePath
    $worldlineR2RuntimeAssets = @(
        'DirectML.dll',
        'Microsoft.ML.OnnxRuntime.dll',
        'System.Numerics.Tensors.dll',
        'onnxruntime.dll',
        'onnxruntime_providers_shared.dll'
    )
    foreach ($asset in $worldlineR2RuntimeAssets) {
        $source = Join-Path $guiRuntimePath $asset
        if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
            throw "WORLDLINE-R2 runtime asset was not published: $asset"
        }
        Copy-Item -LiteralPath $source -Destination $serverRuntimePath -Force
    }
    Copy-Item -LiteralPath (Join-Path $guiRuntimePath 'worldline.dll') -Destination $serverRuntimePath
    Copy-Item -LiteralPath (Join-Path $guiRuntimePath 'worldline-r2-mel.onnx') -Destination $serverRuntimePath

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

    Copy-Item -LiteralPath 'LICENSE', 'README.md', 'THIRD_PARTY_NOTICES.txt' -Destination $guiPath

    $sourceModels = Join-Path $root 'models'
    $bundledModels = @()
    if (Test-Path -LiteralPath $sourceModels) {
		$bundledModels = @(Get-ChildItem -LiteralPath $sourceModels -Filter '*.json' -File)
    }
    if ($bundledModels.Count -eq 0) {
        throw 'No bundled prosody models found. Install self-describing models into models/ with tools/install-prosody-model.ps1.'
    }
    $bundledModels | Copy-Item -Destination $guiModelsPath
    $bundledModels | Copy-Item -Destination $serverModelsPath
    Copy-Item -LiteralPath (Join-Path $root 'plugins/renderers') -Destination $guiPluginsPath -Recurse
    Copy-Item -LiteralPath (Join-Path $root 'plugins/renderers') -Destination $serverPluginsPath -Recurse
    if (-not $cudaAvailable) {
        foreach ($pluginsPath in @($guiPluginsPath, $serverPluginsPath)) {
            $waveformGPUManifest = Join-Path $pluginsPath 'renderers/waveform-gpu'
            if (Test-Path -LiteralPath $waveformGPUManifest) {
                Remove-Item -LiteralPath $waveformGPUManifest -Recurse -Force
            }
        }
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
    Copy-Item -LiteralPath 'LICENSE', 'THIRD_PARTY_NOTICES.txt' -Destination $serverPath

    Write-Host '=== Collect exact third-party licenses ==='
    & (Join-Path $PSScriptRoot 'collect-third-party-licenses.ps1') -PackageRoot $guiPath -Variant windows-gui -CudaIncluded:$cudaAvailable
    if ($LASTEXITCODE -ne 0) { throw 'GUI third-party license collection failed' }
    & (Join-Path $PSScriptRoot 'collect-third-party-licenses.ps1') -PackageRoot $serverPath -Variant windows-server -CudaIncluded:$cudaAvailable
    if ($LASTEXITCODE -ne 0) { throw 'Server third-party license collection failed' }

    foreach ($packagePath in @($guiPath, $serverPath)) {
        Get-ChildItem -LiteralPath $packagePath -Recurse -File |
            Where-Object { $_.Extension -in @('.pdb', '.lib', '.exp') -or $_.Name -eq 'DirectML.Debug.dll' } |
            Remove-Item -Force
    }
    $qmlToolingPath = Join-Path $guiPath 'app/qmltooling'
    if (Test-Path -LiteralPath $qmlToolingPath) {
        Remove-Item -LiteralPath $qmlToolingPath -Recurse -Force
    }

    Write-Host '=== Package ==='
    Compress-Archive -Path (Join-Path $guiPath '*') -DestinationPath $guiZip -CompressionLevel Optimal
    Compress-Archive -Path (Join-Path $serverPath '*') -DestinationPath $serverZip -CompressionLevel Optimal

    & (Join-Path $PSScriptRoot 'test-release-package.ps1') -ReleaseRoot $releaseRoot
    if ($LASTEXITCODE -ne 0) { throw "Release package smoke test failed with exit code $LASTEXITCODE" }

    Write-Host 'GUI:'
    Get-ChildItem -LiteralPath $guiPath | Select-Object Name, Length
    Write-Host 'Server:'
    Get-ChildItem -LiteralPath $serverPath | Select-Object Name, Length
    Get-Item -LiteralPath $guiZip, $serverZip | Select-Object FullName, Length
} finally {
    Pop-Location
}
