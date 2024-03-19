// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package linuxgeneralmodule is the main package of the application
package linuxgeneralmodule

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/opiproject/opi-evpn-bridge/pkg/config"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/common"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"path"
)

// ModulelgmHandler enmpty interface
type ModulelgmHandler struct{}

// RoutingTableMax max value of routing table
const RoutingTableMax = 4000

// RoutingTableMin min value of routing table
const RoutingTableMin = 1000

// lgmComp string constant
const lgmComp string = "lgm"

// brStr string constant
const brStr string = "br-"

// vxlanStr string constant
const vxlanStr string = "vxlan-"

// GenerateRouteTable range specification, note that min <= max
func GenerateRouteTable() uint32 {
	return uint32(rand.Intn(RoutingTableMax-RoutingTableMin+1) + RoutingTableMin)
}

// run runs the commands
func run(cmd []string, flag bool) (string, int) {
	var out []byte
	var err error
	out, err = exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	if err != nil {
		if flag {
			panic(fmt.Sprintf("LGM: Command %s': exit code %s;", out, err.Error()))
		}
		log.Printf("LGM: Command %s': exit code %s;\n", out, err)
		return "Error", -1
	}
	output := string(out)
	return output, 0
}

// HandleEvent handles the events with event data
func (h *ModulelgmHandler) HandleEvent(eventType string, objectData *eventbus.ObjectData) {
	switch eventType {
	case "vrf":
		log.Printf("LGM recevied %s %s\n", eventType, objectData.Name)
		handlevrf(objectData)
	case "svi":
		log.Printf("LGM recevied %s %s\n", eventType, objectData.Name)
		handlesvi(objectData)
	case "logical-bridge":
		log.Printf("LGM recevied %s %s\n", eventType, objectData.Name)
		handleLB(objectData)
	default:
		log.Printf("LGM: error: Unknown event type %s", eventType)
	}
}

// handleLB handles the logical Bridge
func handleLB(objectData *eventbus.ObjectData) {
	var comp common.Component
	lb, err := infradb.GetLB(objectData.Name)
	if err == nil {
		log.Printf("LGM : GetLB Name: %s\n", lb.Name)
	} else {
		log.Printf("LGM: GetLB error: %s %s\n", err, objectData.Name)
		return
	}
	if len(lb.Status.Components) != 0 {
		for i := 0; i < len(lb.Status.Components); i++ {
			if lb.Status.Components[i].Name == lgmComp {
				comp = lb.Status.Components[i]
			}
		}
	}
	if lb.Status.LBOperStatus != infradb.LogicalBridgeOperStatusToBeDeleted {
		status := setUpBridge(lb)
		comp.Name = lgmComp
		if status {
			comp.Details = ""
			comp.CompStatus = common.ComponentStatusSuccess
			comp.Timer = 0
		} else {
			if comp.Timer == 0 {
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2
			}
			comp.CompStatus = common.ComponentStatusError
		}
		log.Printf("LGM: %+v \n", comp)
		err := infradb.UpdateLBStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating lb status: %s\n", err)
		}
	} else {
		status := tearDownBridge(lb)
		comp.Name = lgmComp
		if status {
			comp.CompStatus = common.ComponentStatusSuccess
			comp.Timer = 0
		} else {
			comp.CompStatus = common.ComponentStatusError
			if comp.Timer == 0 {
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2
			}
		}
		log.Printf("LGM: %+v\n", comp)
		err := infradb.UpdateLBStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating lb status: %s\n", err)
		}
	}
}

