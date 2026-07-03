package cmd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewExitError_FormatsMessage(t *testing.T) {
	ee := NewExitError(ExitAuthFailed, "auth failed for %s", "alice")
	assert.Equal(t, ExitAuthFailed, ee.Code)
	assert.NotNil(t, ee.Err)
	assert.Contains(t, ee.Error(), "auth failed for alice")
	assert.Contains(t, ee.Error(), "exit 2")
}

func TestWrapExit(t *testing.T) {
	base := errors.New("underlying")
	ee := WrapExit(ExitTimeout, base)
	assert.Equal(t, ExitTimeout, ee.Code)
	assert.True(t, errors.Is(ee, base))
}

func TestExitCode_PlainError(t *testing.T) {
	assert.Equal(t, ExitOK, ExitCode(nil))
	assert.Equal(t, ExitGeneric, ExitCode(errors.New("boom")))
}

func TestExitCode_ExitError(t *testing.T) {
	ee := NewExitError(ExitConfig, "x")
	assert.Equal(t, ExitConfig, ExitCode(ee))
}

func TestExitError_Unwrap(t *testing.T) {
	base := errors.New("inner")
	ee := WrapExit(ExitUsage, base)
	assert.Equal(t, base, ee.Unwrap())
}

func TestSysexitsConstants(t *testing.T) {
	// 关键 BSD sysexits 值不能改 (AI 文档/脚本会硬编码)
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"ExitOK", ExitOK, 0},
		{"ExitGeneric", ExitGeneric, 1},
		{"ExitAuthFailed", ExitAuthFailed, 2},
		{"ExitUsage", ExitUsage, 64},
		{"ExitDataErr", ExitDataErr, 65},
		{"ExitConfig", ExitConfig, 78},
		{"ExitTimeout", ExitTimeout, 124},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, tc.got, tc.name)
	}
}
