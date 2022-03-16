@echo off

set ROOT_DIR=%~dp0

cd %ROOT_DIR%
cd .. 

set GOBIN=%CD%

set ENABLE_METRICS=true
set CGO_LDFLAGS="-Wl,--export-all-symbols"

for /F %%F in ('type VERSION') do set RELEASE_TAG=%%F
for /F %%F in ('git rev-parse --short HEAD') do set GIT_COMMIT=%%F

set BUILD_FLAGS=-ldflags=" -X github.com/status-im/status-go/params.Version=%RELEASE_TAG% -X github.com/status-im/status-go/params.GitCommit=%GIT_COMMIT% -X github.com/status-im/status-go/vendor/github.com/ethereum/go-ethereum/metrics.EnabledStr=%ENABLE_METRICS%"

md "%GOBIN%\build\bin\statusgo-lib" 2>nul

echo "create `main.go`..."
go run %GOBIN%\cmd\library 2 > %GOBIN%\build\bin\statusgo-lib\main.go

echo "building status-go shared lib..."
go build %BUILD_FLAGS% -buildmode=c-shared -o %GOBIN%\build\bin\libstatus.dll %GOBIN%\build\bin\statusgo-lib
echo "status-go shared lib built..."
