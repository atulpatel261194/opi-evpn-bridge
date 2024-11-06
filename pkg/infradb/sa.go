// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2024 Intel Corporation, or its subsidiaries.
// Copyright (C) 2024 Ericsson AB.

package infradb

import (
	"errors"
	"fmt"
	"log"
	"net"

	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/common"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
	pb "github.com/opiproject/opi-evpn-bridge/pkg/ipsec/gen/go"
)

// SaOperStatus operational Status for Sas
type SaOperStatus int32

const (
	// SaOperStatusUnspecified for Sa unknown state
	SaOperStatusUnspecified SaOperStatus = iota
	// SaOperStatusUp for Sa up state
	SaOperStatusUp = iota
	// SaOperStatusDown for Sa down state
	SaOperStatusDown = iota
	// SaOperStatusToBeDeleted for Sa to be deleted state
	SaOperStatusToBeDeleted = iota
)

// Define a custom type for protocol
type Protocol int

// Define constants for protocol
const (
	IPSecProtoRSVD Protocol = iota
	IPSecProtoESP
	IPSecProtoAH
)

func convertPbToProtocol(p pb.IPSecProtocol) (Protocol, error) {
	switch p {
	case pb.IPSecProtocol_IPSecProtoRSVD:
		return Protocol(IPSecProtoRSVD), nil
	case pb.IPSecProtocol_IPSecProtoESP:
		return Protocol(IPSecProtoESP), nil
	case pb.IPSecProtocol_IPSecProtoAH:
		return Protocol(IPSecProtoAH), nil
	default:
		err := fmt.Errorf("convertPbToProtocol(): Unknown protocol %+v", p)
		return -1, err
	}
}

// Define a custom type for mode
type Mode int

// Define constants for mode
const (
	None Mode = iota
	Transport
	Tunnel
	Beet
	Pass
	Drop
)

func convertPbToMode(m pb.IPSecMode) (Mode, error) {
	switch m {
	case pb.IPSecMode_MODE_NONE:
		return Mode(None), nil
	case pb.IPSecMode_MODE_TRANSPORT:
		return Mode(Transport), nil
	case pb.IPSecMode_MODE_TUNNEL:
		return Mode(Tunnel), nil
	case pb.IPSecMode_MODE_BEET:
		return Mode(Beet), nil
	case pb.IPSecMode_MODE_PASS:
		return Mode(Pass), nil
	case pb.IPSecMode_MODE_DROP:
		return Mode(Drop), nil
	default:
		err := fmt.Errorf("convertPbToMode(): Unknown ipsec mode %+v", m)
		return -1, err
	}
}

// Define a custom type for crypto algorithm for encryption
type CryptoAlg int

// Define constants for crypto algorithm for encryption
const (
	RSVD CryptoAlg = iota
	NULL
	AES_CBC
	AES_CTR
	AES_CCM_8
	AES_CCM_12
	AES_CCM_16
	AES_GCM_8
	AES_GCM_12
	AES_GCM_16
	NULL_AUTH_AES_GMAC
	CHACHA20_POLY1305
)

