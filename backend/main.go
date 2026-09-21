package main

import (
	"encoding/json"
	"errors"
	"log"
	"sync"

	sdk "github.com/t8y2/dbx/plugins/sdk/go/dbx-plugin-sdk"
)

const pluginID = "io.github.caichangqing1120.prometheus"
const version = "0.1.7"

type request struct {
	Connection   connection      `json:"connection"`
	Runtime      runtimeEndpoint `json:"runtime"`
	ConnectionID string          `json:"connectionId"`
	Form         queryForm       `json:"form"`
}

type plugin struct {
	mu       sync.RWMutex
	sessions map[string]*session
}

func newPlugin() *plugin { return &plugin{sessions: make(map[string]*session)} }

func (p *plugin) Handle(_ sdk.RequestContext, method string, raw json.RawMessage, _ *sdk.Emitter) (any, *sdk.PluginError) {
	var input request
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, sdk.NewError(-32602, "请求参数无效")
	}
	result, err := p.handle(method, input)
	if err != nil {
		return nil, sdk.NewError(-32000, err.Error())
	}
	return result, nil
}

func (p *plugin) handle(method string, input request) (any, error) {
	switch method {
	case "connection/test", "connection/connect":
		if method == "connection/connect" && input.Connection.ID == "" {
			return nil, errors.New("缺少连接 ID")
		}
		s, err := newSession(input.Connection, input.Runtime)
		if err != nil {
			return nil, err
		}
		if _, err = s.call("/api/v1/status/buildinfo", nil); err != nil {
			s.close()
			return nil, err
		}
		if method == "connection/test" {
			s.close()
			return map[string]any{"success": true, "message": "Prometheus API 连接成功"}, nil
		}
		p.mu.Lock()
		old := p.sessions[input.Connection.ID]
		p.sessions[input.Connection.ID] = s
		p.mu.Unlock()
		if old != nil {
			old.close()
		}
		return map[string]any{"success": true}, nil
	case "connection/disconnect":
		if input.Connection.ID == "" {
			return nil, errors.New("缺少连接 ID")
		}
		p.mu.Lock()
		old := p.sessions[input.Connection.ID]
		delete(p.sessions, input.Connection.ID)
		p.mu.Unlock()
		if old != nil {
			old.close()
		}
		return map[string]any{"success": true}, nil
	}
	if method != "prometheus/info" && method != "prometheus/query" && method != "prometheus/query_range" && method != "prometheus/targets" && method != "prometheus/alerts" && method != "prometheus/rules" && method != "prometheus/metric_names" && method != "prometheus/label_names" && method != "prometheus/label_values" && method != "prometheus/status_runtime" && method != "prometheus/status_tsdb" && method != "prometheus/status_flags" && method != "prometheus/status_config" && method != "prometheus/service_discovery_services" && method != "prometheus/service_discovery" {
		return nil, errors.New("不支持的操作")
	}
	p.mu.RLock()
	s := p.sessions[input.ConnectionID]
	p.mu.RUnlock()
	if s == nil {
		return nil, errors.New("连接尚未建立或已断开，请重新连接")
	}
	return s.execute(method, input.Form)
}

func main() {
	server := sdk.NewServer(sdk.Metadata{ID: pluginID, Version: version, Capabilities: []string{"connections"}}, newPlugin())
	if err := server.Serve(); err != nil {
		log.Fatal(err)
	}
}
