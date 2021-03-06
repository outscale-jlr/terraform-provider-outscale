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

func resourceOutscaleSubNet() *schema.Resource {
	return &schema.Resource{
		Create: resourceOutscaleSubNetCreate,
		Read:   resourceOutscaleSubNetRead,
		Delete: resourceOutscaleSubNetDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
		},
		Schema: getSubNetSchema(),
	}
}

func resourceOutscaleSubNetCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).FCU

	createOpts := &fcu.CreateSubnetInput{
		AvailabilityZone: aws.String(d.Get("availability_zone").(string)),
		CidrBlock:        aws.String(d.Get("cidr_block").(string)),
		VpcId:            aws.String(d.Get("vpc_id").(string)),
	}

	var res *fcu.CreateSubnetOutput
	var err error
	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		res, err = conn.VM.CreateSubNet(createOpts)

		if err != nil {
			if strings.Contains(err.Error(), "RequestLimitExceeded") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("Error creating subnet: %s", err)
	}

	subnet := res.Subnet
	d.SetId(*subnet.SubnetId)
	log.Printf("[INFO] Subnet ID: %s", *subnet.SubnetId)

	if d.IsNewResource() {
		if err := setTags(conn, d); err != nil {
			return err
		}
		d.SetPartial("tag_set")
	}

	log.Printf("[DEBUG] Waiting for subnet (%s) to become available", *subnet.SubnetId)
	stateConf := &resource.StateChangeConf{
		Pending: []string{"pending"},
		Target:  []string{"available"},
		Refresh: SubnetStateRefreshFunc(conn, *subnet.SubnetId),
		Timeout: 10 * time.Minute,
	}

	_, err = stateConf.WaitForState()

	if err != nil {
		return fmt.Errorf(
			"Error waiting for subnet (%s) to become ready: %s",
			d.Id(), err)
	}

	return resourceOutscaleSubNetRead(d, meta)
}

func resourceOutscaleSubNetRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).FCU

	var resp *fcu.DescribeSubnetsOutput
	var err error
	err = resource.Retry(120*time.Second, func() *resource.RetryError {
		resp, err = conn.VM.DescribeSubNet(&fcu.DescribeSubnetsInput{
			SubnetIds: []*string{aws.String(d.Id())},
		})

		if err != nil {
			if strings.Contains(err.Error(), "RequestLimitExceeded:") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		if strings.Contains(err.Error(), "InvalidSubnetID.NotFound") {
			d.SetId("")
			return nil
		}
		return err
	}
	if resp == nil {
		return nil
	}

	subnet := resp.Subnets[0]

	d.Set("subnet_id", aws.StringValue(subnet.SubnetId))
	d.Set("availability_zone", aws.StringValue(subnet.AvailabilityZone))
	d.Set("cidr_block", aws.StringValue(subnet.CidrBlock))
	d.Set("vpc_id", aws.StringValue(subnet.VpcId))
	d.Set("state", aws.StringValue(subnet.State))
	d.Set("available_ip_address_count", aws.Int64Value(subnet.AvailableIpAddressCount))

	d.Set("request_id", resp.RequestId)

	return d.Set("tag_set", tagsToMap(subnet.Tags))
}

func resourceOutscaleSubNetDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).FCU

	id := d.Id()
	log.Printf("[DEBUG] Deleting Subnet (%s)", id)

	req := &fcu.DeleteSubnetInput{
		SubnetId: &id,
	}

	var err error
	err = resource.Retry(120*time.Second, func() *resource.RetryError {
		_, err = conn.VM.DeleteSubNet(req)

		if err != nil {
			if strings.Contains(err.Error(), "RequestLimitExceeded:") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})
	if err != nil {
		log.Printf("[DEBUG] Error deleting Subnet(%s)", err)
		return err
	}

	return nil
}

// SubnetStateRefreshFunc ...
func SubnetStateRefreshFunc(conn *fcu.Client, id string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {

		var resp *fcu.DescribeSubnetsOutput
		var err error
		err = resource.Retry(120*time.Second, func() *resource.RetryError {
			resp, err = conn.VM.DescribeSubNet(&fcu.DescribeSubnetsInput{
				SubnetIds: []*string{aws.String(id)},
			})

			if err != nil {
				if strings.Contains(err.Error(), "RequestLimitExceeded:") {
					return resource.RetryableError(err)
				}
				return resource.NonRetryableError(err)
			}
			return nil
		})

		if err != nil {
			if strings.Contains(err.Error(), "InvalidSubnetID.NotFound") {
				resp = nil
			} else {
				log.Printf("Error on SubnetStateRefresh: %s", err)
				return nil, "", err
			}
		}

		if resp == nil {
			return nil, "", nil
		}

		subnet := resp.Subnets[0]
		return subnet, *subnet.State, nil
	}
}

func getSubNetSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"vpc_id": &schema.Schema{
			Type:     schema.TypeString,
			Required: true,
			ForceNew: true,
		},
		"cidr_block": &schema.Schema{
			Type:     schema.TypeString,
			Required: true,
			ForceNew: true,
		},
		"availability_zone": &schema.Schema{
			Type:     schema.TypeString,
			Optional: true,
			Computed: true,
			ForceNew: true,
		},
		"available_ip_address_count": &schema.Schema{
			Type:     schema.TypeInt,
			Computed: true,
		},
		"state": &schema.Schema{
			Type:     schema.TypeString,
			Computed: true,
		},
		"subnet_id": &schema.Schema{
			Type:     schema.TypeString,
			Computed: true,
		},
		"request_id": &schema.Schema{
			Type:     schema.TypeString,
			Computed: true,
		},
		"tag_set": tagsSchemaComputed(),
		"tag":     tagsSchema(),
	}
}
