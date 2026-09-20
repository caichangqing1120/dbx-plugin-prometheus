package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type connection struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Host            string           `json:"host"`
	Port            int              `json:"port"`
	Username        string           `json:"username"`
	Password        string           `json:"password"`
	Config          config           `json:"external_config"`
	TransportLayers []transportLayer `json:"transport_layers"`
}

type config struct {
	Scheme      string `json:"scheme"`
	ContextPath string `json:"context_path"`
	Environment string `json:"environment"`
}
type transportLayer struct {
	Type    string `json:"type"`
	Enabled *bool  `json:"enabled"`
}
type runtimeEndpoint struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}
type queryForm struct {
	Query      string  `json:"query"`
	MetricName string  `json:"metricName"`
	LabelName  string  `json:"labelName"`
	Time       float64 `json:"time"`
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	Step       float64 `json:"step"`
}
type session struct {
	mu          sync.RWMutex
	closed      bool
	base        string
	name        string
	environment string
	username    string
	password    string
	client      *http.Client
	transport   *http.Transport
}

var metricNamePattern = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
var labelNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func newSession(c connection, runtime runtimeEndpoint) (*session, error) {
	scheme := c.Config.Scheme
	if scheme == "" {
		scheme = "http"
	}
	if scheme != "http" && scheme != "https" {
		return nil, errors.New("只支持 HTTP 或 HTTPS")
	}
	host := strings.TrimSpace(c.Host)
	if host == "" || strings.ContainsAny(host, "/@?#\\% \t\r\n[]") || (strings.Contains(host, ":") && net.ParseIP(host) == nil) {
		return nil, errors.New("IP / 域名无效，请勿包含协议、端口或路径")
	}
	if c.Port < 1 || c.Port > 65535 {
		return nil, errors.New("端口必须为 1 到 65535")
	}
	path := c.Config.ContextPath
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "?#%\\ \t\r\n") {
		return nil, errors.New("应用路径必须以 / 开头且不含编码、查询或片段")
	}
	for _, part := range strings.Split(path, "/") {
		if part == "." || part == ".." {
			return nil, errors.New("应用路径不可包含 . 或 ..")
		}
	}
	if (c.Username == "") != (c.Password == "") {
		return nil, errors.New("Basic Auth 用户名和密码必须同时填写")
	}
	base := (&url.URL{Scheme: scheme, Host: net.JoinHostPort(host, strconv.Itoa(c.Port)), Path: strings.TrimRight(path, "/")}).String()
	transport := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConnsPerHost: 4,
	}
	active := false
	for _, layer := range c.TransportLayers {
		if layer.Enabled != nil && !*layer.Enabled {
			continue
		}
		if layer.Type != "ssh" && layer.Type != "proxy" && layer.Type != "http_tunnel" {
			return nil, errors.New("不支持的 DBX 传输层")
		}
		active = true
	}
	if active {
		if runtime.Host == "" || strings.ContainsAny(runtime.Host, "/@?#\\ \t\r\n") || runtime.Port < 1 || runtime.Port > 65535 {
			return nil, errors.New("DBX 未提供有效隧道端点，不会绕过代理直连")
		}
		endpoint := net.JoinHostPort(runtime.Host, strconv.Itoa(runtime.Port))
		transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, endpoint)
		}
	}
	return &session{base: base, name: c.Name, environment: c.Config.Environment, username: c.Username, password: c.Password, transport: transport,
		client: &http.Client{Transport: transport, Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (s *session) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.transport.CloseIdleConnections()
}

func (s *session) execute(method string, form queryForm) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errors.New("连接已断开")
	}
	switch method {
	case "prometheus/info":
		data, err := s.call("/api/v1/status/buildinfo", nil)
		if err != nil {
			return nil, err
		}
		return map[string]any{"name": s.name, "baseUrl": s.base, "environment": s.environment, "build": data}, nil
	case "prometheus/targets":
		return s.call("/api/v1/targets", url.Values{"state": {"active"}})
	case "prometheus/alerts":
		return s.call("/api/v1/alerts", nil)
	case "prometheus/rules":
		return s.call("/api/v1/rules", nil)
	case "prometheus/status_runtime":
		return s.call("/api/v1/status/runtimeinfo", nil)
	case "prometheus/status_tsdb":
		return s.call("/api/v1/status/tsdb", url.Values{"limit": {"10"}})
	case "prometheus/status_flags":
		return s.call("/api/v1/status/flags", nil)
	case "prometheus/status_config":
		return s.call("/api/v1/status/config", nil)
	case "prometheus/service_discovery":
		return s.call("/api/v1/targets", url.Values{"state": {"any"}})
	case "prometheus/metric_names":
		return s.call("/api/v1/label/__name__/values", nil)
	case "prometheus/label_names":
		values := url.Values{}
		if form.MetricName != "" {
			if !metricNamePattern.MatchString(form.MetricName) {
				return nil, errors.New("指标名称无效")
			}
			values.Set("match[]", form.MetricName)
		}
		return s.call("/api/v1/labels", values)
	case "prometheus/label_values":
		if !labelNamePattern.MatchString(form.LabelName) {
			return nil, errors.New("标签名称无效")
		}
		values := url.Values{}
		if form.MetricName != "" {
			if !metricNamePattern.MatchString(form.MetricName) {
				return nil, errors.New("指标名称无效")
			}
			values.Set("match[]", form.MetricName)
		}
		return s.call("/api/v1/label/"+url.PathEscape(form.LabelName)+"/values", values)
	case "prometheus/query", "prometheus/query_range":
		q := strings.TrimSpace(form.Query)
		if q == "" || len(q) > 4096 {
			return nil, errors.New("PromQL 不能为空且不能超过 4096 字符")
		}
		v := url.Values{"query": {q}}
		if method == "prometheus/query" {
			if form.Time != 0 {
				if !finite(form.Time) || form.Time <= 0 || form.Time > float64(time.Now().Add(time.Minute).Unix()) {
					return nil, errors.New("评估时间无效")
				}
				v.Set("time", strconv.FormatFloat(form.Time, 'f', -1, 64))
			}
			return s.call("/api/v1/query", v)
		}
		if !finite(form.Start) || !finite(form.End) || !finite(form.Step) || form.Start <= 0 || form.End <= form.Start || form.End > float64(time.Now().Add(time.Minute).Unix()) || form.Step <= 0 || form.Step < 1 || (form.End-form.Start)/form.Step > 11000 {
			return nil, errors.New("时间范围或步长无效（最多 11000 个点，步长至少 1 秒）")
		}
		v.Set("start", strconv.FormatFloat(form.Start, 'f', -1, 64))
		v.Set("end", strconv.FormatFloat(form.End, 'f', -1, 64))
		v.Set("step", strconv.FormatFloat(form.Step, 'f', -1, 64))
		return s.call("/api/v1/query_range", v)
	}
	return nil, errors.New("不支持的操作")
}

