package minio

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioKMSStatus() *schema.Resource {
	return &schema.Resource{
		Description: "Reports status and connectivity information about the KMS configured on the MinIO server.",
		ReadContext: dataSourceMinioKMSStatusRead,
		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Name or type of the KMS backend (e.g. `minio-kes`).",
			},
			"default_key_id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Key ID used by the MinIO server when no explicit key is specified.",
			},
			"version": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Version string reported by the KMS server.",
			},
			"apis": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "Supported `METHOD PATH` endpoints exposed by the KMS server.",
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"endpoints": {
				Type:        schema.TypeMap,
				Computed:    true,
				Description: "Map of KMS endpoint URLs to their reported state (`online` / `offline` / `init`).",
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"state": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "Current KMS server state snapshot.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"version":             {Type: schema.TypeString, Computed: true},
						"key_store_reachable": {Type: schema.TypeBool, Computed: true},
						"keystore_available":  {Type: schema.TypeBool, Computed: true},
						"key_store_latency_ms": {
							Type:        schema.TypeInt,
							Computed:    true,
							Description: "Key-store probe latency in milliseconds.",
						},
						"os":               {Type: schema.TypeString, Computed: true},
						"arch":             {Type: schema.TypeString, Computed: true},
						"uptime_seconds":   {Type: schema.TypeInt, Computed: true},
						"cpus":             {Type: schema.TypeInt, Computed: true},
						"usable_cpus":      {Type: schema.TypeInt, Computed: true},
						"heap_alloc_bytes": {Type: schema.TypeInt, Computed: true},
						"stack_alloc_bytes": {Type: schema.TypeInt, Computed: true},
					},
				},
			},
		},
	}
}

func dataSourceMinioKMSStatusRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	admin := meta.(*S3MinioClient).S3Admin

	status, err := admin.KMSStatus(ctx)
	if err != nil {
		return NewResourceError("reading KMS status", "kms", err)
	}

	versionStr := ""
	if v, err := admin.KMSVersion(ctx); err != nil {
		log.Printf("[DEBUG] KMSVersion unavailable: %v", err)
	} else if v != nil {
		versionStr = v.Version
	}

	apiStrs := []string{}
	if apis, err := admin.KMSAPIs(ctx); err != nil {
		log.Printf("[DEBUG] KMSAPIs unavailable: %v", err)
	} else {
		for _, a := range apis {
			apiStrs = append(apiStrs, fmt.Sprintf("%s %s", a.Method, a.Path))
		}
	}

	endpoints := make(map[string]interface{}, len(status.Endpoints))
	for url, state := range status.Endpoints {
		endpoints[url] = string(state)
	}

	id := status.Name
	if id == "" {
		id = "kms"
	}
	d.SetId(id)

	_ = d.Set("name", status.Name)
	_ = d.Set("default_key_id", status.DefaultKeyID)
	_ = d.Set("version", versionStr)
	_ = d.Set("apis", apiStrs)
	_ = d.Set("endpoints", endpoints)
	heapAlloc, _ := SafeUint64ToInt64(status.State.HeapAlloc)
	stackAlloc, _ := SafeUint64ToInt64(status.State.StackAlloc)

	_ = d.Set("state", []map[string]interface{}{{
		"version":              status.State.Version,
		"key_store_reachable":  status.State.KeyStoreReachable,
		"keystore_available":   status.State.KeystoreAvailable,
		"key_store_latency_ms": status.State.KeyStoreLatency.Milliseconds(),
		"os":                   status.State.OS,
		"arch":                 status.State.Arch,
		"uptime_seconds":       int64(status.State.UpTime.Seconds()),
		"cpus":                 status.State.CPUs,
		"usable_cpus":          status.State.UsableCPUs,
		"heap_alloc_bytes":     heapAlloc,
		"stack_alloc_bytes":    stackAlloc,
	}})

	return nil
}
