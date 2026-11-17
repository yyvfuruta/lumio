# Development and releases

```sh
make test
go vet ./...
make build
```

Integration tests create isolated PostgreSQL schemas and bucket prefixes. Set `LUMIO_TEST_DATABASE_URL` and optionally `LUMIO_TEST_STORAGE_URL` plus provider credentials, then run `make test-integration`. The tests cover authentication, CSRF, limits, deduplication across replicas, original-byte preservation, favorites, pagination, deletion, restarts, and migration. CI tests against PostgreSQL and Garage. GCS/Azure drivers are linked but have not been tested against live accounts.

GoReleaser creates Linux/macOS/Windows binaries for amd64 and arm64, archives, checksums, and release notes. The release workflow also publishes a Linux amd64/arm64 image to `ghcr.io/yyvfuruta/lumio:<tag>`.

```sh
make release-check
```

Release artifacts are built by GitHub Actions when a version tag is pushed.

To publish a release, update `VERSION` to the target version, commit the change, and push an annotated tag matching `vMAJOR.MINOR.PATCH`. For example:

```sh
git push origin main
git tag -a v0.0.1 -m 'Release v0.0.1'
git push origin v0.0.1
```

Releases must update `VERSION` to match their `vMAJOR.MINOR.PATCH` tag. Never move an existing tag. GitHub Actions needs repository contents/package write permissions; make the GHCR package accessible to your cluster or configure an image pull Secret. A manual Release workflow run can target an existing tag if publication needs to be retried.
