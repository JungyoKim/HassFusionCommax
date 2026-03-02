# hass485 빌드 스크립트 (PowerShell)

Write-Host "====================================" -ForegroundColor Cyan
Write-Host "hass485 크로스 컴파일 빌드" -ForegroundColor Cyan
Write-Host "====================================" -ForegroundColor Cyan
Write-Host ""

Write-Host "[1/3] Linux AMD64 빌드 중..." -ForegroundColor Yellow
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -o hass485-linux .
if ($LASTEXITCODE -ne 0) {
    Write-Host "빌드 실패!" -ForegroundColor Red
    exit 1
}
Write-Host "Linux AMD64 빌드 완료: hass485-linux" -ForegroundColor Green
Write-Host ""

Write-Host "[2/3] Linux ARM64 빌드 중..." -ForegroundColor Yellow
$env:GOOS = "linux"
$env:GOARCH = "arm64"
go build -o hass485-linux-arm64 .
if ($LASTEXITCODE -ne 0) {
    Write-Host "빌드 실패!" -ForegroundColor Red
    exit 1
}
Write-Host "Linux ARM64 빌드 완료: hass485-linux-arm64" -ForegroundColor Green
Write-Host ""

Write-Host "[3/3] Windows AMD64 빌드 중..." -ForegroundColor Yellow
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -o hass485.exe .
if ($LASTEXITCODE -ne 0) {
    Write-Host "빌드 실패!" -ForegroundColor Red
    exit 1
}
Write-Host "Windows AMD64 빌드 완료: hass485.exe" -ForegroundColor Green
Write-Host ""

Write-Host "====================================" -ForegroundColor Cyan
Write-Host "모든 빌드 완료!" -ForegroundColor Cyan
Write-Host "====================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "생성된 파일:" -ForegroundColor White
Write-Host "  - hass485-linux (Linux AMD64)" -ForegroundColor White
Write-Host "  - hass485-linux-arm64 (Linux ARM64)" -ForegroundColor White
Write-Host "  - hass485.exe (Windows AMD64)" -ForegroundColor White
Write-Host ""



