ARG GO_VERSION="1.15"

FROM hashicorp/terraform:0.14.0 as terraform
FROM minio/minio:RELEASE.2020-11-25T22-36-25Z as minio
FROM minio/mc:RELEASE.2020-11-25T23-04-07Z-amd64 as mc
FROM rzrbld/adminio-api:release-1.6 as adminio-api
FROM rzrbld/adminio-ui:release-1.7 as adminio-ui
FROM mcr.microsoft.com/vscode/devcontainers/go:0-${GO_VERSION}

# Copy and install Terraform binary
COPY --from=terraform /bin/terraform /usr/local/bin/

# Define environment variables for MinIO
ENV MINIO_ACCESS_KEY=minio
ENV MINIO_SECRET_KEY=minio123
ENV MINIO_HTTP_TRACE=/dev/stdout
ENV MINIO_VOLUMES=/tmp/minio

# Copy and install MinIO binary
COPY --from=minio /usr/bin/minio /usr/local/bin/

# Copy and install MinIO Client (MC) binary
COPY --from=mc /usr/bin/mc /usr/local/bin/

# Define environment variables for AdminIO
ENV MINIO_HOST_PORT=localhost:9000
ENV MINIO_ACCESS=${MINIO_ACCESS_KEY}
ENV MINIO_SECRET=${MINIO_SECRET_KEY}

# Copy and install AdminIO binary
COPY --from=adminio-api /usr/bin/adminio /usr/local/bin/

# Define environment variables for AdminIO-UI
ENV ADMINIO_UI_PORT=1234
ENV ADMINIO_UI_PATH=/usr/local/share/adminio-ui

# Copy and install pre-built AdminIO-UI
COPY --from=adminio-ui /usr/share/nginx/html ${ADMINIO_UI_PATH}
RUN chmod -R 777 ${ADMINIO_UI_PATH}

# Install Node.js
ARG INSTALL_NODE="true"
ARG NODE_VERSION="lts/*"
RUN if [ "${INSTALL_NODE}" = "true" ]; then su vscode -c "source /usr/local/share/nvm/nvm.sh && nvm install ${NODE_VERSION} 2>&1"; fi
