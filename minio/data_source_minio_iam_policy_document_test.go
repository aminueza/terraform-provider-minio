package minio

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccMinioDataSourceIAMPolicyDocument_basic(t *testing.T) {
	// This really ought to be able to be a unit test rather than an
	// acceptance test, but just instantiating the Minio provider requires
	// some Minio API calls, and so this needs valid Minio credentials to work.
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMPolicyDocumentConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_iam_policy_document.test", "json",
						testAccMinioIAMPolicyDocumentExpectedJSON,
					),
				),
			},
		},
	})
}

func TestAccMinioDataSourceIAMPolicyDocument_source(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMPolicyDocumentSourceConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_iam_policy_document.test_source", "json",
						testAccMinioIAMPolicyDocumentSourceExpectedJSON,
					),
				),
			},
			{
				Config: testAccMinioIAMPolicyDocumentSourceBlankConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_iam_policy_document.test_source_blank", "json",
						testAccMinioIAMPolicyDocumentSourceBlankExpectedJSON,
					),
				),
			},
		},
	})
}

func TestAccMinioDataSourceIAMPolicyDocument_sourceConflicting(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMPolicyDocumentSourceConflictingConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_iam_policy_document.test_source_conflicting", "json",
						testAccMinioIAMPolicyDocumentSourceConflictingExpectedJSON,
					),
				),
			},
		},
	})
}

func TestAccMinioDataSourceIAMPolicyDocument_override(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMPolicyDocumentOverrideConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_iam_policy_document.test_override", "json",
						testAccMinioIAMPolicyDocumentOverrideExpectedJSON,
					),
				),
			},
		},
	})
}

func TestAccMinioDataSourceIAMPolicyDocument_noStatementMerge(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMPolicyDocumentNoStatementMergeConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_iam_policy_document.yak_politik", "json",
						testAccMinioIAMPolicyDocumentNoStatementMergeExpectedJSON,
					),
				),
			},
		},
	})
}

func TestAccMinioDataSourceIAMPolicyDocument_noStatementOverride(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMPolicyDocumentNoStatementOverrideConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_iam_policy_document.yak_politik", "json",
						testAccMinioIAMPolicyDocumentNoStatementOverrideExpectedJSON,
					),
				),
			},
		},
	})
}

func TestAccMinioDataSourceIAMPolicyDocument_duplicateSid(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:      testAccMinioIAMPolicyDocumentDuplicateSidConfig,
				ExpectError: regexp.MustCompile(`found duplicate sid`),
			},
			{
				Config: testAccMinioIAMPolicyDocumentDuplicateBlankSidConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_iam_policy_document.test", "json",
						testAccMinioIAMPolicyDocumentDuplicateBlankSidExpectedJSON,
					),
				),
			},
		},
	})
}

func TestAccMinioDataSourceIAMPolicyDocument_Statement_Principal_Identifiers_StringAndSlice(t *testing.T) {
	dataSourceName := "data.minio_iam_policy_document.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMPolicyDocumentConfigStatementPrincipalIdentifiersStringAndSlice,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "json", testAccMinioIAMPolicyDocumentExpectedJSONStatementPrincipalIdentifiersStringAndSlice),
				),
			},
		},
	})
}

func TestAccMinioDataSourceIAMPolicyDocument_Statement_Principal_Identifiers_MultiplePrincipals(t *testing.T) {
	dataSourceName := "data.minio_iam_policy_document.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMPolicyDocumentConfigStatementPrincipalIdentifiersMultiplePrincipals,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "json", testAccMinioIAMPolicyDocumentExpectedJSONStatementPrincipalIdentifiersMultiplePrincipals),
				),
			},
		},
	})
}

func TestAccMinioDataSourceIAMPolicyDocument_Statement_Principal_SpecificARN(t *testing.T) {
	dataSourceName := "data.minio_iam_policy_document.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMPolicyDocumentConfigStatementPrincipalSpecificARN,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "json", testAccMinioIAMPolicyDocumentExpectedJSONStatementPrincipalSpecificARN),
				),
			},
		},
	})
}

