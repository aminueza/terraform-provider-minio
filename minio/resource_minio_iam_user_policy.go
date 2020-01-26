package minio

import (
	b64 "encoding/base64"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"
)

func resourceMinioIAMUserPolicy() *schema.Resource {
	return &schema.Resource{
		Create: minioCreateUserPolicy,
		Read:   minioReadUserPolicy,
		Delete: minioDeleteUserPolicy,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"policy_name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateIAMNamePolicy,
			},
			"user_name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateMinioIamUserName,
			},
		},
	}
}

func minioCreateUserPolicy(d *schema.ResourceData, meta interface{}) error {

	var userName = d.Get("user_name").(string)
	var policyName = d.Get("policy_name").(string)
	minioAdmin := meta.(*S3MinioClient).S3Admin
	err := minioAdmin.SetPolicy(policyName, userName, false)
	if err != nil {
		return NewResourceError("Unable to Set User policy", userName+" "+policyName, err)
	}
	id := minioUserPolicyToStateId(userName, policyName)

	d.SetId(id)

	return err
}

func minioReadUserPolicy(d *schema.ResourceData, meta interface{}) error {
	minioAdmin := meta.(*S3MinioClient).S3Admin

	username, _, err := minioUserPolicyFromStateId(d.Id())
	if err != nil {
		return NewResourceError("Unable to Convert policy State Id", d.Id(), err)
	}
	userInfo, errUser := minioAdmin.GetUserInfo(username)
	if errUser != nil {
		return NewResourceError("Fail to load get user Infos", username, errUser)
	}
	if err := d.Set("policy_name", string(userInfo.PolicyName)); err != nil {
		return err
	}

	return nil
}

func minioDeleteUserPolicy(d *schema.ResourceData, meta interface{}) error {
	minioAdmin := meta.(*S3MinioClient).S3Admin
	username, policyName, err := minioUserPolicyFromStateId(d.Id())
	if err != nil {
		return NewResourceError("Unable to Convert policy State Id", d.Id(), err)
	}

	errIam := minioAdmin.SetPolicy("", username, false)
	if errIam != nil {
		return NewResourceError("Unable to delet user policy", username+" "+policyName, errIam)
	}

	return nil
}

func minioUserPolicyToStateId(userName string, policyName string) string {

	base64UserName := b64.StdEncoding.EncodeToString([]byte(userName))
	base64PolicyName := b64.StdEncoding.EncodeToString([]byte(policyName))

	return fmt.Sprintf("%s|%s", base64UserName, base64PolicyName)
}

func minioUserPolicyFromStateId(policyUserId string) (string, string, error) {

	sEnc := strings.Split(policyUserId, "|")

	sDecUserName, err := b64.StdEncoding.DecodeString(sEnc[0])

	if err != nil {
		return "", "", err
	}
	sDecPolicyName, err := b64.StdEncoding.DecodeString(sEnc[1])
	if err != nil {
		return "", "", err
	}
	return string(sDecUserName), string(sDecPolicyName), err
}
