package command

import (
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// renderQRANSI 用半块字符（▀▄█ 空格）在终端渲染二维码。
// 固定白底黑码：深色终端里白块明显、浅色终端里黑码清晰，两侧再补
// 2 列白边作安静区，大多数手机相机/扫码器可直接扫。
func renderQRANSI(text string) (string, error) {
	q, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		return "", err
	}
	src := q.Bitmap()
	h := len(src)
	w := 0
	if h > 0 {
		w = len(src[0])
	}

	// 统一补 2 模块白边，不依赖库是否已含 quiet zone。
	const pad = 2
	black := func(x, y int) bool {
		if x < pad || y < pad || x >= w+pad || y >= h+pad {
			return false
		}
		return src[y-pad][x-pad]
	}
	W, H := w+2*pad, h+2*pad

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("\x1b[47m\x1b[30m") // 背景白、字黑
	// 上下白行
	for i := 0; i < pad; i++ {
		b.WriteString(strings.Repeat(" ", W+2*pad))
		b.WriteString("\n")
	}
	for y := 0; y < H; y += 2 {
		b.WriteString("    ") // 左侧安静区
		for x := 0; x < W; x++ {
			top := black(x, y)
			bottom := y+1 < H && black(x, y+1)
			switch {
			case top && bottom:
				b.WriteString("█")
			case top:
				b.WriteString("▀")
			case bottom:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		b.WriteString("    \n") // 右侧安静区
	}
	for i := 0; i < pad; i++ {
		b.WriteString(strings.Repeat(" ", W+2*pad))
		b.WriteString("\n")
	}
	b.WriteString("\x1b[0m")
	return b.String(), nil
}
