FROM golang:1.25-alpine@sha256:98e6cffc31ccc44c7c15d83df1d69891efee8115a5bb7ede2bf30a38af3e3c92 AS builder

WORKDIR /src

# Copy the unified Go module structure
COPY go.mod go.sum ./
# Download dependencies first for better layer caching
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY shared/ ./shared/

# Build the unified sigma-deployer binary
RUN go build -ldflags="-s -w" -o /build/sigma-deployer ./cmd/sigma-deployer

FROM python:3.12-alpine@sha256:68d81cd281ee785f48cdadecb6130d05ec6957f1249814570dc90e5100d3b146

WORKDIR /app

# Copy the built binary
COPY --from=builder /build/sigma-deployer /usr/local/bin/sigma-deployer

# Copy the convert action
COPY ./actions/convert ./actions/convert

WORKDIR /app/actions/convert
RUN apk add --no-cache bash~=5.3 && \
    python -m pip install --no-cache-dir --upgrade pip~=25.3.0 && \
    pip install --no-cache-dir uv~=0.9.0

WORKDIR /app

# Copy entrypoint script
COPY ./entrypoint.sh ./entrypoint.sh

RUN chmod +x ./entrypoint.sh

ENTRYPOINT ["/bin/bash", "/app/entrypoint.sh"]

