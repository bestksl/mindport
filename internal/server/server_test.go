package server

import (
	"testing"
	"time"

	"github.com/bestksl/mindport/internal/store"
)

func TestParseAgents(t *testing.T) {
	raw := []byte(`[{
	  "pid": 27447, "id": "4e90d39e",
	  "cwd": "/repo/a", "kind": "background", "startedAt": 1792940930297,
	  "sessionId": "sid-a", "name": "思维瞬间任务管理",
	  "status": "busy", "state": "working"
	}, {
	  "pid": 123, "id": "6e3ba1b4",
	  "cwd": "/repo/b", "kind": "interactive", "startedAt": 1792998295199,
	  "sessionId": "sid-b", "name": "三个联动项目",
	  "status": "idle", "state": "done"
	}]`)
	ss, err := parseAgents(raw)
	if err != nil || len(ss) != 2 {
		t.Fatalf("解析失败: %v %+v", err, ss)
	}
	if ss[0].Name != "思维瞬间任务管理" || ss[0].State != "working" || ss[0].SessionID != "sid-a" {
		t.Fatalf("字段错误: %+v", ss[0])
	}
	if _, err := parseAgents([]byte("not json")); err == nil {
		t.Fatal("坏 JSON 应报错")
	}
}

func TestListPresenceMergesOfficialSnapshot(t *testing.T) {
	dir := t.TempDir()
	v, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// 两个会话在线状态 + 一条远程扁平在线状态（不同 actor）
	if _, err := v.PresentSession(store.Presence{Actor: "claude@studio", SessionID: "sid-a", Dir: "/repo/a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.PresentSession(store.Presence{Actor: "claude@studio", SessionID: "sid-old", Dir: "/repo/old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Present("codex@vps", "", ""); err != nil {
		t.Fatal(err)
	}

	// 官方发现口：sid-a 活着且带着简介与运行态；sid-old 已死亡（全集缺席）。
	s := &Server{V: v, broker: newBroker(), poller: &agentPoller{
		actor: "claude@studio",
		v:     v,
		live: map[string]cliSession{
			"sid-a": {SessionID: "sid-a", CWD: "/repo/a", Name: "思维瞬间任务管理", Kind: "background", State: "working"},
		},
	}}
	got := s.listPresence()
	byKey := map[string]store.Presence{}
	for _, p := range got {
		byKey[p.SessionID+p.Actor] = p
	}
	a := byKey["sid-aclaude@studio"]
	if !a.Online || a.RunState != "working" || a.Name != "思维瞬间任务管理" || a.Kind != "background" {
		t.Fatalf("活会话应以官方快照校正: %+v", a)
	}
	old := byKey["sid-oldclaude@studio"]
	if old.Online {
		t.Fatalf("官方快照缺席的会话应判离线: %+v", old)
	}
	remote := byKey["codex@vps"]
	if remote.SessionID != "" || !remote.Online {
		t.Fatalf("远程扁平在线状态应保留且按 TTL 在线: %+v", remote)
	}

	// poller 缺席（机器没装 claude）时退回 TTL 模型：两个会话都在线
	s2 := &Server{V: v, broker: newBroker()}
	for _, p := range s2.listPresence() {
		if p.SessionID != "" && !p.Online {
			t.Fatalf("无发现口时会话应按 TTL 判在线: %+v", p)
		}
	}
}

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
			t.Fatalf("同会话已有记录时扁平在线状态应被折叠：%+v", out)
		}
	}
	var foundRemote bool
	for _, p := range out {
		if p.Actor == "codex@vps" && p.SessionID == "" {
			foundRemote = true
		}
	}
	if !foundRemote {
		t.Fatalf("无会话记录的远程会话扁平在线状态不应被折叠：%+v", out)
	}
	if len(out) != 3 {
		t.Fatalf("期望保留 3 条，得到 %d：%+v", len(out), out)
	}
}
