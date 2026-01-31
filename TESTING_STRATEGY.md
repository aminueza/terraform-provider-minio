# Testing Strategy for Untested Resources

## 🚨 CRITICAL: Production Resources Without Tests

**Current Status:** 8 out of 27 resources (30%) have NO automated tests

This represents a significant production risk:
- Changes can break functionality without detection
- No regression protection
- Manual testing required for every change
- Difficult to refactor safely

---

## 📊 Untested Resources (Priority Order)

### HIGH PRIORITY (Core IAM Features)

#### 1. `resource_minio_iam_user_policy_attachment.go`
**Risk:** HIGH - Core IAM functionality
**Complexity:** LOW - Simple attachment logic
**Lines:** ~100
**Dependencies:** IAM User, IAM Policy
**Estimated Test Effort:** 2 hours

**Test Scenarios:**
- ✅ Attach single policy to user
- ✅ Attach multiple policies to user
- ✅ Update policy attachment
- ✅ Remove policy attachment
- ✅ Handle non-existent user (error case)
- ✅ Handle non-existent policy (error case)
- ✅ LDAP user detection and handling

#### 2. `resource_minio_iam_group_policy_attachment.go`
**Risk:** HIGH - Core IAM functionality
**Complexity:** LOW - Similar to user attachment
**Lines:** ~100
**Dependencies:** IAM Group, IAM Policy
**Estimated Test Effort:** 2 hours

**Test Scenarios:**
- ✅ Attach single policy to group
- ✅ Attach multiple policies to group
- ✅ Update policy attachment
- ✅ Remove policy attachment
- ✅ Handle non-existent group (error case)
- ✅ Handle non-existent policy (error case)

#### 3. `resource_minio_iam_group_user_attachment.go`
**Risk:** HIGH - User-group membership
**Complexity:** LOW
**Lines:** ~80
**Dependencies:** IAM User, IAM Group
**Estimated Test Effort:** 2 hours

**Test Scenarios:**
- ✅ Add user to group
- ✅ Remove user from group
- ✅ Update group membership
- ✅ Handle non-existent user (error case)
- ✅ Handle non-existent group (error case)
- ✅ Multiple users in same group

### MEDIUM PRIORITY (LDAP Integration)

#### 4. `resource_minio_iam_ldap_user_policy_attachment.go`
**Risk:** MEDIUM - LDAP-specific feature
**Complexity:** MEDIUM - Requires LDAP setup
**Lines:** ~120
**Dependencies:** LDAP configuration, IAM Policy
**Estimated Test Effort:** 4 hours

**Test Scenarios:**
- ✅ Attach policy to LDAP user DN
- ✅ Handle invalid DN format
- ✅ Update LDAP user policy
- ✅ Remove LDAP user policy
**Note:** May require LDAP mock or separate LDAP test environment

#### 5. `resource_minio_iam_ldap_group_policy_attachment.go`
**Risk:** MEDIUM - LDAP-specific feature
**Complexity:** MEDIUM - Requires LDAP setup
**Lines:** ~120
**Dependencies:** LDAP configuration, IAM Policy
**Estimated Test Effort:** 4 hours

**Test Scenarios:**
- ✅ Attach policy to LDAP group DN
- ✅ Handle invalid DN format
- ✅ Update LDAP group policy
- ✅ Remove LDAP group policy
**Note:** May require LDAP mock or separate LDAP test environment

### MEDIUM PRIORITY (ILM & Storage)

#### 6. `resource_minio_ilm_tier.go`
**Risk:** MEDIUM - Data lifecycle management
**Complexity:** MEDIUM - Requires tier backend setup
**Lines:** ~250
**Dependencies:** S3 or Azure backend for tiering
**Estimated Test Effort:** 6 hours

**Test Scenarios:**
- ✅ Create S3 tier
- ✅ Create Azure tier
- ✅ Update tier configuration
- ✅ Delete tier
- ✅ Handle invalid credentials (error case)
- ✅ Validate tier connectivity
**Note:** May require mock S3/Azure endpoint

### LOW PRIORITY (Advanced Features)

