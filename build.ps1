# Builds bin\jusplay.exe: the page bundle, then the Go program as a
# Windows GUI binary (-H=windowsgui: no console window when opened from
# Explorer; see cmd/jusplay/console_windows.go for terminal use).
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$go = (Get-Command go -ErrorAction SilentlyContinue).Source
if (-not $go) { $go = 'C:\Application\Go\bin\go.exe' }
npm --prefix web run build
if ($LASTEXITCODE) { exit $LASTEXITCODE }
# The version comes from VERSION. -s -w leave out the symbol table and
# debug info (panics still name their functions).
$version = (Get-Content VERSION -Raw).Trim()
& $go build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=$version" -o bin\jusplay.exe ./cmd/jusplay
exit $LASTEXITCODE
