Video processing and video tests require FFmpeg and FFprobe on PATH. The runtime container includes both tools.

# Development and releases

```sh
make test
go vet ./...
make build
```

Integration tests create isolated PostgreSQL schemas and bucket prefixes. Set `LUMIO_TEST_DATABASE_URL` and optionally `LUMIO_TEST_STORAGE_URL` plus provider credentials, then run `make test-integration`. The tests cover authentication, CSRF, limits, deduplication across replicas, original-byte preservation, favorites, pagination, deletion, restarts, and migration. CI tests against PostgreSQL and Garage. GCS/Azure drivers are linked but have not been tested against live accounts.

GoReleaser creates separate `lumio` server and `lumio-cli` client binaries for Linux/macOS/Windows on amd64 and arm64. Each platform archive includes both executables, checksums, and release notes. The release workflow also publishes a Linux amd64/arm64 image to `ghcr.io/yyvfuruta/lumio:<tag>`.

Release images reuse the Linux binaries built by GoReleaser; the container packaging step does not compile Go again. CI and releases use `-trimpath` consistently so their native Go compilation caches are compatible. The regular Dockerfile still builds from source for local development. New pushes cancel older CI runs for the same branch, while release runs finish independently.

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

The Android release job also attaches an APK and checksum. Without persistent
Android signing secrets, it labels the APK `lumio-android-dev.apk`; see
[Android signing and release assets](../android/README.md#release-assets).
