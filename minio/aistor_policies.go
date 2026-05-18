package minio

const aistorEdition = "AIStor"

const (
	aistorPolicyReadOnly  = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetBucketLocation","s3:GetObject"],"Resource":["arn:aws:s3:::*"]},{"Effect":"Deny","Action":["admin:CreateUser"],"Resource":["arn:aws:s3:::*"]}]}`
	aistorPolicyReadWrite = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:*"],"Resource":["arn:aws:s3:::*"]}]}`
	aistorPolicyWriteOnly = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:PutObject"],"Resource":["arn:aws:s3:::*"]}]}`
)

func isAIStorClient(client *S3MinioClient) bool {
	return client != nil && client.Edition == aistorEdition
}

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
