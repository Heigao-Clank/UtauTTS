param(
    [Parameter(Mandatory = $true)]
    [string]$PackageRoot,
    [ValidateSet('windows-gui', 'windows-server', 'linux')]
    [string]$Variant = 'windows-gui',
    [switch]$CudaIncluded
)

$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$PackageRoot = [IO.Path]::GetFullPath($PackageRoot)
$licenseRoot = Join-Path $PackageRoot 'licenses'

function Copy-Required([string]$Source, [string]$Destination) {
    if (-not (Test-Path -LiteralPath $Source -PathType Leaf)) {
        throw "Required license file was not found: $Source"
    }
    $destinationDirectory = Split-Path -Parent $Destination
    New-Item -ItemType Directory -Force -Path $destinationDirectory | Out-Null
    Copy-Item -LiteralPath $Source -Destination $Destination -Force
}

function Write-ReleaseText([string]$Path, [string]$Text) {
    $directory = Split-Path -Parent $Path
    New-Item -ItemType Directory -Force -Path $directory | Out-Null
    Set-Content -LiteralPath $Path -Value $Text -Encoding UTF8
}

function Get-CommandOutput([string]$Command, [string[]]$Arguments) {
    $output = & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Command failed with exit code $LASTEXITCODE"
    }
    return ($output -join "`n").Trim()
}

function Resolve-QtRoot {
    if (-not [string]::IsNullOrWhiteSpace($env:QT_ROOT)) {
        $candidate = [IO.Path]::GetFullPath($env:QT_ROOT)
        if (Test-Path -LiteralPath (Join-Path $candidate 'lib/cmake/Qt6/Qt6Config.cmake') -PathType Leaf) {
            return $candidate
        }
    }
    $localQtDirectory = Join-Path $root '.qt'
    $kits = @(Get-ChildItem -LiteralPath $localQtDirectory -Directory -ErrorAction SilentlyContinue | ForEach-Object {
        $candidate = Join-Path $_.FullName 'mingw_64'
        if (Test-Path -LiteralPath (Join-Path $candidate 'lib/cmake/Qt6/Qt6Config.cmake') -PathType Leaf) {
            Get-Item -LiteralPath $candidate
        }
    } | Sort-Object { [version]$_.Parent.Name } -Descending)
    if ($kits.Count -eq 0) {
        throw 'Qt license collection requires the Qt kit used for the GUI build'
    }
    return $kits[0].FullName
}

function Copy-GoLicenses {
    $goRoot = Get-CommandOutput 'go' @('env', 'GOROOT')
    Copy-Required (Join-Path $goRoot 'LICENSE') (Join-Path $licenseRoot 'Go/GO-LICENSE.txt')

    $modules = @(
        'golang.org/x/text',
        'github.com/ikawaha/kagome/v2',
        'github.com/ikawaha/kagome-dict/ipa'
    )
    foreach ($module in $modules) {
        $moduleInfo = Get-CommandOutput 'go' @('list', '-m', '-f={{.Dir}}|{{.Version}}', $module)
        $parts = $moduleInfo.Split('|', 2)
        if ($parts.Count -ne 2) {
            throw "Could not resolve Go module metadata: $moduleInfo"
        }
        $moduleDirectory = $parts[0]
        $moduleVersion = $parts[1]
        $safeName = $module.Replace('/', '_').Replace('.', '_')
        Copy-Required (Join-Path $moduleDirectory 'LICENSE') (Join-Path $licenseRoot "Go/$safeName-$moduleVersion-LICENSE.txt")
        $notice = Join-Path $moduleDirectory 'NOTICE.txt'
        if (Test-Path -LiteralPath $notice -PathType Leaf) {
            Copy-Required $notice (Join-Path $licenseRoot "Go/$safeName-$moduleVersion-NOTICE.txt")
        }
    }
}

