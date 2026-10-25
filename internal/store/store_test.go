package store

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestThoughtPromoteAndTaskLifecycle(t *testing.T) {
	v, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	th, err := v.AddThought("洗澡时的闪念", "int8 也许够用的完整描述", "cli")
	if err != nil {
		t.Fatal(err)
	}

	// promote：闪念转任务，原文进描述，inbox 文件消失
	t1, err := v.PromoteThought(baseName(th.Path), ImpHigh)
	if err != nil {
		t.Fatal(err)
	}
	if t1.Meta.ID != "T0001" || t1.Meta.Status != StatusReady || t1.Meta.Importance != ImpHigh {
		t.Fatalf("promote 字段错误: %+v", t1.Meta)
	}
	if !strings.Contains(t1.Body, "int8 也许够用的完整描述") {
		t.Fatalf("闪念正文未进入描述:\n%s", t1.Body)
	}
	if thoughts, _ := v.ListThoughts(); len(thoughts) != 0 {
		t.Fatalf("promote 后 inbox 应为空，剩 %d", len(thoughts))
	}

	if _, err := v.Claim("T0001", "claude@studio"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Log("T0001", "基准 27.4ms", "claude@studio"); err != nil {
		t.Fatal(err)
	}
	cur, err := v.GetTask("T0001")
	if err != nil {
		t.Fatal(err)
	}
	bodyWithAccept := strings.Replace(cur.Body, "## 验收标准\n",
		"## 验收标准\n\n- [ ] int8 单帧 ≤ 30ms\n- [ ] 三档结果记入日志\n", 1)
	if _, err := v.UpdateTask("T0001", t1.Meta.Title, bodyWithAccept, ImpHigh, ""); err != nil {
		t.Fatal(err)
	}
	done, total, _, err := v.CheckItem("T0001", 1, true, "claude@studio")
	if err != nil || done != 1 || total != 2 {
		t.Fatalf("check: done=%d total=%d err=%v", done, total, err)
	}
	if _, err := v.Done("T0001", "claude@studio"); err != nil {
		t.Fatal(err)
	}

	r, err := parseFile(t1.Path)
	if err != nil {
		t.Fatal(err)
	}
	if r.Meta.Status != StatusDone || *r.Meta.Assignee != "claude@studio" {
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

func TestBlockUnblock(t *testing.T) {
	v, _ := Open(t.TempDir())
	_, _ = v.NewTask("x", "", "", false)
	_, _ = v.Claim("T0001", "a@studio")

	if _, err := v.Block("T0001", "", "a@studio"); err == nil {
		t.Fatal("空原因应拒绝")
	}
	if r, err := v.Block("T0001", "等 Vs3 实测", "a@studio"); err != nil {
		t.Fatal(err)
	} else if r.Meta.Status != StatusBlocked || r.Meta.BlockedAt == nil {
		t.Fatalf("block 字段错误: %+v", r.Meta)
	}
	if _, err := v.Done("T0001", "a@studio"); err != nil {
		t.Fatal("受阻任务应允许显式归港")
	}

	_, _ = v.NewTask("y", "", "", false)
	_, _ = v.Claim("T0002", "a@studio")
	_, _ = v.Block("T0002", "缺数据", "a@studio")
	r, err := v.Unblock("T0002", "a@studio")
	if err != nil || r.Meta.Status != StatusInProgress || r.Meta.BlockedAt != nil {
		t.Fatalf("unblock 错误: %+v err=%v", r.Meta, err)
	}
	if _, err := v.Unblock("T0002", "a@studio"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("重复 unblock 应拒绝，得到 %v", err)
	}
}

func TestImportanceOrderingAndNext(t *testing.T) {
	v, _ := Open(t.TempDir())
	_, _ = v.NewTask("普通", "", "", false)          // T0001 normal
	_, _ = v.NewTask("关键", ImpCritical, "", false) // T0002
	_, _ = v.NewTask("也许", ImpSomeday, "", false)  // T0003
	_, _ = v.NewTask("重要", ImpHigh, "", false)     // T0004

	next, err := v.Next()
	if err != nil {
		t.Fatal(err)
	}
	if next.Meta.ID != "T0002" {
		t.Fatalf("next 应为最关键的 T0002，得到 %s", next.Meta.ID)
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

func TestPriorityMigrationAndEdit(t *testing.T) {
	dir := t.TempDir()
	v, _ := Open(dir)
	old := "---\ntype: task\nid: T0001\ntitle: 旧文件\nstatus: ready\npriority: high\ncreated: 2026-10-25T20:00:00+08:00\n---\n\n## 描述\n旧正文\n"
	mustWrite(t, dir+"/tasks/T0001-old.md", old)

	r, err := v.GetTask("T0001")
	if err != nil {
		t.Fatal(err)
	}
	if r.Meta.Importance != ImpHigh || r.Meta.Priority != "" {
		t.Fatalf("旧字段迁移失败: %+v", r.Meta)
	}

	upd, err := v.UpdateTask("T0001", "新标题", "## 描述\n新正文\n", ImpCritical, "某个目标")
	if err != nil {
		t.Fatal(err)
	}
	if upd.Meta.Goal != "某个目标" || !strings.Contains(upd.Path, "T0001-新标题.md") {
		t.Fatalf("编辑/改名失败: path=%s goal=%q", upd.Path, upd.Meta.Goal)
	}
	if r2, _ := v.GetTask("T0001"); !strings.Contains(r2.Body, "新正文") {
		t.Fatalf("正文未更新")
	}
}

func TestDoubleClaimInvalidDoneAndMissing(t *testing.T) {
	v, _ := Open(t.TempDir())
	_, _ = v.NewTask("x", "", "", false)
	if _, err := v.Claim("T0001", "a@studio"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Claim("T0001", "b@studio"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("重复领取应拒绝，得到 %v", err)
	}
	if _, err := v.NewTask("y", "", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Done("T0002", "a@studio"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("未领取直接 done 应拒绝，得到 %v", err)
	}
	if _, err := v.Claim("T9999", "a@studio"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在任务应 ErrNotFound，得到 %v", err)
	}
}

func TestDraftPublishAndAssignment(t *testing.T) {
	v, _ := Open(t.TempDir())
	rec, err := v.NewTask("打磨中", ImpHigh, "G", true)
	if err != nil {
		t.Fatal(err)
	}
	id := rec.Meta.ID
	if rec.Meta.Status != StatusDraft {
		t.Fatalf("草稿初始状态应为 draft，得到 %s", rec.Meta.Status)
	}
	// 草稿不进 next，也不能被领取
	if _, err := v.Next(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("草稿期间 next 应空，得到 %v", err)
	}
	if _, err := v.Claim(id, "a@studio"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("草稿应禁止 claim，得到 %v", err)
	}
	// 发布并指派
	pub, err := v.Publish(id, "jmzx@studio", "claude@studio")
	if err != nil {
		t.Fatal(err)
	}
	if pub.Meta.Status != StatusReady || pub.Meta.AssigneeValue() != "claude@studio" {
		t.Fatalf("发布后应为 ready 且带指派，得到 %s/%q", pub.Meta.Status, pub.Meta.AssigneeValue())
	}
	// 二次发布拒绝
	if _, err := v.Publish(id, "jmzx@studio", ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("非草稿再次发布应拒绝，得到 %v", err)
	}
	// 被指派的任务可被领取，领取后心跳在线且绑定任务
	if _, err := v.Claim(id, "claude@studio"); err != nil {
		t.Fatal(err)
	}
	ps, err := v.ListPresence()
	if err != nil || len(ps) != 1 || !ps[0].Online || ps[0].Task != id {
		t.Fatalf("领取后应有一条在线心跳绑定 %s，得到 %+v err=%v", id, ps, err)
	}
	// 归港后心跳仍在但任务号清空
	if _, err := v.Done(id, "claude@studio"); err != nil {
		t.Fatal(err)
	}
	ps, _ = v.ListPresence()
	if ps[0].Task != "" {
		t.Fatalf("归港后心跳任务号应清空，得到 %q", ps[0].Task)
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
