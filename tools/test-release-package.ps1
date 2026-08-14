param(
    [string]$ReleaseRoot = "$(Join-Path $PSScriptRoot '..\release')"
)

$ErrorActionPreference = 'Stop'
$null = Add-Type -AssemblyName System.IO.Compression.FileSystem
$ReleaseRoot = [IO.Path]::GetFullPath($ReleaseRoot)
$guiZip = Join-Path $ReleaseRoot 'UtauTTS-win-x64.zip'
$serverZip = Join-Path $ReleaseRoot 'UtauTTS-Server-win-x64.zip'
$temporaryRoot = Join-Path ([IO.Path]::GetTempPath()) ('utautts-release-test-' + [Guid]::NewGuid().ToString('N'))

function Assert-Path([string]$Path, [string]$Description) {
    if (-not (Test-Path -LiteralPath $Path)) {
        throw "Missing ${Description}: $Path"
    }
}

function Assert-Zip([string]$Path) {
    Assert-Path $Path "release archive"
    $archive = [IO.Compression.ZipFile]::OpenRead($Path)
    try {
        if ($archive.Entries.Count -eq 0) {
            throw "Release archive is empty: $Path"
        }
    } finally {
        $archive.Dispose()
    }
}

New-Item -ItemType Directory -Force -Path $temporaryRoot | Out-Null
try {
    Assert-Zip $guiZip
    Assert-Zip $serverZip
    $guiRoot = Join-Path $temporaryRoot 'gui'
    $serverRoot = Join-Path $temporaryRoot 'server'
    Expand-Archive -LiteralPath $guiZip -DestinationPath $guiRoot
    Expand-Archive -LiteralPath $serverZip -DestinationPath $serverRoot

    foreach ($packageRoot in @($guiRoot, $serverRoot)) {
        Assert-Path (Join-Path $packageRoot 'LICENSE') 'project license'
        Assert-Path (Join-Path $packageRoot 'THIRD_PARTY_NOTICES.txt') 'third-party notices'
        Assert-Path (Join-Path $packageRoot 'licenses/README.txt') 'license bundle manifest'
        Assert-Path (Join-Path $packageRoot 'licenses/Go/GO-LICENSE.txt') 'Go runtime license'
        Assert-Path (Join-Path $packageRoot 'licenses/ONNX-Runtime/Microsoft.ML.OnnxRuntime.DirectML-1.23.0-LICENSE') 'ONNX Runtime license'
        Assert-Path (Join-Path $packageRoot 'licenses/ONNX-Runtime/Microsoft.ML.OnnxRuntime.DirectML-1.23.0-ThirdPartyNotices.txt') 'ONNX Runtime third-party notices'
        Assert-Path (Join-Path $packageRoot 'licenses/OpenJTalk/HTS_ENGINE_API_COPYING.txt') 'hts_engine_API license'
    }

    foreach ($asset in @(
        'licenses/Qt/LGPL-3.0.txt',
        'licenses/Qt/Qt-SOURCE-OFFER.txt',
        'licenses/Qt/Qt-THIRD-PARTY-ATTRIBUTIONS.txt',
        'licenses/Qt/FFmpeg-SOURCE-AND-LICENSE.txt',
        'licenses/MinGW/COPYING.RUNTIME',
        'licenses/MinGW/COPYING.MinGW-w64-runtime.txt'
    )) {
        Assert-Path (Join-Path $guiRoot $asset) "GUI license asset $asset"
    }

    $serverRuntime = Join-Path $serverRoot 'runtime'
    foreach ($asset in @(
        'DirectML.dll',
        'Microsoft.ML.OnnxRuntime.dll',
        'System.Numerics.Tensors.dll',
        'onnxruntime.dll',
        'onnxruntime_providers_shared.dll'
    )) {
        Assert-Path (Join-Path $serverRuntime $asset) "Server WORLDLINE-R2 asset $asset"
    }

    $gpuManifest = Test-Path -LiteralPath (Join-Path $serverRoot 'plugins/renderers/waveform-gpu/plugin.json')
    $gpuBinary = Test-Path -LiteralPath (Join-Path $serverRuntime 'utautts-waveform-gpu.dll')
    if ($gpuManifest -ne $gpuBinary) {
        throw "waveform-gpu manifest and runtime DLL disagree: manifest=$gpuManifest dll=$gpuBinary"
    }
    if ($gpuBinary) {
        foreach ($packageRoot in @($guiRoot, $serverRoot)) {
            Assert-Path (Join-Path $packageRoot 'licenses/CUDA/CUDA-EULA.txt') 'CUDA EULA'
            Assert-Path (Join-Path $packageRoot 'licenses/CUDA/CUDA-BUILD.txt') 'CUDA build provenance'
        }
    }

    $unexpectedDebugFiles = @(Get-ChildItem -LiteralPath $guiRoot -Recurse -File |
        Where-Object { $_.Extension -in @('.pdb', '.lib', '.exp') -or $_.Name -eq 'DirectML.Debug.dll' })
    if ($unexpectedDebugFiles.Count -ne 0) {
        throw "Release package contains debug/development files: $($unexpectedDebugFiles.FullName -join ', ')"
    }

    $voicebank = Get-ChildItem -LiteralPath (Join-Path $guiRoot 'voice') -Directory | Select-Object -First 1
    if ($null -eq $voicebank) {
        throw 'GUI release package contains no bundled voicebank'
    }
    $cli = Join-Path $guiRoot 'tools/utautts-cli.exe'
    Assert-Path $cli 'packaged CLI'
    $workingDirectory = Join-Path $temporaryRoot 'working-directory'
    New-Item -ItemType Directory -Force -Path $workingDirectory | Out-Null
    $outputWav = Join-Path $workingDirectory 'package-smoke.wav'
    $smokeText = -join @([char]0x3053, [char]0x3093, [char]0x306B, [char]0x3061, [char]0x306F)
    Push-Location $workingDirectory
    try {
        & $cli --renderer waveform --voicebank $voicebank.FullName --text $smokeText --out $outputWav
        if ($LASTEXITCODE -ne 0) {
            throw "Packaged CLI failed with exit code $LASTEXITCODE"
        }
    } finally {
        Pop-Location
    }
    Assert-Path $outputWav 'packaged CLI output'

    $savedErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $nanOutput = & $cli --renderer waveform --voicebank $voicebank.FullName --text $smokeText --mora-ms NaN --out (Join-Path $workingDirectory 'nan.wav') 2>&1
    } finally {
        $ErrorActionPreference = $savedErrorActionPreference
    }
    $nanExitCode = $LASTEXITCODE
    if ($nanExitCode -eq 0 -or ($nanOutput -join "`n") -match 'panic') {
        throw "Packaged CLI accepted or panicked on NaN input: exit=$nanExitCode output=$($nanOutput -join ' ')"
    }

    $server = Join-Path $serverRoot 'utautts-server.exe'
    Assert-Path $server 'packaged server'
    $port = 18000 + (Get-Random -Minimum 0 -Maximum 1000)
    $stdout = Join-Path $temporaryRoot 'server.stdout.log'
    $stderr = Join-Path $temporaryRoot 'server.stderr.log'
    $savedPath = $env:Path
    Remove-Item Env:PATH -ErrorAction SilentlyContinue
    try {
        $process = Start-Process -FilePath $server -ArgumentList @(
            '--host', '127.0.0.1', '--port', $port.ToString(), '--voice-dir', $voicebank.FullName, '--renderer', 'waveform'
        ) -WorkingDirectory $workingDirectory -WindowStyle Hidden -RedirectStandardOutput $stdout -RedirectStandardError $stderr -PassThru
    } finally {
        $env:Path = $savedPath
    }
    try {
        $health = $null
        for ($attempt = 0; $attempt -lt 40; $attempt++) {
            if ($process.HasExited) {
                $errorText = if (Test-Path -LiteralPath $stderr) { Get-Content -LiteralPath $stderr -Raw } else { '' }
                throw "Packaged server exited with code $($process.ExitCode): $errorText"
            }
            try {
                $health = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$port/api/health" -TimeoutSec 2
                break
            } catch {
                Start-Sleep -Milliseconds 250
            }
        }
        if ($null -eq $health -or $health.StatusCode -ne 200) {
            throw 'Packaged server health check timed out'
        }
    } finally {
        if (-not $process.HasExited) {
            Stop-Process -Id $process.Id -Force
            $process.WaitForExit()
        }
    }
    Write-Host 'Release package smoke test passed'
} finally {
    if (Test-Path -LiteralPath $temporaryRoot) {
        Remove-Item -LiteralPath $temporaryRoot -Recurse -Force
    }
}
exit 0
