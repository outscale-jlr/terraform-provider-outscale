package outscale

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/terraform-providers/terraform-provider-outscale/osc/eim"
)

func dataSourceOutscaleOAPIUserAPIKeys() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceOutscaleOAPIUserAPIKeysRead,
		Schema: map[string]*schema.Schema{
			"user_name": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"api_key_metadata": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"user_name": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"state": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"api_key_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
						//account_id missing
						//tags missing
						//secret_key
					},
				},
			},
			"request_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func dataSourceOutscaleOAPIUserAPIKeysRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).EIM

	request := &eim.ListAccessKeysInput{
		UserName: aws.String(d.Get("user_name").(string)),
	}

	var err error
	var resp *eim.ListAccessKeysOutput
	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		resp, err = conn.API.ListAccessKeys(request)

		if err != nil {
			if strings.Contains(err.Error(), "Throttling:") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("Failure get access key for EIM: %s", err)
	}

	if resp.ListAccessKeysResult == nil {
		return fmt.Errorf("Cannot unmarshal result of AccessKeys")
	}

	if resp.ListAccessKeysResult == nil || len(resp.ListAccessKeysResult.AccessKeyMetadata) == 0 {
		return fmt.Errorf("no matching access_keys found")
	}

	accessKeyMetaList := make([]map[string]interface{}, len(resp.ListAccessKeysResult.AccessKeyMetadata))
	for i, key := range resp.ListAccessKeysResult.AccessKeyMetadata {
		fmt.Println("ENTRO")
		accessKeyMetaList[i] = dataSourceOutscaleOAPIEIMAccessKeyReadResult(d, key)
	}

	if err := d.Set("access_key_metadata", accessKeyMetaList); err != nil {
		return err
	}

	d.SetId(resource.UniqueId())

	return d.Set("request_id", aws.StringValue(resp.ResponseMetadata.RequestID))
}

func dataSourceOutscaleOAPIEIMAccessKeyReadResult(d *schema.ResourceData, key *eim.AccessKeyMetadata) map[string]interface{} {
	accessKeyMeta := make(map[string]interface{})
	accessKeyMeta["api_key_id"] = aws.StringValue(key.AccessKeyID)
	accessKeyMeta["state"] = aws.StringValue(key.Status)
	accessKeyMeta["user_name"] = aws.StringValue(key.UserName)

	return accessKeyMeta
}
