package minio

import (
	"context"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceMinioServerInfo() *schema.Resource {
	return &schema.Resource{
		Description: "Reads MinIO server information including version, deployment ID, and storage metrics.",
		Read:        dataSourceMinioServerInfoRead,
		Schema: map[string]*schema.Schema{
			"version": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "MinIO server version",
			},
			"commit": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Git commit hash of the server build",
			},
			"region": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Server region",
			},
			"sqsarn": {
				Type:        schema.TypeList,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "List of configured SQS ARNs for event notifications",
			},
			"deployment_id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Deployment ID of the MinIO cluster",
			},
			"servers": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "List of servers in the cluster",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"state": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Server state (online, offline)",
						},
						"endpoint": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Server endpoint",
						},
						"uptime": {
							Type:        schema.TypeInt,
							Computed:    true,
							Description: "Server uptime in seconds",
						},
						"version": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Server version",
						},
						"commit_id": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Git commit hash",
						},
						"network": {
							Type:        schema.TypeMap,
							Computed:    true,
							Description: "Network statistics",
							Elem:        &schema.Schema{Type: schema.TypeString},
						},
						"drives": {
							Type:        schema.TypeList,
							Computed:    true,
							Description: "List of drives on this server",
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"endpoint": {
										Type:        schema.TypeString,
										Computed:    true,
										Description: "Drive endpoint/path",
									},
									"state": {
										Type:        schema.TypeString,
										Computed:    true,
										Description: "Drive state (ok, offline, corrupted, etc)",
									},
									"uuid": {
										Type:        schema.TypeString,
										Computed:    true,
										Description: "Drive UUID",
									},
									"total_space": {
										Type:        schema.TypeInt,
										Computed:    true,
										Description: "Total disk space in bytes",
									},
									"used_space": {
										Type:        schema.TypeInt,
										Computed:    true,
										Description: "Used disk space in bytes",
									},
									"available_space": {
										Type:        schema.TypeInt,
										Computed:    true,
										Description: "Available disk space in bytes",
									},
									"read_throughput": {
										Type:        schema.TypeInt,
										Computed:    true,
										Description: "Read throughput in bytes per second",
									},
									"write_throughput": {
										Type:        schema.TypeInt,
										Computed:    true,
										Description: "Write throughput in bytes per second",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func dataSourceMinioServerInfoRead(d *schema.ResourceData, meta interface{}) error {
	m := meta.(*S3MinioClient)
	admin := m.S3Admin

	info, err := admin.ServerInfo(context.Background())
	if err != nil {
		return err
	}

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))

	if len(info.Servers) > 0 {
		_ = d.Set("version", info.Servers[0].Version)
		_ = d.Set("commit", info.Servers[0].CommitID)
	}

	region := info.Region
	if region == "" {
		region = "us-east-1"
	}
	_ = d.Set("region", region)

	if info.SQSARN != nil {
		_ = d.Set("sqsarn", info.SQSARN)
	} else {
		_ = d.Set("sqsarn", []string{})
	}

	_ = d.Set("deployment_id", info.DeploymentID)

	servers := make([]map[string]interface{}, 0, len(info.Servers))
	for _, server := range info.Servers {
		drives := make([]map[string]interface{}, 0, len(server.Disks))
		for _, disk := range server.Disks {
			totalSpace, _ := SafeUint64ToInt64(disk.TotalSpace)
			usedSpace, _ := SafeUint64ToInt64(disk.UsedSpace)
			availableSpace, _ := SafeUint64ToInt64(disk.AvailableSpace)
			readThroughput, _ := SafeUint64ToInt64(uint64(disk.ReadThroughput))
			writeThroughput, _ := SafeUint64ToInt64(uint64(disk.WriteThroughPut))

			drives = append(drives, map[string]interface{}{
				"endpoint":         disk.Endpoint,
				"state":            disk.State,
				"uuid":             disk.UUID,
				"total_space":      totalSpace,
				"used_space":       usedSpace,
				"available_space":  availableSpace,
				"read_throughput":  readThroughput,
				"write_throughput": writeThroughput,
			})
		}

		uptime := SafeInt64ToInt64(server.Uptime)
		servers = append(servers, map[string]interface{}{
			"state":     server.State,
			"endpoint":  server.Endpoint,
			"uptime":    uptime,
			"version":   server.Version,
			"commit_id": server.CommitID,
			"network":   server.Network,
			"drives":    drives,
		})
	}
	_ = d.Set("servers", servers)

	return nil
}