func convertPbToCryptoAlg(c pb.CryptoAlgorithm) (CryptoAlg, error) {
	switch c {
	case pb.CryptoAlgorithm_ENCR_RSVD:
		return CryptoAlg(RSVD), nil
	case pb.CryptoAlgorithm_ENCR_NULL:
		return CryptoAlg(NULL), nil
	case pb.CryptoAlgorithm_ENCR_AES_CBC:
		return CryptoAlg(AES_CBC), nil
	case pb.CryptoAlgorithm_ENCR_AES_CTR:
		return CryptoAlg(AES_CTR), nil
	case pb.CryptoAlgorithm_ENCR_AES_CCM_8:
		return CryptoAlg(AES_CCM_8), nil
	case pb.CryptoAlgorithm_ENCR_AES_CCM_12:
		return CryptoAlg(AES_CCM_12), nil
	case pb.CryptoAlgorithm_ENCR_AES_CCM_16:
		return CryptoAlg(AES_CCM_16), nil
	case pb.CryptoAlgorithm_ENCR_AES_GCM_8:
		return CryptoAlg(AES_GCM_8), nil
	case pb.CryptoAlgorithm_ENCR_AES_GCM_12:
		return CryptoAlg(AES_GCM_12), nil
	case pb.CryptoAlgorithm_ENCR_AES_GCM_16:
		return CryptoAlg(AES_GCM_16), nil
	case pb.CryptoAlgorithm_ENCR_NULL_AUTH_AES_GMAC:
		return CryptoAlg(NULL_AUTH_AES_GMAC), nil
	case pb.CryptoAlgorithm_ENCR_CHACHA20_POLY1305:
		return CryptoAlg(CHACHA20_POLY1305), nil
	default:
		err := fmt.Errorf("convertPbToCryptoAlg(): Unknown ipsec crypto algorithm %+v", c)
		return -1, err
	}
}

// Define a custom type for crypto algorithm for authentication
type IntegAlg int

// Define constants for crypto algorithm for authentication
const (
	NONE IntegAlg = iota
	HMAC_SHA1_96
	AES_XCBC_96
	AES_CMAC_96
	AES_128_GMAC
	AES_192_GMAC
	AES_256_GMAC
	HMAC_SHA2_256_128
	HMAC_SHA2_384_192
	HMAC_SHA2_512_256
	UNDEFINED
)

func convertPbToIntegAlg(c pb.IntegAlgorithm) (IntegAlg, error) {
	switch c {
	case pb.IntegAlgorithm_NONE:
		return IntegAlg(None), nil
	case pb.IntegAlgorithm_AUTH_HMAC_SHA1_96:
		return IntegAlg(HMAC_SHA1_96), nil
	case pb.IntegAlgorithm_AUTH_AES_XCBC_96:
		return IntegAlg(AES_XCBC_96), nil
	case pb.IntegAlgorithm_AUTH_AES_CMAC_96:
		return IntegAlg(AES_CMAC_96), nil
	case pb.IntegAlgorithm_AUTH_AES_128_GMAC:
		return IntegAlg(AES_128_GMAC), nil
	case pb.IntegAlgorithm_AUTH_AES_192_GMAC:
		return IntegAlg(AES_192_GMAC), nil
	case pb.IntegAlgorithm_AUTH_AES_256_GMAC:
		return IntegAlg(AES_256_GMAC), nil
	case pb.IntegAlgorithm_AUTH_HMAC_SHA2_256_128:
		return IntegAlg(HMAC_SHA2_256_128), nil
	case pb.IntegAlgorithm_AUTH_HMAC_SHA2_384_192:
		return IntegAlg(HMAC_SHA2_384_192), nil
	case pb.IntegAlgorithm_AUTH_HMAC_SHA2_512_256:
		return IntegAlg(HMAC_SHA2_512_256), nil
	case pb.IntegAlgorithm_AUTH_UNDEFINED:
		return IntegAlg(UNDEFINED), nil
	default:
		err := fmt.Errorf("convertPbToIntegAlg(): Unknown ipsec authentication algorithm %+v", c)
		return -1, err
	}
}

// Define a custom type for DSCP header field
type DscpCopy int

// Define constants for DSCP header field
const (
	OUT_ONLY DscpCopy = iota
	IN_ONLY
	YES
	NO
)

func convertPbToDscpCopy(d pb.DSCPCopy) (DscpCopy, error) {
	switch d {
	case pb.DSCPCopy_DSCP_COPY_OUT_ONLY:
		return DscpCopy(OUT_ONLY), nil
	case pb.DSCPCopy_DSCP_COPY_IN_ONLY:
		return DscpCopy(IN_ONLY), nil
	case pb.DSCPCopy_DSCP_COPY_YES:
		return DscpCopy(YES), nil
	case pb.DSCPCopy_DSCP_COPY_NO:
		return DscpCopy(NO), nil
	default:
		err := fmt.Errorf("convertPbToDscpCopy(): Unknown ipsec DscpCopy %+v", d)
		return -1, err
	}
}

