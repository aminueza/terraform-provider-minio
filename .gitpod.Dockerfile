FROM minio/minio:RELEASE.2020-01-25T02-50-51Z as minio
FROM hashicorp/terraform:0.12.20 as terraform
FROM golang:1

# Set data for non-root user creation
ARG USERNAME=gitpod
ARG USER_UID=1000
ARG USER_GID=$USER_UID

# Configure the behavior of Go Modules
ENV GO111MODULE=auto

# Copy and install Terraform binary
COPY --from=terraform /bin/terraform /usr/local/bin/

# Define environment variables for MinIO
ENV MINIO_ACCESS_KEY=minio
ENV MINIO_SECRET_KEY=minio123
ENV MINIO_HTTP_TRACE=/dev/stdout
ENV MINIO_VOLUMES=/data

# Copy and install MinIO binary
COPY --from=minio /usr/bin/minio /usr/local/bin/

# Give all permissions to Go Path
RUN chmod -R 777 $GOPATH \
  #
  # Create a non-root user
  && groupadd --gid $USER_GID $USERNAME \
  && useradd -s /bin/bash --uid $USER_UID --gid $USER_GID -m $USERNAME \
  #
  # Add sudo support
  && apt-get install -y sudo \
  && echo $USERNAME ALL=\(root\) NOPASSWD:ALL > /etc/sudoers.d/$USERNAME \
  && chmod 0440 /etc/sudoers.d/$USERNAME \
  #
  # Create folders for MinIO
  && mkdir -p ${MINIO_VOLUMES} && chmod -R 777 ${MINIO_VOLUMES} \
  #
  # Create folders for Terraform
  && mkdir -p /home/${USERNAME}/.terraform.d/plugins \
  && chown -R ${USERNAME}:${USERNAME} /home/${USERNAME}/.terraform.d