// handlesvi handles the svi functionality
func handlesvi(objectData *eventbus.ObjectData) {
	var comp common.Component
	svi, err := infradb.GetSvi(objectData.Name)
	if err == nil {
		log.Printf("LGM : GetSvi Name: %s\n", svi.Name)
	} else {
		log.Printf("LGM: GetSvi error: %s %s\n", err, objectData.Name)
		return
	}
	if objectData.ResourceVersion != svi.ResourceVersion {
		log.Printf("LGM: Mismatch in resoruce version %+v\n and svi resource version %+v\n", objectData.ResourceVersion, svi.ResourceVersion)
		comp.Name = lgmComp
		comp.CompStatus = common.ComponentStatusError
		if comp.Timer == 0 {
			comp.Timer = 2 * time.Second
		} else {
			comp.Timer *= 2
		}
		err := infradb.UpdateSviStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating svi status: %s\n", err)
		}
		return
	}
	if len(svi.Status.Components) != 0 {
		for i := 0; i < len(svi.Status.Components); i++ {
			if svi.Status.Components[i].Name == lgmComp {
				comp = svi.Status.Components[i]
			}
		}
	}
	if svi.Status.SviOperStatus != infradb.SviOperStatusToBeDeleted {
		details, status := setUpSvi(svi)
		comp.Name = lgmComp
		if status {
			comp.Details = details
			comp.CompStatus = common.ComponentStatusSuccess
			comp.Timer = 0
		} else {
			if comp.Timer == 0 {
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2
			}
			comp.CompStatus = common.ComponentStatusError
		}
		log.Printf("LGM: %+v \n", comp)
		err := infradb.UpdateSviStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating svi status: %s\n", err)
		}
	} else {
		status := tearDownSvi(svi)
		comp.Name = lgmComp
		if status {
			comp.CompStatus = common.ComponentStatusSuccess
			comp.Timer = 0
		} else {
			comp.CompStatus = common.ComponentStatusError
			if comp.Timer == 0 {
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2
			}
		}
		log.Printf("LGM: %+v \n", comp)
		err := infradb.UpdateSviStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating svi status: %s\n", err)
		}
	}
}

// handlevrf handles the vrf functionality
func handlevrf(objectData *eventbus.ObjectData) {
	var comp common.Component
	vrf, err := infradb.GetVrf(objectData.Name)
	if err == nil {
		log.Printf("LGM : GetVRF Name: %s\n", vrf.Name)
	} else {
		log.Printf("LGM: GetVRF error: %s %s\n", err, objectData.Name)
		return
	}
	if objectData.ResourceVersion != vrf.ResourceVersion {
		log.Printf("LGM: Mismatch in resoruce version %+v\n and vrf resource version %+v\n", objectData.ResourceVersion, vrf.ResourceVersion)
		comp.Name = lgmComp
		comp.CompStatus = common.ComponentStatusError
		if comp.Timer == 0 { // wait timer is 2 powerof natural numbers ex : 1,2,3...
			comp.Timer = 2 * time.Second
		} else {
			comp.Timer *= 2
		}
		err := infradb.UpdateVrfStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating vrf status: %s\n", err)
		}
		return
	}
	if len(vrf.Status.Components) != 0 {
		for i := 0; i < len(vrf.Status.Components); i++ {
			if vrf.Status.Components[i].Name == lgmComp {
				comp = vrf.Status.Components[i]
			}
		}
	}
	if vrf.Status.VrfOperStatus != infradb.VrfOperStatusToBeDeleted {
		details, status := set_up_vrf(vrf)
		comp.Name = lgmComp
		if status {
			comp.Details = details
			comp.CompStatus = common.ComponentStatusSuccess
			comp.Timer = 0
		} else {
			if comp.Timer == 0 { // wait timer is 2 powerof natural numbers ex : 1,2,3...
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2
			}
			comp.CompStatus = common.ComponentStatusError
		}
		log.Printf("LGM: %+v \n", comp)
		err := infradb.UpdateVrfStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, vrf.Metadata, comp)
		if err != nil {
			log.Printf("error in updating vrf status: %s\n", err)
		}
	} else {
		status := tearDownVrf(vrf)
		comp.Name = lgmComp
		if status {
			comp.CompStatus = common.ComponentStatusSuccess
			comp.Timer = 0
		} else {
			comp.CompStatus = common.ComponentStatusError
			if comp.Timer == 0 { // wait timer is 2 powerof natural numbers ex : 1,2,3...
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2
			}
		}
		log.Printf("LGM: %+v\n", comp)
		err := infradb.UpdateVrfStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating vrf status: %s\n", err)
		}
	}
}

/*func readConfig(filename string) (*Config, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}*/

var defaultVtep string
var ipMtu int
var brTenant string
var ctx context.Context
var nlink utils.Netlink

