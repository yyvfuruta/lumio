# Lumio for Android

A native Java uploader for Android 8.0+ (API 26). It connects to the same HTTP API
as the web interface and CLI. There are no third-party runtime dependencies.

## Install and use

Build with `make android-apk`; the installable, signed APK is
`dist/lumio-android.apk`. Transfer it to your phone and open it, permitting APK
installation for the app you use to open the file. Alternatively:

```sh
adb install -r dist/lumio-android.apk
```

1. Enter your Lumio URL, for example `https://photos.example.com`, and password.
2. Tap **Conectar ao servidor**, then **Selecionar fotos e vídeos**.
3. Choose files using Android's system file picker, then tap **Enviar para o Lumio**.

Keep the app open during uploads. The screen stays awake while a request is in
progress. Rotation retains the active operation. The pending selection is saved
with persistent read permissions; after process termination, reopen the app and
tap Send to retry it. Completed files are removed from the queue, failures remain
selected, and retrying an uncertain upload is safe because the server deduplicates
original bytes. This version does not perform automatic background backups.

The server validates supported formats and enforces its per-file limit, including
for videos (32 MiB by default). The app reads that limit after login and checks
known sizes before sending. Network requests run off the UI thread, stream files
without buffering them in memory, and show per-file progress/results.

Use an address reachable from your phone. `localhost` on the phone points to the
phone, not your computer. Compose binds port 8080 to loopback by default. For LAN
testing, publish it on an appropriate interface with your own Compose override,
use your computer's LAN address, select **Permitir HTTP na rede local**, and set
`LUMIO_SECURE_COOKIE=false` on that development server. Use HTTPS for normal use.
The app uses Android's certificate trust and rejects redirects; self-signed
certificates are not silently accepted.

Only Internet access is requested. The system picker grants access to selected
files without broad gallery/storage permissions. Session credentials are encrypted
using an AES-GCM key in Android Keystore and bound to their server URL. Passwords
are not saved; app backup is disabled. Sign out attempts server revocation and
always removes the local credential, reporting if server revocation failed.

## Build

Requirements: Bash, Python 3, JDK 21, and the official Android SDK with
`platforms;android-35` and `build-tools;35.0.0`. Install the SDK command-line tools
from [Android Developers](https://developer.android.com/studio#command-line-tools-only),
accept its license, and install the packages:

```sh
sdkmanager 'platforms;android-35' 'build-tools;35.0.0' 'platform-tools'
export ANDROID_HOME=/path/to/android-sdk
make android-test
make android-apk
```

The build script uses `aapt2`, `javac`, D8, `zipalign`, and `apksigner` directly.
It verifies the final APK's signature. The default local development signing key
is generated once at `.local/android-debug.keystore`; preserve it to install
updates over an existing build. Neither signing keys nor APKs are committed.
For a distribution key, set `LUMIO_ANDROID_KEYSTORE`, `LUMIO_ANDROID_KEY_ALIAS`,
`LUMIO_ANDROID_STORE_PASSWORD`, and `LUMIO_ANDROID_KEY_PASSWORD`. Passwords are
passed to the signer by environment variable. CI builds use development keys;
those artifacts are for testing, not stable release updates.

`android/test.sh` runs JVM transport tests for login, cookies, multipart streaming
with known/unknown sizes, unsafe URLs, and redirects. Server integration tests
exercise the API against PostgreSQL and local/S3 storage. These do not replace
on-device testing of the picker, Keystore, lifecycle, or device-specific UI.

## Release assets

The release workflow attaches an APK and its SHA-256 checksum after publishing
the server/client archives and container. With repository secrets
`ANDROID_KEYSTORE_BASE64`, `ANDROID_KEY_ALIAS`, `ANDROID_STORE_PASSWORD`, and
`ANDROID_KEY_PASSWORD`, it signs `lumio-android.apk` using your persistent key.
Keep that key backed up for future Android updates. Without a configured key,
it publishes `lumio-android-dev.apk`, explicitly a development build; its key is
new for each workflow and installing a later build may require uninstalling the
previous one. No private signing material is committed to this repository.
App version name and increasing version code are derived from root `VERSION`.
