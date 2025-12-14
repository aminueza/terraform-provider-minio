terraform {
  required_providers {
    minio = {
      source  = "aminueza/minio"
      version = ">= 2.0.0"
    }
  }
}

# Configure the MinIO Provider
# Note: MinIO must be configured with LDAP authentication
# See: https://min.io/docs/minio/linux/operations/external-iam/configure-ad-ldap-external-identity-management.html
provider "minio" {
  minio_server   = var.minio_server
  minio_user     = var.minio_access_key
  minio_password = var.minio_secret_key
  minio_ssl      = var.minio_ssl
}

# Create an IAM policy for read-only access
resource "minio_iam_policy" "readonly" {
  name = "readonly-policy"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:ListBucket",
        ]
        Resource = [
          "arn:aws:s3:::*",
        ]
      }
    ]
  })
}

# Create an IAM policy for read-write access
resource "minio_iam_policy" "readwrite" {
  name = "readwrite-policy"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:DeleteObject",
          "s3:ListBucket",
        ]
        Resource = [
          "arn:aws:s3:::*",
        ]
      }
    ]
  })
}

# Attach policy to an LDAP group
# The group_dn must match the Distinguished Name of the LDAP group
resource "minio_iam_ldap_group_policy_attachment" "developers" {
  group_dn    = "cn=developers,ou=groups,dc=example,dc=com"
  policy_name = minio_iam_policy.readwrite.name
}

# Attach policy to another LDAP group
resource "minio_iam_ldap_group_policy_attachment" "viewers" {
  group_dn    = "cn=viewers,ou=groups,dc=example,dc=com"
  policy_name = minio_iam_policy.readonly.name
}

# Attach policy to a specific LDAP user
# The user_dn must match the Distinguished Name of the LDAP user
resource "minio_iam_ldap_user_policy_attachment" "admin_user" {
  user_dn     = "cn=admin,ou=users,dc=example,dc=com"
  policy_name = minio_iam_policy.readwrite.name
}