// Init initializes the config, logger and subscribers
func Init() {
	/*config, err := readConfig("config.yaml")
	if err != nil {
		log.Fatal(err)
	}*/
	eb := eventbus.EBus
	for _, subscriberConfig := range config.GlobalConfig.Subscribers {
		if subscriberConfig.Name == lgmComp {
			for _, eventType := range subscriberConfig.Events {
				eb.StartSubscriber(subscriberConfig.Name, eventType, subscriberConfig.Priority, &ModulelgmHandler{})
			}
		}
	}
	brTenant = "br-tenant"
	defaultVtep = config.GlobalConfig.Linux_frr.Default_vtep
	ipMtu = config.GlobalConfig.Linux_frr.Ip_mtu
	ctx = context.Background()
	nlink = utils.NewNetlinkWrapper()
}

// routingTableBusy checks if the route is in filterred list
func routingTableBusy(table uint32) bool {
	_, err := nlink.RouteListFiltered(ctx, netlink.FAMILY_V4, &netlink.Route{Table: int(table)}, netlink.RT_FILTER_TABLE)
	return err == nil
}

// setUpBridge sets up the bridge
func setUpBridge(lb *infradb.LogicalBridge) bool {
	link := fmt.Sprintf("vxlan-%+v", lb.Spec.VlanID)
	if !reflect.ValueOf(lb.Spec.Vni).IsZero() {
		// Vni := fmt.Sprintf("%+v", *lb.Spec.Vni)
		// VtepIP := fmt.Sprintf("%+v", lb.Spec.VtepIP.IP)
		// Vlanid := fmt.Sprintf("%+v", lb.Spec.VlanId)
		// ipMtu := fmt.Sprintf("%+v", ipMtu)
		brIntf, err := nlink.LinkByName(ctx, brTenant)
		if err != nil {
			log.Printf("LGM: Failed to get link information for %s: %v\n", brTenant, err)
			return false
		}
		vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: link}, VxlanId: int(*lb.Spec.Vni), Port: 4789, Learning: false, SrcAddr: lb.Spec.VtepIP.IP}
		if err := nlink.LinkAdd(ctx, vxlan); err != nil {
			log.Printf("LGM: Failed to create Vxlan linki %s: %v\n", link, err)
			return false
		}
		// Example: ip link set vxlan-<lb-vlan-id> master br-tenant addrgenmode none
		if err = nlink.LinkSetMaster(ctx, vxlan, brIntf); err != nil {
			log.Printf("LGM: Failed to add Vxlan %s to bridge %s: %v\n", link, brTenant, err)
			return false
		}
		// Example: ip link set vxlan-<lb-vlan-id> up
		if err = nlink.LinkSetUp(ctx, vxlan); err != nil {
			log.Printf("LGM: Failed to up Vxlan link %s: %v\n", link, err)
			return false
		}
		// Example: bridge vlan add dev vxlan-<lb-vlan-id> vid <lb-vlan-id> pvid untagged
		if err = nlink.BridgeVlanAdd(ctx, vxlan, uint16(lb.Spec.VlanID), true, true, false, false); err != nil {
			log.Printf("LGM: Failed to add vlan to bridge %s: %v\n", brTenant, err)
			return false
		}
		if err = nlink.LinkSetBrNeighSuppress(ctx, vxlan, true); err != nil {
			log.Printf("LGM: Failed to add bridge %v neigh_suppress: %s\n", vxlan, err)
			return false
		}
		/*
			CP, err := run([]string{"ip", "link", "add", link, "type", "vxlan", "id", Vni, "local", VtepIP, "dstport", "4789", "nolearning", "proxy"}, false)
			if err != 0 {
				log.Printf("LGM:Error in executing command %s %s\n", "link add ", link)
				log.Printf("%s\n", CP)
				return false
			}
			CP, err = run([]string{"ip", "link", "set", link, "master", brTenant, "up", "mtu", ipMtu}, false)
			if err != 0 {
				log.Printf("LGM:Error in executing command %s %s\n", "link set ", link)
				log.Printf("%s\n", CP)
				return false
			}
			CP, err = run([]string{"bridge", "vlan", "add", "dev", link, "vid", Vlanid, "pvid", "untagged"}, false)
			if err != 0 {
				log.Printf("LGM:Error in executing command %s %s\n", "bridge vlan add dev", link)
				log.Printf("%s\n", CP)
				return false
			}
			CP, err = run([]string{"bridge", "link", "set", "dev", link, "neigh_suppress", "on"}, false)
			if err != 0 {
				log.Printf("LGM:Error in executing command %s %s\n", "bridge link set dev link neigh_suppress on", link)
				log.Printf("%s\n", CP)
				return false
			}*/
		return true
	}
	return true
}

