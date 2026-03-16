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

go build -ldflags $ldflags