var testAccMinioIAMPolicyDocumentConfig = `
data "minio_iam_policy_document" "test" {
    policy_id = "policy_id"
    statement {
    	sid = "1"
        actions = [
            "s3:ListAllMyBuckets",
            "s3:GetBucketLocation",
        ]
        resources = [
            "arn:aws:s3:::*",
        ]
    }
    statement {
        actions = [
            "s3:ListBucket",
        ]
        resources = [
            "arn:aws:s3:::foo",
        ]
        condition {
            test = "StringLike"
            variable = "s3:prefix"
            values = [
                "home/&{aws:username}/"
            ]
        }
    }
    statement {
        actions = [
            "s3:*",
        ]
        resources = [
            "arn:aws:s3:::foo/home/&{aws:username}",
            "arn:aws:s3:::foo/home/&{aws:username}/*",
        ]
        principal = "*"
    }
    # Normalization of wildcard principals
    statement {
        effect = "Allow"
        actions = ["kinesis:*"]
		principal = "*"
    }
    statement {
        effect = "Allow"
        actions = ["firehose:*"]
		principal = "*"
    }
}
`

var testAccMinioIAMPolicyDocumentExpectedJSON = `{
  "Version": "2012-10-17",
  "Id": "policy_id",
  "Statement": [
    {
      "Sid": "1",
      "Effect": "Allow",
      "Action": [
        "s3:ListAllMyBuckets",
        "s3:GetBucketLocation"
      ],
      "Resource": "arn:aws:s3:::*"
    },
    {
      "Sid": "",
      "Effect": "Allow",
      "Action": "s3:ListBucket",
      "Resource": "arn:aws:s3:::foo",
      "Condition": {
        "StringLike": {
          "s3:prefix": [
            "home/${aws:username}/"
          ]
        }
      }
    },
    {
      "Sid": "",
      "Effect": "Allow",
      "Action": "s3:*",
      "Resource": [
        "arn:aws:s3:::foo/home/${aws:username}/*",
        "arn:aws:s3:::foo/home/${aws:username}"
      ],
      "Principal": "*"
    },
    {
      "Sid": "",
      "Effect": "Allow",
      "Action": "kinesis:*",
      "Principal": "*"
    },
    {
      "Sid": "",
      "Effect": "Allow",
      "Action": "firehose:*",
      "Principal": "*"
    }
  ]
}`

var testAccMinioIAMPolicyDocumentSourceConfig = `
data "minio_iam_policy_document" "test" {
    policy_id = "policy_id"
    statement {
        sid = "1"
        actions = [
            "s3:ListAllMyBuckets",
            "s3:GetBucketLocation",
        ]
        resources = [
            "arn:aws:s3:::*",
        ]
    }
    statement {
        actions = [
            "s3:ListBucket",
        ]
        resources = [
            "arn:aws:s3:::foo",
        ]
        condition {
            test = "StringLike"
            variable = "s3:prefix"
            values = [
                "home/&{aws:username}/",
            ]
        }
    }
    statement {
        actions = [
            "s3:*",
        ]
        resources = [
            "arn:aws:s3:::foo/home/&{aws:username}",
            "arn:aws:s3:::foo/home/&{aws:username}/*",
        ]
        principal = "*"
    }
    # Normalization of wildcard principals
    statement {
        effect = "Allow"
        actions = ["kinesis:*"]
        principal = "*"
    }
    statement {
        effect = "Allow"
        actions = ["firehose:*"]
        principal = "*"
    }
}
data "minio_iam_policy_document" "test_source" {
    source_json = "${data.minio_iam_policy_document.test.json}"
    statement {
        sid       = "SourceJSONTest1"
        actions   = ["*"]
        resources = ["*"]
    }
}
`

