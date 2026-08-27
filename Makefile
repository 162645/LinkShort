# URL Shortener Go Micro Makefile
.PHONY: help deps proto clean test build-all build-rpc build-rest build-analytics build-redirect run-rpc run-rest run-analytics run-redirect run-all stop-all test-api test-api-comprehensive test-rpc-internal demo-api setup-nats stop-nats video-demo setup-protoc swagger docs demo-comprehensive demo-swagger open-swagger open-docs restart-all restart-and-test quick-test demo-full reset-environment generate-business-traffic test-business-kpis demo-business-kpis start-tracing demo-tracing infra-up monitoring-up docker-up-all

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build information
VERSION ?= latest
GITREV = $(shell git rev-parse --short HEAD)
GITBRANCH = $(shell git rev-parse --abbrev-ref HEAD)
BUILDDATE = $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

# Directories
MAIN_DIR=./cmd
RPC_DIR=./services/url-shortener-svc/cmd
REST_DIR=./services/rest-api-svc/cmd
DOCS_DIR=./services/rest-api-svc/docs
BIN_DIR=./bin

# Binary paths (all in bin/ directory)
RPC_BINARY=$(BIN_DIR)/url-shortener-rpc
REST_BINARY=$(BIN_DIR)/rest-api-svc
REDIRECT_BINARY=$(BIN_DIR)/redirect-service
ANALYTICS_BINARY=$(BIN_DIR)/analytics-service

# Ports
RPC_PORT=50051
REST_PORT=8085
NATS_PORT=4222

help: ## Show this help message
	@echo 'URL Shortener Go Micro Microservices'
	@echo ''
	@echo '🚀 QUICK START COMMANDS:'
	@echo '  setup-full           🎯 Complete setup: infrastructure + migrations + build'
	@echo '  restart-all          🔄 Stop, build, and start all services'
	@echo '  quick-test          🧪 Run comprehensive API tests'
	@echo '  demo-full           🎬 Complete restart + test cycle'
	@echo ''
	@echo '🏗️  INFRASTRUCTURE:'
	@echo '  dev-up              🚀 Start infrastructure + monitoring (Docker-based)'
	@echo '  infra-up            🏗️  Start only core infrastructure (PostgreSQL, Redis, NATS, ClickHouse)'
	@echo '  monitoring-up       📊 Start only monitoring stack (Prometheus, Grafana, Jaeger)'
	@echo '  docker-up-all       🚀 Start ALL services via Docker Compose'
	@echo '  dev-down            🛑 Stop all Docker services'
	@echo '  migrate-status      🔍 Check migration status and verify tables'
	@echo '  migrate-logs        📋 View migration container logs'
	@echo ''
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# =============================================================================
# INFRASTRUCTURE & MIGRATION COMMANDS
# =============================================================================

dev-up: ## Start infrastructure services (PostgreSQL, Redis, NATS, ClickHouse, monitoring)
	@echo "🚀 Starting infrastructure services..."
	docker-compose up -d postgres redis clickhouse nats prometheus grafana jaeger
	@echo "⏳ Waiting for services to be healthy..."
	@sleep 10
	@echo "✅ Infrastructure services started!"
	@echo "📊 PostgreSQL: localhost:5432 (url_shortener)"
	@echo "🔑 Redis: localhost:6379"
	@echo "📨 NATS: localhost:4222"
	@echo "📈 ClickHouse: localhost:8123, localhost:9001"
	@echo "📊 Prometheus: http://localhost:9090"
	@echo "📈 Grafana: http://localhost:3000 (admin/admin)"
	@echo "🔍 Jaeger: http://localhost:16686"

dev-down: ## Stop all Docker services
	@echo "🛑 Stopping all Docker services..."
	docker-compose down
	@echo "✅ All Docker services stopped"

infra-up: ## Start only infrastructure (PostgreSQL, Redis, NATS, ClickHouse)
	@echo "🚀 Starting core infrastructure..."
	docker-compose up -d postgres redis clickhouse nats
	@echo "⏳ Waiting for services to be healthy..."
	@sleep 10
	@echo "✅ Core infrastructure started!"