func finite(n float64) bool { return n == n && n < 1e300 && n > -1e300 }

func (s *session) call(path string, values url.Values) (any, error) {
	u := s.base + path
	if values != nil {
		u += "?" + values.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if s.username != "" {
		req.SetBasicAuth(s.username, s.password)
	}
	req.Header.Set("Accept", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Prometheus API 请求失败: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("Prometheus API 返回 HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 4<<20 {
		return nil, errors.New("响应超过 4 MiB 限制，请缩小查询范围")
	}
	var payload struct {
		Status    string          `json:"status"`
		Data      json.RawMessage `json:"data"`
		ErrorType string          `json:"errorType"`
		Error     string          `json:"error"`
		Warnings  []string        `json:"warnings"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("Prometheus API 响应不是有效 JSON")
	}
	if payload.Status != "success" {
		if payload.Error != "" {
			return nil, fmt.Errorf("Prometheus API %s: %s", payload.ErrorType, payload.Error)
		}
		return nil, errors.New("Prometheus API 未返回成功状态")
	}
	if len(payload.Data) == 0 || string(payload.Data) == "null" {
		return nil, errors.New("Prometheus API 缺少数据")
	}
	var data any
	if err := json.Unmarshal(payload.Data, &data); err != nil {
		return nil, errors.New("Prometheus API 数据无效")
	}
	return map[string]any{"data": data, "warnings": payload.Warnings}, nil
}
