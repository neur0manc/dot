# powershell-workflow

during testing

```pwsh
go test ./...
go build -o bin/dotmgr.exe ./cmd/dotmgr

$testRoot = Join-Path $env:TEMP "dotmgr-diff-$([guid]::NewGuid())"
$env:DOTMGR_REPO = Join-Path $testRoot "repository"
$testFile = Join-Path $HOME ".dotmgr-vimrc-test"
$utf8WithoutBom = [System.Text.UTF8Encoding]::new($false)

[System.IO.File]::WriteAllText(
    $testFile,
    "set number`n",
    $utf8WithoutBom
)

.\bin\dotmgr.exe init
.\bin\dotmgr.exe add $testFile
.\bin\dotmgr.exe diff

[System.IO.File]::WriteAllText(
    $testFile,
    "set number`nset ruler`n",
    $utf8WithoutBom
)

.\bin\dotmgr.exe status
.\bin\dotmgr.exe diff

.\bin\dotmgr.exe add $testFile
.\bin\dotmgr.exe diff

Remove-Item $testFile
Remove-Item -Recurse -Force $testRoot
Remove-Item Env:DOTMGR_REPO
```