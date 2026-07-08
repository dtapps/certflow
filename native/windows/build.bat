@echo off
REM 编译 Windows 生物识别 Helper
REM 需要 .NET 6.0+ SDK

echo Compiling biometric_helper for Windows...

REM 编译为独立可执行文件
dotnet publish -c Release -r win-x64 --self-contained true -p:PublishSingleFile=true -p:PublishTrimmed=true

REM 复制到目标目录
if not exist "..\..\internal\biometric\binaries\windows" mkdir "..\..\internal\biometric\binaries\windows"
copy bin\Release\net6.0\win-x64\publish\biometric_helper.exe "..\..\internal\biometric\binaries\windows\"

echo Done!
pause
