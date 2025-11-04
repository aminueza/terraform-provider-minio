<p align="center">
  <a href="https://github.com/aminueza/terraform-provider-minio">
    <img src="https://i.imgur.com/yijdDec.png" alt="minio-provider-terraform" width="200">
  </a>
  <h3 align="center" style="font-weight: bold"><a href="https://developer.hashicorp.com/terraform">Terraform Provider</a> for <a href="https://min.io">MinIO</a></h3>
  <p align="center">
    <a href="https://github.com/aminueza/terraform-provider-minio/graphs/contributors">
      <img alt="Contributors" src="https://img.shields.io/github/contributors/aminueza/terraform-provider-minio">
    </a>
    <a href="https://golang.org/doc/devel/release.html">
      <img alt="GitHub go.mod Go version" src="https://img.shields.io/github/go-mod/go-version/aminueza/terraform-provider-minio">
    </a>
    <a href="https://github.com/aminueza/terraform-provider-minio/actions?query=workflow%3A%22Terraform+Provider+CI%22">
      <img alt="GitHub Workflow Status" src="https://img.shields.io/github/actions/workflow/status/aminueza/terraform-provider-minio/go.yml?branch=main">
    </a>
    <a href="https://github.com/aminueza/terraform-provider-minio/releases/latest">
      <img alt="Latest release on GitHub" src="https://img.shields.io/github/v/release/aminueza/terraform-provider-minio">
    </a>
  </p>
  <p align="center">
    <a href="https://github.com/aminueza/terraform-provider-minio/tree/main/docs"><strong>Explore the docs »</strong></a>
  </p>
</p>

## Building and Installing

Prebuilt versions of this provider are available on the [Releases page](https://github.com/aminueza/terraform-provider-minio/releases).

But if you need to build it yourself, just download this repository, [install Task](https://taskfile.dev/docs/installation), then run the following command to build and install the plugin in the correct folder (resolved automatically based on the current Operating System):

```sh
task install
```

## Examples

Explore the [examples](./examples/) folder for more usage scenarios.

To get started quickly, you can use the configuration from [examples/user/main.tf](./examples/user/main.tf) as shown below:

```hcl
terraform {
  required_providers {
    minio = {
      source  = "aminueza/minio"
      version = ">= 3.0.0"
    }
  }
}

provider "minio" {
  minio_server   = var.minio_server
  minio_region   = var.minio_region
  minio_user     = var.minio_user
  minio_password = var.minio_password
}
```

You may use variables to configure your provider (as in the example):

```hcl
variable "minio_region" {
  description = "Default MINIO region"
  default     = "us-east-1"
}

variable "minio_server" {
  description = "Default MINIO host and port"
  default     = "localhost:9000"
}

variable "minio_user" {
  description = "MINIO user"
  default     = "minio"
}

variable "minio_password" {
  description = "MINIO password"
  default     = "minio123"
}
```

## Testing

For testing locally, run the docker compose to spin up a minio server:

```sh
docker compose up
```

To run the acceptance tests:

```sh
docker compose run --rm test
```

Run a specific test by name pattern:

```sh
TEST_PATTERN=TestAccAWSUser_SettingAccessKey docker compose run --rm test
```

## Accessing the MinIO Console

After running `docker compose up`, you can access the MinIO Console (the web UI) for each MinIO instance:

- Main MinIO: [http://localhost:9001](http://localhost:9001)
- Second MinIO: [http://localhost:9003](http://localhost:9003)
- Third MinIO: [http://localhost:9005](http://localhost:9005)
- Fourth MinIO: [http://localhost:9007](http://localhost:9007)

**Login credentials** are set in your `docker-compose.yml` for each service. For example, for the main MinIO instance:

- Username: `minio`
- Password: `minio123`

For the other instances, use the corresponding `MINIO_ROOT_PASSWORD` (e.g., `minio321`, `minio456`, `minio654`).

## Roadmap

See the [open issues](https://github.com/aminueza/terraform-provider-minio/issues) for a list of proposed features (and known issues). See [CONTRIBUTING](./.github/CONTRIBUTING.md) for more information.

## License

All versions of this provider starting from v2.0.0 are distributed under the AGPL-3.0 License. See [LICENSE](./LICENSE) for more information.
