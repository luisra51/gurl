# Dockerfile Guide

This project uses two different Dockerfiles optimized for different use cases:

## Files

### 1. `Dockerfile` (Development)
- **Purpose**: Local development and testing
- **Features**: 
  - Basic multi-stage build
  - Faster builds for development
  - Standard Alpine base
- **Usage**: `docker build -f Dockerfile -t gurl-dev .`

### 2. `Dockerfile.production` (Production)
- **Purpose**: Production builds and GitHub Actions
- **Features**:
  - Multi-platform support (amd64/arm64)
  - Security optimizations (non-root user)
  - Health checks
  - Optimized binary size (stripped symbols)
  - CA certificates for HTTPS
  - Timezone data
- **Usage**: Used automatically by GitHub Actions

## Key Differences

| Feature | Development | Production |
|---------|-------------|------------|
| **Multi-platform** | No | Yes (amd64/arm64) |
| **Security** | Root user | Non-root user |
| **Health checks** | No | Yes |
| **Binary optimization** | Basic | Full (-w -s flags) |
| **CA certificates** | No | Yes |
| **Build time** | Faster | Slower (more features) |

## Usage

### Development
```bash
# Build development image
docker build -t gurl-dev .

# Or use docker-compose
docker-compose up --build
```

### Production (GitHub Actions)
```bash
# Pulls the pre-built image
docker pull luisra51/gurl:latest

# Or use the hub compose file
docker-compose -f docker-compose.hub.yml up -d
```

## Why Two Files?

1. **Development Speed**: The dev Dockerfile is optimized for fast rebuilds during development
2. **Production Security**: The production Dockerfile includes security best practices
3. **Multi-platform**: Production builds support both Intel and ARM architectures
4. **Separation of Concerns**: Different optimizations for different use cases