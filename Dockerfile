FROM golang:1.27-alpine@sha256:eb61c3873c97416fff0116553fff65d67a3549f72e11d9758849a689cdd8b523 AS builder

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

FROM python:3.14-alpine@sha256:a18b20ad69e74df6cf6d2e1b1a260d7a1ecf5380fd83b310f83ba88c461cf894 AS pybuilder

# google-re2 (a pysigma 2.x dependency) has no musl wheels, so build the venv here where the compiler is available
RUN apk add --no-cache g++~=15.2 re2-dev~=2025.11 abseil-cpp-dev~=20250814 py3-pybind11-dev~=3.0 && \
    pip install --no-cache-dir uv~=0.9.0

WORKDIR /app/actions/convert
COPY ./actions/convert/pyproject.toml ./actions/convert/uv.lock ./
RUN uv sync --frozen --no-dev --no-install-project

FROM python:3.14-alpine@sha256:a18b20ad69e74df6cf6d2e1b1a260d7a1ecf5380fd83b310f83ba88c461cf894

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

