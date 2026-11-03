package store

import (
	"strings"
	"testing"
)

func TestSessionMarkLifecycle(t *testing.T) {
	v, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const sid = "sess-mark-0001"

	if _, ok := v.SessionMark(sid); ok {
		t.Fatalf("初始不应有标记")
	}
	if err := v.MarkSession(sid, "/repo/proj", MarkGood); err != nil {
		t.Fatal(err)
	}
	mk, ok := v.SessionMark(sid)
	if !ok || mk.Outcome != MarkGood || mk.Dir != "/repo/proj" {
		t.Fatalf("标记读回错误: %+v ok=%v", mk, ok)
	}
	if all := v.SessionMarks(); len(all) != 1 || all[sid].Outcome != MarkGood {
		t.Fatalf("全量标记错误: %+v", all)
	}

	// 重复打即覆盖（good → stop）
	if err := v.MarkSession(sid, "/repo/proj", MarkStop); err != nil {
		t.Fatal(err)
	}
	if mk, _ := v.SessionMark(sid); mk.Outcome != MarkStop {
		t.Fatalf("覆盖后应为 stop，得到 %q", mk.Outcome)
	}

	// 撤标记
	if err := v.UnmarkSession(sid); err != nil {
		t.Fatal(err)
	}
	if _, ok := v.SessionMark(sid); ok {
		t.Fatalf("撤标记后应查无")
	}
	if err := v.UnmarkSession(sid); err != nil {
		t.Fatalf("重复撤不应报错，得到 %v", err)
	}

	// 非法取值 / 非法 id
	if err := v.MarkSession(sid, "", "maybe"); err == nil {
		t.Fatalf("非法结局应拒绝")
	}
	if err := v.MarkSession("", "", MarkGood); err == nil {
		t.Fatalf("空 sid 应拒绝")
	}
	if err := v.MarkSession(strings.Repeat("x", 65), "", MarkGood); err == nil {
		t.Fatalf("超长 sid 应拒绝")
	}
}

func TestBindSessionOrthogonalToStatus(t *testing.T) {
	v, _ := Open(t.TempDir())
	r, err := v.NewTask("关联会话的任务", ImpHigh, "", true)
	if err != nil {
		t.Fatal(err)
	}
	id := r.Meta.ID
	const sid1 = "sess-fleet-aaaa"
	const sid2 = "sess-fleet-bbbb"

	// 草稿态也能挂（与指派状态机正交）
	got, err := v.BindSession(id, sid1, "/repo/proj", "web")
	if err != nil {
		t.Fatal(err)
	}
	if got.Meta.Session != sid1 || got.Meta.SessionDir != "/repo/proj" {
		t.Fatalf("挂载字段错误: %+v", got.Meta)
	}
	if !strings.Contains(got.Body, "web：关联到会话 sess-fle（proj）") {
		t.Fatalf("挂载日志错误:\n%s", got.Body)
	}
	if bound, _ := v.TasksBySession(sid1); len(bound) != 1 || bound[0].Meta.ID != id {
		t.Fatalf("TasksBySession 应找到 %s，得到 %+v", id, bound)
	}

	// 换会话：旧会话清空，新会话能查到
	if _, err := v.BindSession(id, sid2, "", ""); err != nil {
		t.Fatal(err)
	}
	if b, _ := v.TasksBySession(sid1); len(b) != 0 {
		t.Fatalf("换会话后旧会话不应再有，得到 %d", len(b))
	}
	if b, _ := v.TasksBySession(sid2); len(b) != 1 {
		t.Fatalf("新会话应有 1 个，得到 %d", len(b))
	}

	// 取下
	got, err = v.UnbindSession(id, "web")
	if err != nil {
		t.Fatal(err)
	}
	if got.Meta.Session != "" || got.Meta.SessionDir != "" {
		t.Fatalf("取下后字段应空: %+v", got.Meta)
	}
	if !strings.Contains(got.Body, "web：取消关联会话") {
		t.Fatalf("取下日志错误:\n%s", got.Body)
	}
	if b, _ := v.TasksBySession(sid2); len(b) != 0 {
		t.Fatalf("取下后会话不应再挂，得到 %d", len(b))
	}
	// 幂等：本来没挂时取下不报错
	if _, err := v.UnbindSession(id, ""); err != nil {
		t.Fatalf("空挂时取下不应报错，得到 %v", err)
	}

	// 挂到不存在的任务 / 非法 sid
	if _, err := v.BindSession("T9999", sid1, "", ""); err == nil {
		t.Fatalf("不存在的任务应拒绝")
	}
	if _, err := v.BindSession(id, "", "", ""); err == nil {
		t.Fatalf("空 sid 应拒绝")
	}
}
