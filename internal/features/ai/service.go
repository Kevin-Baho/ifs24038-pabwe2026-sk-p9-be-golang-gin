package ai

import (
	"errors"
	"fmt"
	"strings"

	"app/internal/database"
)

var ErrTopicRequired = errors.New("topic is required")
var ErrPromptRequired = ErrTopicRequired

type AiService interface {
	GeneratePost(req GeneratePostRequest) (*GeneratePostResponse, error)
}

type aiService struct {
	client       AiClientInterface
	searchClient SearchEngineClient
}

func NewAiService(client AiClientInterface, searchClient SearchEngineClient) AiService {
	return &aiService{
		client:       client,
		searchClient: searchClient,
	}
}

// matchCategory returns the canonical category name for a case-insensitive input.
func matchCategory(input string) (string, bool) {
	input = strings.TrimSpace(input)
	for _, c := range database.PostCategories {
		if strings.EqualFold(c, input) {
			return c, true
		}
	}
	return "", false
}

// resolveCategory prefers the model's category, then the caller's hint, then "Umum".
func resolveCategory(generated, hint string) string {
	if c, ok := matchCategory(generated); ok {
		return c
	}
	if c, ok := matchCategory(hint); ok {
		return c
	}
	return database.DefaultCategory
}

// appendReferences guarantees a "## Referensi" section whenever web sources were used.
func appendReferences(content string, sources []string) string {
	if len(sources) == 0 || strings.Contains(strings.ToLower(content), "## referensi") {
		return content
	}

	var b strings.Builder
	b.WriteString(strings.TrimRight(content, "\n"))
	b.WriteString("\n\n## Referensi\n\n")
	for i, src := range sources {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, src))
	}
	return b.String()
}

func (s *aiService) GeneratePost(req GeneratePostRequest) (*GeneratePostResponse, error) {
	prompt := strings.TrimSpace(req.Topic)
	if prompt == "" {
		prompt = strings.TrimSpace(req.Prompt)
	}
	if prompt == "" {
		return nil, ErrTopicRequired
	}

	// 1. Verify the OpenAI connection before doing any other work
	if err := s.client.CheckAccess(); err != nil {
		return nil, fmt.Errorf("gagal mengakses OpenAI: %w", err)
	}

	// 2. Only when requested, gather up-to-date facts from the search engine.
	// A failing search engine degrades gracefully to a generation without sources.
	sources := []string{}
	if req.IncludeWebSearch && s.searchClient != nil {
		snippets, err := s.searchClient.Search(prompt)
		if err == nil && len(snippets) > 0 {
			sources = snippets
		}
	}

	// 3. Generate the article
	gen, err := s.client.GenerateBlog(prompt, req.Category, sources)
	if err != nil {
		return nil, err
	}

	return &GeneratePostResponse{
		Title:         strings.TrimSpace(gen.Title),
		Category:      resolveCategory(gen.Category, req.Category),
		Excerpt:       strings.TrimSpace(gen.Excerpt),
		Content:       appendReferences(gen.Content, sources),
		SearchSources: sources,
	}, nil
}
