package minio

import (
	"testing"
)

func TestAccMinioBatchJob_basic(t *testing.T) {
	t.Skip("Batch job tests require a pre-configured bucket and multi-cluster replication setup not available in the shared CI fixture. To run manually, create a bucket and set up the required replication configuration, then run with TF_ACC=1.")
}
