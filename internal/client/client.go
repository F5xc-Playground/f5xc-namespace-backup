// internal/client/client.go
package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
)

// Client is an F5 XC API client.
type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
	sem        chan struct{}
	mu         sync.Mutex
	certErr    error
}

type Option func(*Client)

func WithToken(token string) Option {
	return func(c *Client) { c.token = token }
}

func WithCert(certFile, keyFile string) Option {
	return func(c *Client) {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			slog.Error("failed to load client certificate", "error", err, "cert", certFile, "key", keyFile)
			c.certErr = err
			return
		}
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
			},
		}
		c.httpClient = &http.Client{Transport: transport}
	}
}

func WithParallel(n int) Option {
	return func(c *Client) { c.sem = make(chan struct{}, n) }
}

func New(tenantURL string, opts ...Option) *Client {
	c := &Client{
		baseURL:    NormalizeTenantURL(tenantURL),
		httpClient: &http.Client{},
		sem:        make(chan struct{}, 10),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

// NewForTest creates a client for testing with a custom HTTP client.
func NewForTest(baseURL string, httpClient *http.Client, token string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
		token:      token,
		sem:        make(chan struct{}, 10),
	}
}

func (c *Client) do(method, path string, body io.Reader) ([]byte, int, error) {
	if c.certErr != nil {
		return nil, 0, fmt.Errorf("client certificate error: %w", c.certErr)
	}

	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	url := c.baseURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "APIToken "+c.token)
	}

	slog.Debug("API request", "method", method, "path", path)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return data, resp.StatusCode, fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	return data, resp.StatusCode, nil
}

func (c *Client) List(path string) ([]map[string]any, error) {
	data, _, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decoding list response: %w", err)
	}
	return resp.Items, nil
}

func (c *Client) Get(path string) (map[string]any, error) {
	data, _, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("decoding get response: %w", err)
	}
	return obj, nil
}

func (c *Client) Create(path string, obj map[string]any) error {
	body, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("encoding object: %w", err)
	}
	_, _, err = c.do("POST", path, bytes.NewReader(body))
	return err
}

func (c *Client) Replace(path string, obj map[string]any) error {
	body, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("encoding object: %w", err)
	}
	_, _, err = c.do("PUT", path, bytes.NewReader(body))
	return err
}
