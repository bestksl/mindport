// 会话「在忙还是在等人」的幂等事实判定：只认 transcript 尾部结构，
// 不依赖进程上报或内存事件标记——同一份文件任何时刻重算结论一致。
package transcript

// humanWaitTools 是「tool_use 挂起 = 在等用户」的交互工具：结果要用户在
// UI 上操作才会落盘。实测 CLI 2.1.310 的 agents 口在这期间仍报 busy/working
// （turn 未结束就等于忙，不区分挂起的是什么工具），思港据此自行纠偏。
var humanWaitTools = map[string]bool{
	"AskUserQuestion": true, // 多选/单选提问卡
	"ExitPlanMode":    true, // Plan 模式请用户批准出计划
}

// PendingHumanTool 判定最后一轮 assistant 是否挂着等人回答的交互工具。
// 返回工具名与 true 表示「待你输入」；其余情况 false。
func PendingHumanTool(path string) (string, bool) {
	name, _, pending := lastOpenToolUse(path)
	if !pending || !humanWaitTools[name] {
		return "", false
	}
	return name, true
}

// OpenToolUse 报告最后一条 assistant 消息里是否有尚未闭合（无 tool_result）
// 的工具调用，有则返回工具名。长跑命令无输出期间文件静止但工具仍挂着，
// 据此可区分「还在干活」与「已经停下等人说话」。
func OpenToolUse(path string) (string, bool) {
	name, _, pending := lastOpenToolUse(path)
	return name, pending
}

// lastOpenToolUse 从尾部找最后一个 assistant 工具块：foldToolResults 已把
// 同窗口 tool_result 并入工具卡（Result 非空=闭合）；跨窗口找不到归属的
// result 保留为独立 user result 块，用 resultIDs 兜底。
func lastOpenToolUse(path string) (name, tid string, pending bool) {
	pg, err := ReadTail(path)
	if err != nil || len(pg.Events) == 0 {
		return "", "", false
	}
	resultIDs := map[string]bool{}
	for _, ev := range pg.Events {
		if ev.Role != "user" {
			continue
		}
		for _, b := range ev.Blocks {
			if b.Kind == "result" && b.ToolID != "" {
				resultIDs[b.ToolID] = true
			}
		}
	}
	for i := len(pg.Events) - 1; i >= 0; i-- {
		if pg.Events[i].Role != "assistant" {
			continue
		}
		for j := len(pg.Events[i].Blocks) - 1; j >= 0; j-- {
			b := pg.Events[i].Blocks[j]
			if b.Kind != "tool" {
				continue // 只看该 assistant 消息的最后一个工具块
			}
			if b.Result != "" || resultIDs[b.ToolID] {
				return b.Name, b.ToolID, false
			}
			return b.Name, b.ToolID, true
		}
	}
	return "", "", false
}
