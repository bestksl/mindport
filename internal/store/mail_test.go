package store

import (
	"testing"
)

func TestMailAppendPullIdempotent(t *testing.T) {
	v, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const sid = "sess-abcd-1234"
	if _, err := v.AppendMail(sid, "先把鉴权补上"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.AppendMail(sid, "再加滚动"); err != nil {
		t.Fatal(err)
	}
	if n, _ := v.MailPending(sid); n != 2 {
		t.Fatalf("应有 2 条待投递，得到 %d", n)
	}

	got, err := v.PullMail(sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Text != "先把鉴权补上" ||
		!got[0].Delivered || !got[1].Delivered {
		t.Fatalf("拉取结果错误（拉出即应回标 delivered）: %+v", got)
	}
	if n, _ := v.MailPending(sid); n != 0 {
		t.Fatalf("拉取后应为 0 条待投递，得到 %d", n)
	}
	// 钩子重放/并发再拉：幂等，不重复注入
	again, err := v.PullMail(sid)
	if err != nil || len(again) != 0 {
		t.Fatalf("重复拉取应空且无错，得到 %+v err=%v", again, err)
	}

	// 新留言在已投递历史之后仍能单独拉出
	if _, err := v.AppendMail(sid, "第三条"); err != nil {
		t.Fatal(err)
	}
	got2, err := v.PullMail(sid)
	if err != nil || len(got2) != 1 || got2[0].Text != "第三条" {
		t.Fatalf("应只拉到新留言: %+v err=%v", got2, err)
	}
}

func TestMailRejectsBadSid(t *testing.T) {
	v, _ := Open(t.TempDir())
	for _, bad := range []string{"../etc/passwd", "a/b", "", "x\x00y"} {
		if _, err := v.AppendMail(bad, "留言"); err == nil {
			t.Fatalf("非法 sid %q 应被拒绝", bad)
		}
		if _, err := v.PullMail(bad); err == nil {
			t.Fatalf("非法 sid %q 拉取也应被拒绝", bad)
		}
	}
}
