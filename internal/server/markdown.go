package server

// 对话气泡的 Markdown 渲染：assistant 回复走 GFM（代码块/列表/表格/删除线/autolink），
// 输出安全 HTML 供前端富文本展示。goldmark 默认即安全——不开 html.WithUnsafe，
// 原文里的裸 HTML/脚本会被转义成文本，链接也只放行安全协议（javascript: 被滤）。

import (
	"strings"

	"github.com/bestksl/mindport/internal/transcript"
	"github.com/yuin/goldmark"
	gmextension "github.com/yuin/goldmark/extension"
)

var chatMD = goldmark.New(goldmark.WithExtensions(gmextension.GFM))

// chatHTML 把一段 Markdown 转成受信 HTML（GFM 渲染、脚本无害化）。
func chatHTML(src string) string {
	var buf strings.Builder
	if err := chatMD.Convert([]byte(src), &buf); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

// enrichMarkdown 给一页事件里所有 assistant 文本块填充 HTML 字段。
// 只渲染助手的话：用户原话保留纯文本气泡，不把用户输入当受信 HTML。
func enrichMarkdown(evs []transcript.Event) {
	for i := range evs {
		if evs[i].Role != "assistant" {
			continue
		}
		for j := range evs[i].Blocks {
			b := &evs[i].Blocks[j]
			if b.Kind == "text" && strings.TrimSpace(b.Text) != "" {
				b.HTML = chatHTML(b.Text)
			}
		}
	}
}
