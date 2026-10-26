package server

import (
	"testing"
	"time"

	"github.com/bestksl/mindport/internal/store"
)

func TestMergePresence(t *testing.T) {
	now := time.Now()
	in := []store.Presence{
		{Actor: "claude@studio", SessionID: "s1", Dir: "/repo/a", Seen: now, Online: true},
		{Actor: "claude@studio", Seen: now, Online: true}, // 旧钩子残留的扁平记录
		{Actor: "codex@vps", Seen: now, Online: true},     // 远程船：只有扁平记录，必须保留
		{Actor: "codex@studio", SessionID: "s9", Dir: "/repo/b", Seen: now},
	}
	out := mergePresence(in)
	for _, p := range out {
		if p.Actor == "claude@studio" && p.SessionID == "" {
			t.Fatalf("同船已有会话记录时扁平心跳应被折叠：%+v", out)
		}
	}
	var foundRemote bool
	for _, p := range out {
		if p.Actor == "codex@vps" && p.SessionID == "" {
			foundRemote = true
		}
	}
	if !foundRemote {
		t.Fatalf("无会话记录的远程船扁平心跳不应被折叠：%+v", out)
	}
	if len(out) != 3 {
		t.Fatalf("期望保留 3 条，得到 %d：%+v", len(out), out)
	}
}
