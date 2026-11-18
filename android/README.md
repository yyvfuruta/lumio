# Lumio for Android

An Android client for Android 8.0+ (API 26). Its **Biblioteca** tab displays the
server's actual responsive web frontend in a WebView, keeping the same gallery,
upload-date timeline, search, favorites, light/dark/system themes, photo viewer,
video player, selection, deletion, and library status as the browser. The
**Envios** tab retains the native uploader and its persistent pending queue.
There are no third-party runtime dependencies.

## Install and use

Build with `make android-apk`; the installable, signed APK is
`dist/lumio-android.apk`. Transfer it to your phone and open it, permitting APK
installation for the app you use to open the file. Alternatively:

```sh
adb install -r dist/lumio-android.apk
```

1. Enter your Lumio URL, for example `https://photos.example.com`, and password.
2. After signing in, the app opens **Biblioteca**, using the same interface as the web.
3. Browse your photos/videos, search, use the date timeline or favorites, and tap
   a thumbnail to open its viewer. The library menu includes appearance and status.
4. Tap the web interface's upload button to select photos/videos with Android's
   system picker. Originals and selection ZIPs open Android's Save dialog.
5. Use **Envios** for the native upload queue/retries, or the app's ⋮ menu to
   refresh the library or configure another server.

The Android Back button closes the viewer, menus, or selection before leaving
the library. Video fullscreen is supported. Rotation keeps the WebView and active
file picker intact. Errors opening the server show Retry and server settings.
The native chrome follows the library's theme. The phone stays awake during
uploads; keep the app open until they finish. There are no automatic background
backups or offline gallery. Update Android System WebView for current web support.

The web interface is loaded directly from your configured server, so frontend
updates reach Android without shipping a second copy of its HTML/CSS/JS in the
APK. The interface therefore matches the version running on your server.

Native uploads preserve their pending selection with persistent read permissions.
After process termination, reopen **Envios** and retry. Successful files leave
the queue, failures remain, and uncertain retries are deduplicated by the server.
The library refreshes after native uploads. Uploads started in the web interface
use its existing per-file validation/retry flow and do not enter the native queue.
The server's format and upload limits apply to both flows (32 MiB by default).

Downloads stream originals and ZIPs to a destination selected with Android's
Storage Access Framework. They reuse the library's session, reject redirects and
foreign origins, verify declared response lengths, and remove partial documents
when the provider allows it. No broad storage/gallery permission is required.

Use a server address reachable from your phone. `localhost` on the phone refers
to the phone. Compose binds port 8080 to loopback by default. For LAN testing,
publish it on an appropriate interface with a Compose override, use the computer's
LAN address, select **Permitir HTTP na rede local**, and set
`LUMIO_SECURE_COOKIE=false` on that development server. Use HTTPS for normal use.

Only Internet access is requested. HTTPS uses Android's certificate trust;
certificate errors are rejected. The WebView is restricted to the configured
origin, rejects mixed content, disables file-URL access, third-party cookies,
popups, and JavaScript-to-native interfaces. Selected content URIs are supplied
only through the system picker. An encrypted session snapshot in Android Keystore
restores native authentication; the WebView also maintains its own cookie store
inside the app's private storage. Browser session changes synchronize back to
the native uploader. Passwords are not saved; app backup is disabled.

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
with known/unknown sizes, origin isolation, download allowlists, authenticated
original/ZIP streaming, redirects, and interrupted transfers. Server integration tests
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
