FROM golang:1.25-alpine@sha256:aee43c3ccbf24fdffb7295693b6e33b21e01baec1b2a55acc351fde345e9ec34 AS builder

WORKDIR /src

COPY ./deploy ./deploy
COPY ./integrate ./integrate

WORKDIR /src/deploy
RUN go build -ldflags="-s -w" -o /build/deploy .
WORKDIR /src/integrate
RUN go build -ldflags="-s -w" -o /build/integrate .

FROM python:3.10-alpine@sha256:63f6d26c66126481336273c8d34a2017729843ac3b2de2897c428f27e066f1be

WORKDIR /app

COPY --from=builder /build/deploy /usr/local/bin/deploy
COPY --from=builder /build/integrate /usr/local/bin/integrate

COPY ./convert ./convert

WORKDIR /app/convert
RUN apk add --no-cache bash=5.2.37-r0 && \
    python -m pip install --no-cache-dir --upgrade pip==25.0.1 && \
    pip install --no-cache-dir uv==0.6.13

WORKDIR /app

COPY ./entrypoint.sh ./entrypoint.sh

RUN chmod +x ./entrypoint.sh

ENTRYPOINT ["/bin/bash", "/app/entrypoint.sh"]
