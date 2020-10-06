VERSION=1.0
TARGET=~/.terraform.d/plugins/registry.terraform.io/hashicorp/minio/1.0/darwin_amd64/

# helper to run go from project root
GO := ${GOROOT}/bin/go

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
test:
	TF_ACC=0 MINIO_ENDPOINT=localhost:9000 \
	MINIO_ACCESS_KEY=minio MINIO_SECRET_KEY=minio123 \
	MINIO_ENABLE_HTTPS=false $(GO) test -count=1 -v -cover ./...

.PHONY: default build build_cleanup test

install: build
	mkdir -p ${TARGET}
	cp -r terraform-provider-minio_v1.0_darwin_amd64 ${TARGET}/terraform-provider-minio
