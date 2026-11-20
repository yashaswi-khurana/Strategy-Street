"""Market Session Filter & Missing-Bar Policy for Strategy Street.

1:1 mirror of Go package internal/marketcal.
Provides authoritative session checks, trading days, deterministic bar timing,
and read-time missing bar fill policy.
"""

from __future__ import annotations

import datetime
from collections.abc import Sequence
from dataclasses import dataclass
from enum import Enum
from zoneinfo import ZoneInfo

# Standard Indian Standard Time (UTC+05:30)
LOCATION_IST = ZoneInfo("Asia/Kolkata")


class FillPolicy(str, Enum):
    """Missing bar fill policy."""

    CARRY_FORWARD = "CarryForward"
    SKIP = "Skip"

    # CamelCase aliases matching Go enum names
    CarryForward = "CarryForward"
    Skip = "Skip"


@dataclass(frozen=True)
class SpecialSession:
    """Non-standard market session for a specific date (e.g. Muhurat trading)."""

    date: datetime.date
    open_time: str  # "HH:MM:SS"
    close_time: str  # "HH:MM:SS"
    label: str = ""


@dataclass
class Candle:
    """OHLCV Bar representation in the Python strategy engine."""

    stock_id: int
    bucket_time: datetime.datetime
    open: float
    high: float
    low: float
    close: float
    volume: int = 0
    oi: int = 0
    is_adjusted: bool = False
    is_filled: bool = False


DEFAULT_HOLIDAYS_2026_2027: dict[str, str] = {
    # 2026
    "2026-01-26": "Republic Day",
    "2026-03-03": "Holi",
    "2026-03-20": "Id-Ul-Fitr (Ramzan Id)",
    "2026-03-27": "Shri Ram Navami",
    "2026-04-03": "Good Friday",
    "2026-04-14": "Dr. Baba Saheb Ambedkar Jayanti",
    "2026-05-01": "Maharashtra Day",
    "2026-05-27": "Bakri Id / Eid ul-Adha",
    "2026-06-26": "Muharram",
    "2026-08-15": "Independence Day",
    "2026-09-04": "Milad-un-Nabi (Id-e-Milad)",
    "2026-10-02": "Mahatma Gandhi Jayanti",
    "2026-10-20": "Dussehra",
    "2026-11-08": "Diwali Laxmi Pujan",
    "2026-11-10": "Diwali Balipratipada",
    "2026-11-24": "Gurunanak Jayanti",
    "2026-12-25": "Christmas",
    # 2027
    "2027-01-26": "Republic Day",
    "2027-03-22": "Holi",
    "2027-03-26": "Good Friday",
    "2027-04-14": "Dr. Baba Saheb Ambedkar Jayanti",
    "2027-05-01": "Maharashtra Day",
    "2027-08-15": "Independence Day",
    "2027-10-02": "Mahatma Gandhi Jayanti",
    "2027-10-09": "Dussehra",
    "2027-10-28": "Diwali Laxmi Pujan",
    "2027-10-29": "Diwali Balipratipada",
    "2027-11-14": "Gurunanak Jayanti",
    "2027-12-25": "Christmas",
}

DEFAULT_SPECIAL_SESSIONS: dict[str, SpecialSession] = {
    "2026-11-08": SpecialSession(
        date=datetime.date(2026, 11, 8),
        open_time="18:15:00",
        close_time="19:15:00",
        label="muhurat",
    ),
    "2027-10-28": SpecialSession(
        date=datetime.date(2027, 10, 28),
        open_time="18:15:00",
        close_time="19:15:00",
        label="muhurat",
    ),
}

BARS_PER_SESSION_MAP: dict[str, int] = {
    "1s": 22500,
    "1m": 375,
    "5m": 75,
    "10m": 38,
    "15m": 25,
    "30m": 13,
    "1h": 7,
    "3h": 3,
    "1d": 1,
}

TIMEFRAME_SECONDS: dict[str, int] = {
    "1s": 1,
    "1m": 60,
    "5m": 300,
    "10m": 600,
    "15m": 900,
    "30m": 1800,
    "1h": 3600,
    "3h": 10800,
    "1d": 86400,
}


