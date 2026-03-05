resource "minio_s3_object_retention" "compliance" {
  bucket            = "locked-bucket"
  key               = "important-doc.pdf"
  mode              = "COMPLIANCE"
  retain_until_date = "2027-01-01T00:00:00Z"
}