monitoring-up: ## Start monitoring stack (Prometheus, Grafana, Jaeger)
	@echo "📊 Starting monitoring stack..."
	docker-compose up -d prometheus grafana jaeger
	@echo "⏳ Waiting for monitoring services..."
	@sleep 5
	@echo "✅ Monitoring stack started!"
	@echo "📊 Prometheus: http://localhost:9090"
	@echo "📈 Grafana: http://localhost:3000 (admin/admin)"
	@echo "🔍 Jaeger: http://localhost:16686"

docker-up-all: ## Start all services via Docker Compose
	@echo "🚀 Starting all services via Docker Compose..."
	docker-compose up -d --build
	@echo "⏳ Waiting for all services to be ready..."
	@sleep 20
	@echo "✅ All services started!"
	@echo "🌐 REST API: http://localhost:8080"
	@echo "📋 Swagger: http://localhost:8080/docs/index.html"

migrate-status: ## Check migration status
	@echo "🔍 Checking migration status..."
	@echo "📊 PostgreSQL Tables (URL Shortener + Redirect Services):"
	@docker exec url-shortener-postgres psql -U postgres -d url_shortener -c "\dt" 2>/dev/null || echo "Database not ready yet"
	@echo ""
	@echo "📈 ClickHouse Tables (Analytics Service):"
	@curl -s "http://localhost:8123/" --data "SHOW TABLES FROM analytics FORMAT PrettyCompact" 2>/dev/null || echo "ClickHouse not ready yet"

migrate-logs: ## View migration logs
	@echo "📋 Migration Logs:"
	@echo "🏗️  PostgreSQL Migration (URL Shortener):"
	@docker logs url-shortener-migrate 2>/dev/null || echo "Migration container not found"
	@echo ""
	@echo "📈 ClickHouse Migration (Analytics):"
	@docker logs analytics-clickhouse-migrate 2>/dev/null || echo "Migration container not found"

migrate-down: ## Rollback database migrations (DESTRUCTIVE)
	@echo "⚠️  WARNING: This will rollback migrations and may cause data loss!"
	@read -p "Are you sure? (y/N): " confirm && [ "$$confirm" = "y" ] || exit 1
	@echo "🔄 Rolling back Analytics Service migrations..."
	migrate -path database/migrations/analytics-svc \
		-database "postgres://postgres:password@localhost:5432/url_shortener_db?sslmode=disable" \
		down 1
	@echo "🔄 Rolling back URL Shortener Service migrations..."
	migrate -path database/migrations/url-shortener-svc \
		-database "postgres://postgres:password@localhost:5432/url_shortener_db?sslmode=disable" \
		down 1

migrate-reset: ## Reset all migrations (VERY DESTRUCTIVE)
	@echo "⚠️  WARNING: This will drop ALL tables and data!"
	@read -p "Are you absolutely sure? Type 'DESTROY' to confirm: " confirm && [ "$$confirm" = "DESTROY" ] || exit 1
	@$(MAKE) migrate-down
	@echo "💥 All migrations reset"

setup-full: dev-up build-all ## Complete setup: infrastructure + monitoring + build
	@echo ""
	@echo "⏳ Waiting for automatic migrations to complete..."
	@sleep 15
	@echo ""
	@echo "🔍 Checking migration status..."
	@$(MAKE) migrate-status
	@echo ""
	@echo "🎉 FULL SETUP COMPLETED!"
	@echo "✅ Infrastructure: Running"
	@echo "✅ Monitoring: Running"
	@echo "✅ Migrations: Automatically executed"
	@echo "✅ Services: Built"
	@echo ""
	@echo "🚀 Ready to start services with: make run-all"
	@echo "🚀 Or start everything with Docker: make docker-up-all"

# =============================================================================
# QUICK START COMMANDS (New convenient targets)
# =============================================================================

