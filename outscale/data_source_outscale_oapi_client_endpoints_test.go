package outscale

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform/helper/acctest"
	"github.com/hashicorp/terraform/helper/resource"
)

func TestAccOutscaleOAPIDSCustomerGateways_basic(t *testing.T) {
	o := os.Getenv("OUTSCALE_OAPI")

	oapi, err := strconv.ParseBool(o)
	if err != nil {
		oapi = false
	}

	if !oapi {
		t.Skip()
	}

	rBgpAsn := acctest.RandIntRange(64512, 65534)
	rInt := acctest.RandInt()
	resource.Test(t, resource.TestCase{
		PreCheck:      func() { testAccPreCheck(t) },
		IDRefreshName: "outscale_client_endpoint.foo",
		Providers:     testAccProviders,
		CheckDestroy:  testAccCheckCustomerGatewayDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccOAPICustomerGatewaysDSConfig(rInt, rBgpAsn),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckOutscaleCGsDataSourceID("data.outscale_client_endpoints.test"),
					resource.TestCheckResourceAttr("data.outscale_client_endpoints.test", "customer_gateway_set.#", "1"),
				),
			},
		},
	})
}

func testAccOAPICustomerGatewaysDSConfig(rInt, rBgpAsn int) string {
	return fmt.Sprintf(`
		resource "outscale_client_endpoint" "foo" {
			bgp_asn = %d
			ip_range = "172.0.0.1"
			type = "ipsec.1"
			tag {
				Name = "foo-gateway-%d"
			}
		}

		data "outscale_client_endpoints" "test" {
			client_endpoint_id = ["${outscale_client_endpoint.foo.id}"]
		}
		`, rBgpAsn, rInt)
}
