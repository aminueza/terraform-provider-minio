FROM minio/minio:RELEASE.2020-01-25T02-50-51Z as minio
FROM hashicorp/terraform:0.12.20 as terraform
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
ENV MINIO_VOLUMES=$HOME/.minio/data

# Copy and install MinIO binary
COPY --from=minio /usr/bin/minio /usr/local/bin/

# Create folders for MinIO and Terraform
RUN  mkdir -p ${MINIO_VOLUMES} ${TERRAFORM_PLUGINS_DIR}
