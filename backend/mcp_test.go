package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func mcpInvoke(t *testing.T, p *plugin, method string, input any) map[string]any {
	t.Helper()
	value, problem := invoke(p, method, input)
	if problem != nil {
		t.Fatalf("%s: %v", method, problem)
	}
	data, _ := json.Marshal(value)
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMCPDiscoveryMatchesDeclaredReadTools(t *testing.T) {
	p := newPlugin()
	result := mcpInvoke(t, p, "mcp/tools", map[string]any{"connectionId": "missing"})
	tools, ok := result["tools"].([]any)
	if !ok || len(tools) != 15 || len(p.sessions) != 0 {
		t.Fatalf("unexpected discovery: %v", result)
	}
	data, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Contributions []struct {
			Type     string
			External bool `json:"external_tools"`
			Tools    map[string]struct{ Access, Toolset string }
		} `json:"contributions"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	declarations := map[string]struct{ Access, Toolset string }{}
	for _, contribution := range manifest.Contributions {
		if contribution.Type == "mcp" && contribution.External {
			declarations = contribution.Tools
		}
	}
	if len(declarations) != len(tools) {
		t.Fatalf("manifest declares %d of %d tools", len(declarations), len(tools))
	}
	for _, raw := range tools {
		tool := raw.(map[string]any)
		declaration := declarations[tool["name"].(string)]
		if declaration.Access != "read" || declaration.Toolset == "" || tool["annotations"].(map[string]any)["readOnlyHint"] != true {
			t.Fatalf("unsafe/undeclared tool: %v", tool)
		}
	}
}

func TestMCPCallsAllReadOperationsWithHostLifecycle(t *testing.T) {
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		user, password, ok := r.BasicAuth()
		if !ok || user != "demo" || password != "fixture-secret" || r.Method != "GET" {
			t.Errorf("wrong host authentication/method")
		}
		if r.URL.Path == "/prom/api/v1/targets" && r.URL.Query().Get("scrapePool") != "" {
			fmt.Fprint(w, `{"status":"success","data":{"activeTargets":[],"droppedTargets":[]}}`)
			return
		}
		fmt.Fprint(w, `{"status":"success","data":{"value":"fixture-secret","password":"different-secret","healthy":true}}`)
	}))
	defer server.Close()
	lifecycle := testConnection(t, server, map[string]any{"username": "demo", "password": "fixture-secret", "read_only": true})
	p := newPlugin()
	tests := []struct {
		name, path string
		args       map[string]any
	}{
		{"prometheus_info", "/status/buildinfo", nil},
		{"prometheus_targets", "/targets", nil},
		{"prometheus_alerts", "/alerts", nil},
		{"prometheus_rules", "/rules", nil},
		{"prometheus_status_runtime", "/status/runtimeinfo", nil},
		{"prometheus_status_tsdb", "/status/tsdb", nil},
		{"prometheus_status_flags", "/status/flags", nil},
		{"prometheus_status_config", "/status/config", nil},
		{"prometheus_service_discovery_services", "/scrape_pools", nil},
		{"prometheus_service_discovery", "/targets", map[string]any{"scrapePool": "node", "state": "active", "page": 1, "pageSize": 20}},
		{"prometheus_metric_names", "/label/__name__/values", nil},
		{"prometheus_label_names", "/labels", map[string]any{"metricName": "up"}},
		{"prometheus_label_values", "/label/job/values", map[string]any{"labelName": "job", "metricName": "up"}},
		{"prometheus_query", "/query", map[string]any{"query": "up", "time": 1600000000}},
		{"prometheus_query_range", "/query_range", map[string]any{"query": "up", "start": 1600000000, "end": 1600000300, "step": 30}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.args
			if args == nil {
				args = map[string]any{}
			}
			args["connectionId"] = "demo"
			result := mcpInvoke(t, p, "mcp/call", map[string]any{"tool": tt.name, "arguments": args, "lifecycle": lifecycle})
			data, _ := json.Marshal(result)
			if result["isError"] != false || !strings.HasSuffix(paths[len(paths)-1], tt.path) || strings.Contains(string(data), "fixture-secret") || strings.Contains(string(data), "different-secret") {
				t.Fatalf("bad route/result: %s %s", paths[len(paths)-1], data)
			}
		})
	}
	if len(paths) != 15 || len(p.sessions) != 0 {
		t.Fatalf("unexpected session/login effects: %v", paths)
	}
}

func TestMCPRejectsMalformedUnknownAndCrossConnectionArguments(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `{"status":"success","data":{}}`)
	}))
	defer server.Close()
	lifecycle := testConnection(t, server, nil)
	p := newPlugin()
	for _, args := range []any{map[string]any{"connectionId": "other"}, map[string]any{"query": "up", "password": "model-secret"}, map[string]any{"query": "up", "time": "bad"}, map[string]any{}, []any{}} {
		result := mcpInvoke(t, p, "mcp/call", map[string]any{"tool": "prometheus_query", "arguments": args, "lifecycle": lifecycle})
		if result["isError"] != true {
			t.Fatalf("accepted %v", args)
		}
	}
	for _, input := range []any{map[string]any{"tool": "prometheus_delete_series", "lifecycle": lifecycle}, map[string]any{"tool": "prometheus_alerts", "arguments": map[string]any{}}} {
		if result := mcpInvoke(t, p, "mcp/call", input); result["isError"] != true {
			t.Fatalf("accepted %v", input)
		}
	}
	if requests != 0 {
		t.Fatalf("bad input reached service %d times", requests)
	}
}

func TestMCPRedactsConfigurationAndErrorSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/query") {
			fmt.Fprint(w, `{"status":"error","errorType":"bad_data","error":"query failed fixture-secret"}`)
			return
		}
		fmt.Fprint(w, `{"status":"success","data":{"yaml":"scrape_configs:\n  - job_name: node\n    authorization:\n      type: Bearer\n      credentials: other-token\n    basic_auth:\n      username: demo\n      password: hidden-password\n    bearer_token: another-token\n"}}`)
	}))
	defer server.Close()
	lifecycle := testConnection(t, server, map[string]any{"username": "demo", "password": "fixture-secret"})
	for _, tool := range []string{"prometheus_status_config", "prometheus_query"} {
		args := map[string]any{}
		if tool == "prometheus_query" {
			args["query"] = "up"
		}
		result := mcpInvoke(t, newPlugin(), "mcp/call", map[string]any{"tool": tool, "arguments": args, "lifecycle": lifecycle})
		data, _ := json.Marshal(result)
		if tool == "prometheus_status_config" && !strings.Contains(string(data), "job_name") {
			t.Fatalf("nonsecret configuration disappeared: %s", data)
		}
		for _, secret := range []string{"other-token", "hidden-password", "another-token", "fixture-secret"} {
			if strings.Contains(string(data), secret) {
				t.Fatalf("secret in result: %s", data)
			}
		}
		if (tool == "prometheus_query") != (result["isError"] == true) {
			t.Fatalf("error envelope: %v", result)
		}
	}
}

func TestMCPDoesNotLeakSecretsInURLFieldsOrLargeResults(t *testing.T) {
	for _, data := range []string{
		`{"status":"success","data":{"scrapeUrl":"https://demo:url-secret@metrics.example/metrics?token=query-secret&job=node"}}`,
		`{"status":"success","data":{"message":"` + strings.Repeat("x", 1<<20) + `"}}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, data) }))
		lifecycle := testConnection(t, server, nil)
		result := mcpInvoke(t, newPlugin(), "mcp/call", map[string]any{"tool": "prometheus_targets", "lifecycle": lifecycle})
		encoded, _ := json.Marshal(result)
		if strings.Contains(string(encoded), "url-secret") || strings.Contains(string(encoded), "query-secret") {
			t.Fatalf("URL credential leaked: %s", encoded)
		}
		if len(data) > 1<<20 && result["isError"] != true {
			t.Fatal("oversized result accepted")
		}
		server.Close()
	}
}

func TestMCPExplicitNullAndZeroPaginationAreNotDefaults(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `{"status":"success","data":{"activeTargets":[],"droppedTargets":[]}}`)
	}))
	defer server.Close()
	lifecycle := testConnection(t, server, nil)
	for _, args := range []map[string]any{
		{"scrapePool": "node", "page": 0},
		{"scrapePool": "node", "pageSize": 0},
		{"scrapePool": "node", "state": nil},
		{"scrapePool": "node", "connectionId": nil},
	} {
		result := mcpInvoke(t, newPlugin(), "mcp/call", map[string]any{"tool": "prometheus_service_discovery", "arguments": args, "lifecycle": lifecycle})
		if result["isError"] != true {
			t.Fatalf("invalid explicit parameter accepted: %v", args)
		}
	}
	if requests != 0 {
		t.Fatalf("invalid input performed %d service calls", requests)
	}
}
