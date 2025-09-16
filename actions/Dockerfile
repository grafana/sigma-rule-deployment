FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY ./deploy ./deploy
COPY ./integrate ./integrate

WORKDIR /src/deploy
RUN go build -ldflags="-s -w" -o /build/deploy .
WORKDIR /src/integrate
RUN go build -ldflags="-s -w" -o /build/integrate .

FROM python:3.10-alpine

WORKDIR /app

COPY --from=builder /build/deploy /usr/local/bin/deploy
COPY --from=builder /build/integrate /usr/local/bin/integrate

COPY ./convert ./convert

WORKDIR /app/convert
# TODO: remove git once we released a new version of sigma-rule-deployer
RUN apk add --no-cache bash=5.2.37-r0 git=2.49.1-r0 && \
    python -m pip install --no-cache-dir --upgrade pip==25.0.1 && \
    pip install --no-cache-dir uv==0.6.13

WORKDIR /app

COPY ./entrypoint.sh ./entrypoint.sh

RUN chmod +x ./entrypoint.sh

ENTRYPOINT ["/bin/bash", "/app/entrypoint.sh"]
