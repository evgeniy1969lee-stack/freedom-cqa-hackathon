"""Build and run the CQA app with the local Ollama evaluator."""

import argparse
from contextlib import ExitStack
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
from tempfile import TemporaryDirectory
import time
from urllib.error import URLError
from urllib.request import ProxyHandler, build_opener

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from ml.evaluator import MODEL


def wait_for_api(url, processes):
    opener = build_opener(ProxyHandler({}))
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        for name, process in processes.items():
            if process.poll() is not None:
                raise RuntimeError(f"{name} stopped with exit code {process.returncode}")
        try:
            with opener.open(f"{url}/api/health", timeout=2) as response:
                health = json.load(response)
            if health.get("ml") == "http" and health.get("ml_ready") and health.get("ml_model") == MODEL:
                return
        except (URLError, TimeoutError, ValueError):
            pass
        time.sleep(0.25)
    raise RuntimeError(f"CQA did not become ready. Check that Ollama is running and {MODEL} is installed.")


def stop(process):
    if process.poll() is None:
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api-port", type=int, default=8080)
    parser.add_argument("--ml-port", type=int, default=8091)
    parser.add_argument("--db", type=Path, default=ROOT / "backend/resources/cqa.db")
    parser.add_argument("--go", default="go", help="Go executable or its full path")
    args = parser.parse_args()
    if args.api_port == args.ml_port or not all(1 <= port <= 65535 for port in (args.api_port, args.ml_port)):
        parser.error("Choose two different ports between 1 and 65535.")

    addr = f"127.0.0.1:{args.api_port}"
    url = f"http://{addr}"
    db = args.db.resolve()
    try:
        for port in (args.api_port, args.ml_port):
            with socket.socket() as probe:
                if os.name == "nt":
                    probe.setsockopt(socket.SOL_SOCKET, socket.SO_EXCLUSIVEADDRUSE, 1)
                try:
                    probe.bind(("127.0.0.1", port))
                except OSError as exc:
                    raise RuntimeError(f"Port {port} is unavailable.") from exc

        with TemporaryDirectory(prefix="cqa-") as build_dir, ExitStack() as stack:
            binary = Path(build_dir) / ("cqa-api.exe" if os.name == "nt" else "cqa-api")
            subprocess.run([args.go, "build", "-o", str(binary), "./cmd/api"], cwd=ROOT / "backend", check=True)
            db.parent.mkdir(parents=True, exist_ok=True)
            bridge = subprocess.Popen([sys.executable, "-X", "utf8", "-m", "ml.service",
                                       "--port", str(args.ml_port)], cwd=ROOT)
            stack.callback(stop, bridge)
            api = subprocess.Popen([str(binary), "-addr", addr, "-db", str(db),
                                    "-static", str(ROOT / "frontend"), "-workers", "1",
                                    "-ml-url", f"http://127.0.0.1:{args.ml_port}/evaluate"], cwd=ROOT / "backend")
            stack.callback(stop, api)
            processes = {"ML bridge": bridge, "Go API": api}
            wait_for_api(url, processes)
            print(f"CQA ready at {url}/ with {MODEL}. Ctrl+C to stop.", flush=True)
            while all(process.poll() is None for process in processes.values()):
                time.sleep(0.5)
            raise RuntimeError("A CQA service stopped. See its log above.")
    except KeyboardInterrupt:
        return 0
    except (OSError, RuntimeError, subprocess.CalledProcessError) as exc:
        print(f"CQA startup error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
