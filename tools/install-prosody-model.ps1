param(
    [Parameter(Mandatory = $true)]
    [string]$ModelPath,
    [string]$DestinationDirectory = "models"
)

$ErrorActionPreference = 'Stop'
$source = [IO.Path]::GetFullPath($ModelPath)
$destinationRoot = [IO.Path]::GetFullPath($DestinationDirectory)
$model = Get-Content -LiteralPath $source -Raw -Encoding UTF8 | ConvertFrom-Json
if ([string]::IsNullOrWhiteSpace([string]$model.id)) {
    throw 'The model does not contain a stable id.'
}
if ([string]::IsNullOrWhiteSpace([string]$model.display_name)) {
    throw 'The model does not contain display_name.'
}
if ([string]$model.id -notmatch '^[A-Za-z0-9][A-Za-z0-9._-]*$') {
    throw "Invalid model id: $($model.id)"
}
New-Item -ItemType Directory -Force -Path $destinationRoot | Out-Null
$destination = Join-Path $destinationRoot ($model.id + '.json')
Copy-Item -LiteralPath $source -Destination $destination -Force
Write-Host "Installed model $($model.display_name) ($($model.id)): $destination"
