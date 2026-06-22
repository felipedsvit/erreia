SHELL := /bin/bash
BIN  ?= bin/erreia
APP  := erreia
PKG  := ./...

GITHUB_TOKEN ?=
GH_TOKEN     ?= $(GITHUB_TOKEN)
GITHUB_REPO  ?= felipedsvit/erreia

.PHONY: help tidy templ js build run dev compose-up compose-down compose-logs fmt vet test coverage lint test-pg-up test-pg-down clean pull ci ci-checks ci-build act act-build

help:
	@echo "Targets:"
	@echo "  tidy          - go mod tidy"
	@echo "  templ         - generate templ templates"
	@echo "  js            - download vendored JS assets (htmx, htmx-sse, sortable)"
	@echo "  build         - build static binary into $(BIN)"
	@echo "  run           - run the app natively"
	@echo "  compose-up    - start app + postgres + minio via docker compose"
	@echo "  compose-down  - stop everything"
	@echo "  fmt vet test  - standard Go chores"
	@echo "  pull          - push to github (requires GITHUB_TOKEN=ghp_... or GH_TOKEN=ghp_...)"
	@echo "  ci            - run full CI locally (checks + docker build) via docker compose"
	@echo "  ci-checks     - run the test job (templ, vet, lint, unit + integration) in docker"
	@echo "  act           - run the real workflow 'test' job locally via nektos/act"
	@echo "  act-build     - run build-and-push (as pull_request, builds without publishing)"

tidy:
	go mod tidy

templ:
	go tool templ generate

js:
	@mkdir -p internal/web/static/js
	@curl -sSLf -o internal/web/static/js/htmx.min.js \
		https://unpkg.com/htmx.org@2.0.3/dist/htmx.min.js
	@curl -sSLf -o internal/web/static/js/htmx-sse.js \
		https://unpkg.com/htmx-ext-sse@2.2.2/sse.js
	@curl -sSLf -o internal/web/static/js/sortable.min.js \
		https://cdn.jsdelivr.net/npm/sortablejs@1.15.6/Sortable.min.js
	@echo "JS assets vendored"

build: templ js
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/server

run:
	go run ./cmd/server

dev: templ
	go run ./cmd/server

compose-up:
	docker compose up -d --build

compose-down:
	docker compose down -v

compose-logs:
	docker compose logs -f

fmt:
	gofmt -s -w .

vet:
	go vet $(PKG)

test:
	go test -race -count=1 $(PKG)

test-integration:
	ERREIA_TEST_DATABASE_URL=postgres://erreia:erreia@127.0.0.1:5432/erreia?sslmode=disable \
		go test -race -count=1 -tags=integration $(PKG)

test-pg-up:
	docker run -d --name erreia-test-pg -p 5432:5432 \
		-e POSTGRES_USER=erreia -e POSTGRES_PASSWORD=erreia -e POSTGRES_DB=erreia \
		postgres:16-alpine >/dev/null
	@echo "Waiting for Postgres to be ready..."
	@for i in $$(seq 1 30); do \
		if docker exec erreia-test-pg pg_isready -U erreia >/dev/null 2>&1; then \
			echo "Postgres ready"; break; \
		fi; \
		sleep 1; \
	done

test-pg-down:
	docker rm -f erreia-test-pg 2>/dev/null || true

coverage:
	go test -cover -race -count=1 $(PKG)

lint:
	golangci-lint run --timeout=5m

clean:
	rm -rf bin

ci: ci-checks ci-build

ci-checks:
	HOST_UID=$$(id -u) HOST_GID=$$(id -g) \
	  docker compose -f docker/docker-compose.ci.yml up --build --abort-on-container-exit --exit-code-from ci; \
	  status=$$?; \
	  docker compose -f docker/docker-compose.ci.yml down -v; \
	  exit $$status

ci-build:
	docker build -t erreia:ci .

act:
	gh act -j test

act-build:
	gh act pull_request -j build-and-push

pull:
	@if [ -z "$(GH_TOKEN)" ]; then \
	  echo "ERROR: GH_TOKEN is required. Run: make pull GH_TOKEN=ghp_..."; \
	  exit 1; \
	fi
	@git branch -m master main 2>/dev/null || true
	@GH_TOKEN=$(GH_TOKEN) gh repo create $(GITHUB_REPO) --public --source=. --remote=origin 2>/dev/null || \
	  git remote set-url origin https://github.com/$(GITHUB_REPO).git 2>/dev/null || \
	  git remote add origin https://github.com/$(GITHUB_REPO).git
	@git push -u origin main
	@git push origin --tags
	@echo ""
	@echo "Done. CI/CD iniciado em:"
	@echo "  https://github.com/$(GITHUB_REPO)/actions"
	@echo "Imagem publicada em:"
	@echo "  ghcr.io/$(GITHUB_REPO):v1.0.0"
