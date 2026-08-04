package minio

// ReadWritePolicy returns a policy where objects can be uploaded and read.
//
// MinIO has exactly three anonymous shapes, and the read-write one is what `mc anonymous set
// public` writes, so this is the same policy as PublicPolicy. Keeping it a single builder means
// the two access types cannot drift apart into a shape no client recognises.
func ReadWritePolicy(bucket *S3MinioBucket) BucketPolicy {
	return PublicPolicy(bucket)
}