restart-all: ## 🔄 Stop all services, rebuild, and start fresh
	@echo "🔄 RESTARTING ALL URL SHORTENER SERVICES"
	@echo "========================================"
	@echo "🛑 1. Stopping any running services..."
	@$(MAKE) stop-all
	@$(MAKE) dev-down
	@sleep 2
	@echo ""
	@echo "🚀 2. Starting infrastructure (with automatic migrations)..."
	@$(MAKE) dev-up
	@echo ""
	@echo "⏳ 3. Waiting for automatic migrations..."
	@sleep 15
	@$(MAKE) migrate-status
	@echo ""
	@echo "🔨 4. Building all services..."
	@$(MAKE) build-all
	@echo ""
	@echo "🚀 5. Starting all services..."
	@$(MAKE) run-all
	@echo ""
	@echo "✅ Restart completed!"
	@echo "🌐 REST API: http://localhost:$(REST_PORT)"
	@echo "📋 Swagger:  http://localhost:$(REST_PORT)/docs/index.html"

restart-and-test: ## 🧪 Restart all services and run comprehensive tests
	@echo "🧪 RESTART AND TEST CYCLE"
	@echo "========================="
	@$(MAKE) restart-all
	@echo ""
	@echo "⏳ Waiting 10 seconds for services to fully initialize..."
	@sleep 10
	@echo ""
	@echo "🧪 Running comprehensive API tests..."
	@$(MAKE) test-api-comprehensive

quick-test: ## 🧪 Run comprehensive API tests (assumes services are running)
	@echo "🧪 QUICK COMPREHENSIVE TEST SUITE"
	@echo "=================================="
	@echo "🔍 Checking if REST API is available..."
	@curl -s http://localhost:$(REST_PORT)/health > /dev/null 2>&1 || (echo "❌ REST API not running on port $(REST_PORT). Run 'make restart-all' first." && exit 1)
	@echo "✅ REST API is running"
	@echo ""
	@$(MAKE) test-api-comprehensive
	@echo ""
	@echo "📊 Testing analytics endpoints..."
	@$(MAKE) test-analytics

demo-full: ## 🎬 Complete demo cycle: restart + test + analytics
	@echo "🎬 FULL URL SHORTENER DEMO"
	@echo "=========================="
	@$(MAKE) restart-all
	@echo ""
	@echo "⏳ Waiting 15 seconds for complete service initialization..."
	@sleep 15
	@echo ""
	@echo "🧪 Running comprehensive tests..."
	@$(MAKE) test-api-comprehensive
	@echo ""
	@echo "📊 Testing analytics..."
	@$(MAKE) test-analytics
	@echo ""
	@echo "🎯 Creating demo data with multiple URLs..."
	@$(MAKE) create-demo-data
	@echo ""
	@echo "🎉 DEMO COMPLETED SUCCESSFULLY!"
	@echo "📋 View Swagger UI: http://localhost:$(REST_PORT)/docs/index.html"
	@echo "🌐 API Base URL:    http://localhost:$(REST_PORT)"

reset-environment: ## 🧹 Clean everything and start fresh
	@echo "🧹 RESETTING ENVIRONMENT"
	@echo "========================"
	@echo "🛑 Stopping all services..."
	@$(MAKE) stop-all
	@echo ""
	@echo "🧹 Cleaning build artifacts..."
	@$(MAKE) clean
	@echo ""
	@echo "📦 Installing dependencies..."
	@$(MAKE) deps
	@echo ""
	@echo "🔨 Building all services..."
	@$(MAKE) build-all
	@echo ""
	@echo "🚀 Starting all services..."
	@$(MAKE) run-all
	@echo ""
	@echo "✅ Environment reset completed!"

create-demo-data: ## 🎯 Create demo data for testing (requires running services)
	@echo "🎯 Creating demo data..."
	@echo "1️⃣ Creating Google short URL..."
	@curl -s -X POST http://localhost:$(REST_PORT)/api/v1/shorten \
		-H "Content-Type: application/json" \
		-d '{"long_url":"https://google.com","user_id":"demo_user","custom_alias":"google"}' | jq .
	@echo ""
	@echo "2️⃣ Creating GitHub short URL..."
	@curl -s -X POST http://localhost:$(REST_PORT)/api/v1/shorten \
		-H "Content-Type: application/json" \
		-d '{"long_url":"https://github.com","user_id":"demo_user","custom_alias":"github"}' | jq .
	@echo ""
	@echo "3️⃣ Creating YouTube short URL..."
	@curl -s -X POST http://localhost:$(REST_PORT)/api/v1/shorten \
		-H "Content-Type: application/json" \
		-d '{"long_url":"https://youtube.com","user_id":"demo_user"}' | jq .
	@echo ""
	@echo "🔍 Testing redirect (GitHub)..."
	@curl -s -I "http://localhost:$(REST_PORT)/github" | grep -E "(HTTP|Location)"
	@echo ""
	@echo "📋 Listing demo user URLs..."
	@curl -s "http://localhost:$(REST_PORT)/api/v1/users/demo_user/urls" | jq .

