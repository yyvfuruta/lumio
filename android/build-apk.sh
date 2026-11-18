#!/usr/bin/env bash
# Build the dependency-free Java client using the official Android SDK.
set -euo pipefail
project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(dirname -- "$project_dir")"
sdk_dir="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}"
if [[ -z "$sdk_dir" ]]; then
  echo 'Set ANDROID_HOME to an Android SDK with platforms;android-35 and build-tools;35.0.0.' >&2
  exit 1
fi
tools_dir="$sdk_dir/build-tools/35.0.0"
android_jar="$sdk_dir/platforms/android-35/android.jar"
for tool in javac jar keytool python3; do command -v "$tool" >/dev/null; done
for tool in aapt2 d8 zipalign apksigner; do test -x "$tools_dir/$tool"; done
app_version="$(cat "$repo_dir/VERSION")"
version_code="$(python3 - "$app_version" <<'VERSIONPY'
import re, sys
version = sys.argv[1]
if not re.fullmatch(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)', version):
    raise SystemExit('VERSION must be MAJOR.MINOR.PATCH')
major, minor, patch = map(int, version.split('.'))
code = major * 1000000 + minor * 1000 + patch
if minor > 999 or patch > 999 or not 0 < code <= 2100000000:
    raise SystemExit('VERSION cannot be represented as an Android version code')
print(code)
VERSIONPY
)"
mkdir -p "$repo_dir/.local" "$repo_dir/dist"
build_dir="$(mktemp -d "$repo_dir/.local/android-build-XXXXXX")"
trap 'rm -rf -- "$build_dir"' EXIT
mkdir -p "$build_dir/generated" "$build_dir/classes" "$build_dir/dex"
"$tools_dir/aapt2" compile --dir "$project_dir/app/src/main/res" -o "$build_dir/resources.zip"
"$tools_dir/aapt2" link -o "$build_dir/resources.apk" -I "$android_jar" \
  --manifest "$project_dir/app/src/main/AndroidManifest.xml" --java "$build_dir/generated" \
  --min-sdk-version 26 --target-sdk-version 35 --version-code "$version_code" --version-name "$app_version" \
  "$build_dir/resources.zip"
find "$project_dir/app/src/main/java" "$build_dir/generated" -name '*.java' > "$build_dir/sources.txt"
javac -encoding UTF-8 --release 8 -classpath "$android_jar" -d "$build_dir/classes" @"$build_dir/sources.txt"
jar --create --file "$build_dir/classes.jar" -C "$build_dir/classes" .
"$tools_dir/d8" --lib "$android_jar" --min-api 26 --output "$build_dir/dex" "$build_dir/classes.jar"
python3 - "$build_dir" <<'PY'
import pathlib, sys, zipfile
build = pathlib.Path(sys.argv[1])
with zipfile.ZipFile(build / 'resources.apk', 'a') as apk:
    for dex in sorted((build / 'dex').glob('*.dex')):
        apk.write(dex, dex.name)
PY
"$tools_dir/zipalign" -f -p 4 "$build_dir/resources.apk" "$build_dir/aligned.apk"
# Preserve the local signing key so rebuilds can update an installed APK.
keystore="${LUMIO_ANDROID_KEYSTORE:-$repo_dir/.local/android-debug.keystore}"
if [[ -z "${LUMIO_ANDROID_KEYSTORE:-}" && ! -f "$keystore" ]]; then
  keytool -genkeypair -keystore "$keystore" -storepass android -keypass android \
    -alias androiddebugkey -keyalg RSA -keysize 2048 -validity 10000 \
    -dname 'CN=Lumio Local Development' >/dev/null
  chmod 600 "$keystore"
fi
export LUMIO_ANDROID_STORE_PASSWORD="${LUMIO_ANDROID_STORE_PASSWORD:-android}"
export LUMIO_ANDROID_KEY_PASSWORD="${LUMIO_ANDROID_KEY_PASSWORD:-$LUMIO_ANDROID_STORE_PASSWORD}"
"$tools_dir/apksigner" sign --ks "$keystore" --ks-key-alias "${LUMIO_ANDROID_KEY_ALIAS:-androiddebugkey}" \
  --ks-pass env:LUMIO_ANDROID_STORE_PASSWORD --key-pass env:LUMIO_ANDROID_KEY_PASSWORD \
  --out "$repo_dir/dist/lumio-android.apk" "$build_dir/aligned.apk"
"$tools_dir/apksigner" verify --verbose "$repo_dir/dist/lumio-android.apk"
echo "APK: $repo_dir/dist/lumio-android.apk"
