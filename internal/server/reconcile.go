// 状态纠偏：CLI agents 口把「AskUserQuestion 挂起等用户回答」也报成
// busy/working（turn 未结束），网页桥事件态同理。渲染出口统一在这过一遍
// transcript 尾部的幂等事实，把这类 busy 翻成 waiting。
// 判定本身查询幂等；按文件 (size,mtime) 缓存只是省解析，文件不动结论不变。
package server

import (
	"os"
	"time"

	"github.com/bestksl/mindport/internal/transcript"
)

type waitCacheEntry struct {
	size    int64
	modTime time.Time
	waiting bool
}

func (s *Server) reconcileBusy(sid, dir, status string) string {
	if status != "busy" || dir == "" || isAnonSID(sid) {
		return status
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return status
	}
	path := transcript.Path(home, dir, sid)
	st, err := os.Stat(path)
	if err != nil {
		return status
	}
	size, mod := st.Size(), st.ModTime()
	s.waitMu.RLock()
	e, ok := s.waitCache[sid]
	s.waitMu.RUnlock()
	if ok && e.size == size && e.modTime.Equal(mod) {
		if e.waiting {
			return "waiting"
		}
		return "busy"
	}
	_, pending := transcript.PendingHumanTool(path)
	s.waitMu.Lock()
	if s.waitCache == nil { // 测试里绕过 New 构造的 Server
		s.waitCache = map[string]waitCacheEntry{}
	}
	s.waitCache[sid] = waitCacheEntry{size: size, modTime: mod, waiting: pending}
	s.waitMu.Unlock()
	if pending {
		return "waiting"
	}
	return "busy"
}
