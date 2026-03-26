param (
    [string]$StudioVersion = "v2026.03.04",
    [string]$ImageTag = "supadash/studio:latest"
)

$ErrorActionPreference = "Stop"
$ScriptDir = $PSScriptRoot
$BuildDir = Join-Path $ScriptDir ".build"

Write-Host "========================================"
Write-Host "  SupaDash Studio Builder (PowerShell)"
Write-Host "  Version: $StudioVersion"
Write-Host "  Image:   $ImageTag"
Write-Host "========================================"

Write-Host "`n[1/6] Cleaning previous build..."
if (Test-Path $BuildDir) {
    Remove-Item -Recurse -Force $BuildDir
}
New-Item -ItemType Directory -Force -Path $BuildDir | Out-Null

Write-Host "[2/6] Downloading Supabase Studio $StudioVersion..."
Set-Location $BuildDir
git clone --depth 1 --branch $StudioVersion --filter=blob:none --sparse https://github.com/supabase/supabase.git supabase-src
if ($LASTEXITCODE -ne 0) {
    Write-Host "⚠ Tag $StudioVersion not found, trying as branch..."
    git clone --depth 1 --filter=blob:none --sparse https://github.com/supabase/supabase.git supabase-src
}
Set-Location supabase-src
git sparse-checkout set apps/studio packages
git checkout $StudioVersion
Write-Host "  > Studio source downloaded"

Write-Host "[3/6] Extracting Studio app..."
Copy-Item -Recurse -Force apps/studio "$BuildDir/studio"
if (Test-Path packages) {
    Copy-Item -Recurse -Force packages "$BuildDir/packages"
}
Write-Host "  > Studio app extracted"

Write-Host "[4/6] Skipping legacy patches..."

Write-Host "[4.5/6] Cleaning up original Next.js API routes..."
if (Test-Path "$BuildDir/studio/pages/api") {
    Remove-Item -Recurse -Force "$BuildDir/studio/pages/api"
}
New-Item -ItemType Directory -Force -Path "$BuildDir/studio/pages/api" | Out-Null

Write-Host "[5/6] Copying replacement files..."
if (Test-Path "$ScriptDir/files") {
    Copy-Item -Recurse -Force "$ScriptDir/files/*" "$BuildDir/studio/"
    Write-Host "  > Replacement files copied"
} else {
    Write-Host "  (no replacement files)"
}

Write-Host "[6/6] Building Docker image: $ImageTag..."
Copy-Item -Force "$ScriptDir/Dockerfile" "$BuildDir/studio/Dockerfile"
Set-Location "$BuildDir/studio"

# We must ensure Docker uses Linux containers and executes successfully
docker build -t $ImageTag .

Write-Host "`n========================================"
Write-Host "  [OK] Build complete!"
Write-Host "  Image: $ImageTag"
Write-Host "  Run:   docker run -p 3000:3000 $ImageTag"
Write-Host "========================================"
