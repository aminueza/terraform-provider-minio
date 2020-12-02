FROM hashicorp/terraform:0.13.5 as terraform
FROM minio/minio:RELEASE.2020-11-25T22-36-25Z as minio
FROM gitpod/workspace-full

USER gitpod

# Define environment variables for Terraform
ENV TERRAFORM_PLUGINS_DIR=$HOME/.terraform.d/plugins

# Copy and install Terraform binary
COPY --from=terraform /bin/terraform /usr/local/bin/

# Define environment variables for MinIO
ENV MINIO_ACCESS_KEY=minio
ENV MINIO_SECRET_KEY=minio123
ENV MINIO_HTTP_TRACE=/dev/stdout
ENV MINIO_VOLUMES=/tmp/minio

# Copy and install MinIO binary
COPY --from=minio /usr/bin/minio /usr/local/bin/

# Create folders for MinIO and Terraform
RUN  mkdir -p ${MINIO_VOLUMES} ${TERRAFORM_PLUGINS_DIR}
