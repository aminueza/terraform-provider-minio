terraform {
  required_providers {
    minio = {
      source  = "aminueza/minio"
      version = "~> 2"
    }
    vault = {
      source  = "hashicorp/vault"
      version = "~> 4.8"
    }
  }
}

provider "minio" {
  minio_server   = "https://minio1.example.com"
  minio_user     = "minioadmin"
  minio_password = "minio123"
}

ephemeral "vault_kv_secret_v2" "site1_secret" {
  mount = "secret"
  name  = "minio/site-replication/site1"
}

ephemeral "vault_kv_secret_v2" "site2_secret" {
  mount = "secret"
  name  = "minio/site-replication/site2"
}

ephemeral "vault_kv_secret_v2" "site3_secret" {
  mount = "secret"
  name  = "minio/site-replication/site3"
}

# Example: Basic site replication with static configuration
resource "minio_site_replication" "primary" {
  name = "my-cluster-replication"

  site {
    name       = "site1"
    endpoint   = "https://minio1.example.com"
    access_key = "minioadmin"
    secret_key = "minio123"
  }

  site {
    name       = "site2"
    endpoint   = "https://minio2.example.com"
    access_key = "minioadmin"
    secret_key = "minio456"
  }

  site {
    name       = "site3"
    endpoint   = "https://minio3.example.com"
    access_key = "minioadmin"
    secret_key = "minio789"
  }
}

# Example: Site replication with write-only secrets from Vault
resource "minio_site_replication" "primary_write_only" {
  name = "my-cluster-replication-write-only"

  site {
    name                  = "site1"
    endpoint              = "https://minio1.example.com"
    access_key            = "minioadmin"
    secret_key_wo         = tostring(ephemeral.vault_kv_secret_v2.site1_secret.data.secret_key)
    secret_key_wo_version = 1
  }

  site {
    name                  = "site2"
    endpoint              = "https://minio2.example.com"
    access_key            = "minioadmin"
    secret_key_wo         = tostring(ephemeral.vault_kv_secret_v2.site2_secret.data.secret_key)
    secret_key_wo_version = 1
  }

  site {
    name                  = "site3"
    endpoint              = "https://minio3.example.com"
    access_key            = "minioadmin"
    secret_key_wo         = tostring(ephemeral.vault_kv_secret_v2.site3_secret.data.secret_key)
    secret_key_wo_version = 1
  }
}
