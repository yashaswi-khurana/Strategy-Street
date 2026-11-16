FROM python:3.12-slim

WORKDIR /app

COPY pyproject.toml README.md ./
COPY engine ./engine
RUN pip install --no-cache-dir .

ENV PYTHONUNBUFFERED=1

CMD ["python", "-m", "engine.main"]
