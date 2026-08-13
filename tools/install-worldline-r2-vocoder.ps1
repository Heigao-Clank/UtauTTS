param(
    [Parameter(Mandatory = $true)]
    [string]$OutputPath,
    [switch]$AcceptNonCommercialLicense
)

$ErrorActionPreference = 'Stop'
if (-not $AcceptNonCommercialLicense) {
    throw 'The official vocoder weights use CC BY-NC-SA 4.0. Read the release terms, then rerun with -AcceptNonCommercialLicense.'
}

$url = 'https://github.com/openvpi/vocoders/releases/download/pc-nsf-hifigan-44.1k-hop512-128bin-2025.02/pc_nsf_hifigan_44.1k_hop512_128bin_2025.02.oudep'
$archiveHash = 'BA7D43142D41F6900C8264B5662CA7125A50FEB8760BB8B9615C61A8F5E6902E'
$modelHash = 'B297C0C790DBCFE6B2123D87CC20BF8718969F91FFA3170A198C8FB53686155D'
$modelName = 'pc_nsf_hifigan_44.1k_hop512_128bin_2025.02.onnx'
$output = [IO.Path]::GetFullPath($OutputPath)
$outputDirectory = Split-Path -Parent $output
$temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ('utautts-r2-' + [Guid]::NewGuid().ToString('N'))
$archive = Join-Path $temporaryDirectory 'vocoder.zip'

New-Item -ItemType Directory -Force -Path $temporaryDirectory, $outputDirectory | Out-Null
try {
    Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $archive
    $actualArchiveHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $archive).Hash
    if ($actualArchiveHash -ne $archiveHash) {
        throw "vocoder package SHA-256 mismatch: $actualArchiveHash"
    }
    Expand-Archive -LiteralPath $archive -DestinationPath $temporaryDirectory
    $model = Join-Path $temporaryDirectory $modelName
    $actualModelHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $model).Hash
    if ($actualModelHash -ne $modelHash) {
        throw "vocoder model SHA-256 mismatch: $actualModelHash"
    }
    Copy-Item -LiteralPath $model -Destination $output -Force
    Write-Host "Installed WORLDLINE-R2 vocoder: $output"
} finally {
    if (Test-Path -LiteralPath $temporaryDirectory) {
        Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force
    }
}
