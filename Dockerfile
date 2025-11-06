FROM golang:1.25-alpine@sha256:8b6b77a5e6a9dda591e864e1a2856d436d94219befa5f54d7ce76d2a77cc7a06 AS builder

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

FROM python:3.10-alpine@sha256:63f6d26c66126481336273c8d34a2017729843ac3b2de2897c428f27e066f1be

WORKDIR /app

# Copy the built binary
COPY --from=builder /build/sigma-deployer /usr/local/bin/sigma-deployer

# Copy the convert action
COPY ./actions/convert ./actions/convert

WORKDIR /app/actions/convert
RUN apk add --no-cache bash=5.2.37-r0 && \
    python -m pip install --no-cache-dir --upgrade pip==25.0.1 && \
    pip install --no-cache-dir uv==0.6.13

WORKDIR /app

# Copy entrypoint script
COPY ./entrypoint.sh ./entrypoint.sh

RUN chmod +x ./entrypoint.sh

ENTRYPOINT ["/bin/bash", "/app/entrypoint.sh"]

