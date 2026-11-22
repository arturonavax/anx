"""Exercise install.sh's assisted setup under a local PTY, without installing."""
import errno
import os
from pathlib import Path
import pty
import select
import shutil
import signal
import sys
import tempfile
import time


def run_installer(installer, answers, *arguments):
    with tempfile.TemporaryDirectory() as directory:
        fixture = Path(directory)
        scripts = fixture / "scripts"
        scripts.mkdir()
        (fixture / "go.mod").touch()
        shutil.copyfile(installer, fixture / "install.sh")
        (scripts / "bootstrap_anx.sh").write_text(
            'printf "DISPATCH checkout=<%s> args=<%s>\\n" '
            '"${ANX_CONFIG_REPO_DIR:-default}" "$*"; exit 0\n'
        )
        stub = fixture / "curl"
        stub.write_text("#!/bin/sh\nprintf '000'\n")
        stub.chmod(0o755)

        pid, fd = pty.fork()
        if pid == 0:
            os.chdir(fixture)
            os.execve("/bin/sh", ["sh", "./install.sh", *arguments], {
                "HOME": directory, "PWD": directory, "PATH": f"{directory}:/usr/bin:/bin",
                "TERM": "dumb", "NO_COLOR": "1",
            })
        output = bytearray()
        try:
            os.write(fd, answers)
            deadline = time.monotonic() + 8
            while time.monotonic() < deadline:
                readable, _, _ = select.select([fd], [], [], 0.2)
                if not readable:
                    continue
                try:
                    chunk = os.read(fd, 8192)
                except OSError as exc:
                    if exc.errno == errno.EIO:
                        break
                    raise
                if not chunk:
                    break
                output.extend(chunk)
            else:
                os.kill(pid, signal.SIGTERM)
                raise AssertionError("installer fixture timed out")
        finally:
            os.close(fd)
            _, status = os.waitpid(pid, 0)
        assert status == 0, output.decode(errors="replace")
        return directory, output.decode(errors="replace")


def main():
    installer = Path(sys.argv[1]).resolve()
    base, output = run_installer(installer, b"1\n1\nother checkout\n")
    assert "Environment checkout path" in output, output
    assert f"DISPATCH checkout=<{base}/other checkout> args=<wizard>" in output, output
    _, output = run_installer(installer, b"1\n1\n\n")
    assert "DISPATCH checkout=<default> args=<wizard>" in output, output
    _, output = run_installer(installer, b"1\n2\n")
    assert "Environment checkout path" not in output, output
    base, output = run_installer(installer, b"manual path\n", "--wizard")
    assert f"DISPATCH checkout=<{base}/manual path> args=<wizard>" in output, output
    _, output = run_installer(installer, b"1\n1\n~/another checkout\n")
    assert "DISPATCH checkout=<~/another checkout> args=<wizard>" in output, output
    for option in ("--yes", "--offline", "--script", "--dry-run"):
        _, output = run_installer(installer, b"", "wizard", option)
        assert "Environment checkout path" not in output, output
        assert "DISPATCH checkout=<default>" in output, output
    print("Interactive checkout prompt: custom, default, engine-only, explicit wizard and unattended checks passed.")


if __name__ == "__main__":
    main()
