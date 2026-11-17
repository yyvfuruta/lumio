# Storage

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
3. Run `lumio migrate-storage`. The source URL is read from PostgreSQL. Every original, thumbnail, and HEIC preview is copied and checked by size and SHA-256; originals are also checked against their content ID. Existing matching destination objects are reused, so interrupted migrations can be retried.
4. After success, set `LUMIO_STORAGE_URL` to the exact target URL and restart all replicas. On Kubernetes, switch to the bucket overlay and remove the application PVC mount after verification.

```sh
export DATABASE_URL='postgres://...'
export LUMIO_MIGRATE_TO='gs://my-photo-bucket?prefix=lumio/'
lumio migrate-storage
```

The migration retains all source files. It changes the registered backend only after verification succeeds. A database lock prevents uploads/deletion/collection from racing the switch; stale replicas fail readiness and reject storage mutations. For a Kubernetes migration Job, mount the existing PVC at `/data`, use `args: [migrate-storage]`, and inject the variables above through Secrets/ConfigMaps. Retain the source volume until the migrated library and a new backup have been checked.

The same environment must be able to authenticate to both source and target. For two S3 accounts with different credentials, use AWS profiles in their respective URLs. Use an empty destination prefix; conflicting destination objects are refused rather than overwritten.

## Data and backups

PostgreSQL contains photo metadata, favorites, hashed session tokens, login rate limits, and the registered storage URL. The storage backend contains `<sha256>/original`, `<sha256>/thumb.jpg`, and a `<sha256>/preview.jpg` for HEIC photos. Original bytes are not transcoded. Back up **both PostgreSQL and storage together**; the database is required to reconstruct the library.

Deletion hides photos immediately. A background task removes objects and metadata within about 30 seconds, retrying failures. There is no user-facing trash/restore flow. A process crash during an uncommitted upload can leave unreferenced objects; automatic orphan scanning is not implemented. Source files retained after migration are backup copies, and later gallery deletion does not remove those copies.
