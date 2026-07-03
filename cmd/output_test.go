package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteOut_Plain_String(t *testing.T) {
	var buf bytes.Buffer
	err := WriteOut(&buf, "hello", OutputPlain)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", buf.String())
}

func TestWriteOut_Plain_Error(t *testing.T) {
	var buf bytes.Buffer
	err := WriteOut(&buf, errString("boom"), OutputPlain)
	require.NoError(t, err)
	assert.Equal(t, "boom\n", buf.String())
}

func TestWriteOut_JSON_String(t *testing.T) {
	// string 类型在 JSON 模式也 OK, 但会被编码为 JSON 字符串
	var buf bytes.Buffer
	err := WriteOut(&buf, "hi", OutputJSON)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"hi"`)
}

func TestWriteOut_JSON_Struct(t *testing.T) {
	type obj struct {
		Node string `json:"node"`
	}
	v := obj{Node: "prod-1"}
	var buf bytes.Buffer
	err := WriteOut(&buf, v, OutputJSON)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"node": "prod-1"`)
	assert.Contains(t, buf.String(), "\n", "JSON 应带换行")
}

func TestWriteOut_JSON_Slice(t *testing.T) {
	var buf bytes.Buffer
	err := WriteOut(&buf, []string{"a", "b"}, OutputJSON)
	require.NoError(t, err)
	// 验证是合法 JSON
	var got []string
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.Equal(t, []string{"a", "b"}, got)
}

func TestWriteOut_NilWriterDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		_ = WriteOut(nil, "x", OutputPlain)
	})
}

func TestWriteOut_UnknownFormat_DefaultsToPlain(t *testing.T) {
	var buf bytes.Buffer
	err := WriteOut(&buf, "hello", OutputFormat("xml"))
	require.NoError(t, err)
	assert.Equal(t, "hello\n", buf.String())
}

// errString implements error interface for test
type errString string

func (e errString) Error() string { return string(e) }

func TestGetOutputFormat_NoCmd_UsesPlain(t *testing.T) {
	got := GetOutputFormat(nil)
	assert.Equal(t, OutputPlain, got)
}

func TestOutputFormat_Constants(t *testing.T) {
	assert.Equal(t, "plain", string(OutputPlain))
	assert.Equal(t, "json", string(OutputJSON))
	assert.True(t, strings.Contains("json/plain", string(OutputPlain)))
}
