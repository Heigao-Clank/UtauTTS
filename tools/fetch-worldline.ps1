param(
    [Parameter(Mandatory = $true)]
    [string]$OutputPath
)

$ErrorActionPreference = 'Stop'
$url = 'https://raw.githubusercontent.com/openutau/OpenUtau/0.1.565/runtimes/win-x64/native/worldline.dll'
$expectedHash = '1A478B290E3EE4409A38BA37435C8B2DD8BFCAB555CE511A406F753D6BD8A05F'
$output = [IO.Path]::GetFullPath($OutputPath)
$directory = Split-Path -Parent $output
$temporary = "$output.download"

New-Item -ItemType Directory -Force -Path $directory | Out-Null
try {
    Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $temporary
    $actualHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $temporary).Hash
    if ($actualHash -ne $expectedHash) {
        throw "worldline.dll SHA-256 mismatch: $actualHash"
    }
    Move-Item -Force -LiteralPath $temporary -Destination $output
} finally {
    if (Test-Path -LiteralPath $temporary) {
        Remove-Item -Force -LiteralPath $temporary
    }
}
