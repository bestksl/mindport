package server

import (
	"strings"
	"testing"
)

func TestChatHTMLMarkdown(t *testing.T) {
	// 代码块、列表、inline code、表格（GFM）都应成形
	src := "先看结果：\n\n```go\nfmt.Println(\"hi\")\n```\n\n- 一项\n- 二项\n\n`x` 与 **粗**"
	h := chatHTML(src)
	for _, want := range []string{"<pre><code", "<li>一项</li>", "<code>x</code>", "<strong>粗</strong>"} {
		if !strings.Contains(h, want) {
			t.Fatalf("缺少 %s：\n%s", want, h)
		}
	}
}

func TestChatHTMLStripsDangerous(t *testing.T) {
	h := chatHTML("正常 <script>alert(1)</script> 文本\n\n[坏](javascript:alert(1)) [好](https://example.com)")
	low := strings.ToLower(h)
	if strings.Contains(low, "<script>") {
		t.Fatalf("裸脚本标签不应原样输出：\n%s", h)
	}
	if strings.Contains(low, "javascript:alert") {
		t.Fatalf("javascript: 协议应被过滤：\n%s", h)
	}
	if !strings.Contains(h, "https://example.com") {
		t.Fatalf("安全链接应保留：\n%s", h)
	}
}
