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

## Run

```sh
go build -o agentcore ./experiments/agentcore/cmd/agentcore
export MINIO_ENDPOINT=localhost:9000 MINIO_USER=minio MINIO_PASSWORD=minio123
echo '{"type":"minio_s3_bucket","id":"demo","attributes":{"bucket":"demo"}}' | ./agentcore converge
echo '{"type":"minio_s3_bucket","id":"demo"}' | ./agentcore delete
```

The tests that do not need a server run with `go test ./experiments/agentcore/`.
The tests that create buckets run when `MINIO_ENDPOINT` is set.
