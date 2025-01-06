FROM python:3.10-slim-bookworm

ARG SIGMA_CLI_VERSION=1.0.4
ARG SIGMA_PLUGIN=loki

RUN pip install sigma-cli==${SIGMA_CLI_VERSION}
RUN sigma plugin install ${SIGMA_PLUGIN}

COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
