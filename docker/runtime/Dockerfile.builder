# Unified Builder Base Image for Laravel PaaS
# Contains PHP, Composer, Node.js, Pnpm, and Bun

# Global ARGs for multi-stage selection
ARG PHP_VERSION=8.3
ARG NODE_VERSION=20

# Stage 1: Get official Node.js (Alpine version) to ensure musl compatibility
FROM node:${NODE_VERSION}-alpine AS node-source

# Stage 2: Get official Bun (Alpine version)
FROM oven/bun:alpine AS bun-source

# Stage 3: Main Builder
FROM php:${PHP_VERSION}-cli-alpine

# Re-declare ARGs for internal use
ARG PHP_VERSION
ARG NODE_VERSION

# Install system build dependencies (minimal needed for PHP extensions and general building)
RUN apk add --no-cache \
    git \
    unzip \
    zip \
    curl \
    bash \
    python3 \
    make \
    g++ \
    libc6-compat \
    libstdc++ \
    libgcc \
    libpng-dev \
    libjpeg-turbo-dev \
    freetype-dev \
    libzip-dev \
    oniguruma-dev \
    icu-dev

# Install required PHP extensions for Composer builds
RUN docker-php-ext-configure gd --with-freetype --with-jpeg \
    && docker-php-ext-install -j$(nproc) gd zip pdo pdo_mysql mbstring exif pcntl bcmath intl

# Install Composer globally
COPY --from=composer:latest /usr/bin/composer /usr/bin/composer

# --- NODE.JS INTEGRATION (The Reliable Way) ---
# Instead of 'apk add nodejs' or 'n', we copy the verified Alpine binaries
COPY --from=node-source /usr/local/bin/node /usr/local/bin/node
COPY --from=node-source /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && \
    ln -s /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

# --- PACKAGE MANAGERS ---
# Install pnpm and yarn via npm (simpler than corepack for multi-stage setups)
RUN npm install -g pnpm yarn

# Copy Bun binary from the source stage
COPY --from=bun-source /usr/local/bin/bun /usr/local/bin/bun

# Set working directory
WORKDIR /app
