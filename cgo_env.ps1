<#
.SYNOPSIS
    wb-ui CGO 构建环境设置脚本 (PowerShell)
.DESCRIPTION
    设置 CGO_ENABLED=1 并添加 Skia DLL 到 PATH
.PARAMETER Command
    build  - go build ./...
    test   - go test ./...
    run    - go run <target>
.EXAMPLE
    .\cgo_env.ps1          - 仅显示环境状态
    .\cgo_env.ps1 build    - 构建全项目
    .\cgo_env.ps1 test     - 运行全部测试
#>

param(
    [string]$Command = ""
)

$env:CGO_ENABLED = "1"
$SkiaDllDir = "F:\syproject\goskia\bin"
$env:PATH = "$SkiaDllDir;$env:PATH"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "   wb-ui CGO 环境" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  CGO_ENABLED = $env:CGO_ENABLED"
$gcc = (Get-Command gcc -ErrorAction SilentlyContinue)
if ($gcc) {
    Write-Host "  CC          = $($gcc.Source)"
} else {
    Write-Host "  CC          = [not found]" -ForegroundColor Yellow
}
$dllPath = Join-Path $SkiaDllDir "libSkiaSharp.dll"
if (Test-Path $dllPath) {
    Write-Host "  DLL status  = [FOUND] $dllPath" -ForegroundColor Green
} else {
    Write-Host "  DLL status  = [NOT FOUND] $dllPath" -ForegroundColor Red
}
$goVer = go version
Write-Host "  Go version  = $goVer"
Write-Host "========================================" -ForegroundColor Cyan

switch ($Command.ToLower()) {
    "build" {
        Write-Host ">> go build ./..." -ForegroundColor Green
        go build ./...
        if ($LASTEXITCODE -eq 0) {
            Write-Host "[BUILD OK]" -ForegroundColor Green
        } else {
            Write-Host "[BUILD FAILED]" -ForegroundColor Red
        }
    }
    "test" {
        Write-Host ">> go test ./..." -ForegroundColor Green
        go test ./...
        if ($LASTEXITCODE -eq 0) {
            Write-Host "[ALL TESTS PASSED]" -ForegroundColor Green
        } else {
            Write-Host "[SOME TESTS FAILED]" -ForegroundColor Red
        }
    }
    "run" {
        if ($args.Length -eq 0) {
            Write-Host "[ERROR] run requires a target" -ForegroundColor Red
            return
        }
        Write-Host ">> go run $args" -ForegroundColor Green
        go run $args
    }
    default {
        Write-Host ""
        Write-Host "Environment ready. Available commands:" -ForegroundColor Yellow
        Write-Host "  .\cgo_env.ps1 build       - build all packages" -ForegroundColor White
        Write-Host "  .\cgo_env.ps1 test        - run all tests" -ForegroundColor White
        Write-Host "  .\cgo_env.ps1 test -v     - run all tests (verbose)" -ForegroundColor White
        Write-Host "  .\cgo_env.ps1 run main.go - run a program" -ForegroundColor White
    }
}
