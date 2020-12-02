<p align="center">
  <a href="https://github.com/aminueza/terraform-provider-minio">
    <img src="https://i.imgur.com/yijdDec.png" alt="minio-provider-terraform" width="200">
  </a>
  <h3 align="center" style="font-weight: bold">Terraform Provider for MinIO</h3>
  <p align="center">
    <a href="https://github.com/aminueza/terraform-provider-minio/graphs/contributors">
      <img alt="Contributors" src="https://img.shields.io/github/contributors/aminueza/terraform-provider-minio">
    </a>
    <a href="https://golang.org/doc/devel/release.html">
      <img alt="GitHub go.mod Go version" src="https://img.shields.io/github/go-mod/go-version/aminueza/terraform-provider-minio">
    </a>
    <a href="https://gitpod.io/#https://github.com/aminueza/terraform-provider-minio">
      <img alt="Gitpod Ready-to-Code" src="https://img.shields.io/badge/Gitpod-Ready--to--Code-blue?logo=gitpod">
    </a>
    <a href="https://github.com/aminueza/terraform-provider-minio/actions?query=workflow%3A%22Terraform+Provider+CI%22">
      <img alt="GitHub Workflow Status" src="https://img.shields.io/github/workflow/status/aminueza/terraform-provider-minio/Terraform%20Provider%20CI">
    </a>
    <a href="https://github.com/aminueza/terraform-provider-minio/releases">
      <img alt="GitHub release (latest by date including pre-releases)" src="https://img.shields.io/github/v/release/aminueza/terraform-provider-minio?include_prereleases">
    </a>
  </p>
  <p align="center">
    <a href="https://github.com/aminueza/terraform-provider-minio/tree/master/docs"><strong>Explore the docs »</strong></a>
  </p>
</p>

## Table of Contents

- [Table of Contents](#table-of-contents)
- [About this project](#about-this-project)
- [Requirements](#requirements)
- [Building and Installing](#building-and-installing)
- [Examples](#examples)
- [Testing](#testing)
- [Usage](#usage)
- [Developing inside a container](#developing-inside-a-container)
- [Roadmap](#roadmap)
- [License](#license)
- [Acknowledgements](#acknowledgements)

## About this project

A [Terraform](https://www.terraform.io) provider to manage [MinIO Cloud Storage](https://min.io).

Made with <span style="color: #e25555;">&#9829;</span> using [Go](https://golang.org/).

## Requirements

- Go v1.13 or higher;
- Terraform v0.13.5 or higher;

## Building and Installing

Prebuilt versions of this provider are available for MacOS and Linux on the [releases page](https://github.com/aminueza/terraform-provider-minio/releases/latest).

But if you need to build it yourself, just download this repository and follow the instructions:

### MacOS

To build and install this plugin on MacOS, run:
```
make install_mac
```

### Linux

To build and install this plugin on Linux, run:
```
make install_linux
```

### Windows

For Windows we don't have a Makefile, but it should be as simple as:
```
go build -o terraform-provider-minio_v1.0.0
mkdir "%APPDATA%\HashiCorp\Terraform\plugins\registry.terraform.io\aminueza\minio\1.0.0\windows_amd64"
move terraform-provider-minio_v1.0.0 %APPDATA%\HashiCorp\Terraform\plugins\registry.terraform.io\aminueza\minio\1.0.0\windows_amd64
```

## Examples

Use [examples/main.tf](./examples/user/main.tf) to create some test config, such as:

```hcl
provider "minio" {
  minio_server = "localhost:9000"
  minio_region = "us-east-1"
  minio_access_key = "minio"
  minio_secret_key = "minio123"
}
```

You may use variables to fill up configurations:

```hcl
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

## Developing inside a container

Inside `.devcontainer` folder is the configuration of a Docker Container with all tools needed to develop this project. It's meant to be used with [VS Code](https://code.visualstudio.com), requiring only the installation of [Remote - Containers](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers) extension. For usage instructions, refer to [this tutorial](https://code.visualstudio.com/docs/remote/containers).

## Roadmap

See the [open issues](https://github.com/aminueza/terraform-provider-minio/issues) for a list of proposed features (and known issues). See [CONTRIBUTION.md](./docs/github/CONTRIBUTING.md) for more information.

## License

Distributed under the Apache License. See [LICENSE](./LICENSE) for more information.

## Acknowledgements

- [HashiCorp Terraform](https://www.hashicorp.com/products/terraform)
- [MinIO](https://min.io)
- [Best Readme](https://github.com/othneildrew/Best-README-Template)
