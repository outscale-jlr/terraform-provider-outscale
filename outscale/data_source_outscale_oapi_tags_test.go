package outscale

import (
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform/helper/resource"
)

func TestAccOutscaleOAPITagsDataSource_basic(t *testing.T) {
	o := os.Getenv("OUTSCALE_OAPI")

	oapi, err := strconv.ParseBool(o)
	if err != nil {
		oapi = false
	}

	if !oapi {
		t.Skip()
	}

	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccOAPITagsDataSourceConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.outscale_tags.web", "tag_set.#", "2"),
				),
			},
		},
	})
}

// Lookup based on InstanceID
const testAccOAPITagsDataSourceConfig = `
resource "outscale_vm" "basic" {
  image_id = "ami-8a6a0120"
	type = "m1.small"
}
resource "outscale_vm" "basic2" {
  image_id = "ami-8a6a0120"
	type = "m1.small"
}

data "outscale_tags" "web" {
	filter {
    name = "resource-type"
    values = ["instance"]
	}
}`
