param(
    [string]$Output = "wesaver.exe"
)

$ErrorActionPreference = "Stop"

Write-Host "Building WeSaver..."
go build -ldflags "-s -w -H windowsgui" -o $Output .

$size = (Get-Item $Output).Length / 1MB
Write-Host ("Done. Output: {0} ({1:F1} MB)" -f $Output, $size)