func convertPbToBoolean(b pb.Bool) (bool, error) {
	switch b {
	case pb.Bool_TRUE:
		return true, nil
	case pb.Bool_FALSE:
		return false, nil
	default:
		err := fmt.Errorf("convertPbToBoolean(): Unknown boolean value %+v", b)
		return false, err
	}
}

type Lifetime struct {
	Life   uint64
	Rekey  uint64
	Jitter uint64
}

func NewLifetime(lt *pb.LifeTime) *Lifetime {
	if lt == nil {
		return nil
	}
	return &Lifetime{
		Life:   lt.Life,
		Rekey:  lt.Rekey,
		Jitter: lt.Jitter,
	}
}

type LifetimeCfg struct {
	Time    *Lifetime
	Bytes   *Lifetime
	Packets *Lifetime
}

func NewLifetimeCfg(time, bytes, packets *Lifetime) *LifetimeCfg {
	if time == nil && bytes == nil && packets == nil {
		return nil
	}
	return &LifetimeCfg{
		Time:    time,
		Bytes:   bytes,
		Packets: packets,
	}
}

// SaStatus holds Sa Status
type SaStatus struct {
	SaOperStatus SaOperStatus
	Components   []common.Component
}

// SaSpec holds Sa Spec
type SaSpec struct {
	SrcIP        *net.IP
	DstIP        *net.IP
	Spi          *uint32
	Protocol     Protocol
	IfId         uint32
	Mode         Mode
	Interface    string
	LifetimeCfg  *LifetimeCfg
	EncAlg       CryptoAlg
	EncKey       []byte
	IntAlg       IntegAlg
	IntKey       []byte
	ReplayWindow uint32
	UdpEncap     bool
	Esn          bool
	CopyDf       bool
	CopyEcn      bool
	CopyDscp     DscpCopy
	Inbound      bool
	Vrf          string
}

// SaMetadata holds Sa Metadata
type SaMetadata struct{}

// Sa holds SA info
type Sa struct {
	Name            string
	Spec            *SaSpec
	Status          *SaStatus
	Metadata        *SaMetadata
	Vrf             string
	Index           uint32
	OldVersions     []string
	ResourceVersion string
}

