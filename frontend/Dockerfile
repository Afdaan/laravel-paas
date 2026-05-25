# ===========================================================
# Frontend Dockerfile
# ===========================================================
# Multi-stage build for React + Nginx
# ===========================================================

# Stage 1: Build
FROM node:20-alpine AS builder

WORKDIR /app

# Copy package files
COPY package*.json ./
# Install packages leveraging BuildKit NPM local cache
RUN --mount=type=cache,target=/root/.npm \
    npm ci

# Copy source and build
COPY . .
RUN npm run build

# Stage 2: Production with Nginx
FROM nginx:1.27-alpine

LABEL maintainer="Laravel PaaS"
LABEL description="Frontend Dashboard"

ENV TZ=Asia/Jakarta

RUN apk update && apk add --no-cache curl tzdata && \
    cp /usr/share/zoneinfo/Asia/Jakarta /etc/localtime && \
    echo "Asia/Jakarta" > /etc/timezone

# Copy nginx config
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Copy built files
COPY --from=builder /app/dist /usr/share/nginx/html

# Health check
HEALTHCHECK --interval=5s --timeout=3s --start-period=2s --retries=3 \
    CMD curl -f http://localhost/ || exit 1

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