function Copy-OnnxLicenses {
    $assetsPath = Join-Path $root 'tools/worldline-bridge/obj/project.assets.json'
    if (-not (Test-Path -LiteralPath $assetsPath -PathType Leaf)) {
        throw "WORLDLINE-R2 assets file was not found: $assetsPath"
    }
    $assets = Get-Content -LiteralPath $assetsPath -Raw | ConvertFrom-Json
    $packageRoot = if (-not [string]::IsNullOrWhiteSpace($env:NUGET_PACKAGES)) {
        [IO.Path]::GetFullPath($env:NUGET_PACKAGES)
    } else {
        Join-Path $env:USERPROFILE '.nuget/packages'
    }
    $packageIds = @(
        'Microsoft.AI.DirectML',
        'Microsoft.ML.OnnxRuntime.DirectML',
        'Microsoft.ML.OnnxRuntime.Managed',
        'System.Memory',
        'System.Numerics.Tensors'
    )
    $destinationRoot = Join-Path $licenseRoot 'ONNX-Runtime'
    foreach ($packageId in $packageIds) {
        $library = $assets.libraries.PSObject.Properties |
            Where-Object { $_.Name -like "$packageId/*" -or $_.Name -like "$($packageId.ToLowerInvariant())/*" } |
            Select-Object -First 1
        if ($null -eq $library) {
            throw "The published WORLDLINE-R2 dependency is missing from project.assets.json: $packageId"
        }
        $libraryParts = $library.Name.Split('/', 2)
        $resolvedId = $libraryParts[0]
        $version = $libraryParts[1]
        $sourceDirectory = Join-Path (Join-Path $packageRoot $resolvedId.ToLowerInvariant()) $version
        $noticeFiles = @(Get-ChildItem -LiteralPath $sourceDirectory -File -ErrorAction Stop |
            Where-Object { $_.Name -match '(?i)(license|copying|third.?party|notice)' })
        if ($noticeFiles.Count -eq 0) {
            throw "No license or notice files were found in NuGet package $resolvedId/$version"
        }
        foreach ($noticeFile in $noticeFiles) {
            $destinationName = "$resolvedId-$version-$($noticeFile.Name)"
            Copy-Required $noticeFile.FullName (Join-Path $destinationRoot $destinationName)
        }
    }
}

function Copy-OpenJTalkLicenses {
    $source = Join-Path $root 'licenses/openjtalk'
    if (-not (Test-Path -LiteralPath $source -PathType Container)) {
        throw "Open JTalk license sources are missing: $source"
    }
    Copy-Item -LiteralPath $source -Destination (Join-Path $licenseRoot 'OpenJTalk') -Recurse -Force
    $dictionaryCopying = Join-Path $PackageRoot 'runtime/open_jtalk_dic_utf_8-1.11/COPYING'
    Copy-Required $dictionaryCopying (Join-Path $licenseRoot 'OpenJTalk/DICTIONARY_COPYING.txt')
}

function Copy-QtLicenses {
    $qtRoot = Resolve-QtRoot
    $qtVersion = (Get-Item -LiteralPath $qtRoot).Parent.Name
    $toolsRoot = [IO.Path]::GetFullPath((Join-Path $qtRoot '../../Tools'))
    $lgpl = Get-ChildItem -LiteralPath $toolsRoot -Recurse -File -Filter 'LGPLv3.txt' -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -eq $lgpl) {
        throw "The Qt SDK did not provide an LGPLv3 license text: $toolsRoot"
    }
    Copy-Required $lgpl.FullName (Join-Path $licenseRoot 'Qt/LGPL-3.0.txt')

    $qtSourceOffer = @"
Qt source offer
===============

This package contains dynamically linked Qt $qtVersion libraries.
The corresponding Qt source for the modules used by this package can be obtained
from the official Qt source archive:

https://download.qt.io/official_releases/qt/$($qtVersion.Substring(0, $qtVersion.LastIndexOf('.')))/$qtVersion/submodules/
https://code.qt.io/cgit/qt/qt5.git/tag/?h=v$qtVersion

The Qt modules used here include Qt Core, Qt GUI, Qt QML, Qt Quick,
Qt Quick Controls, Qt Multimedia, and Qt Concurrent. This source offer is
provided for the corresponding Qt version used by the build.
"@
    Write-ReleaseText (Join-Path $licenseRoot 'Qt/Qt-SOURCE-OFFER.txt') $qtSourceOffer

    $qtAttributions = @"
Qt $qtVersion third-party attributions
======================================

Qt's modules contain third-party components with their own copyright and license
terms. The authoritative attribution list for this Qt version is:

https://doc.qt.io/qt-6.8/licenses-used-in-qt.html

Qt Multimedia uses FFmpeg. The Qt Multimedia license and source guidance is:
https://doc.qt.io/qt-6.8/qtmultimedia-index.html
https://ffmpeg.org/legal.html