var testAccMinioIAMPolicyDocumentSourceExpectedJSON = `{
  "Version": "2012-10-17",
  "Id": "policy_id",
  "Statement": [
    {
      "Sid": "1",
      "Effect": "Allow",
      "Action": [
        "s3:ListAllMyBuckets",
        "s3:GetBucketLocation"
      ],
      "Resource": "arn:aws:s3:::*"
    },
    {
      "Sid": "",
      "Effect": "Allow",
      "Action": "s3:ListBucket",
      "Resource": "arn:aws:s3:::foo",
      "Condition": {
        "StringLike": {
          "s3:prefix": [
            "home/${aws:username}/"
          ]
        }
      }
    },
    {
      "Sid": "",
      "Effect": "Allow",
      "Action": "s3:*",
      "Resource": [
        "arn:aws:s3:::foo/home/${aws:username}/*",
        "arn:aws:s3:::foo/home/${aws:username}"
      ],
      "Principal": "*"
    },
    {
      "Sid": "",
      "Effect": "Allow",
      "Action": "kinesis:*",
      "Principal": "*"
    },
    {
      "Sid": "",
      "Effect": "Allow",
      "Action": "firehose:*",
      "Principal": "*"
    },
    {
      "Sid": "SourceJSONTest1",
      "Effect": "Allow",
      "Action": "*",
      "Resource": "*"
    }
  ]
}`

var testAccMinioIAMPolicyDocumentSourceBlankConfig = `
data "minio_iam_policy_document" "test_source_blank" {
    source_json = ""
    statement {
        sid       = "SourceJSONTest2"
        actions   = ["*"]
        resources = ["*"]
    }
}
`

var testAccMinioIAMPolicyDocumentSourceBlankExpectedJSON = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SourceJSONTest2",
      "Effect": "Allow",
      "Action": "*",
      "Resource": "*"
    }
  ]
}`

var testAccMinioIAMPolicyDocumentSourceConflictingConfig = `
data "minio_iam_policy_document" "test_source" {
    statement {
        sid       = "SourceJSONTestConflicting"
        actions   = ["s3:*"]
        resources = ["*"]
    }
}
data "minio_iam_policy_document" "test_source_conflicting" {
    source_json = "${data.minio_iam_policy_document.test_source.json}"
    statement {
        sid       = "SourceJSONTestConflicting"
        actions   = ["*"]
        resources = ["*"]
    }
}
`

var testAccMinioIAMPolicyDocumentSourceConflictingExpectedJSON = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SourceJSONTestConflicting",
      "Effect": "Allow",
      "Action": "*",
      "Resource": "*"
    }
  ]
}`

var testAccMinioIAMPolicyDocumentOverrideConfig = `
data "minio_iam_policy_document" "override" {
  statement {
    sid = "SidToOverwrite"
    actions   = ["s3:*"]
    resources = ["*"]
  }
}
data "minio_iam_policy_document" "test_override" {
  override_json = "${data.minio_iam_policy_document.override.json}"
  statement {
    actions   = ["s3:*"]
    resources = ["*"]
  }
  statement {
    sid = "SidToOverwrite"
    actions = ["s3:*"]
    resources = [
      "arn:aws:s3:::somebucket",
      "arn:aws:s3:::somebucket/*",
    ]
  }
}
`

var testAccMinioIAMPolicyDocumentOverrideExpectedJSON = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "",
      "Effect": "Allow",
      "Action": "s3:*",
      "Resource": "*"
    },
    {
      "Sid": "SidToOverwrite",
      "Effect": "Allow",
      "Action": "s3:*",
      "Resource": "*"
    }
  ]
}`

var testAccMinioIAMPolicyDocumentNoStatementMergeConfig = `
data "minio_iam_policy_document" "source" {
  statement {
    sid = ""
    actions   = ["s3:GetObject"]
    resources = ["*"]
  }
}
data "minio_iam_policy_document" "override" {
  statement {
    sid = "OverridePlaceholder"
    actions   = ["s3:GetObject"]
    resources = ["*"]
  }
}
data "minio_iam_policy_document" "yak_politik" {
  source_json = "${data.minio_iam_policy_document.source.json}"
  override_json = "${data.minio_iam_policy_document.override.json}"
}
`

var testAccMinioIAMPolicyDocumentNoStatementMergeExpectedJSON = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "",
      "Effect": "Allow",
      "Action": "s3:GetObject",
      "Resource": "*"
    },
    {
      "Sid": "OverridePlaceholder",
      "Effect": "Allow",
      "Action": "s3:GetObject",
      "Resource": "*"
    }
  ]
}`

