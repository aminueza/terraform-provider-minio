# Documentation Workflow & Pull Request Guidelines

How documentation is generated in this repository, and how pull requests are
written. The short version of the mandatory rules is in the repository root
`AGENTS.md`.

## Documentation

- Edit templates in `templates/`, not files in `docs/`.
- Run `task generate-docs` after template changes.
- Include example usage in each resource template.
- Keep examples referenced by templates in `examples/resources/<resource_name>/resource.tf` (used via `{{ tffile ... }}`).
- For LDAP policy attachment resources, the import ID format is `<distinguished-name>/<policy-name>` (DNs often contain commas, so quote the import string).
- LDAP resources attach policies only; MinIO LDAP configuration itself must be done outside Terraform (e.g. `mc admin config`).

**Template structure:** templates are `*.md.tmpl` files with frontmatter and a standard layout.

````tmpl
---
page_title: "{{.Name}} {{.Type}} - {{.ProviderName}}"
subcategory: ""
description: |-
{{ .Description | plainmarkdown | trimspace | prefixlines "  " }}
---

# {{.Name}} ({{.Type}})

{{ .Description | trimspace }}

## Example Usage

```terraform
{{ tffile "examples/resources/<resource_name>/resource.tf" }}
```

{{ .SchemaMarkdown | trimspace }}

## Import

...

````

**Template rules:**

- Put the user-facing documentation in the resource/data source `Description` in Go; `SchemaMarkdown` renders the schema.
- Put longer Terraform configuration examples in `examples/` and reference them from the template.
- Don't manually edit generated docs; regenerate them.

## Commit Guidelines

**Commit messages:**

- Use **Conventional Commits** format: `type(scope): description`
- Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`
- Examples:
  - `feat(ilm): add support for abort-only rules`
  - `fix(bucket): handle missing lifecycle configuration`
  - `docs(s3): update object lock examples`

**Important:** Do NOT add yourself as a co-author of the commit.

## Pull Request Guidelines

- Follow the template in `.github/PULL_REQUEST_TEMPLATE.md`
- Reference related issues (`Resolves #123`)
- Provide clear description of changes
- Ensure all tests pass
- Update documentation templates if adding/changing attributes
- Run `task lint` and fix any issues
