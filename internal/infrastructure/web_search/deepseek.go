package web_search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const (
	// defaultDeepseekSearchURL 是 DeepSeek 官方搜索端点，固定不对外配置以避免 SSRF。
	defaultDeepseekSearchURL = "https://api.deepseek.com/v1/tools/search"
	defaultDeepseekTimeout   = 30 * time.Second
	defaultDeepseekResults   = 10
	maxDeepseekResults       = 50
	maxDeepseekResponseBytes = 4 << 20
)

// DeepseekProvider 通过 DeepSeek 自研搜索引擎执行网页搜索。
// 接口按「成功请求次数」计费，不计 token，也不按返回条数计费。
type DeepseekProvider struct {
	client  *http.Client
	baseURL string
	apiKey  string
}

func NewDeepseekProvider(params types.WebSearchProviderParameters) (interfaces.WebSearchProvider, error) {
	if err := ValidateDeepseekParameters(params); err != nil {
		return nil, err
	}
	client, err := NewSearchHTTPClient(defaultDeepseekTimeout, params.ProxyURL)
	if err != nil {
		return nil, err
	}
	return &DeepseekProvider{
		client:  client,
		baseURL: defaultDeepseekSearchURL,
		apiKey:  strings.TrimSpace(params.APIKey),
	}, nil
}

func ValidateDeepseekParameters(params types.WebSearchProviderParameters) error {
	if strings.TrimSpace(params.APIKey) == "" {
		return fmt.Errorf("API key is required for DeepSeek provider")
	}
	return nil
}

func (p *DeepseekProvider) Name() string { return "deepseek" }

func (p *DeepseekProvider) Search(
	ctx context.Context, query string, maxResults int, includeDate bool,
) ([]*types.WebSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is empty")
	}
	if maxResults <= 0 {
		maxResults = defaultDeepseekResults
	}
	if maxResults > maxDeepseekResults {
		maxResults = maxDeepseekResults
	}

	// need_content 保持 false：summary 已经是 600~800 字、可直接喂给模型的摘要，
	// 拉取全文会让响应体明显变大，而检索召回并不依赖它。
	body, err := json.Marshal(deepseekSearchRequest{
		Query: query, Count: maxResults,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal DeepSeek request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create DeepSeek request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	logger.Infof(ctx, "[WebSearch][DeepSeek] query=%q maxResults=%d", query, maxResults)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute DeepSeek request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readDeepseekResponseBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, deepseekHTTPError(resp.StatusCode, respBody)
	}

	var response deepseekSearchResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal DeepSeek response: %w", err)
	}
	results := make([]*types.WebSearchResult, 0, len(response.Results))
	for _, item := range response.Results {
		if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.URL) == "" {
			continue
		}
		// summary 是长摘要，snippet 是短摘要；优先给模型可读的长版本。
		snippet := strings.TrimSpace(item.Summary)
		if snippet == "" {
			snippet = strings.TrimSpace(item.Snippet)
		}
		result := &types.WebSearchResult{
			Title:   item.Title,
			URL:     item.URL,
			Snippet: snippet,
			Content: strings.TrimSpace(item.Content),
			Source:  "deepseek",
		}
		if includeDate {
			if publishedAt, ok := parseDeepseekDate(item.Date); ok {
				result.PublishedAt = &publishedAt
			}
		}
		results = append(results, result)
		if len(results) >= maxResults {
			break
		}
	}
	logger.Infof(ctx, "[WebSearch][DeepSeek] returned %d results", len(results))
	return results, nil
}

func readDeepseekResponseBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxDeepseekResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read DeepSeek response: %w", err)
	}
	if len(body) > maxDeepseekResponseBytes {
		return nil, fmt.Errorf("DeepSeek response exceeds %d bytes", maxDeepseekResponseBytes)
	}
	return body, nil
}

// deepseekHTTPError 解析 DeepSeek 的 OpenAI 兼容错误信封，把可读原因透出来。
func deepseekHTTPError(statusCode int, body []byte) error {
	var apiError struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &apiError) == nil {
		if detail := strings.TrimSpace(apiError.Error.Message); detail != "" {
			return fmt.Errorf("DeepSeek API returned status %d: %s", statusCode, detail)
		}
	}
	detail := strings.TrimSpace(string(body))
	if len(detail) > 4096 {
		detail = detail[:4096]
	}
	if detail == "" {
		return fmt.Errorf("DeepSeek API returned status %d", statusCode)
	}
	return fmt.Errorf("DeepSeek API returned status %d: %s", statusCode, detail)
}

func parseDeepseekDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

type deepseekSearchRequest struct {
	Query       string `json:"query"`
	Count       int    `json:"count,omitempty"`
	NeedContent bool   `json:"need_content,omitempty"`
}

type deepseekSearchResponse struct {
	Results []deepseekSearchResult `json:"results"`
}

type deepseekSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Summary string `json:"summary"`
	Content string `json:"content"`
	Date    string `json:"date"`
}
