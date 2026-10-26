// 调度台鉴权（ADR-0008）：serve 绑定 0.0.0.0 后，网页既能看到全部任务也能看会话
// transcript（含命令/代码/思考），必须先有一把共享密钥。单用户内网模型：
// .port/state/web.token 存 32B 随机 hex，登录后写 HttpOnly cookie，不做用户体系。
package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	cookieName = "mp_key"
	cookieDays = 30
)

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
	return nil
}

func (s *Server) authOn() bool { return s.authToken != "" }

// publicPath 是无需登录即可访问的路径（静态资产 + 登录口本身）。
func publicPath(p string) bool {
	switch {
	case p == "/login" || p == "/logout":
		return true
	case strings.HasPrefix(p, "/static/"):
		return true
	case p == "/sw.js" || p == "/icon.svg" || p == "/manifest.webmanifest":
		return true
	}
	return false
}

// withAuth 包住整棵路由树：未登录的网页请求 302 到 /login，API 请求 401。
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authOn() || publicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie(cookieName)
		if err == nil && c.Value != "" &&
			subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.authToken)) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		to := "/login"
		if r.URL.Path != "/" {
			to = "/login?next=" + url.QueryEscape(r.URL.RequestURI())
		}
		http.Redirect(w, r, to, http.StatusSeeOther)
	})
}

// handleLogin GET 渲染入港证页；POST 校验密钥后写 cookie。
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		tok := r.PostFormValue("key")
		if tok == "" || s.authOn() && subtle.ConstantTimeCompare([]byte(tok), []byte(s.authToken)) != 1 {
			render(w, http.StatusOK, loginTmpl, map[string]any{"Error": "密钥不对，再试一次"})
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    tok,
			Path:     "/",
			MaxAge:   cookieDays * 86400,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		to := safeNext(r.PostFormValue("next"))
		http.Redirect(w, r, to, http.StatusSeeOther)
		return
	}
	if !s.authOn() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	render(w, http.StatusOK, loginTmpl, map[string]any{"Next": safeNext(r.URL.Query().Get("next"))})
}

// handleLogout 清 cookie。
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// safeNext 只允许站内相对路径，挡住开放重定向。反斜杠也拒：浏览器会把
// /\evil 规范化成 //evil 当协议相对 URL 跳出去。
func safeNext(p string) string {
	if p == "" || p[0] != '/' || len(p) < 2 || p[1] == '/' || p[1] == '\\' {
		return "/"
	}
	return p
}
