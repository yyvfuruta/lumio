# Lumio for Android

A native Java client for Android 8.0+ (API 26), using Android views and the Lumio
HTTP API. The library follows the web frontend's layout and colors, with a photo
grid grouped by upload date, a date timeline, search, favorites, selection, and
light/dark/system appearance. No WebView, HTML, or JavaScript is used in the app.
There are no third-party runtime dependencies.

## Install and use

Build with `make android-apk`; the installable, signed APK is
`dist/lumio-android.apk`. Transfer it to your phone and open it, permitting APK
installation for the app you use to open the file. Alternatively:

```sh
adb install -r dist/lumio-android.apk
```

1. Enter the Lumio URL, for example `https://photos.example.com`, and password.
2. After signing in, **Biblioteca** opens a native gallery. The menu provides
   photos, favorites, library/storage status, appearance, refresh, and sign out.
3. Search by filename or use the year/month timeline. More files load as you scroll.
4. Tap a photo to open the native viewer; pinch/double-tap to zoom. Videos use
   Android VideoView and native playback controls. The viewer's actions include
   favorites, original downloads, and confirmed deletion.
5. Use the checkboxes or a long press to select files. Select an entire day or
   all matching files (up to 1000), download a ZIP, or delete with confirmation.
6. Tap Upload to select photos/videos with Android's system picker and send
   them. **Envios** also provides the persistent queue and per-file retry results.

Android Back closes the viewer/selection or returns from status before leaving
the library. Rotation resizes the native grid without cancelling the active
picker. Themes also update Android's system bars. Keep the app open while sending
files; the screen stays awake during transfers. There is no automatic background
backup or offline library.

The app and web frontend share the API, not rendering code. Native rows recycle
as you scroll; thumbnails use a bounded bitmap cache. Image previews are sampled
to at most 2048 pixels per edge before display and honor JPEG EXIF orientation.
GIFs display their first frame. Video playback depends on Android codec support;
unsupported media can still be downloaded as unchanged originals.

Uploads preserve the pending selection with persistent read permissions. After
process termination, reopen **Envios** and retry. Completed files leave the queue,
failures remain, and uncertain retries are deduplicated by the server. The gallery
refreshes after uploads. The server's format/size limits apply (32 MiB by default).
Timeline bounds and date groups use the device timezone, including DST changes.

Originals and ZIPs stream to a location chosen in Android's Save dialog, without
buffering the whole download in RAM. Requests use the native session, reject
foreign origins and redirects, verify declared lengths, and remove partial
documents when the chosen provider allows it. Only Internet access is requested;
the picker grants access to chosen files without broad storage/gallery permission.

Use an address reachable from your phone. `localhost` refers to the phone.
Compose binds port 8080 to loopback by default. For LAN testing, publish it on an
appropriate interface, use the computer's LAN address, select **Permitir HTTP na
rede local**, and set `LUMIO_SECURE_COOKIE=false` on that development server.
Use HTTPS for normal use. Certificate errors are rejected.

Sessions are encrypted with an AES-GCM key in Android Keystore and bound to the
selected server. Passwords are not saved, app backup is disabled, and media
caches are cleared when the library/account closes. Expired sessions return to
login. Native views do not maintain a second browser cookie store.

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
with known/unknown sizes, origin isolation, media/download allowlists, authenticated
original/ZIP streaming, redirects, interrupted transfers, and timezone/DST/leap-year
boundaries used by the native timeline and day selections. Server integration tests
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
