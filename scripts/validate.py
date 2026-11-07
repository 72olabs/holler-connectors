#!/usr/bin/env python3
"""Validate extracted packages without private core source or a running harness."""
import hashlib
import json
from pathlib import Path, PurePosixPath
import subprocess

ROOT = Path(__file__).resolve().parents[1]
MARKETPLACE = ROOT / "connectors/marketplace"


def validate(root=MARKETPLACE):
    count = 0
    for path in root.rglob("*"):
        if path.is_symlink():
            raise ValueError(f"Symlink in package: {path}")
        if path.suffix == ".json":
            json.loads(path.read_text())
    for name, harness in [("holler", "codex"), ("claude-holler", "claude"), ("opencode-holler", "opencode")]:
        package = root / "plugins" / name
        manifest = json.loads((package / "connector.json").read_text())
        if manifest["harness"] != harness or manifest["schema_version"] != 1:
            raise ValueError(f"Invalid identity/schema: {name}")
        if harness == "codex":
            event_names = {"SessionStart": {"startup", "resume", "clear", "compact"},
                           "SessionEnd": {"end"}, "PostToolUse": {"post_tool_use"}, "Stop": {"stop"}}
            hooks = json.loads((package / "hooks/hooks.json").read_text())["hooks"]
            lifecycle = manifest["lifecycle_events"]
            if set(hooks) != set(event_names) or set(lifecycle) != set().union(*event_names.values()):
                raise ValueError("Codex hook events and lifecycle contract do not match")
            starts = set().union(*(set(entry.get("matcher", "").split("|")) for entry in hooks["SessionStart"]))
            if starts != event_names["SessionStart"]:
                raise ValueError("Codex SessionStart hook events and lifecycle contract do not match")
            for event in ("PostToolUse", "Stop"):
                commands = [hook["command"] for entry in hooks[event] for hook in entry["hooks"]]
                if not commands or not all(command.endswith(" codex-attention --event " + event) for command in commands):
                    raise ValueError(f"Codex {event} hook events and attention command do not match")
        digest = hashlib.sha256()
        assets = manifest["required_assets"]
        if len(assets) != len(set(assets)):
            raise ValueError("Duplicate assets")
        for relative in sorted(assets):
            path = PurePosixPath(relative)
            if path.is_absolute() or ".." in path.parts or str(path) != relative:
                raise ValueError(f"Unsafe asset: {relative}")
            asset = package / relative
            if not asset.is_file() or asset.is_symlink():
                raise ValueError(f"Missing/unsafe asset: {relative}")
            digest.update(relative.encode() + b"\0" + asset.read_bytes() + b"\0")
        if "sha256:" + digest.hexdigest() != manifest["package_hash"]:
            raise ValueError(f"Package hash mismatch: {name}")
        if not (package / "scripts/holler").stat().st_mode & 0o111:
            raise ValueError(f"Launcher not executable: {name}")
        subprocess.run(["/bin/sh", "-n", str(package / "scripts/holler")], check=True)
        for overlay in [".codex-plugin/plugin.json", ".claude-plugin/plugin.json"]:
            path = package / overlay
            if path.exists():
                plugin = json.loads(path.read_text())
                if overlay == ".codex-plugin/plugin.json" and plugin.get("license") != "Apache-2.0":
                    raise ValueError(f"Invalid product license: {name}")
                if plugin["name"] != manifest["plugin_name"] or plugin["version"] != manifest["connector_version"]:
                    raise ValueError(f"Plugin identity/version mismatch: {name}")
        count += 1
    codex = json.loads((root / ".agents/plugins/marketplace.json").read_text())
    claude = json.loads((root / ".claude-plugin/marketplace.json").read_text())
    if codex["name"] != "holler" or claude["name"] != "holler":
        raise ValueError("Marketplace identity changed")
    for marketplace in [codex, claude]:
        for plugin in marketplace["plugins"]:
            source = plugin["source"]
            relative = source["path"] if isinstance(source, dict) else source
            path = PurePosixPath(relative)
            if path.is_absolute() or ".." in path.parts or not (root / path).is_dir():
                raise ValueError("Marketplace source escapes or is missing")
    return count


if __name__ == "__main__":
    print(f"Validated {validate()} connector packages; no live readiness claim.")
