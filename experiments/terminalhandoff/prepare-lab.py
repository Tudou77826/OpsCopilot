"""Prepare pinned Alpine x86_64 shell binaries inside the ignored lab directory.

No system installation, shell-profile edit, symlink extraction or service change.
Requires Python 3.10+. Use only with the disposable Alpine/musl WSL lab.
"""

import hashlib
import io
import json
from pathlib import Path, PurePosixPath
import shutil
import tarfile
import urllib.request


def main():
    source = Path(__file__).resolve().parent
    runtime = source.parents[1] / "e2e" / ".runtime" / "terminal-handoff"
    runtime.mkdir(parents=True, exist_ok=True)
    target = (runtime / "alpine").resolve()
    dependencies = json.loads((source / "lab-dependencies.json").read_text("utf-8"))
    for dependency in dependencies:
        archive = runtime / (dependency["package"] + ".apk")
        if archive.exists():
            data = archive.read_bytes()
        else:
            with urllib.request.urlopen(dependency["source"], timeout=30) as response:
                data = response.read(20 * 1024 * 1024 + 1)
        if hashlib.sha256(data).hexdigest() != dependency["sha256"]:
            raise RuntimeError("Dependency hash mismatch: " + dependency["package"])
        archive.write_bytes(data)
        with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as apk:
            for member in apk:
                name = PurePosixPath(member.name)
                if not member.isfile() or name.is_absolute() or ".." in name.parts:
                    continue
                destination = target.joinpath(*name.parts).resolve()
                if not destination.is_relative_to(target):
                    raise RuntimeError("Archive path escapes lab directory")
                destination.parent.mkdir(parents=True, exist_ok=True)
                content = apk.extractfile(member)
                if content is None:
                    raise RuntimeError("Missing archive content")
                with content, destination.open("wb") as output:
                    shutil.copyfileobj(content, output)
        print("Verified and extracted " + dependency["package"] + " " + dependency["version"])
    # APK links are deliberately ignored; create a regular local copy instead.
    library = target / "usr" / "lib"
    shutil.copyfile(library / "libreadline.so.8.2", library / "libreadline.so.8")
    print("Lab files: " + str(target))


if __name__ == "__main__":
    main()
