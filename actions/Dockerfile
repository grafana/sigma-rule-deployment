FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY ./deploy ./deploy
COPY ./integrate ./integrate

RUN cd deploy && go build -o /build/deploy .
RUN cd integrate && go build -o /build/integrate .

FROM python:3.10-alpine

WORKDIR /app

COPY --from=builder /build/deploy /usr/local/bin/deploy
COPY --from=builder /build/integrate /usr/local/bin/integrate

COPY ./convert ./convert

WORKDIR /app/convert
RUN apk add --no-cache bash
RUN python -m pip install --upgrade pip
RUN pip install --disable-pip-version-check uv

WORKDIR /app

COPY ./entrypoint.sh ./entrypoint.sh

RUN chmod +x ./entrypoint.sh

ENTRYPOINT ["/bin/bash", "/app/entrypoint.sh"]
