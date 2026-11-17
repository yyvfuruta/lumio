# Lumio

A small personal photo library in Go. One binary, a PostgreSQL database, and either a local directory/PVC or a cloud bucket. The frontend is a minimal cgit-style photo browser using the dark grays and monospace typography of miv, Yomu, and the author's Waybar.

## Features

- Single-user password login with shared PostgreSQL sessions.
- JPEG, PNG, and GIF uploads, immutable originals, and JPEG thumbnails.
- SHA-256 deduplication, favorites, filename search, download, and deletion.
- Timeline grouped by upload date, with cursor pagination.
- Keyboard navigation: `h j k l`, `/` to search, `u` to upload, `?` for help.
- Local/PVC, S3-compatible storage (including Garage), GCS, and Azure Blob through [Go CDK](https://gocloud.dev/howto/blob/).
- Status panel with library size, filesystem usage, and database/storage availability.
- Verified storage migration, structured JSON logs, health endpoints, and Kubernetes manifests.

This first version does not include HEIC, video, mobile background backup, albums, face recognition, EXIF capture-date sorting, or EXIF orientation correction for thumbnails. GIF thumbnails show the first frame; originals are preserved. The list/search API has bounded pages; search currently scans filenames and is intended for personal libraries.

## Run locally

Docker Compose uses PostgreSQL and a **named Docker volume** for photos by default:

```sh
cp .env.example .env
# Set your own LUMIO_PASSWORD in .env (at least 16 characters).
make dev
```

Open <http://localhost:8080> and sign in using that password. `docker compose down` stops services and retains data. `docker compose down -v` deletes the development database and photo volumes.

To run Go directly against a fresh development database:

```sh
export LUMIO_PASSWORD='choose-a-long-personal-password'
docker compose up -d --wait postgres
export DATABASE_URL='postgres://lumio:lumio-dev-only@localhost:5432/lumio?sslmode=disable'
export LUMIO_SECURE_COOKIE=false
go run ./cmd/lumio
```

Local storage defaults to `./data`. Use the same storage URL on subsequent runs against that database; switching an existing library requires migration. All application configuration comes from environment variables. There is no application configuration file or frontend build step.

## Environment

| Variable | Default | Purpose |
| --- | --- | --- |
| `DATABASE_URL` | required | PostgreSQL connection URL; pgx pool options such as `pool_max_conns=10` can be included in its query |
| `LUMIO_PASSWORD` | required | Personal login password, at least 16 characters; changing it invalidates previous sessions |
| `LUMIO_ADDR` | `:8080` | HTTP listen address |
| `LUMIO_STORAGE_URL` | local directory | `file:///data`, `s3://bucket?...`, `gs://bucket`, or `azblob://container` |
| `LUMIO_DATA_DIR` | `data` | Directory used when `LUMIO_STORAGE_URL` is unset |
| `LUMIO_SECURE_COOKIE` | `true` | HTTPS-only session cookie; set `false` for local HTTP/port-forwarding |
| `LUMIO_MAX_UPLOAD_MB` | `32` | Per-file limit in MiB, between 1 and 256 |
| `LUMIO_MIGRATE_TO` | required for migration | Target storage URL for `lumio migrate-storage` |

Logs are JSON on stdout, including HTTP method, path, status, duration, bytes, and request ID. Health probe requests are omitted from access logs. Passwords, cookies, request bodies, and query strings are not logged. Send SIGTERM for graceful HTTP shutdown.

## Buckets

Provision a private bucket/container first. The application needs read, write, and delete access; the status panel also uses bucket-list permission to check accessibility. It proxies authenticated downloads; it does not make the bucket public. Use a dedicated bucket or a prefix such as `?prefix=lumio/`. Do not configure a lifecycle rule that expires live photo objects.

**Garage / S3-compatible:**

```sh
export LUMIO_STORAGE_URL='s3://lumio?region=garage&endpoint=http://garage:3900&use_path_style=true'
export AWS_ACCESS_KEY_ID='your-access-key'
export AWS_SECRET_ACCESS_KEY='your-secret-key'
```

For AWS S3, use `s3://your-bucket?region=us-east-1` and the AWS default credential chain (environment, IAM role, or workload identity). For local Garage testing on a **fresh library**, use:

```sh
docker build -t lumio:dev .
docker compose -f compose.yaml -f compose.bucket.yaml --profile bucket up --no-build -d
```

The Garage configuration and fixed development credentials in Compose are for local tests only. Garage is optional and runs only with the `bucket` profile. Its RPC and admin ports are not published.

**Google Cloud Storage:**

```sh
export LUMIO_STORAGE_URL='gs://your-bucket?prefix=lumio/'
# Use Application Default Credentials / workload identity, or mount credentials:
export GOOGLE_APPLICATION_CREDENTIALS=/run/secrets/google-credentials.json
```

**Azure Blob Storage:**

```sh
export LUMIO_STORAGE_URL='azblob://your-container?prefix=lumio/'
export AZURE_STORAGE_ACCOUNT='your-account'
export AZURE_STORAGE_KEY='your-key'
```

The Azure driver also supports the Azure default credential chain and SAS credentials. Provider-specific environment variables and URL parameters follow the [Go CDK driver documentation](https://gocloud.dev/howto/blob/).

## Move from PVC to a bucket

1. Back up PostgreSQL and your photo volume. Stop all Lumio replicas for the maintenance window.
2. Run the same binary/image with the existing `DATABASE_URL`, access to the source volume at its existing path, the target provider credentials, and `LUMIO_MIGRATE_TO` set to the target URL.
3. Run `lumio migrate-storage`. The source URL is read from PostgreSQL. Every original and thumbnail is copied and checked by size and SHA-256; originals are also checked against their content ID. Existing matching destination objects are reused, so interrupted migrations can be retried.
4. After success, set `LUMIO_STORAGE_URL` to the exact target URL and restart all replicas. On Kubernetes, switch to the bucket overlay and remove the application PVC mount after verification.

```sh
export DATABASE_URL='postgres://...'
export LUMIO_MIGRATE_TO='gs://my-photo-bucket?prefix=lumio/'
lumio migrate-storage
```

The migration retains all source files. It changes the registered backend only after verification succeeds. A database lock prevents uploads/deletion/collection from racing the switch; stale replicas fail readiness and reject storage mutations. For a Kubernetes migration Job, mount the existing PVC at `/data`, use `args: [migrate-storage]`, and inject the variables above through Secrets/ConfigMaps. Retain the source volume until the migrated library and a new backup have been checked.

The same environment must be able to authenticate to both source and target. For two S3 accounts with different credentials, use AWS profiles in their respective URLs. Use an empty destination prefix; conflicting destination objects are refused rather than overwritten.

## Kubernetes

The default deployment uses one replica, `Recreate`, and a 100 GiB ReadWriteOnce PVC. PostgreSQL is an external dependency: use your homelab's existing PostgreSQL service or operator. Lumio creates its initial tables and indexes at startup under a migration lock; the database role needs schema creation privileges.

```sh
kubectl apply -f deploy/k8s/base/namespace.yaml
# Put DATABASE_URL and LUMIO_PASSWORD in an untracked .env.k8s file.
kubectl -n lumio create secret generic lumio-secrets --from-env-file=.env.k8s
kubectl apply -k deploy/k8s/pvc
```

Set the image tag to an image you have built/published before applying; `v0.1.0` is the initial planned release tag. Configure the PVC size/storage class for your cluster. The container runs as UID/GID 65532 with a read-only root filesystem; the volume must permit this user to write. `/tmp` has a separate ephemeral volume.

For buckets, edit `deploy/k8s/bucket/kustomization.yaml` to set your storage URL and add provider credentials to `lumio-secrets`. GCS credentials files can be mounted from a Secret; set `GOOGLE_APPLICATION_CREDENTIALS` to their mount path. Workload identity may require a Kubernetes service account and token projection appropriate to your provider. The base disables automatic service-account token mounting.

```sh
kubectl apply -k deploy/k8s/bucket
```

The bucket overlay runs two replicas without a photo PVC. All replicas must share the same database, password, and storage configuration. PostgreSQL coordinates deduplication and deletion, and sessions work across replicas without sticky sessions. Each process handles one upload at a time to bound decode memory; increase throughput with more bucket-backed replicas. The deployment is prepared for scaling but has not been load-tested.

Adapt `deploy/k8s/ingress.example.yaml` to your ingress controller, hostname, and TLS setup. Preserve the browser's Host header, allow request bodies of at least the upload limit plus 1 MiB multipart overhead, and allow up to two minutes for an upload. For plain HTTP port-forwarding, temporarily set `LUMIO_SECURE_COOKIE=false`.

`/livez` reports that HTTP is running. `/readyz` checks PostgreSQL and the registered storage configuration; it does not probe bucket permissions on every request. Uploads detect bucket permission or availability failures. The authenticated status panel checks bucket accessibility on demand. Filesystem usage shows the entire mounted filesystem, including files outside Lumio; bucket capacity/quota is not exposed as filesystem capacity.

## Data and backups

PostgreSQL contains photo metadata, favorites, hashed session tokens, login rate limits, and the registered storage URL. The storage backend contains `<sha256>/original` and `<sha256>/thumb.jpg`. Original bytes are not transcoded. Back up **both PostgreSQL and storage together**; the database is required to reconstruct the library.

Deletion hides photos immediately. A background task removes objects and metadata within about 30 seconds, retrying failures. There is no user-facing trash/restore flow. A process crash during an uncommitted upload can leave unreferenced objects; automatic orphan scanning is not implemented. Source files retained after migration are backup copies, and later gallery deletion does not remove those copies.

## Development and releases

```sh
make test
go vet ./...
make build
```

Integration tests create isolated PostgreSQL schemas and bucket prefixes. Set `LUMIO_TEST_DATABASE_URL` and optionally `LUMIO_TEST_STORAGE_URL` plus provider credentials, then run `make test-integration`. The tests cover authentication, CSRF, limits, deduplication across replicas, original-byte preservation, favorites, pagination, deletion, restarts, and migration. CI tests against PostgreSQL and Garage. GCS/Azure drivers are linked but have not been tested against live accounts.

GoReleaser creates Linux/macOS/Windows binaries for amd64 and arm64, archives, checksums, and release notes. The release workflow also publishes a Linux amd64/arm64 image to `ghcr.io/yyvfuruta/lumio:<tag>`.

```sh
make release-check
make release-snapshot          # local artifacts only; no publishing
```

To publish the first release after reviewing CI, ensure `VERSION` is `0.1.0`, commit the changes, push main so the workflow is registered, then push an annotated tag separately:

```sh
git push origin main
git tag -a v0.1.0 -m 'Release v0.1.0'
git push origin v0.1.0
```

Subsequent releases must update `VERSION` to match their `vMAJOR.MINOR.PATCH` tag. Never move an existing tag. GitHub Actions needs repository contents/package write permissions; make the GHCR package accessible to your cluster or configure an image pull Secret. A manual Release workflow run can target an existing tag if publication needs to be retried.

See [API documentation](docs/api.md) for the HTTP resource contract.
