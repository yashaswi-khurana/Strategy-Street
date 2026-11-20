"""Unit and cross-language fixture tests for engine.marketcal."""

import datetime
import json
from pathlib import Path

import pytest

from engine.marketcal import (
    LOCATION_IST,
    Calendar,
    Candle,
    FillPolicy,
    SpecialSession,
    bars_per_session,
    filled_range,
    missing_bars,
)


@pytest.fixture
def test_calendar() -> Calendar:
    """Calendar matching Go test fixture calendar."""
    holidays = {
        "2026-01-26": "Republic Day",
        "2026-03-03": "Holi",
        "2026-03-20": "Id-Ul-Fitr (Ramzan Id)",
        "2026-03-27": "Shri Ram Navami",
    }
    special_sessions = {
        "2026-03-14": SpecialSession(
            date=datetime.date(2026, 3, 14),
            open_time="09:15:00",
            close_time="15:30:00",
            label="special_session",
        ),
        "2026-11-08": SpecialSession(
            date=datetime.date(2026, 11, 8),
            open_time="18:15:00",
            close_time="19:15:00",
            label="muhurat",
        ),
    }
    return Calendar(holidays=holidays, special_sessions=special_sessions)


def test_acceptance_criterion_1_is_open_boundaries(test_calendar: Calendar) -> None:
    """Acceptance Criterion 1: IsOpen boundary conditions."""
    t_before = datetime.datetime.fromisoformat("2026-03-14T09:14:59+05:30")
    t_open = datetime.datetime.fromisoformat("2026-03-14T09:15:00+05:30")
    t_before_close = datetime.datetime.fromisoformat("2026-03-14T15:29:59+05:30")
    t_close = datetime.datetime.fromisoformat("2026-03-14T15:30:00+05:30")

    assert test_calendar.is_open(t_before) is False
    assert test_calendar.is_open(t_open) is True
    assert test_calendar.is_open(t_before_close) is True
    assert test_calendar.is_open(t_close) is False


def test_acceptance_criterion_2_holiday_and_weekend(test_calendar: Calendar) -> None:
    """Acceptance Criterion 2: Session() returns ok=false on holiday/weekend."""
    cal = Calendar()

    # Holiday (Republic Day)
    republic_day = datetime.datetime(2026, 1, 26, 10, 0, tzinfo=LOCATION_IST)
    _, _, ok_hol = cal.session(republic_day)
    assert ok_hol is False

    # Weekend (Sunday 2026-03-15, Saturday 2026-03-21)
    sunday = datetime.datetime(2026, 3, 15, 11, 0, tzinfo=LOCATION_IST)
    _, _, ok_sun = cal.session(sunday)
    assert ok_sun is False

    saturday = datetime.datetime(2026, 3, 21, 11, 0, tzinfo=LOCATION_IST)
    _, _, ok_sat = cal.session(saturday)
    assert ok_sat is False

    # Regular trading day (Friday 2026-03-13)
    friday = datetime.datetime(2026, 3, 13, 0, 0, tzinfo=LOCATION_IST)
    open_t, close_t, ok_fri = cal.session(friday)
    assert ok_fri is True
    assert open_t.hour == 9 and open_t.minute == 15
    assert close_t.hour == 15 and close_t.minute == 30


def test_acceptance_criterion_3_carry_forward_decorator(
    test_calendar: Calendar,
) -> None:
    """Acceptance Criterion 3: 1m missing 11:00-11:04 filled with 10:59 close."""
    t1059 = datetime.datetime.fromisoformat("2026-03-13T10:59:00+05:30")
    t1105 = datetime.datetime.fromisoformat("2026-03-13T11:05:00+05:30")

    bar1059 = Candle(
        stock_id=42,
        bucket_time=t1059,
        open=2450.0,
        high=2455.0,
        low=2448.0,
        close=2452.5,
        volume=1200,
        oi=50000,
    )
    bar1105 = Candle(
        stock_id=42,
        bucket_time=t1105,
        open=2453.0,
        high=2458.0,
        low=2451.0,
        close=2456.0,
        volume=1500,
        oi=50500,
    )

    series = [bar1059, bar1105]
    result = filled_range(
        series, cal=test_calendar, tf="1m", policy=FillPolicy.CarryForward
    )

    assert len(result) == 7  # 10:59 + 5 filled + 11:05

    # Check original bars untouched
    assert result[0].bucket_time == t1059
    assert result[0].is_filled is False
    assert result[0].volume == 1200

    assert result[6].bucket_time == t1105
    assert result[6].is_filled is False
    assert result[6].volume == 1500

    # Check 5 filled bars (11:00, 11:01, 11:02, 11:03, 11:04)
    for i in range(5):
        filled_bar = result[1 + i]
        expected_t = datetime.datetime.fromisoformat(f"2026-03-13T11:0{i}:00+05:30")
        assert filled_bar.bucket_time == expected_t.astimezone(datetime.timezone.utc)
        assert filled_bar.is_filled is True
        assert filled_bar.volume == 0
        assert filled_bar.open == 2452.5
        assert filled_bar.high == 2452.5
        assert filled_bar.low == 2452.5
        assert filled_bar.close == 2452.5

    # Skip policy test
    skipped = filled_range(series, cal=test_calendar, tf="1m", policy=FillPolicy.Skip)
    assert len(skipped) == 2


