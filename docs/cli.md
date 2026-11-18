# Command-line client

`lumio-cli` is a separate remote client binary. `lumio` runs the server and
administrative commands. The client links only the standard library and needs no
local PostgreSQL, storage credentials, FFmpeg, or server configuration.
`make build` builds both binaries; run `./lumio-cli help` to get started.
Release archives include both executables. To build only the client:

```sh
CGO_ENABLED=0 go build -trimpath -o lumio-cli ./cmd/lumio-cli
```

```sh
export LUMIO_URL=https://photos.example.com
# Pipe a password from your password manager, or read it without terminal echo:
read -rs -p 'Lumio password: ' lumio_password
printf '%s\n' "$lumio_password" | ./lumio-cli login --password-stdin
unset lumio_password

./lumio-cli status
./lumio-cli upload ./holiday.jpg ./clip.mp4
./lumio-cli list --all --query holiday
./lumio-cli --json list --all --favorite
./lumio-cli show PHOTO_ID
./lumio-cli favorite PHOTO_ID
./lumio-cli favorite --unset PHOTO_ID
./lumio-cli download --output holiday-original.jpg PHOTO_ID
./lumio-cli delete --yes PHOTO_ID
./lumio-cli logout
```

Photo IDs are the full lowercase SHA-256 hashes shown by `list` and `upload`.
Options for a command go before its positional arguments. Global `--url`,
`--config`, and `--json` options precede the command.

`list` fetches one page by default and prints its next cursor to stderr. Use
`list --cursor CURSOR` with the same filters for another page, or `--all` to
follow all pages. `--from` and `--until` accept RFC 3339 upload dates (inclusive /
exclusive). `--json list` emits one complete photo object per line (NDJSON),
preserving all API fields. Other metadata commands return JSON; multiple uploads
emit one upload response per line. Errors and server logs do not mix with client
stdout. Failures exit with a nonzero status. Batch operations stop on the first
error; earlier successful uploads or deletions are retained.

Sessions are stored in `lumio/cli.json` below the OS user configuration directory
(`~/.config` on most Linux systems). `LUMIO_CLI_CONFIG` or `--config` overrides
this location. The file is written atomically with owner-only permissions on
Unix. It contains a session credential, never the password. Keep it private.
The selected URL is saved with the session, and credentials are sent only when
the selected URL exactly matches it. An explicit `--url` takes precedence over
`LUMIO_URL`, then the saved URL, then `http://localhost:8080`. This is a
single-session configuration; use distinct config files for multiple servers.
`LUMIO_PASSWORD` is an alternative to `login --password-stdin`.

Uploads stream from disk and are sequential. Duplicate files reuse the server's
existing resource. Downloads stream to a temporary file in the destination
directory and publish it only on success; existing files are never overwritten.
The destination filesystem must support hard links. Ctrl+C cancels the active
request. Redirects are rejected so credentials are never forwarded to a new URL.

HTTPS uses the operating system's certificate trust. For an HTTP development
server, set `LUMIO_SECURE_COOKIE=false` on the server; do not disable certificate
validation. Default secure cookies cannot be used over HTTP.

Server and administrative commands continue to use environment configuration:

```sh
./lumio serve           # Also the default when no command is supplied.
./lumio worker          # Maintenance only; no HTTP listener.
./lumio migrate         # Idempotent schema initialization/update.
./lumio migrate-storage # DATABASE_URL and LUMIO_MIGRATE_TO.
./lumio version
```