# =============================================================================
# END OF NEW COMMANDS
# =============================================================================

deps: ## Install dependencies
	@echo "📦 Installing Go Micro dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "🔧 Installing protoc-gen-micro plugin..."
	go install github.com/micro/go-micro/cmd/protoc-gen-micro@latest
	@echo "🔧 Installing swag CLI tool..."
	go install github.com/swaggo/swag/cmd/swag@latest
	@echo "✅ Dependencies, protoc-gen-micro, and swag installed"
	@echo "ℹ️  Database migrations are handled by Docker containers (migrate/migrate image)"

setup-protoc: ## Install protoc-gen-micro plugin
	@echo "🔧 Installing protoc-gen-micro plugin..."
	go install github.com/micro/go-micro/cmd/protoc-gen-micro@latest
	@echo "✅ protoc-gen-micro plugin installed"

setup-swagger: ## Install swag CLI tool for OpenAPI/Swagger generation
	@echo "🔧 Installing swag CLI tool..."
	go install github.com/swaggo/swag/cmd/swag@latest
	@echo "✅ swag CLI tool installed"

swagger: ## Generate OpenAPI/Swagger documentation
	@echo "📖 Generating OpenAPI/Swagger documentation..."
	@mkdir -p $(DOCS_DIR)
	swag init -g services/rest-api-svc/cmd/main.go -o $(DOCS_DIR) --parseInternal
	@echo "✅ Swagger documentation generated in $(DOCS_DIR)"

docs: swagger ## Generate documentation (alias for swagger)

proto: ## Generate protobuf files (Go + Go Micro)
	@echo "🔧 Generating protobuf files (Go + Go Micro)..."
	@echo "📄 Generating standard Go protobuf files..."
	protoc --go_out=. --go_opt=paths=source_relative \
		proto/url/url.proto
	protoc --go_out=. --go_opt=paths=source_relative \
		proto/redirect/redirect.proto
	protoc --go_out=. --go_opt=paths=source_relative \
		proto/analytics/analytics.proto
	@echo "🌐 Generating Go Micro service interfaces..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--micro_out=. --micro_opt=paths=source_relative \
		proto/url/url.proto
	protoc --go_out=. --go_opt=paths=source_relative \
		--micro_out=. --micro_opt=paths=source_relative \
		proto/redirect/redirect.proto
	protoc --go_out=. --go_opt=paths=source_relative \
		--micro_out=. --micro_opt=paths=source_relative \
		proto/analytics/analytics.proto
	@echo "✅ All protobuf files generated (url.pb.go + redirect.pb.go + analytics.pb.go + Go Micro files)"

clean: ## Clean build artifacts
	@echo "🧹 Cleaning build artifacts..."
	$(GOCLEAN)
	rm -rf $(BIN_DIR)
	@echo "✅ Clean completed"

test: ## Run all tests
	@echo "🧪 Running all tests..."
	@echo "📊 Testing Database Layer..."
	$(GOTEST) -v ./utils/database/
	@echo "📊 Testing Cache Layer..."
	$(GOTEST) -v ./utils/cache/
	@echo "📊 Testing Domain Layer..."
	$(GOTEST) -v ./services/url-shortener-svc/domain/
	@echo "📊 Running all tests..."
	$(GOTEST) -v ./...
	@echo "✅ All tests completed"

# Build targets for Go Micro architecture
build-all: create-bin-dir build-rpc build-rest build-redirect build-analytics ## Build all services

create-bin-dir: ## Create bin directory
	@mkdir -p $(BIN_DIR)

build-rpc: create-bin-dir ## Build RPC service
	@echo "🔨 Building RPC service..."
	$(GOBUILD) -o $(RPC_BINARY) -ldflags "-X main.Version=$(VERSION)" $(RPC_DIR)
	@echo "✅ RPC service built: $(RPC_BINARY)"

