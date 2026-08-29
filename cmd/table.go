package cmd

import (
	"fmt"
	"io"
	"strings"
)

// PrintTable 打印对齐的文本表格 (CLI plain 输出统一使用)
//
// 设计目标:
//   - 一处实现, 多处复用 (替换 list / config / ping 中的重复实现)
//   - 零外部依赖, 不引入 text/tabwriter 等 stdlib 之外的库
//   - 列宽自动按 "表头" 和 "数据" 的最大值计算
//   - 支持左对齐 (字符串) 和右对齐 (最后一列通常是数字 / 状态, 不做对齐以匹配历史输出)
//
// 参数:
//   - w:        输出 writer; nil 时 no-op (避免 MCP stdio 模式污染 JSON-RPC)
//   - headers:  表头列名 (长度 == 列数)
//   - rows:     数据行, 每行是与 headers 等长的字符串切片
//
// 输出格式:
//   - 表头行 (按 widths 左对齐, 最后一列左对齐)
//   - 分隔线 "-" (总宽度 = widths + 2 * (列数-1) [列间空格])
//   - 数据行
//
// 如果 headers 为空或 rows 全空, 不打印任何内容。
func PrintTable(w io.Writer, headers []string, rows [][]string) error {
	if w == nil {
		return nil
	}
	if len(headers) == 0 {
		return nil
	}
	if len(rows) == 0 {
		return nil
	}

	// 计算每列宽度: 表头与所有行的最大值
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// 格式化串: 前 N-1 列左对齐, 最后一列左对齐 (避免数字列右对齐打破历史布局)
	// 列间隔固定 2 空格 (与历史输出保持一致)
	parts := make([]string, len(widths))
	for i, w := range widths {
		parts[i] = fmt.Sprintf("%%-%ds", w)
	}
	rowFmt := strings.Join(parts, "  ") + "\n"

	// 分隔线总宽度 = sum(widths) + 2 * (列数 - 1)
	totalWidth := 0
	for _, w := range widths {
		totalWidth += w
	}
	totalWidth += 2 * (len(widths) - 1)
	if totalWidth < 0 {
		totalWidth = 0
	}

	// 表头
	headerRow := make([]any, len(headers))
	for i, h := range headers {
		headerRow[i] = h
	}
	fmt.Fprintf(w, rowFmt, headerRow...)

	// 分隔线
	fmt.Fprintln(w, strings.Repeat("-", totalWidth))

	// 数据行
	for _, row := range rows {
		fmt.Fprintf(w, rowFmt, anySlice(row)...)
	}
	return nil
}

// anySlice 把 []string 转为 []any 以便 fmt.Fprintf 接受可变参数
func anySlice(row []string) []any {
	out := make([]any, len(row))
	for i, s := range row {
		out[i] = s
	}
	return out
}
