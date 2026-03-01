## Pull Request Checklist

### General Requirements
- [ ] I have read the [Contributing Guidelines](../CONTRIBUTING.md)
- [ ] I have read the [Project Vision](../VISION.md) and understand the scope
- [ ] My code follows the project's coding standards
- [ ] I have performed a self-review of my own code
- [ ] I have signed my commits (`git commit -s`)

### Testing Requirements
- [ ] I have added tests that prove my fix is effective or that my feature works
- [ ] New and existing unit tests pass locally with my changes
- [ ] Any new or modified resources have acceptance tests
- [ ] I have tested the changes with a real MinIO setup

### Documentation Requirements
- [ ] I have updated the documentation templates in `templates/` if needed
- [ ] I have run `task generate-docs` to update generated documentation
- [ ] I have added examples to the `examples/` directory if applicable
- [ ] I have updated the README if this is a user-facing change

### Breaking Changes
- [ ] If this is a breaking change, I have described the impact and migration path
- [ ] If this is a breaking change, I have updated the version appropriately

## Description

### Type of Change
- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update
- [ ] Refactoring (no functional changes, code improvement)
- [ ] Performance improvement
- [ ] Security fix

### What does this PR do?
<!-- 
Provide a clear and concise description of what this pull request does.
Include the motivation for the change and any relevant context.
-->

### How was this change tested?
<!-- 
Describe how you tested this change. Include:
- Unit tests run
- Acceptance tests run
- Manual testing scenarios
- Any specific configurations used
-->

### Related Issues
<!-- 
Link to any related issues using GitHub syntax:
- Fixes #123
- Related to #456
- Part of #789
-->

## Screenshots / Examples

<!-- 
If this is a UI change or includes new functionality, add screenshots or examples.
For Terraform resources, include example HCL configuration.
-->

### Example Usage (if applicable)

```hcl
# Add example Terraform configuration here
terraform {
  required_providers {
    minio = {
      source = "aminueza/minio"
      version = ">= 3.0.0"
    }
  }
}

provider "minio" {
  minio_server   = var.minio_server
  minio_user     = var.minio_user
  minio_password = var.minio_password
}

# Example resource usage
resource "minio_s3_bucket" "example" {
  bucket = "example-bucket"
  # ... other configuration
}
```

## Checklist for Specific Changes

### For New Resources
- [ ] Resource file created: `minio/resource_minio_<name>.go`
- [ ] Acceptance tests created: `minio/resource_minio_<name>_test.go`
- [ ] Resource registered in `minio/provider.go`
- [ ] Documentation template created in `templates/`
- [ ] Example configuration added to `examples/resources/<name>/`
- [ ] Import functionality implemented and tested

### For New Data Sources
- [ ] Data source file created: `minio/data_source_minio_<name>.go`
- [ ] Acceptance tests created: `minio/data_source_minio_<name>_test.go`
- [ ] Data source registered in `minio/provider.go`
- [ ] Documentation template created in `templates/`
- [ ] Example configuration added to `examples/data-sources/<name>/`

### For Bug Fixes
- [ ] Root cause identified and documented
- [ ] Regression tests added
- [ ] Error handling follows project patterns using `NewResourceError()`
- [ ] Edge cases considered and handled

### For Security Changes
- [ ] Security implications documented
- [ ] Input validation added if needed
- [ ] Credential handling reviewed
- [ ] Security tests added if applicable

## Additional Context

<!-- 
Add any other context about the problem here.
Include:
- Performance considerations
- Security considerations
- Backward compatibility notes
- Alternative approaches considered
- References to MinIO documentation
-->

## Reviewer Focus Areas

<!-- 
Please highlight specific areas you'd like reviewers to focus on:
- Specific complex logic
- Security implications
- Performance considerations
- Documentation accuracy
- Test coverage
-->

### Areas of Concern
- [ ] Error handling and edge cases
- [ ] Performance implications
- [ ] Security considerations
- [ ] Documentation accuracy
- [ ] Test coverage completeness

## Release Notes

<!-- 
If this change should be included in release notes, describe what users should know:
-->

```markdown
### Added
- New feature or resource description

### Fixed
- Bug fix description

### Changed
- Breaking change description with migration guide

### Deprecated
- Feature deprecation notice

### Security
- Security fix description
```

## Additional Information

<!-- 
Any additional information that reviewers should know:
- Dependencies on other changes
- Coordination with other teams/projects
- Timeline considerations
- Rollback plans if needed
-->

---

Thank you for contributing to the Terraform Provider for MinIO! 🎉