build-rest: create-bin-dir ## Build REST API service 
	@echo "🔨 Building REST API service..."
	$(GOBUILD) -o $(REST_BINARY) -ldflags "-X main.Version=$(VERSION)" $(REST_DIR)
	@echo "✅ REST API service built: $(REST_BINARY)"

build-redirect: create-bin-dir ## Build redirect service
	@echo "🔨 Building redirect service..."
	$(GOBUILD) -o $(REDIRECT_BINARY) services/redirect-svc/cmd/main.go
	@echo "✅ Redirect service built: $(REDIRECT_BINARY)"

build-analytics: create-bin-dir ## Build analytics service
	@echo "🔨 Building analytics service..."
	$(GOBUILD) -o $(ANALYTICS_BINARY) services/analytics-svc/cmd/main.go
	@echo "✅ Analytics service built: $(ANALYTICS_BINARY)"

# Run individual services
run-rpc: build-rpc ## Run RPC service
	@echo "🚀 Starting RPC service on port $(RPC_PORT)..."
	@echo "🔧 Connecting to PostgreSQL and Redis..."
	PORT=$(RPC_PORT) $(RPC_BINARY)

run-rest: build-rest ## Run REST API service
	@echo "🚀 Starting REST API service on port $(REST_PORT)..."
	@echo "🔧 Connecting to RPC service..."
	PORT=$(REST_PORT) $(REST_BINARY)

run-redirect: build-redirect ## Run redirect service 
	@echo "🚀 Starting redirect service on port 50052..."
	@echo "📊 Service will register with NATS for service discovery"
	$(REDIRECT_BINARY)

run-analytics: build-analytics ## Run analytics service
	@echo "🚀 Starting analytics service..."
	@echo "📊 Service will register with NATS for service discovery"
	@echo "🔧 Connecting to PostgreSQL and Redis for analytics data..."
	$(ANALYTICS_BINARY)

# Run all services
run-all: build-all ## Build and run all services in background
	@echo "🚀 Starting all URL Shortener services..."
	@echo "📊 Starting analytics service..."
	$(ANALYTICS_BINARY) > /tmp/analytics.log 2>&1 &
	@sleep 3
	@echo "🔧 Starting RPC service on port $(RPC_PORT)..."
	PORT=$(RPC_PORT) $(RPC_BINARY) > /tmp/rpc.log 2>&1 &
	@sleep 3
	@echo "📊 Starting redirect service..."
	$(REDIRECT_BINARY) > /tmp/redirect.log 2>&1 &
	@sleep 3
	@echo "🌐 Starting REST API service on port $(REST_PORT)..."
	PORT=$(REST_PORT) $(REST_BINARY) > /tmp/rest-api.log 2>&1 &
	@sleep 3
	@echo ""
	@echo "✅ All services started in background!"
	@echo "🌐 REST API: http://localhost:$(REST_PORT)"
	@echo "📋 Swagger UI: http://localhost:$(REST_PORT)/docs/index.html"
	@echo "🏠 Documentation: http://localhost:$(REST_PORT)/"
	@echo ""
	@echo "📊 Service logs:"
	@echo "  Analytics: tail -f /tmp/analytics.log"
	@echo "  RPC:       tail -f /tmp/rpc.log"
	@echo "  Redirect:  tail -f /tmp/redirect.log"
	@echo "  REST API:  tail -f /tmp/rest-api.log"
	@echo ""
	@echo "🛑 To stop all services: make stop-all"

stop-all: ## Stop all running services
	@echo "🛑 Stopping all URL Shortener services..."
	@pkill -f "$(ANALYTICS_BINARY)" 2>/dev/null || true
	@pkill -f "$(RPC_BINARY)" 2>/dev/null || true
	@pkill -f "$(REDIRECT_BINARY)" 2>/dev/null || true
	@pkill -f "$(REST_BINARY)" 2>/dev/null || true
	@echo "✅ All services stopped"

