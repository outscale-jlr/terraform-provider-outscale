package outscale

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
	"github.com/terraform-providers/terraform-provider-outscale/osc/oapi"
)

func TestAccOutscaleOAPIVolumeAttachment_basic(t *testing.T) {
	o := os.Getenv("OUTSCALE_OAPI")

	oapiFlag, err := strconv.ParseBool(o)
	if err != nil {
		oapiFlag = false
	}

	if !oapiFlag {
		t.Skip()
	}

	var i oapi.Vm
	var v oapi.Volume

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckOAPIVolumeAttachmentDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccOAPIVolumeAttachmentConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"outscale_volumes_link.ebs_att", "device_name", "/dev/sdh"),
					testAccCheckOAPIVMExists(
						"outscale_vm.web", &i),
					testAccCheckOAPIVolumeAttachmentExists(
						"outscale_volumes_link.ebs_att", &i, &v),
				),
			},
		},
	})
}

func testAccCheckOAPIVolumeAttachmentDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		log.Printf("\n\n----- This is never called")
		if rs.Type != "outscale_volume_link" {
			continue
		}
	}
	return nil
}

func testAccCheckOAPIVolumeAttachmentExists(n string, i *oapi.Vm, v *oapi.Volume) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set")
		}

		for _, b := range i.BlockDeviceMappings {
			if rs.Primary.Attributes["device_name"] == b.DeviceName {
				if rs.Primary.Attributes["volume_id"] == b.Bsu.VolumeId {
					// pass
					return nil
				}
			}
		}

		return fmt.Errorf("Error finding instance/volume")
	}
}

const testAccOAPIVolumeAttachmentConfig = `
resource "outscale_vm" "web" {
	image_id               = "ami-5c450b62"
	vm_type                = "c4.large"
	keypair_name           = "testkp"
	security_group_ids     = ["sg-9752b7a6"]
}
resource "outscale_volume" "example" {
  subregion_name = "in-west-2a"
	size = 1
}
resource "outscale_volumes_link" "ebs_att" {
  device_name = "/dev/sdh"
	volume_id = "${outscale_volume.example.id}"
	vm_id = "${outscale_vm.web.id}"
}
`