// NewSa creates new SA object from protobuf message
func NewSa(name string, sa *pb.AddSAReq) (*Sa, error) {
	components := make([]common.Component, 0)

	/*poolIndex := saIdxPool.getId(name)
	if poolIndex == nil {
		return nil, error
	}*/

	srcIP := net.ParseIP(sa.SaId.Src)
	if srcIP == nil {
		err := fmt.Errorf("NewSa(): Incorrect src IP format %+v", sa.SaId.Src)
		return nil, err
	}

	dstIP := net.ParseIP(sa.SaId.Dst)
	if dstIP == nil {
		err := fmt.Errorf("NewSa(): Incorrect dst IP format %+v", sa.SaId.Dst)
		return nil, err
	}

	proto, err := convertPbToProtocol(sa.SaId.Proto)
	if err != nil {
		return nil, err
	}
	mode, err := convertPbToMode(sa.SaData.Mode)
	if err != nil {
		return nil, err
	}

	LTtime := NewLifetime(sa.SaData.Lifetime.Time)
	LTbytes := NewLifetime(sa.SaData.Lifetime.Bytes)
	LTpackets := NewLifetime(sa.SaData.Lifetime.Packets)
	lifetimeCfg := NewLifetimeCfg(LTtime, LTbytes, LTpackets)

	encAlg, err := convertPbToCryptoAlg(sa.SaData.EncAlg)
	if err != nil {
		return nil, err
	}

	intAlg, err := convertPbToIntegAlg(sa.SaData.IntAlg)
	if err != nil {
		return nil, err
	}

	udpEncap, err := convertPbToBoolean(sa.SaData.Encap)
	if err != nil {
		return nil, err
	}

	esn, err := convertPbToBoolean(sa.SaData.Esn)
	if err != nil {
		return nil, err
	}

	copyDf, err := convertPbToBoolean(sa.SaData.CopyDf)
	if err != nil {
		return nil, err
	}

	copyEcn, err := convertPbToBoolean(sa.SaData.CopyEcn)
	if err != nil {
		return nil, err
	}

	copyDscp, err := convertPbToDscpCopy(sa.SaData.CopyDscp)
	if err != nil {
		return nil, err
	}

	inbound, err := convertPbToBoolean(sa.SaData.Inbound)
	if err != nil {
		return nil, err
	}

	subscribers := eventbus.EBus.GetSubscribers("sa")
	if len(subscribers) == 0 {
		log.Println("NewSa(): No subscribers for SA objects")
		return nil, errors.New("no subscribers found for SAs")
	}

	for _, sub := range subscribers {
		component := common.Component{Name: sub.Name, CompStatus: common.ComponentStatusPending, Details: ""}
		components = append(components, component)
	}

	return &Sa{
		Name: name,
		Spec: &SaSpec{
			SrcIP:        &srcIP,
			DstIP:        &dstIP,
			Spi:          &sa.SaId.Spi,
			Protocol:     proto,
			IfId:         sa.SaId.IfId,
			Mode:         mode,
			Interface:    sa.SaData.Interface,
			LifetimeCfg:  lifetimeCfg,
			EncAlg:       encAlg,
			EncKey:       sa.SaData.EncKey,
			IntAlg:       intAlg,
			IntKey:       sa.SaData.IntKey,
			ReplayWindow: sa.SaData.ReplayWindow,
			UdpEncap:     udpEncap,
			Esn:          esn,
			CopyDf:       copyDf,
			CopyEcn:      copyEcn,
			CopyDscp:     copyDscp,
			Inbound:      inbound,
		},
		Status: &SaStatus{
			SaOperStatus: SaOperStatus(SaOperStatusDown),

			Components: components,
		},
		Metadata: &SaMetadata{},
		//Index: poolIndex,
		ResourceVersion: generateVersion(),
	}, nil

}

// setComponentState set the stat of the component
func (in *Sa) setComponentState(component common.Component) {
	saComponents := in.Status.Components
	for i, comp := range saComponents {
		if comp.Name == component.Name {
			in.Status.Components[i] = component
			break
		}
	}
}

// checkForAllSuccess check if all the components are in Success state
func (in *Sa) checkForAllSuccess() bool {
	for _, comp := range in.Status.Components {
		if comp.CompStatus != common.ComponentStatusSuccess {
			return false
		}
	}
	return true
}

// parseMeta parse metadata
func (in *Sa) parseMeta(saMeta *SaMetadata) {
	if saMeta != nil {
		in.Metadata = saMeta
	}
}

// prepareObjectsForReplay prepares an object for replay by setting the unsuccessful components
// in pending state and returning a list of the components that need to be contacted for the
// replay of the particular object that called the function.
func (in *Sa) prepareObjectsForReplay(componentName string, saSubs []*eventbus.Subscriber) []*eventbus.Subscriber {
	// We assume that the list of Components that are returned
	// from DB is ordered based on the priority as that was the
	// way that has been stored in the DB in first place.
	saComponents := in.Status.Components
	tempSubs := []*eventbus.Subscriber{}
	for i, comp := range saComponents {
		if comp.Name == componentName || comp.CompStatus != common.ComponentStatusSuccess {
			in.Status.Components[i] = common.Component{Name: comp.Name, CompStatus: common.ComponentStatusPending, Details: ""}
			tempSubs = append(tempSubs, saSubs[i])
		}
	}
	if in.Status.SaOperStatus == SaOperStatusUp {
		in.Status.SaOperStatus = SaOperStatusDown
	}

	in.ResourceVersion = generateVersion()
	return tempSubs
}
