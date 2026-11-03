// Web 访问鉴权：设备级会话模型。
//
// serve 绑定非本机地址后，网页既能看到全部任务也能看会话 transcript
// （含命令/代码/思考），必须先验证身份。单用户模型下不做账号体系：
//   - 配对（推荐）：电脑端 `port pair` 铸一次性短码，手机扫码交换成该设备
//     独立的会话凭证（存 .port/state/web-sessions/），可在 `port devices`
//     里单独吊销，不影响其他设备；
//   - 密钥登录（兜底）：粘贴 `port web-token` 的共享密钥，同样签发设备会话；
//   - 旧版 mp_key  cookie（值即共享密钥）继续兼容放行，自然淘汰。
package server

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/bestksl/mindport/internal/store"
)

const (
	cookieName      = "mp_sid"
	legacyCookie    = "mp_key"
	cookieDays      = 30
	loginRateWindow = time.Minute
	loginRateMax    = 10
)

// loginRate 限制单 IP 的登录/配对尝试，每分钟最多 loginRateMax 次。
type loginRate struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newLoginRate() *loginRate { return &loginRate{hits: map[string][]time.Time{}} }

func (l *loginRate) allow(ip string) bool {
	if ip == "127.0.0.1" || ip == "::1" {
		return true
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	hits := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if now.Sub(t) <= loginRateWindow {
			hits = append(hits, t)
		}
	}
	if len(hits) >= loginRateMax {
		l.hits[ip] = hits
		return false
	}
	l.hits[ip] = append(hits, now)
	return true
}

// WebTokenPath 是共享密钥文件路径（运行时状态，不进任务目录）。
func WebTokenPath(root string) string {
	return filepath.Join(root, ".port", "state", "web.token")
}

// EnsureWebToken 读取密钥；不存在则生成 32B 随机 hex（0600）。
func EnsureWebToken(root string) (string, error) {
	p := WebTokenPath(root)
	if b, err := os.ReadFile(p); err == nil {
		tok := strings.TrimSpace(string(b))
		if len(tok) == 64 {
			return tok, nil
		}
	}
	var raw [32]byte
	if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(raw[:])
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, []byte(tok+"\n"), 0o600); err != nil {
		return "", err
	}
	return tok, nil
}

// EnableAuth 让该服务开始强制鉴权（仅 serve 命令调用；测试构造的 Server 不锁）。
func (s *Server) EnableAuth() error {
	tok, err := EnsureWebToken(s.V.Root)
	if err != nil {
		return err
	}
	s.authToken = tok
	s.rate = newLoginRate()
	return nil
}

func (s *Server) authOn() bool { return s.authToken != "" }

// publicPath 是无需登录即可访问的路径（静态资产 + 登录/配对口本身）。
func publicPath(p string) bool {
	switch {
	case p == "/login" || p == "/logout" || p == "/pair" || p == "/api/pair":
		return true
	case strings.HasPrefix(p, "/static/"):
		return true
	case p == "/sw.js" || p == "/icon.svg" || p == "/manifest.webmanifest":
		return true
	}
	return false
}

// withAuth 包住整棵路由树：设备会话或旧共享密钥 cookie 均可放行；
// 未登录的网页请求 302 到 /login，API 请求 401。
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authOn() || publicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if s.validSession(r) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, `{"error":"unauthorized"}`)
			return
		}
		to := "/login"
		if r.URL.Path != "/" {
			to = "/login?next=" + url.QueryEscape(r.URL.RequestURI())
		}
		http.Redirect(w, r, to, http.StatusSeeOther)
	})
}

// validSession 校验设备会话 cookie，旧版共享密钥 cookie 继续兼容。
func (s *Server) validSession(r *http.Request) bool {
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		if sess, err := s.V.LookupWebSession(c.Value); err == nil && sess != nil {
			return true
		}
	}
	if c, err := r.Cookie(legacyCookie); err == nil && c.Value != "" &&
		subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.authToken)) == 1 {
		return true
	}
	return false
}