// set_up_vrf sets up the vrf
func set_up_vrf(vrf *infradb.Vrf) (string, bool) {
	IPMtu := fmt.Sprintf("%+v", ipMtu)
	Ifname := strings.Split(vrf.Name, "/")
	ifwlen := len(Ifname)
	vrf.Name = Ifname[ifwlen-1]
	if vrf.Name == "GRD" {
		vrf.Metadata.RoutingTable = make([]*uint32, 2)
		vrf.Metadata.RoutingTable[0] = new(uint32)
		vrf.Metadata.RoutingTable[1] = new(uint32)
		*vrf.Metadata.RoutingTable[0] = 254
		*vrf.Metadata.RoutingTable[1] = 255
		return "", true
	}
	routingTable := GenerateRouteTable()
	vrf.Metadata.RoutingTable = make([]*uint32, 1)
	vrf.Metadata.RoutingTable[0] = new(uint32)
	if routingTableBusy(routingTable) {
		log.Printf("LGM :Routing table %d is not empty\n", routingTable)
		// return "Error"
	}
	var vtip string
	if !reflect.ValueOf(vrf.Spec.VtepIP).IsZero() {
		vtip = fmt.Sprintf("%+v", vrf.Spec.VtepIP.IP)
		// Verify that the specified VTEP IP exists as local IP
		err := nlink.RouteListIPTable(ctx, vtip)
		// Not found similar API in viswananda library so retain the linux commands as it is .. not able to get the route list exact vtip table local
		if !err {
			log.Printf(" LGM: VTEP IP not found: %+v\n", vrf.Spec.VtepIP)
			return "", false
		}
	} else {
		// Pick the IP of interface default VTEP interface
		// log.Printf("LGM: VTEP iP %+v\n",getIPAddress(defaultVtep))
		vtip = fmt.Sprintf("%+v", vrf.Spec.VtepIP.IP)
		*vrf.Spec.VtepIP = getIPAddress(defaultVtep)
	}
	log.Printf("set_up_vrf: %s %d %d\n", vtip, routingTable)
	// Create the vrf interface for the specified routing table and add loopback address

	linkAdderr := nlink.LinkAdd(ctx, &netlink.Vrf{
		LinkAttrs: netlink.LinkAttrs{Name: vrf.Name},
		Table:     routingTable,
	})
	if linkAdderr != nil {
		log.Printf("LGM: Error in Adding vrf link table %d\n", routingTable)
		return "", false
	}

	log.Printf("LGM: vrf link %s Added with table id %d\n", vrf.Name, routingTable)

	link, linkErr := nlink.LinkByName(ctx, vrf.Name)
	if linkErr != nil {
		log.Printf("LGM : Link %s not found\n", vrf.Name)
		return "", false
	}

	linkmtuErr := nlink.LinkSetMTU(ctx, link, ipMtu)
	if linkmtuErr != nil {
		log.Printf("LGM : Unable to set MTU to link %s \n", vrf.Name)
		return "", false
	}

	linksetupErr := nlink.LinkSetUp(ctx, link)
	if linksetupErr != nil {
		log.Printf("LGM : Unable to set link %s UP \n", vrf.Name)
		return "", false
	}
	Lbip := fmt.Sprintf("%+v", vrf.Spec.LoopbackIP.IP)

	var address = vrf.Spec.LoopbackIP
	var Addrs = &netlink.Addr{
		IPNet: address,
	}
	addrErr := nlink.AddrAdd(ctx, link, Addrs)
	if addrErr != nil {
		log.Printf("LGM: Unable to set the loopback ip to vrf link %s \n", vrf.Name)
		return "", false
	}

	log.Printf("LGM: Added Address %s dev %s\n", Lbip, vrf.Name)

	Src1 := net.IPv4(0, 0, 0, 0)
	route := netlink.Route{
		Table:    int(routingTable),
		Type:     unix.RTN_THROW,
		Protocol: 255,
		Priority: 9999,
		Src:      Src1,
	}
	routeaddErr := nlink.RouteAdd(ctx, &route)
	if routeaddErr != nil {
		log.Printf("LGM : Failed in adding Route throw default %+v\n", routeaddErr)
		return "", false
	}

	log.Printf("LGM : Added route throw default table %d proto opi_evpn_br metric 9999\n", routingTable)
	// Disable reverse-path filtering to accept ingress traffic punted by the pipeline
	// disable_rp_filter("rep-"+vrf.Name)
	// Configuration specific for VRFs associated with L3 EVPN
	if !reflect.ValueOf(vrf.Spec.Vni).IsZero() {
		// Create bridge for external VXLAN under vrf
		// Linux apparently creates a deterministic MAC address for a bridge type link with a given
		// name. We need to assign a true random MAC address to avoid collisions when pairing two
		// IPU servers.

		brErr := nlink.LinkAdd(ctx, &netlink.Bridge{
			LinkAttrs: netlink.LinkAttrs{Name: brStr + vrf.Name},
		})
		if brErr != nil {
			log.Printf("LGM : Error in added bridge port\n")
			return "", false
		}
		log.Printf("LGM : Added link br-%s type bridge\n", vrf.Name)

		rmac := fmt.Sprintf("%+v", GenerateMac()) // str(macaddress.MAC(b'\x00'+random.randbytes(5))).replace("-", ":")
		hw, _ := net.ParseMAC(rmac)

		linkBr, brErr := nlink.LinkByName(ctx, brStr+vrf.Name)
		if brErr != nil {
			log.Printf("LGM : Error in getting the br-%s\n", vrf.Name)
			return "", false
		}
		hwErr := nlink.LinkSetHardwareAddr(ctx, linkBr, hw)
		if hwErr != nil {
			log.Printf("LGM: Failed in the setting Hardware Address\n")
			return "", false
		}

		linkmtuErr := nlink.LinkSetMTU(ctx, linkBr, ipMtu)
		if linkmtuErr != nil {
			log.Printf("LGM : Unable to set MTU to link br-%s \n", vrf.Name)
			return "", false
		}

		linkMaster, errMaster := nlink.LinkByName(ctx, vrf.Name)
		if errMaster != nil {
			log.Printf("LGM : Error in getting the %s\n", vrf.Name)
			return "", false
		}

		err := nlink.LinkSetMaster(ctx, linkBr, linkMaster)
		if err != nil {
			log.Printf("LGM : Unable to set the master to br-%s link", vrf.Name)
			return "", false
		}

		linksetupErr = nlink.LinkSetUp(ctx, linkBr)
		if linksetupErr != nil {
			log.Printf("LGM : Unable to set link %s UP \n", vrf.Name)
			return "", false
		}
		log.Printf("LGM: link set  br-%s master  %s up mtu \n", vrf.Name, IPMtu)

		// Create the VXLAN link in the external bridge

		SrcVtep := vrf.Spec.VtepIP.IP
		vxlanErr := nlink.LinkAdd(ctx, &netlink.Vxlan{
			LinkAttrs: netlink.LinkAttrs{Name: vxlanStr + vrf.Name, MTU: ipMtu}, VxlanId: int(*vrf.Spec.Vni), SrcAddr: SrcVtep, Learning: false, Proxy: true, Port: 4789})
		if vxlanErr != nil {
			log.Printf("LGM : Error in added vxlan port\n")
			return "", false
		}

		log.Printf("LGM : link added vxlan-%s type vxlan id %d local %s dstport 4789 nolearning proxy\n", vrf.Name, *vrf.Spec.Vni, vtip)

		linkVxlan, vxlanErr := nlink.LinkByName(ctx, vxlanStr+vrf.Name)
		if vxlanErr != nil {
			log.Printf("LGM : Error in getting the %s\n", vxlanStr+vrf.Name)
			return "", false
		}

		err = nlink.LinkSetMaster(ctx, linkVxlan, linkBr)
		if err != nil {
			log.Printf("LGM : Unable to set the master to vxlan-%s link", vrf.Name)
			return "", false
		}

		log.Printf("LGM: vrf Link vxlan setup master\n")

		linksetupErr = nlink.LinkSetUp(ctx, linkVxlan)
		if linksetupErr != nil {
			log.Printf("LGM : Unable to set link %s UP \n", vrf.Name)
			return "", false
		}
	}
	details := fmt.Sprintf("{\"routingTable\":\"%d\"}", routingTable)
	*vrf.Metadata.RoutingTable[0] = routingTable
	return details, true
}

