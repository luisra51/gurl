# Docker Hub Setup for GitHub Actions

This document explains how to set up automatic Docker image publishing to Docker Hub.

## Prerequisites

1. **Docker Hub Account**: Create an account at https://hub.docker.com
2. **GitHub Repository**: Fork or clone this repository to your GitHub account

## Setup Steps

### 1. Create Docker Hub Repository

1. Log into Docker Hub
2. Click "Create Repository"
3. Name: `gurl`
4. Visibility: Public
5. Click "Create"

Your image will be available as: `luisra51/gurl`

### 2. Create Docker Hub Access Token

1. Go to Docker Hub → Account Settings → Security
2. Click "New Access Token"
3. Description: "GitHub Actions GURL"
4. Permissions: "Read, Write, Delete"
5. Click "Generate Token"
6. **Copy the token** - you won't see it again!

### 3. Add GitHub Secrets

1. Go to your GitHub repository
2. Settings → Secrets and variables → Actions
3. Click "New repository secret"
4. Add these secrets:

   **DOCKER_USERNAME**
   ```
   luisra51
   ```

   **DOCKER_PASSWORD**
   ```
   [paste your Docker Hub access token here]
   ```

### 4. Trigger the Build

The workflow will automatically trigger when you:
- Push to `main` or `master` branch
- Create a new tag (e.g., `v1.0.0`)
- Create a pull request

### 5. Using the Published Image

Once the workflow completes, your image will be available:

```bash
# Pull the latest image
docker pull luisra51/gurl:latest

# Run with Docker Compose
docker-compose -f docker-compose.hub.yml up -d

# Run standalone
docker run -d --name gurl-crawler -p 8080:8080 luisra51/gurl:latest
```

## Available Tags

The workflow creates these tags automatically:

- `latest` - Latest build from main/master branch
- `v1.0.0` - Specific version tags
- `v1.0` - Major.minor tags
- `v1` - Major version tags

## Multi-platform Support

The image is built for both:
- `linux/amd64` (Intel/AMD processors)
- `linux/arm64` (ARM processors, Apple Silicon, Raspberry Pi)

## Troubleshooting

### Workflow Fails
- Check that `DOCKER_USERNAME` and `DOCKER_PASSWORD` secrets are set correctly
- Verify Docker Hub token has "Read, Write, Delete" permissions
- Ensure repository name matches exactly: `luisra51/gurl`

### Image Not Found
- Workflow must complete successfully first
- Check GitHub Actions tab for build status
- Verify tag/branch triggered the workflow

### Permission Denied
- Regenerate Docker Hub access token
- Update `DOCKER_PASSWORD` secret in GitHub
- Ensure token has correct permissions