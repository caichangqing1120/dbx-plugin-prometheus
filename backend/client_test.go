package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	sdk "github.com/t8y2/dbx/plugins/sdk/go/dbx-plugin-sdk"
)

func invoke(p *plugin, method string, params any) (any, *sdk.PluginError) {
	raw, _ := json.Marshal(params)
	return p.Handle(sdk.RequestContext{}, method, raw, nil)
}

func testConnection(t *testing.T, server *httptest.Server, extra map[string]any) map[string]any {
	t.Helper()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	c := map[string]any{"id": "demo", "name": "测试", "host": u.Hostname(), "port": port, "external_config": map[string]any{"scheme": "http", "context_path": "/prom"}}
	for key, value := range extra {
		c[key] = value
	}
	return map[string]any{"connection": c}
}

func TestReadRoutesAndBasicAuth(t *testing.T) {
	seen := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		user, password, ok := r.BasicAuth()
		if !ok || user != "demo" || password != "fixture-secret" || r.Method != "GET" {
			t.Errorf("unexpected request: %s %v", r.Method, r.Header)
		}
		if r.URL.Path == "/prom/api/v1/query" && r.URL.Query().Get("query") != `up{job="node"}` {
			t.Errorf("query not encoded: %s", r.URL.RawQuery)
		}
		if r.URL.Path == "/prom/api/v1/targets" && r.URL.Query().Get("state") != "active" {
			t.Error("missing active filter")
		}
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	}))
	defer server.Close()
	p := newPlugin()
	connection := testConnection(t, server, map[string]any{"username": "demo", "password": "fixture-secret"})
	if _, err := invoke(p, "connection/connect", connection); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"prometheus/info", "prometheus/query", "prometheus/targets", "prometheus/alerts", "prometheus/rules"} {
		result, err := invoke(p, method, map[string]any{"connectionId": "demo", "form": map[string]any{"query": `up{job="node"}`}})
		if err != nil || result == nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	if len(seen) != 6 {
		t.Fatal(seen)
	}
	if _, err := invoke(p, "prometheus/raw", map[string]any{"connectionId": "demo"}); err == nil {
		t.Fatal("arbitrary endpoint allowed")
	}
	if len(seen) != 6 {
		t.Fatal("unexpected request", seen)
	}
	invoke(p, "connection/disconnect", connection)
	if _, err := invoke(p, "prometheus/alerts", map[string]any{"connectionId": "demo"}); err == nil {
		t.Fatal("disconnected session usable")
	}
}

