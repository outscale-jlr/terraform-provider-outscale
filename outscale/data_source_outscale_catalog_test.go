package outscale

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
)

func TestAccOutscaleDSCatalog_basic(t *testing.T) {
	o := os.Getenv("OUTSCALE_OAPI")

	oapi, err := strconv.ParseBool(o)
	if err != nil {
		oapi = false
	}

	if oapi {
		t.Skip()
	}

	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDSCatalogConfig(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckOutscaleCatalogDataSourceID("data.outscale_catalog.test"),
					resource.TestCheckResourceAttrSet("data.outscale_catalog.test", "catalog.#"),
					resource.TestCheckResourceAttrSet("data.outscale_catalog.test", "catalog.0.entries.#"),
					resource.TestCheckResourceAttrSet("data.outscale_catalog.test", "catalog.0.attributes.#"),
				),
			},
		},
	})
}

func testAccCheckOutscaleCatalogDataSourceID(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Can't find Catalog data source: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("Catalog data source ID not set")
		}
		return nil
	}
}

func testAccDSCatalogConfig() string {
	return fmt.Sprintf(`
		data "outscale_catalog" "test" {}
	`)
}
