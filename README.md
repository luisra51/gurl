```
 ██████╗ ██╗   ██╗██████╗ ██╗     
██╔════╝ ██║   ██║██╔══██╗██║     
██║  ███╗██║   ██║██████╔╝██║     
██║   ██║██║   ██║██╔══██╗██║     
╚██████╔╝╚██████╔╝██║  ██║███████╗
 ╚═════╝  ╚═════╝ ╚═╝  ╚═╝╚══════╝
```

# GURL - Go URL Email Crawler

**An intelligent web crawler built in Go that extracts email addresses and social profiles from websites with precision and speed.**

[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8?style=for-the-badge&logo=go)](https://golang.org/)
[![Docker](https://img.shields.io/badge/docker-supported-2496ED?style=for-the-badge&logo=docker)](https://www.docker.com/)
[![Redis](https://img.shields.io/badge/redis-cache-DC382D?style=for-the-badge&logo=redis)](https://redis.io/)
[![License](https://img.shields.io/badge/license-MIT-green?style=for-the-badge)](LICENSE)

> 🚀 **Fast, intelligent, and scalable email discovery for modern web applications**

## ✨ Features

- **🧠 Intelligent Crawling**: Prioritizes contact and information pages
- **🌍 Multi-language Support**: Recognizes keywords in 6 languages (Spanish, English, French, German, Italian, Portuguese)
- **🔄 Meta Redirects**: Automatically follows HTML meta redirects
- **⚡ Redis Cache**: Smart caching with 12-month persistence and 5,400x speed improvement
- **🚀 Async Processing**: Background jobs with webhook notifications
- **🔍 Auto Deduplication**: Automatically removes duplicate emails
- **🌐 Social Discovery**: Extracts social profile links from known platforms
- **🐳 Dockerized**: Easy deployment with Docker Compose
- **📡 REST API**: Both synchronous and asynchronous endpoints
- **⚙️ Configurable Depth**: Explore up to 3 levels deep (configurable)
- **🛡️ Input Guardrails**: Accepts only absolute HTTP/HTTPS URLs
- **📏 Crawl Limits**: Restricts per-page response size and total pages visited
- **♻️ Async Recovery**: Requeues interrupted processing jobs on restart

## ☕ Support

If you like this project, consider [buying me a coffee](https://buymeacoffee.com/luisra51) ☕💛
<a href="https://www.buymeacoffee.com/luisra51" target="_blank">
  <img src="https://cdn.buymeacoffee.com/buttons/v2/default-yellow.png" alt="Buy Me A Coffee" width="200"/>
</a>

## 📋 Requirements

- Docker
- Docker Compose

## 🚀 Quick Start

### Option 1: Use Pre-built Docker Image (Recommended)

```bash
# Pull and run the latest image
docker run -d --name gurl-crawler \
  -p 8080:8080 \
  -p 6379:6379 \
  luisra51/gurl:latest

# Or use with external Redis
docker run -d --name gurl-crawler \
  -p 8080:8080 \
  -e REDIS_HOST=your-redis-host \
  -e REDIS_PORT=6379 \
  luisra51/gurl:latest
```

### Option 2: Clone and Build from Source

```bash
git clone https://github.com/luisra51/gurl.git
cd gurl
docker-compose up --build
```

### 2. Use the API

Service will be available at `http://localhost:8080`

The service assumes authentication, authorization, and rate limiting are handled by an upstream gateway or internal platform layer.

#### **Synchronous Scanning (Immediate Response)**
```bash
# Basic scan
curl "http://localhost:8080/scan?url=example.com"

# With specific protocol
curl "http://localhost:8080/scan?url=https://company.com"
```

**Response:**
```json
{
  "emails": ["info@example.com", "contact@example.com"],
  "social_profiles": [
    {
      "platform": "linkedin",
      "url": "https://linkedin.com/company/example",
      "handle": "example",
      "source_page": "https://company.com",
      "confidence": "high"
    }
  ],
  "from_cache": false,
  "crawl_time": "2.3s"
}
```

#### **Asynchronous Scanning (For Slow URLs)**
```bash
curl -X POST "http://localhost:8080/scan/async" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "slow-website.com",
    "webhook_url": "https://your-api.com/webhook",
    "callback_id": "optional-tracking-id"
  }'
```

**Immediate Response:**
```json
{
  "job_id": "uuid-123-456-789",
  "status": "queued",
  "estimated_time": "30-60s",
  "webhook_url": "https://your-api.com/webhook",
  "check_status_url": "/scan/status/uuid-123-456-789"
}
```

**Webhook Callback (When Complete):**
```json
{
  "job_id": "uuid-123-456-789",
  "callback_id": "optional-tracking-id",
  "status": "completed",
  "url": "https://slow-website.com",
  "emails": ["contact@slow-website.com"],
  "social_profiles": [
    {
      "platform": "instagram",
      "url": "https://instagram.com/slow-website",
      "handle": "slow-website",
      "source_page": "https://slow-website.com",
      "confidence": "high"
    }
  ],
  "crawl_time": "45.2s",
  "pages_visited": 15,
  "completed_at": "2025-08-07T10:30:00Z"
}
```

### 3. Response Types

#### **Success with Emails Found:**
```json
{
  "emails": ["info@example.com", "contact@example.com"],
  "social_profiles": [],
  "from_cache": true,
  "crawl_time": "396µs"
}
```

#### **Success without Emails:**
```json
{
  "emails": [],
  "from_cache": false,
  "crawl_time": "2.1s"
}
```

#### **Error:**
```json
{
  "error": "Invalid URL provided"
}
```

## 🌍 Multi-language Support

The crawler intelligently recognizes contact-related keywords in **6 languages**:

- **🇪🇸 Spanish**: contacto, información, equipo, nosotros, empresa
- **🇺🇸 English**: contact, about, team, support, help, office  
- **🇫🇷 French**: nous-contacter, équipe, aide, assistance, bureau
- **🇩🇪 German**: kontakt, über-uns, impressum, unser-team, hilfe
- **🇮🇹 Italian**: contatti, chi-siamo, squadra, informazioni, supporto
- **🇵🇹 Portuguese**: contato, sobre-nos, equipe, ajuda, suporte

> **43+ keywords** total across all languages for maximum coverage

## 🔌 API Endpoints

### **Synchronous Endpoints**

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/scan?url=<website>` | Scan website (immediate response) |
| `GET` | `/cache/stats` | View Redis cache statistics |
| `DELETE` | `/cache/invalidate` | Clear all cache |
| `DELETE` | `/cache/invalidate?url=<website>` | Clear specific URL cache |

### **Asynchronous Endpoints**

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/scan/async` | Create async scan job |
| `GET` | `/scan/status/<job_id>` | Check job status |
| `DELETE` | `/scan/cancel/<job_id>` | Cancel queued or processing job |
| `GET` | `/scan/jobs` | View active job statistics |

### **Advanced Usage Examples**

```bash
# View cache statistics
curl "http://localhost:8080/cache/stats"

# Check async job status
curl "http://localhost:8080/scan/status/uuid-123-456"

# Cancel queued job
curl -X DELETE "http://localhost:8080/scan/cancel/uuid-123-456"

# View active jobs and statistics
curl "http://localhost:8080/scan/jobs"

# Clear complete cache
curl -X DELETE "http://localhost:8080/cache/invalidate"
```

## ⚙️ Configuration

### **Environment Variables**

```bash
# Crawler Settings
CRAWLER_MAX_DEPTH=3                    # Maximum crawling depth
CRAWLER_DEDUPLICATE_EMAILS=true        # Remove duplicate emails
CRAWLER_MAX_RESPONSE_BYTES=1048576     # Max bytes read per page (1 MiB)
CRAWLER_MAX_PAGES_VISITED=50           # Max pages visited per crawl

# Cache Settings  
CACHE_ENABLED=true                     # Enable Redis cache
CACHE_EXPIRATION_MONTHS=12             # Cache TTL in months

# Async Processing Settings
ASYNC_ENABLED=true                     # Enable async processing
ASYNC_WORKERS=3                        # Number of parallel workers
ASYNC_JOB_TIMEOUT_SECONDS=300          # Job timeout (5 minutes)
ASYNC_WEBHOOK_RETRIES=3                # Webhook retry attempts

# Redis Configuration
REDIS_HOST=localhost                   # Redis host
REDIS_PORT=6379                        # Redis port
REDIS_PERSIST_DISK=false              # Disk persistence (prod: true)

# Server Configuration
SERVER_PORT=8080                       # Server port
SERVER_HOST=0.0.0.0                    # Server host
SERVER_READ_HEADER_TIMEOUT_SECONDS=5   # Read header timeout
SERVER_READ_TIMEOUT_SECONDS=15         # Full request read timeout
SERVER_WRITE_TIMEOUT_SECONDS=30        # Response write timeout
SERVER_IDLE_TIMEOUT_SECONDS=60         # Keep-alive idle timeout
```

### **How It Works**

- **🎯 Smart Crawling**: Prioritizes contact pages with multilingual keywords
- **📊 Depth Control**: Configurable depth (default: 3 levels)
- **📏 Resource Limits**: Parses only HTML, caps page size, and caps total pages visited
- **🌐 Social Platforms**: LinkedIn, X/Twitter, Instagram, Facebook, YouTube, TikTok, GitHub, Telegram, WhatsApp, Linktree
- **⚡ Cache System**: Redis-based caching with 12-month TTL
- **🔄 Auto Deduplication**: Automatic email normalization and deduplication
- **🚀 Performance**: 5,400x faster responses with cache hits
- **♻️ Restart Recovery**: Async jobs left in `processing` are requeued on startup

## Input Rules

- Scan targets must resolve to absolute `http://` or `https://` URLs.
- Async webhook targets must also be absolute `http://` or `https://` URLs.
- Bare domains such as `example.com` are normalized to `https://example.com`.

## Social Profiles

Each discovered social profile includes:

- `platform`: normalized platform name
- `url`: canonical normalized profile URL
- `handle`: optional value derived from the URL path
- `source_page`: crawled page where the link was found
- `confidence`: simple heuristic signal (`high`, `medium`, `low`)

## TODO

- Improve social profile quality heuristics with more platform-specific filters, canonicalization rules, and confidence signals.

## 🏗️ Project Architecture

```
/
├── .env                     # Environment variables (development)
├── .env.example             # Configuration example
├── go.mod                   # Go dependencies
├── Dockerfile               # Container definition
├── docker-compose.yml       # Redis + App services
├── scan_urls.sh             # Batch processing script
├── cmd/
│   └── crawler/
│       └── main.go          # Application entry point
└── internal/
    ├── cache/
    │   └── cache.go         # Redis cache management
    ├── config/
    │   └── config.go        # Environment configuration
    ├── crawler/
    │   └── crawler.go       # Core crawling logic
    ├── handler/
    │   └── handler.go       # HTTP endpoints (sync + async)
    └── jobs/
        ├── types.go         # Job data types
        ├── queue.go         # Redis job queue
        └── worker.go        # Worker system + webhooks
```

### **Core Components**

- **🗄️ Cache Layer**: Redis with configurable TTL and optional persistence
- **⚙️ Job Queue**: Redis-based async system with parallel workers
- **📡 Webhook System**: Result delivery with retries and exponential backoff
- **🌐 Multi-language**: 43+ keywords across 6 languages
- **🔧 Config Management**: Environment-based configuration

## 🔧 Development

### **With Docker (Recommended)**

```bash
# Copy environment variables
cp .env.example .env

# Start complete stack
docker-compose up --build
```

### **Without Docker**

```bash
# Install Redis locally
# Ubuntu/Debian: sudo apt install redis-server
# macOS: brew install redis

# Start Redis
redis-server

# Install Go dependencies
go mod tidy

# Run application
go run cmd/crawler/main.go
```

## 🤝 Contributing

We welcome contributions! Here's how you can help:

### **Ways to Contribute**

- 🐛 **Bug Reports**: Found a bug? [Open an issue](https://github.com/your-username/gurl/issues)
- ✨ **Feature Requests**: Have an idea? [Start a discussion](https://github.com/your-username/gurl/discussions)
- 📝 **Documentation**: Improve docs, add examples, fix typos
- 🌍 **Translations**: Add support for more languages
- 🧪 **Testing**: Write tests, test edge cases
- 💻 **Code**: Implement new features or fix bugs

### **Development Setup**

1. **Fork the repository**
2. **Clone your fork**:
   ```bash
   git clone https://github.com/your-username/gurl.git
   cd gurl
   ```
3. **Create a feature branch**:
   ```bash
   git checkout -b feature/amazing-feature
   ```
4. **Make your changes**
5. **Test your changes**:
   ```bash
   docker-compose up --build
   # Test your changes
   ```
6. **Commit and push**:
   ```bash
   git commit -m "Add amazing feature"
   git push origin feature/amazing-feature
   ```
7. **Open a Pull Request**

### **Code Style**

- Follow standard Go conventions (`go fmt`, `go vet`)
- Add tests for new features
- Update documentation for API changes
- Use meaningful commit messages


## 📝 Limitations

- **JavaScript**: Does not execute JavaScript, only analyzes static HTML
- **Single Page Applications**: Limited on SPAs that load content dynamically  
- **Rate limiting**: Does not implement throttling between requests
- **Same domain**: Only crawls pages from the same base domain

## 🚀 Use Cases

- **💼 Lead Generation**: Find contact emails from company websites
- **🔍 Research Automation**: Collect contact information at scale
- **📊 Competitive Analysis**: Study competitor contact pages
- **🔗 API Integration**: Integrate with CRMs via webhooks
- **📦 Batch Processing**: Process thousands of URLs with `scan_urls.sh`
- **🏗️ Microservices**: Email discovery service for distributed architectures

## 🐳 Docker

### **Using Docker Hub Image (Production)**

[![Docker Hub](https://img.shields.io/docker/v/luisra51/gurl?label=Docker%20Hub&logo=docker)](https://hub.docker.com/r/luisra51/gurl)
[![Docker Pulls](https://img.shields.io/docker/pulls/luisra51/gurl?logo=docker)](https://hub.docker.com/r/luisra51/gurl)

```bash
# Single container (no Redis persistence)
docker run -d --name gurl-crawler \
  -p 8080:8080 \
  luisra51/gurl:latest

# With Docker Compose (includes Redis)
docker-compose -f docker-compose.hub.yml up -d

# Production with external Redis
docker run -d --name gurl-crawler \
  -p 8080:8080 \
  -e REDIS_HOST=your-redis-host \
  -e REDIS_PORT=6379 \
  -e REDIS_PERSIST_DISK=true \
  -e ASYNC_WORKERS=5 \
  -e CACHE_EXPIRATION_MONTHS=12 \
  luisra51/gurl:latest
```

### **Development (from source)**
```bash
# Quick development (no persistence)
docker-compose up --build

# Fast rebuilds
docker-compose up --build crawler-app

# Clean and start fresh
docker-compose down -v && docker-compose up --build
```

### **Manual build**
```bash
docker build -t email-crawler .
docker run -p 8080:8080 email-crawler
```

## 🔍 Monitoring and Debugging

```bash
# View cache statistics
curl "http://localhost:8080/cache/stats"

# View worker and job status
curl "http://localhost:8080/scan/jobs"

# Application logs
docker-compose logs -f crawler-app

# Redis logs
docker-compose logs -f redis

# Enter container for debugging
docker-compose exec crawler-app sh
```
