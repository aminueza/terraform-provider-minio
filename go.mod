module github.com/aminueza/terraform-minio-provider

go 1.13

require (
	github.com/aws/aws-sdk-go v1.25.3
	github.com/dustin/go-humanize v1.0.0
	github.com/go-ini/ini v1.51.0 // indirect
	github.com/hashicorp/terraform v0.12.17
	github.com/minio/minio v0.0.0-20191209145531-bf3a97d3aae3
	github.com/minio/minio-go v6.0.14+incompatible
	github.com/minio/minio-go/v6 v6.0.44
	github.com/minio/sha256-simd v0.1.1
	github.com/minio/sio v0.2.0
	github.com/secure-io/sio-go v0.3.0
	github.com/shirou/gopsutil v2.18.12+incompatible
	golang.org/x/crypto v0.0.0-20190923035154-9ee001bba392
	golang.org/x/sys v0.0.0-20190922100055-0a153f010e69
	gotest.tools v2.2.0+incompatible
)
