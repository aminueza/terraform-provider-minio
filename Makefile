VERSION=1.0

# helper to run go from project root
GO := go

# run build command
define building_provider
	echo "Building terraform-provider-minio_${VERSION}_linux_amd64..."
	env GOOS=linux GOARCH=amd64 $(GO) build -o terraform-provider-minio_v${VERSION}_linux_amd64 .
	echo "Building terraform-provider-s3minio_${VERSION}_darwin_amd64..."
	env GOOS=darwin GOARCH=amd64 $(GO) build -o terraform-provider-minio_v${VERSION}_darwin_amd64 .
endef

default: build

build: build_cleanup
	$(call building_provider,build)

build_cleanup:
	rm -f ./terraform-provider-s3minio_*

.PHONY: default build build_cleanup