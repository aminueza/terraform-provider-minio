# S3 BUCKET

Manages S3 Buckets.

## Example of usage

```hcl
resource "minio_s3_bucket" "state_terraform_s3" {
  bucket = "state-terraform-s3"
  acl    = "public"
}

output "minio_id" {
  value = "${minio_s3_bucket.state_terraform_s3.id}"
}

output "minio_url" {
  value = "${minio_s3_bucket.state_terraform_s3.bucket_domain_name}"
}
```

## Argument Reference

The following arguments are supported:

* **bucket** - (Required) The bucket's name.
* **acl** - (Required) The canned ACL to apply. There are five predefined ACLs: private, public-write, public-read, public-read-write and public. Defaults to "private".

## Output

The following outputs are supported:

* **id** - (Optional) Returns a bucket's id. It's same of user name.
* **bucket_domain_name** - (Optional) The bucket domain name. Will be of format `bucket_server:port/minio/bucket`

