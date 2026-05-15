# Unified Builder Base Image for Laravel PaaS
# Contains PHP, Composer, Node.js, Pnpm, and Bun

# Stage 1: Get Bun binary from official Alpine-compatible image
FROM oven/bun:alpine AS bun-source

# Stage 2: Main Builder
ARG PHP_VERSION=8.3
FROM php:${PHP_VERSION}-cli-alpine

# Set build arguments
ARG NODE_VERSION=20

# Install system build dependencies
# We include libc6-compat and libstdc++ for binary compatibility
RUN apk add --no-cache \
    git \
    unzip \
    zip \
    curl \
    bash \
    nodejs \
    npm \
    yarn \
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

# Install 'n' node version manager and upgrade Node.js to target version
# This ensures pnpm and other modern tools have the required engine version
RUN npm install -g n && n $NODE_VERSION

# Enable corepack for modern pnpm/yarn management (avoids EBADENGINE errors)
RUN corepack enable && corepack prepare pnpm@latest --activate

# Copy Bun binary from the source stage
COPY --from=bun-source /usr/local/bin/bun /usr/local/bin/bun

# Set working directory
WORKDIR /app

# The build command will be executed dynamically here via multi-stage builds.
