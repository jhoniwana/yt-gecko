#!/usr/bin/env python3
"""Copy the host libraries a binary needs but the juNest rootfs does not ship.

Walks the dynamic dependency closure of the given binary; every resolved
library outside the rootfs is copied (with its own dependencies) into the
destination directory, so the payload runs on machines without those system
libraries.
"""
import os
import shutil
import subprocess
import sys


def ldd(path):
    """Return {soname: resolved path} for a binary, skipping unresolved ones."""
    try:
        out = subprocess.run(["ldd", path], capture_output=True, text=True).stdout
    except OSError:
        return {}
    libs = {}
    for line in out.splitlines():
        line = line.strip()
        if "=>" in line:
            name, rest = line.split("=>", 1)
            rest = rest.strip()
            if rest.startswith("not found"):
                continue
            lib = rest.split(" (")[0].strip()
            if lib:
                libs[name.strip()] = lib
        elif line.startswith("/") and "(" in line:
            lib = line.split(" (")[0].strip()
            libs[os.path.basename(lib)] = lib
    return libs


def main():
    if len(sys.argv) != 4:
        print(__doc__)
        return 1
    root, binary, dest = sys.argv[1], sys.argv[2], sys.argv[3]
    os.makedirs(dest, exist_ok=True)
    root_prefix = os.path.realpath(root) + os.sep

    queue = [binary]
    seen = set()
    copied = set()
    while queue:
        current = queue.pop()
        if current in seen:
            continue
        seen.add(current)
        for name, path in ldd(current).items():
            if name.startswith("ld-linux") or name.startswith("ld-musl"):
                continue  # the rootfs ships its own dynamic loader
            real = os.path.realpath(path)
            if real.startswith(root_prefix):
                continue  # already inside the rootfs
            target = os.path.join(dest, name)
            if os.path.exists(target):
                continue
            try:
                shutil.copy2(real, target, follow_symlinks=True)
            except OSError as err:
                print(f"warning: could not copy {path}: {err}", file=sys.stderr)
                continue
            copied.add(name)
            queue.append(target)

    print("bundled extra libraries:", " ".join(sorted(copied)) or "(none)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
