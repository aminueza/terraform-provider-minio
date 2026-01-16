package minio

import (
	"context"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v3"
)

type siteDiff struct {
	toAdd    []madmin.PeerSite
	toRemove []string
}

// calculateSiteDiff compares old and new site configurations.
// MinIO doesn't support in-place updates, so site changes are handled as remove+add operations.
func calculateSiteDiff(oldSites, newSites []madmin.PeerSite) *siteDiff {
	diff := &siteDiff{
		toAdd:    make([]madmin.PeerSite, 0),
		toRemove: make([]string, 0),
	}

	oldSiteMap := make(map[string]madmin.PeerSite)
	newSiteMap := make(map[string]madmin.PeerSite)

	for _, site := range oldSites {
		oldSiteMap[site.Name] = site
	}
	for _, site := range newSites {
		newSiteMap[site.Name] = site
	}

	for siteName := range oldSiteMap {
		if _, exists := newSiteMap[siteName]; !exists {
			diff.toRemove = append(diff.toRemove, siteName)
		}
	}

	for siteName, newSite := range newSiteMap {
		if oldSite, exists := oldSiteMap[siteName]; !exists {
			diff.toAdd = append(diff.toAdd, newSite)
		} else if sitesDiffer(oldSite, newSite) {
			diff.toRemove = append(diff.toRemove, siteName)
			diff.toAdd = append(diff.toAdd, newSite)
		}
	}

	return diff
}

func sitesDiffer(oldSite, newSite madmin.PeerSite) bool {
	return oldSite.Endpoint != newSite.Endpoint ||
		oldSite.AccessKey != newSite.AccessKey ||
		oldSite.SecretKey != newSite.SecretKey
}

func isSiteReplicationError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "site replication not configured") ||
		strings.Contains(errStr, "Global deployment ID mismatch") ||
		strings.Contains(errStr, "Unable to fetch server info")
}

func resourceMinioSiteReplication() *schema.Resource {
	return &schema.Resource{
		CreateContext: minioCreateSiteReplication,
		ReadContext:   minioReadSiteReplication,
		UpdateContext: minioUpdateSiteReplication,
		DeleteContext: minioDeleteSiteReplication,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the site replication configuration",
			},
			"site": {
				Type:        schema.TypeList,
				Required:    true,
				MinItems:    2,
				Description: "List of sites to replicate between (minimum 2). Access_key and secret_key are stored in state but not returned by the MinIO API during read operations for security reasons.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Unique name for this site",
						},
						"endpoint": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "MinIO server endpoint URL",
						},
						"access_key": {
							Type:        schema.TypeString,
							Optional:    true,
							Computed:    true,
							Description: "Access key for the site. Stored in Terraform state but not returned by the MinIO API for security reasons.",
						},
						"secret_key": {
							Type:        schema.TypeString,
							Optional:    true,
							Computed:    true,
							Sensitive:   true,
							Description: "Secret key for the site. Stored in Terraform state but not returned by the MinIO API for security reasons.",
						},
					},
				},
			},
			"enabled": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Whether site replication is enabled",
			},
		},
	}
}

func minioCreateSiteReplication(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Admin
	name := d.Get("name").(string)

	sites := expandSites(d.Get("site").([]interface{}))

	log.Printf("[DEBUG] Creating site replication: %s with %d sites", name, len(sites))

	status, err := client.SiteReplicationAdd(ctx, sites)
	if err != nil {
		return NewResourceError("error creating site replication", name, err)
	}

	log.Printf("[DEBUG] Site replication created: %+v", status)

	d.SetId(name)
	return minioReadSiteReplication(ctx, d, meta)
}

