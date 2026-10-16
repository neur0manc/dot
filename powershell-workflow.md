# powershell-workflow

during testing

```pwsh
go test ./...
go build -o bin/dotmgr.exe ./cmd/dotmgr

$testRoot = Join-Path $env:TEMP "dotmgr-diff-$([guid]::NewGuid())"
$env:DOTMGR_HOME = Join-Path $testRoot "home"
$env:DOTMGR_REPO = Join-Path $testRoot "repo.git"
New-Item -ItemType Directory -Force $env:DOTMGR_HOME | Out-Null
$testFile = Join-Path $env:DOTMGR_HOME ".vimrc-test"
$utf8WithoutBom = [System.Text.UTF8Encoding]::new($false)

[System.IO.File]::WriteAllText($testFile, "set number`n", $utf8WithoutBom)

.\bin\dotmgr.exe init
.\bin\dotmgr.exe add $testFile
.\bin\dotmgr.exe status

[System.IO.File]::WriteAllText($testFile, "set number`nset ruler`n", $utf8WithoutBom)

.\bin\dotmgr.exe status
.\bin\dotmgr.exe diff

.\bin\dotmgr.exe add $testFile
.\bin\dotmgr.exe diff --staged

Remove-Item -Recurse -Force $testRoot
Remove-Item Env:DOTMGR_HOME, Env:DOTMGR_REPO
```
