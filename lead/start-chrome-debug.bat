@echo off

set CHROME="C:\Program Files\Google\Chrome\Application\chrome.exe"
set USER_DATA_DIR=%TEMP%\chrome-debug

%CHROME% --remote-debugging-port=9222 --user-data-dir=%USER_DATA_DIR%
