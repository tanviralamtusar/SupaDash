# SupaDash Studio — Patches

This directory contains `.patch` files applied to the official Supabase Studio source.

Patches are applied **in alphabetical order** by `patch.sh`.

## Patch Files

| # | File | Purpose |
|---|------|---------|
| 01 | `01-api-urls.patch` | Redirect all API calls from Supabase Cloud to SupaDash Go API |
| 02 | `02-auth-integration.patch` | Use SupaDash JWT auth instead of GoTrue |
| 03 | `03-remove-cloud-features.patch` | Remove Stripe billing, cloud regions, support tickets |
| 04 | `04-branding.patch` | Replace logos, title, favicon with SupaDash branding |

## Creating New Patches

When upgrading to a newer Studio version:

1. Apply existing patches — they may work cleanly
2. If a patch fails, manually edit the new source to achieve the same effect
3. Generate a new patch: `diff -ruN original/ modified/ > patches/XX-name.patch`
4. Test the full build: `./build.sh <new-version>`

## Patch Format

All patches use unified diff format (`diff -ruN`), applied with `patch -p1`.
