package outscale

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/terraform-providers/terraform-provider-outscale/osc/oapi"
	"github.com/terraform-providers/terraform-provider-outscale/utils"
)

func resourceOutscaleOAPIRouteTable() *schema.Resource {
	return &schema.Resource{
		Create: resourceOutscaleOAPIRouteTableCreate,
		Read:   resourceOutscaleOAPIRouteTableRead,
		Delete: resourceOutscaleOAPIRouteTableDelete,
		Importer: &schema.ResourceImporter{
			State: resourceOutscaleRouteTableImportState,
		},

		Schema: getOAPIRouteTableSchema(),
	}
}

func resourceOutscaleOAPIRouteTableCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).OAPI

	createOpts := &oapi.CreateRouteTableRequest{
		NetId: d.Get("net_id").(string),
	}
	log.Printf("[DEBUG] RouteTable create config: %#v", createOpts)

	var resp *oapi.POST_CreateRouteTableResponses
	var err error
	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		resp, err = conn.POST_CreateRouteTable(*createOpts)
		if err != nil {
			if strings.Contains(fmt.Sprint(err), "RequestLimitExceeded") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})
	var errString string
	if err != nil || resp.OK == nil {
		if err != nil {
			errString = err.Error()
		} else if resp.Code401 != nil {
			errString = fmt.Sprintf("ErrorCode: 401, %s", utils.ToJSONString(resp.Code401))
		} else if resp.Code400 != nil {
			errString = fmt.Sprintf("ErrorCode: 400, %s", utils.ToJSONString(resp.Code400))
		} else if resp.Code500 != nil {
			errString = fmt.Sprintf("ErrorCode: 500, %s", utils.ToJSONString(resp.Code500))
		}

		return fmt.Errorf("Error creating route table: %s", errString)
	}

	result := resp.OK
	d.SetId(result.RouteTable.RouteTableId)
	log.Printf("[INFO] Route Table ID: %s", d.Id())

	log.Printf("[DEBUG] Waiting for route table (%s) to become available", d.Id())
	stateConf := &resource.StateChangeConf{
		Pending: []string{"pending"},
		Target:  []string{"ready"},
		Refresh: resourceOutscaleOAPIRouteTableStateRefreshFunc(conn, d.Id()),
		Timeout: 5 * time.Minute,
	}
	if _, err := stateConf.WaitForState(); err != nil {
		return fmt.Errorf(
			"Error waiting for route table (%s) to become available: %s",
			d.Id(), err)
	}

	if d.IsNewResource() {
		if err := setOAPITags(conn, d); err != nil {
			return err
		}
		d.SetPartial("tags")
	}

	a := make([]interface{}, 0)

	d.Set("tags", a)
	d.Set("routes", a)
	d.Set("link_route_tables", a)

	return resourceOutscaleOAPIRouteTableRead(d, meta)
}

func resourceOutscaleOAPIRouteTableRead(d *schema.ResourceData, meta interface{}) error {
	rtRaw, requestId, err := readOAPIRouteTable(meta.(*OutscaleClient).OAPI, d.Id())
	if err != nil {
		return err
	}
	if rtRaw == nil {
		d.SetId("")
		return nil
	}

	rt := rtRaw.(oapi.RouteTable)
	d.Set("request_id", requestId)
	d.Set("route_table_id", rt.RouteTableId)
	d.Set("net_id", rt.NetId)
	d.Set("route_propagating_virtual_gateways", setOAPIPropagatingVirtualGateways(rt.RoutePropagatingVirtualGateways))
	d.Set("routes", setOAPIRoutes(rt.Routes))
	d.Set("link_route_tables", setOAPILinkRouteTables(rt.LinkRouteTables))
	d.Set("tags", tagsOAPIToMap(rt.Tags))

	return nil
}

func resourceOutscaleOAPIRouteTableDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).OAPI

	rtRaw, _, err := readOAPIRouteTable(meta.(*OutscaleClient).OAPI, d.Id())
	if err != nil {
		return err
	}
	if rtRaw == nil {
		return nil
	}
	rt := rtRaw.(oapi.RouteTable)

	for _, a := range rt.LinkRouteTables {
		log.Printf("[INFO] Disassociating LinkRouteTable: %s", a.LinkRouteTableId)

		var err error
		err = resource.Retry(5*time.Minute, func() *resource.RetryError {

			_, err := conn.POST_UnlinkRouteTable(oapi.UnlinkRouteTableRequest{
				LinkRouteTableId: a.LinkRouteTableId,
			})
			if err != nil {
				if strings.Contains(fmt.Sprint(err), "RequestLimitExceeded") {
					return resource.RetryableError(err)
				}
				return resource.NonRetryableError(err)
			}
			return nil
		})

		if err != nil {
			if strings.Contains(fmt.Sprint(err), "InvalidAssociationID.NotFound") {
				err = nil
			}
			return err
		}
	}

	log.Printf("[INFO] Deleting Route Table: %s", d.Id())

	err = resource.Retry(15*time.Minute, func() *resource.RetryError {
		_, err = conn.POST_DeleteRouteTable(oapi.DeleteRouteTableRequest{
			RouteTableId: d.Id(),
		})
		if err != nil {
			if strings.Contains(fmt.Sprint(err), "RequestLimitExceeded") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		if strings.Contains(fmt.Sprint(err), "InvalidRouteTableID.NotFound") {
			return nil
		}

		return fmt.Errorf("Error deleting route table: %s", err)
	}

	log.Printf("[DEBUG] Waiting for route table (%s) to become destroyed", d.Id())

	stateConf := &resource.StateChangeConf{
		Pending: []string{"ready"},
		Target:  []string{},
		Refresh: resourceOutscaleOAPIRouteTableStateRefreshFunc(conn, d.Id()),
		Timeout: 5 * time.Minute,
	}
	if _, err := stateConf.WaitForState(); err != nil {
		return fmt.Errorf("Error waiting for route table (%s) to become destroyed: %s", d.Id(), err)
	}

	return nil
}

func readOAPIRouteTable(conn *oapi.Client, routeTableId string, linkIds ...string) (interface{}, string, error) {
	log.Printf("[DEBUG] Looking for RouteTable with: id %v and link_id %v", routeTableId, linkIds)
	var resp *oapi.POST_ReadRouteTablesResponses
	var err error
	routeTableRequest := &oapi.ReadRouteTablesRequest{}
	routeTableRequest.Filters = oapi.FiltersRouteTable{RouteTableIds: []string{routeTableId}}
	if len(linkIds) > 0 {
		routeTableRequest.Filters = oapi.FiltersRouteTable{
			RouteTableIds:     []string{routeTableId},
			LinkRouteTableIds: []string{linkIds[0]},
		}
	}

	err = resource.Retry(15*time.Minute, func() *resource.RetryError {
		resp, err = conn.POST_ReadRouteTables(*routeTableRequest)
		if err != nil {
			if strings.Contains(fmt.Sprint(err), "RequestLimitExceeded") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		if strings.Contains(fmt.Sprint(err), "InvalidRouteTableID.NotFound") {
			resp = nil
		} else {
			log.Printf("[DEBUG] Error on RouteTableStateRefresh: %s", err)
			return nil, resp.OK.ResponseContext.RequestId, err
		}
	}

	if resp == nil {
		return resp, resp.OK.ResponseContext.RequestId, nil
	}

	result := resp.OK
	var errString string
	if err != nil || resp.OK == nil {
		if err != nil {
			errString = err.Error()
		} else if resp.Code401 != nil {
			errString = fmt.Sprintf("ErrorCode: 401, %s", utils.ToJSONString(resp.Code401))
		} else if resp.Code400 != nil {
			errString = fmt.Sprintf("ErrorCode: 400, %s", utils.ToJSONString(resp.Code400))
		} else if resp.Code500 != nil {
			errString = fmt.Sprintf("ErrorCode: 500, %s", utils.ToJSONString(resp.Code500))
		}

		return nil, "", fmt.Errorf("Error getting route table: %s", errString)
	}
	if len(result.RouteTables) <= 0 {
		return nil, resp.OK.ResponseContext.RequestId, err
	}
	return result.RouteTables[0], resp.OK.ResponseContext.RequestId, err
}

func resourceOutscaleOAPIRouteTableStateRefreshFunc(conn *oapi.Client, routeTableId string, linkIds ...string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		rtRaw, _, err := readOAPIRouteTable(conn, routeTableId, linkIds...)
		if rtRaw == nil {
			return nil, "", err
		}
		return rtRaw.(oapi.RouteTable), "ready", err
	}
}

func setOAPIRoutes(rt []oapi.Route) []map[string]interface{} {
	route := make([]map[string]interface{}, len(rt))
	if len(rt) > 0 {
		for k, r := range rt {
			m := make(map[string]interface{})
			if r.GatewayId != "" && r.GatewayId == "local" {
				continue
			}
			if r.CreationMethod != "" && r.CreationMethod == "EnableVgwRoutePropagation" {
				continue
			}
			if r.DestinationPrefixListId != "" {
				continue
			}

			if r.CreationMethod != "" {
				m["creation_method"] = r.CreationMethod
			}
			if r.DestinationIpRange != "" {
				m["destination_ip_range"] = r.DestinationIpRange
			}
			if r.DestinationPrefixListId != "" {
				m["destination_prefix_list_id"] = r.DestinationPrefixListId
			}
			if r.GatewayId != "" {
				m["gateway_id"] = r.GatewayId
			}
			if r.NatServiceId != "" {
				m["nat_service_id"] = r.NatServiceId
			}
			if r.NetPeeringId != "" {
				m["net_peering_id"] = r.NetPeeringId
			}
			if r.VmId != "" {
				m["vm_id"] = r.VmId
			}
			if r.NicId != "" {
				m["nic_id"] = r.NicId
			}
			if r.State != "" {
				m["state"] = r.State
			}
			if r.VmAccountId != "" {
				m["vm_account_id"] = r.VmAccountId
			}
			route[k] = m
		}
	}

	return route
}

func setOAPILinkRouteTables(rt []oapi.LinkRouteTable) []map[string]interface{} {
	linkRouteTables := make([]map[string]interface{}, len(rt))
	log.Printf("[DEBUG] LinkRouteTable: %#v", rt)
	if len(rt) > 0 {
		for k, r := range rt {
			m := make(map[string]interface{})
			if r.Main != false {
				m["main"] = r.Main
			}
			if r.RouteTableId != "" {
				m["route_table_id"] = r.RouteTableId
			}
			if r.LinkRouteTableId != "" {
				m["link_route_table_id"] = r.LinkRouteTableId
			}
			if r.SubnetId != "" {
				m["subnet_id"] = r.SubnetId
			}
			linkRouteTables[k] = m
		}
	}

	return linkRouteTables
}

func getOAPIRouteTableSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"net_id": {
			Type:     schema.TypeString,
			Required: true,
			ForceNew: true,
		},
		"route_table_id": {
			Type:     schema.TypeString,
			Computed: true,
		},
		"request_id": {
			Type:     schema.TypeString,
			Computed: true,
		},

		"tags": tagsListOAPISchema(),

		"route_propagating_virtual_gateways": {
			Type:     schema.TypeList,
			Computed: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"virtual_gateway_id": {
						Type:     schema.TypeString,
						Computed: true,
					},
				},
			},
		},
		"routes": {
			Type:     schema.TypeList,
			Computed: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"destination_ip_range": {
						Type:     schema.TypeString,
						Computed: true,
					},
					"destination_prefix_list_id": {
						Type:     schema.TypeString,
						Computed: true,
					},
					"gateway_id": {
						Type:     schema.TypeString,
						Computed: true,
					},
					"vm_id": {
						Type:     schema.TypeString,
						Computed: true,
					},
					"vm_account_id": {
						Type:     schema.TypeString,
						Computed: true,
					},
					"nic_id": {
						Type:     schema.TypeString,
						Computed: true,
					},
					"creation_method": {
						Type:     schema.TypeString,
						Computed: true,
					},
					"state": {
						Type:     schema.TypeString,
						Computed: true,
					},
					"nat_service_id": {
						Type:     schema.TypeString,
						Computed: true,
					},
					"net_peering_id": {
						Type:     schema.TypeString,
						Computed: true,
					},
				},
			},
		},
		"link_route_tables": {
			Type:     schema.TypeList,
			Computed: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"main": {
						Type:     schema.TypeBool,
						Computed: true,
					},
					"route_table_to_subnet_link_id": {
						Type:     schema.TypeString,
						Computed: true,
					},
					"link_route_table_id": {
						Type:     schema.TypeString,
						Computed: true,
					},
					"route_table_id": {
						Type:     schema.TypeString,
						Computed: true,
					},
					"subnet_id": {
						Type:     schema.TypeString,
						Computed: true,
					},
				},
			},
		},
	}
}

func setOAPIPropagatingVirtualGateways(vg []oapi.RoutePropagatingVirtualGateway) (propagatingVGWs []map[string]interface{}) {
	if len(vg) > 0 {
		for k, vgw := range vg {
			m := make(map[string]interface{})
			if vgw.VirtualGatewayId != "" {
				m["virtual_gateway_id"] = vgw.VirtualGatewayId
			}
			propagatingVGWs[k] = m
		}
	}
	return
}