def _ensure_ist(t: datetime.datetime | datetime.date) -> datetime.datetime:
    """Ensure timestamp is a datetime in Asia/Kolkata timezone."""
    if isinstance(t, datetime.date) and not isinstance(t, datetime.datetime):
        return datetime.datetime(t.year, t.month, t.day, 0, 0, 0, tzinfo=LOCATION_IST)
    if t.tzinfo is None:
        return t.replace(tzinfo=LOCATION_IST)
    return t.astimezone(LOCATION_IST)


class Calendar:
    """Market calendar providing session boundaries and trading day calculations."""

    def __init__(
        self,
        holidays: dict[str, str] | None = None,
        special_sessions: dict[str, SpecialSession] | None = None,
    ) -> None:
        self.holidays = (
            dict(holidays) if holidays is not None else dict(DEFAULT_HOLIDAYS_2026_2027)
        )
        self.special_sessions = (
            dict(special_sessions)
            if special_sessions is not None
            else dict(DEFAULT_SPECIAL_SESSIONS)
        )
        self.loc = LOCATION_IST

    def session(
        self, day: datetime.datetime | datetime.date
    ) -> tuple[datetime.datetime, datetime.datetime, bool]:
        """Return (open, close, ok) for a calendar day in Asia/Kolkata.

        Returns ok=False on holidays and weekends without a special session.
        """
        ist_day = _ensure_ist(day)
        date_key = ist_day.strftime("%Y-%m-%d")
        year, month, d = ist_day.year, ist_day.month, ist_day.day

        # 1. Special session override (e.g. Muhurat or special Saturday session)
        if date_key in self.special_sessions:
            spec = self.special_sessions[date_key]
            oh, om, os = (int(x) for x in spec.open_time.split(":"))
            ch, cm, cs = (int(x) for x in spec.close_time.split(":"))
            open_t = datetime.datetime(year, month, d, oh, om, os, tzinfo=self.loc)
            close_t = datetime.datetime(year, month, d, ch, cm, cs, tzinfo=self.loc)
            return open_t, close_t, True

        # 2. Exchange holiday
        if date_key in self.holidays:
            zero = datetime.datetime.min.replace(tzinfo=self.loc)
            return zero, zero, False

        # 3. Weekend (Saturday = 5, Sunday = 6 in Python)
        if ist_day.weekday() in (5, 6):
            zero = datetime.datetime.min.replace(tzinfo=self.loc)
            return zero, zero, False

        # 4. Regular session: [09:15:00, 15:30:00) IST
        open_t = datetime.datetime(year, month, d, 9, 15, 0, tzinfo=self.loc)
        close_t = datetime.datetime(year, month, d, 15, 30, 0, tzinfo=self.loc)
        return open_t, close_t, True

    def is_open(self, t: datetime.datetime) -> bool:
        """Return True if timestamp t falls inside [open, close) session boundaries."""
        open_t, close_t, ok = self.session(t)
        if not ok:
            return False
        ist_t = _ensure_ist(t)
        return open_t <= ist_t < close_t

    def trading_days(
        self,
        from_date: datetime.datetime | datetime.date,
        to_date: datetime.datetime | datetime.date,
    ) -> list[datetime.datetime]:
        """Return sorted list of active trading days (midnight IST) in range."""
        start = _ensure_ist(from_date).date()
        end = _ensure_ist(to_date).date()
        if start > end:
            return []

        days: list[datetime.datetime] = []
        cur = start
        one_day = datetime.timedelta(days=1)
        while cur <= end:
            midnight = datetime.datetime(
                cur.year, cur.month, cur.day, 0, 0, 0, tzinfo=self.loc
            )
            _, _, ok = self.session(midnight)
            if ok:
                days.append(midnight)
            cur += one_day
        return days

    def bars_per_session(self, tf: str) -> int:
        """Return expected bar count for a regular session."""
        norm_tf = tf.lower()
        if norm_tf not in BARS_PER_SESSION_MAP:
            msg = f"unsupported timeframe: {tf}"
            raise ValueError(msg)
        return BARS_PER_SESSION_MAP[norm_tf]

    def next_bar_close(
        self, now: datetime.datetime, tf: str
    ) -> tuple[datetime.datetime | None, bool]:
        """Calculate the deterministic bar-close instant for now and timeframe tf.

        Returns (None, False) outside a valid session.
        Pure function with respect to wall-clock seconds.
        """
        open_t, close_t, ok = self.session(now)
        if not ok:
            return None, False

        ist_now = _ensure_ist(now)
        if not (open_t <= ist_now < close_t):
            return None, False

        norm_tf = tf.lower()
        if norm_tf == "1d":
            return close_t, True

        sec = TIMEFRAME_SECONDS.get(norm_tf)
        if sec is None or sec <= 0:
            return close_t, True

        # Origin anchor 2015-01-01 09:15:00 IST
        origin = datetime.datetime(2015, 1, 1, 9, 15, 0, tzinfo=self.loc)
        diff_sec = int((ist_now - origin).total_seconds())
        rem = diff_sec % sec
        bar_start = ist_now - datetime.timedelta(seconds=rem)
        next_close = bar_start + datetime.timedelta(seconds=sec)

        if next_close > close_t:
            next_close = close_t

        return next_close, True


