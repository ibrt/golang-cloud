package cloudz

import (
	"os"
	"path/filepath"
	"regexp"

	"github.com/go-playground/validator/v10"
	"github.com/ibrt/golang-bites/filez"
	"github.com/ibrt/golang-validation/vz"
)

func init() {
	vz.MustRegisterValidator("resource-name", vz.RegexpValidatorFactory(regexp.MustCompile(`^[a-z][a-z0-9-]{0,32}$`)))
	vz.MustRegisterValidator("parent-dir", parentDirValidator)
}

func parentDirValidator(fl validator.FieldLevel) bool {
	fileInfo, err := os.Stat(filepath.Dir(filez.MustAbs(fl.Field().String())))
	return err == nil && fileInfo.IsDir()
}
