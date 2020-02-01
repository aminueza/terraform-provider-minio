# IAM POLICY DOCUMENT

Generates an IAM policy document in JSON format.
For more information, see the [AWS terraform examples](https://www.terraform.io/docs/providers/aws/d/iam_policy_document.html).

## Example of usage

```hcl

data "minio_iam_policy_document" "example" {
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
      "arn:aws:s3:::state-terraform-s3",
    ]

    condition {
      test     = "StringLike"
      variable = "s3:prefix"

      values = [
        "",
        "home/",
      ]
    }
  }

  statement {
    actions = [
      "s3:PutObject",
    ]

    resources = [
      "arn:aws:s3:::state-terraform-s3",
      "arn:aws:s3:::state-terraform-s3/*",
    ]
  }
}
resource "minio_iam_policy" "test_policy" {
  name      = "state-terraform-s3"
  policy    = data.minio_iam_policy_document.example.json

}
```

## Argument Reference

The following arguments are supported:

* **policy_id** - (Optional) - An ID for the policy document.
* **source_json** - (Optional) - An IAM policy document to import as a base for the current policy document. Statements with non-blank `sids` in the current policy document will overwrite statements with the same `sid` in the source json. Statements without an `sid` cannot be overwritten.
* **override_json** - (Optional) - An IAM policy document to import and override the current policy document. Statements with non-blank `sids` in the override document will overwrite statements with the same `sid` in the current document. Statements without an `sid` cannot be overwritten.
* **statement** - (Optional) - A nested configuration block (described below) configuring one statement to be included in the policy document.
* **version** - (Optional) - IAM policy document version. Valid values: `2008-10-17`, `2012-10-17`. Defaults to `2012-10-17`. 

Each document configuration may have one or more `statement` blocks, which each accept the following arguments:

* **sid** - (Optional) - An ID for the policy statement.
* **effect** - (Optional) - Either `Allow` or `Deny`, to specify whether this statement allows or denies the given actions. The default is `Allow`.
* **actions** - (Optional) - A list of actions that this statement either allows or denies. For example, `["s3:PutObject", "s3:GetObject"]`.
* **resources** - (Optional) - A list of resource ARNs that this statement applies to. This is required by AWS if used for an IAM policy.
* **principal** - (Optional) - A nested configuration block (described below) specifying a resource (or resource pattern) to which this statement applies. Due to MinIO limitations, the principal available is only `AWS: "*"`.
* **condition** - (Optional) - A nested configuration block (described below) that defines a further, possibly-service-specific condition that constrains whether this statement applies.

Each policy statement may have zero or more condition blocks, which each accept the following arguments:

* **test** - (Required) The name of the IAM condition operator to evaluate.
* **variable** - (Required) The name of a Context Variable to apply the condition to. Context variables may either be standard AWS variables starting with `aws:`, or service-specific variables prefixed with the service name.
* **values** - (Required) The values to evaluate the condition against. If multiple values are provided, the condition matches if at least one of them applies. (That is, the tests are combined with the "OR" boolean operation.)

## Output

The following outputs are supported:

* **id** - (Optional) The name to identify the policy document.
* **json** - (Optional) Convert arguments serialized as a standard JSON policy document.
