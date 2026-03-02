# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| >= 3.0.0| :white_check_mark: |

## Reporting a Vulnerability

The Terraform Provider for MinIO team takes security vulnerabilities seriously. We appreciate your efforts to responsibly disclose your findings.

If you discover a security vulnerability, please **DO NOT** open a public issue.

### How to Report

**Preferred Method:** Use GitHub's [Private Vulnerability Reporting](https://github.com/aminueza/terraform-provider-minio/security/advisories/new)

**Alternative:** Send an email to `security@aminueza.com`

### What to Include

Please include the following information in your report:

- **Vulnerability Type**: What type of vulnerability is it (e.g., buffer overflow, SQL injection, cross-site scripting)
- **Affected Versions**: Which versions of the provider are affected
- **Impact**: What is the impact of the vulnerability (e.g., data exposure, privilege escalation)
- **Reproduction Steps**: Detailed steps to reproduce the vulnerability
- **Proof of Concept**: If possible, include a minimal proof of concept
- **Mitigation**: Any suggested mitigation measures

### Response Timeline

- **Initial Response**: Within 48 hours of receiving your report
- **Detailed Assessment**: Within 7 days with an estimated timeline for fix
- **Public Disclosure**: After a fix is released, typically within 14 days of the initial report

### Security Updates

Security fixes are included in regular releases. We recommend:

1. Using the latest version of the provider
2. Monitoring our [GitHub releases](https://github.com/aminueza/terraform-provider-minio/releases)
3. Subscribing to security advisories on GitHub

## Threat Model

### Project Scope

The Terraform Provider for MinIO is infrastructure as code software that:
- Manages MinIO object storage resources (buckets, objects, IAM policies)
- Interacts with MinIO servers via S3-compatible APIs
- Runs in user environments with their credentials
- Has access to MinIO credentials and configurations

### Trust Boundaries

**Trusted Components:**
- MinIO server endpoints configured by users
- Terraform configuration files
- User-provided credentials and access keys

**Untrusted Inputs:**
- All user-provided configuration values
- External MinIO server responses
- Environment variables

### Security Considerations

**Credential Protection:**
- Provider stores MinIO credentials in Terraform state
- Credentials may be logged in debug output
- State files should be protected appropriately

**Network Security:**
- Provider communicates with MinIO servers over HTTP/HTTPS
- TLS verification can be configured but may be disabled for testing
- No built-in network filtering or validation

**Data Access:**
- Provider has full access to configured MinIO resources
- Can read, modify, and delete any accessible objects or buckets
- Respects MinIO's built-in permission system

### Potential Attack Vectors

**High Risk:**
- Compromised MinIO credentials leading to data access/exfiltration
- Injection attacks through malicious configuration values
- Man-in-the-middle attacks on unencrypted connections

**Medium Risk:**
- Denial of service through resource exhaustion
- Information disclosure through error messages
- State file manipulation

**Low Risk:**
- Resource name enumeration
- Timing attacks

### Security Controls

**Implemented:**
- Input validation for configuration parameters
- TLS certificate validation (when enabled)
- Error handling to prevent information leakage
- Dependency scanning via GitHub Actions

**Recommended:**
- Always use HTTPS connections to MinIO servers
- Rotate MinIO credentials regularly
- Encrypt Terraform state files
- Use least-privilege IAM policies
- Enable audit logging on MinIO servers

## Security Best Practices for Users

1. **Credential Management**
   - Use environment variables or secure credential storage
   - Never hardcode credentials in Terraform files
   - Rotate credentials regularly

2. **Network Security**
   - Always use HTTPS connections to MinIO
   - Consider VPN or private networks for sensitive data
   - Implement proper firewall rules

3. **State Protection**
   - Encrypt Terraform state files
   - Use remote state backends with proper access controls
   - Regularly back up state files

4. **Monitoring**
   - Enable MinIO audit logging
   - Monitor Terraform provider logs
   - Set up alerts for suspicious activities

## Security Updates and Patches

We follow a responsible disclosure process:

1. **Private Fix Development**: Vulnerabilities are fixed privately
2. **Coordinated Disclosure**: Security advisories are published when fixes are available
3. **Credit**: Security researchers are credited in advisories (with permission)

## Security Team

The security team for this project includes:
- Project maintainers with commit access
- Security reviewers from the broader community

For security-related questions not related to vulnerability reports, please use GitHub Discussions.

## Related Resources

- [MinIO Security Documentation](https://min.io/docs/minio/linux/operations/monitor-security.html)
- [Terraform Security Best Practices](https://www.terraform.io/docs/cloud/security/index.html)
- [GitHub Security Advisories](https://docs.github.com/en/code-security/security-advisories/about-github-security-advisories)
