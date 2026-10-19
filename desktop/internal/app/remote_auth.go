package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"taskpicker/internal/reader"
	"taskpicker/internal/storage"
)

const remoteBase = "https://docs.qq.com/openapi/mcp"

type remoteCredentials struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	ClientID     string  `json:"client_id"`
	ExpiresAt    float64 `json:"expires_at"`
	ExpiresIn    int     `json:"expires_in"`
}

type RemoteAuthState struct {
	Authorized bool    `json:"authorized"`
	ExpiresAt  float64 `json:"expires_at"`
}

func remoteAuthPath(root string) string { return filepath.Join(filepath.Dir(root), "remote-auth.json") }

func loadRemoteCredentials(root string) (remoteCredentials, error) {
	var auth remoteCredentials
	data, err := os.ReadFile(remoteAuthPath(root))
	if err != nil {
		return auth, errors.New("请先在文档设置中授权腾讯文档，才能回写状态。")
	}
	if json.Unmarshal(data, &auth) != nil || auth.AccessToken == "" {
		return auth, errors.New("腾讯文档授权无效，请重新授权。")
	}
	return auth, nil
}

func saveRemoteCredentials(root string, auth remoteCredentials) error {
	if err := os.MkdirAll(filepath.Dir(remoteAuthPath(root)), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(auth)
	if err != nil {
		return err
	}
	return storage.AtomicWrite(remoteAuthPath(root), data)
}

func oauthRequest(ctx context.Context, client *http.Client, path string, data any, form bool, result any) error {
	var raw []byte
	var err error
	contentType := "application/json"
	if form {
		raw = []byte(data.(url.Values).Encode())
		contentType = "application/x-www-form-urlencoded"
	} else {
		raw, err = json.Marshal(data)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, remoteBase+"/oauth/"+path, strings.NewReader(string(raw)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return errors.New("腾讯文档授权连接失败，请稍后重试。")
	}
	defer response.Body.Close()
	body, err := reader.LimitedRead(response.Body, 1<<20)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("腾讯文档授权失败（HTTP %d），请重新授权。", response.StatusCode)
	}
	if err := json.Unmarshal(body, result); err != nil {
		return errors.New("腾讯文档授权响应无法识别。")
	}
	return nil
}

func remoteAccessToken(ctx context.Context, root string, client *http.Client) (string, error) {
	auth, err := loadRemoteCredentials(root)
	if err != nil {
		return "", err
	}
	if auth.ExpiresAt > float64(time.Now().Unix()+60) {
		return auth.AccessToken, nil
	}
	if auth.RefreshToken == "" {
		return "", errors.New("腾讯文档授权已过期，请重新授权。")
	}
	var refreshed remoteCredentials
	err = oauthRequest(ctx, client, "token", url.Values{"grant_type": {"refresh_token"}, "client_id": {auth.ClientID}, "refresh_token": {auth.RefreshToken}}, true, &refreshed)
	if err != nil || refreshed.AccessToken == "" {
		return "", errors.New("腾讯文档授权已过期，请重新授权。")
	}
	refreshed.ClientID = auth.ClientID
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = auth.RefreshToken
	}
	refreshed.ExpiresAt = float64(time.Now().Unix() + int64(refreshed.ExpiresIn))
	if err = saveRemoteCredentials(root, refreshed); err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

func (a *App) GetRemoteAuth() RemoteAuthState {
	auth, err := loadRemoteCredentials(a.root)
	return RemoteAuthState{Authorized: err == nil && (auth.ExpiresAt > float64(time.Now().Unix()) || auth.RefreshToken != ""), ExpiresAt: auth.ExpiresAt}
}

func randomOAuthString() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), err
}

func (a *App) AuthorizeRemote() (RemoteAuthState, error) {
	ctx, finish, err := a.startStatusOperation(10 * time.Minute)
	if err != nil {
		return RemoteAuthState{}, err
	}
	defer finish()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return RemoteAuthState{}, err
	}
	defer listener.Close()
	redirect := "http://" + listener.Addr().String() + "/callback"
	verifier, err := randomOAuthString()
	if err != nil {
		return RemoteAuthState{}, err
	}
	state, err := randomOAuthString()
	if err != nil {
		return RemoteAuthState{}, err
	}
	client := &http.Client{Timeout: 25 * time.Second}
	var registration struct {
		ClientID string `json:"client_id"`
	}
	err = oauthRequest(ctx, client, "register", map[string]any{"client_name": "我的任务 · 本地任务库", "redirect_uris": []string{redirect}, "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "scope": "docs:read docs:write", "token_endpoint_auth_method": "none"}, false, &registration)
	if err != nil {
		return RemoteAuthState{}, err
	}
	if registration.ClientID == "" {
		return RemoteAuthState{}, errors.New("授权应用注册失败。")
	}
	type callback struct {
		code string
		err  error
	}
	result := make(chan callback, 1)
	handler := http.NewServeMux()
	handler.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method != http.MethodGet || q.Get("state") != state {
			http.Error(w, "授权校验失败", http.StatusBadRequest)
			return
		}
		entry := callback{code: q.Get("code")}
		if q.Get("error") != "" || entry.code == "" {
			entry.err = errors.New("授权未完成，请重新尝试。")
		}
		select {
		case result <- entry:
			fmt.Fprint(w, "<h2>腾讯文档授权已接收</h2><p>请返回“我的任务”查看授权结果。</p>")
		default:
			http.Error(w, "授权已处理", http.StatusConflict)
		}
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go server.Serve(listener)
	hash := sha256.Sum256([]byte(verifier))
	query := url.Values{"authType": {"2"}, "client_id": {registration.ClientID}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"docs:read docs:write"}, "state": {state}, "code_challenge": {base64.RawURLEncoding.EncodeToString(hash[:])}, "code_challenge_method": {"S256"}}
	wruntime.BrowserOpenURL(a.ctx, "https://docs.qq.com/scenario/open-claw.html?"+query.Encode())
	var entry callback
	select {
	case entry = <-result:
	case <-ctx.Done():
		return RemoteAuthState{}, errors.New("腾讯文档授权已取消或超时。")
	}
	if entry.err != nil {
		return RemoteAuthState{}, entry.err
	}
	var auth remoteCredentials
	err = oauthRequest(ctx, client, "token", url.Values{"grant_type": {"authorization_code"}, "client_id": {registration.ClientID}, "code": {entry.code}, "redirect_uri": {redirect}, "code_verifier": {verifier}}, true, &auth)
	if err != nil {
		return RemoteAuthState{}, err
	}
	if auth.AccessToken == "" {
		return RemoteAuthState{}, errors.New("腾讯文档没有返回有效授权。")
	}
	auth.ClientID = registration.ClientID
	auth.ExpiresAt = float64(time.Now().Unix() + int64(auth.ExpiresIn))
	if err = saveRemoteCredentials(a.root, auth); err != nil {
		return RemoteAuthState{}, err
	}
	return a.GetRemoteAuth(), nil
}
