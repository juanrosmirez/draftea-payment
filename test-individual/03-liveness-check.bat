@echo off
echo ========================================
echo        LIVENESS CHECK TEST
echo ========================================
echo Testing: http://localhost:8081/live
echo.

curl -s http://localhost:8081/live

echo.
echo ========================================
echo Test completed. Press any key to exit.
pause > nul