The Qt source offer and the LGPLv3 text are included beside this file.
"@
    Write-ReleaseText (Join-Path $licenseRoot 'Qt/Qt-THIRD-PARTY-ATTRIBUTIONS.txt') $qtAttributions

    $mingwRoots = @(Get-ChildItem -LiteralPath $toolsRoot -Directory -ErrorAction SilentlyContinue |
        Where-Object { Test-Path -LiteralPath (Join-Path $_.FullName 'licenses/mingw-w64/COPYING.MinGW-w64-runtime.txt') } |
        Sort-Object Name -Descending)
    if ($mingwRoots.Count -eq 0) {
        throw "The Qt SDK MinGW runtime licenses were not found: $toolsRoot"
    }
    $mingwLicenseRoot = Join-Path $mingwRoots[0].FullName 'licenses'
    $mingwFiles = @(
        'gcc/COPYING',
        'gcc/COPYING.LIB',
        'gcc/COPYING.RUNTIME',
        'mingw-w64/COPYING',
        'mingw-w64/COPYING.MinGW-w64-runtime.txt',
        'mingw-w64/COPYING.MinGW-w64.txt',
        'winpthreads/COPYING'
    )
    foreach ($relativePath in $mingwFiles) {
        Copy-Required (Join-Path $mingwLicenseRoot $relativePath) (Join-Path $licenseRoot "MinGW/$([IO.Path]::GetFileName($relativePath))")
    }

    $ffmpegDlls = @(Get-ChildItem -LiteralPath (Join-Path $PackageRoot 'app') -Recurse -File -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^(avcodec|avformat|avutil)-\d+\.dll$|ffmpeg' })
    if ($ffmpegDlls.Count -gt 0) {
        $dllList = ($ffmpegDlls | ForEach-Object { $_.FullName.Substring($PackageRoot.Length + 1) } | Sort-Object) -join "`n"
        $ffmpegNotice = @"
FFmpeg as deployed by Qt Multimedia
===================================

The GUI package contains the following FFmpeg-related files from the Qt
Multimedia deployment for Qt ${qtVersion}:

$dllList

Qt's prebuilt FFmpeg configuration and the applicable license/source guidance
are documented by Qt here:
https://doc.qt.io/qt-6.8/qtmultimedia-index.html
https://doc.qt.io/qt-6.8/qtwebengine-3rdparty-ffmpeg.html
https://ffmpeg.org/legal.html

The corresponding Qt source archive is identified in Qt-SOURCE-OFFER.txt.
"@
        Write-ReleaseText (Join-Path $licenseRoot 'Qt/FFmpeg-SOURCE-AND-LICENSE.txt') $ffmpegNotice
    }
}

function Copy-CudaLicense {
    $nvcc = (Get-Command nvcc -ErrorAction Stop).Source
    $cudaRoot = Split-Path -Parent (Split-Path -Parent $nvcc)
    Copy-Required (Join-Path $cudaRoot 'EULA.txt') (Join-Path $licenseRoot 'CUDA/CUDA-EULA.txt')
    $cudaLicense = Join-Path $cudaRoot 'LICENSE'
    if (Test-Path -LiteralPath $cudaLicense -PathType Leaf) {
        Copy-Required $cudaLicense (Join-Path $licenseRoot 'CUDA/CUDA-LICENSE.txt')
    }
    $version = (& $nvcc '--version' | Out-String).Trim()
    $cudaNotice = @"
CUDA renderer build provenance
=============================

The optional waveform renderer was built with nvcc at:
$nvcc

nvcc version output:
$version

The renderer uses the statically linked CUDA runtime (-cudart static). The
applicable NVIDIA CUDA Toolkit terms are included in CUDA-EULA.txt. No NVIDIA
GPU driver is distributed by this package.
"@
    Write-ReleaseText (Join-Path $licenseRoot 'CUDA/CUDA-BUILD.txt') $cudaNotice
}

New-Item -ItemType Directory -Force -Path $licenseRoot | Out-Null
Copy-GoLicenses
Copy-OnnxLicenses
Copy-OpenJTalkLicenses

if ($Variant -eq 'windows-gui') {
    Copy-QtLicenses
}

if ($CudaIncluded) {
    Copy-CudaLicense
}

$manifest = @(
    'This directory contains license and notice files copied from the exact',
    'SDK/package/toolchain versions used to assemble this release.',
    '',
    'The project-wide summary is ../THIRD_PARTY_NOTICES.txt.'
)
$manifest += @(Get-ChildItem -LiteralPath $licenseRoot -Recurse -File | ForEach-Object {
    $_.FullName.Substring($PackageRoot.Length + 1)
} | Sort-Object)
Write-ReleaseText (Join-Path $licenseRoot 'README.txt') ($manifest -join "`n")

Write-Host "Collected third-party licenses for $Variant at $licenseRoot"
