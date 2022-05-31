package cloudz

import (
	"fmt"
	"strings"

	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	gocf "github.com/awslabs/goformation/v6/cloudformation"
	goecs "github.com/awslabs/goformation/v6/cloudformation/ecs"
	gotags "github.com/awslabs/goformation/v6/cloudformation/tags"
	"github.com/iancoleman/strcase"
	"github.com/ibrt/golang-bites/stringz"
	"github.com/ibrt/golang-errors/errorz"
)

// CloudAtt describes a cloud attribute.
type CloudAtt string

// Name returns a name.
func (a CloudAtt) Name() string {
	return strcase.ToKebab(strings.ReplaceAll(string(a), ".", "-"))
}

// Ref returns a reference.
func (a CloudAtt) Ref() string {
	return string(a)
}

// CloudRef describes a cloud reference.
type CloudRef string

// Name returns a name.
func (r CloudRef) Name(p Plugin) string {
	return fmt.Sprintf("%v-%v", CloudGetStackName(p), r)
}

// Ref returns a reference.
func (r CloudRef) Ref() string {
	return strcase.ToCamel(string(r))
}

// ExpRefRef returns a reference to a reference export.
func (r CloudRef) ExpRefRef() string {
	return (r + "-exp-ref").Ref()
}

// ExpRefName returns a name for a reference export.
func (r CloudRef) ExpRefName(p Plugin) string {
	return (r + "-exp-ref").Name(p)
}

// ExpAttRef returns a reference to an attribute export.
func (r CloudRef) ExpAttRef(att CloudAtt) string {
	return (r + "-exp").Ref() + strings.ReplaceAll(att.Ref(), ".", "")
}

// ExpAttName returns a name for an attribute export.
func (r CloudRef) ExpAttName(p Plugin, att CloudAtt) string {
	return (r + "-exp").Name(p) + "-" + att.Name()
}

// CloudExports describes a set of cloud exports.
type CloudExports interface {
	GetRef(ref CloudRef) string
	GetAtt(ref CloudRef, att CloudAtt) string
}

type cloudExports struct {
	stack *awscft.Stack
}

// NewCloudExports initializes a new set of cloud exports.
func NewCloudExports(stack *awscft.Stack) CloudExports {
	return &cloudExports{
		stack: stack,
	}
}

// GetRef gets the value of a reference export.
func (e *cloudExports) GetRef(ref CloudRef) string {
	expRefRef := ref.ExpRefRef()

	for _, output := range e.stack.Outputs {
		if *output.OutputKey == expRefRef {
			return *output.OutputValue
		}
	}

	panic(errorz.Errorf("no such export: ref for %v", errorz.A(ref.Ref())))
}

// GetAtt gets the value of an attribute export.
func (e *cloudExports) GetAtt(ref CloudRef, att CloudAtt) string {
	expAttRef := ref.ExpAttRef(att)

	for _, output := range e.stack.Outputs {
		if *output.OutputKey == expAttRef {
			return *output.OutputValue
		}
	}

	panic(errorz.Errorf("no such export: att %v for ref %v", errorz.A(att, ref)))
}

// NewAssumeRolePolicyDocument generates a new assume role policy document.
func NewAssumeRolePolicyDocument(service string) interface{} {
	return map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Principal": map[string]interface{}{
					"Service": service,
				},
				"Action": "sts:AssumeRole",
			},
		},
	}
}

// NewPolicyDocument generates a new policy document.
func NewPolicyDocument(statements ...*PolicyStatement) interface{} {
	materializedStatements := make([]interface{}, 0)
	for _, statement := range statements {
		materializedStatements = append(materializedStatements, statement.Build())
	}

	return map[string]interface{}{
		"Version":   "2012-10-17",
		"Statement": materializedStatements,
	}
}

// PolicyStatement describes a policy statement.
type PolicyStatement struct {
	Actions   []string
	Resources []string
	Principal interface{}
}

// NewPolicyStatement initializes a new PolicyStatement.
func NewPolicyStatement() *PolicyStatement {
	return &PolicyStatement{
		Actions:   make([]string, 0),
		Resources: make([]string, 0),
	}
}

