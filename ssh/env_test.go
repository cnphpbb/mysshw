package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveEnvRef_NoMatch(t *testing.T) {
	// 无 ${...} 引用时直接返回原字符串
	assert.Equal(t, "plaintext", ResolveEnvRef("plaintext"))
	assert.Equal(t, "", ResolveEnvRef(""))
	assert.Equal(t, "hello world", ResolveEnvRef("hello world"))
	assert.Equal(t, "no.dollar.here", ResolveEnvRef("no.dollar.here"))
}

func TestResolveEnvRef_SetVar(t *testing.T) {
	t.Setenv("MYSSHW_TEST_PWD", "secret123")

	assert.Equal(t, "secret123", ResolveEnvRef("${MYSSHW_TEST_PWD}"))
	assert.Equal(t, "pre-secret123-suf", ResolveEnvRef("pre-${MYSSHW_TEST_PWD}-suf"))
}

func TestResolveEnvRef_UnsetVar(t *testing.T) {
	t.Setenv("MYSSHW_TEST_UNSET", "")

	// 未设置的变量 → 空字符串
	assert.Equal(t, "", ResolveEnvRef("${MYSSHW_TEST_UNSET}"))
	// 含前后文本
	assert.Equal(t, "pre--suf", ResolveEnvRef("pre-${MYSSHW_TEST_UNSET}-suf"))
}

func TestResolveEnvRef_WithDefault(t *testing.T) {
	t.Setenv("MYSSHW_TEST_DEFAULT", "")

	// 未设置时使用 default
	assert.Equal(t, "fallback", ResolveEnvRef("${MYSSHW_TEST_DEFAULT:-fallback}"))
	assert.Equal(t, "pre-fallback-suf", ResolveEnvRef("pre-${MYSSHW_TEST_DEFAULT:-fallback}-suf"))
	// default 中允许空格和特殊字符
	assert.Equal(t, "hello world!", ResolveEnvRef("${MYSSHW_TEST_DEFAULT:-hello world!}"))
}

func TestResolveEnvRef_SetVarOverridesDefault(t *testing.T) {
	t.Setenv("MYSSHW_TEST_OVERRIDE", "realvalue")

	// env 存在时, default 被忽略
	assert.Equal(t, "realvalue", ResolveEnvRef("${MYSSHW_TEST_OVERRIDE:-fallback}"))
}

func TestResolveEnvRef_EmptyDefault(t *testing.T) {
	t.Setenv("MYSSHW_TEST_EMPTY", "")

	// ${VAR:-} (显式空默认) → 空字符串
	assert.Equal(t, "", ResolveEnvRef("${MYSSHW_TEST_EMPTY:-}"))
}

func TestResolveEnvRef_MultipleVars(t *testing.T) {
	t.Setenv("MYSSHW_TEST_A", "AAA")
	t.Setenv("MYSSHW_TEST_B", "BBB")

	assert.Equal(t, "AAA-BBB", ResolveEnvRef("${MYSSHW_TEST_A}-${MYSSHW_TEST_B}"))
	assert.Equal(t, "AAAthenBBB", ResolveEnvRef("${MYSSHW_TEST_A}then${MYSSHW_TEST_B}"))
}

func TestResolveEnvRef_InvalidSyntax(t *testing.T) {
	// 非法格式应原样保留 (不解析)
	assert.Equal(t, "${ 1INVALID }", ResolveEnvRef("${ 1INVALID }"))
	assert.Equal(t, "${}", ResolveEnvRef("${}"))
}
