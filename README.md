# Lumio

A self-hosted photo library written in Go. Lumio uses PostgreSQL for metadata and supports local storage, persistent volumes, and cloud buckets.

## Screenshots

Gallery with date navigation:

![Photo gallery with a vertical upload timeline](docs/screenshots/gallery.png)

Storage and library status:

![Library statistics and disk usage](docs/screenshots/status.png)

[Demo image sources](docs/screenshots/SOURCES.md).

## Features

- Photo uploads, thumbnails, favorites, filename search, and original downloads.
- Upload timeline with year/month navigation and date filtering.
- SHA-256 deduplication and unchanged original files.
- Password authentication with sessions shared across replicas.
- Local files, S3-compatible buckets, Google Cloud Storage, and Azure Blob Storage.
- Disk usage, library statistics, and storage availability in the status panel.
- JSON logs, health endpoints, and Kubernetes manifests.

Uploads support JPEG, PNG, and GIF images up to 40 megapixels. The timeline uses upload dates. The application ships as a single binary with an embedded web interface.

## Quick start

Requires Docker, Docker Compose, and Make.

```sh
cp .env.example .env
# Set LUMIO_PASSWORD in .env to a password of at least 16 characters.
make dev
```

Open <http://localhost:8080> and sign in with the configured password. Compose starts the app and PostgreSQL, with named volumes for the database and photos.

```sh
docker compose logs -f app
docker compose down
```

Stopping the services preserves their volumes. `docker compose down -v` deletes the development data.

## Configuration

All application configuration is provided through environment variables.

| Variable | Default | Description |
| --- | --- | --- |
| `DATABASE_URL` | required | PostgreSQL connection URL |
| `LUMIO_PASSWORD` | required | Login password; at least 16 characters |
| `LUMIO_ADDR` | `:8080` | HTTP listen address |
| `LUMIO_STORAGE_URL` | local directory | Storage backend URL |
| `LUMIO_DATA_DIR` | `data` | Local directory when the storage URL is unset |
| `LUMIO_SECURE_COOKIE` | `true` | HTTPS-only cookies; use `false` for local HTTP |
| `LUMIO_MAX_UPLOAD_MB` | `32` | Per-file upload limit in MiB, from 1 to 256 |
| `LUMIO_MIGRATE_TO` | unset | Destination URL for storage migration |

Storage URLs use `file:///data`, `s3://bucket?region=us-east-1`, `gs://bucket`, or `azblob://container`. Credentials follow the selected provider's environment variables or default credential chain. See the [storage guide](docs/storage.md) for configuration, migration, and backup instructions.

## Deployment

Kustomize overlays support a single replica with a PVC or multiple replicas sharing a bucket and PostgreSQL database. See the [Kubernetes deployment guide](docs/deployment.md).

Version-tagged container images are published to `ghcr.io/yyvfuruta/lumio:<tag>` by the release workflow.

## Development

Use the Go version declared in [go.mod](go.mod).

```sh
make test
go vet ./...
make build
```

CI runs integration tests with PostgreSQL and Garage. Release tags trigger GitHub Actions to build binaries and multi-platform container images. See [development and releases](docs/development.md) and the [HTTP API reference](docs/api.md).

## License

[GNU GPL v3](LICENSE).
