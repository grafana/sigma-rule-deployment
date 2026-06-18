FROM golang:1.26-alpine@sha256:3ad57304ad93bbec8548a0437ad9e06a455660655d9af011d58b993f6f615648 AS builder

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

FROM python:3.14-alpine@sha256:dd4d2bd5b53d9b25a51da13addf2be586beebd5387e289e798e4083d94ca837a

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

