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

## Run

```sh
go build -o agentcore ./experiments/agentcore/cmd/agentcore
export MINIO_ENDPOINT=localhost:9000 MINIO_USER=minio MINIO_PASSWORD=minio123
echo '{"type":"minio_s3_bucket","id":"demo","attributes":{"bucket":"demo"}}' | ./agentcore converge
echo '{"type":"minio_s3_bucket","id":"demo"}' | ./agentcore delete
```

The tests that do not need a server run with `go test ./experiments/agentcore/`.
The tests that create buckets run when `MINIO_ENDPOINT` is set.