func (s *Server) setSessionCookie(w http.ResponseWriter, sid string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    sid,
		Path:     "/",
		MaxAge:   cookieDays * 86400,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// deviceLabel 从 User-Agent 推导一个人类可读的设备名。
func deviceLabel(ua string) string {
	switch {
	case strings.Contains(ua, "iPhone"):
		return "iPhone"
	case strings.Contains(ua, "iPad"):
		return "iPad"
	case strings.Contains(ua, "Android"):
		if strings.Contains(ua, "Mobile") {
			return "Android 手机"
		}
		return "Android 平板"
	case strings.Contains(ua, "Macintosh"):
		return "Mac"
	case strings.Contains(ua, "Windows"):
		return "Windows 电脑"
	case strings.Contains(ua, "CrOS"):
		return "Chromebook"
	case strings.Contains(ua, "Linux"), strings.Contains(ua, "X11"):
		return "Linux 电脑"
	default:
		return "未知设备"
	}
}

// handleLogin GET 渲染登录页；POST 校验共享密钥后签发设备会话 cookie。
// 推荐路径是 /pair 扫码；密钥登录只作兜底。
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.authOn() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodPost {
		if !s.rate.allow(clientIP(r)) {
			render(w, http.StatusTooManyRequests, loginTmpl, map[string]any{"Error": "尝试太频繁，稍后再试", "Next": safeNext(r.PostFormValue("next"))})
			return
		}
		_ = r.ParseForm()
		tok := r.PostFormValue("key")
		if tok == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(s.authToken)) != 1 {
			render(w, http.StatusOK, loginTmpl, map[string]any{"Error": "密钥不对，再试一次", "Next": safeNext(r.PostFormValue("next"))})
			return
		}
		sess, err := s.V.CreateWebSession(deviceLabel(r.UserAgent()), r.UserAgent(), clientIP(r))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.setSessionCookie(w, sess.ID)
		// 旧版「cookie 即密钥」清掉，换成设备凭证。
		http.SetCookie(w, &http.Cookie{Name: legacyCookie, Path: "/", MaxAge: -1})
		http.Redirect(w, r, safeNext(r.PostFormValue("next")), http.StatusSeeOther)
		return
	}
	render(w, http.StatusOK, loginTmpl, map[string]any{"Next": safeNext(r.URL.Query().Get("next"))})
}

// handlePair 渲染配对落地页（短码在 URL fragment 里，由页面 JS 换取会话，
// 不进服务端日志）。未启用鉴权时配对无意义，回首屏。
func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	if !s.authOn() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	render(w, http.StatusOK, pairTmpl, map[string]any{"Err": r.URL.Query().Get("err")})
}

// handlePairAPI 用一次性配对码交换设备会话。JSON 与表单提交都支持：
// 前者给配对页 fetch 用（JSON 响应），后者给无 JS 兜底（302 跳转）。
func (s *Server) handlePairAPI(w http.ResponseWriter, r *http.Request) {
	if !s.authOn() {
		// 无锁模式没有配对这回事，一律回首屏（与 /login 行为一致）。
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/pair", http.StatusSeeOther)
		return
	}
	wantJSON := strings.Contains(r.Header.Get("Content-Type"), "application/json")
	fail := func(msg string, code int) {
		if wantJSON {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			json.NewEncoder(w).Encode(map[string]string{"error": msg})
			return
		}
		http.Redirect(w, r, "/pair?err="+url.QueryEscape(msg), http.StatusSeeOther)
	}
	ip := clientIP(r)
	if !s.rate.allow(ip) {
		fail("尝试太频繁，稍后再试", http.StatusTooManyRequests)
		return
	}
	var code string
	if wantJSON {
		var body struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			fail("请求格式不对", http.StatusBadRequest)
			return
		}
		code = body.Code
	} else {
		_ = r.ParseForm()
		code = r.PostFormValue("code")
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if err := s.V.ConsumePairingCode(code); err != nil {
		msg := "配对码不对"
		switch {
		case errors.Is(err, store.ErrPairingExpired):
			msg = "配对码已过期，请在电脑上重新运行 port pair"
		case errors.Is(err, store.ErrPairingUsed):
			msg = "配对码已被使用过，请重新生成"
		}
		fail(msg, http.StatusForbidden)
		return
	}
	sess, err := s.V.CreateWebSession(deviceLabel(r.UserAgent()), r.UserAgent(), ip)
	if err != nil {
		fail("会话建立失败："+err.Error(), http.StatusInternalServerError)
		return
	}
	s.setSessionCookie(w, sess.ID)
	if wantJSON {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "device": sess.Name})
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLogout 吊销当前设备会话并清 cookie；未启用鉴权时回首屏。
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" && s.authOn() {
		_ = s.V.RevokeWebSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: legacyCookie, Path: "/", MaxAge: -1})
	to := "/login"
	if !s.authOn() {
		to = "/"
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// safeNext 只允许站内相对路径，挡住开放重定向。反斜杠也拒：浏览器会把
// /\evil 规范化成 //evil 当协议相对 URL 跳出去。
func safeNext(p string) string {
	if p == "" || p[0] != '/' || len(p) < 2 || p[1] == '/' || p[1] == '\\' {
		return "/"
	}
	return p
}
