package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"app/internal/database"
)

type AiClientInterface interface {
	CheckAccess() error
	GenerateBlog(prompt, categoryHint string, searchSnippets []string) (*GeneratedPost, error)
}

type openAiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAiRequest struct {
	Model       string          `json:"model"`
	Messages    []openAiMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
}

type openAiChoice struct {
	Message openAiMessage `json:"message"`
}

type openAiResponse struct {
	Choices []openAiChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type OpenAiClient struct {
	Endpoint   string
	Key        string
	Model      string
	HttpClient *http.Client
}

func NewOpenAiClient(endpoint, key, model string) *OpenAiClient {
	return &OpenAiClient{
		Endpoint: strings.TrimRight(endpoint, "/"),
		Key:      key,
		Model:    model,
		HttpClient: &http.Client{
			// Writing a full article is slower than a short completion.
			Timeout: 120 * time.Second,
		},
	}
}

func (c *OpenAiClient) CheckAccess() error {
	if strings.TrimSpace(c.Key) == "" {
		return errors.New("OpenAI API key is not configured")
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/models", c.Endpoint), nil)
	if err != nil {
		return fmt.Errorf("invalid OpenAI endpoint: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.Key))

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to OpenAI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenAI API returned status %d: %s", resp.StatusCode, string(respBytes))
	}
	return nil
}

func buildSystemInstruction() string {
	return fmt.Sprintf(`Kamu adalah penulis blog profesional berbahasa Indonesia.
Tulis SATU artikel blog lengkap dan kembalikan HANYA objek JSON murni, TANPA blok markdown, TANPA kata pembuka/penutup.
Format JSON wajib:
{"title": "Judul menarik", "category": "salah satu dari: %s", "excerpt": "Ringkasan 1-2 kalimat (maks. 200 karakter)", "content": "Isi artikel dalam Markdown"}
Aturan "content":
- Gunakan Markdown: sub-judul (##), paragraf, dan daftar bila perlu.
- Jika diberikan rujukan web, gunakan faktanya, kutip dengan penanda sitasi [1], [2], dst., lalu tutup artikel dengan bagian "## Referensi" berisi daftar tautan sumber.`,
		strings.Join(database.PostCategories, ", "))
}

func buildUserPrompt(prompt, categoryHint string, searchSnippets []string) string {
	var b strings.Builder
	b.WriteString(prompt)

	if strings.TrimSpace(categoryHint) != "" {
		b.WriteString(fmt.Sprintf("\n\nKategori yang diinginkan: %s", categoryHint))
	}

	if len(searchSnippets) > 0 {
		b.WriteString("\n\nKonteks Penelusuran Mesin Pencari / Search Engine:\n")
		for i, snippet := range searchSnippets {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, snippet))
		}
		b.WriteString("\nGunakan fakta dan rujukan di atas, sertakan sitasi [n] di dalam artikel, dan cantumkan tautan sumber pada bagian \"## Referensi\".")
	}
	return b.String()
}

// parseGeneratedPost extracts the JSON object from the model output (tolerating
// ```json fences or surrounding prose) and validates the mandatory fields.
func parseGeneratedPost(raw string) (*GeneratedPost, error) {
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}

	var gen GeneratedPost
	if err := json.Unmarshal([]byte(raw), &gen); err != nil {
		return nil, fmt.Errorf("failed to parse AI response into blog post: %w", err)
	}
	if strings.TrimSpace(gen.Title) == "" || strings.TrimSpace(gen.Content) == "" {
		return nil, errors.New("AI response is missing title or content")
	}
	return &gen, nil
}

func (c *OpenAiClient) GenerateBlog(prompt, categoryHint string, searchSnippets []string) (*GeneratedPost, error) {
	reqPayload := openAiRequest{
		Model: c.Model,
		Messages: []openAiMessage{
			{Role: "system", Content: buildSystemInstruction()},
			{Role: "user", Content: buildUserPrompt(prompt, categoryHint, searchSnippets)},
		},
		Temperature: 0.7,
	}

	// Marshalling plain strings/floats cannot fail.
	bodyBytes, _ := json.Marshal(reqPayload)

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/chat/completions", c.Endpoint), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("invalid OpenAI endpoint: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.Key))

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenAI API: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var aiResp openAiResponse
	if err := json.Unmarshal(respBytes, &aiResp); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
	}

	if len(aiResp.Choices) == 0 {
		return nil, errors.New("empty response from OpenAI API")
	}

	return parseGeneratedPost(aiResp.Choices[0].Message.Content)
}
