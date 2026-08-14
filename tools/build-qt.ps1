param(
    [string]$QtRoot = $env:QT_ROOT,
    [string]$OutputDirectory = ""
)
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$standaloneDevelopmentPackage = [string]::IsNullOrWhiteSpace($OutputDirectory)
$nativeDir = Join-Path $root 'build/native'
$qtBuildDir = Join-Path $root 'build/qt'
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) { $OutputDirectory = Join-Path $root 'build/qt-package' }
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
$appDirectory = Join-Path $OutputDirectory 'app'

if ([string]::IsNullOrWhiteSpace($QtRoot)) {
    $localQtDirectory = Join-Path $root '.qt'
    if (Test-Path -LiteralPath $localQtDirectory -PathType Container) {
        $localKits = @(Get-ChildItem -LiteralPath $localQtDirectory -Directory | ForEach-Object {
            $kit = Join-Path $_.FullName 'mingw_64'
            if (Test-Path -LiteralPath (Join-Path $kit 'lib/cmake/Qt6/Qt6Config.cmake') -PathType Leaf) {
                Get-Item -LiteralPath $kit
            }
        } | Sort-Object { [version]$_.Parent.Name } -Descending)
        if ($localKits.Count -gt 0) { $QtRoot = $localKits[0].FullName }
    }
}
if ([string]::IsNullOrWhiteSpace($QtRoot)) {
    throw 'Qt 6.5+ SDK was not found. Install Qt Quick, Qt Multimedia, and Qt Concurrent, then set QT_ROOT to the compiler kit directory (for example C:\Qt\6.8.3\mingw_64), or install it under .qt/<version>/mingw_64.'
}
$QtRoot = [IO.Path]::GetFullPath($QtRoot)
$qtConfig = Join-Path $QtRoot 'lib/cmake/Qt6/Qt6Config.cmake'
$deployTool = Join-Path $QtRoot 'bin/windeployqt.exe'
if (-not (Test-Path -LiteralPath $qtConfig -PathType Leaf)) { throw "Qt6Config.cmake was not found under $QtRoot" }
if (-not (Test-Path -LiteralPath $deployTool -PathType Leaf)) { throw "windeployqt.exe was not found under $QtRoot" }

$toolsRoot = [IO.Path]::GetFullPath((Join-Path $QtRoot '../../Tools'))
$compilerDirectory = Join-Path $toolsRoot 'mingw1310_64/bin'
$cc = Join-Path $compilerDirectory 'gcc.exe'
$cxx = Join-Path $compilerDirectory 'g++.exe'
$goCC = 'C:\msys64\mingw64\bin\clang.exe'
$goCXX = 'C:\msys64\mingw64\bin\clang++.exe'
$gendef = Join-Path $compilerDirectory 'gendef.exe'
$dlltool = Join-Path $compilerDirectory 'dlltool.exe'
$cmake = Join-Path $toolsRoot 'CMake_64/bin/cmake.exe'
$ninjaDirectory = Join-Path $toolsRoot 'Ninja'
foreach ($tool in @($cc,$cxx,$goCC,$goCXX,$gendef,$dlltool,$cmake,(Join-Path $ninjaDirectory 'ninja.exe'))) { if (-not (Test-Path -LiteralPath $tool -PathType Leaf)) { throw "Required Qt build tool was not found: $tool" } }

New-Item -ItemType Directory -Force -Path $nativeDir, $qtBuildDir, $OutputDirectory | Out-Null
if (Test-Path -LiteralPath $appDirectory) { Remove-Item -LiteralPath $appDirectory -Recurse -Force }
New-Item -ItemType Directory -Force -Path $appDirectory | Out-Null
$previousCgo = $env:CGO_ENABLED; $previousCC=$env:CC; $previousCXX=$env:CXX; $previousPath=$env:Path; $previousGoCache=$env:GOCACHE
try {
    $env:CGO_ENABLED='1';$env:CC=$goCC;$env:CXX=$goCXX;$env:GOCACHE=Join-Path $root '.tmp-go-cache-qt-cgo';$env:Path=(Split-Path $goCC -Parent)+';'+$env:Path
    Push-Location $root
    try { & go build -buildmode=c-shared -o (Join-Path $nativeDir 'utautts_native.dll') ./cmd/utautts-native; if ($LASTEXITCODE -ne 0) { throw 'Go native library build failed' } }
    finally { Pop-Location }
    Push-Location $nativeDir
    try {
        & $gendef 'utautts_native.dll'; if ($LASTEXITCODE -ne 0) { throw 'gendef failed' }
        $nativeDefinition = Get-Content -LiteralPath 'utautts_native.def' -Raw
        $requiredExports = @('UtauTTSCreate','UtauTTSLastError','UtauTTSCall','UtauTTSDestroy','UtauTTSFree')
        $missingExports = @($requiredExports | Where-Object { $nativeDefinition -notmatch "(?m)^$([regex]::Escape($_))\s*$" })
        if ($missingExports.Count -gt 0) {
            throw "Go native DLL is missing C exports: $($missingExports -join ', '). Keep each //export directive directly above its Go function."
        }
        & $dlltool -d 'utautts_native.def' -l 'utautts_native.dll.a' -D 'utautts_native.dll'; if ($LASTEXITCODE -ne 0) { throw 'dlltool failed' }
    } finally { Pop-Location }
} finally { $env:CGO_ENABLED=$previousCgo;$env:CC=$previousCC;$env:CXX=$previousCXX;$env:GOCACHE=$previousGoCache;$env:Path=$previousPath }

