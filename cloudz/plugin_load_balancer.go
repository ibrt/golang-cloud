package cloudz

import (
	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	gocf "github.com/awslabs/goformation/v6/cloudformation"
	goec2 "github.com/awslabs/goformation/v6/cloudformation/ec2"
	goelbv2 "github.com/awslabs/goformation/v6/cloudformation/elasticloadbalancingv2"
	dctypes "github.com/docker/cli/cli/compose/types"
	"github.com/ibrt/golang-bites/numeric/intz"
	"github.com/ibrt/golang-bites/stringz"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-validation/vz"

	"github.com/ibrt/golang-cloud/cloudz/internal/assets"
)

// Load balancer constants.
const (
	LoadBalancerPluginDisplayName            = "Load Balancer"
	LoadBalancerPluginName                   = "load-balancer"
	LoadBalancerRefLoadBalancer              = CloudRef("lb")
	LoadBalancerRefListenerHTTP              = CloudRef("l")
	LoadBalancerRefListenerHTTPS             = CloudRef("l-s")
	LoadBalancerRefSecurityGroupIngressHTTP  = CloudRef("sgi")
	LoadBalancerRefSecurityGroupIngressHTTPS = CloudRef("sgi-s")
	LoadBalancerAttCanonicalHostedZoneID     = CloudAtt("CanonicalHostedZoneID")
	LoadBalancerAttDNSName                   = CloudAtt("DNSName")
	LoadBalancerAttLoadBalancerFullName      = CloudAtt("LoadBalancerFullName")
	LoadBalancerAttLoadBalancerName          = CloudAtt("LoadBalancerName")
	LoadBalancerAttSecurityGroups            = CloudAtt("SecurityGroups")
	LoadBalancerAttListenerArn               = CloudAtt("ListenerArn")
)

var (
	_ LoadBalancer = &loadBalancerImpl{}
	_ Plugin       = &loadBalancerImpl{}
)

// LoadBalancerConfigFunc returns the load balancer config for a given Stage.
type LoadBalancerConfigFunc func(Stage, *LoadBalancerDependencies) *LoadBalancerConfig

// LoadBalancerConfig describes the load balancer config.
type LoadBalancerConfig struct {
	Stage Stage `validate:"required"`
}

// MustValidate validates the load balancer config.
func (c *LoadBalancerConfig) MustValidate(_ StageTarget) {
	vz.MustValidateStruct(c)
}

// LoadBalancerDependencies describes the load balancer dependencies.
type LoadBalancerDependencies struct {
	Certificate       Certificate `validate:"required"`
	Network           Network     `validate:"required"`
	OtherDependencies OtherDependencies
}

// MustValidate validates the load balancer dependencies.
func (d *LoadBalancerDependencies) MustValidate() {
	vz.MustValidateStruct(d)
}

// LoadBalancerCloudMetadata describes the load balancer cloud metadata.
type LoadBalancerCloudMetadata struct {
	Exports CloudExports
}

// LoadBalancer describes a load balancer.
type LoadBalancer interface {
	Plugin
	GetConfig() *LoadBalancerConfig
	GetDependencies() *LoadBalancerDependencies
	GetCloudMetadata() *LoadBalancerCloudMetadata
}

type loadBalancerImpl struct {
	cfgFunc       LoadBalancerConfigFunc
	deps          *LoadBalancerDependencies
	cfg           *LoadBalancerConfig
	cloudMetadata *LoadBalancerCloudMetadata
}

// NewLoadBalancer initializes a new LoadBalancer.
func NewLoadBalancer(cfgFunc LoadBalancerConfigFunc, deps *LoadBalancerDependencies) LoadBalancer {
	deps.MustValidate()

	return &loadBalancerImpl{
		cfgFunc: cfgFunc,
		deps:    deps,
	}
}

// GetDisplayName implements the Plugin interface.
func (*loadBalancerImpl) GetDisplayName() string {
	return LoadBalancerPluginDisplayName
}

// GetName implements the Plugin interface.
func (p *loadBalancerImpl) GetName() string {
	return LoadBalancerPluginName
}

