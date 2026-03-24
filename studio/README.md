# SupaDash Studio

> Custom build of official Supabase Studio, patched to work with SupaDash Go API.

## How It Works

```
Official Supabase Studio Source
        ↓ (download)
   Apply patches/ 
        ↓ (01-api-urls, 02-auth, 03-cloud, 04-branding)
   Copy files/ 
        ↓ (logos, icons)
   Docker Build
        ↓
  supadash/studio:latest
```

## Build

```bash
# Build with default version
./build.sh

# Build specific version
./build.sh v2026.03.04 supadash/studio:v2026.03.04

# Run locally
docker run -p 3000:3000 \
  -e SUPADASH_API_URL=http://host.docker.internal:8080 \
  supadash/studio:latest
```

## Directory Structure

```
studio/
├── build.sh              # Main build script
├── patch.sh              # Applies patches to Studio source
├── Dockerfile            # Multi-stage Docker build
├── .env.example          # Environment variable reference
├── patches/              # Unified diff patches
│   ├── 01-api-urls.patch
│   ├── 02-auth-integration.patch
│   ├── 03-remove-cloud-features.patch
│   └── 04-branding.patch
└── files/                # Files copied into Studio (overrides)
    └── public/
        └── img/
            ├── supadash-icon.png
            ├── supadash-full-white.png
            └── supadash-full-black.png
```

## Upgrading Studio Version

1. Change the version in `build.sh` or pass as argument
2. Run `./build.sh <new-version>`
3. If patches fail, manually update them for the new source
4. Test the built image locally before deploying