def test_acceptance_criterion_4_bars_and_trading_days() -> None:
    """Acceptance Criterion 4: BarsPerSession and Q1 2026 trading days."""
    cal = Calendar()

    assert bars_per_session("1m") == 375
    assert bars_per_session("1s") == 22500
    assert bars_per_session("1d") == 1
    assert bars_per_session("5m") == 75

    start = datetime.date(2026, 1, 1)
    end = datetime.date(2026, 3, 31)
    trading_days = cal.trading_days(start, end)
    assert len(trading_days) > 0

    td_strings = {d.strftime("%Y-%m-%d") for d in trading_days}

    # Seeded holidays
    seeded = ["2026-01-26", "2026-03-03", "2026-03-20", "2026-03-27"]
    for hol in seeded:
        assert hol not in td_strings, f"holiday {hol} must be excluded"

    # No weekends
    for d in trading_days:
        assert d.weekday() not in (5, 6), f"{d} must not be a weekend"


def test_cross_language_fixtures_match_go_output() -> None:
    """Acceptance Criterion 5: Fixtures in sessions.json match 100%."""
    fixture_path = Path(__file__).parent.parent / "testdata" / "sessions.json"
    if not fixture_path.exists():
        fixture_path = Path(
            "/home/yashaswi/GitHub/Strategy-Street/engine/testdata/sessions.json"
        )

    assert fixture_path.exists(), f"Fixture file not found: {fixture_path}"

    with open(fixture_path, encoding="utf-8") as f:
        data = json.load(f)

    cal = Calendar(
        holidays={
            "2026-01-26": "Republic Day",
            "2026-03-03": "Holi",
            "2026-03-20": "Id-Ul-Fitr (Ramzan Id)",
            "2026-03-27": "Shri Ram Navami",
        },
        special_sessions={
            "2026-03-14": SpecialSession(
                date=datetime.date(2026, 3, 14),
                open_time="09:15:00",
                close_time="15:30:00",
                label="special_session",
            ),
            "2026-11-08": SpecialSession(
                date=datetime.date(2026, 11, 8),
                open_time="18:15:00",
                close_time="19:15:00",
                label="muhurat",
            ),
        },
    )

    # 1. Bars per session
    for tf, expected_count in data["bars_per_session"].items():
        assert cal.bars_per_session(tf) == expected_count

    # 2. Trading days Q1 2026
    default_cal = Calendar()
    py_tds = default_cal.trading_days(
        datetime.date(2026, 1, 1), datetime.date(2026, 3, 31)
    )
    py_td_strings = [d.strftime("%Y-%m-%d") for d in py_tds]
    assert py_td_strings == data["trading_days_q1_2026"]

    # 3. Sessions
    for sf in data["sessions"]:
        d = datetime.date.fromisoformat(sf["date"])
        open_t, close_t, ok = cal.session(d)
        assert ok == sf["ok"]
        if ok:
            assert open_t.isoformat() == sf["open_time"]
            assert close_t.isoformat() == sf["close_time"]

    # 4. IsOpen cases
    for case in data["is_open_cases"]:
        ts = datetime.datetime.fromisoformat(case["timestamp"])
        assert cal.is_open(ts) == case["is_open"], f"Failed on {case['timestamp']}"

    # 5. NextBarClose cases
    for case in data["next_bar_close_cases"]:
        now = datetime.datetime.fromisoformat(case["now"])
        c_inst, ok = cal.next_bar_close(now, case["timeframe"])
        assert ok == case["expected_ok"], f"Failed ok for {case}"
        if ok:
            assert c_inst is not None
            assert c_inst.isoformat() == case["expected_close"]


def test_missing_bars_helper() -> None:
    """Test missing_bars helper identifies dropped timestamps."""
    t1 = datetime.datetime(2026, 3, 13, 10, 0, tzinfo=datetime.timezone.utc)
    t2 = datetime.datetime(2026, 3, 13, 10, 1, tzinfo=datetime.timezone.utc)
    t3 = datetime.datetime(2026, 3, 13, 10, 2, tzinfo=datetime.timezone.utc)

    expected = [t1, t2, t3]
    actual_bars = [
        Candle(stock_id=1, bucket_time=t1, open=10, high=10, low=10, close=10),
        Candle(stock_id=1, bucket_time=t3, open=10, high=10, low=10, close=10),
    ]

    missing = missing_bars(expected, actual_bars)
    assert missing == [t2]
