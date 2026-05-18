package minio

import (
	"testing"
)

func TestAccDataSourceMinioBatchJobs_basic(t *testing.T) {
	t.Skip("Batch job tests require a pre-configured bucket and multi-cluster replication setup not available in the shared CI fixture. To run manually, create batch jobs on the MinIO instance, then run with TF_ACC=1.")
}

func TestAccDataSourceMinioBatchJobs_filterByType(t *testing.T) {
	t.Skip("Batch job tests require a pre-configured bucket and multi-cluster replication setup not available in the shared CI fixture. To run manually, create batch jobs on the MinIO instance, then run with TF_ACC=1.")
}
