from engine import __version__
from engine.main import log_json


def test_engine_version() -> None:
    assert __version__ == "0.1.0"


def test_smoke_log_json(capsys) -> None:  # type: ignore[no-untyped-def]
    log_json("info", "test message", key="val")
    captured = capsys.readouterr()
    assert "test message" in captured.out
    assert "info" in captured.out
