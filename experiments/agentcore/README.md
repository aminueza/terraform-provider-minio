# agentcore

An experiment. It drives the provider through the Terraform plugin protocol
(`tfprotov6`) without Terraform, so a caller can say "make this resource look
like this" and get back what happened.

The core keeps no state file. It discovers the prior state of a resource from
its id: it builds a state stub that holds only `id`, calls `ReadResource`, and
when the resource exists it calls `ImportResourceState` to fill the attributes
the importer knows about. It then runs the same sequence Terraform runs:
`PlanResourceChange`, `ApplyResourceChange`, `ReadResource`, and a second
`PlanResourceChange` to check that the provider has nothing left to change.

## Verbs

| Verb       | What it does                                                          |
|------------|-----------------------------------------------------------------------|
| `types`    | Lists the resource types the provider serves.                         |
| `plan`     | Discovers the prior state and reports the action and the planned state. |
| `converge` | Plans, applies, reads back and plans again. Reports `converged`.       |
| `delete`   | Destroys the resource when it exists.                                 |

Every verb reads one JSON request from stdin:

```json
{"type": "minio_s3_bucket", "id": "demo", "attributes": {"bucket": "demo", "tags": {"owner": "me"}}}
```

`attributes` must fit the resource schema. A key the schema does not declare is
refused before any provider call. The provider is configured from the
`MINIO_*` environment variables.

## Any provider

The core talks to a provider through the plugin protocol, so it is not tied to
this one. `Open` runs the MinIO provider in process. `OpenBinary` starts any
provider binary the way Terraform does: the go-plugin handshake, which settles
on protocol 5 or 6, then gRPC over `tfplugin5.proto` or `tfplugin6.proto`. The
generated protocol code under `internal/` is copied from `terraform-plugin-go`,
as the protocol files say to do, with the descriptor package renamed so that it
can be linked beside the original.

`random` and `null` both serve protocol 5. Their resources have nothing to read
back from a server, so discovery by id recovers the id and nothing else: a
second request for the same pet plans a replacement, not a no-op. A core that
serves such providers must remember the attributes it sent.

Set `AGENTCORE_PROVIDER` to a provider binary and the CLI uses it instead of
the MinIO provider:

```sh
go install github.com/hashicorp/terraform-provider-random/v3@v3.7.2
AGENTCORE_PROVIDER=$(go env GOPATH)/bin/terraform-provider-random ./agentcore types
echo '{"type":"random_pet","attributes":{"length":3}}' | AGENTCORE_PROVIDER=$(go env GOPATH)/bin/terraform-provider-random ./agentcore converge
```

The tests against `random` and `null` run when `AGENTCORE_RANDOM_PROVIDER` and
`AGENTCORE_NULL_PROVIDER` point at the binaries.

## The AWS provider against MinIO

`AGENTCORE_PROVIDER_CONFIG` carries the provider configuration as JSON when the
provider cannot be configured from the environment alone. The AWS provider
needs the `skip_*` flags and an S3 endpoint to run against MinIO:

```sh
export AGENTCORE_PROVIDER=$(go env GOPATH)/bin/terraform-provider-aws
export AGENTCORE_PROVIDER_CONFIG='{"region":"us-east-1","access_key":"minio","secret_key":"minio123","skip_credentials_validation":true,"skip_requesting_account_id":true,"skip_region_validation":true,"skip_metadata_api_check":"true","s3_use_path_style":true,"endpoints":[{"s3":"http://localhost:9000"}]}'
echo '{"type":"aws_s3_bucket","id":"demo","attributes":{"bucket":"demo"}}' | ./agentcore converge
```

The tests against the AWS provider run when `AGENTCORE_AWS_PROVIDER` and
`MINIO_ENDPOINT` are both set. `aws_s3_bucket` creates, updates and deletes and
converges on every step. `aws_s3_bucket_lifecycle_configuration` stores the
rule on MinIO but the provider times out waiting for the server to echo
`transition_default_minimum_object_size`, which MinIO never reports. The plan
that follows shows the rule read back equal to the rule sent and only that
attribute different. This is the mismatch behind
[hashicorp/terraform-provider-aws#43333](https://github.com/hashicorp/terraform-provider-aws/issues/43333).

Discovery runs `ImportResourceState` first and `ReadResource` on an id stub
only when the importer refuses the id. Providers built on the plugin framework
read by the attributes the importer fills in, not by `id`, so the other order
reports a resource as absent when it exists. The `timeouts` block is left out
of every comparison, as Terraform does.

## Run

```sh
go build -o agentcore ./experiments/agentcore/cmd/agentcore
export MINIO_ENDPOINT=localhost:9000 MINIO_USER=minio MINIO_PASSWORD=minio123
echo '{"type":"minio_s3_bucket","id":"demo","attributes":{"bucket":"demo"}}' | ./agentcore converge
echo '{"type":"minio_s3_bucket","id":"demo"}' | ./agentcore delete
```

The tests that do not need a server run with `go test ./experiments/agentcore/`.
The tests that create buckets run when `MINIO_ENDPOINT` is set.
