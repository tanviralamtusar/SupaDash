# SupaDash Studio — Custom Build
# Based on official Supabase Studio with SupaDash patches applied.

FROM node:20-alpine AS base

# Install dependencies
FROM base AS deps
WORKDIR /app
COPY package.json package-lock.json* yarn.lock* pnpm-lock.yaml* ./
RUN \
  if [ -f yarn.lock ]; then yarn install --frozen-lockfile; \
  elif [ -f package-lock.json ]; then npm ci; \
  elif [ -f pnpm-lock.yaml ]; then corepack enable pnpm && pnpm install --frozen-lockfile; \
  else echo "No lockfile found" && npm install; \
  fi

# Build
FROM base AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .

# Environment variables for build
ENV NEXT_TELEMETRY_DISABLED=1
ENV NODE_ENV=production

# SupaDash-specific build args
ARG SUPADASH_API_URL=http://localhost:8080
ENV NEXT_PUBLIC_SUPADASH_API_URL=$SUPADASH_API_URL

# Enable the multi-project platform experience
ENV NEXT_PUBLIC_IS_PLATFORM=true
# Set API URL to be caught by our custom proxy
ENV NEXT_PUBLIC_API_URL=/api

RUN \
  if [ -f yarn.lock ]; then yarn build; \
  elif [ -f package-lock.json ]; then npm run build; \
  elif [ -f pnpm-lock.yaml ]; then corepack enable pnpm && pnpm build; \
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
