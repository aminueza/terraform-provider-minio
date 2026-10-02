// Command hosted-cleanup removes the buckets an acceptance run left behind on a
// hosted S3 backend. It only touches buckets whose name starts with one of the
// prefixes the acceptance tests use and that were created at or after the time
// given in -since, so a run never deletes a bucket it did not create.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var testBucketPrefixes = []string{"tfacc", "tf-"}

func main() {
	since := flag.String("since", "", "RFC 3339 time the run started; only buckets created at or after it are removed")
	dryRun := flag.Bool("dry-run", false, "list the buckets that would be removed without removing them")
	flag.Parse()

	if err := run(*since, *dryRun); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(since string, dryRun bool) error {
	start, err := time.Parse(time.RFC3339, since)
	if err != nil {
		return fmt.Errorf("-since must be an RFC 3339 time: %w", err)
	}

	client, err := minio.New(os.Getenv("MINIO_ENDPOINT"), &minio.Options{
		Creds:  credentials.NewStaticV4(os.Getenv("MINIO_USER"), os.Getenv("MINIO_PASSWORD"), ""),
		Secure: os.Getenv("MINIO_ENABLE_HTTPS") != "false",
		Region: os.Getenv("MINIO_REGION"),
	})
	if err != nil {
		return fmt.Errorf("creating the S3 client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	buckets, err := client.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("listing buckets: %w", err)
	}

	var leftovers []string
	for _, bucket := range buckets {
		if !isTestBucket(bucket.Name) || bucket.CreationDate.Before(start) {
			continue
		}
		if dryRun {
			fmt.Printf("would remove %s (created %s)\n", bucket.Name, bucket.CreationDate.Format(time.RFC3339))
			continue
		}
		if err := removeBucket(ctx, client, bucket.Name); err != nil {
			leftovers = append(leftovers, fmt.Sprintf("%s: %v", bucket.Name, err))
			continue
		}
		fmt.Printf("removed %s\n", bucket.Name)
	}

	if len(leftovers) > 0 {
		return fmt.Errorf("could not remove %d bucket(s):\n  %s", len(leftovers), strings.Join(leftovers, "\n  "))
	}
	return nil
}

func isTestBucket(name string) bool {
	for _, prefix := range testBucketPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// removeBucket empties the bucket and deletes it. Versioned listing comes first
// so that noncurrent versions and delete markers go too; a backend that does not
// implement it gets a plain listing, which is all such a backend can hold.
func removeBucket(ctx context.Context, client *minio.Client, name string) error {
	if err := removeObjects(ctx, client, name, true); err != nil {
		if minio.ToErrorResponse(err).Code != "NotImplemented" {
			return err
		}
		if err := removeObjects(ctx, client, name, false); err != nil {
			return err
		}
	}
	return client.RemoveBucket(ctx, name)
}

func removeObjects(ctx context.Context, client *minio.Client, bucket string, withVersions bool) error {
	for object := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true, WithVersions: withVersions}) {
		if object.Err != nil {
			return object.Err
		}
		err := client.RemoveObject(ctx, bucket, object.Key, minio.RemoveObjectOptions{
			VersionID:        object.VersionID,
			GovernanceBypass: true,
		})
		if err != nil {
			return fmt.Errorf("removing %s: %w", object.Key, err)
		}
	}
	return nil
}