func TestRangeBoundsAndEncoding(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/prom/api/v1/query_range" && (r.URL.Query().Get("step") != "30" || r.URL.Query().Get("query") != "rate(up[5m])") {
			t.Errorf("bad range: %s", r.URL)
		}
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[]}}`)
	}))
	defer server.Close()
	p := newPlugin()
	invoke(p, "connection/connect", testConnection(t, server, nil))
	end := time.Now().Unix()
	for _, form := range []map[string]any{
		{"query": "", "start": end - 3600, "end": end, "step": 30},
		{"query": "up", "start": end - 3600, "end": end, "step": 0},
		{"query": "up", "start": end, "end": end, "step": 30},
		{"query": "up", "start": 1, "end": end, "step": 1},
		{"query": "up", "start": end - 60, "end": end + 3600, "step": 30},
	} {
		if _, err := invoke(p, "prometheus/query_range", map[string]any{"connectionId": "demo", "form": form}); err == nil {
			t.Fatal("invalid range accepted", form)
		}
	}
	if requests != 1 {
		t.Fatalf("invalid queries reached API: %d", requests)
	}
	if _, err := invoke(p, "prometheus/query_range", map[string]any{"connectionId": "demo", "form": map[string]any{"query": "rate(up[5m])", "start": end - 3600, "end": end, "step": 30}}); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatal(requests)
	}
}

func TestInstantQueryEvaluationTime(t *testing.T) {
	evaluation := float64(time.Now().Add(-5 * time.Minute).Unix())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/prom/api/v1/query" && r.URL.Query().Get("time") != strconv.FormatFloat(evaluation, 'f', -1, 64) {
			t.Errorf("missing evaluation time: %s", r.URL)
		}
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	}))
	defer server.Close()
	p := newPlugin()
	invoke(p, "connection/connect", testConnection(t, server, nil))
	if _, err := invoke(p, "prometheus/query", map[string]any{"connectionId": "demo", "form": map[string]any{"query": "up", "time": evaluation}}); err != nil {
		t.Fatal(err)
	}
}

func TestCompletionRoutesAndValidation(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/prom/api/v1/label/__name__/values":
			fmt.Fprint(w, `{"status":"success","data":["up","node_cpu_seconds_total"]}`)
		case "/prom/api/v1/labels":
			if r.URL.Query().Get("match[]") != "node_cpu_seconds_total" {
				t.Errorf("missing metric matcher: %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"status":"success","data":["instance","job","mode"]}`)
		case "/prom/api/v1/label/mode/values":
			if r.URL.Query().Get("match[]") != "node_cpu_seconds_total" {
				t.Errorf("missing label value matcher: %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"status":"success","data":["idle","system","user"]}`)
		default:
			fmt.Fprint(w, `{"status":"success","data":{}}`)
		}
	}))
	defer server.Close()
	p := newPlugin()
	if _, err := invoke(p, "connection/connect", testConnection(t, server, nil)); err != nil {
		t.Fatal(err)
	}
	for _, call := range []struct {
		method string
		form   map[string]any
	}{
		{"prometheus/metric_names", nil},
		{"prometheus/label_names", map[string]any{"metricName": "node_cpu_seconds_total"}},
		{"prometheus/label_values", map[string]any{"metricName": "node_cpu_seconds_total", "labelName": "mode"}},
	} {
		if _, err := invoke(p, call.method, map[string]any{"connectionId": "demo", "form": call.form}); err != nil {
			t.Fatalf("%s: %v", call.method, err)
		}
	}
	if requests != 4 {
		t.Fatalf("unexpected request count: %d", requests)
	}
	for _, call := range []struct {
		method string
		form   map[string]any
	}{
		{"prometheus/label_names", map[string]any{"metricName": "bad metric"}},
		{"prometheus/label_values", map[string]any{"labelName": "../secret"}},
	} {
		if _, err := invoke(p, call.method, map[string]any{"connectionId": "demo", "form": call.form}); err == nil {
			t.Fatalf("invalid completion input accepted: %#v", call)
		}
	}
	if requests != 4 {
		t.Fatalf("invalid completion input reached API: %d", requests)
	}
}

func TestStatusRoutes(t *testing.T) {
	seen := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = r.URL.RawQuery
		fmt.Fprint(w, `{"status":"success","data":{}}`)
	}))
	defer server.Close()
	p := newPlugin()
	if _, err := invoke(p, "connection/connect", testConnection(t, server, nil)); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{
		"prometheus/status_runtime",
		"prometheus/status_tsdb",
		"prometheus/status_flags",
		"prometheus/status_config",
		"prometheus/service_discovery_services",
	} {
		if _, err := invoke(p, method, map[string]any{"connectionId": "demo"}); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	for _, path := range []string{
		"/prom/api/v1/status/runtimeinfo",
		"/prom/api/v1/status/tsdb",
		"/prom/api/v1/status/flags",
		"/prom/api/v1/status/config",
		"/prom/api/v1/scrape_pools",
	} {
		if _, ok := seen[path]; !ok {
			t.Fatalf("status route not requested: %s", path)
		}
	}
	if seen["/prom/api/v1/status/tsdb"] != "limit=10" {
		t.Fatalf("unexpected TSDB query: %s", seen["/prom/api/v1/status/tsdb"])
	}
}

func TestRejectBadConnectionAndRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com", http.StatusFound)
	}))
	defer server.Close()
	p := newPlugin()
	for _, path := range []string{"//evil", "/../private", "/%2e%2e", "/bad?key=1"} {
		c := testConnection(t, server, nil)
		c["connection"].(map[string]any)["external_config"] = map[string]any{"context_path": path}
		if _, err := invoke(p, "connection/test", c); err == nil {
			t.Fatal("invalid path accepted", path)
		}
	}
	c := testConnection(t, server, nil)
	if _, err := invoke(p, "connection/test", c); err == nil || !strings.Contains(err.Message, "HTTP 302") {
		t.Fatalf("redirect accepted: %v", err)
	}
	c["connection"].(map[string]any)["transport_layers"] = []map[string]any{{"type": "ssh", "enabled": true}}
	if _, err := invoke(p, "connection/test", c); err == nil || !strings.Contains(err.Message, "隧道端点") {
		t.Fatalf("bypassed missing tunnel: %v", err)
	}
}

func TestTunnelUsesRuntimeEndpointWithoutChangingHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "prometheus.internal:9090" {
			t.Errorf("Host header changed: %s", r.Host)
		}
		fmt.Fprint(w, `{"status":"success","data":{}}`)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	p := newPlugin()
	c := map[string]any{"id": "demo", "host": "prometheus.internal", "port": 9090, "transport_layers": []map[string]any{{"type": "ssh", "enabled": true}}}
	input := map[string]any{"connection": c, "runtime": map[string]any{"host": u.Hostname(), "port": port}}
	if _, err := invoke(p, "connection/connect", input); err != nil {
		t.Fatal(err)
	}
	if _, err := invoke(p, "prometheus/alerts", map[string]any{"connectionId": "demo"}); err != nil {
		t.Fatal(err)
	}
}

func TestAPIErrorAndBoundedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/prom/api/v1/targets" {
			fmt.Fprint(w, `{"status":"error","errorType":"bad_data","error":"invalid target"}`)
			return
		}
		if r.URL.Path == "/prom/api/v1/rules" {
			fmt.Fprint(w, strings.Repeat("x", 4<<20+1))
			return
		}
		fmt.Fprint(w, `{"status":"success","data":{}}`)
	}))
	defer server.Close()
	p := newPlugin()
	if _, err := invoke(p, "connection/connect", testConnection(t, server, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := invoke(p, "prometheus/targets", map[string]any{"connectionId": "demo"}); err == nil || !strings.Contains(err.Message, "invalid target") {
		t.Fatalf("API error lost: %v", err)
	}
	if _, err := invoke(p, "prometheus/rules", map[string]any{"connectionId": "demo"}); err == nil || !strings.Contains(err.Message, "4 MiB") {
		t.Fatalf("oversize accepted: %v", err)
	}
}

func TestServiceDiscoveryStreamsSelectedServicePage(t *testing.T) {
	largeAnnotation := strings.Repeat("x", 40<<10)
	targets := make([]map[string]any, 0, 125)
	for i := 0; i < 125; i++ {
		targets = append(targets, map[string]any{
			"scrapePool": "kubernetes-pods",
			"scrapeUrl":  fmt.Sprintf("http://172.16.1.%d:9253/metrics", i+1),
			"health":     "up",
			"labels": map[string]string{
				"job":                  "kubernetes-pods",
				"instance":             fmt.Sprintf("172.16.1.%d:9253", i+1),
				"kubernetes_namespace": "kube-system",
			},
			"discoveredLabels": map[string]string{
				"__address__":                                    fmt.Sprintf("172.16.1.%d:9253", i+1),
				"__meta_kubernetes_namespace":                    "kube-system",
				"__meta_kubernetes_pod_name":                     fmt.Sprintf("nodelocaldns-%03d", i+1),
				"__meta_kubernetes_pod_annotation_large_payload": largeAnnotation,
			},
		})
	}
	payload, err := json.Marshal(map[string]any{
		"status": "success",
		"data":   map[string]any{"activeTargets": targets, "droppedTargets": []any{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) <= 4<<20 {
		t.Fatalf("fixture must exceed the generic response limit, got %d bytes", len(payload))
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prom/api/v1/scrape_pools":
			fmt.Fprint(w, `{"status":"success","data":{"scrapePools":["kubernetes-pods","node"]}}`)
		case "/prom/api/v1/targets":
			if r.URL.Query().Get("scrapePool") != "kubernetes-pods" || r.URL.Query().Get("state") != "active" {
				t.Errorf("unexpected discovery query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write(payload)
		default:
			fmt.Fprint(w, `{"status":"success","data":{}}`)
		}
	}))
	defer server.Close()

	p := newPlugin()
	if _, pluginErr := invoke(p, "connection/connect", testConnection(t, server, nil)); pluginErr != nil {
		t.Fatal(pluginErr)
	}
	servicesResult, pluginErr := invoke(p, "prometheus/service_discovery_services", map[string]any{"connectionId": "demo"})
	if pluginErr != nil {
		t.Fatal(pluginErr)
	}
	services := servicesResult.(map[string]any)["data"].(map[string]any)["scrapePools"].([]any)
	if len(services) != 2 || services[0] != "kubernetes-pods" {
		t.Fatalf("unexpected scrape pools: %#v", services)
	}

	result, pluginErr := invoke(p, "prometheus/service_discovery", map[string]any{
		"connectionId": "demo",
		"form":         map[string]any{"scrapePools": []string{"kubernetes-pods"}, "state": "active", "page": 2, "pageSize": 20},
	})
	if pluginErr != nil {
		t.Fatalf("oversized discovery response should be streamed: %v", pluginErr)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 512<<10 {
		t.Fatalf("paged discovery response exceeds bridge budget: %d bytes", len(encoded))
	}
	envelope := result.(map[string]any)
	data := envelope["data"].(map[string]any)
	items := data["items"].([]map[string]any)
	if len(items) != 20 || data["total"] != 125 || data["page"] != 2 || data["pageSize"] != 20 || data["hasNext"] != true {
		t.Fatalf("unexpected page: %#v", data)
	}
	if items[0]["scrapeUrl"] != "http://172.16.1.21:9253/metrics" {
		t.Fatalf("wrong page start: %#v", items[0])
	}
	firstLabels := items[0]["discoveredLabels"].(map[string]string)
	if firstLabels["__meta_kubernetes_namespace"] != "kube-system" {
		t.Fatalf("searchable discovery label was lost: %#v", firstLabels)
	}
	if _, exists := firstLabels["__meta_kubernetes_pod_annotation_large_payload"]; exists {
		t.Fatal("oversized annotation should not cross the plugin bridge")
	}
	selected := data["scrapePools"].([]string)
	if len(selected) != 1 || selected[0] != "kubernetes-pods" || data["state"] != "active" {
		t.Fatalf("selection metadata missing: %#v", data)
	}
}

func TestServiceDiscoveryMergesSelectedServicesBeforePaging(t *testing.T) {
	makeTargets := func(scrapePool string, count int) []map[string]any {
		targets := make([]map[string]any, 0, count)
		for i := 0; i < count; i++ {
			targets = append(targets, map[string]any{
				"scrapePool": scrapePool,
				"scrapeUrl":  fmt.Sprintf("http://%s-%02d.example/metrics", scrapePool, i+1),
				"health":     "up",
				"labels":     map[string]string{"job": scrapePool, "instance": fmt.Sprintf("%s-%02d", scrapePool, i+1)},
			})
		}
		return targets
	}
	payloads := map[string][]byte{}
	for scrapePool, targets := range map[string][]map[string]any{
		"node":       makeTargets("node", 25),
		"prometheus": makeTargets("prometheus", 22),
	} {
		payload, err := json.Marshal(map[string]any{"status": "success", "data": map[string]any{"activeTargets": targets, "droppedTargets": []any{}}})
		if err != nil {
			t.Fatal(err)
		}
		payloads[scrapePool] = payload
	}
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prom/api/v1/targets" {
			fmt.Fprint(w, `{"status":"success","data":{}}`)
			return
		}
		scrapePool := r.URL.Query().Get("scrapePool")
		requests = append(requests, scrapePool)
		_, _ = w.Write(payloads[scrapePool])
	}))
	defer server.Close()

	p := newPlugin()
	if _, err := invoke(p, "connection/connect", testConnection(t, server, nil)); err != nil {
		t.Fatal(err)
	}
	result, pluginErr := invoke(p, "prometheus/service_discovery", map[string]any{
		"connectionId": "demo",
		"form": map[string]any{
			"scrapePools": []string{"node", "prometheus"}, "state": "active", "page": 2, "pageSize": 20,
		},
	})
	if pluginErr != nil {
		t.Fatal(pluginErr)
	}
	data := result.(map[string]any)["data"].(map[string]any)
	items := data["items"].([]map[string]any)
	if len(items) != 20 || data["total"] != 47 || data["hasNext"] != true {
		t.Fatalf("unexpected merged page: %#v", data)
	}
	if items[0]["scrapeUrl"] != "http://node-21.example/metrics" || items[19]["scrapeUrl"] != "http://prometheus-15.example/metrics" {
		t.Fatalf("merged page order is wrong: first=%#v last=%#v", items[0], items[19])
	}
	if !reflect.DeepEqual(requests, []string{"node", "prometheus"}) {
		t.Fatalf("unexpected service requests: %#v", requests)
	}
}

