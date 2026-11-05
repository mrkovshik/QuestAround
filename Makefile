.PHONY: test test-pg vet build

TEST_DB ?= postgres://postgres:postgres@localhost:5432/questline_test

test:
	go test ./...

# Тесты хранилища на настоящем Postgres (docker compose up -d)
test-pg:
	QUESTLINE_TEST_DATABASE_URL=$(TEST_DB) go test -race ./...

vet:
	go vet ./...

build: web
	go build -o bin/questline ./cmd/questline

# Фронтенд: собирается в internal/webui/dist/app и вшивается в бинарник.
.PHONY: web web-dev
web:
	cd web && npm ci && npm run build

web-dev:
	cd web && npm run dev
