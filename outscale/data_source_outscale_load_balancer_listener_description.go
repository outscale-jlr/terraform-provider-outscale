package outscale

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/terraform-providers/terraform-provider-outscale/osc/lbu"
)

func dataSourceOutscaleLoadBalancerLD() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceOutscaleLoadBalancerLDRead,

		Schema: map[string]*schema.Schema{
			"load_balancer_name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"listener_descriptions": &schema.Schema{
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"instance_port": &schema.Schema{
							Type:     schema.TypeString,
							Computed: true,
						},
						"instance_protocol": &schema.Schema{
							Type:     schema.TypeString,
							Computed: true,
						},
						"load_balancer_port": &schema.Schema{
							Type:     schema.TypeString,
							Computed: true,
						},
						"protocol": &schema.Schema{
							Type:     schema.TypeString,
							Computed: true,
						},
						"ssl_certificate_id": &schema.Schema{
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"policy_names": &schema.Schema{
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"request_id": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func dataSourceOutscaleLoadBalancerLDRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).LBU
	ename, ok := d.GetOk("load_balancer_name")

	if !ok {
		return fmt.Errorf("please provide the name of the load balancer")
	}

	elbName := ename.(string)

	// Retrieve the ELB properties for updating the state
	describeElbOpts := &lbu.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{aws.String(elbName)},
	}

	var resp *lbu.DescribeLoadBalancersOutput
	var describeResp *lbu.DescribeLoadBalancersResult
	var err error
	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		resp, err = conn.API.DescribeLoadBalancers(describeElbOpts)
		if err != nil {
			if strings.Contains(fmt.Sprint(err), "Throttling:") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		describeResp = resp.DescribeLoadBalancersResult
		return nil
	})

	if err != nil {
		if isLoadBalancerNotFound(err) {
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error retrieving ELB: %s", err)
	}

	if describeResp.LoadBalancerDescriptions == nil {
		return fmt.Errorf("NO ELB FOUND")
	}

	if len(describeResp.LoadBalancerDescriptions) != 1 {
		return fmt.Errorf("Unable to find ELB: %#v", describeResp.LoadBalancerDescriptions)
	}

	lb := describeResp.LoadBalancerDescriptions[0]

	ls := make([]map[string]interface{}, len(lb.ListenerDescriptions))

	for k1, v2 := range lb.ListenerDescriptions {
		l := make(map[string]interface{})
		l["instance_port"] = strconv.Itoa(int(aws.Int64Value(v2.Listener.InstancePort)))
		l["instance_protocol"] = aws.StringValue(v2.Listener.InstanceProtocol)
		l["load_balancer_port"] = strconv.Itoa(int(aws.Int64Value(v2.Listener.LoadBalancerPort)))
		l["protocol"] = aws.StringValue(v2.Listener.Protocol)
		l["ssl_certificate_id"] = aws.StringValue(v2.Listener.SSLCertificateId)
		ls[k1] = l
	}

	if err := d.Set("listener_descriptions", ls); err != nil {
		return err
	}

	d.Set("request_id", resp.ResponseMetadata.RequestID)
	d.SetId(resource.UniqueId())

	return d.Set("policy_names", flattenStringList(lb.ListenerDescriptions[0].PolicyNames))
}