# Testing
test-api: ## Test REST API endpoints (PUBLIC - End User Facing)
	@echo "🧪 Testing REST API endpoints (End User Facing)..."
	@echo "🌐 REST API runs on http://localhost:$(REST_PORT)"
	@echo ""
	@echo "1️⃣ Health check:"
	@curl -s http://localhost:$(REST_PORT)/health | jq .
	@echo ""
	@echo "2️⃣ Shorten URL:"
	@curl -s -X POST http://localhost:$(REST_PORT)/api/v1/shorten \
		-H "Content-Type: application/json" \
		-d '{"long_url":"https://example.com","user_id":"user123"}' | jq .
	@echo ""
	@echo "3️⃣ Get URL info (replace 'abc123' with actual short_code from step 2):"
	@echo "curl 'http://localhost:$(REST_PORT)/api/v1/urls/abc123?user_id=user123' | jq ."

test-api-comprehensive: ## Comprehensive REST API testing (PUBLIC)
	@echo "🧪 Comprehensive REST API Testing Suite"
	@echo "========================================"
	@echo "🌐 Testing REST API on http://localhost:$(REST_PORT)"
	@echo ""
	@echo "🔍 1. Health Check"
	@echo "-------------------"
	curl -s http://localhost:$(REST_PORT)/health | jq .
	@echo ""
	@echo "📝 2. Create Short URL"
	@echo "----------------------"
	@echo "Request: POST /api/v1/shorten"
	curl -s -X POST http://localhost:$(REST_PORT)/api/v1/shorten \
		-H "Content-Type: application/json" \
		-d '{"long_url":"https://google.com","user_id":"testuser123","custom_alias":"google"}' | jq . > /tmp/shorten_response.json
	@cat /tmp/shorten_response.json | jq .
	@echo ""
	@echo "🔍 3. Get URL Info"
	@echo "------------------"
	@echo "Request: GET /api/v1/urls/google?user_id=testuser123"
	curl -s "http://localhost:$(REST_PORT)/api/v1/urls/google?user_id=testuser123" | jq .
	@echo ""
	@echo "📋 4. List User URLs"
	@echo "--------------------"
	@echo "Request: GET /api/v1/users/testuser123/urls"
	curl -s "http://localhost:$(REST_PORT)/api/v1/users/testuser123/urls" | jq .
	@echo ""
	@echo "🗑️  5. Delete URL"
	@echo "------------------"
	@echo "Request: DELETE /api/v1/urls/google?user_id=testuser123"
	curl -s -X DELETE "http://localhost:$(REST_PORT)/api/v1/urls/google?user_id=testuser123" | jq .

test-rpc-internal: ## Test internal RPC service (NOT for end users)
	@echo "⚠️  WARNING: This tests INTERNAL RPC service"
	@echo "🔒 RPC Service is NOT exposed to end users"
	@echo "🏗️  RPC Service runs on port $(RPC_PORT) (Go Micro over NATS)"
	@echo ""
	@echo "ℹ️  RPC service can only be tested by:"
	@echo "   1. Other Go Micro services (like our REST API)"
	@echo "   2. Go Micro client tools"
	@echo "   3. Unit tests within the codebase"
	@echo ""
	@echo "🌐 For end-user testing, use: make test-api"

test-analytics: ## Test analytics endpoints (requires running services)
	@echo "📊 Testing Analytics REST API endpoints..."
	@echo "🌐 Analytics API runs on http://localhost:$(REST_PORT)"
	@echo ""
	@echo "1️⃣ Analytics Dashboard:"
	@curl -s "http://localhost:$(REST_PORT)/api/v1/analytics/dashboard" | jq .
	@echo ""
	@echo "2️⃣ Top URLs (limit 5):"
	@curl -s "http://localhost:$(REST_PORT)/api/v1/analytics/top-urls?limit=5" | jq .
	@echo ""
	@echo "3️⃣ URL Analytics (replace 'abc123' with actual short_code):"
	@echo "curl 'http://localhost:$(REST_PORT)/api/v1/analytics/urls/abc123' | jq ."
	@echo ""
	@echo "📊 Available Analytics Parameters:"
	@echo "  ?start_time=UNIX_TIMESTAMP  - Start time filter"
	@echo "  ?end_time=UNIX_TIMESTAMP    - End time filter"
	@echo "  ?granularity=hour|day|week  - Time granularity"
	@echo "  ?limit=N                    - Number of results"
	@echo "  ?sort_by=clicks|unique_clicks - Sort criteria"

