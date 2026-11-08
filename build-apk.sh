#!/usr/bin/env bash
# Builds dist/inferno-wings-android.apk: a full-screen WebView app with game/ packed into its assets.
# No Gradle needed, only: a JDK 17+, Android SDK build-tools 35.0.0 and platform android-35.
# Pure Java, no native code, so one APK runs on every Android ABI (arm64, armv7, x86, x86_64), Android 5.0+.
set -euo pipefail
cd "$(dirname "$0")"

SDK=${ANDROID_HOME:-$HOME/Android/Sdk}
BT=$SDK/build-tools/35.0.0
PLATFORM=$SDK/platforms/android-35/android.jar
if [ -z "${JAVA_HOME:-}" ] && [ -x "$HOME/.local/opt/jdk-17/bin/javac" ]; then JAVA_HOME=$HOME/.local/opt/jdk-17; fi
[ -n "${JAVA_HOME:-}" ] && export JAVA_HOME PATH="$JAVA_HOME/bin:$PATH"

VERSION_CODE=${VERSION_CODE:-1}
VERSION_NAME=${VERSION_NAME:-1.0}
# Signing key. Keep this file safe: Android only installs updates signed with the same key.
KEYSTORE=${KEYSTORE:-android/inferno-wings.keystore}
KS_PASS=${KS_PASS:-infernowings}

B=android/build
rm -rf "$B"; mkdir -p "$B/classes" "$B/dex" "$B/assets" dist
cp -r game/. "$B/assets/"

echo "-> resources"
"$BT/aapt2" compile --dir android/res -o "$B/res.zip"
"$BT/aapt2" link -o "$B/unsigned.apk" -I "$PLATFORM" --manifest android/AndroidManifest.xml \
  -A "$B/assets" "$B/res.zip" --min-sdk-version 21 --target-sdk-version 35 \
  --version-code "$VERSION_CODE" --version-name "$VERSION_NAME" -0 woff2

echo "-> java"
javac -nowarn -Xlint:-options -source 8 -target 8 -bootclasspath "$PLATFORM" -d "$B/classes" \
  $(find android/src -name '*.java')
"$BT/d8" --release --min-api 21 --lib "$PLATFORM" --output "$B/dex" $(find "$B/classes" -name '*.class')
(cd "$B/dex" && zip -q -j ../unsigned.apk classes.dex)

echo "-> sign"
if [ ! -f "$KEYSTORE" ]; then
  keytool -genkeypair -keystore "$KEYSTORE" -storepass "$KS_PASS" -keypass "$KS_PASS" -alias inferno \
    -keyalg RSA -keysize 4096 -validity 10000 -dname "CN=Inferno Wings" >/dev/null 2>&1
  echo "   created new signing key $KEYSTORE"
fi
"$BT/zipalign" -p -f 4 "$B/unsigned.apk" "$B/aligned.apk"
"$BT/apksigner" sign --ks "$KEYSTORE" --ks-pass "pass:$KS_PASS" --ks-key-alias inferno \
  --out dist/inferno-wings-android.apk "$B/aligned.apk"
"$BT/apksigner" verify dist/inferno-wings-android.apk
rm -f dist/inferno-wings-android.apk.idsig
ls -lh dist/inferno-wings-android.apk
