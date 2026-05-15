# Unified Builder Base Image for Laravel PaaS
# Contains PHP, Composer, and Node.js (with npm, yarn, pnpm)
ARG PHP_VERSION=8.3
FROM php:${PHP_VERSION}-cli-alpine

# Set build arguments
ARG NODE_VERSION=20

# Install system build dependencies, Node.js, and Composer
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

# Install pnpm, bun, and n (node version manager) globally
RUN npm install -g pnpm bun n

# Set working directory
WORKDIR /app

# The build command will be executed dynamically here via multi-stage builds.