// GetInstanceName implements the Plugin interface.
func (p *loadBalancerImpl) GetInstanceName() *string {
	return nil
}

// GetDependenciesMap implements the Plugin interface.
func (p *loadBalancerImpl) GetDependenciesMap() map[Plugin]struct{} {
	dependenciesMap := map[Plugin]struct{}{
		p.deps.Certificate: {},
		p.deps.Network:     {},
	}

	for _, otherDependency := range p.deps.OtherDependencies {
		dependenciesMap[otherDependency] = struct{}{}
	}

	return dependenciesMap
}

// Configure implements the Plugin interface.
func (p *loadBalancerImpl) Configure(stage Stage) {
	p.cfg = p.cfgFunc(stage, p.deps)
	p.cfg.MustValidate(stage.GetTarget())
}

// GetStage implements the Plugin interface.
func (p *loadBalancerImpl) GetStage() Stage {
	errorz.Assertf(p.cfg != nil, "plugin not configured", errorz.Prefix(LoadBalancerPluginName))
	return p.cfg.Stage
}

// GetConfig implements the LoadBalancer interface.
func (p *loadBalancerImpl) GetConfig() *LoadBalancerConfig {
	return p.cfg
}

// GetDependencies implements the LoadBalancer interface.
func (p *loadBalancerImpl) GetDependencies() *LoadBalancerDependencies {
	return p.deps
}

// GetCloudMetadata implements the LoadBalancer interface.
func (p *loadBalancerImpl) GetCloudMetadata() *LoadBalancerCloudMetadata {
	errorz.Assertf(p.cloudMetadata != nil, "cloud not deployed", errorz.Prefix(LoadBalancerPluginName))
	return p.cloudMetadata
}

// IsDeployed implements the Plugin interface.
func (p *loadBalancerImpl) IsDeployed() bool {
	return p.cloudMetadata != nil
}

// UpdateLocalTemplate implements the Plugin interface.
func (p *loadBalancerImpl) UpdateLocalTemplate(_ *dctypes.Config, _ string) {
	// nothing to do here
}

