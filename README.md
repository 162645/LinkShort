# URL Shortener - Production-Ready Microservices Implementation

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=for-the-badge&logo=go)](https://golang.org/)
[![Go Micro](https://img.shields.io/badge/Go_Micro-v5-blue?style=for-the-badge)](https://go-micro.dev/)
[![NATS](https://img.shields.io/badge/NATS-Transport-green?style=for-the-badge&logo=nats)](https://nats.io/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-Database-336791?style=for-the-badge&logo=postgresql)](https://postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-Cache-DC382D?style=for-the-badge&logo=redis)](https://redis.io/)
[![ClickHouse](https://img.shields.io/badge/ClickHouse-Analytics-FFCC02?style=for-the-badge&logo=clickhouse)](https://clickhouse.com/)
[![Docker](https://img.shields.io/badge/Docker-Containerized-2496ED?style=for-the-badge&logo=docker)](https://docker.com/)
[![Prometheus](https://img.shields.io/badge/Prometheus-Monitoring-E6522C?style=for-the-badge&logo=prometheus)](https://prometheus.io/)
[![Grafana](https://img.shields.io/badge/Grafana-Dashboards-F46800?style=for-the-badge&logo=grafana)](https://grafana.com/)
[![Jaeger](https://img.shields.io/badge/Jaeger-Tracing-60A5FA?style=for-the-badge)](https://jaegertracing.io/)

## 📋 Table of Contents

- [🎯 Project Overview](#-project-overview)
- [🏗️ System Architecture](#️-system-architecture)
- [✨ Key Features](#-key-features)
- [🚀 Quick Start](#-quick-start)
- [📖 API Documentation](#-api-documentation)
- [🔧 Development Guide](#-development-guide)
- [📊 Monitoring & Observability](#-monitoring--observability)

## 🎯 Project Overview

A **production-ready URL shortener** built with **Go Micro v5 microservices architecture**, demonstrating modern software engineering practices and comprehensive observability. This project serves as a reference implementation for building scalable, maintainable microservices systems in Go.

### Product Screenshots

<div align="center">
  <img src="images/linkshort-home.png" alt="LinkShort user interface" width="900"/>
  <p><em>LinkShort 用户端：输入原始链接并生成短链接</em></p>
  <img src="images/linkshort-created.png" alt="Generated short link" width="900"/>
  <p><em>生成成功：短链接、二维码和创建时间</em></p>
  <img src="images/linkshort-custom-alias.png" alt="Custom alias short link" width="900"/>
  <p><em>自定义短码：支持品牌化短链接</em></p>
</div>

### 🎪 **Live Demo & Testing**

| Component | Access Point | Description |
|-----------|--------------|-------------|
| **🌐 REST API** | `http://localhost:8080` | Main API gateway |
| **📖 Interactive Swagger UI** | `http://localhost:8080/docs/index.html` | Try APIs in browser |
| **🏠 API Documentation** | `http://localhost:8080/` | Beautiful landing page |
| **📊 Prometheus Metrics** | `http://localhost:9090` | Metrics collection |
| **📈 Grafana Dashboards** | `http://localhost:3000` | Visual monitoring |
| **🔍 Jaeger Tracing** | `http://localhost:16686` | Distributed tracing |
| **📊 ClickHouse Analytics** | `http://localhost:8123` | Real-time analytics database |

### 🎯 **Business Value**

- **📈 Scalability**: Horizontal scaling via microservices architecture
- **⚡ Performance**: Redis caching with 95%+ cache hit ratio
- **🔍 Observability**: Complete monitoring stack with Prometheus, Grafana, and Jaeger
- **📊 Analytics**: Real-time click tracking with ClickHouse time-series storage
- **🛡️ Reliability**: Production-ready patterns with circuit breakers and health checks
- **🚀 Developer Experience**: Interactive API documentation and comprehensive testing

## 🏗️ System Architecture

<div align="center">
  <img src="images/architecture.png" alt="URL Shortener Microservices Architecture" width="900"/>
  <p><em>🏗️ Complete microservices architecture with Go Micro, NATS, and comprehensive observability stack</em></p>
</div>

### 🔄 **Service Communication Flow**

```
External Client → REST API → NATS Discovery → RPC Services → Database/Cache
     ↓              ↓             ↓              ↓              ↓
HTTP Request → gRPC Client → NATS Transport → gRPC Handler → Business Logic
```

### 🏗️ **Microservices Overview**

| Service | Port | Responsibility | Technology Stack |
|---------|------|----------------|------------------|
| **REST API Service** | 8080 | HTTP gateway, API documentation | Gin, Swagger, NATS discovery |
| **URL Shortener RPC** | 50051 | Core business logic | Go Micro, Protocol Buffers |
| **Analytics Service** | 50052 | Real-time analytics processing | ClickHouse, NATS events |
| **Redirect Service** | 50053 | URL resolution, click tracking | Go Micro, Redis cache |

### 🗄️ **Tech Stack & Data Architecture**

#### **Core Frameworks**
- **Go 1.23+**: High-concurrency runtime with optimized memory management.
- **Go Micro v5**: Professional microservices framework handling service abstraction and client-side load balancing.
- **Gin Web Framework**: High-performance HTTP routing for the REST Gateway.

#### **Message & Discovery**
- **NATS Server**: 
  - **Service Discovery**: Decouples service locations from code.
  - **Transport**: Low-latency binary communication between services.
  - **NATS JetStream**: Reliable event streaming for analytics data.

#### **High-Performance Storage**
- **PostgreSQL**: Primary ACID-compliant storage for URL mappings and user data.
- **Redis (Cluster Ready)**:
  - **L2 Cache**: Multi-node data synchronization.
  - **Bloom Filter**: Memory-efficient probabilistic filter for anti-penetration.
  - **Atomic Counter**: High-performance ID generation for Base62 encoding.
- **ClickHouse**: OLAP database for real-time, high-volume click analytics and time-series reporting.

#### **Observability (The "Gold Standard")**
- **Prometheus**: Multi-dimensional data model with advanced PromQL for performance tracking.
- **Grafana**: Enterprise dashboards for business KPIs and system health.
- **Jaeger**: OpenTelemetry-compatible distributed tracing for identifying latency bottlenecks.

## ✨ Key Features

### 🎯 **Core Functionality**
- ✅ **URL Shortening** with custom algorithms and validation
- ✅ **Custom Aliases** for branded short links
- ✅ **User Management** with personal URL collections
- ✅ **Expiration Handling** with automatic cleanup
- ✅ **Click Tracking** with real-time analytics
- ✅ **Multi-level Caching** for sub-millisecond redirects

### 📊 **Analytics & Monitoring**
- ✅ **Real-time Click Analytics** with ClickHouse
- ✅ **Business KPI Dashboards** in Grafana
- ✅ **Distributed Tracing** with Jaeger
- ✅ **Prometheus Metrics** for all services

## 🧠 Technical Deep Dive

### ⚡ **Multi-level Redirection Flow**
To achieve sub-5ms latency, the `redirect-svc` implements a sophisticated retrieval pipeline:
1.  **L1 Cache**: In-process memory (`go-cache`) for hyper-hot URLs (Latency: ~10μs).
2.  **Bloom Filter**: Redis-backed bitset to intercept 100% of non-existent codes before reaching the database.
3.  **Singleflight**: Coalesces concurrent requests for the same cold URL into a single DB query to prevent thundering herd.
4.  **L2 Cache**: Distributed Redis cluster for cross-node consistency (Latency: ~1ms).
5.  **Database**: PostgreSQL indexed fallback.

### 🔢 **Short Code Generation Logic**
The system uses a **Redis-based counter + Base62 encoding** strategy:
- **Seed Offset**: Counter starts at `10,000,000,000`.
- **Length Expansion**: 
  - **6 Characters**: Counter range `[10B, 56.8B]` (Base62: $62^6 \approx 56.8B$).
  - **7 Characters**: Automatically expands to 7 chars once the counter exceeds `56,800,235,583`.
- **Reliability**: Guarantees zero collisions and supports up to **3.5 trillion** unique URLs.

### 🧩 **Asynchronous Architecture**
All analytics data is published to a **NATS JetStream** subject. The `analytics-svc` consumes these events out-of-band to:
- Persistent click logs in **ClickHouse**.
- Update real-time Grafana dashboards.
- This ensures that the user's redirection experience is never blocked by database writes.

## 🚀 Quick Start

### 📋 **Prerequisites**
- **Go 1.23+**
- **Docker & Docker Compose**
- **Protocol Buffers compiler** (`protoc`)
- **Make** (for automation)

### ⚡ **One-Command Setup**
```bash
# Complete setup and run
make setup && make run-all
```

### 🔧 **Step-by-Step Setup**

#### 1️⃣ **Clone and Install Dependencies**
```bash
git clone https://github.com/go-systems-lab/go-url-shortener.git
cd go-url-shortener
make deps
```

#### 2️⃣ **Generate Code and Documentation**
```bash
make proto    # Generate Protocol Buffers
make swagger  # Generate API documentation
```

#### 3️⃣ **Start Infrastructure**
```bash
make dev-up        # PostgreSQL + Redis + ClickHouse
make setup-nats    # NATS server
```

#### 4️⃣ **Build and Run Services**
```bash
make build-all

# Terminal 1: RPC Service
make run-rpc

# Terminal 2: Redirect Service  
make run-redirect

# Terminal 3: REST API Service
PORT=8080 make run-rest

# Terminal 4: Analytics Service
make run-analytics
```

#### 5️⃣ **Start Monitoring Stack**
```bash
make start-monitoring  # Prometheus + Grafana + Jaeger
```

### 🎉 **Verification**
```bash
# Test API health
make health

# Run comprehensive API tests
make test-api

# Open interactive documentation
make demo-swagger
```

## 📖 API Documentation

### 🌟 **Interactive Swagger UI**

Our REST API includes **comprehensive OpenAPI 3.0 documentation** with interactive testing capabilities.

#### **📱 Access Points**
- **🔗 Swagger UI**: `http://localhost:8080/docs/index.html`
- **📄 OpenAPI Spec**: `http://localhost:8080/docs/doc.json`
- **🏠 Landing Page**: `http://localhost:8080/`

#### **🎯 Core Endpoints**

| Method | Endpoint | Description | Example |
|--------|----------|-------------|---------|
| `POST` | `/api/v1/shorten` | Create short URL | [Try it →](http://localhost:8080/docs/index.html) |
| `GET` | `/api/v1/urls/{shortCode}` | Get URL info | [Try it →](http://localhost:8080/docs/index.html) |
| `GET` | `/{shortCode}` | Redirect to long URL | Direct browser access |
| `GET` | `/api/v1/users/{userID}/urls` | List user URLs | [Try it →](http://localhost:8080/docs/index.html) |
| `DELETE` | `/api/v1/urls/{shortCode}` | Delete URL | [Try it →](http://localhost:8080/docs/index.html) |

#### **📊 Analytics Endpoints**

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/analytics/dashboard` | System metrics |
| `GET` | `/api/v1/analytics/urls/{shortCode}` | URL-specific analytics |
| `GET` | `/api/v1/analytics/top-urls` | Most popular URLs |

### 🧪 **Quick API Testing**

```bash
# Create a short URL
curl -X POST http://localhost:8080/api/v1/shorten \
  -H "Content-Type: application/json" \
  -d '{
    "long_url": "https://example.com",
    "user_id": "user123",
    "custom_code": "mylink"
  }'

# Test redirect
curl -L http://localhost:8080/mylink

# Get analytics
curl http://localhost:8080/api/v1/analytics/urls/mylink
```

## 🔧 Development Guide

### 🏗️ **Project Structure**

```
go-url-shortener/
├── 📁 services/                 # Microservices
│   ├── 📁 rest-api-svc/        # HTTP gateway
│   ├── 📁 url-shortener-svc/   # Core business logic
│   ├── 📁 redirect-svc/        # URL resolution
│   └── 📁 analytics-svc/       # Real-time analytics
├── 📁 proto/                   # Protocol Buffers
├── 📁 utils/                   # Shared utilities
├── 📁 database/                # Database migrations
├── 📁 infrastructure/          # Docker & configs
├── 📄 Makefile                 # Automation scripts
└── 📄 README.md               # This file
```

### 🛠️ **Development Commands**

```bash
# 🔧 Code Generation
make proto              # Generate Protocol Buffers
make swagger            # Update API documentation

# 🏗️ Building
make build-all          # Build all services
make build-rest         # Build REST API only
make build-rpc          # Build RPC service only

# 🧪 Testing
make test               # Run all tests (26/26)
make test-integration   # Integration tests
make test-api           # API endpoint tests

# 🚀 Running Services
make run-all            # Start all services
make run-rest           # REST API service
make run-rpc            # RPC service
make run-redirect       # Redirect service
make run-analytics      # Analytics service

# 🔍 Monitoring
make logs               # View service logs
make health             # Check service health
make metrics            # View Prometheus metrics
```

### 🔄 **Development Workflow**

1. **Make Changes** to source code
2. **Regenerate** Protocol Buffers: `make proto`
3. **Update** API docs: `make swagger`
4. **Build** services: `make build-all`
5. **Test** changes: `make test`
6. **Run** locally: `make run-all`
7. **Verify** via Swagger UI

## 📊 Monitoring & Observability

Our comprehensive observability stack provides real-time insights into system performance, user behavior, and service health. Experience **enterprise-grade monitoring** with beautiful dashboards and distributed tracing.

### 🚀 **Accessing Monitoring Tools**

```bash
# Start complete monitoring stack
make start-monitoring

# Access dashboards
open http://localhost:9090    # Prometheus
open http://localhost:3000    # Grafana (admin/admin)  
open http://localhost:16686   # Jaeger tracing

# Generate sample data
make generate-traffic
```

### 📈 **Live Monitoring Dashboards**

#### 🎯 **Prometheus Metrics Collection**
Real-time metrics collection and alerting for all microservices with custom business KPIs.

<div align="center">
  <img src="images/prometheus-metrics.png" alt="Prometheus Metrics Dashboard" width="800"/>
  <p><em>📊 Prometheus metrics showing service health, request rates, and custom business metrics</em></p>
</div>

---

#### 📊 **Grafana Business Intelligence**
Beautiful, interactive dashboards providing insights into system performance and user engagement.

<div align="center">
  <img src="images/grafana-dashboard-1.png" alt="Grafana Dashboard - System Overview" width="800"/>
  <p><em>🎛️ System Overview: Service health, response times, and resource utilization</em></p>
</div>

<div align="center">
  <img src="images/grafana-dashboard-2.png" alt="Grafana Dashboard - Business KPIs" width="800"/>
  <p><em>📈 Business KPIs: URL creation rates, click analytics, and user engagement metrics</em></p>
</div>

---

#### 🔍 **Jaeger Distributed Tracing**
End-to-end request tracing across all microservices for performance optimization and debugging.

<div align="center">
  <img src="images/jaeger-traces.png" alt="Jaeger Distributed Tracing" width="800"/>
  <p><em>🕸️ Distributed tracing showing request flow through microservices with timing analysis</em></p>
</div>

---

### 📊 **Key Business Metrics**

| Metric | Description | Dashboard Panel |
|--------|-------------|-----------------|
| **URL Creation Rate** | New URLs created per minute | Business KPIs |
| **Click-Through Rate** | Successful redirects per minute | User Engagement |
| **Cache Hit Ratio** | Redis cache effectiveness | Performance |
| **Error Rate** | Failed requests percentage | Service Health |
| **Response Time** | API latency percentiles | Performance |

⭐ **Star this repository** if you find it helpful!

🔗 **Connect with us**: [GitHub Issues](https://github.com/go-systems-lab/go-url-shortener/issues) | [Discussions](https://github.com/go-systems-lab/go-url-shortener/discussions)

**Made with ❤️ by the Go Systems Lab team**
