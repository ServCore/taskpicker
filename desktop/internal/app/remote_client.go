package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"taskpicker/internal/reader"
)

type remoteCaller interface {
	Call(context.Context, string, map[string]any) (map[string]any, error)
}

type docsRemoteClient struct {
	token    string
	client   *http.Client
	endpoint string
}

func newDocsRemoteClient(ctx context.Context, root string) (*docsRemoteClient, error) {
	client := &http.Client{Timeout: 40 * time.Second}
	token, err := remoteAccessToken(ctx, root, client)
	if err != nil {
		return nil, err
	}
	return &docsRemoteClient{token: token, client: client, endpoint: remoteBase}, nil
}

func (c *docsRemoteClient) Call(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("MCP-Protocol-Version", "2025-03-26")
	response, err := c.client.Do(req)
	if err != nil {
		return nil, errors.New("腾讯文档连接失败；本地修改已保留，请重试。")
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return nil, errors.New("腾讯文档拒绝访问，请检查账号编辑权限或重新授权。")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("腾讯文档服务异常（HTTP %d），本地修改已保留。", response.StatusCode)
	}
	body, err := reader.LimitedRead(response.Body, 16<<20)
	if err != nil {
		return nil, err
	}
	if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		scanner := bufio.NewScanner(bytes.NewReader(body))
		scanner.Buffer(make([]byte, 4096), 16<<20)
		var data strings.Builder
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		body = []byte(data.String())
	}
	var envelope struct {
		Result struct {
			Structured map[string]any `json:"structuredContent"`
			Content    []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return nil, errors.New("腾讯文档返回了无法识别的结果。")
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("腾讯文档接口失败：%s", envelope.Error.Message)
	}
	result := envelope.Result.Structured
	if result == nil {
		for _, block := range envelope.Result.Content {
			if block.Type == "text" && json.Unmarshal([]byte(block.Text), &result) == nil {
				break
			}
		}
	}
	if envelope.Result.IsError || result == nil {
		return nil, errors.New("腾讯文档操作失败，请检查文档权限和授权状态。")
	}
	if message := reader.StringValue(result["error"]); message != "" {
		return nil, fmt.Errorf("腾讯文档：%s", message)
	}
	return result, nil
}
