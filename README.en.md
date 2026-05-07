# Moonlight Box

English | [简体中文](README.md)

Enterprise-grade multi-protocol package registry management system with proxy, caching, hosting, and security scanning capabilities.

## Features

- **Multi-Protocol Support:** Built-in adapters for npm, Maven, PyPI, Go Modules, NuGet, YUM, APT, and other mainstream package registries
- **Smart Proxy Routing:** Intelligent routing between local and remote registries with automatic fallback and cache acceleration
- **AI Assistant:** Integrated LLM support with tools for log querying, database queries, package info lookup, security analysis, and more
- **Security Scanning:** Automatic package scanning on upload with blocking of critical and high-severity vulnerabilities
- **Access Control:** Role-based access control (RBAC) with CAS single sign-on support
- **Multi-Storage Backend:** Local filesystem and Amazon S3 object storage support
- **Multi-Database Support:** SQLite (default) and PostgreSQL
- **Observability:** Prometheus metrics collection and structured logging
- **Data Migration:** Migration support from Nexus Repository Manager

## Quick Start

### Prerequisites

- Go >= 1.26
- Node.js >= 20 (for frontend build)
- SQLite or PostgreSQL

### Installation

```bash
# Clone the repository
git clone https://github.com/moonlight-box/moonlight-box.git
cd moonlight-box

# Build
make build

# Or run directly
make run
```

### Configuration

Copy the example configuration file and modify it according to your needs:

```bash
cp configs/config.example.yaml configs/config.yaml
```

Core configuration options:

| Option | Description | Default |
|--------|-------------|---------|
| `server.port` | Server port | 9081 |
| `database.driver` | Database driver (sqlite / postgres) | sqlite |
| `storage.backend` | Storage backend (local / s3) | local |
| `ai.enabled` | Enable AI assistant | false |
| `cache.enabled` | Enable caching | true |
| `security.enabled` | Enable security scanning | true |

### Running

```bash
# Start with default configuration
./bin/moonlight-box

# Start with custom configuration file
./bin/moonlight-box -config configs/config.yaml

# Show version
./bin/moonlight-box -version
```

After starting, access the application at `http://localhost:9081`.

## Architecture

### Core Modules

```
┌─────────────────────────────────────────────────────────────┐
│                        Moonlight Box                         │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │  Adapter  │  │  Proxy   │  │   AI     │  │ Security │   │
│  │  Layer    │  │  Router  │  │ Service  │  │  Scanner │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘   │
│         │              │             │             │        │
│  ┌──────┴──────────────┴─────────────┴─────────────┴──────┐ │
│  │                  Core Services                          │ │
│  │  (Auth / RBAC / Cache / Storage / Migration)           │ │
│  └────────────────────────────────────────────────────────┘ │
│                            │                                │
│  ┌─────────────────────────┴──────────────────────────────┐ │
│  │              Data Layer                                 │ │
│  │  (SQLite / PostgreSQL)  +  (Local FS / S3)             │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### Adapter Architecture

The system uses the Adapter interface to unify protocol differences across package registries:

- **npm Adapter:** npm registry protocol support
- **Maven Adapter:** Maven repository protocol support
- **PyPI Adapter:** Python Package Index protocol support
- **Go Modules Adapter:** Go module proxy protocol support
- **NuGet Adapter:** NuGet OData protocol support
- **YUM/APT Adapter:** Linux package manager protocol support
- **Generic Adapter:** Custom package format support

### Proxy Routing

The Proxy Router intelligently routes package requests to the correct source:

1. **Local First:** Prioritize local registry lookup
2. **Proxy Fallback:** Automatically fetch from configured remote registries on local miss
3. **Cache Acceleration:** Automatically cache fetched results to reduce redundant requests
4. **Health Check:** Periodically check remote registry availability

## AI Assistant

The system includes a built-in AI assistant with the following tools:

| Tool | Description |
|------|-------------|
| `log_query` | Query system logs |
| `db_query` | Execute database queries |
| `package_info` | Query package information and dependencies |
| `security_analysis` | Security vulnerability analysis |
| `code_generator` | Code generation |

Configuration example:

```yaml
ai:
  enabled: true
  provider: "chatglm"  # chatglm, qwen, custom
  base_url: "http://localhost:8000/v1"
  api_key: "your-api-key"
  model: "chatglm3-6b"
```

## Development Guide

### Common Commands

```bash
# Build
make build

# Run
make run

# Run tests
make test

# Generate test coverage report
make test-coverage

# Lint code
make lint

# Clean build artifacts
make clean

# Development mode (hot reload)
make dev

# Build frontend and embed
make embed-web
```

### Project Structure

```
├── cmd/registry/          # Main application entry point
├── configs/               # Configuration files
├── internal/
│   ├── adapter/           # Package registry adapter layer
│   ├── ai/                # AI service
│   ├── config/            # Configuration management
│   ├── database/          # Database initialization and migration
│   ├── handler/           # HTTP handlers
│   ├── middleware/        # Middleware (auth, logging, rate limiting, etc.)
│   ├── migration/         # Data migration service
│   ├── model/             # Data models
│   ├── proxy/             # Proxy routing and caching
│   ├── repository/        # Data access layer
│   ├── response/          # Unified response format
│   └── service/           # Business service layer
├── web/                   # Frontend project (Vue 3 + TypeScript)
└── Makefile
```

### Adding a New Package Adapter

Implement the `Adapter` interface to add support for a new package type:

```go
type Adapter interface {
    Type() types.PackageType
    RoutePrefix() string
    RegisterRoutes(r *gin.RouterGroup, ...)
    ParsePackagePath(path string) (*types.PackageIdentity, error)
    Upload(ctx context.Context, req *types.UploadRequest) (*types.PackageVersionResult, error)
    Download(ctx context.Context, identity *types.PackageIdentity) (*types.PackageContent, error)
    GetMetadata(ctx context.Context, name string) (*types.PackageMeta, error)
    Delete(ctx context.Context, identity *types.PackageIdentity) error
    ListVersions(ctx context.Context, name string) ([]string, error)
}
```

## License

[MIT](./LICENSE)
