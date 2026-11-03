from pathlib import Path
import shutil
import json
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
from validate import MARKETPLACE, validate


class PackageTests(unittest.TestCase):
    def test_product_license_preserves_prior_grants(self):
        license_text = (MARKETPLACE.parents[1] / "LICENSE").read_text()
        self.assertTrue(license_text.startswith("Holler Proprietary Software License\n"))
        self.assertIn("Appendix A: License retained", license_text)
        self.assertIn("Apache License", license_text)
        self.assertIn("Nothing in this license withdraws or limits rights previously granted", license_text)

    def test_all_packages(self):
        self.assertEqual(validate(), 3)

    def test_codex_hook_events_match_manifest_independently_of_asset_hash(self):
        for event in ("SessionStart", "SessionEnd", "PostToolUse", "Stop"):
            with self.subTest(event=event), tempfile.TemporaryDirectory() as directory:
                root = Path(directory) / "marketplace"
                shutil.copytree(MARKETPLACE, root)
                path = root / "plugins/holler/hooks/hooks.json"
                hooks = json.loads(path.read_text())
                del hooks["hooks"][event]
                path.write_text(json.dumps(hooks))
                with self.assertRaisesRegex(ValueError, "hook events"):
                    validate(root)

    def test_changed_asset_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "marketplace"
            shutil.copytree(MARKETPLACE, root)
            (root / "plugins/holler/scripts/holler").write_text("changed")
            with self.assertRaises(ValueError):
                validate(root)

    def test_symlink_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "marketplace"
            shutil.copytree(MARKETPLACE, root)
            (root / "unexpected").symlink_to("plugins/holler")
            with self.assertRaises(ValueError):
                validate(root)

    def test_codex_lifecycle_fails_open_but_mcp_fails_closed(self):
        wrapper = MARKETPLACE / "plugins/holler/scripts/holler"
        with tempfile.TemporaryDirectory() as directory:
            for command in ["hook", "session-end", "monitor", "codex-attention", "mcp"]:
                result = subprocess.run(["/bin/sh", str(wrapper), command],
                                        env={"PATH": directory, "HOME": directory}, capture_output=True)
                self.assertEqual(result.returncode, 2 if command == "mcp" else 0)

    def test_codex_forwards_arguments_without_shell_interpretation(self):
        wrapper = MARKETPLACE / "plugins/holler/scripts/holler"
        result = subprocess.run(["/bin/sh", str(wrapper), "mcp", "literal; value"],
                                env={"HOLLER_BIN": "/bin/echo"}, capture_output=True, text=True, check=True)
        self.assertEqual(result.stdout, "mcp literal; value\n")

    def test_claude_noninteractive_monitor_does_not_block(self):
        wrapper = MARKETPLACE / "plugins/claude-holler/scripts/holler"
        for entry in ["sdk-cli", "sdk-ts", "sdk-py"]:
            subprocess.run(["/bin/sh", str(wrapper), "monitor"], check=True,
                           env={"HOLLER_BIN": "/usr/bin/false", "CLAUDE_CODE_ENTRYPOINT": entry})


if __name__ == "__main__":
    unittest.main()
