$ErrorActionPreference = "Stop"

$ProjectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$SrcDir = Join-Path $ProjectRoot "src\main\java"
$OutDir = Join-Path $ProjectRoot "out"

if (Test-Path $OutDir) {
    Remove-Item -Path $OutDir -Recurse -Force
}

New-Item -Path $OutDir -ItemType Directory | Out-Null

$JavaFiles = Get-ChildItem -Path $SrcDir -Recurse -Filter *.java | ForEach-Object { $_.FullName }
if ($JavaFiles.Count -eq 0) {
    throw "No Java source files found under $SrcDir"
}

Write-Host "Compiling gateway sources..."
javac --release 21 -d $OutDir $JavaFiles

if ($LASTEXITCODE -ne 0) {
    throw "Compilation failed."
}

$Port = $env:GATEWAY_PORT
if ([string]::IsNullOrWhiteSpace($Port)) {
    $Port = "8080"
}

Write-Host "Starting API Gateway on port $Port..."
java -cp $OutDir com.interview.gateway.Application
