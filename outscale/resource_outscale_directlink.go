package outscale

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/terraform-providers/terraform-provider-outscale/osc/dl"
)

func resourceOutscaleDirectLink() *schema.Resource {
	return &schema.Resource{
		Create: resourceOutscaleDirectLinkCreate,
		Read:   resourceOutscaleDirectLinkRead,
		Delete: resourceOutscaleDirectLinkDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		Schema: map[string]*schema.Schema{
			"location": {
				Type:     schema.TypeString,
				Computed: true,
				Optional: true,
				ForceNew: true,
			},
			"bandwidth": {
				Type:         schema.TypeString,
				Computed:     true,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validateDxConnectionBandWidth,
			},
			"connection_name": {
				Type:     schema.TypeString,
				Computed: true,
				Optional: true,
				ForceNew: true,
			},
			"connection_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"connection_state": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"owner_account": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"region": {
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
func resourceOutscaleDirectLinkCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).DL

	req := &dl.CreateConnectionInput{}

	if v, ok := d.GetOk("bandwidth"); ok {
		req.Bandwidth = aws.String(v.(string))
	}
	if v, ok := d.GetOk("connection_name"); ok {
		req.ConnectionName = aws.String(v.(string))
	}
	if v, ok := d.GetOk("location"); ok {
		req.Location = aws.String(v.(string))
	}

	var resp *dl.Connection
	var err error
	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		resp, err = conn.API.CreateConnection(req)

		if err != nil {
			if strings.Contains(fmt.Sprint(err), "RequestLimitExceeded:") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		return err
	}

	d.SetId(aws.StringValue(resp.ConnectionID))

	return resourceOutscaleDirectLinkRead(d, meta)
}

func resourceOutscaleDirectLinkRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).DL

	var resp *dl.Connections
	var err error
	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		resp, err = conn.API.DescribeConnections(&dl.DescribeConnectionsInput{
			ConnectionID: aws.String(d.Id()),
		})

		if err != nil {
			if strings.Contains(fmt.Sprint(err), "RequestLimitExceeded:") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		if isNoSuchDxConnectionErr(err) {
			log.Printf("[WARN] Direct Connect connection (%s) not found, removing from state", d.Id())
			d.SetId("")
			return nil
		}
		return err
	}

	if len(resp.Connections) < 1 {
		log.Printf("[WARN] Direct Connect connection (%s) not found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if len(resp.Connections) != 1 {
		return fmt.Errorf("[ERROR] Number of Direct Connect connections (%s) isn't one, got %d", d.Id(), len(resp.Connections))
	}

	connection := resp.Connections[0]
	if d.Id() != aws.StringValue(connection.ConnectionID) {
		return fmt.Errorf("[ERROR] Direct Connect connection (%s) not found", d.Id())
	}

	if aws.StringValue(connection.ConnectionState) == "deleted" {
		log.Printf("[WARN] Direct Connect connection (%s) not found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	d.Set("bandwidth", aws.StringValue(connection.Bandwidth))
	d.Set("connection_id", aws.StringValue(connection.ConnectionID))
	d.Set("connection_name", aws.StringValue(connection.ConnectionName))
	d.Set("location", aws.StringValue(connection.Location))
	d.Set("owner_account", aws.StringValue(connection.OwnerAccount))
	d.Set("region", aws.StringValue(connection.Region))
	d.Set("request_id", resp.RequestID)

	return nil
}

func resourceOutscaleDirectLinkDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).DL

	var err error
	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		_, err = conn.API.DeleteConnection(&dl.DeleteConnectionInput{
			ConnectionID: aws.String(d.Id()),
		})

		if err != nil {
			if strings.Contains(fmt.Sprint(err), "RequestLimitExceeded:") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		if isNoSuchDxConnectionErr(err) {
			return nil
		}
		return err
	}

	deleteStateConf := &resource.StateChangeConf{
		Pending:    []string{"pending", "ordering", "available", "requested", "deleting"},
		Target:     []string{"deleted"},
		Refresh:    dxConnectionRefreshStateFunc(conn, d.Id()),
		Timeout:    10 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}
	_, err = deleteStateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("Error waiting for Direct Connect connection (%s) to be deleted: %s", d.Id(), err)
	}
	return nil
}

func dxConnectionRefreshStateFunc(conn *dl.Client, connID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		input := &dl.DescribeConnectionsInput{
			ConnectionID: aws.String(connID),
		}

		var resp *dl.Connections
		var err error
		err = resource.Retry(5*time.Minute, func() *resource.RetryError {
			resp, err = conn.API.DescribeConnections(input)

			if err != nil {
				if strings.Contains(fmt.Sprint(err), "RequestLimitExceeded:") {
					return resource.RetryableError(err)
				}
				return resource.NonRetryableError(err)
			}
			return nil
		})

		if err != nil {
			return nil, "failed", err
		}
		if len(resp.Connections) < 1 {
			return resp, "deleted", nil
		}
		return resp, *resp.Connections[0].ConnectionState, nil
	}
}

func isNoSuchDxConnectionErr(err error) bool {
	return strings.Contains(fmt.Sprint(err), "DirectConnectClientException")
}

func validateDxConnectionBandWidth(v interface{}, k string) (ws []string, errors []error) {
	val, ok := v.(string)
	if !ok {
		errors = append(errors, fmt.Errorf("expected type of %s to be string", k))
		return
	}
	validBandWidth := []string{"1Gbps", "10Gbps"}
	for _, str := range validBandWidth {
		if val == str {
			return
		}
	}
	errors = append(errors, fmt.Errorf("expected %s to be one of %v, got %s", k, validBandWidth, val))
	return
}
