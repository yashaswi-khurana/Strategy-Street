import json
import logging
import os
import signal
import sys
import time
from typing import Any

logging.basicConfig(
    level=os.getenv("LOG_LEVEL", "INFO").upper(),
    format="%(message)s",
)


def log_json(level: str, message: str, **kwargs: Any) -> None:
    payload = {
        "level": level,
        "message": message,
        "timestamp": time.time(),
        "service": "engine",
        **kwargs,
    }
    sys.stdout.write(json.dumps(payload) + "\n")
    sys.stdout.flush()


def run() -> None:
    running = True

    def _sig_handler(signum: int, frame: Any) -> None:
        nonlocal running
        log_json(
            "info",
            "Received termination signal, shutting down engine worker",
            signal=signum,
        )
        running = False

    signal.signal(signal.SIGINT, _sig_handler)
    signal.signal(signal.SIGTERM, _sig_handler)

    log_json("info", "Starting Strategy Street Python Engine worker stub")

    while running:
        time.sleep(1)

    log_json("info", "Engine worker shut down cleanly")


if __name__ == "__main__":
    run()
