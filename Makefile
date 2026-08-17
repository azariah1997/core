.PHONY: doctor fmt test build run-api run-realtime run-worker local-up local-down local-logs smoke validate-config validate-contracts validate-all admin-install admin-build flutter-get flutter-test terraform-fmt terraform-validate helm-lint

doctor:
	@./scripts/doctor.sh

fmt:
	gofmt -w backend packages/go

build:
	go build ./backend/core-api/... ./backend/realtime-gateway/... ./backend/worker/... ./packages/go/platformkit/...

test:
	go test ./backend/core-api/... ./backend/realtime-gateway/... ./backend/worker/... ./packages/go/platformkit/...

run-api:
	go run ./backend/core-api/cmd/server

run-realtime:
	go run ./backend/realtime-gateway/cmd/server

run-worker:
	go run ./backend/worker/cmd/worker

local-up:
	docker compose -f infra/docker/docker-compose.yml --env-file .env up -d

local-down:
	docker compose -f infra/docker/docker-compose.yml --env-file .env down

local-logs:
	docker compose -f infra/docker/docker-compose.yml --env-file .env logs -f

smoke:
	./scripts/smoke.sh

validate-config:
	python3 scripts/validate_config.py

validate-contracts:
	python3 scripts/validate_contracts.py

validate-all: validate-config validate-contracts test build

admin-install:
	cd apps/admin && npm install

admin-build:
	cd apps/admin && npm run build

flutter-get:
	cd apps/mobile && flutter pub get

flutter-test:
	cd apps/mobile && flutter test

terraform-fmt:
	terraform -chdir=infra/terraform fmt -recursive -check

terraform-validate:
	terraform -chdir=infra/terraform init -backend=false && terraform -chdir=infra/terraform validate

helm-lint:
	helm lint infra/kubernetes/charts/core-platform
