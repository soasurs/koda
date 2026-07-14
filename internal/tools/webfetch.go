package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/soasurs/adk/tool"
)

const (
	maxFetchBodyBytes = 4 << 20
	maxFetchRedirects = 10
)

var testAllowLoopback bool

type webFetchInput struct {
	URL      string `json:"url" jsonschema:"URL to fetch content from"`
	MaxChars int    `json:"max_chars,omitempty" jsonschema:"Maximum returned characters; defaults to 32768 and is capped"`
}

type webFetchOutput struct {
	URL         string `json:"url"`
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type,omitempty"`
	Content     string `json:"content"`
	Truncated   bool   `json:"truncated"`
}

func (s service) newWebFetchTool() (tool.Tool, error) {
	return tool.NewFunc(tool.Definition{
		Name: "web_fetch",
		Description: "Fetch content from a URL. Sends Accept: text/markdown to prefer " +
			"markdown-formatted responses when available.",
	}, s.webFetch)
}

func (s service) webFetch(ctx context.Context, input webFetchInput) (webFetchOutput, error) {
	if strings.TrimSpace(input.URL) == "" {
		return webFetchOutput{}, handled(fmt.Errorf("url must not be empty"))
	}
	u, err := url.Parse(input.URL)
	if err != nil {
		return webFetchOutput{}, handled(fmt.Errorf("invalid url: %w", err))
	}
	if err := validateFetchURL(u, testAllowLoopback); err != nil {
		return webFetchOutput{}, handled(err)
	}
	maxChars := clamp(input.MaxChars, defaultMaxChars, defaultMaxChars)

	client := &http.Client{
		Timeout: s.commandTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxFetchRedirects {
				return fmt.Errorf("too many redirects")
			}
			return validateFetchURL(req.URL, false)
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return webFetchOutput{}, handled(fmt.Errorf("create request: %w", err))
	}
	req.Header.Set("Accept", "text/markdown, text/html;q=0.9, text/plain;q=0.8")
	req.Header.Set("User-Agent", "Koda/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return webFetchOutput{}, handled(fmt.Errorf("fetch: %w", err))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBodyBytes))
	if err != nil {
		return webFetchOutput{}, handled(fmt.Errorf("read response: %w", err))
	}

	content := string(body)
	truncated := false
	if len([]rune(content)) > maxChars {
		runes := []rune(content)
		content = string(runes[:maxChars])
		truncated = true
	}
	if !truncated {
		extra := make([]byte, 1)
		if n, _ := resp.Body.Read(extra); n > 0 {
			truncated = true
		}
	}

	return webFetchOutput{
		URL:         input.URL,
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Content:     content,
		Truncated:   truncated,
	}, nil
}

func validateFetchURL(u *url.URL, allowLoopback bool) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported url scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("url must include a host")
	}
	ip := net.ParseIP(host)
	if ip != nil {
		if isRestrictedIP(ip, allowLoopback) {
			return fmt.Errorf("url resolves to a restricted address")
		}
		return nil
	}
	if isRestrictedHost(host, allowLoopback) {
		return fmt.Errorf("url resolves to a restricted address")
	}
	return nil
}

func isRestrictedIP(ip net.IP, allowLoopback bool) bool {
	if allowLoopback && ip.IsLoopback() {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func isRestrictedHost(host string, allowLoopback bool) bool {
	lower := strings.ToLower(host)
	if allowLoopback && lower == "localhost" {
		return false
	}
	return lower == "localhost" || strings.HasSuffix(lower, ".local")
}
