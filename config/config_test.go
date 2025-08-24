package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCfgPath(t *testing.T) {
	var cfgPath string
	var err error
	cfgPath = "~/.mysshw.toml"
	//cfgPath = "$HOME/.mysshw.toml"
	_cfgPath, err := GetCfgPath(cfgPath)
	t.Log(_cfgPath)
	assert.Equal(t, cfgPath, _cfgPath)
	assert.EqualValues(t, CFG_PATH, _cfgPath)
	if err != nil {
		assert.EqualError(t, err, "GetCfgPath:Error")
	}
}
