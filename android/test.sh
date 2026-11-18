#!/usr/bin/env bash
set -euo pipefail
project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
test_dir="$(mktemp -d)"
trap 'rm -rf -- "$test_dir"' EXIT
javac --add-modules jdk.httpserver -d "$test_dir" \
  "$project_dir/app/src/main/java/io/github/yyvfuruta/lumio/LumioApi.java" "$project_dir/tests/TransportTest.java"
java --add-modules jdk.httpserver -cp "$test_dir" TransportTest
