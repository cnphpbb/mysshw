package ssh

import (
	"errors"
	"fmt"
	"strings"

	"mysshw/config"
)

// ErrNodeNotFound 节点未找到（导出供调用方 errors.Is 判定）
var ErrNodeNotFound = errors.New("node not found")

// NodeSelector 多维度节点定位参数
//
// 行为:
//   - len(Names) > 0 → 按 name 批量匹配，按 Names 顺序保留结果，去重
//   - Alias != ""  → 精确匹配 Alias
//   - Host != ""   → 匹配 Host；可与 User/Port 组合缩小范围（不影响"必须相等"语义）
//   - 三者皆空     → 视为"全部节点"（mass fan-out，默认行为）
type NodeSelector struct {
	Names []string
	Alias string
	Host  string
	User  string
	Port  int
}

// IsEmpty 判断 selector 是否完全未指定（用于 "无参数 → 全部" 兜底）
func (s NodeSelector) IsEmpty() bool {
	return len(s.Names) == 0 && s.Alias == "" && s.Host == "" && s.User == "" && s.Port == 0
}

// ResolveNodes 根据 sel 在 cfg 中查找一个或多个节点
//
// 优先级:
//  1. Names        (批量)
//  2. Alias        (单点，精确)
//  3. Host [+User][+Port] (单点/多点)
//
// 返回:
//   - 找到的节点（绝不返回 nil slice）
//   - 错误: 找不到时 fmt.Errorf("%w: ...", ErrNodeNotFound, ...)
//
// 警告/容错:
//   - Names 批量模式下，部分找不到的 name 不会让整体失败，会通过 error 返回"未找到的列表"
//     让调用方决定是否继续
func ResolveNodes(cfg *config.Configs, sel NodeSelector) ([]*config.SSHNode, error) {
	if cfg == nil {
		return nil, fmt.Errorf("mysshw:: nil config")
	}

	pool := flattenNodes(cfg)
	if len(pool) == 0 {
		return nil, fmt.Errorf("%w: no nodes defined in config", ErrNodeNotFound)
	}

	// 1) names 批量
	if len(sel.Names) > 0 {
		var matched []*config.SSHNode
		var missing []string
		seen := make(map[string]bool)
		for _, name := range sel.Names {
			if seen[name] {
				continue
			}
			seen[name] = true
			n := findByName(pool, name)
			if n != nil {
				matched = append(matched, n)
			} else {
				missing = append(missing, name)
			}
		}
		switch {
		case len(matched) == 0:
			return nil, fmt.Errorf("%w: no matches for --node %v (searched %d names across %d total nodes)",
				ErrNodeNotFound, sel.Names, len(sel.Names), len(pool))
		case len(missing) > 0:
			return matched, fmt.Errorf("mysshw:: some --node names not found: %v (continuing with %d matched)",
				missing, len(matched))
		default:
			return matched, nil
		}
	}

	// 2) alias 单点
	if sel.Alias != "" {
		var hits []*config.SSHNode
		for _, n := range pool {
			if n.Alias == sel.Alias {
				hits = append(hits, n)
			}
		}
		if len(hits) == 0 {
			return nil, fmt.Errorf("%w: --alias %q not found (searched %d nodes)", ErrNodeNotFound, sel.Alias, len(pool))
		}
		return hits, nil
	}

	// 3) host [+ user + port] 单点/多点
	if sel.Host != "" {
		filtered := filterByHost(pool, sel.Host)
		// user / port 是缩小过滤器, 不强制必须存在
		// (例如 --host X -u app 找到 user=app 的所有, 若没有则退回到 user=空的所有)
		filtered = narrowByUser(filtered, sel.User)
		filtered = narrowByPort(filtered, sel.Port)
		if len(filtered) == 0 {
			return nil, fmt.Errorf("%w: --host %q (user=%q port=%d) matched no nodes",
				ErrNodeNotFound, sel.Host, sel.User, sel.Port)
		}
		return filtered, nil
	}

	// 4) 兜底：全部节点（mass fan-out）
	return pool, nil
}

// SummaryLine 生成单节点摘要，便于错误信息中显示
func SummaryLine(n *config.SSHNode) string {
	if n == nil {
		return "<nil>"
	}
	parts := []string{"name=" + n.Name}
	if n.Alias != "" {
		parts = append(parts, "alias="+n.Alias)
	}
	u := n.User
	if u == "" {
		u = "root"
	}
	p := n.Port
	if p == 0 {
		p = 22
	}
	parts = append(parts, fmt.Sprintf("%s@%s:%d", u, n.Host, p))
	return strings.Join(parts, " ")
}

// flattenNodes 把 cfg.Nodes ([][]*SSHNode) 扁平为 []*SSHNode
func flattenNodes(cfg *config.Configs) []*config.SSHNode {
	var out []*config.SSHNode
	for _, group := range cfg.Nodes {
		out = append(out, group.SSHNodes...)
	}
	return out
}

func findByName(pool []*config.SSHNode, name string) *config.SSHNode {
	for _, n := range pool {
		if n.Name == name {
			return n
		}
	}
	return nil
}

func filterByHost(pool []*config.SSHNode, host string) []*config.SSHNode {
	var out []*config.SSHNode
	for _, n := range pool {
		if n.Host == host {
			out = append(out, n)
		}
	}
	return out
}

// narrowByUser 保留 user 匹配的节点; 若没有任何匹配，回退到保留 user 空的节点
// 这样 --host X -u app 在 app 缺失时不会"无结果", 至少能退回到默认 root
func narrowByUser(pool []*config.SSHNode, user string) []*config.SSHNode {
	if user == "" {
		return pool
	}
	var direct, fallback []*config.SSHNode
	for _, n := range pool {
		if n.User == user {
			direct = append(direct, n)
		} else if n.User == "" {
			fallback = append(fallback, n)
		}
	}
	if len(direct) > 0 {
		return direct
	}
	return fallback
}

// narrowByPort 逻辑同上
func narrowByPort(pool []*config.SSHNode, port int) []*config.SSHNode {
	if port <= 0 {
		return pool
	}
	var direct, fallback []*config.SSHNode
	for _, n := range pool {
		if n.Port == port {
			direct = append(direct, n)
		} else if n.Port == 0 {
			fallback = append(fallback, n)
		}
	}
	if len(direct) > 0 {
		return direct
	}
	return fallback
}
