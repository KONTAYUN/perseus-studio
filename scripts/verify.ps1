param(
  [switch]$E2E,
  [switch]$Docker
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

Push-Location $root
try {
  npm run test:ui
  npm run test:structure
  npm run test:unit
  npm run build

  Push-Location (Join-Path $root "backend")
  try {
    go test ./...
  } finally {
    Pop-Location
  }

  if ($Docker) {
    docker compose build
  }

  if ($E2E) {
    npm run test:e2e
  }
} finally {
  Pop-Location
}
