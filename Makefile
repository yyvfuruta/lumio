.PHONY: run build test test-integration dev down release-check android-apk android-test

run:
	go run ./cmd/lumio
build:
	CGO_ENABLED=0 go build -trimpath -o lumio ./cmd/lumio
	CGO_ENABLED=0 go build -trimpath -o lumio-cli ./cmd/lumio-cli
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

android-apk:
	./android/build-apk.sh
android-test:
	./android/test.sh
