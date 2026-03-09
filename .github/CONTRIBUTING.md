# Contributing to Terraform Provider for MinIO

Thank you for considering contributing to the Terraform Provider for MinIO! This document provides guidelines and information for contributors.

## Table of Contents

- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Testing](#testing)
- [Submitting Changes](#submitting-changes)
- [Code Review Process](#code-review-process)
- [Maintainer Expectations](#maintainer-expectations)
- [Community Guidelines](#community-guidelines)

## Getting Started

### Prerequisites

- [Go](https://golang.org/doc/install) 1.25 or later
- [Docker](https://www.docker.com/get-started) and Docker Compose
- [Task](https://taskfile.dev/docs/installation) (recommended) or Make
- [Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git)

### First Time Setup

1. **Fork the Repository**

   ```bash
   # Fork on GitHub, then clone your fork
   git clone https://github.com/YOUR_USERNAME/terraform-provider-minio.git
   cd terraform-provider-minio
   ```

2. **Add Upstream Remote**

   ```bash
   git remote add upstream https://github.com/aminueza/terraform-provider-minio.git
   ```

3. **Install Dependencies**
   ```bash
   go mod download
   ```

## Development Setup

### Building the Provider

```bash
# Using Task (recommended)
task build

# Or using go directly
go build -o terraform-provider-minio
```

### Local Development

1. **Start Test Environment**

   ```bash
   docker compose up -d
   ```

2. **Run Tests**

   ```bash
   # All tests
   task test

   # Specific test
   TEST_PATTERN=TestAccMinioS3Bucket_basic docker compose run --rm test
   ```

3. **Development Workflow**

   ```bash
   # Create feature branch
   git checkout -b feature/your-feature-name

   # Make changes
   # ... edit files ...

   # Run linting (required before submitting)
   task lint

   # Run tests
   task test
   ```

### Project Structure

```
├── minio/                    # Core provider code
│   ├── provider.go          # Provider definition
│   ├── resource_*.go        # Resource implementations
│   ├── data_source_*.go     # Data source implementations
│   ├── *_test.go           # Acceptance tests
│   ├── error.go            # Error handling utilities
│   ├── utils.go            # Common utilities
│   └── new_client.go       # MinIO client creation
├── examples/               # Example configurations
├── docs/                   # Generated documentation
├── templates/              # Documentation templates
└── .github/               # GitHub workflows and templates
```

## Making Changes

### Code Style

We follow these coding standards:

- **Formatting**: Use `gofmt` (standard Go formatting)
- **Linting**: Configured via `.github/golangci.yml`
- **Error Handling**: Always use `NewResourceError()` from `minio/error.go`
- **Documentation**: Include comments for public functions and complex logic

### Error Handling Pattern

Always use `NewResourceError()` from `minio/error.go` for consistent error handling and diagnostics.

**Correct Pattern:**

```go
func minioCreateBucket(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
    config := BucketConfig(d, meta)
    bucketName := config.MinioBucket

    log.Printf("[DEBUG] Creating bucket: %s", bucketName)

    if err := config.MinioClient.MakeBucket(ctx, bucketName, ""); err != nil {
        return NewResourceError("creating bucket", bucketName, err)
    }

    d.SetId(bucketName)
    log.Printf("[DEBUG] Created bucket: %s", bucketName)

    return minioReadBucket(ctx, d, meta)
}
```

**When setting resource data:**

```go
// ✅ Correct - always check d.Set errors
if err := d.Set("name", bucket.Name); err != nil {
    return NewResourceError("setting name", d.Id(), err)
}

if err := d.Set("region", bucket.Region); err != nil {
    return NewResourceError("setting region", d.Id(), err)
}
```

**Wrong Patterns (never use):**

```go
// ❌ Wrong - bypasses NewResourceError
return diag.FromErr(err)
return diag.Errorf("error: %v", err)

// ❌ Wrong - ignores d.Set errors
d.Set("name", bucket.Name)  // Missing error check!
```

**Function Signature:**

```go
// All CRUD functions must return diag.Diagnostics
func minioCreateX(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics
func minioReadX(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics
func minioUpdateX(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics
func minioDeleteX(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics
```

### Resource Implementation Pattern

```go
func resourceMinioExample() *schema.Resource {
    return &schema.Resource{
        CreateContext: minioCreateExample,
        ReadContext:   minioReadExample,
        UpdateContext: minioUpdateExample,
        DeleteContext: minioDeleteExample,
        Importer: &schema.ResourceImporter{
            StateContext: schema.ImportStatePassthroughContext,
        },
        Schema: map[string]*schema.Schema{
            // Define schema here
        },
    }
}
```

### Adding New Resources

1. **Create Resource File**: `minio/resource_minio_<name>.go`
2. **Implement CRUD Functions**: Create, Read, Update, Delete
3. **Add Acceptance Tests**: `minio/resource_minio_<name>_test.go`
4. **Update Provider Registration**: Add resource to `minio/provider.go`
5. **Add Documentation Template**: Create template in `templates/`
6. **Generate Documentation**: Run `task generate-docs`

## Testing

### Test Types

1. **Unit Tests**: Test individual functions in isolation
2. **Acceptance Tests**: Test against real MinIO instances
3. **Integration Tests**: Test complete workflows

### Running Tests

```bash
# All acceptance tests
docker compose run --rm test

# Specific test pattern
TEST_PATTERN=TestAccMinioS3Bucket docker compose run --rm test

# Unit tests only
go test ./minio/... -v

# With coverage
go test -coverprofile=coverage.out ./minio/...
go tool cover -html=coverage.out
```

### Writing Acceptance Tests

```go
func TestAccMinioExample_basic(t *testing.T) {
    resource.Test(t, resource.TestCase{
        PreCheck:          func() { testAccPreCheck(t) },
        ProviderFactories: testAccProviderFactories,
        CheckDestroy:      testAccCheckExampleDestroy,
        Steps: []resource.TestStep{
            {
                Config: testAccExampleConfig_basic,
                Check: resource.ComposeTestCheckFunc(
                    testAccCheckExampleExists("minio_example.test"),
                    resource.TestCheckResourceAttr("minio_example.test", "name", "test-bucket"),
                ),
            },
        },
    })
}
```

## Submitting Changes

### Before Submitting

1. **Run All Tests**: Ensure all tests pass
2. **Run Linting**: Run `task lint` to check code style and catch issues
3. **Update Documentation**: Add/update docs for new features
4. **Check Style**: Run `task lint` and fix any issues
5. **Sign Commits**: Use `git commit -s` for DCO compliance

### Commit Message Format

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): description

feat(bucket): add support for object lock configuration
fix(iam): handle missing user gracefully
docs(s3): update bucket examples
```

### Pull Request Process

1. **Create Pull Request**: Use descriptive title and description
2. **Fill Template**: Complete the PR template completely
3. **Link Issues**: Reference related issues (e.g., "Fixes #123")
4. **Wait for Review**: Maintainers will review your changes
5. **Address Feedback**: Make requested changes promptly

## Code Review Process

### Review Criteria

- **Functionality**: Does the code work as intended?
- **Testing**: Are tests comprehensive and passing?
- **Documentation**: Is documentation clear and complete?
- **Style**: Does code follow project conventions?
- **Security**: Are there security considerations?

### Review Timeline

- **Initial Review**: Within 7 days of submission
- **Response Time**: Maintainers aim to respond within 3 days
- **Merge Timeline**: Changes are typically merged within 14 days

### Required Approvals

- **Code Changes**: At least one maintainer approval
- **Breaking Changes**: Multiple maintainer approvals
- **Security Changes**: Security team review required

## Maintainer Expectations

### Time Commitment

- **Core Maintainers**: ~5-10 hours per week
- **Review Focus**: Prioritize bugs and security issues
- **Response Time**: Within 7 days for PRs, 48 hours for security issues

### Responsibilities

- **Code Review**: Review submitted pull requests
- **Issue Triage**: Categorize and prioritize issues
- **Releases**: Cut releases and publish documentation
- **Community**: Answer questions and guide contributors

### Decision Making

- **Technical Decisions**: Made by maintainers with relevant expertise
- **Breaking Changes**: Require discussion and consensus
- **Security Issues**: Handled by security team privately

### Communication

- **Public Channels**: Use GitHub issues, PRs, and discussions
- **Private Matters**: Use email for security and personal issues
- **Transparency**: Document decisions and rationale publicly

## Community Guidelines

### Code of Conduct

We follow the [Contributor Covenant Code of Conduct](./CODE_OF_CONDUCT.md). Please read and follow these guidelines.

### Getting Help

- **GitHub Issues**: For bug reports and feature requests
- **GitHub Discussions**: For questions and general discussion
- **Security Issues**: See [./SECURITY.md](./SECURITY.md) for reporting

### Ways to Contribute

1. **Code**: New features, bug fixes, improvements
2. **Documentation**: Guides, examples, API docs
3. **Testing**: Test cases, bug reports, validation
4. **Community**: Answer questions, review PRs, share knowledge

### Recognition

- **Contributors**: Listed in README and release notes
- **Significant Contributions**: Callouts in blog posts and social media
- **Maintainers**: Invited based on consistent, high-quality contributions

## Development Tools

### Useful Commands

```bash
# Development
task build          # Build provider
task install        # Install locally
task test           # Run all tests
task lint           # Run linters
task generate-docs  # Generate documentation

# Docker
docker compose up -d           # Start test environment
docker compose down            # Stop test environment
docker compose logs -f minio   # View logs

# Go
go mod tidy          # Clean dependencies
go vet ./...          # Run go vet
go test -v ./...      # Run tests with verbose output
```

### IDE Configuration

For VS Code users, recommended extensions:

- Go (golang.go)
- Docker (ms-azuretools.vscode-docker)
- Terraform (hashicorp.terraform)

## Resources

- [Project Vision](./VISION.md)
- [Security Policy](./SECURITY.md)
- [Terraform Plugin SDK](https://www.terraform.io/docs/plugin/sdk)
- [MinIO Documentation](https://min.io/docs/minio/linux/operations)
- [Go Documentation](https://golang.org/doc/)

## Questions?

If you have questions about contributing:

1. Check existing [GitHub Discussions](https://github.com/aminueza/terraform-provider-minio/discussions)
2. Search existing [Issues](https://github.com/aminueza/terraform-provider-minio/issues)
3. Create a new Discussion for general questions
4. Open an Issue for specific problems or feature requests

Thank you for contributing to the Terraform Provider for MinIO! 🎉
