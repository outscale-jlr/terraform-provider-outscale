package outscale

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/terraform-providers/terraform-provider-outscale/osc/fcu"
)

func resourcedOutscaleSnapshotAttributes() *schema.Resource {
	return &schema.Resource{
		Exists: resourcedOutscaleSnapshotAttributesExists,
		Create: resourcedOutscaleSnapshotAttributesCreate,
		Read:   resourcedOutscaleSnapshotAttributesRead,
		Delete: resourcedOutscaleSnapshotAttributesDelete,

		Schema: map[string]*schema.Schema{
			"snapshot_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"create_volume_permission_add": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"group": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"user_id": {
							Type:     schema.TypeString,
							Optional: true,
						},
					},
				},
			},
			"create_volume_permissions": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"group": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"user_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"account_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"request_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourcedOutscaleSnapshotAttributesExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	conn := meta.(*OutscaleClient).FCU

	sid := d.Get("snapshot_id").(string)
	aid := d.Get("account_id").(string)
	return hasCreateVolumePermission(conn, sid, aid)
}

func resourcedOutscaleSnapshotAttributesCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).FCU

	sid := d.Get("snapshot_id").(string)
	aid := ""

	req := &fcu.ModifySnapshotAttributeInput{
		SnapshotId: aws.String(sid),
		Attribute:  aws.String("createVolumePermission"),
	}

	if v, ok := d.GetOk("create_volume_permission_add"); ok {
		add := v.([]interface{})
		if len(add) > 0 {
			a := make([]*fcu.CreateVolumePermission, len(add))

			for k, v1 := range v.([]interface{}) {
				data := v1.(map[string]interface{})
				a[k] = &fcu.CreateVolumePermission{
					UserId: aws.String(data["user_id"].(string)),
					Group:  aws.String(data["group"].(string)),
				}
				aid = data["user_id"].(string)
			}
			req.CreateVolumePermission = &fcu.CreateVolumePermissionModifications{Add: a}
		}
	}

	var err error
	err = resource.Retry(2*time.Minute, func() *resource.RetryError {
		_, err = conn.VM.ModifySnapshotAttribute(req)
		if err != nil {
			if strings.Contains(fmt.Sprint(err), "RequestLimitExceeded") {
				log.Printf("[DEBUG] Error: %q", err)
				return resource.RetryableError(err)
			}

			return resource.NonRetryableError(err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("Error adding snapshot createVolumePermission: %s", err)
	}

	d.SetId(fmt.Sprintf("%s-%s", sid, aid))
	d.Set("account_id", aid)
	d.Set("create_volume_permissions", make([]map[string]interface{}, 0))

	// Wait for the account to appear in the permission list
	stateConf := &resource.StateChangeConf{
		Pending:    []string{"denied"},
		Target:     []string{"granted"},
		Refresh:    resourcedOutscaleSnapshotAttributesStateRefreshFunc(conn, sid, aid),
		Timeout:    5 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 10 * time.Second,
	}
	if _, err := stateConf.WaitForState(); err != nil {
		return fmt.Errorf(
			"Error waiting for snapshot createVolumePermission (%s) to be added: %s",
			d.Id(), err)
	}

	return resourcedOutscaleSnapshotAttributesRead(d, meta)
}

func resourcedOutscaleSnapshotAttributesRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).FCU
	sid := d.Get("snapshot_id").(string)

	var attrs *fcu.DescribeSnapshotAttributeOutput
	var err error
	err = resource.Retry(2*time.Minute, func() *resource.RetryError {
		attrs, err = conn.VM.DescribeSnapshotAttribute(&fcu.DescribeSnapshotAttributeInput{
			SnapshotId: aws.String(sid),
			Attribute:  aws.String("createVolumePermission"),
		})
		if err != nil {
			if strings.Contains(fmt.Sprint(err), "RequestLimitExceeded") {
				log.Printf("[DEBUG] Error: %q", err)
				return resource.RetryableError(err)
			}

			return resource.NonRetryableError(err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("Error refreshing snapshot createVolumePermission state: %s", err)
	}

	cvp := make([]map[string]interface{}, len(attrs.CreateVolumePermissions))
	for k, v := range attrs.CreateVolumePermissions {
		c := make(map[string]interface{})
		c["group"] = aws.StringValue(v.Group)
		c["user_id"] = aws.StringValue(v.UserId)
		cvp[k] = c
	}

	d.Set("request_id", aws.StringValue(attrs.RequestId))

	return d.Set("create_volume_permissions", cvp)
}

func resourcedOutscaleSnapshotAttributesDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).FCU

	sid := d.Get("snapshot_id").(string)
	v := d.Get("create_volume_permission_add")
	aid := ""

	req := &fcu.ModifySnapshotAttributeInput{
		SnapshotId: aws.String(sid),
		Attribute:  aws.String("createVolumePermission"),
	}

	remove := v.([]interface{})

	a := make([]*fcu.CreateVolumePermission, 0)

	for _, v1 := range remove {
		data := v1.(map[string]interface{})
		item := &fcu.CreateVolumePermission{
			UserId: aws.String(data["user_id"].(string)),
			Group:  aws.String(data["group"].(string)),
		}
		a = append(a, item)
		aid = data["user_id"].(string)
	}
	req.CreateVolumePermission = &fcu.CreateVolumePermissionModifications{Remove: a}

	var err error
	err = resource.Retry(2*time.Minute, func() *resource.RetryError {
		_, err := conn.VM.ModifySnapshotAttribute(req)
		if err != nil {
			if strings.Contains(fmt.Sprint(err), "RequestLimitExceeded") {
				log.Printf("[DEBUG] Error: %q", err)
				return resource.RetryableError(err)
			}

			return resource.NonRetryableError(err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("Error removing snapshot createVolumePermission: %s", err)
	}

	// Wait for the account to disappear from the permission list
	stateConf := &resource.StateChangeConf{
		Pending:    []string{"granted"},
		Target:     []string{"denied"},
		Refresh:    resourcedOutscaleSnapshotAttributesStateRefreshFunc(conn, sid, aid),
		Timeout:    5 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 10 * time.Second,
	}
	if _, err := stateConf.WaitForState(); err != nil {
		return fmt.Errorf(
			"Error waiting for snapshot createVolumePermission (%s) to be removed: %s",
			d.Id(), err)
	}

	return nil
}

func hasCreateVolumePermission(conn *fcu.Client, sid string, aid string) (bool, error) {
	_, state, err := resourcedOutscaleSnapshotAttributesStateRefreshFunc(conn, sid, aid)()
	if err != nil {
		return false, err
	}
	if state == "granted" {
		return true, nil
	}
	return false, nil
}

func resourcedOutscaleSnapshotAttributesStateRefreshFunc(conn *fcu.Client, sid string, aid string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {

		var attrs *fcu.DescribeSnapshotAttributeOutput
		var err error
		err = resource.Retry(2*time.Minute, func() *resource.RetryError {
			attrs, err = conn.VM.DescribeSnapshotAttribute(&fcu.DescribeSnapshotAttributeInput{
				SnapshotId: aws.String(sid),
				Attribute:  aws.String("createVolumePermission"),
			})
			if err != nil {
				if strings.Contains(fmt.Sprint(err), "RequestLimitExceeded") {
					log.Printf("[DEBUG] Error: %q", err)
					return resource.RetryableError(err)
				}

				return resource.NonRetryableError(err)
			}

			return nil
		})

		if err != nil {
			return nil, "", fmt.Errorf("Error refreshing snapshot createVolumePermission state: %s", err)
		}

		for _, vp := range attrs.CreateVolumePermissions {
			if *vp.UserId == aid {
				return attrs, "granted", nil
			}
		}
		return attrs, "denied", nil
	}
}