func minioReadSiteReplication(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Admin

	info, err := client.SiteReplicationInfo(ctx)
	if err != nil {
		if isSiteReplicationError(err) {
			log.Printf("[WARN] Site replication not configured or disabled, removing from state: %v", err)
			d.SetId("")
			return nil
		}
		return NewResourceError("error reading site replication", d.Id(), err)
	}

	if !info.Enabled {
		log.Printf("[WARN] Site replication not enabled, removing from state")
		d.SetId("")
		return nil
	}

	name := d.Get("name").(string)
	if name == "" {
		name = d.Id()
	}
	_ = d.Set("name", name)

	_ = d.Set("enabled", info.Enabled)
	_ = d.Set("site", flattenSitesWithState(d, info.Sites))

	return nil
}

func minioUpdateSiteReplication(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Admin

	if d.HasChange("site") {
		old, new := d.GetChange("site")
		oldSites := expandSites(old.([]interface{}))
		newSites := expandSites(new.([]interface{}))

		diff := calculateSiteDiff(oldSites, newSites)

		if len(diff.toRemove) > 0 {
			log.Printf("[DEBUG] Removing %d sites from replication: %v", len(diff.toRemove), diff.toRemove)
			_, err := client.SiteReplicationRemove(ctx, madmin.SRRemoveReq{
				SiteNames: diff.toRemove,
			})
			if err != nil {
				return NewResourceError("error removing sites from replication", d.Id(), err)
			}
		}

		if len(diff.toAdd) > 0 {
			log.Printf("[DEBUG] Adding %d sites to replication: %v", len(diff.toAdd),
				func() []string {
					names := make([]string, len(diff.toAdd))
					for i, site := range diff.toAdd {
						names[i] = site.Name
					}
					return names
				}())
			_, err := client.SiteReplicationAdd(ctx, diff.toAdd)
			if err != nil {
				return NewResourceError("error adding sites to replication", d.Id(), err)
			}
		}
	}

	return minioReadSiteReplication(ctx, d, meta)
}

func minioDeleteSiteReplication(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*S3MinioClient).S3Admin

	sites := expandSites(d.Get("site").([]interface{}))
	siteNames := make([]string, len(sites))
	for i, site := range sites {
		siteNames[i] = site.Name
	}

	log.Printf("[DEBUG] Deleting site replication with %d sites: %v", len(siteNames), siteNames)

	_, err := client.SiteReplicationRemove(ctx, madmin.SRRemoveReq{
		SiteNames: siteNames,
		RemoveAll: true,
	})
	if err != nil {
		if isSiteReplicationError(err) {
			log.Printf("[INFO] Site replication already removed or not configured: %v", err)
			return nil
		}
		return NewResourceError("error deleting site replication", d.Id(), err)
	}

	return nil
}

func expandSites(sites []interface{}) []madmin.PeerSite {
	result := make([]madmin.PeerSite, len(sites))
	for i, s := range sites {
		site := s.(map[string]interface{})
		result[i] = madmin.PeerSite{
			Name:      site["name"].(string),
			Endpoint:  site["endpoint"].(string),
			AccessKey: site["access_key"].(string),
			SecretKey: site["secret_key"].(string),
		}
	}
	return result
}

// flattenSitesWithState preserves sensitive credentials from Terraform state
// because MinIO API doesn't return access_key/secret_key for security.
func flattenSitesWithState(d *schema.ResourceData, sites []madmin.PeerInfo) []map[string]interface{} {
	result := make([]map[string]interface{}, len(sites))

	existingSites := d.Get("site").([]interface{})

	for i, site := range sites {
		siteMap := map[string]interface{}{
			"name":     site.Name,
			"endpoint": site.Endpoint,
		}

		// Preserve access_key and secret_key from state if they exist
		// This is necessary because MinIO API doesn't return these sensitive values
		if i < len(existingSites) {
			if existingSite, ok := existingSites[i].(map[string]interface{}); ok {
				if accessKey, ok := existingSite["access_key"].(string); ok && accessKey != "" {
					siteMap["access_key"] = accessKey
				}
				if secretKey, ok := existingSite["secret_key"].(string); ok && secretKey != "" {
					siteMap["secret_key"] = secretKey
				}
			}
		}

		result[i] = siteMap
	}
	return result
}
