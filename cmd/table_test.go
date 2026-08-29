package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrintTable_Basic(t *testing.T) {
	var buf bytes.Buffer
	err := PrintTable(&buf,
		[]string{"A", "B"},
		[][]string{
			{"1", "2"},
			{"abc", "defg"},
		},
	)
	assert.NoError(t, err)
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// 期望: 表头 / 分隔线 / 2 行数据
	assert.Len(t, lines, 4)
	// 列宽: A 列 max("A","1","abc")=3, B 列 max("B","2","defg")=4
	//   表头行 = "A  " + "  " + "B   " = "A    B   "
	assert.Equal(t, "A    B   ", lines[0])
	// 分隔线: 总宽 = 3+4 + 2*(2-1) = 9
	assert.Equal(t, strings.Repeat("-", 9), lines[1])
	// 数据行 1: "1  " + "  " + "2   " = "1    2   "
	assert.Equal(t, "1    2   ", lines[2])
	// 数据行 2: "abc" + "  " + "defg" = "abc  defg"
	assert.Equal(t, "abc  defg", lines[3])
}

func TestPrintTable_NilWriter(t *testing.T) {
	// nil writer 不应 panic (MCP stdio 模式保护)
	assert.NotPanics(t, func() {
		_ = PrintTable(nil, []string{"A"}, [][]string{{"x"}})
	})
}

func TestPrintTable_EmptyRows(t *testing.T) {
	var buf bytes.Buffer
	err := PrintTable(&buf, []string{"A", "B"}, nil)
	assert.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestPrintTable_EmptyHeaders(t *testing.T) {
	var buf bytes.Buffer
	err := PrintTable(&buf, nil, [][]string{{"x"}})
	assert.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestPrintTable_ColumnWidthFromHeader(t *testing.T) {
	// 列宽应取表头与所有行的最大值
	var buf bytes.Buffer
	err := PrintTable(&buf,
		[]string{"LONGER-HEADER"},
		[][]string{{"x"}},
	)
	assert.NoError(t, err)
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// "LONGER-HEADER" 长度 = 13, 单列, 分隔线宽度 = 13
	assert.Equal(t, strings.Repeat("-", 13), lines[1])
}
