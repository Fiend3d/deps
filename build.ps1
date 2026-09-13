$icon = $false

foreach ($arg in $args) {
    if ($arg -eq "icon") {
        $icon = $true
    }
}

$commit    = git rev-parse --short HEAD
$version   = git describe --tags --abbrev=0
$dirty     = git status --porcelain

if (-not $version) { $version = "dev" }
if ($dirty) { $version += "-dirty" }

$ldflags = @(
    "-X 'main.Version=$version'"
    "-X 'main.GitCommit=$commit'"
) -join " "

if ($icon) {
    rsrc -ico .\assets\icon.ico
}

$outputPath = Join-Path $PSScriptRoot "deps.exe"
if (Test-Path -LiteralPath $outputPath) {
    Remove-Item -LiteralPath $outputPath -ErrorAction Stop
}

go build -a -o $outputPath -ldflags $ldflags
if ($LASTEXITCODE -ne 0) {
    throw "go build failed with exit code $LASTEXITCODE"
}