demo-comprehensive: ## Run comprehensive API demo with Swagger (requires running services)
	@echo "🎬 Running comprehensive API demo..."
	@./scripts/api-demo.sh

demo-api: ## Interactive API demo for end users
	@echo "🎬 URL Shortener REST API Demo"
	@echo "================================"
	@echo "🌐 Public REST API: http://localhost:$(REST_PORT)"
	@echo ""
	@echo "📖 API Documentation:"
	@echo "  📋 Swagger UI:     http://localhost:$(REST_PORT)/docs/index.html"
	@echo "  📄 OpenAPI JSON:   http://localhost:$(REST_PORT)/swagger/doc.json"
	@echo "  🏠 Docs Home:      http://localhost:$(REST_PORT)/"
	@echo ""
	@echo "📱 Available Endpoints for End Users:"
	@echo "  POST   /api/v1/shorten              - Create short URL"
	@echo "  GET    /api/v1/urls/:shortCode      - Get URL info"
	@echo "  DELETE /api/v1/urls/:shortCode      - Delete URL"
	@echo "  GET    /api/v1/users/:userID/urls   - List user URLs"
	@echo "  GET    /api/v1/analytics/urls/:shortCode - Get URL analytics"
	@echo "  GET    /api/v1/analytics/top-urls   - Get top performing URLs"
	@echo "  GET    /api/v1/analytics/dashboard  - Get analytics dashboard"
	@echo "  GET    /health                      - Health check"
	@echo ""
	@echo "🧪 Run comprehensive tests:"
	@echo "  make test-api-comprehensive"
	@echo "  make demo-comprehensive     # Full demo with live API calls"
	@echo ""
	@echo "📚 Example cURL commands:"
	@echo "  # Health check"
	@echo "  curl http://localhost:$(REST_PORT)/health"
	@echo ""
	@echo "  # Create short URL"
	@echo "  curl -X POST http://localhost:$(REST_PORT)/api/v1/shorten \\"
	@echo "    -H 'Content-Type: application/json' \\"
	@echo "    -d '{\"long_url\":\"https://google.com\",\"user_id\":\"user123\"}'"

# Generate continuous traffic for Business KPI metrics testing
generate-business-traffic:
	@echo "🚀 Generating continuous Business KPI traffic..."
	@echo "📊 This will create sustained load for proper rate calculations in Grafana"
	@echo "⏱️  Running for 2 minutes with requests every 10 seconds..."
	@for i in $$(seq 1 12); do \
		echo "🔄 Traffic round $$i/12:"; \
		curl -s -X POST http://localhost:8085/api/v1/shorten -H "Content-Type: application/json" -d "{\"long_url\":\"https://business-kpi-test-$$i.com\",\"user_id\":\"kpi_user_$$i\"}" | jq -r '.short_code // "failed"' | sed 's/^/  📎 Created: /'; \
		curl -s "http://localhost:8085/api/v1/analytics/dashboard" > /dev/null && echo "  📈 Analytics dashboard accessed"; \
		curl -s "http://localhost:8085/api/v1/analytics/top-urls" > /dev/null && echo "  🏆 Top URLs queried"; \
		SHORT_CODE=$$(curl -s "http://localhost:8085/api/v1/users/kpi_user_$$i/urls" | jq -r '.urls[0].short_code // "github2"'); \
		curl -s "http://localhost:8085/$$SHORT_CODE" > /dev/null && echo "  🎯 Redirect tested: $$SHORT_CODE"; \
		curl -s "http://localhost:8085/api/v1/urls/$$SHORT_CODE?user_id=kpi_user_$$i" > /dev/null && echo "  📋 URL info accessed"; \
		echo "  ⏰ Waiting 10 seconds..."; \
		sleep 10; \
	done
	@echo "✅ Business KPI traffic generation completed!"
	@echo "🎯 You can now check the Grafana Business KPIs dashboard for data"

