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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
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
	Query       string   `json:"query"`
	MetricName  string   `json:"metricName"`
	LabelName   string   `json:"labelName"`
	Time        float64  `json:"time"`
	Start       float64  `json:"start"`
	End         float64  `json:"end"`
	Step        float64  `json:"step"`
	ScrapePool  string   `json:"scrapePool"`
	ScrapePools []string `json:"scrapePools"`
	State       string   `json:"state"`
	Page        int      `json:"page"`
	PageSize    int      `json:"pageSize"`
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

const responseLimit = 4 << 20
const discoveryUpstreamLimit = 64 << 20
const discoveryTargetLimit = 200000
const discoveryServiceLimit = 20
const finalLabelLimit = 40
const discoveredLabelLimit = 32

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
	case "prometheus/service_discovery_services":
		return s.call("/api/v1/scrape_pools", nil)
	case "prometheus/service_discovery":
		return s.callServiceDiscovery(form)
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
	body, err := s.readResponse(path, values, responseLimit)
	if err != nil {
		return nil, err
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
	if err := validatePayload(payload.Status, payload.ErrorType, payload.Error, payload.Data); err != nil {
		return nil, err
	}
	var data any
	if err := json.Unmarshal(payload.Data, &data); err != nil {
		return nil, errors.New("Prometheus API 数据无效")
	}
	return map[string]any{"data": data, "warnings": payload.Warnings}, nil
}

func (s *session) readResponse(path string, values url.Values, limit int64) ([]byte, error) {
	res, err := s.request(path, values)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("响应超过 %d MiB 限制，请缩小查询范围", limit>>20)
	}
	return body, nil
}

func (s *session) request(path string, values url.Values) (*http.Response, error) {
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
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		res.Body.Close()
		return nil, fmt.Errorf("Prometheus API 返回 HTTP %d", res.StatusCode)
	}
	return res, nil
}

func validatePayload(status, errorType, message string, data json.RawMessage) error {
	if status != "success" {
		if message != "" {
			return fmt.Errorf("Prometheus API %s: %s", errorType, message)
		}
		return errors.New("Prometheus API 未返回成功状态")
	}
	if len(data) == 0 || string(data) == "null" {
		return errors.New("Prometheus API 缺少数据")
	}
	return nil
}

