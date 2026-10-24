# powershell-workflow

during testing

```pwsh
go test ./...
go build -o bin/dot.exe ./cmd/dot

$testRoot = Join-Path $env:TEMP "dot-diff-$([guid]::NewGuid())"
$env:DOT_HOME = Join-Path $testRoot "home"
$env:DOT_REPO = Join-Path $testRoot "repo.git"
New-Item -ItemType Directory -Force $env:DOT_HOME | Out-Null
$testFile = Join-Path $env:DOT_HOME ".vimrc-test"
$utf8WithoutBom = [System.Text.UTF8Encoding]::new($false)

[System.IO.File]::WriteAllText($testFile, "set number`n", $utf8WithoutBom)

.\bin\dot.exe init
.\bin\dot.exe add $testFile
.\bin\dot.exe status

[System.IO.File]::WriteAllText($testFile, "set number`nset ruler`n", $utf8WithoutBom)

.\bin\dot.exe status
.\bin\dot.exe diff

.\bin\dot.exe add $testFile
.\bin\dot.exe diff --staged

Remove-Item -Recurse -Force $testRoot
Remove-Item Env:DOT_HOME, Env:DOT_REPO
```
