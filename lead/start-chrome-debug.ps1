# 以调试模式启动 Chrome（用于绕过反爬检测）
# 使用前请先关闭所有 Chrome 窗口

$chromePath = "C:\Program Files\Google\Chrome\Application\chrome.exe"
$userDataDir = "$env:TEMP\chrome-debug"

if (-not (Test-Path $chromePath)) {
    Write-Host "未找到 Chrome，请修改脚本中的路径" -ForegroundColor Red
    exit 1
}

Write-Host "启动 Chrome 调试模式..." -ForegroundColor Green
Write-Host "调试端口: 9222" -ForegroundColor Gray
Write-Host "用户数据目录: $userDataDir" -ForegroundColor Gray
Write-Host ""
Write-Host "启动后，在另一个终端运行:" -ForegroundColor Yellow
Write-Host '  $env:CHROME_DEBUG_PORT = "9222"' -ForegroundColor Cyan
Write-Host '  .\lead.exe' -ForegroundColor Cyan
Write-Host ""

& $chromePath --remote-debugging-port=9222 --user-data-dir=$userDataDir
