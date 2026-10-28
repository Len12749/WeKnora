package dingtalk

import "strings"

// hardenLineBreaks 把 markdown 的软换行升成钉钉能识别的硬换行。
//
// 钉钉按 CommonMark 渲染：单个换行是软换行，客户端折叠成一个空格；
// 行尾两个空格 + 换行才是硬换行（CommonMark §6.7）。飞书卡片和网页端
// 把软换行当换行显示，所以同一份内容在那边正常、在钉钉会挤成一段。
//
// 代码块内部的行原样保留：那里的换行本就按字面渲染，补空格会污染可复制的代码。
// 已经是硬换行的行不动，所以本函数可以重复调用。
func hardenLineBreaks(content string) string {
	if content == "" {
		return content
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// 围栏行本身是块级标记，不需要硬换行；进/出代码块靠它翻转状态。
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || trimmed == "" {
			continue
		}
		if !strings.HasSuffix(line, "  ") {
			lines[i] = line + "  "
		}
	}
	return strings.Join(lines, "\n")
}