// setUpSvi sets up the svi
func setUpSvi(svi *infradb.Svi) (string, bool) {
	linkSvi := fmt.Sprintf("%+v-%+v", path.Base(svi.Spec.Vrf), strings.Split(path.Base(svi.Spec.LogicalBridge), "vlan")[1])
	MacAddress := fmt.Sprintf("%+v", svi.Spec.MacAddress)
	// ipMtu := fmt.Sprintf("%+v", ipMtu)
	// vid := strings.Split(path.Base(svi.Spec.LogicalBridge),"vlan")[1]
	vid, _ := strconv.Atoi(strings.Split(path.Base(svi.Spec.LogicalBridge), "vlan")[1])
	brIntf, err := nlink.LinkByName(ctx, brTenant)
	if err != nil {
		log.Printf("LGM : Failed to get link information for %s: %v\n", brTenant, err)
		return "", false
	}
	if err = nlink.BridgeVlanAdd(ctx, brIntf, uint16(vid), false, false, true, false); err != nil {
		log.Printf("LGM : Failed to add VLAN %d to bridge interface %s: %v\n", vid, brTenant, err)
		return "", false
	}
	/*
		CP, err := run([]string{"bridge", "vlan", "add", "dev", brTenant, "vid", vid ,"self"},false)
		if err != 0 {
			log.Printf("LGM: Error in executing command %s %s\n", "bridge vlan add dev ", brTenant)
			log.Printf("%s\n", CP)
			return "", false
		}*/
	log.Printf("LGM Executed : bridge vlan add dev %s vid %d self\n", brTenant, vid)
	vlanLink := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: linkSvi, ParentIndex: brIntf.Attrs().Index}, VlanId: vid}
	if err = nlink.LinkAdd(ctx, vlanLink); err != nil {
		log.Printf("LGM : Failed to add VLAN sub-interface %s: %v\n", linkSvi, err)
		return "", false
	}
	/*
		CP, err = run([]string{"ip", "link", "add", "link", brTenant, "name", linkSvi, "type", "vlan", "id", vid}, false)
		if err != 0 {
			log.Printf("LGM: Error in executing command %s %s %s\n", "ip link add link",brTenant, linkSvi)
			log.Printf("%s\n", CP)
			return "", false
		}*/
	log.Printf("LGM Executed : ip link add link %s name %s type vlan id %d\n", brTenant, linkSvi, vid)
	if err = nlink.LinkSetHardwareAddr(ctx, vlanLink, *svi.Spec.MacAddress); err != nil {
		log.Printf("LGM : Failed to set link %v: %s\n", vlanLink, err)
		return "", false
	}
	/*
		CP, err = run([]string{"ip", "link", "set", linkSvi, "address", MacAddress}, false)
		if err != 0 {
			log.Printf("LGM: Error in executing command %s %s\n", "ip link set", linkSvi)
			log.Printf("%s\n", CP)
			return "", false
		}*/
	log.Printf("LGM Executed : ip link set %s address %s\n", linkSvi, MacAddress)
	vrfIntf, err := nlink.LinkByName(ctx, path.Base(svi.Spec.Vrf))
	if err != nil {
		log.Printf("LGM : Failed to get link information for %s: %v\n", path.Base(svi.Spec.Vrf), err)
		return "", false
	}
	if err = nlink.LinkSetMaster(ctx, vlanLink, vrfIntf); err != nil {
		log.Printf("LGM : Failed to set master for %v: %s\n", vlanLink, err)
		return "", false
	}
	if err = nlink.LinkSetUp(ctx, vlanLink); err != nil {
		log.Printf("LGM : Failed to set up link for %v: %s\n", vlanLink, err)
		return "", false
	}
	if err = nlink.LinkSetMTU(ctx, vlanLink, ipMtu); err != nil {
		log.Printf("LGM : Failed to set MTU for %v: %s\n", vlanLink, err)
		return "", false
	}
	/*
		CP, err = run([]string{"ip", "link", "set", linkSvi, "master", path.Base(svi.Spec.Vrf), "up", "mtu", ipMtu}, false)
		if err != 0 {
			log.Printf("LGM: Error in executing command %s %s\n", "ip link set", linkSvi)
			log.Printf("%s\n", CP)
			return "", false
		}*/
	log.Printf("LGM Executed :  ip link set %s master %s up mtu %d\n", linkSvi, path.Base(svi.Spec.Vrf), ipMtu)
	//Ignoring the error as CI env doesn't allow to write to the filesystem
	command := fmt.Sprintf("net.ipv4.conf.%s.arp_accept=1", linkSvi)
	CP, err1 := run([]string{"sysctl", "-w", command}, false)
	if err1 != 0 {
		log.Printf("LGM: Error in executing command %s %s\n", "sysctl -w net.ipv4.conf.linkSvi.arp_accept=1", linkSvi)
		log.Printf("%s\n", CP)
		//return "", false
	}
	for _, ipIntf := range svi.Spec.GatewayIPs {
		addr := &netlink.Addr{
			IPNet: &net.IPNet{
				IP:   ipIntf.IP,
				Mask: ipIntf.Mask,
			},
		}
		if err := nlink.AddrAdd(ctx, vlanLink, addr); err != nil {
			log.Printf("LGM: Failed to add ip address %v to %v: %v\n", addr, vlanLink, err)
			return "", false
		}
		/*
			IP := fmt.Sprintf("+%v", ipIntf.IP.To4())
			CP, err = run([]string{"ip", "address", "add", IP, "dev", linkSvi}, false)
			if err != 0 {
				log.Printf("LGM: Error in executing command %s %s\n","ip address add",ipIntf.IP.To4())
				log.Printf("%s\n", CP)
				return "", false
			}*/
		log.Printf("LGM Executed :  ip address add %s dev %+v\n", addr, vlanLink)
	}
	return "", true
}

