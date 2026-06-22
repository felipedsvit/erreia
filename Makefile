SHELL := /bin/bash
BIN  ?= bin/erreia
APP  := erreia
PKG  := ./...

GITHUB_TOKEN ?=
GITHUB_REPO  ?= felipedsvit/erreia

.PHONY: help tidy templ js build run dev compose-up compose-down compose-logs fmt vet test coverage lint test-pg-up test-pg-down clean pull

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
	@echo "  pull          - auth with gh and push to github (requires GITHUB_TOKEN=ghp_...)"

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

pull:
	@if [ -z "$(GITHUB_TOKEN)" ]; then \
	  echo "ERROR: GITHUB_TOKEN is required. Run: make pull GITHUB_TOKEN=ghp_..."; \
	  exit 1; \
	fi
	@echo "$(GITHUB_TOKEN)" | gh auth login --with-token
	@git branch -m master main 2>/dev/null || true
	@gh repo create $(GITHUB_REPO) --public --source=. --remote=origin 2>/dev/null || \
	  git remote set-url origin https://github.com/$(GITHUB_REPO).git 2>/dev/null || \
	  git remote add origin https://github.com/$(GITHUB_REPO).git
	@git push -u origin main
	@git push origin --tags
	@echo ""
	@echo "Done. CI/CD iniciado em:"
	@echo "  https://github.com/$(GITHUB_REPO)/actions"
	@echo "Imagem publicada em:"
	@echo "  ghcr.io/$(GITHUB_REPO):v1.0.0"
