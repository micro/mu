package local

import (
	"fmt"
	gmai "go-micro.dev/v6/model"
)

// Android supplies an explicit provider; never load a hosted instance's settings.
func provider(gmai.ToolHandler) (gmai.Model, error) {
	return nil, fmt.Errorf("configure a model in the app")
}
func Configured() bool { return false }
func Describe() string { return "device-configured" }
