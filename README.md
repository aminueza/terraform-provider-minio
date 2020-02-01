<p align="center">
  <a href="https://github.com/aminueza/terraform-provider-minio">
    <img src="https://i.imgur.com/yijdDec.png" alt="minio-provider-terraform" width="200">
  </a>
  <h3 align="center" style="font-weight: bold">Terraform Provider for MinIO</h3>
  <p align="center">
    <img alt="GitHub contributors" src="https://img.shields.io/github/contributors/aminueza/terraform-provider-minio">
    <img alt="GitHub go.mod Go version" src="https://img.shields.io/github/go-mod/go-version/aminueza/terraform-provider-minio">
    <img alt="GitHub Workflow" src="https://github.com/aminueza/terraform-provider-minio/workflows/.github/workflows/go.yml/badge.svg">
    <img alt="GitHub release (latest by date including pre-releases)" src="https://img.shields.io/github/v/release/aminueza/terraform-provider-minio?include_prereleases">
  </p>
  <p align="center">
    <a href="https://github.com/aminueza/terraform-provider-minio/tree/master/docs"><strong>Explore the docs »</strong></a>
  </p>
</p>

## Table of Contents

- [Table of Contents](#table-of-contents)
- [About this project](#about-this-project)
- [Requirements](#requirements)
- [Installing the plugin](#installing-the-plugin)
- [Examples](#examples)
- [Testing](#testing)
- [Usage](#usage)
- [Roadmap](#roadmap)
- [License](#license)
- [Acknowledgement](#acknowledgement)

## About this project

A [Terraform](https://www.terraform.io) provider to manage [MinIO Cloud Storages](https://min.io).

Made with <span style="color: #e25555;">&#9829;</span> using [Go](https://golang.org/).

## Requirements

- Go v1.31 or higher;
- Terraform v0.12.17 or higher;
- Docker v19.03.4 or higher for testing minio;
- Govendor for dependencies.

## Installing the plugin

We release darwin and linux amd64 packages on the releases page. Once you have the plugin you should remove the _os_arch from the end of the file name and place it in `~/.terraform.d/plugins` which is where terraform init will look for plugins. To install release binaries, download the version from your OS, then:

```sh
mv terraform-provider-minio_v1.0_darwin_amd64 ~/.terraform.d/plugins/terraform-provider-minio_v1.0
```

If you require a different architecture you will need to build the plugin from source, see below for more details:

```sh
make build
```

Valid provider filenames are `terraform-provider-NAME_X.X.X` or `terraform-provider-NAME_vX.X.X`

## Examples

Use [examples/main.tf](./examples/main.tf) to create some test config, such as:

```go
provider "minio" {
  minio_server = "localhost:9000"
  minio_region = "us-east-1"
  minio_access_key = "minio"
  minio_secret_key = "minio123"
}
```

You may use variables to fill up configurations:

```go
variable "minio_region" {
  description = "Default MINIO region"
  default     = "us-east-1"
}

variable "minio_server" {
  description = "Default MINIO host and port"
  default = "localhost:9000"
}

variable "minio_access_key" {
  description = "MINIO user"
  default = "minio"
}

variable "minio_secret_key" {
  description = "MINIO secret user"
  default = "minio123"
}
```

## Testing

For testing locally, run the docker compose to spin up a minio server:

```sh
docker-compose up
```

## Usage

See our [Examples](examples/) folder.

## Roadmap

See the [open issues](https://github.com/aminueza/terraform-minio-provider/issues) for a list of proposed features (and known issues). See [CONTRIBUTION.md](./docs/github/CONTRIBUTING.md) for more information.

## License

Distributed under the Apache License. See `LICENSE` for more information.

## Acknowledgement

- [Hashicorp](https://www.hashicorp.com)
- [Best Readme](https://github.com/othneildrew/Best-README-Template)
- [MinIO](https://min.io)
