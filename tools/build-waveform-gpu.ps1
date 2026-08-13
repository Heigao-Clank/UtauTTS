param([string]$OutputDirectory = "")
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $OutputDirectory = Join-Path $root 'build/gpu'
}
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
$nvcc = (Get-Command nvcc -ErrorAction Stop).Source
$cl = Get-Command cl.exe -ErrorAction SilentlyContinue
if ($null -eq $cl) {
    $vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
    if (-not (Test-Path -LiteralPath $vswhere -PathType Leaf)) {
        throw 'Visual Studio Build Tools were not found (vswhere.exe is missing)'
    }
    $visualStudio = & $vswhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
    if ([string]::IsNullOrWhiteSpace($visualStudio)) {
        throw 'Install the Visual Studio Desktop development with C++ workload to build the CUDA renderer'
    }
    $developerShell = Join-Path $visualStudio 'Common7/Tools/VsDevCmd.bat'
    $environment = & $env:ComSpec /d /s /c "`"$developerShell`" -no_logo -arch=x64 >nul && set"
    if ($LASTEXITCODE -ne 0) { throw 'Visual Studio developer environment initialization failed' }
    $developerPaths = @()
    foreach ($line in $environment) {
        $separator = $line.IndexOf('=')
        if ($separator -gt 0) {
            $name = $line.Substring(0, $separator)
            $value = $line.Substring($separator + 1)
            if ($name -ieq 'PATH') {
                $developerPaths += $value
            } else {
                [Environment]::SetEnvironmentVariable($name, $value, 'Process')
            }
        }
    }
    $developerPath = $developerPaths | Where-Object {
        $candidatePath = $_
        @($candidatePath.Split(';') | Where-Object {
            -not [string]::IsNullOrWhiteSpace($_) -and (Test-Path -LiteralPath (Join-Path $_ 'cl.exe') -PathType Leaf)
        }).Count -gt 0
    } | Select-Object -First 1
    if ([string]::IsNullOrWhiteSpace($developerPath)) { throw 'Visual Studio developer PATH was not produced' }
    $env:Path = $developerPath
    $cl = Get-Command cl.exe -ErrorAction SilentlyContinue
    if ($null -eq $cl) { throw 'Visual Studio developer environment did not provide cl.exe' }
}
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
$output = Join-Path $OutputDirectory 'utautts-waveform-gpu.dll'
& $nvcc -shared -O3 -std=c++17 -cudart static '-Xcompiler=/wd4819' -o $output (Join-Path $root 'tools/waveform-gpu/wsola.cu')
if ($LASTEXITCODE -ne 0) { throw "CUDA waveform backend build failed with exit code $LASTEXITCODE" }
Write-Host "Built CUDA waveform backend at $output"