// GenerateMac Generates the random mac
func GenerateMac() net.HardwareAddr {
	buf := make([]byte, 5)
	var mac net.HardwareAddr
	_, err := rand.Read(buf)
	if err != nil {
		log.Printf("failed to generate random mac %+v\n", err)
	}

	// Set the local bit
	//  buf[0] |= 8

	mac = append(mac, 00, buf[0], buf[1], buf[2], buf[3], buf[4])

	return mac
}

// NetMaskToInt convert network mask to int value
func NetMaskToInt(mask int) (netmaskint [4]int64) {
	var binarystring string

	for ii := 1; ii <= mask; ii++ {
		binarystring += "1"
	}
	for ii := 1; ii <= (32 - mask); ii++ {
		binarystring += "0"
	}
	oct1 := binarystring[0:8]
	oct2 := binarystring[8:16]
	oct3 := binarystring[16:24]
	oct4 := binarystring[24:]
	// var netmaskint [4]int
	netmaskint[0], _ = strconv.ParseInt(oct1, 2, 64)
	netmaskint[1], _ = strconv.ParseInt(oct2, 2, 64)
	netmaskint[2], _ = strconv.ParseInt(oct3, 2, 64)
	netmaskint[3], _ = strconv.ParseInt(oct4, 2, 64)

	// netmaskstring = strconv.Itoa(int(ii1)) + "." + strconv.Itoa(int(ii2)) + "." + strconv.Itoa(int(ii3)) + "." + strconv.Itoa(int(ii4))
	return netmaskint
}

