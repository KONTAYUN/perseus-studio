@echo off
setlocal
cd /d "%~dp0"
title Perseus Studio - backend tests

if not exist "test-results" mkdir "test-results"

docker info >nul 2>&1
if errorlevel 1 (
  echo Docker is not running. Start Docker Desktop and run this script again.
  pause
  exit /b 1
)

set IMG=golang:1.27-bookworm
rem Mount the whole repo, not just backend\: document tests read ..\tests\fixtures.
set RUN=docker run --rm -v "%CD%:/repo" -v perseus-gomod:/go/pkg/mod -w /repo/backend -e GOFLAGS=-buildvcs=false %IMG%

echo [1/3] New regression test (agent content policy)...
%RUN% go test ./internal/ai/ -run TestContentOnlyCompletionAfterReadOnlyTools -count=1 -v > "test-results\backend-new-test.log" 2>&1
echo     exit code: %errorlevel%

echo [2/3] go vet ./...
%RUN% go vet ./... > "test-results\backend-vet.log" 2>&1
echo     exit code: %errorlevel%

echo [3/3] Full backend test suite (can take several minutes)...
%RUN% go test ./... -count=1 > "test-results\backend-all-tests.log" 2>&1
echo     exit code: %errorlevel%

echo.
echo Done. Logs are in test-results\ (backend-new-test.log, backend-vet.log, backend-all-tests.log).
echo Tell Claude it finished; no need to copy anything.
echo.
pause
