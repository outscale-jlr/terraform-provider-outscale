package outscale

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
	"github.com/terraform-providers/terraform-provider-outscale/osc/fcu"
)

func TestAccOutscaleLinInternetGatewayLink_basic(t *testing.T) {
	o := os.Getenv("OUTSCALE_OAPI")

	oapi, err := strconv.ParseBool(o)
	if err != nil {
		oapi = false
	}

	if oapi {
		t.Skip()
	}
	var conf fcu.InternetGateway

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckOutscaleLinInternetGatewayLinkDettached,
		Steps: []resource.TestStep{
			resource.TestStep{
				Config: testAccOutscaleLinInternetGatewayLinkConfig,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckOutscaleLinInternetGatewayLinkExists("outscale_lin_internet_gateway_link.link", &conf),
				),
			},
		},
	})
}

func testAccCheckOutscaleLinInternetGatewayLinkExists(n string, res *fcu.InternetGateway) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No internet gateway link id is set")
		}
		var resp *fcu.DescribeInternetGatewaysOutput
		conn := testAccProvider.Meta().(*OutscaleClient)

		id := rs.Primary.Attributes["internet_gateway_id"]

		err := resource.Retry(5*time.Minute, func() *resource.RetryError {
			var err error
			resp, err = conn.FCU.VM.DescribeInternetGateways(&fcu.DescribeInternetGatewaysInput{
				InternetGatewayIds: []*string{aws.String(id)},
			})
			if err != nil {
				if strings.Contains(err.Error(), "RequestLimitExceeded:") {
					return resource.RetryableError(err)
				}
				return resource.NonRetryableError(err)
			}
			return resource.NonRetryableError(err)
		})
		if err != nil {
			return err
		}
		if len(resp.InternetGateways) != 1 ||
			*resp.InternetGateways[0].InternetGatewayId != id {
			return fmt.Errorf("Internet Gateway not found")
		}

		*res = *resp.InternetGateways[0]

		return nil
	}
}

func testAccCheckOutscaleLinInternetGatewayLinkDettached(s *terraform.State) error {
	conn := testAccProvider.Meta().(*OutscaleClient)

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "outscale_lin_internet_gateway_link" {
			continue
		}

		id := rs.Primary.Attributes["internet_gateway_id"]

		// Try to find an internet gateway
		var resp *fcu.DescribeInternetGatewaysOutput
		err := resource.Retry(5*time.Minute, func() *resource.RetryError {
			var err error
			resp, err = conn.FCU.VM.DescribeInternetGateways(&fcu.DescribeInternetGatewaysInput{
				InternetGatewayIds: []*string{aws.String(id)},
			})

			if err != nil {
				if strings.Contains(err.Error(), "RequestLimitExceeded:") {
					return resource.RetryableError(err)
				}
				return resource.NonRetryableError(err)
			}

			return resource.RetryableError(err)
		})

		if resp == nil {
			return nil
		}

		if err == nil {
			if len(resp.InternetGateways) > 0 {
				return fmt.Errorf("still exist")
			}
			return nil
		}

		// Verify the error is what we want
		ec2err, ok := err.(awserr.Error)
		if !ok {
			return err
		}
		if ec2err.Code() != "InvalidInternetGateway.NotFound" {
			return err
		}
	}

	return nil
}

const testAccOutscaleLinInternetGatewayLinkConfig = `
resource "outscale_lin_internet_gateway" "gateway" {}

resource "outscale_lin" "vpc" {
	cidr_block = "10.0.0.0/16"
}

resource "outscale_lin_internet_gateway_link" "link" {
	vpc_id = "${outscale_lin.vpc.id}"
	internet_gateway_id = "${outscale_lin_internet_gateway.gateway.id}"
}
`
