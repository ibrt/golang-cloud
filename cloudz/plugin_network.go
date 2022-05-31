package cloudz

import (
	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	gocf "github.com/awslabs/goformation/v6/cloudformation"
	goec2 "github.com/awslabs/goformation/v6/cloudformation/ec2"
	dctypes "github.com/docker/cli/cli/compose/types"
	"github.com/ibrt/golang-bites/boolz"
	"github.com/ibrt/golang-bites/stringz"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-validation/vz"
)

// Network constants.
const (
	NetworkPluginDisplayName                      = "Network"
	NetworkPluginName                             = "network"
	NetworkRefVPC                                 = CloudRef("v")
	NetworkRefInternetGateway                     = CloudRef("ig")
	NetworkRefVPCGatewayAttachment                = CloudRef("vig")
	NetworkRefRouteTablePublic                    = CloudRef("rt-pub")
	NetworkRefRoutePublic                         = CloudRef("r-pub")
	NetworkRefSubnetPublicA                       = CloudRef("s-pub-a")
	NetworkRefSubnetRouteTableAssociationPublicA  = CloudRef("srt-pub-a")
	NetworkRefSubnetPublicB                       = CloudRef("s-pub-b")
	NetworkRefSubnetRouteTableAssociationPublicB  = CloudRef("srt-pub-b")
	NetworkRefEIPA                                = CloudRef("eip-a")
	NetworkRefNATGatewayA                         = CloudRef("ng-a")
	NetworkRefRouteTablePrivateA                  = CloudRef("rt-pri-a")
	NetworkRefRoutePrivateA                       = CloudRef("r-pri-a")
	NetworkRefSubnetPrivateA                      = CloudRef("s-pri-a")
	NetworkRefSubnetRouteTableAssociationPrivateA = CloudRef("s-rt-pri-a")
	NetworkRefEIPB                                = CloudRef("eip-b")
	NetworkRefNATGatewayB                         = CloudRef("ng-b")
	NetworkRefRouteTablePrivateB                  = CloudRef("rt-pri-b")
	NetworkRefRoutePrivateB                       = CloudRef("r-pri-b")
	NetworkRefSubnetPrivateB                      = CloudRef("s-pri-b")
	NetworkRefSubnetRouteTableAssociationPrivateB = CloudRef("srt-pri-b")
	NetworkRefSecurityGroup                       = CloudRef("sg")
	NetworkRefSecurityGroupIngress                = CloudRef("sgi")
	NetworkAttAllocationID                        = CloudAtt("AllocationId")
	NetworkAttCIDRBlock                           = CloudAtt("CidrBlock")
	NetworkAttCIDRBlockAssociations               = CloudAtt("CidrBlockAssociations")
	NetworkAttDefaultNetworkACL                   = CloudAtt("DefaultNetworkAcl")
	NetworkAttDefaultSecurityGroup                = CloudAtt("DefaultSecurityGroup")
	NetworkAttGroupID                             = CloudAtt("GroupId")
	NetworkAttID                                  = CloudAtt("Id")
	NetworkAttInternetGatewayID                   = CloudAtt("InternetGatewayId")
	NetworkAttNetworkACLAssociationID             = CloudAtt("NetworkAclAssociationId")
	NetworkAttRouteTableID                        = CloudAtt("RouteTableId")
	NetworkAttSubnetID                            = CloudAtt("SubnetId")
	NetworkAttVPCID                               = CloudAtt("VpcId")

	CIDRAllDestinations = "0.0.0.0/0"
	CIDRVPC             = "10.0.0.0/16"
	CIDRSubnetPublicA   = "10.0.0.0/19"
	CIDRSubnetPublicB   = "10.0.32.0/19"
	CIDRSubnetPrivateA  = "10.0.64.0/19"
	CIDRSubnetPrivateB  = "10.0.96.0/19"
)

var (
	_ Network = &networkImpl{}
	_ Plugin  = &networkImpl{}
)

// NetworkConfigFunc returns the network config for a given Stage.
type NetworkConfigFunc func(Stage, *NetworkDependencies) *NetworkConfig

// NetworkEventHookFunc describes a network event hook.
type NetworkEventHookFunc func(Network, Event, string)

// NetworkConfig describes the network config.
type NetworkConfig struct {
	Stage     Stage `validate:"required"`
	EventHook NetworkEventHookFunc
}