/*// validIP structure containing ip and mask
type validIP struct {
	IP   string
	Mask int
}*/

// getIPAddress gets the ip address from link
func getIPAddress(dev string) net.IPNet {
	link, err := nlink.LinkByName(ctx, dev)
	if err != nil {
		log.Printf("LGM: Error in LinkByName %+v\n", err)
		return net.IPNet{
			IP: net.ParseIP("0.0.0.0"),
		}
	}

	addrs, err := nlink.AddrList(ctx, link, netlink.FAMILY_V4) // ip address show
	if err != nil {
		log.Printf("LGM: Error in AddrList\n")
		return net.IPNet{
			IP: net.ParseIP("0.0.0.0"),
		}
	}
	var address = &net.IPNet{
		IP:   net.IPv4(127, 0, 0, 0),
		Mask: net.CIDRMask(8, 32)}
	var addr = &netlink.Addr{IPNet: address}
	var validIps []netlink.Addr
	for index := 0; index < len(addrs); index++ {
		if !addr.Equal(addrs[index]) {
			validIps = append(validIps, addrs[index])
		}
	}
	return *validIps[0].IPNet
}

// tearDownVrf tears down the vrf
func tearDownVrf(vrf *infradb.Vrf) bool {
	Ifname := strings.Split(vrf.Name, "/")
	ifwlen := len(Ifname)
	vrf.Name = Ifname[ifwlen-1]
	link, err1 := nlink.LinkByName(ctx, vrf.Name)
	if err1 != nil {
		log.Printf("LGM : Link %s not found %+v\n", vrf.Name, err1)
		return true
	}

	if vrf.Name == "GRD" {
		return true
	}
	routingTable := *vrf.Metadata.RoutingTable[0]
	// Delete the Linux networking artefacts in reverse order
	if !reflect.ValueOf(vrf.Spec.Vni).IsZero() {
		linkVxlan, linkErr := nlink.LinkByName(ctx, vxlanStr+vrf.Name)
		if linkErr != nil {
			log.Printf("LGM : Link vxlan-%s not found %+v\n", vrf.Name, linkErr)
			return false
		}
		delerr := nlink.LinkDel(ctx, linkVxlan)
		if delerr != nil {
			log.Printf("LGM: Error in delete vxlan %+v\n", delerr)
			return false
		}
		log.Printf("LGM : Delete vxlan-%s\n", vrf.Name)

		linkBr, linkbrErr := nlink.LinkByName(ctx, brStr+vrf.Name)
		if linkbrErr != nil {
			log.Printf("LGM : Link br-%s not found %+v\n", vrf.Name, linkbrErr)
			return false
		}
		delerr = nlink.LinkDel(ctx, linkBr)
		if delerr != nil {
			log.Printf("LGM: Error in delete br %+v\n", delerr)
			return false
		}
		log.Printf("LGM : Delete br-%s\n", vrf.Name)

		routeTable := fmt.Sprintf("%+v", routingTable)
		flusherr := nlink.RouteFlushTable(ctx, routeTable)
		if flusherr != nil {
			log.Printf("LGM: Error in flush table  %+v\n", routeTable)
			return false
		}
		log.Printf("LGM Executed : ip route flush table %s\n", routeTable)

		delerr = nlink.LinkDel(ctx, link)
		if delerr != nil {
			log.Printf("LGM: Error in delete br %+v\n", delerr)
			return false
		}
		log.Printf("LGM :link delete  %s\n", vrf.Name)
	}
	return true
}