$env:Path = $ninjaDirectory + ';' + $compilerDirectory + ';' + $env:Path
& $cmake -S (Join-Path $root 'qt') -B $qtBuildDir -G Ninja "-DCMAKE_PREFIX_PATH=$QtRoot" "-DUTAUTTS_NATIVE_DIR=$nativeDir" "-DCMAKE_C_COMPILER=$cc" "-DCMAKE_CXX_COMPILER=$cxx" -DCMAKE_BUILD_TYPE=Release
if ($LASTEXITCODE -ne 0) { throw 'Qt CMake configure failed' }
& $cmake --build $qtBuildDir --config Release
if ($LASTEXITCODE -ne 0) { throw 'Qt build failed' }
$executable = Get-ChildItem -LiteralPath $qtBuildDir -Recurse -Filter 'utautts.exe' -File | Select-Object -First 1
if ($null -eq $executable) { throw 'Qt executable was not produced' }
Copy-Item -LiteralPath $executable.FullName -Destination (Join-Path $appDirectory 'utautts-gui.exe') -Force
Copy-Item -LiteralPath (Join-Path $nativeDir 'utautts_native.dll') -Destination $appDirectory -Force
& $deployTool --release --qmldir (Join-Path $root 'qt/qml') (Join-Path $appDirectory 'utautts-gui.exe')
if ($LASTEXITCODE -ne 0) { throw 'windeployqt failed' }
$previousLauncherGoCache = $env:GOCACHE
$env:GOCACHE = Join-Path $root '.tmp-go-cache'
Push-Location $root
try {
    & go build -trimpath -ldflags '-H windowsgui' -o (Join-Path $OutputDirectory 'utautts.exe') ./cmd/utautts-launcher
    if ($LASTEXITCODE -ne 0) { throw 'Qt launcher build failed' }
} finally {
    Pop-Location
    $env:GOCACHE = $previousLauncherGoCache
}
$staleDeployDirectories = @('generic','iconengines','imageformats','multimedia','networkinformation','platforms','qml','qmltooling','tls','translations')
foreach ($name in $staleDeployDirectories) {
	$stalePath = Join-Path $OutputDirectory $name
	if (Test-Path -LiteralPath $stalePath -PathType Container) { Remove-Item -LiteralPath $stalePath -Recurse -Force }
}
$appQmlToolingPath = Join-Path $appDirectory 'qmltooling'
if (Test-Path -LiteralPath $appQmlToolingPath -PathType Container) { Remove-Item -LiteralPath $appQmlToolingPath -Recurse -Force }
Get-ChildItem -LiteralPath $OutputDirectory -Filter '*.dll' -File | Remove-Item -Force
if ($standaloneDevelopmentPackage) {
    foreach ($name in @('models','plugins','runtime','voice')) {
        $assetPath = Join-Path $OutputDirectory $name
        if (Test-Path -LiteralPath $assetPath) { Remove-Item -LiteralPath $assetPath -Recurse -Force }
    }
    Copy-Item -LiteralPath (Join-Path $root 'models') -Destination (Join-Path $OutputDirectory 'models') -Recurse
    New-Item -ItemType Directory -Force -Path (Join-Path $OutputDirectory 'plugins') | Out-Null
    Copy-Item -LiteralPath (Join-Path $root 'plugins/renderers') -Destination (Join-Path $OutputDirectory 'plugins') -Recurse
    $runtimeCandidates = @(
        (Join-Path $root 'runtime'),
        (Join-Path $root 'release/UtauTTS/runtime')
    )
    $runtimeSource = $runtimeCandidates | Where-Object {
        Test-Path -LiteralPath (Join-Path $_ 'utautts-openjtalk-features.exe') -PathType Leaf
    } | Select-Object -First 1
    if ($runtimeSource) {
        Copy-Item -LiteralPath $runtimeSource -Destination (Join-Path $OutputDirectory 'runtime') -Recurse
    } else {
        Write-Warning 'Runtime assets were not found. Run build.bat once to prepare Open JTalk and WORLDLINE assets.'
    }
    $voiceDirectory = Join-Path $OutputDirectory 'voice'
    New-Item -ItemType Directory -Force -Path $voiceDirectory | Out-Null
    $voiceArchives = @(Get-ChildItem -LiteralPath (Join-Path $root 'voice') -Filter '*.zip' -File)
    foreach ($archive in $voiceArchives) {
        Expand-Archive -LiteralPath $archive.FullName -DestinationPath $voiceDirectory
    }
}
Write-Host "Built Qt package at $OutputDirectory"
