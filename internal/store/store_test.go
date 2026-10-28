package store

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestTaskLifecycle(t *testing.T) {
	v, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// 入港即立箱：直接建草稿任务（v0.4 起不再有独立闪念/inbox）
	t1, err := v.NewTask("洗澡时的闪念", ImpHigh, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if t1.Meta.ID != "T0001" || t1.Meta.Status != StatusDraft || t1.Meta.Importance != ImpHigh {
		t.Fatalf("草稿字段错误: %+v", t1.Meta)
	}
	if !strings.Contains(t1.Body, "洗澡时的闪念") {
		t.Fatalf("标题未进入描述:\n%s", t1.Body)
	}

	// 草稿阶段可打磨验收标准
	bodyWithAccept := strings.Replace(t1.Body, "## 验收标准\n",
		"## 验收标准\n\n- [ ] int8 单帧 ≤ 30ms\n- [ ] 三档结果记入日志\n", 1)
	if _, err := v.UpdateTask("T0001", t1.Meta.Title, bodyWithAccept, ImpHigh, ""); err != nil {
		t.Fatal(err)
	}

	// 直接派船给 claude@studio
	pub, err := v.Publish("T0001", "jmzx@studio", "claude@studio", "")
	if err != nil {
		t.Fatal(err)
	}
	if pub.Meta.Status != StatusReady || pub.Meta.AssigneeValue() != "claude@studio" {
		t.Fatalf("派船后应为 ready 且指派 claude，得到 %s/%q", pub.Meta.Status, pub.Meta.AssigneeValue())
	}

	// 别的船不能接令
	if _, err := v.Claim("T0001", "codex@studio", ""); !errors.Is(err, ErrNotAssignee) {
		t.Fatalf("非承担船接令应 ErrNotAssignee，得到 %v", err)
	}
	if _, err := v.Claim("T0001", "claude@studio", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Log("T0001", "基准 27.4ms", "claude@studio"); err != nil {
		t.Fatal(err)
	}

	// 只勾一项时归港被硬门禁拦下
	if _, _, _, err := v.CheckItem("T0001", 1, true, "claude@studio"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Done("T0001", "claude@studio", "", false, ""); !errors.Is(err, ErrResultRequired) {
		t.Fatalf("验收未全勾应 ErrResultRequired，得到 %v", err)
	}
	if _, _, _, err := v.CheckItem("T0001", 2, true, "claude@studio"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Done("T0001", "claude@studio", "", false, ""); err != nil {
		t.Fatal(err)
	}

	r, err := parseFile(t1.Path)
	if err != nil {
		t.Fatal(err)
	}
	if r.Meta.Status != StatusDone || r.Meta.AssigneeValue() != "claude@studio" {
		t.Fatalf("持久化字段错误: %+v", r.Meta)
	}
	for _, want := range []string{"claimed by claude@studio", "claude@studio：基准 27.4ms", "验收项 1/2 完成"} {
		if !strings.Contains(r.Body, want) {
			t.Fatalf("日志缺少 %q:\n%s", want, r.Body)
		}
	}
	if strings.HasPrefix(r.Body, "---") {
		t.Fatalf("frontmatter 闭合标记残留进正文")
	}
}

func TestAnchorageDispatchReassign(t *testing.T) {
	v, _ := Open(t.TempDir())
	rec, _ := v.NewTask("锚地货", "", "", false)
	id := rec.Meta.ID
	if rec.Meta.Status != StatusAnchored {
		t.Fatalf("非草稿建任务应进锚地，得到 %s", rec.Meta.Status)
	}
	// 锚地货对所有船不可见
	if _, err := v.Next("claude@studio", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("锚地货不应进 next，得到 %v", err)
	}
	if _, err := v.Claim(id, "claude@studio", ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("锚地货不能直接接令，得到 %v", err)
	}
	// 派船必须点名
	if _, err := v.Assign(id, "jmzx@studio", "  ", ""); err == nil {
		t.Fatal("空船名派船应报错")
	}
	if _, err := v.Assign(id, "jmzx@studio", "claude@studio", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Next("codex@studio", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("货没派给 codex，next 应空，得到 %v", err)
	}
	next, err := v.Next("claude@studio", "")
	if err != nil || next.Meta.ID != id {
		t.Fatalf("claude next 应得到 %s，得到 %v err=%v", id, next, err)
	}
	// 待接令阶段改派
	r2, err := v.Assign(id, "jmzx@studio", "codex@studio", "")
	if err != nil {
		t.Fatal(err)
	}
	if r2.Meta.AssigneeValue() != "codex@studio" || !strings.Contains(r2.Body, "reassigned") {
		t.Fatalf("改派字段/日志错误: assignee=%q", r2.Meta.AssigneeValue())
	}
	if _, err := v.Claim(id, "claude@studio", ""); !errors.Is(err, ErrNotAssignee) {
		t.Fatalf("改派后原船不能接令，得到 %v", err)
	}
	if _, err := v.Claim(id, "codex@studio", ""); err != nil {
		t.Fatal(err)
	}
	// 出港中封令：不能改派、不能再编辑
	if _, err := v.Assign(id, "jmzx@studio", "claude@studio", ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("出港中不应允许改派，得到 %v", err)
	}
	if _, err := v.UpdateTask(id, "锚地货", r2.Body, "", ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("派船封舱后不应允许编辑，得到 %v", err)
	}
}

func TestBlockUnblockAndBlockedReassign(t *testing.T) {
	v, _ := Open(t.TempDir())
	_, _ = v.NewTask("x", "", "", true)
	_, _ = v.Publish("T0001", "jmzx@studio", "a@studio", "")
	_, _ = v.Claim("T0001", "a@studio", "")

	// 只有承担船能上报受阻
	if _, err := v.Block("T0001", "等 Vs3 实测", "b@studio"); !errors.Is(err, ErrNotAssignee) {
		t.Fatalf("非承担船受阻上报应 ErrNotAssignee，得到 %v", err)
	}
	if _, err := v.Block("T0001", "", "a@studio"); err == nil {
		t.Fatal("空原因应拒绝")
	}
	r, err := v.Block("T0001", "等 Vs3 实测", "a@studio")
	if err != nil {
		t.Fatal(err)
	}
	if r.Meta.Status != StatusBlocked || r.Meta.BlockedAt == nil {
		t.Fatalf("block 字段错误: %+v", r.Meta)
	}
	// 无验收项归港必须带结果
	if _, err := v.Done("T0001", "a@studio", "", false, ""); !errors.Is(err, ErrResultRequired) {
		t.Fatalf("无结果归港应 ErrResultRequired，得到 %v", err)
	}
	if _, err := v.Done("T0001", "a@studio", "替代方案已验证", false, ""); err != nil {
		t.Fatal("带交付结果应允许归港")
	}

	// 受阻改派：货回码头，新船重新待接令
	_, _ = v.NewTask("y", "", "", true)
	_, _ = v.Publish("T0002", "jmzx@studio", "a@studio", "")
	_, _ = v.Claim("T0002", "a@studio", "")
	rb, _ := v.Block("T0002", "缺数据", "a@studio")
	rr, err := v.Assign("T0002", "jmzx@studio", "b@studio", "")
	if err != nil {
		t.Fatal(err)
	}
	if rr.Meta.Status != StatusReady || rr.Meta.AssigneeValue() != "b@studio" || rr.Meta.BlockedAt != nil {
		t.Fatalf("受阻改派字段错误: %+v", rr.Meta)
	}
	if !strings.Contains(rr.Body, "原船受阻") {
		t.Fatalf("受阻改派日志缺失: %s", rr.Body)
	}
	if _, err := v.Claim("T0002", "a@studio", ""); !errors.Is(err, ErrNotAssignee) {
		t.Fatalf("改派后原船不能接令，得到 %v", err)
	}
	if _, err := v.Claim("T0002", "b@studio", ""); err != nil {
		t.Fatal(err)
	}
	_ = rb
}

func TestImportanceOrderingAndNext(t *testing.T) {
	v, _ := Open(t.TempDir())
	_, _ = v.NewTask("普通", "", "", true)          // T0001 normal draft
	_, _ = v.NewTask("关键", ImpCritical, "", true) // T0002
	_, _ = v.NewTask("也许", ImpSomeday, "", true)  // T0003
	_, _ = v.NewTask("重要", ImpHigh, "", true)     // T0004
	// 只把 T0002 派给 a
	if _, err := v.Publish("T0002", "jmzx@studio", "a@studio", ""); err != nil {
		t.Fatal(err)
	}

	next, err := v.Next("a@studio", "")
	if err != nil {
		t.Fatal(err)
	}
	if next.Meta.ID != "T0002" {
		t.Fatalf("next 应为派给 a 的 T0002，得到 %s", next.Meta.ID)
	}
	if _, err := v.Next("", ""); err == nil {
		t.Fatal("空船名 next 应报错")
	}

	tasks, _ := v.ListTasks("")
	want := []string{"T0002", "T0004", "T0001", "T0003"}
	for i, w := range want {
		if tasks[i].Meta.ID != w {
			t.Fatalf("排序错误: %v", ids(tasks))
		}
	}

	if _, err := v.NewTask("bad", "urgent!!", "", false); !errors.Is(err, ErrInvalidImportance) {
		t.Fatalf("非法 importance 应拒绝，得到 %v", err)
	}
}

func TestMigrationLegacyReadyAndEdit(t *testing.T) {
	dir := t.TempDir()
	v, _ := Open(dir)
	// 旧版「已发布、任领取」（ready 无 assignee）→ 自动退回锚地
	old := "---\ntype: task\nid: T0001\ntitle: 旧文件\nstatus: ready\npriority: high\ncreated: 2026-10-25T20:00:00+08:00\n---\n\n## 描述\n旧正文\n"
	mustWrite(t, dir+"/tasks/T0001-old.md", old)
	r, err := v.GetTask("T0001")
	if err != nil {
		t.Fatal(err)
	}
	if r.Meta.Importance != ImpHigh || r.Meta.Priority != "" {
		t.Fatalf("旧字段迁移失败: %+v", r.Meta)
	}
	if r.Meta.Status != StatusAnchored {
		t.Fatalf("无指派的旧 ready 应退回锚地，得到 %s", r.Meta.Status)
	}
	// 锚地阶段仍可编辑
	upd, err := v.UpdateTask("T0001", "新标题", "## 描述\n新正文\n", ImpCritical, "某个目标")
	if err != nil {
		t.Fatal(err)
	}
	if upd.Meta.Goal != "某个目标" || !strings.Contains(upd.Path, "T0001-新标题.md") {
		t.Fatalf("编辑/改名失败: path=%s goal=%q", upd.Path, upd.Meta.Goal)
	}

	// 旧版带指派的 ready 保持待接令
	old2 := "---\ntype: task\nid: T0002\ntitle: 已指派\nstatus: ready\nassignee: claude@studio\nimportance: normal\ncreated: 2026-10-25T20:00:00+08:00\n---\n\n## 描述\nx\n"
	mustWrite(t, dir+"/tasks/T0002-x.md", old2)
	r2, err := v.GetTask("T0002")
	if err != nil {
		t.Fatal(err)
	}
	if r2.Meta.Status != StatusReady || r2.Meta.AssigneeValue() != "claude@studio" {
		t.Fatalf("带指派的旧 ready 应保持 ready，得到 %s/%q", r2.Meta.Status, r2.Meta.AssigneeValue())
	}
}

func TestForceDoneEscapeHatch(t *testing.T) {
	v, _ := Open(t.TempDir())
	// 草稿期写一个验收项，派船接令后故意不勾，模拟死锁
	rec, _ := v.NewTask("死锁货", "", "", true)
	cur, _ := v.GetTask(rec.Meta.ID)
	body := strings.Replace(cur.Body, "## 验收标准\n",
		"## 验收标准\n\n- [ ] 永远做不到的事\n", 1)
	if _, err := v.UpdateTask(rec.Meta.ID, cur.Meta.Title, body, "", ""); err != nil {
		t.Fatal(err)
	}
	_, _ = v.Publish(rec.Meta.ID, "jmzx@studio", "a@studio", "")
	_, _ = v.Claim(rec.Meta.ID, "a@studio", "")
	id := rec.Meta.ID
	// 无结果归港被硬门禁拦
	if _, err := v.Done(id, "a@studio", "", false, ""); !errors.Is(err, ErrResultRequired) {
		t.Fatalf("验收未完成应 ErrResultRequired，得到 %v", err)
	}
	// --force 不带理由：拒绝
	if _, err := v.Done(id, "a@studio", "", true, ""); err == nil {
		t.Fatal("强制归港不带理由应拒绝")
	}
	// --force + 理由：放行且永久留痕
	r, err := v.Done(id, "a@studio", "", true, "验收项已失效，客户口头确认")
	if err != nil {
		t.Fatal(err)
	}
	if r.Meta.Status != StatusDone {
		t.Fatalf("强制归港后状态错误: %s", r.Meta.Status)
	}
	if !strings.Contains(r.Body, "⚠️ 强制归港") || !strings.Contains(r.Body, "验收项已失效") {
		t.Fatalf("强制归港理由未留痕:\n%s", r.Body)
	}
}

func TestInvalidActionsAndMissing(t *testing.T) {
	v, _ := Open(t.TempDir())
	_, _ = v.NewTask("x", "", "", true)
	_, _ = v.Publish("T0001", "jmzx@studio", "a@studio", "")
	if _, err := v.Claim("T0001", "a@studio", ""); err != nil {
		t.Fatal(err)
	}
	// 重复接令：状态已不匹配
	if _, err := v.Claim("T0001", "b@studio", ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("出港后再接令应拒绝，得到 %v", err)
	}
	// 未派船的草稿不能归港
	if _, err := v.NewTask("y", "", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Done("T0002", "a@studio", "", false, ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("草稿归港应拒绝，得到 %v", err)
	}
	if _, err := v.Claim("T9999", "a@studio", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在任务应 ErrNotFound，得到 %v", err)
	}
}

func TestDraftPublishNoAssignee(t *testing.T) {
	v, _ := Open(t.TempDir())
	rec, _ := v.NewTask("打磨中", ImpHigh, "G", true)
	id := rec.Meta.ID
	pub, err := v.Publish(id, "jmzx@studio", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if pub.Meta.Status != StatusAnchored || pub.Meta.AssigneeValue() != "" {
		t.Fatalf("无指派放行应进锚地，得到 %s/%q", pub.Meta.Status, pub.Meta.AssigneeValue())
	}
	if _, err := v.Publish(id, "jmzx@studio", "", ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("非草稿再次放行应拒绝，得到 %v", err)
	}
}

func TestSessionPresence(t *testing.T) {
	v, _ := Open(t.TempDir())
	// 同一艘船在两个项目目录各开一个会话
	if _, err := v.PresentSession(Presence{
		Actor: "claude@studio", SessionID: "s1", Dir: "/repo/jiepai",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.PresentSession(Presence{
		Actor: "claude@studio", SessionID: "s2", Dir: "/repo/mindport",
	}); err != nil {
		t.Fatal(err)
	}
	ps, _ := v.ListPresence()
	if len(ps) != 2 {
		t.Fatalf("同船两会话应有两条心跳，得到 %d", len(ps))
	}
	// claim 把任务号写到最近活动的会话
	if err := v.SetActorSessionTask("claude@studio", "T0009"); err != nil {
		t.Fatal(err)
	}
	ps, _ = v.ListPresence()
	var tasked int
	for _, p := range ps {
		if p.Task == "T0009" {
			tasked++
		}
	}
	if tasked != 1 {
		t.Fatalf("应只更新最近一个会话的任务号，得到 %+v", ps)
	}
	// 保活只刷自己会话的 seen，不动另一个
	if err := v.KeepaliveSession("claude@studio", "s2"); err != nil {
		t.Fatal(err)
	}
	// 一个会话下线不影响另一个
	if err := v.AwaySession("claude@studio", "s1"); err != nil {
		t.Fatal(err)
	}
	ps, _ = v.ListPresence()
	if len(ps) != 1 || ps[0].SessionID != "s2" {
		t.Fatalf("s1 下线后应只剩 s2，得到 %+v", ps)
	}
	// 归港清任务号
	if err := v.ClearActorTask("claude@studio"); err != nil {
		t.Fatal(err)
	}
	ps, _ = v.ListPresence()
	if ps[0].Task != "" {
		t.Fatalf("归港后任务号应清空，得到 %+v", ps[0])
	}
}

func TestBerthDispatchAndClaim(t *testing.T) {
	v, _ := Open(t.TempDir())
	rec := mustNewTask(t, v, "街拍 app 的活", "high")
	if _, err := v.Publish(rec.Meta.ID, "jmzx@studio", "claude@studio", "/repo/jiepai"); err != nil {
		t.Fatal(err)
	}
	got, err := v.GetTask(rec.Meta.ID)
	if err != nil || got.Meta.AssignCwd != "/repo/jiepai" {
		t.Fatalf("船位未写入: %+v err=%v", got.Meta, err)
	}
	// 别的项目目录的会话看不到这票货
	if _, err := v.Next("claude@studio", "/repo/mindport"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("非指定船位 next 应找不到，得到 %v", err)
	}
	if _, err := v.Next("claude@studio", "/repo/jiepai"); err != nil {
		t.Fatalf("指定船位 next 应命中: %v", err)
	}
	// 在错误船位 claim 被拒
	if _, err := v.Claim(rec.Meta.ID, "claude@studio", "/repo/mindport"); !errors.Is(err, ErrWrongBerth) {
		t.Fatalf("错误船位接令应 ErrWrongBerth，得到 %v", err)
	}
	// 正确船位接令成功
	if _, err := v.Claim(rec.Meta.ID, "claude@studio", "/repo/jiepai"); err != nil {
		t.Fatalf("正确船位应能接令: %v", err)
	}
	// 不指定船位的货任意目录可接（兼容旧行为/远程船）
	rec2 := mustNewTask(t, v, "通用的活", "normal")
	if _, err := v.Publish(rec2.Meta.ID, "jmzx@studio", "codex@studio", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Claim(rec2.Meta.ID, "codex@studio", "/any/where"); err != nil {
		t.Fatalf("无船位任务应任意目录可接: %v", err)
	}
}

func TestPresenceAwayTTLAndDebounce(t *testing.T) {
	v, _ := Open(t.TempDir())
	if _, err := v.Present("", "T1", ""); err == nil {
		t.Fatal("空 actor 应报错")
	}
	wrote, err := v.Present("claude@studio", "T1", "写代码")
	if err != nil || !wrote {
		t.Fatalf("首次心跳应写盘，wrote=%v err=%v", wrote, err)
	}
	// 20s 防抖：同内容再来一次不写盘
	if wrote, _ := v.Present("claude@studio", "T1", "写代码"); wrote {
		t.Fatal("相同内容短时间内心跳应被防抖")
	}
	// 状态行变化立即写
	if wrote, _ := v.Present("claude@studio", "T1", "跑测试"); !wrote {
		t.Fatal("状态变化应立即写盘")
	}
	if err := v.Away("claude@studio"); err != nil {
		t.Fatal(err)
	}
	ps, _ := v.ListPresence()
	if len(ps) != 0 {
		t.Fatalf("away 后不应再有心跳，得到 %+v", ps)
	}
	if err := v.Away("claude@studio"); err != nil {
		t.Fatal("重复 away 应当幂等无错")
	}
}

func mustNewTask(t *testing.T, v *Vault, title, imp string) *Record {
	t.Helper()
	r, err := v.NewTask(title, imp, "", true)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func ids(rs []Record) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Meta.ID
	}
	return out
}

func baseName(p string) string {
	i := strings.LastIndex(p, "/")
	return p[i+1:]
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteTask(t *testing.T) {
	v, _ := Open(t.TempDir())

	draft, err := v.NewTask("没打磨完的闪念", "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	anchored, _ := v.NewTask("锚地待派的货", "", "", false)
	dispatched, _ := v.NewTask("已派船的货", "", "", false)
	if _, err := v.Assign(dispatched.Meta.ID, "test", "claude@studio", ""); err != nil {
		t.Fatal(err)
	}

	// 草稿可删：文件抹掉，再查 ErrNotFound
	if err := v.DeleteTask(draft.Meta.ID); err != nil {
		t.Fatalf("草稿应可删除: %v", err)
	}
	if _, err := v.GetTask(draft.Meta.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删除后 GetTask 应 ErrNotFound，得到 %v", err)
	}
	if tasks, _ := v.ListTasks(StatusDraft); len(tasks) != 0 {
		t.Fatalf("删除后草稿列表应为空，得到 %d 条", len(tasks))
	}

	// 锚地待派（未出港、无流转历史）同样可删
	if err := v.DeleteTask(anchored.Meta.ID); err != nil {
		t.Fatalf("锚地任务应可删除: %v", err)
	}
	if _, err := v.GetTask(anchored.Meta.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("锚地任务删除后应 ErrNotFound，得到 %v", err)
	}

	// 已派船出港的货有流转历史，不能直接删
	if err := v.DeleteTask(dispatched.Meta.ID); !errors.Is(err, ErrNotDraft) {
		t.Fatalf("已派船任务删除应 ErrNotDraft，得到 %v", err)
	}
	if _, err := v.GetTask(dispatched.Meta.ID); err != nil {
		t.Fatalf("被拒后任务应仍在: %v", err)
	}

	// 不存在的编号也是 not found
	if err := v.DeleteTask("T9999"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删除不存在任务应 ErrNotFound，得到 %v", err)
	}
}
