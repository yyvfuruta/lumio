#!/usr/bin/env bash
set -euo pipefail
project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
test_dir="$(mktemp -d)"
trap 'rm -rf -- "$test_dir"' EXIT
javac --add-modules jdk.httpserver -d "$test_dir" \
  "$project_dir/app/src/main/java/io/github/yyvfuruta/lumio/LumioApi.java" "$project_dir/app/src/main/java/io/github/yyvfuruta/lumio/MediaPolicy.java" \
  "$project_dir/app/src/main/java/io/github/yyvfuruta/lumio/MediaDownload.java" \
  "$project_dir/app/src/main/java/io/github/yyvfuruta/lumio/LibraryDates.java" \
  "$project_dir/tests/LibraryDatesTest.java" "$project_dir/tests/TransportTest.java" "$project_dir/tests/LibraryTest.java"
java --add-modules jdk.httpserver -cp "$test_dir" TransportTest
java --add-modules jdk.httpserver -cp "$test_dir" LibraryTest
java --add-modules jdk.httpserver -cp "$test_dir" LibraryDatesTest
