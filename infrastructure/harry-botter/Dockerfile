FROM python:3.13-alpine AS builder

RUN pip install --no-cache-dir --prefix=/opt/deps kubernetes PyJWT[crypto] requests

FROM python:3.13-alpine

COPY --from=builder /opt/deps /usr/local

RUN adduser -D -u 1000 appuser

USER appuser

WORKDIR /app

CMD ["python", "/script/harry-botter.py"]