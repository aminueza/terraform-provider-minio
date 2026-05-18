package minio

const aistorEdition = "AIStor"

// Canned AIStor anonymous-access policies. AIStor classifies a bucket as
// readonly/readwrite/writeonly only when the stored policy matches one of
// these templates byte-equivalent. Sources confirmed by users in #873:
// https://github.com/aminueza/terraform-provider-minio/discussions/873.
const (
	aistorPolicyReadOnly  = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetBucketLocation","s3:GetObject"],"Resource":["arn:aws:s3:::*"]},{"Effect":"Deny","Action":["admin:CreateUser"],"Resource":["arn:aws:s3:::*"]}]}`
	aistorPolicyReadWrite = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:*"],"Resource":["arn:aws:s3:::*"]}]}`
	aistorPolicyWriteOnly = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:PutObject"],"Resource":["arn:aws:s3:::*"]}]}`
)

func isAIStorClient(client *S3MinioClient) bool {
	return client != nil && client.Edition == aistorEdition
}

// aistorCannedPolicy returns AIStor's canned anonymous-access policy JSON for
// the given access_type, or ok=false when no equivalent exists. AIStor exposes
// only three canned templates, so the OSS provider's `public` and
// `public-read-write` both map to AIStor's `readwrite` (s3:*).
func aistorCannedPolicy(accessType string) (string, bool) {
	switch accessType {
	case "public-read":
		return aistorPolicyReadOnly, true
	case "public-read-write", "public":
		return aistorPolicyReadWrite, true
	case "public-write":
		return aistorPolicyWriteOnly, true
	}
	return "", false
}