#### 7. `resource_minio_kms_key.go`
**Risk:** LOW - KMS is optional feature
**Complexity:** HIGH - Requires KMS setup
**Lines:** ~180
**Dependencies:** External KMS (Vault, AWS KMS, etc.)
**Estimated Test Effort:** 8 hours

**Test Scenarios:**
- ✅ Create KMS key
- ✅ Import existing KMS key
- ✅ Update key policy
- ✅ Delete KMS key
- ✅ Key rotation
**Note:** Requires KMS backend, may need extensive mocking

#### 8. `resource_minio_s3_bucket_server_side_encryption_configuration.go`
**Risk:** LOW - Encryption config (not SSE itself)
**Complexity:** MEDIUM - Depends on KMS
**Lines:** ~200
**Dependencies:** Bucket, optionally KMS
**Estimated Test Effort:** 4 hours

**Test Scenarios:**
- ✅ Configure SSE-S3
- ✅ Configure SSE-KMS with key
- ✅ Update encryption config
- ✅ Remove encryption config
- ✅ Handle invalid KMS key (error case)

---

## 📋 Testing Implementation Plan

### Phase 1: Core IAM Tests (Week 1 - Priority 1)
**Duration:** 3 days
**Resources:** 1-3 (Policy & Group Attachments)

```bash
# Create test files
touch minio/resource_minio_iam_user_policy_attachment_test.go
touch minio/resource_minio_iam_group_policy_attachment_test.go
touch minio/resource_minio_iam_group_user_attachment_test.go
```

**Template Structure:**
```go
func TestAccMinioIAMUserPolicyAttachment_basic(t *testing.T) {
    userName := "test-user-" + acctest.RandString(8)
    policyName := "test-policy-" + acctest.RandString(8)

    resource.ParallelTest(t, resource.TestCase{
        PreCheck:          func() { testAccPreCheck(t) },
        ProviderFactories: testAccProviders,
        Steps: []resource.TestStep{
            {
                Config: testAccUserPolicyAttachmentConfig(userName, policyName),
                Check: resource.ComposeTestCheckFunc(
                    resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.test", "user_name", userName),
                    resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.test", "policy_name", policyName),
                ),
            },
        },
    })
}
```

### Phase 2: LDAP Tests (Week 2 - Priority 2)
**Duration:** 4 days
**Resources:** 4-5 (LDAP Attachments)

**Decision Required:**
- Option A: Mock LDAP responses (faster, less setup)
- Option B: Real LDAP container in docker-compose (more realistic)
- **Recommendation:** Start with Option A, add Option B later

### Phase 3: ILM & Storage Tests (Week 3 - Priority 3)
**Duration:** 5 days
**Resources:** 6 (ILM Tier)

**Setup Required:**
- Add MinIO tier backend to docker-compose
- Or mock tier endpoint responses

### Phase 4: Advanced Features (Week 4 - Priority 4)
**Duration:** 5 days
**Resources:** 7-8 (KMS, SSE)

**Setup Required:**
- Vault container for KMS testing (or mock)
- KMS key configuration

---

## 🔧 Test Template (Copy-Paste Ready)

### For Policy Attachment Resources

```go
package minio

import (
    "fmt"
    "testing"

    "github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
    "github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccMinioIAMUserPolicyAttachment_basic(t *testing.T) {
    userName := "tfacc-user-" + acctest.RandString(8)
    policyName := "tfacc-policy-" + acctest.RandString(8)

    resource.ParallelTest(t, resource.TestCase{
        PreCheck:          func() { testAccPreCheck(t) },
        ProviderFactories: testAccProviders,
        CheckDestroy:      testAccCheckMinioIAMUserPolicyAttachmentDestroy,
        Steps: []resource.TestStep{
            {
                Config: testAccMinioIAMUserPolicyAttachmentConfig(userName, policyName),
                Check: resource.ComposeTestCheckFunc(
                    resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.test", "user_name", userName),
                    resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.test", "policy_name", policyName),
                ),
            },
            // Test import
            {
                ResourceName:      "minio_iam_user_policy_attachment.test",
                ImportState:       true,
                ImportStateVerify: true,
            },
        },
    })
}

func testAccMinioIAMUserPolicyAttachmentConfig(userName, policyName string) string {
    return fmt.Sprintf(`