def bars_per_session(tf: str) -> int:
    """Return bars per session for timeframe."""
    return Calendar().bars_per_session(tf)


def missing_bars(
    expected: Sequence[datetime.datetime],
    actual: Sequence[Candle | datetime.datetime],
) -> list[datetime.datetime]:
    """Identify timestamps present in expected but missing from actual."""
    actual_set: set[int] = set()
    for item in actual:
        t = item.bucket_time if isinstance(item, Candle) else item
        actual_set.add(int(t.astimezone(datetime.timezone.utc).timestamp()))

    missing: list[datetime.datetime] = []
    for exp in expected:
        exp_ts = int(exp.astimezone(datetime.timezone.utc).timestamp())
        if exp_ts not in actual_set:
            missing.append(exp)
    return missing


def filled_range(
    bars: Sequence[Candle],
    cal: Calendar | None = None,
    tf: str = "1m",
    policy: FillPolicy = FillPolicy.CarryForward,
) -> list[Candle]:
    """Decorate candle series by filling missing bars under policy.

    Under CarryForward, missing bars repeat previous close with volume=0
    and is_filled=True. Under Skip, returns copy of original bars.
    Filled bars are read-time and must never be written to storage.
    """
    if cal is None:
        cal = Calendar()

    if len(bars) <= 1 or policy in (FillPolicy.SKIP, FillPolicy.Skip):
        return list(bars)

    norm_tf = tf.lower()

    if norm_tf == "1d":
        result: list[Candle] = [bars[0]]
        for i in range(len(bars) - 1):
            prev = bars[i]
            curr = bars[i + 1]

            prev_date = _ensure_ist(prev.bucket_time).date()
            curr_date = _ensure_ist(curr.bucket_time).date()

            intermediate_days = cal.trading_days(
                prev_date + datetime.timedelta(days=1),
                curr_date - datetime.timedelta(days=1),
            )
            for day in intermediate_days:
                bt = datetime.datetime(
                    day.year, day.month, day.day, 9, 15, 0, tzinfo=cal.loc
                )
                filled = Candle(
                    stock_id=prev.stock_id,
                    bucket_time=bt.astimezone(datetime.timezone.utc),
                    open=prev.close,
                    high=prev.close,
                    low=prev.close,
                    close=prev.close,
                    volume=0,
                    oi=prev.oi,
                    is_adjusted=prev.is_adjusted,
                    is_filled=True,
                )
                result.append(filled)
            result.append(curr)
        return result

    step_sec = TIMEFRAME_SECONDS.get(norm_tf)
    if step_sec is None or step_sec <= 0:
        return list(bars)

    step = datetime.timedelta(seconds=step_sec)
    result = [bars[0]]

    for i in range(len(bars) - 1):
        prev = bars[i]
        curr = bars[i + 1]

        cur_t = prev.bucket_time + step
        while cur_t < curr.bucket_time:
            if cal.is_open(cur_t):
                filled = Candle(
                    stock_id=prev.stock_id,
                    bucket_time=cur_t.astimezone(datetime.timezone.utc),
                    open=prev.close,
                    high=prev.close,
                    low=prev.close,
                    close=prev.close,
                    volume=0,
                    oi=prev.oi,
                    is_adjusted=prev.is_adjusted,
                    is_filled=True,
                )
                result.append(filled)
            cur_t += step
        result.append(curr)

    return result