// tearDownSvi tears down the svi
func tearDownSvi(svi *infradb.Svi) bool {
	linkSvi := fmt.Sprintf("%+v-%+v", path.Base(svi.Spec.Vrf), strings.Split(path.Base(svi.Spec.LogicalBridge), "vlan")[1])
	Intf, err := nlink.LinkByName(ctx, linkSvi)
	if err != nil {
		log.Printf("LGM : Failed to get link %s: %v\n", linkSvi, err)
		return true
	}
	/*
		CP, err := run([]string{"ifconfig", "-a", linkSvi}, false)
		if err != 0 {
			log.Printf("CP LGM %s\n", CP)
			return true
		}*/
	if err = nlink.LinkDel(ctx, Intf); err != nil {
		log.Printf("LGM : Failed to delete link %s: %v\n", linkSvi, err)
		return false
	}
	log.Printf("LGM: Executed ip link delete %s\n", linkSvi)
	/*
		CP, err = run([]string{"ip", "link", "del", linkSvi}, false)
		if err != 0 {
			log.Printf("LGM: Error in executing command %s %s\n","ip link del", linkSvi)
			return false
		}*/
	return true
}

// tearDownBridge tears down the bridge
func tearDownBridge(lb *infradb.LogicalBridge) bool {
	link := fmt.Sprintf("vxlan-%+v", lb.Spec.VlanID)
	if !reflect.ValueOf(lb.Spec.Vni).IsZero() {
		Intf, err := nlink.LinkByName(ctx, link)
		if err != nil {
			log.Printf("LGM: Failed to get link %s: %v\n", link, err)
			return true
		}
		if err = nlink.LinkDel(ctx, Intf); err != nil {
			log.Printf("LGM : Failed to delete link %s: %v\n", link, err)
			return false
		}
		log.Printf("LGM: Executed ip link delete %s", link)
		/*
			CP, err := run([]string{"ip", "link", "del", link}, false)
			if err != 0 {
				log.Printf("LGM:Error in executing command %s %s\n", "ip link del ", link)
				log.Printf("%s\n", CP)
				return false
			}*/
		return true
	}
	return true
}