type discoveryTarget struct {
	ScrapePool       string            `json:"scrapePool,omitempty"`
	Health           string            `json:"health,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	DiscoveredLabels map[string]string `json:"discoveredLabels,omitempty"`
	LastScrape       string            `json:"lastScrape,omitempty"`
	LastError        string            `json:"lastError,omitempty"`
	ScrapeURL        string            `json:"scrapeUrl,omitempty"`
}

func (s *session) callServiceDiscovery(form queryForm) (any, error) {
	scrapePools, err := normalizeScrapePools(form)
	if err != nil {
		return nil, err
	}
	if form.State != "active" && form.State != "dropped" {
		return nil, errors.New("目标状态必须为 active 或 dropped")
	}
	if form.Page < 1 || form.Page > 100000 {
		return nil, errors.New("页码必须为 1 到 100000")
	}
	if form.PageSize != 20 && form.PageSize != 50 && form.PageSize != 100 {
		return nil, errors.New("每页条数只支持 20、50 或 100")
	}
	pageStart, pageEnd := (form.Page-1)*form.PageSize, form.Page*form.PageSize
	items := make([]map[string]any, 0, form.PageSize)
	warnings := []string{}
	total, compacted := 0, false
	for _, scrapePool := range scrapePools {
		res, requestErr := s.request("/api/v1/targets", url.Values{"state": {form.State}, "scrapePool": {scrapePool}})
		if requestErr != nil {
			return nil, requestErr
		}
		limited := &io.LimitedReader{R: res.Body, N: discoveryUpstreamLimit + 1}
		windowStart, windowEnd := pageStart-total, pageEnd-total
		if windowStart < 0 {
			windowStart = 0
		}
		if windowEnd < 0 {
			windowEnd = 0
		}
		poolItems, poolTotal, poolWarnings, poolCompacted, decodeErr := decodeDiscoveryResponse(limited, form.State, windowStart, windowEnd)
		res.Body.Close()
		if limited.N <= 0 {
			return nil, fmt.Errorf("服务 %s 的目标响应超过 %d MiB 安全限制", scrapePool, discoveryUpstreamLimit>>20)
		}
		if decodeErr != nil {
			return nil, decodeErr
		}
		if total+poolTotal > discoveryTargetLimit {
			return nil, fmt.Errorf("所选服务目标总数超过 %d 条安全限制", discoveryTargetLimit)
		}
		for _, item := range poolItems {
			if item["scrapePool"] == "" {
				item["scrapePool"] = scrapePool
			}
		}
		items = append(items, poolItems...)
		warnings = append(warnings, poolWarnings...)
		total += poolTotal
		compacted = compacted || poolCompacted
	}
	if compacted {
		warnings = append(warnings, "当前页中的超长标签已精简")
	}
	data := map[string]any{
		"items": items, "total": total, "page": form.Page, "pageSize": form.PageSize,
		"hasNext": form.Page*form.PageSize < total, "scrapePools": scrapePools, "state": form.State,
	}
	return map[string]any{"data": data, "warnings": warnings}, nil
}

func normalizeScrapePools(form queryForm) ([]string, error) {
	values := form.ScrapePools
	if len(values) == 0 && strings.TrimSpace(form.ScrapePool) != "" {
		values = []string{form.ScrapePool}
	}
	if len(values) == 0 {
		return nil, errors.New("请至少选择一个服务")
	}
	if len(values) > discoveryServiceLimit {
		return nil, fmt.Errorf("一次最多选择 %d 个服务", discoveryServiceLimit)
	}
	seen := map[string]struct{}{}
	scrapePools := make([]string, 0, len(values))
	for _, raw := range values {
		scrapePool := strings.TrimSpace(raw)
		if scrapePool == "" || len(scrapePool) > 512 || !utf8.ValidString(scrapePool) || strings.IndexFunc(scrapePool, unicode.IsControl) >= 0 {
			return nil, errors.New("请选择有效的服务")
		}
		if _, exists := seen[scrapePool]; exists {
			continue
		}
		seen[scrapePool] = struct{}{}
		scrapePools = append(scrapePools, scrapePool)
	}
	return scrapePools, nil
}

func decodeDiscoveryResponse(reader io.Reader, state string, start, end int) ([]map[string]any, int, []string, bool, error) {
	decoder := json.NewDecoder(reader)
	if err := expectDelimiter(decoder, '{'); err != nil {
		return nil, 0, nil, false, err
	}
	status, errorType, errorMessage := "", "", ""
	warnings := []string{}
	items := []map[string]any{}
	total, compacted, dataSeen := 0, false, false
	for decoder.More() {
		key, err := readObjectKey(decoder)
		if err != nil {
			return nil, 0, nil, false, err
		}
		switch key {
		case "status":
			err = decoder.Decode(&status)
		case "errorType":
			err = decoder.Decode(&errorType)
		case "error":
			err = decoder.Decode(&errorMessage)
		case "warnings":
			err = decoder.Decode(&warnings)
		case "data":
			items, total, compacted, err = decodeDiscoveryData(decoder, state, start, end)
			dataSeen = err == nil
		default:
			err = skipJSONValue(decoder)
		}
		if err != nil {
			return nil, 0, nil, false, errors.New("Prometheus API 响应不是有效 JSON")
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, 0, nil, false, errors.New("Prometheus API 响应不是有效 JSON")
	}
	if status != "success" {
		if errorMessage != "" {
			return nil, 0, nil, false, fmt.Errorf("Prometheus API %s: %s", errorType, errorMessage)
		}
		return nil, 0, nil, false, errors.New("Prometheus API 未返回成功状态")
	}
	if !dataSeen {
		return nil, 0, nil, false, errors.New("Prometheus API 缺少数据")
	}
	return items, total, warnings, compacted, nil
}

func decodeDiscoveryData(decoder *json.Decoder, state string, start, end int) ([]map[string]any, int, bool, error) {
	if err := expectDelimiter(decoder, '{'); err != nil {
		return nil, 0, false, err
	}
	selected := state + "Targets"
	items := []map[string]any{}
	total, compacted := 0, false
	for decoder.More() {
		key, err := readObjectKey(decoder)
		if err != nil {
			return nil, 0, false, err
		}
		if key != selected {
			if err := skipJSONValue(decoder); err != nil {
				return nil, 0, false, err
			}
			continue
		}
		items, total, compacted, err = decodeTargetArray(decoder, start, end)
		if err != nil {
			return nil, 0, false, err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, 0, false, err
	}
	return items, total, compacted, nil
}

func decodeTargetArray(decoder *json.Decoder, start, end int) ([]map[string]any, int, bool, error) {
	if err := expectDelimiter(decoder, '['); err != nil {
		return nil, 0, false, err
	}
	capacity := end - start
	if capacity < 0 {
		capacity = 0
	}
	items := make([]map[string]any, 0, capacity)
	total, compacted := 0, false
	for decoder.More() {
		if total >= discoveryTargetLimit {
			return nil, 0, false, fmt.Errorf("目标数量超过 %d 条安全限制", discoveryTargetLimit)
		}
		var target discoveryTarget
		if err := decoder.Decode(&target); err != nil {
			return nil, 0, false, err
		}
		if total >= start && total < end {
			item, itemCompacted := compactDiscoveryTarget(target)
			items = append(items, item)
			compacted = compacted || itemCompacted
		}
		total++
	}
	if _, err := decoder.Token(); err != nil {
		return nil, 0, false, err
	}
	return items, total, compacted, nil
}

func compactDiscoveryTarget(target discoveryTarget) (map[string]any, bool) {
	labels, labelsCompacted := compactLabels(target.Labels, finalLabelLimit, false)
	discovered, discoveredCompacted := compactLabels(target.DiscoveredLabels, discoveredLabelLimit, true)
	return map[string]any{
		"scrapePool": truncate(target.ScrapePool, 256), "scrapeUrl": truncate(target.ScrapeURL, 512),
		"health": truncate(target.Health, 64), "lastScrape": truncate(target.LastScrape, 128),
		"lastError": truncate(target.LastError, 512), "labels": labels, "discoveredLabels": discovered,
	}, labelsCompacted || discoveredCompacted
}

func expectDelimiter(decoder *json.Decoder, expected json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != expected {
		return errors.New("unexpected JSON structure")
	}
	return nil
}

func readObjectKey(decoder *json.Decoder) (string, error) {
	token, err := decoder.Token()
	if err != nil {
		return "", err
	}
	key, ok := token.(string)
	if !ok {
		return "", errors.New("unexpected JSON key")
	}
	return key, nil
}

func skipJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		for decoder.More() {
			if _, err := readObjectKey(decoder); err != nil {
				return err
			}
			if err := skipJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := skipJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected JSON structure")
	}
	_, err = decoder.Token()
	return err
}

func compactLabels(input map[string]string, limit int, discovered bool) (map[string]string, bool) {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := discoveryLabelPriority(keys[i], discovered), discoveryLabelPriority(keys[j], discovered)
		if left != right {
			return left < right
		}
		return keys[i] < keys[j]
	})
	output := make(map[string]string, min(limit, len(keys)))
	compacted := len(keys) > limit
	for _, key := range keys {
		if len(output) >= limit {
			break
		}
		value := input[key]
		if len(key) > 128 || len(value) > 512 {
			compacted = true
			continue
		}
		output[key] = truncate(value, 256)
		compacted = compacted || len(value) > 256
	}
	return output, compacted
}

func discoveryLabelPriority(key string, discovered bool) int {
	for index, preferred := range []string{"job", "instance", "__address__", "kubernetes_namespace", "namespace", "pod", "node", "service", "container"} {
		if key == preferred || strings.HasSuffix(key, "_"+preferred) {
			return index
		}
	}
	if discovered && strings.HasPrefix(key, "__meta_kubernetes_") {
		if strings.Contains(key, "_annotation_") {
			return 25
		}
		return 20
	}
	return 30
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