var testAccMinioIAMPolicyDocumentNoStatementOverrideConfig = `
data "minio_iam_policy_document" "source" {
  statement {
    sid = "OverridePlaceholder"
    actions   = ["s3:GetObject"]
    resources = ["*"]
  }
}
data "minio_iam_policy_document" "override" {
  statement {
    sid = "OverridePlaceholder"
    actions   = ["s3:GetObject"]
    resources = ["*"]
  }
}
data "minio_iam_policy_document" "yak_politik" {
  source_json = "${data.minio_iam_policy_document.source.json}"
  override_json = "${data.minio_iam_policy_document.override.json}"
}
`

var testAccMinioIAMPolicyDocumentNoStatementOverrideExpectedJSON = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "OverridePlaceholder",
      "Effect": "Allow",
      "Action": "s3:GetObject",
      "Resource": "*"
    }
  ]
}`

var testAccMinioIAMPolicyDocumentDuplicateSidConfig = `
data "minio_iam_policy_document" "test" {
  statement {
    sid    = "1"
    effect = "Allow"
    actions = ["s3:GetObject"]
    resources = ["*"]
  }
  statement {
    sid    = "1"
    effect = "Allow"
    actions = ["s3:GetObject"]
    resources = ["*"]
  }
}`

var testAccMinioIAMPolicyDocumentDuplicateBlankSidConfig = `
  data "minio_iam_policy_document" "test" {
    statement {
      sid    = ""
      effect = "Allow"
      actions = ["s3:GetObject"]
      resources = ["*"]
    }
    statement {
      sid    = ""
      effect = "Allow"
      actions = ["s3:GetObject"]
      resources = ["*"]
    }
  }`

var testAccMinioIAMPolicyDocumentDuplicateBlankSidExpectedJSON = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "",
      "Effect": "Allow",
      "Action": "s3:GetObject",
      "Resource": "*"
    },
    {
      "Sid": "",
      "Effect": "Allow",
      "Action": "s3:GetObject",
      "Resource": "*"
    }
  ]
}`

var testAccMinioIAMPolicyDocumentConfigStatementPrincipalIdentifiersStringAndSlice = `
data "minio_iam_policy_document" "test" {
  statement {
    actions   = ["*"]
    resources = ["*"]
    sid       = "StatementPrincipalIdentifiersStringAndSlice"
    principal = "*"
  }
}
`

var testAccMinioIAMPolicyDocumentExpectedJSONStatementPrincipalIdentifiersStringAndSlice = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "StatementPrincipalIdentifiersStringAndSlice",
      "Effect": "Allow",
      "Action": "*",
      "Resource": "*",
      "Principal": "*"
    }
  ]
}`

var testAccMinioIAMPolicyDocumentConfigStatementPrincipalIdentifiersMultiplePrincipals = `
data "minio_iam_policy_document" "test" {
  statement {
    actions   = ["*"]
    resources = ["*"]
    sid       = "StatementPrincipalIdentifiersStringAndSlice"
    principal = "*"

  }
}
`

var testAccMinioIAMPolicyDocumentExpectedJSONStatementPrincipalIdentifiersMultiplePrincipals = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "StatementPrincipalIdentifiersStringAndSlice",
      "Effect": "Allow",
      "Action": "*",
      "Resource": "*",
      "Principal": "*"
    }
  ]
}`

var testAccMinioIAMPolicyDocumentConfigStatementPrincipalSpecificARN = `
data "minio_iam_policy_document" "test" {
  statement {
    actions   = ["s3:*"]
    resources = ["arn:aws:s3:::test-bucket", "arn:aws:s3:::test-bucket/*"]
    sid       = "SpecificPrincipalARN"
    effect    = "Allow"
    principal = "arn:aws:iam:::user/p10439088:SLCDTCG0PS1QPQJKQ99F"
  }
}
`

var testAccMinioIAMPolicyDocumentExpectedJSONStatementPrincipalSpecificARN = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SpecificPrincipalARN",
      "Effect": "Allow",
      "Action": "s3:*",
      "Resource": [
        "arn:aws:s3:::test-bucket/*",
        "arn:aws:s3:::test-bucket"
      ],
      "Principal": "arn:aws:iam:::user/p10439088:SLCDTCG0PS1QPQJKQ99F"
    }
  ]
}`
