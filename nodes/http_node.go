package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"

	goflow "goflow"
)

// HTTPNodeConfig describes how to drive an HTTP request from shared state.
type HTTPNodeConfig struct {
	ID             string
	Method         string
	URL            string
	QueryParams    map[string]string
	Headers        map[string]string
	BodyTemplate   string
	Timeout        time.Duration
	Client         *http.Client
	ResponseKey    string
	StatusKey      string
	ResponseAsJSON bool
}

func DefaultHTTPNodeConfig(id string) HTTPNodeConfig {
	key := fmt.Sprintf("%s_response", id)
	return HTTPNodeConfig{
		ID:             id,
		Method:         http.MethodGet,
		QueryParams:    map[string]string{},
		Headers:        map[string]string{"Content-Type": "application/json"},
		Timeout:        30 * time.Second,
		ResponseKey:    key,
		StatusKey:      fmt.Sprintf("%s_status", id),
		ResponseAsJSON: true,
	}
}

// HTTPNode executes a request and writes the serialized response into shared state.
type HTTPNode struct {
	cfg      HTTPNodeConfig
	client   *http.Client
	urlTmpl  *template.Template
	bodyTmpl *template.Template
}

// NewHTTPNode builds an HTTPNode from config. The URL/body can be templated using Go templates.
func NewHTTPNode(cfg HTTPNodeConfig) (*HTTPNode, error) {
	if cfg.ID == "" {
		return nil, errors.New("http node requires id")
	}
	if cfg.URL == "" {
		return nil, errors.New("http node requires url")
	}
	if cfg.Method == "" {
		cfg.Method = http.MethodGet
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{}
	}

	node := &HTTPNode{cfg: cfg, client: cfg.Client}
	if strings.Contains(cfg.URL, "{{") {
		tmpl, err := template.New(cfg.ID + "-url").Parse(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("compile url template: %w", err)
		}
		node.urlTmpl = tmpl
	}
	if cfg.BodyTemplate != "" {
		tmpl, err := template.New(cfg.ID + "-body").Parse(cfg.BodyTemplate)
		if err != nil {
			return nil, fmt.Errorf("compile body template: %w", err)
		}
		node.bodyTmpl = tmpl
	}
	return node, nil
}

func (n *HTTPNode) Name() string {
	return n.cfg.ID
}

func (n *HTTPNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
	var cancel context.CancelFunc
	if n.cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, n.cfg.Timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	urlStr, err := n.renderURL(shared)
	if err != nil {
		return nil, err
	}

	bodyReader, err := n.renderBody(shared)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(n.cfg.Method), urlStr, bodyReader)
	if err != nil {
		return nil, err
	}
	for key, value := range n.cfg.Headers {
		req.Header.Set(key, value)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if n.cfg.StatusKey != "" {
		shared[n.cfg.StatusKey] = resp.StatusCode
	}

	stored := any(string(payload))
	if n.cfg.ResponseAsJSON {
		var parsed any
		if err := json.Unmarshal(payload, &parsed); err == nil {
			stored = parsed
		}
	}
	if n.cfg.ResponseKey != "" {
		shared[n.cfg.ResponseKey] = stored
	}

	return goflow.ResultWithAction(goflow.ActionNext), nil
}

func (n *HTTPNode) renderURL(shared map[string]any) (string, error) {
	target := n.cfg.URL
	if n.urlTmpl != nil {
		buf := &strings.Builder{}
		if err := n.urlTmpl.Execute(buf, shared); err != nil {
			return "", err
		}
		target = buf.String()
	}

	parsed, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	for key, value := range n.cfg.QueryParams {
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (n *HTTPNode) renderBody(shared map[string]any) (io.Reader, error) {
	if n.bodyTmpl == nil {
		return nil, nil
	}
	buf := &bytes.Buffer{}
	if err := n.bodyTmpl.Execute(buf, shared); err != nil {
		return nil, err
	}
	return bytes.NewReader(buf.Bytes()), nil
}

func init() {
	RegisterNode(NodeDefinition{
		ID:          "http",
		Description: "Executes an HTTP request and stores the response in shared state.",
		Example:     `nodes.NewHTTPNode(nodes.HTTPNodeConfig{ID: "notify", URL: "https://example.com/event", Method: http.MethodPost, BodyTemplate: "{\"text\": \"{{.message}}\"}"})`,
	})
}
