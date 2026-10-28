"""Build CLI and GUI release assets on the target OS. Requires Go, Node and Wails 3."""

import argparse
import json
import os
from pathlib import Path
import plistlib
import re
import shutil
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parents[1]
GUI = ROOT / "gui"


def run(*args, cwd=ROOT, env=None):
    print("+", " ".join(map(str, args)), flush=True)
    subprocess.run(list(map(str, args)), cwd=cwd, env=env, check=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag", nargs="?", default=os.environ.get("RELEASE_TAG"))
    args = parser.parse_args()
    if not args.tag or not re.fullmatch(r"v\d+\.\d+\.\d+", args.tag):
        parser.error("provide a tag such as v1.2.3 (or set RELEASE_TAG)")
    version = args.tag[1:]
    goos, arch = subprocess.check_output(
        ["go", "env", "GOHOSTOS", "GOHOSTARCH"], text=True
    ).split()
    if goos not in {"windows", "darwin", "linux"} or arch not in {"amd64", "arm64"}:
        parser.error(f"unsupported host: {goos}/{arch}")
    suffix = f"{'macos' if goos == 'darwin' else goos}-{arch}"
    dist = ROOT / "dist"
    dist.mkdir(exist_ok=True)
    cli = dist / f"crdl-{suffix}{'.exe' if goos == 'windows' else ''}"
    resource = ROOT / f"release_windows_{arch}.syso"
    metadata = [GUI / "build/config.yml", GUI / "build/windows/info.json", GUI / "build/darwin/Info.plist"]
    originals = {path: path.read_bytes() for path in metadata}
    if resource.exists():
        raise RuntimeError(f"refusing to overwrite {resource}")
    env = dict(os.environ, GOOS=goos, GOARCH=arch)
    try:
        config, info, plist = metadata
        config.write_text(re.sub(r'(?m)^(  version:) "[^"]+"', rf'\1 "{version}"', config.read_text()))
        data = json.loads(info.read_text())
        data["fixed"]["file_version"] = version
        data["info"]["0409"]["ProductVersion"] = version
        info.write_text(json.dumps(data, indent=2))
        data = plistlib.loads(originals[plist])
        data["CFBundleShortVersionString"] = version
        data["CFBundleVersion"] = version
        plist.write_bytes(plistlib.dumps(data))
        run("wails3", "task", "common:generate:icons", cwd=GUI)
        if goos == "windows":
            run("wails3", "generate", "syso", "-arch", arch,
                "-icon", GUI / "build/windows/icon.ico", "-manifest", ROOT / "scripts/cli.manifest",
                "-info", info, "-out", resource)
        run("go", "build", "-trimpath", "-ldflags=-s -w", "-o", cli, ".",
            env=dict(env, CGO_ENABLED="0"))
        run("wails3", "package" if goos == "darwin" else "build", f"ARCH={arch}", cwd=GUI, env=env)
        if goos == "windows":
            shutil.copy2(GUI / "bin/crdl-gui.exe", dist / f"crdl-gui-{suffix}.exe")
        elif goos == "darwin":
            # Preserve bundle permissions and its ad-hoc signature.
            run("ditto", "-c", "-k", "--sequesterRsrc", "--keepParent",
                GUI / "bin/crdl-gui.app", dist / f"crdl-gui-{suffix}.zip")
        else:
            with tarfile.open(dist / f"crdl-gui-{suffix}.tar.gz", "w:gz") as archive:
                archive.add(GUI / "bin/crdl-gui", arcname="bin/crdl-gui")
                archive.add(GUI / "build/linux/crdl-gui.desktop", arcname="share/applications/crdl-gui.desktop")
                archive.add(GUI / "build/appicon.png", arcname="share/pixmaps/crdl-gui.png")
                archive.add(ROOT / "LICENSE.txt", arcname="LICENSE.txt")
    finally:
        for path, content in originals.items():
            path.write_bytes(content)
        resource.unlink(missing_ok=True)


if __name__ == "__main__":
    main()