// AddActions adds actions to the policy statement.
func (s *PolicyStatement) AddActions(actions ...string) *PolicyStatement {
	s.Actions = append(s.Actions, actions...)
	return s
}

// AddResources adds resources to the policy statement.
func (s *PolicyStatement) AddResources(resources ...string) *PolicyStatement {
	s.Resources = append(s.Resources, resources...)
	return s
}

// SetCurrentRootAccountPrincipal sets the current root account as principal on the policy statement.
func (s *PolicyStatement) SetCurrentRootAccountPrincipal() *PolicyStatement {
	s.Principal = map[string]interface{}{
		"AWS": gocf.Sub("arn:aws:iam::${AWS::AccountId}:root"),
	}
	return s
}

// SetWildcardPrincipal sets a wildcard as principal on the policy statement.
func (s *PolicyStatement) SetWildcardPrincipal() *PolicyStatement {
	s.Principal = "*"
	return s
}

// SetServicePrincipal sets a service as principal on the policy statement.
func (s *PolicyStatement) SetServicePrincipal(service string) *PolicyStatement {
	s.Principal = map[string]interface{}{
		"Service": service,
	}
	return s
}

// SetAnyRootAccountPrincipal sets any root account as principal on the policy statement.
func (s *PolicyStatement) SetAnyRootAccountPrincipal() *PolicyStatement {
	s.Principal = map[string]interface{}{
		"AWS": "*",
	}
	return s
}

// Build builds the policy statement.
func (s *PolicyStatement) Build() interface{} {
	errorz.Assertf(len(s.Actions) > 0, "actions unexpectedly empty")
	errorz.Assertf(len(s.Resources) > 0, "resources unexpectedly empty")

	m := map[string]interface{}{
		"Effect":   "Allow",
		"Action":   s.Actions,
		"Resource": s.Resources,
	}

	if s.Principal != nil {
		m["Principal"] = s.Principal
	}

	return m
}

// CloudGetDefaultTags returns a set of default tags.
func CloudGetDefaultTags(name string) *[]gotags.Tag {
	return &[]gotags.Tag{
		{
			Key:   "Name",
			Value: name,
		},
	}
}

// CloudGetTaskDefinitionKeyValuePairs converts a map of strings to a slice of TaskDefinition_KeyValuePair.
func CloudGetTaskDefinitionKeyValuePairs(m map[string]string) *[]goecs.TaskDefinition_KeyValuePair {
	kvs := make([]goecs.TaskDefinition_KeyValuePair, 0, len(m))
	for k, v := range m {
		kvs = append(kvs, goecs.TaskDefinition_KeyValuePair{
			Name:  stringz.Ptr(k),
			Value: stringz.Ptr(v),
		})
	}
	return &kvs
}

// CloudAddExpRef adds a reference export to the given template.
func CloudAddExpRef(tpl *gocf.Template, p Plugin, ref CloudRef) {
	tpl.Outputs[ref.ExpRefRef()] = gocf.Output{
		Value: gocf.Ref(ref.Ref()),
		Export: &gocf.Export{
			Name: ref.ExpRefName(p),
		},
	}
}

// CloudAddExpGetAtt adds a get attribute export to the given template.
func CloudAddExpGetAtt(tpl *gocf.Template, p Plugin, ref CloudRef, att CloudAtt) {
	tpl.Outputs[ref.ExpAttRef(att)] = gocf.Output{
		Value: gocf.GetAtt(ref.Ref(), att.Ref()),
		Export: &gocf.Export{
			Name: ref.ExpAttName(p, att),
		},
	}
}

// CloudGetStackName generates a stack name for the given plugin.
func CloudGetStackName(p Plugin) string {
	parts := []string{
		p.GetStage().GetConfig().App.GetConfig().Name,
		p.GetStage().GetName(),
		p.GetName(),
	}

	if instanceName := p.GetInstanceName(); instanceName != nil {
		parts = append(parts, *instanceName)
	}

	return strings.Join(parts, "-")
}
