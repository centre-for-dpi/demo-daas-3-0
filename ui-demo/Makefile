.PHONY: run run-waltid run-mock run-production run-alt build test vet clean docker docker-up docker-down docker-restart lifecycle-test

# Default: run with mock backend
run: run-mock

# Run with Walt.id backend (requires Docker stack running)
run-waltid: build
	set -a && . ./.env && set +a && ./server -config config/waltid.json

# Run with Walt.id + Keycloak + WSO2 SSO (requires Docker stack running)
run-sso: build
	@set -a && . ./.env && set +a && cd docker/stack && bash wso2-init.sh && cd ../..
	set -a && . ./.env && set +a && ./server -config config/waltid-sso.json

# Run with mock backend (no Docker needed)
run-mock: build
	./server -config config/default.json

# Run in production mode
run-production: build
	./server -config config/production.json

# Run with alternate brand
run-alt: build
	./server -config config/alt-brand.json

# Build the server binary
build:
	go build -o server ./cmd/server/

# Run Go vet
vet:
	go vet ./...

# Run tests
test: build
	@echo "=== Build OK ==="
	go vet ./...
	@echo "=== Vet OK ==="

# Clean build artifacts
clean:
	rm -f server

# Build Docker image
docker:
	docker build -t vcplatform:latest .

# Start Walt.id Docker stack + initialize SSO providers
docker-up:
	cd docker/stack && docker compose --env-file ../../.env up -d
	@echo "Waiting for services to start..."
	@sleep 20
	@echo "Initializing WSO2 application..."
	@set -a && . ./.env && set +a && cd docker/stack && bash wso2-init.sh

# Stop Walt.id Docker stack
docker-down:
	cd docker/stack && docker compose down

# Restart Walt.id Docker stack
docker-restart:
	cd docker/stack && docker compose down && docker compose up -d

# Full reset: destroy all data (users, credentials, DIDs) and start fresh
docker-clean:
	cd docker/stack && docker compose down -v && docker compose up -d

# Create a WSO2 user (admin-provisioned). Usage: make wso2-user NAME=jane.doe PASS=Secret123! FIRST=Jane LAST=Doe EMAIL=jane@example.com
wso2-user:
	@curl -sk -X POST "https://localhost:${WSO2_PORT}/scim2/Users" \
		-u ${WSO2_ADMIN_USER}:${WSO2_ADMIN_PASSWORD} \
		-H "Content-Type: application/scim+json" \
		-d '{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"$(NAME)","password":"$(PASS)","name":{"givenName":"$(FIRST)","familyName":"$(LAST)"},"emails":[{"value":"$(EMAIL)","primary":true}]}' \
		| python3 -c "import json,sys; d=json.load(sys.stdin); print(f'Created: {d.get(\"userName\",d.get(\"detail\",\"error\"))}') if 'id' in d or 'detail' in d else print(d)"

# List WSO2 users
wso2-users:
	@curl -sk "https://localhost:${WSO2_PORT}/scim2/Users" \
		-u ${WSO2_ADMIN_USER}:${WSO2_ADMIN_PASSWORD} \
		| python3 -c "import json,sys; d=json.load(sys.stdin); [print(f'  {u[\"userName\"]}') for u in d.get('Resources',[])]"

# Show Walt.id Docker stack status
docker-ps:
	cd docker/stack && docker compose ps

# Run the DPG lifecycle test (requires Docker stack running)
lifecycle-test:
	cd docker/stack && bash test-lifecycle.sh

# Start everything: Docker stack + server with Walt.id config
up: docker-up
	@echo "Waiting for Walt.id stack..."
	@sleep 15
	@echo "Starting server..."
	$(MAKE) run-waltid

# Quick start with mock (no Docker needed)
start: run-mock
