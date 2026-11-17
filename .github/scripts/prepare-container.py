"""Stage GoReleaser's Linux binaries for the multi-platform container build."""

import json
from pathlib import Path
import shutil


artifacts = json.loads(Path("dist/artifacts.json").read_text())
context = Path("dist/container")
(context / "data").mkdir(parents=True, exist_ok=True)

for arch in ("amd64", "arm64"):
    matches = [
        artifact
        for artifact in artifacts
        if artifact["type"] == "Binary"
        and artifact.get("goos") == "linux"
        and artifact.get("goarch") == arch
        and artifact["name"] == "lumio"
    ]
    if len(matches) != 1:
        raise SystemExit(f"Expected one Linux {arch} binary, found {len(matches)}")
    target = context / "linux" / arch / "lumio"
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(matches[0]["path"], target)
    target.chmod(0o755)