// MustValidate validates the network config.
func (c *NetworkConfig) MustValidate(_ StageTarget) {
	vz.MustValidateStruct(c)
}

// NetworkDependencies describes the network dependencies.
type NetworkDependencies struct {
	OtherDependencies OtherDependencies
}

// MustValidate validates the network dependencies.
func (d *NetworkDependencies) MustValidate() {
	vz.MustValidateStruct(d)
}

// NetworkCloudMetadata describes the network cloud metadata.
type NetworkCloudMetadata struct {
	Exports CloudExports
}

// Network describes a network.
type Network interface {
	Plugin
	GetConfig() *NetworkConfig
	GetCloudMetadata(require bool) *NetworkCloudMetadata
}

type networkImpl struct {
	cfgFunc       NetworkConfigFunc
	deps          *NetworkDependencies
	cfg           *NetworkConfig
	cloudMetadata *NetworkCloudMetadata
}

// NewNetwork initializes a new Network.
func NewNetwork(cfgFunc NetworkConfigFunc, deps *NetworkDependencies) Network {
	deps.MustValidate()

	return &networkImpl{
		cfgFunc: cfgFunc,
		deps:    deps,
	}
}

// GetDisplayName implements the Plugin interface.
func (*networkImpl) GetDisplayName() string {
	return NetworkPluginDisplayName
}

// GetName implements the Plugin interface.
func (p *networkImpl) GetName() string {
	return NetworkPluginName
}

// GetInstanceName implements the Plugin interface.
func (p *networkImpl) GetInstanceName() *string {
	return nil
}

// GetDependenciesMap implements the Plugin interface.
func (p *networkImpl) GetDependenciesMap() map[Plugin]struct{} {
	dependenciesMap := map[Plugin]struct{}{}
	for _, otherDependency := range p.deps.OtherDependencies {
		dependenciesMap[otherDependency] = struct{}{}
	}
	return dependenciesMap
}

// Configure implements the Plugin interface.
func (p *networkImpl) Configure(stage Stage) {
	p.cfg = p.cfgFunc(stage, p.deps)
	p.cfg.MustValidate(stage.GetTarget())
}

// GetStage implements the Plugin interface.
func (p *networkImpl) GetStage() Stage {
	errorz.Assertf(p.cfg != nil, "plugin not configured", errorz.Prefix(NetworkPluginName))
	return p.cfg.Stage
}

// GetConfig implements the Network interface.
func (p *networkImpl) GetConfig() *NetworkConfig {
	return p.cfg
}

// GetCloudMetadata implements the Network interface.
func (p *networkImpl) GetCloudMetadata(require bool) *NetworkCloudMetadata {
	errorz.Assertf(!require || p.cloudMetadata != nil, "cloud not deployed", errorz.Prefix(NetworkPluginName))
	return p.cloudMetadata
}

// IsDeployed implements the Plugin interface.
func (p *networkImpl) IsDeployed() bool {
	return p.cloudMetadata != nil
}

// UpdateLocalTemplate implements the Plugin interface.
func (p *networkImpl) UpdateLocalTemplate(_ *dctypes.Config, _ string) {
	// nothing to do here
}