resource "minio_iam_user" "test" {
  name   = %[1]q
  secret = "Test123456"
}

resource "minio_iam_policy" "test" {
  name   = %[2]q
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject"]
      Resource = ["arn:aws:s3:::*"]
    }]
  })
}

resource "minio_iam_user_policy_attachment" "test" {
  user_name   = minio_iam_user.test.name
  policy_name = minio_iam_policy.test.name
}
`, userName, policyName)
}

func testAccCheckMinioIAMUserPolicyAttachmentDestroy(s *terraform.State) error {
    // Verify attachment is removed
    return nil
}
```

---

## 🎯 Quick Start: Add First Test Today

**1. Create test file:**
```bash
cp minio/resource_minio_iam_user_test.go minio/resource_minio_iam_user_policy_attachment_test.go
```

**2. Modify test:**
- Change test function names
- Update resource names
- Add policy attachment configuration

**3. Run test:**
```bash
./test-health-status.sh  # Use existing test script with new test name
```

**4. Commit:**
```bash
git add minio/resource_minio_iam_user_policy_attachment_test.go
git commit -m "test: add tests for IAM user policy attachment"
```

---

## 📈 Success Metrics

**Target:** 100% test coverage for all resources by end of Month 1

| Week | Resources Tested | Cumulative Coverage | Target |
|------|------------------|---------------------|---------|
| 0 (Current) | 19/27 | 70% | - |
| 1 | +3 (IAM) | 81% | 80% |
| 2 | +2 (LDAP) | 89% | 85% |
| 3 | +1 (ILM) | 93% | 90% |
| 4 | +2 (KMS/SSE) | 100% | 100% ✅ |

---

## 🚀 Immediate Action Items

### Today (Priority 1)
1. Create `resource_minio_iam_user_policy_attachment_test.go`
2. Run test to verify infrastructure
3. Commit and push

### This Week (Priority 1)
1. Complete all 3 IAM attachment tests
2. Verify tests pass in CI/CD
3. Document any infrastructure needs

### Next Week (Priority 2)
1. Decide on LDAP testing strategy
2. Implement LDAP tests or mocks
3. Add ILM tier tests

---

## 🔍 Testing Best Practices

1. **Always test these scenarios:**
   - ✅ Basic CRUD operations
   - ✅ Update/modification
   - ✅ Import/export
   - ✅ Concurrent operations (if applicable)
   - ✅ Error cases (non-existent resources)
   - ✅ Edge cases (empty values, special characters)

2. **Use consistent naming:**
   - Test resources: `test-resource-{random}`
   - Test functions: `TestAccMinio{Resource}_{scenario}`

3. **Clean up resources:**
   - Use `CheckDestroy` to verify cleanup
   - Ensure tests don't leak resources

4. **Document dependencies:**
   - List required resources in comments
   - Note any special setup (LDAP, KMS, etc.)

---

## ⚠️ Risk Assessment

**Without Tests:**
- ❌ Breaking changes go undetected
- ❌ Refactoring is dangerous
- ❌ Bug fixes can't be verified
- ❌ CI/CD can't catch regressions
- ❌ Community contributions harder to review

**With Tests:**
- ✅ Confidence in changes
- ✅ Safe refactoring
- ✅ Regression protection
- ✅ Better documentation through tests
- ✅ Easier code review

---

## 📞 Questions & Support

**Q: Do LDAP tests require real LDAP server?**
A: Start with mocked responses, add real LDAP tests later for integration testing.

**Q: How to test KMS without external KMS?**
A: Use MinIO's built-in KMS or mock KMS responses.

**Q: What if test setup is too complex?**
A: Document the limitation, add unit tests for logic, skip acceptance test temporarily.

**Q: How to prioritize which tests to write first?**
A: Follow priority order above - IAM core features first, advanced features last.
