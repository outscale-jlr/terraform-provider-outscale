package outscale

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
)

func TestAccOutscaleOAPIImagesDataSource_Instance(t *testing.T) {
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
				Config: testAccCheckOutscaleOAPIImagesDataSourceConfig,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckOutscaleOAPIImagesDataSourceID("data.outscale_images.nat_ami"),
					resource.TestCheckResourceAttr("data.outscale_images.nat_ami", "image_set.0.architecture", "x86_64"),
					resource.TestCheckResourceAttr("data.outscale_images.nat_ami", "image_set.0.description", "Debian 9 - 4.9.51"),
					resource.TestCheckResourceAttr("data.outscale_images.nat_ami", "image_set.0.block_device_mappings.#", "1"),
					resource.TestMatchResourceAttr("data.outscale_images.nat_ami", "image_set.0.image_id", regexp.MustCompile("^ami-")),
					resource.TestCheckResourceAttr("data.outscale_images.nat_ami", "image_set.0.type", "machine"),
					resource.TestCheckResourceAttr("data.outscale_images.nat_ami", "image_set.0.is_public", "true"),
					resource.TestCheckResourceAttr("data.outscale_images.nat_ami", "image_set.0.root_device_name", "/dev/sda1"),
					resource.TestCheckResourceAttr("data.outscale_images.nat_ami", "image_set.0.root_device_type", "ebs"),
					resource.TestCheckResourceAttr("data.outscale_images.nat_ami", "image_set.0.state", "available"),
				),
			},
		},
	})
}

func testAccCheckOutscaleOAPIImagesDataSourceID(n string) resource.TestCheckFunc {
	// Wait for IAM role
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Can't find AMI data source: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("AMI data source ID not set")
		}
		return nil
	}
}

const testAccCheckOutscaleOAPIImagesDataSourceConfig = `
data "outscale_images" "nat_ami" {
	filter {
		name = "architectures"
		values = ["x86_64"]
	}
	filter {
		name = "virtualization_types"
		values = ["hvm"]
	}
	filter {
		name = "root_device_types"
		values = ["ebs"]
	}
	filter {
		name = "block_device_mapping_volume_type"
		values = ["standard"]
	}
}
`