// GetCloudTemplate implements the Plugin interface.
func (p *loadBalancerImpl) GetCloudTemplate(_ string) *gocf.Template {
	tpl := gocf.NewTemplate()

	tpl.Resources[LoadBalancerRefLoadBalancer.Ref()] = &goelbv2.LoadBalancer{
		IpAddressType: stringz.Ptr("ipv4"),
		LoadBalancerAttributes: &[]goelbv2.LoadBalancer_LoadBalancerAttribute{
			{
				Key: stringz.Ptr("deletion_protection.enabled"),
				Value: func() *string {
					if p.cfg.Stage.GetMode().IsProduction() {
						return stringz.Ptr("true")
					}
					return stringz.Ptr("false")
				}(),
			},
		},
		Name:   stringz.Ptr(LoadBalancerRefLoadBalancer.Name(p)),
		Scheme: stringz.Ptr("internet-facing"),
		SecurityGroups: &[]string{
			p.deps.Network.GetCloudMetadata().Exports.GetRef(NetworkRefSecurityGroup),
		},
		Subnets: &[]string{
			p.deps.Network.GetCloudMetadata().Exports.GetRef(NetworkRefSubnetPublicA),
			p.deps.Network.GetCloudMetadata().Exports.GetRef(NetworkRefSubnetPublicB),
		},
		Type: stringz.Ptr("application"),
		Tags: CloudGetDefaultTags(LoadBalancerRefLoadBalancer.Name(p)),
	}
	CloudAddExpRef(tpl, p, LoadBalancerRefLoadBalancer)
	CloudAddExpGetAtt(tpl, p, LoadBalancerRefLoadBalancer, LoadBalancerAttCanonicalHostedZoneID)
	CloudAddExpGetAtt(tpl, p, LoadBalancerRefLoadBalancer, LoadBalancerAttDNSName)
	CloudAddExpGetAtt(tpl, p, LoadBalancerRefLoadBalancer, LoadBalancerAttLoadBalancerFullName)
	CloudAddExpGetAtt(tpl, p, LoadBalancerRefLoadBalancer, LoadBalancerAttLoadBalancerName)
	CloudAddExpGetAtt(tpl, p, LoadBalancerRefLoadBalancer, LoadBalancerAttSecurityGroups)

	tpl.Resources[LoadBalancerRefListenerHTTP.Ref()] = &goelbv2.Listener{
		DefaultActions: []goelbv2.Listener_Action{
			{
				RedirectConfig: &goelbv2.Listener_RedirectConfig{
					Host:       stringz.Ptr("#{host}"),
					Path:       stringz.Ptr("/#{path}"),
					Port:       stringz.Ptr("443"),
					Protocol:   stringz.Ptr("HTTPS"),
					Query:      stringz.Ptr("#{query}"),
					StatusCode: "HTTP_301",
				},
				Type: "redirect",
			},
		},
		LoadBalancerArn: gocf.Ref(LoadBalancerRefLoadBalancer.Ref()),
		Port:            intz.Ptr(80),
		Protocol:        stringz.Ptr("HTTP"),
	}
	CloudAddExpRef(tpl, p, LoadBalancerRefListenerHTTP)
	CloudAddExpGetAtt(tpl, p, LoadBalancerRefListenerHTTP, LoadBalancerAttListenerArn)

	tpl.Resources[LoadBalancerRefListenerHTTPS.Ref()] = &goelbv2.Listener{
		Certificates: &[]goelbv2.Listener_Certificate{
			{
				CertificateArn: stringz.Ptr(p.deps.Certificate.GetCloudMetadata().ARN),
			},
		},
		DefaultActions: []goelbv2.Listener_Action{
			{
				FixedResponseConfig: &goelbv2.Listener_FixedResponseConfig{
					ContentType: stringz.Ptr("text/html"),
					MessageBody: stringz.Ptr(assets.LoadBalancerNotFoundHTMLAsset),
					StatusCode:  "404",
				},
				Type: "fixed-response",
			},
		},
		LoadBalancerArn: gocf.Ref(LoadBalancerRefLoadBalancer.Ref()),
		Port:            intz.Ptr(443),
		Protocol:        stringz.Ptr("HTTPS"),
	}
	CloudAddExpRef(tpl, p, LoadBalancerRefListenerHTTPS)
	CloudAddExpGetAtt(tpl, p, LoadBalancerRefListenerHTTPS, LoadBalancerAttListenerArn)

	tpl.Resources[LoadBalancerRefSecurityGroupIngressHTTP.Ref()] = &goec2.SecurityGroupIngress{
		GroupId:    stringz.Ptr(p.deps.Network.GetCloudMetadata().Exports.GetRef(NetworkRefSecurityGroup)),
		IpProtocol: "tcp",
		FromPort:   intz.Ptr(80),
		ToPort:     intz.Ptr(80),
		CidrIp:     stringz.Ptr(CIDRAllDestinations),
	}

	tpl.Resources[LoadBalancerRefSecurityGroupIngressHTTPS.Ref()] = &goec2.SecurityGroupIngress{
		GroupId:    stringz.Ptr(p.deps.Network.GetCloudMetadata().Exports.GetRef(NetworkRefSecurityGroup)),
		IpProtocol: "tcp",
		FromPort:   intz.Ptr(443),
		ToPort:     intz.Ptr(443),
		CidrIp:     stringz.Ptr(CIDRAllDestinations),
	}

	return tpl
}

// UpdateCloudMetadata implements the Plugin interface.
func (p *loadBalancerImpl) UpdateCloudMetadata(stack *awscft.Stack) {
	p.cloudMetadata = &LoadBalancerCloudMetadata{
		Exports: NewCloudExports(stack),
	}
}

// BeforeDeployHook implements the Plugin interface.
func (*loadBalancerImpl) BeforeDeployHook(_ string) {
	// nothing to do here
}

// AfterDeployHook implements the Plugin interface.
func (*loadBalancerImpl) AfterDeployHook(_ string) {
	// nothing to do here
}
