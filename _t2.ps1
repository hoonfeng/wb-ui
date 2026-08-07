$env:CGO_ENABLED = '1'
Set-Location F:\syproject\wb-ui
Remove-Item -ErrorAction SilentlyContinue _t1.ps1
Write-Host '--- build core ---'
go build ./app ./bindings ./bridge ./dom ./html ./jsc ./page ./webkit ./platform/... ./style/... ./rendering/... 2>&1 | Select-Object -First 20
Write-Host "core exit=$LASTEXITCODE"
Write-Host '--- build browser ---'
go build ./cmd/browser/ 2>&1 | Select-Object -First 10
Write-Host "browser exit=$LASTEXITCODE"
Write-Host '--- build mem_probe ---'
go build ./dev/mem_probe/ 2>&1 | Select-Object -First 10
Write-Host "memprobe exit=$LASTEXITCODE"