// GetCloudTemplate implements the Plugin interface.
func (p *networkImpl) GetCloudTemplate(_ string) *gocf.Template {
	tpl := gocf.NewTemplate()

	tpl.Resources[NetworkRefVPC.Ref()] = &goec2.VPC{
		CidrBlock:          CIDRVPC,
		EnableDnsHostnames: boolz.Ptr(true),
		EnableDnsSupport:   boolz.Ptr(true),
		Tags:               CloudGetDefaultTags(NetworkRefVPC.Name(p)),
	}
	CloudAddExpRef(tpl, p, NetworkRefVPC)
	CloudAddExpGetAtt(tpl, p, NetworkRefVPC, NetworkAttCIDRBlock)
	CloudAddExpGetAtt(tpl, p, NetworkRefVPC, NetworkAttCIDRBlockAssociations)
	CloudAddExpGetAtt(tpl, p, NetworkRefVPC, NetworkAttDefaultNetworkACL)
	CloudAddExpGetAtt(tpl, p, NetworkRefVPC, NetworkAttDefaultSecurityGroup)

	tpl.Resources[NetworkRefInternetGateway.Ref()] = &goec2.InternetGateway{
		Tags: CloudGetDefaultTags(NetworkRefInternetGateway.Name(p)),
	}
	CloudAddExpRef(tpl, p, NetworkRefInternetGateway)
	CloudAddExpGetAtt(tpl, p, NetworkRefInternetGateway, NetworkAttInternetGatewayID)

	tpl.Resources[NetworkRefVPCGatewayAttachment.Ref()] = &goec2.VPCGatewayAttachment{
		InternetGatewayId: stringz.Ptr(gocf.Ref(NetworkRefInternetGateway.Ref())),
		VpcId:             gocf.Ref(NetworkRefVPC.Ref()),
	}
	CloudAddExpRef(tpl, p, NetworkRefVPCGatewayAttachment)

	tpl.Resources[NetworkRefRouteTablePublic.Ref()] = &goec2.RouteTable{
		VpcId: gocf.Ref(NetworkRefVPC.Ref()),
		Tags:  CloudGetDefaultTags(NetworkRefRouteTablePublic.Name(p)),
	}
	CloudAddExpRef(tpl, p, NetworkRefRouteTablePublic)
	CloudAddExpGetAtt(tpl, p, NetworkRefRouteTablePublic, NetworkAttRouteTableID)

	tpl.Resources[NetworkRefRoutePublic.Ref()] = &goec2.Route{
		AWSCloudFormationDependsOn: []string{
			NetworkRefVPCGatewayAttachment.Ref(),
		},
		DestinationCidrBlock: stringz.Ptr(CIDRAllDestinations),
		GatewayId:            stringz.Ptr(gocf.Ref(NetworkRefInternetGateway.Ref())),
		RouteTableId:         gocf.Ref(NetworkRefRouteTablePublic.Ref()),
	}
	CloudAddExpRef(tpl, p, NetworkRefRoutePublic)

	tpl.Resources[NetworkRefSubnetPublicA.Ref()] = &goec2.Subnet{
		AvailabilityZone:    stringz.Ptr(p.cfg.Stage.GetConfig().App.GetConfig().AWSConfig.Region + "a"),
		CidrBlock:           stringz.Ptr(CIDRSubnetPublicA),
		MapPublicIpOnLaunch: boolz.Ptr(true),
		VpcId:               gocf.Ref(NetworkRefVPC.Ref()),
		Tags:                CloudGetDefaultTags(NetworkRefSubnetPublicA.Name(p)),
	}
	CloudAddExpRef(tpl, p, NetworkRefSubnetPublicA)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetPublicA, NetworkAttNetworkACLAssociationID)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetPublicA, NetworkAttSubnetID)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetPublicA, NetworkAttVPCID)

	tpl.Resources[NetworkRefSubnetRouteTableAssociationPublicA.Ref()] = &goec2.SubnetRouteTableAssociation{
		RouteTableId: gocf.Ref(NetworkRefRouteTablePublic.Ref()),
		SubnetId:     gocf.Ref(NetworkRefSubnetPublicA.Ref()),
	}
	CloudAddExpRef(tpl, p, NetworkRefSubnetRouteTableAssociationPublicA)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetRouteTableAssociationPublicA, NetworkAttID)

	tpl.Resources[NetworkRefSubnetPublicB.Ref()] = &goec2.Subnet{
		AvailabilityZone:    stringz.Ptr(p.cfg.Stage.GetConfig().App.GetConfig().AWSConfig.Region + "b"),
		CidrBlock:           stringz.Ptr(CIDRSubnetPublicB),
		MapPublicIpOnLaunch: boolz.Ptr(true),
		VpcId:               gocf.Ref(NetworkRefVPC.Ref()),
		Tags:                CloudGetDefaultTags(NetworkRefSubnetPublicB.Name(p)),
	}
	CloudAddExpRef(tpl, p, NetworkRefSubnetPublicB)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetPublicB, NetworkAttNetworkACLAssociationID)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetPublicB, NetworkAttSubnetID)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetPublicB, NetworkAttVPCID)

	tpl.Resources[NetworkRefSubnetRouteTableAssociationPublicB.Ref()] = &goec2.SubnetRouteTableAssociation{
		RouteTableId: gocf.Ref(NetworkRefRouteTablePublic.Ref()),
		SubnetId:     gocf.Ref(NetworkRefSubnetPublicB.Ref()),
	}
	CloudAddExpRef(tpl, p, NetworkRefSubnetRouteTableAssociationPublicB)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetRouteTableAssociationPublicB, NetworkAttID)

	tpl.Resources[NetworkRefEIPA.Ref()] = &goec2.EIP{
		Domain: stringz.Ptr("vpc"),
		Tags:   CloudGetDefaultTags(NetworkRefEIPA.Name(p)),
	}
	CloudAddExpRef(tpl, p, NetworkRefEIPA)
	CloudAddExpGetAtt(tpl, p, NetworkRefEIPA, NetworkAttAllocationID)

	tpl.Resources[NetworkRefNATGatewayA.Ref()] = &goec2.NatGateway{
		AllocationId: stringz.Ptr(gocf.GetAtt(NetworkRefEIPA.Ref(), NetworkAttAllocationID.Ref())),
		SubnetId:     gocf.Ref(NetworkRefSubnetPublicA.Ref()),
		Tags:         CloudGetDefaultTags(NetworkRefNATGatewayA.Name(p)),
	}
	CloudAddExpRef(tpl, p, NetworkRefNATGatewayA)

	if p.cfg.Stage.GetMode().IsProduction() {
		tpl.Resources[NetworkRefEIPB.Ref()] = &goec2.EIP{
			Domain: stringz.Ptr("vpc"),
			Tags:   CloudGetDefaultTags(NetworkRefEIPB.Name(p)),
		}
		CloudAddExpRef(tpl, p, NetworkRefEIPB)
		CloudAddExpGetAtt(tpl, p, NetworkRefEIPB, NetworkAttAllocationID)

		tpl.Resources[NetworkRefNATGatewayB.Ref()] = &goec2.NatGateway{
			AllocationId: stringz.Ptr(gocf.GetAtt(NetworkRefEIPB.Ref(), NetworkAttAllocationID.Ref())),
			SubnetId:     gocf.Ref(NetworkRefSubnetPublicB.Ref()),
			Tags:         CloudGetDefaultTags(NetworkRefNATGatewayB.Name(p)),
		}
		CloudAddExpRef(tpl, p, NetworkRefNATGatewayB)
	}

	tpl.Resources[NetworkRefRouteTablePrivateA.Ref()] = &goec2.RouteTable{
		VpcId: gocf.Ref(NetworkRefVPC.Ref()),
		Tags:  CloudGetDefaultTags(NetworkRefRouteTablePrivateA.Name(p)),
	}
	CloudAddExpRef(tpl, p, NetworkRefRouteTablePrivateA)
	CloudAddExpGetAtt(tpl, p, NetworkRefRouteTablePrivateA, NetworkAttRouteTableID)

	tpl.Resources[NetworkRefRoutePrivateA.Ref()] = &goec2.Route{
		DestinationCidrBlock: stringz.Ptr(CIDRAllDestinations),
		NatGatewayId:         stringz.Ptr(gocf.Ref(NetworkRefNATGatewayA.Ref())),
		RouteTableId:         gocf.Ref(NetworkRefRouteTablePrivateA.Ref()),
	}
	CloudAddExpRef(tpl, p, NetworkRefRoutePrivateA)

	tpl.Resources[NetworkRefSubnetPrivateA.Ref()] = &goec2.Subnet{
		AvailabilityZone: stringz.Ptr(p.cfg.Stage.GetConfig().App.GetConfig().AWSConfig.Region + "a"),
		CidrBlock:        stringz.Ptr(CIDRSubnetPrivateA),
		VpcId:            gocf.Ref(NetworkRefVPC.Ref()),
		Tags:             CloudGetDefaultTags(NetworkRefSubnetPrivateA.Name(p)),
	}
	CloudAddExpRef(tpl, p, NetworkRefSubnetPrivateA)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetPrivateA, NetworkAttNetworkACLAssociationID)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetPrivateA, NetworkAttSubnetID)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetPrivateA, NetworkAttVPCID)

	tpl.Resources[NetworkRefSubnetRouteTableAssociationPrivateA.Ref()] = &goec2.SubnetRouteTableAssociation{
		RouteTableId: gocf.Ref(NetworkRefRouteTablePrivateA.Ref()),
		SubnetId:     gocf.Ref(NetworkRefSubnetPrivateA.Ref()),
	}
	CloudAddExpRef(tpl, p, NetworkRefSubnetRouteTableAssociationPrivateA)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetRouteTableAssociationPrivateA, NetworkAttID)

	tpl.Resources[NetworkRefRouteTablePrivateB.Ref()] = &goec2.RouteTable{
		VpcId: gocf.Ref(NetworkRefVPC.Ref()),
		Tags:  CloudGetDefaultTags(NetworkRefRouteTablePrivateB.Name(p)),
	}
	CloudAddExpRef(tpl, p, NetworkRefRouteTablePrivateB)
	CloudAddExpGetAtt(tpl, p, NetworkRefRouteTablePrivateB, NetworkAttRouteTableID)

	tpl.Resources[NetworkRefRoutePrivateB.Ref()] = &goec2.Route{
		DestinationCidrBlock: stringz.Ptr(CIDRAllDestinations),
		NatGatewayId: func() *string {
			if p.cfg.Stage.GetMode().IsProduction() {
				return stringz.Ptr(gocf.Ref(NetworkRefNATGatewayB.Ref()))
			}
			return stringz.Ptr(gocf.Ref(NetworkRefNATGatewayA.Ref()))
		}(),
		RouteTableId: gocf.Ref(NetworkRefRouteTablePrivateB.Ref()),
	}
	CloudAddExpRef(tpl, p, NetworkRefRoutePrivateB)

	tpl.Resources[NetworkRefSubnetPrivateB.Ref()] = &goec2.Subnet{
		AvailabilityZone: stringz.Ptr(p.cfg.Stage.GetConfig().App.GetConfig().AWSConfig.Region + "b"),
		CidrBlock:        stringz.Ptr(CIDRSubnetPrivateB),
		VpcId:            gocf.Ref(NetworkRefVPC.Ref()),
		Tags:             CloudGetDefaultTags(NetworkRefSubnetPrivateB.Name(p)),
	}
	CloudAddExpRef(tpl, p, NetworkRefSubnetPrivateB)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetPrivateB, NetworkAttNetworkACLAssociationID)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetPrivateB, NetworkAttSubnetID)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetPrivateB, NetworkAttVPCID)

	tpl.Resources[NetworkRefSubnetRouteTableAssociationPrivateB.Ref()] = &goec2.SubnetRouteTableAssociation{
		RouteTableId: gocf.Ref(NetworkRefRouteTablePrivateB.Ref()),
		SubnetId:     gocf.Ref(NetworkRefSubnetPrivateB.Ref()),
	}
	CloudAddExpRef(tpl, p, NetworkRefSubnetRouteTableAssociationPrivateB)
	CloudAddExpGetAtt(tpl, p, NetworkRefSubnetRouteTableAssociationPrivateB, NetworkAttID)

	tpl.Resources[NetworkRefSecurityGroup.Ref()] = &goec2.SecurityGroup{
		GroupDescription: NetworkRefSecurityGroup.Name(p),
		GroupName:        stringz.Ptr(NetworkRefVPC.Name(p)),
		SecurityGroupEgress: &[]goec2.SecurityGroup_Egress{
			{
				IpProtocol: "-1",
				CidrIp:     stringz.Ptr(CIDRAllDestinations),
			},
		},
		VpcId: stringz.Ptr(gocf.Ref(NetworkRefVPC.Ref())),
		Tags:  CloudGetDefaultTags(NetworkRefSecurityGroup.Name(p)),
	}
	CloudAddExpRef(tpl, p, NetworkRefSecurityGroup)
	CloudAddExpGetAtt(tpl, p, NetworkRefSecurityGroup, NetworkAttGroupID)
	CloudAddExpGetAtt(tpl, p, NetworkRefSecurityGroup, NetworkAttVPCID)

	tpl.Resources[NetworkRefSecurityGroupIngress.Ref()] = &goec2.SecurityGroupIngress{
		GroupId:               stringz.Ptr(gocf.Ref(NetworkRefSecurityGroup.Ref())),
		IpProtocol:            "-1",
		SourceSecurityGroupId: stringz.Ptr(gocf.Ref(NetworkRefSecurityGroup.Ref())),
	}

	return tpl
}

// UpdateCloudMetadata implements the Plugin interface.
func (p *networkImpl) UpdateCloudMetadata(stack *awscft.Stack) {
	p.cloudMetadata = &NetworkCloudMetadata{
		Exports: NewCloudExports(stack),
	}
}

// EventHook implements the Plugin interface.
func (p *networkImpl) EventHook(event Event, buildDirPath string) {
	if p.cfg.EventHook != nil {
		p.cfg.EventHook(p, event, buildDirPath)
	}
}
