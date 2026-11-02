FROM golang:1.27-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS builder

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

FROM python:3.14-alpine@sha256:eb2c62658b8d51bff0acc408504b76521d02030de56a8a3c1e830083e89b89d2 AS pybuilder

# google-re2 (a pysigma 2.x dependency) has no musl wheels, so build the venv here where the compiler is available
RUN apk add --no-cache g++~=15.2 re2-dev~=2025.11 abseil-cpp-dev~=20250814 py3-pybind11-dev~=3.0 && \
    pip install --no-cache-dir uv~=0.9.0

WORKDIR /app/actions/convert
COPY ./actions/convert/pyproject.toml ./actions/convert/uv.lock ./
RUN uv sync --frozen --no-dev --no-install-project

FROM python:3.14-alpine@sha256:eb2c62658b8d51bff0acc408504b76521d02030de56a8a3c1e830083e89b89d2

WORKDIR /app

# Copy the built binary
COPY --from=builder /build/sigma-deployer /usr/local/bin/sigma-deployer

# Copy the convert action
COPY ./actions/convert ./actions/convert

WORKDIR /app/actions/convert
# Prebuilt venv without dev dependencies; UV_NO_DEV stops `uv run` from installing them at startup
ENV UV_NO_DEV=1
# re2 is the runtime library for google-re2
COPY --from=pybuilder /app/actions/convert/.venv ./.venv
RUN apk add --no-cache bash~=5.3 re2~=2025.11 && \
    python -m pip install --no-cache-dir --upgrade pip~=25.3.0 && \
    pip install --no-cache-dir uv~=0.9.0

WORKDIR /app

# Copy entrypoint script
COPY ./entrypoint.sh ./entrypoint.sh

RUN chmod +x ./entrypoint.sh

ENTRYPOINT ["/bin/bash", "/app/entrypoint.sh"]

