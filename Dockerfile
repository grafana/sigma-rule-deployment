FROM golang:1.27-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder

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

FROM python:3.14-alpine@sha256:9e9fde4d32eedce0b661d9ab91e826b62dddf28e928c230ec55f1866cac66b01

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

