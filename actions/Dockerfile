FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY ./deploy ./deploy
COPY ./integrate ./integrate

RUN cd deploy && go build -o /build/deploy .
RUN cd integrate && go build -o /build/integrate .

FROM alpine:latest

COPY --from=builder /build/deploy /usr/local/bin/deploy
COPY --from=builder /build/integrate /usr/local/bin/integrate
COPY ./entrypoint.sh /usr/local/bin/entrypoint.sh

RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
