.PHONY: run build test test-integration dev down release-check release-snapshot

run:
	go run ./cmd/lumio
build:
	CGO_ENABLED=0 go build -trimpath -o lumio ./cmd/lumio
test:
	go test -race ./...
test-integration:
	@test -n "$$LUMIO_TEST_DATABASE_URL" || (echo 'Set LUMIO_TEST_DATABASE_URL'; exit 1)
	go test -race -count=1 ./internal/app
dev:
	docker build -t lumio:dev .
	docker compose up --no-build -d
down:
	docker compose down
release-check:
	goreleaser check
release-snapshot:
	goreleaser release --snapshot --clean
