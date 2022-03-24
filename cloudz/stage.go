package cloudz

import (
	"github.com/ibrt/golang-errors/errorz"
)

// StageTarget describes a Stage target.
type StageTarget string

// MustValidate validates the StageTarget.
func (s StageTarget) MustValidate() {
	errorz.Assertf(s == Local || s == Cloud, "invalid stage target")
}

// IsLocal returns true if the Stage target is Local.
func (s StageTarget) IsLocal() bool {
	return s == Local
}

// IsCloud returns true if the Stage target is Cloud.
func (s StageTarget) IsCloud() bool {
	return s == Cloud
}

// String implements the fmt.Stringer interface.
func (s StageTarget) String() string {
	return string(s)
}

// Known Stage targets.
const (
	Local StageTarget = "local"
	Cloud StageTarget = "cloud"
)

// StageMode describes a Stage mode.
type StageMode string

// MustValidate validates the StageMode.
func (s StageMode) MustValidate() {
	errorz.Assertf(s == Production || s == Staging, "invalid stage mode")
}

// IsProduction returns true if the Stage mode is Production.
func (s StageMode) IsProduction() bool {
	return s == Production
}

// IsStaging returns true if the Stage mode is Staging.
func (s StageMode) IsStaging() bool {
	return s == Staging
}

// String implements the fmt.Stringer interface.
func (s StageMode) String() string {
	return string(s)
}

// Known Stage modes.
const (
	Production StageMode = "prod"
	Staging    StageMode = "staging"
)

// StageConfig describes the common config for a stage.
type StageConfig struct {
	App          App `validate:"required"`
	CustomConfig interface{}
}

// Stage describes a Stage.
type Stage interface {
	GetName() string
	GetTarget() StageTarget
	GetMode() StageMode
	GetConfig() *StageConfig
	AsCloudStage() CloudStage
	AsLocalStage() LocalStage
	Deploy()
}
