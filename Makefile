VERSION=1.0

GO := ${GOROOT}/bin/go
ifeq ($(GOROOT),)
GO = go
endif

default: build

build: build_cleanup build_mac build_linux

build_cleanup:
	rm -f ./terraform-provider-minio_*

build_mac:
	@echo "Building terraform-provider-minio_v${VERSION}_darwin_amd64..."
	env GOOS=darwin GOARCH=amd64 $(GO) build -o terraform-provider-minio_v${VERSION}_darwin_amd64 .
	@echo "Done!"

build_linux:
	@echo "Building terraform-provider-minio_v${VERSION}_linux_amd64..."
	env GOOS=linux GOARCH=amd64 $(GO) build -o terraform-provider-minio_v${VERSION}_linux_amd64 .
	@echo "Done!"

install_mac: build_cleanup build_mac
	mkdir -p ~/.terraform.d/plugins/darwin_amd64
	@echo "Moving terraform-provider-minio_v${VERSION}_darwin_amd64 into ~/.terraform.d/plugins/darwin_amd64..."
	mv terraform-provider-minio_v${VERSION}_darwin_amd64 ~/.terraform.d/plugins/darwin_amd64/terraform-provider-minio_v${VERSION}
	@echo "Done!"

install_linux: build_cleanup build_linux
	mkdir -p ~/.terraform.d/plugins/linux_amd64
	@echo "Moving terraform-provider-minio_v${VERSION}_linux_amd64 into ~/.terraform.d/plugins/linux_amd64..."
	mv terraform-provider-minio_v${VERSION}_linux_amd64 ~/.terraform.d/plugins/linux_amd64/terraform-provider-minio_v${VERSION}
	@echo "Done!"

test:
	TF_ACC=0 MINIO_ENDPOINT=localhost:9000 \
	MINIO_ACCESS_KEY=minio MINIO_SECRET_KEY=minio123 \
	MINIO_ENABLE_HTTPS=false $(GO) test -count=1 -v -cover ./...

.PHONY: default build build_cleanup build_mac build_linux install_mac install_linux test