func TestServiceDiscoveryBackfillsSelectedServiceForDroppedTargets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prom/api/v1/targets" {
			fmt.Fprint(w, `{"status":"success","data":{}}`)
			return
		}
		fmt.Fprint(w, `{"status":"success","data":{"activeTargets":[],"droppedTargets":[{"discoveredLabels":{"__address__":"example:9090"}}]}}`)
	}))
	defer server.Close()

	p := newPlugin()
	if _, err := invoke(p, "connection/connect", testConnection(t, server, nil)); err != nil {
		t.Fatal(err)
	}
	result, pluginErr := invoke(p, "prometheus/service_discovery", map[string]any{
		"connectionId": "demo",
		"form": map[string]any{
			"scrapePools": []string{"kubernetes-pods"}, "state": "dropped", "page": 1, "pageSize": 20,
		},
	})
	if pluginErr != nil {
		t.Fatal(pluginErr)
	}
	items := result.(map[string]any)["data"].(map[string]any)["items"].([]map[string]any)
	if len(items) != 1 || items[0]["scrapePool"] != "kubernetes-pods" {
		t.Fatalf("dropped target service was not restored: %#v", items)
	}
}

func TestServiceDiscoveryRejectsInvalidPagingBeforeRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `{"status":"success","data":{}}`)
	}))
	defer server.Close()
	p := newPlugin()
	if _, err := invoke(p, "connection/connect", testConnection(t, server, nil)); err != nil {
		t.Fatal(err)
	}
	for _, form := range []map[string]any{
		{"scrapePools": []string{}, "state": "active", "page": 1, "pageSize": 20},
		{"scrapePools": []string{"node"}, "state": "any", "page": 1, "pageSize": 20},
		{"scrapePools": []string{"node"}, "state": "active", "page": 0, "pageSize": 20},
		{"scrapePools": []string{"node"}, "state": "active", "page": 1, "pageSize": 25},
		{"scrapePools": append([]string{}, make([]string, 21)...), "state": "active", "page": 1, "pageSize": 20},
	} {
		if _, err := invoke(p, "prometheus/service_discovery", map[string]any{"connectionId": "demo", "form": form}); err == nil {
			t.Fatalf("invalid discovery form accepted: %#v", form)
		}
	}
	if requests != 1 {
		t.Fatalf("invalid forms reached API: %d requests", requests-1)
	}
}