# Test Business KPI queries in Prometheus
test-business-kpis:
	@echo "🔍 Testing Business KPI Prometheus queries..."
	@echo ""
	@echo "1️⃣ URL Creation Rate:"
	@curl -s "http://localhost:9090/api/v1/query?query=increase(http_requests_total{service=\"rest-api\",endpoint=\"/api/v1/shorten\",status=~\"2..\"}[5m])*12" | jq '.data.result[] | {endpoint: .metric.endpoint, status: .metric.status, rate_per_minute: .value[1]}' || echo "No data yet"
	@echo ""
	@echo "2️⃣ Analytics Consumption:"
	@curl -s "http://localhost:9090/api/v1/query?query=increase(http_requests_total{service=\"rest-api\",endpoint=\"/api/v1/analytics/dashboard\",status=~\"2..\"}[5m])*12" | jq '.data.result[] | {endpoint: .metric.endpoint, rate_per_minute: .value[1]}' || echo "No data yet"
	@echo ""
	@echo "3️⃣ User Engagement (Redirects):"
	@curl -s "http://localhost:9090/api/v1/query?query=increase(http_requests_total{service=\"rest-api\",endpoint=\"/:shortCode\",status=~\"2..|3..\"}[5m])*12" | jq '.data.result[] | {endpoint: .metric.endpoint, status: .metric.status, rate_per_minute: .value[1]}' || echo "No data yet"

# Full Business KPI demo workflow
demo-business-kpis: restart-all
	@echo "🎯 Full Business KPI Demo Workflow"
	@echo "================================="
	@echo "1️⃣ Starting all services..."
	@sleep 15
	@echo "2️⃣ Generating Business KPI traffic..."
	@$(MAKE) generate-business-traffic
	@echo "3️⃣ Testing KPI queries in Prometheus..."
	@$(MAKE) test-business-kpis
	@echo ""
	@echo "🎉 Demo complete! Check these URLs:"
	@echo "   📊 Grafana Business KPIs: http://localhost:3000/d/url-shortener-services"
	@echo "   🔍 Prometheus Metrics: http://localhost:9090/graph"

start-tracing: ## Start Jaeger tracing infrastructure
	@echo "🔍 Starting Jaeger distributed tracing..."
	docker-compose up -d jaeger
	@echo "⏳ Waiting for Jaeger to be ready..."
	@sleep 10
	@echo "✅ Jaeger started successfully!"
	@echo "🌐 Jaeger UI: http://localhost:16686"

demo-tracing: ## Complete distributed tracing demonstration
	@echo "🔍 Complete Distributed Tracing Demonstration"
	@echo "============================================="
	@echo "🎯 This demo shows end-to-end request tracing across all microservices"
	@echo "🌐 Jaeger UI: http://localhost:16686"
	@echo ""
	@DEMO_TRACE_ID="demo-$$(date +%s)"; \
	echo "🔍 Trace ID: $$DEMO_TRACE_ID"; \
	echo "📝 Step 1: Create short URL (REST → RPC)"; \
	SHORT_CODE=$$(curl -s -X POST http://localhost:8085/api/v1/shorten \
		-H "Content-Type: application/json" \
		-H "X-Trace-ID: $$DEMO_TRACE_ID" \
		-d '{"long_url":"https://github.com/go-systems-lab/go-url-shortener","user_id":"demo_trace_user","custom_alias":"demo-traced"}' | jq -r '.short_code'); \
	echo "✅ Short URL created: $$SHORT_CODE"; \
	sleep 2; \
	echo "🔗 Step 2: Test redirect (REST → Redirect Service → Analytics)"; \
	curl -s -H "X-Trace-ID: $$DEMO_TRACE_ID" "http://localhost:8085/$$SHORT_CODE" > /dev/null; \
	echo "✅ Redirect completed with analytics event"; \
	sleep 2; \
	echo "📊 Step 3: Retrieve analytics (REST → Analytics Service)"; \
	curl -s -H "X-Trace-ID: $$DEMO_TRACE_ID" "http://localhost:8085/api/v1/analytics/urls/$$SHORT_CODE" > /dev/null; \
	echo "✅ Analytics retrieved"; \
	echo "🎯 Tracing Demo Complete! View traces at: http://localhost:16686"
 
