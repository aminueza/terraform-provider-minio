ARG GO_VERSION="1.15"

FROM hashicorp/terraform:0.13.5 as terraform
FROM minio/minio:RELEASE.2020-11-25T22-36-25Z as minio
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

# Install Node.js
ARG INSTALL_NODE="true"
ARG NODE_VERSION="lts/*"
RUN if [ "${INSTALL_NODE}" = "true" ]; then su vscode -c "source /usr/local/share/nvm/nvm.sh && nvm install ${NODE_VERSION} 2>&1"; fi
