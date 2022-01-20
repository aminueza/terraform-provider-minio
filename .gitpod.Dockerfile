FROM hashicorp/terraform:0.14.0 as terraform
FROM minio/minio:RELEASE.2021-04-06T23-11-00Z as minio
FROM minio/mc:RELEASE.2021-04-22T17-40-00Z as mc
FROM rzrbld/adminio-api:v1.82 as adminio-api
FROM rzrbld/adminio-ui:v1.93 as adminio-ui
FROM gitpod/workspace-full

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
RUN sudo chmod -R 777 ${ADMINIO_UI_PATH}

# Install Task
RUN sudo sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin
