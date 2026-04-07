# SupaDash Studio — Custom Build
# Based on official Supabase Studio with SupaDash patches applied.

FROM node:20-alpine AS base

# Install dependencies
FROM base AS deps
WORKDIR /app
COPY pnpm-lock.yaml pnpm-workspace.yaml package.json ./
COPY packages/ ./packages/
ARG CACHE_BUST=1
RUN \
  if [ -f yarn.lock ]; then yarn install --no-frozen-lockfile; \
  elif [ -f package-lock.json ]; then npm ci; \
  elif [ -f pnpm-lock.yaml ]; then corepack enable pnpm && pnpm install --no-frozen-lockfile; \
  else echo "No lockfile found" && npm install; \
  fi

# Build
FROM base AS builder
WORKDIR /app
COPY --from=deps /app ./
COPY . .
RUN if [ -d files ]; then cp -R files/* .; fi

# Environment variables for build
ENV NEXT_TELEMETRY_DISABLED=1
ENV NODE_ENV=production
# Prevent OOM during Next.js build on low-memory servers
ENV NODE_OPTIONS="--max-old-space-size=2048"
# Skip Supabase's asset upload script (not needed for self-hosted)
ENV SKIP_ASSET_UPLOAD=1

# SupaDash-specific build args
ARG SUPADASH_API_URL=http://localhost:8080
ENV NEXT_PUBLIC_SUPADASH_API_URL=$SUPADASH_API_URL

# Enable the multi-project platform experience
ENV NEXT_PUBLIC_IS_PLATFORM=true
# Set API URL to be caught by our custom proxy
ENV NEXT_PUBLIC_API_URL=/api
# Disable Sentry (not needed for self-hosted)
ENV SENTRY_SUPPRESS_TURBOPACK_WARNING=1

RUN \
  if [ -f yarn.lock ]; then yarn --cwd packages/ui run generate-styles && yarn build; \
  elif [ -f package-lock.json ]; then npm run generate-styles --prefix packages/ui && npm run build; \
  elif [ -f pnpm-lock.yaml ]; then corepack enable pnpm && pnpm --filter ui run generate-styles && pnpm build; \
  else npm run build; \
  fi

# Production
FROM base AS runner
WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV HOSTNAME=0.0.0.0
ENV PORT=3000

RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nextjs

# Copy built output
COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs

EXPOSE 3000

HEALTHCHECK --interval=5s --timeout=5s --retries=3 \
  CMD node -e "fetch('http://127.0.0.1:3000/api/platform/profile').then(r => { if (r.status !== 200) throw new Error(r.status) })" || exit 1

CMD ["node", "server.js"]
