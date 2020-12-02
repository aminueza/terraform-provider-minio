# [Choice] Go version: 1, 1.15, 1.14
ARG VARIANT="1"

FROM hashicorp/terraform as terraform
FROM minio/minio as minio
FROM mcr.microsoft.com/vscode/devcontainers/go:0-${VARIANT}

# Copy and install Terraform binary
COPY --from=terraform /bin/terraform /usr/local/bin/

# Define environment variables for MinIO
ENV MINIO_ACCESS_KEY=minio
ENV MINIO_SECRET_KEY=minio123
ENV MINIO_HTTP_TRACE=/dev/stdout
ENV MINIO_VOLUMES=/tmp/minio

# Copy and install MinIO binary
COPY --from=minio /usr/bin/minio /usr/local/bin/

# [Option] Install Node.js
ARG INSTALL_NODE="true"
ARG NODE_VERSION="lts/*"
RUN if [ "${INSTALL_NODE}" = "true" ]; then su vscode -c "source /usr/local/share/nvm/nvm.sh && nvm install ${NODE_VERSION} 2>&1"; fi
