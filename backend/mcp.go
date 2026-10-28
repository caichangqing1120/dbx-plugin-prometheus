package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations"`
}

func mcpReadTool(name, description string, properties map[string]any, required ...string) mcpTool {
	if properties == nil {
		properties = map[string]any{}
	}
	properties["connectionId"] = map[string]any{"type": "string", "description": "Host-bound connection ID; cannot select another connection."}
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return mcpTool{
		Name: "prometheus_" + name, Description: description,
		InputSchema: schema,
		Annotations: map[string]any{"readOnlyHint": true, "destructiveHint": false, "idempotentHint": true, "openWorldHint": true},
	}
}

func mcpTools() []mcpTool {
	text := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	number := func(description string) map[string]any {
		return map[string]any{"type": "number", "description": description, "minimum": 0}
	}
	query := text("PromQL expression, at most 4096 characters.")
	query["maxLength"] = 4096
	return []mcpTool{
		mcpReadTool("info", "Read Prometheus connection and build information.", nil),
		mcpReadTool("targets", "Read active Prometheus scrape targets.", nil),
		mcpReadTool("alerts", "Read current Prometheus alerts.", nil),
		mcpReadTool("rules", "Read recording and alerting rules.", nil),
		mcpReadTool("status_runtime", "Read Prometheus runtime status.", nil),
		mcpReadTool("status_tsdb", "Read bounded TSDB cardinality statistics.", nil),
		mcpReadTool("status_flags", "Read Prometheus flags with credentials redacted.", nil),
		mcpReadTool("status_config", "Read configuration with authentication blocks redacted; not a backup export.", nil),
		mcpReadTool("service_discovery_services", "List configured scrape pools.", nil),
		mcpReadTool("service_discovery", "Read a bounded page of active or dropped discovery targets from selected scrape pools.", map[string]any{
			"scrapePool":  text("One scrape pool; use scrapePools for multiple pools."),
			"scrapePools": map[string]any{"type": "array", "items": text("Scrape pool name"), "minItems": 1, "maxItems": 20},
			"state":       map[string]any{"type": "string", "enum": []string{"active", "dropped"}, "description": "Defaults to active."},
			"page":        map[string]any{"type": "integer", "minimum": 1, "maximum": 100000, "description": "Defaults to 1."},
			"pageSize":    map[string]any{"type": "integer", "minimum": 20, "maximum": 100, "description": "20, 50 or 100; defaults to 20."},
		}),
		mcpReadTool("metric_names", "List metric names for query completion.", nil),
		mcpReadTool("label_names", "List label names, optionally filtered by metric.", map[string]any{"metricName": text("Optional metric name.")}),
		mcpReadTool("label_values", "List values of one label, optionally filtered by metric.", map[string]any{"metricName": text("Optional metric name."), "labelName": text("Label name.")}, "labelName"),
		mcpReadTool("query", "Execute a read-only instant PromQL query.", map[string]any{"query": query, "time": number("Optional evaluation time, Unix seconds.")}, "query"),
		mcpReadTool("query_range", "Execute a read-only range PromQL query, up to 11000 points with a step of at least one second.", map[string]any{"query": query, "start": number("Range start, Unix seconds."), "end": number("Range end, Unix seconds."), "step": number("Step in seconds, at least 1.")}, "query", "start", "end", "step"),
	}
}

func decodeMCP(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("Invalid MCP parameters")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("Invalid trailing MCP parameters")
	}
	return nil
}

var mcpSecretField = regexp.MustCompile(`(?i)(password|passwd|token|secret|authorization|credential|cookie|api[_-]?key|private[_-]?key|http_headers|basic_auth|oauth2)`)

func redactMCP(value any, secrets []string) any {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if mcpSecretField.MatchString(key) {
				value[key] = "[REDACTED]"
			} else if strings.EqualFold(key, "yaml") {
				text, ok := child.(string)
				var config map[string]any
				if !ok || yaml.Unmarshal([]byte(text), &config) != nil {
					value[key] = "[REDACTED: unparseable configuration]"
				} else if encoded, err := yaml.Marshal(redactMCP(config, secrets)); err != nil {
					value[key] = "[REDACTED: unsupported configuration]"
				} else {
					value[key] = string(encoded)
				}
			} else {
				value[key] = redactMCP(child, secrets)
			}
		}
	case []any:
		for i, child := range value {
			value[i] = redactMCP(child, secrets)
		}
	case map[any]any:
		normalized := map[string]any{}
		for key, child := range value {
			name, ok := key.(string)
			if !ok {
				return "[REDACTED: nonstring configuration key]"
			}
			normalized[name] = child
		}
		return redactMCP(normalized, secrets)
	case string:
		for _, secret := range secrets {
			if secret != "" {
				value = strings.ReplaceAll(value, secret, "[REDACTED]")
			}
		}
		if parsed, err := url.Parse(value); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
			parsed.User = nil
			query := parsed.Query()
			for key := range query {
				if mcpSecretField.MatchString(key) {
					query.Set(key, "[REDACTED]")
				}
			}
			parsed.RawQuery = query.Encode()
			parsed.Fragment, parsed.RawFragment = "", ""
			value = parsed.String()
		}
		return value
	}
	return value
}

