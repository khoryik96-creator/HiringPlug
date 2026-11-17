@echo off
setlocal
"%~dp0HiringPlugMigration.exe" %*
if errorlevel 1 pause
