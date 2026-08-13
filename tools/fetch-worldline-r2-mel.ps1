param(
    [Parameter(Mandatory = $true)]
    [string]$OutputPath
)

$ErrorActionPreference = 'Stop'
$commit = '29e0e16d1623cda79ba7c3724614d6129ba3b9d5'
$url = "https://raw.githubusercontent.com/openutau/OpenUtau/$commit/OpenUtau.Core/Classic/Data/mel.onnx"
$expectedHash = '1493FFC868C6FB8B66104E2C6A2902CBF1A8C02BB09215DB014F02E9D0E82F94'
$output = [IO.Path]::GetFullPath($OutputPath)
$directory = Split-Path -Parent $output
$temporary = "$output.download"

New-Item -ItemType Directory -Force -Path $directory | Out-Null
try {
    Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $temporary
    $actualHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $temporary).Hash
    if ($actualHash -ne $expectedHash) {
        throw "WORLDLINE-R2 mel model SHA-256 mismatch: $actualHash"
    }
    Move-Item -Force -LiteralPath $temporary -Destination $output
} finally {
    if (Test-Path -LiteralPath $temporary) {
        Remove-Item -Force -LiteralPath $temporary
    }
}
