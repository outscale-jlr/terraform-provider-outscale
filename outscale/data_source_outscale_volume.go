package outscale

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/terraform-providers/terraform-provider-outscale/osc/fcu"
)

func datasourceOutscaleVolume() *schema.Resource {
	return &schema.Resource{
		Read: datasourceVolumeRead,

		Schema: map[string]*schema.Schema{
			// Arguments
			"filter": dataSourceFiltersSchema(),
			"availability_zone": {
				Type: schema.TypeString,

				Computed: true,
			},
			"iops": {
				Type: schema.TypeInt,

				Computed: true,
			},
			"size": {
				Type: schema.TypeInt,

				Computed: true,
			},
			"snapshot_id": {
				Type: schema.TypeString,

				Computed: true,
			},
			"volume_type": {
				Type: schema.TypeString,

				Computed: true,
			},
			// Attributes
			"attachment_set": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"delete_on_termination": {
							Type:     schema.TypeBool,
							Computed: true,
						},
						"device": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"instance_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"status": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"volume_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"status": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"tag_set": {
				Type: schema.TypeList,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"key": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"value": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
				Computed: true,
			},
			"tags": tagsSchema(),
			"volume_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

func datasourceVolumeRead(d *schema.ResourceData, meta interface{}) error {

	conn := meta.(*OutscaleClient).FCU

	filters, filtersOk := d.GetOk("filter")
	VolumeIds, VolumeIdsOk := d.GetOk("volume_id")

	fmt.Printf("[DEBUG] DS Volume Read Variables : %s, %s ", filters, filtersOk)

	if filtersOk == false {
		return fmt.Errorf("One of filters must be assigned")
	}

	// Build up search parameters

	request := &fcu.DescribeVolumesInput{
		VolumeIds: []*string{aws.String(d.Id())},
	}

	if VolumeIdsOk {
		var allocs []*string
		for _, v := range VolumeIds.([]interface{}) {
			allocs = append(allocs, aws.String(v.(string)))
		}
		request.VolumeIds = allocs
	}

	if filtersOk {
		request.Filters = buildOutscaleDataSourceFilters(filters.(*schema.Set))
	}

	var response *fcu.DescribeVolumesOutput
	var err error

	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		response, err = conn.VM.DescribeVolumes(request)
		if err != nil {
			if strings.Contains(err.Error(), "RequestLimitExceeded:") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return resource.NonRetryableError(err)
	})

	if err != nil {
		if strings.Contains(fmt.Sprint(err), "InvalidVolume.NotFound") {
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error reading Outscale volume %s: %s", d.Id(), err)
	}

	return readVolume(d, response.Volumes[0])
}
