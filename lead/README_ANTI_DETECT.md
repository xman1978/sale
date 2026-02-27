# 绕过乙方宝反爬检测说明

若仍被 SafeLine WAF 拦截（"Debugging Detected"），可尝试以下方案：

## 方案 1：连接已启动的 Chrome（推荐）

**原理**：手动启动的 Chrome 无自动化标识，最难被检测。

**重要**：必须同时指定 `--user-data-dir`，否则 Chrome 不会监听 9222 端口。

**步骤**：

1. **关闭所有 Chrome 窗口**

2. **以调试模式启动 Chrome**（PowerShell）：
   ```powershell
   & "C:\Program Files\Google\Chrome\Application\chrome.exe" --remote-debugging-port=9222 --user-data-dir="$env:TEMP\chrome-debug"
   ```
   或使用固定路径：
   ```powershell
   & "C:\Program Files\Google\Chrome\Application\chrome.exe" --remote-debugging-port=9222 --user-data-dir="C:\Users\你的用户名\AppData\Local\Temp\chrome-debug"
   ```

3. **验证端口**：在浏览器访问 http://127.0.0.1:9222/json/version 应返回 JSON

4. **设置环境变量并运行**：
   ```powershell
   $env:CHROME_DEBUG_PORT = "9222"
   .\lead.exe
   ```

或直接指定完整 WebSocket URL（从 Chrome 启动后控制台输出获取）：
```powershell
$env:CHROME_DEBUG_URL = "ws://127.0.0.1:9222/devtools/browser/4dcf09f2-ba2b-463a-8ff5-90d27c6cc913"
.\lead.exe
```

## 方案 2：NewUserMode（默认）

程序默认使用 `launcher.NewUserMode()`，会使用你的真实 Chrome 配置。

**注意**：运行前需**关闭所有 Chrome 窗口**，否则会冲突。

## 方案 3：其他建议

- **代理**：更换 IP（如住宅代理）可能绕过 IP 风控
- **降低频率**：程序已增加 5 秒等待，可适当再调大
- **联系网站**：若为合规业务，可申请 API 或白名单
