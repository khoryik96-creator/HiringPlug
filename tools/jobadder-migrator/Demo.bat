@echo off
setlocal
"%~dp0HiringPlugMigration.exe" --demo %*
if errorlevel 1 pause