func mcpResult(value any, problem error, secrets []string) map[string]any {
	if problem != nil {
		value = problem.Error()
	}
	encoded, err := json.Marshal(value)
	if err == nil {
		var decoded any
		err = json.Unmarshal(encoded, &decoded)
		if err == nil {
			encoded, err = json.Marshal(redactMCP(decoded, secrets))
		}
	}
	if err != nil || len(encoded) > 1<<20 {
		problem = errors.New("MCP result exceeds the 1 MiB limit or cannot be encoded; narrow the request")
		encoded, _ = json.Marshal(problem.Error())
	}
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": string(encoded)}}, "isError": problem != nil}
}

func (p *plugin) handleMCP(method string, raw json.RawMessage) any {
	if method == "mcp/tools" {
		return map[string]any{"tools": mcpTools()}
	}
	if len(raw) > 1<<20 {
		return mcpResult(nil, errors.New("MCP request exceeds 1 MiB"), nil)
	}
	var call struct {
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
		Lifecycle json.RawMessage `json:"lifecycle"`
	}
	if err := decodeMCP(raw, &call); err != nil {
		return mcpResult(nil, err, nil)
	}
	var tool *mcpTool
	for _, candidate := range mcpTools() {
		if candidate.Name == call.Tool {
			tool = &candidate
			break
		}
	}
	if tool == nil {
		return mcpResult(nil, errors.New("Unknown MCP tool"), nil)
	}
	if len(call.Arguments) == 0 {
		call.Arguments = json.RawMessage(`{}`)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(call.Arguments, &fields) != nil || fields == nil {
		return mcpResult(nil, errors.New("Tool arguments must be an object"), nil)
	}
	properties := tool.InputSchema["properties"].(map[string]any)
	for key, raw := range fields {
		if _, ok := properties[key]; !ok {
			return mcpResult(nil, errors.New("Unknown tool argument"), nil)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return mcpResult(nil, errors.New("Tool arguments cannot be null"), nil)
		}
		field := properties[key].(map[string]any)
		if minimum, ok := field["minimum"]; ok {
			var number float64
			if json.Unmarshal(raw, &number) != nil {
				return mcpResult(nil, errors.New("Invalid numeric tool argument"), nil)
			}
			min, _ := json.Marshal(minimum)
			var lower float64
			json.Unmarshal(min, &lower)
			if number < lower {
				return mcpResult(nil, errors.New("Tool argument below its minimum"), nil)
			}
		}
	}
	if required, ok := tool.InputSchema["required"].([]string); ok {
		for _, name := range required {
			if _, exists := fields[name]; !exists {
				return mcpResult(nil, errors.New("Missing required tool argument: "+name), nil)
			}
		}
	}
	var args struct {
		ConnectionID string `json:"connectionId"`
		queryForm
	}
	if err := decodeMCP(call.Arguments, &args); err != nil {
		return mcpResult(nil, err, nil)
	}
	var lifecycle request
	if json.Unmarshal(call.Lifecycle, &lifecycle) != nil || strings.TrimSpace(lifecycle.Connection.ID) == "" || (args.ConnectionID != "" && args.ConnectionID != lifecycle.Connection.ID) {
		return mcpResult(nil, errors.New("Missing or mismatched host-bound connection"), nil)
	}
	secrets := []string{lifecycle.Connection.Password}
	if lifecycle.Connection.Password != "" {
		secrets = append(secrets, base64.StdEncoding.EncodeToString([]byte(lifecycle.Connection.Username+":"+lifecycle.Connection.Password)))
	}
	s, err := newSession(lifecycle.Connection, lifecycle.Runtime)
	if err != nil {
		return mcpResult(nil, err, secrets)
	}
	// Request-local transport honors the supplied lifecycle without touching UI sessions.
	defer s.close()
	form := args.queryForm
	if call.Tool == "prometheus_service_discovery" {
		if form.State == "" {
			form.State = "active"
		}
		if form.Page == 0 {
			form.Page = 1
		}
		if form.PageSize == 0 {
			form.PageSize = 20
		}
	}
	result, err := s.execute("prometheus/"+strings.TrimPrefix(call.Tool, "prometheus_"), form)
	return mcpResult(result, err, secrets)
}
