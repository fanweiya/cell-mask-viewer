@echo off
chcp 65001 > nul
cd /d "%~dp0"
echo [1/2] Embedding Windows application icon...
where rsrc >nul 2>nul
if %ERRORLEVEL% equ 0 (
    rsrc -ico icon.ico -o rsrc.syso
) else if exist "C:\Users\Lenovo\go\bin\rsrc.exe" (
    "C:\Users\Lenovo\go\bin\rsrc.exe" -ico icon.ico -o rsrc.syso
)

echo [2/2] Compiling CellMaskViewer with mygo native UI...
go build -ldflags "-s -w -H windowsgui" -o "CellMaskViewer.exe" .
if %ERRORLEVEL% equ 0 (
    echo.
    echo ====================================================
    echo [BUILD SUCCESS] CellMaskViewer.exe is ready with embedded icon.
    echo ====================================================
) else (
    echo [BUILD FAILED] Code: %ERRORLEVEL%
)
pause
