package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"app/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type mockAiClient struct {
	post         *GeneratedPost
	err          error
	checkErr     error
	gotPrompt    string
	gotHint      string
	gotSnippets  []string
	generateCall int
}

func (m *mockAiClient) CheckAccess() error { return m.checkErr }

func (m *mockAiClient) GenerateBlog(prompt, categoryHint string, searchSnippets []string) (*GeneratedPost, error) {
	m.generateCall++
	m.gotPrompt = prompt
	m.gotHint = categoryHint
	m.gotSnippets = searchSnippets
	return m.post, m.err
}

type mockSearchEngineClient struct {
	results []string
	err     error
	called  int
}

func (m *mockSearchEngineClient) Search(query string) ([]string, error) {
	m.called++
	return m.results, m.err
}

func newMockPost() *GeneratedPost {
	return &GeneratedPost{
		Title:    "  Masa Depan Golang  ",
		Category: "teknologi",
		Excerpt:  " Ringkasan singkat ",
		Content:  "## Pendahuluan\n\nIsi artikel [1].",
	}
}

func postJSON(h gin.HandlerFunc, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/posts/ai-generate", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h(c)
	return w
}

func chatBody(content string) []byte {
	b, _ := json.Marshal(openAiResponse{Choices: []openAiChoice{{Message: openAiMessage{Role: "assistant", Content: content}}}})
	return b
}

// ---------- service ----------

func TestAiService_GeneratePost_WithoutSearch(t *testing.T) {
	client := &mockAiClient{post: newMockPost()}
	search := &mockSearchEngineClient{results: []string{"Go - Bahasa (https://go.dev)"}}
	svc := NewAiService(client, search)

	res, err := svc.GeneratePost(GeneratePostRequest{Prompt: "  Tulis tentang Golang  ", IncludeWebSearch: false})
	require.NoError(t, err)

	assert.Equal(t, "Masa Depan Golang", res.Title)
	assert.Equal(t, "Teknologi", res.Category)
	assert.Equal(t, "Ringkasan singkat", res.Excerpt)
	assert.Equal(t, "## Pendahuluan\n\nIsi artikel [1].", res.Content)
	assert.NotNil(t, res.SearchSources)
	assert.Empty(t, res.SearchSources)
	assert.Equal(t, 0, search.called)
	assert.Equal(t, "Tulis tentang Golang", client.gotPrompt)

	raw, _ := json.Marshal(res)
	assert.Contains(t, string(raw), `"search_sources":[]`)
}

func TestAiService_GeneratePost_WithSearch(t *testing.T) {
	client := &mockAiClient{post: newMockPost()}
	search := &mockSearchEngineClient{results: []string{"Go - Bahasa (https://go.dev)", "Gin (https://gin-gonic.com)"}}
	svc := NewAiService(client, search)

	res, err := svc.GeneratePost(GeneratePostRequest{Prompt: "Golang", IncludeWebSearch: true})
	require.NoError(t, err)

	assert.Equal(t, search.results, res.SearchSources)
	assert.Equal(t, search.results, client.gotSnippets)
	assert.Contains(t, res.Content, "## Referensi")
	assert.Contains(t, res.Content, "1. Go - Bahasa (https://go.dev)")
	assert.Contains(t, res.Content, "2. Gin (https://gin-gonic.com)")

	// The model already wrote its own references section: content is left untouched
	client.post = &GeneratedPost{Title: "T", Content: "Isi\n\n## Referensi\n- https://go.dev"}
	res, err = svc.GeneratePost(GeneratePostRequest{Prompt: "Golang", IncludeWebSearch: true})
	require.NoError(t, err)
	assert.Equal(t, "Isi\n\n## Referensi\n- https://go.dev", res.Content)
}

func TestAiService_GeneratePost_SearchDegradesGracefully(t *testing.T) {
	client := &mockAiClient{post: newMockPost()}
	search := &mockSearchEngineClient{err: errors.New("search engine down")}
	svc := NewAiService(client, search)

	res, err := svc.GeneratePost(GeneratePostRequest{Prompt: "Golang", IncludeWebSearch: true})
	require.NoError(t, err)
	assert.Empty(t, res.SearchSources)
	assert.NotContains(t, res.Content, "## Referensi")

	search.err = nil
	search.results = nil
	res, err = svc.GeneratePost(GeneratePostRequest{Prompt: "Golang", IncludeWebSearch: true})
	require.NoError(t, err)
	assert.Empty(t, res.SearchSources)

	// no search client configured at all
	svcNoSearch := NewAiService(client, nil)
	res, err = svcNoSearch.GeneratePost(GeneratePostRequest{Prompt: "Golang", IncludeWebSearch: true})
	require.NoError(t, err)
	assert.Empty(t, res.SearchSources)
}

func TestAiService_GeneratePost_Errors(t *testing.T) {
	client := &mockAiClient{post: newMockPost()}
	svc := NewAiService(client, &mockSearchEngineClient{})

	_, err := svc.GeneratePost(GeneratePostRequest{Prompt: "   "})
	assert.ErrorIs(t, err, ErrPromptRequired)
	assert.Equal(t, 0, client.generateCall)

	client.checkErr = errors.New("cannot reach openai")
	_, err = svc.GeneratePost(GeneratePostRequest{Prompt: "Golang"})
	assert.ErrorContains(t, err, "gagal mengakses OpenAI")
	assert.ErrorIs(t, err, client.checkErr)
	assert.Equal(t, 0, client.generateCall)
	client.checkErr = nil

	client.err = errors.New("upstream timeout")
	_, err = svc.GeneratePost(GeneratePostRequest{Prompt: "Golang"})
	assert.EqualError(t, err, "upstream timeout")
}

func TestResolveCategory_And_AppendReferences(t *testing.T) {
	assert.Equal(t, "Bisnis", resolveCategory("BISNIS", "Opini"))
	assert.Equal(t, "Opini", resolveCategory("Olahraga", " opini "))
	assert.Equal(t, database.DefaultCategory, resolveCategory("Olahraga", "Hiburan"))
	assert.Equal(t, database.DefaultCategory, resolveCategory("", ""))

	assert.Equal(t, "isi", appendReferences("isi", nil))
	assert.Equal(t, "isi\n\n## Referensi\n\n1. a\n", appendReferences("isi\n\n", []string{"a"}))
	assert.Equal(t, "x ## REFERENSI", appendReferences("x ## REFERENSI", []string{"a"}))
}

// ---------- handler ----------

func TestAiHandler_GeneratePost(t *testing.T) {
	client := &mockAiClient{post: newMockPost()}
	search := &mockSearchEngineClient{results: []string{"Go (https://go.dev)"}}
	h := NewAiHandler(NewAiService(client, search))

	// success with web search
	w := postJSON(h.GeneratePost, `{"prompt":"Golang","category":"Teknologi","include_web_search":true}`)
	assert.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Status string               `json:"status"`
		Data   GeneratePostResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "success", env.Status)
	assert.Equal(t, "Masa Depan Golang", env.Data.Title)
	assert.Equal(t, []string{"Go (https://go.dev)"}, env.Data.SearchSources)
	assert.Equal(t, "Teknologi", client.gotHint)

	// invalid JSON, missing prompt, blank prompt
	assert.Equal(t, http.StatusBadRequest, postJSON(h.GeneratePost, `{invalid`).Code)
	assert.Equal(t, http.StatusBadRequest, postJSON(h.GeneratePost, `{}`).Code)
	assert.Equal(t, http.StatusBadRequest, postJSON(h.GeneratePost, `{"prompt":"   "}`).Code)

	// upstream failure
	client.err = errors.New("upstream timeout")
	w = postJSON(h.GeneratePost, `{"prompt":"Golang"}`)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"error"`)
}

// ---------- OpenAI client ----------

func TestOpenAiClient_GenerateBlog_Success(t *testing.T) {
	var gotAuth, gotPath string
	var gotReq openAiRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotReq)
		content, _ := json.Marshal(GeneratedPost{Title: "Judul", Category: "Teknologi", Excerpt: "Ringkas", Content: "## Isi\n\nTeks [1]."})
		_, _ = w.Write(chatBody("```json\n" + string(content) + "\n```"))
	}))
	defer srv.Close()

	client := NewOpenAiClient(srv.URL+"/", "test-key", "test-model")
	post, err := client.GenerateBlog("Tulis Golang", "Teknologi", []string{"Snippet 1", "Snippet 2"})
	require.NoError(t, err)
	assert.Equal(t, "Judul", post.Title)
	assert.Equal(t, "## Isi\n\nTeks [1].", post.Content)

	assert.Equal(t, "Bearer test-key", gotAuth)
	assert.Equal(t, "/chat/completions", gotPath)
	assert.Equal(t, "test-model", gotReq.Model)
	require.Len(t, gotReq.Messages, 2)
	assert.Equal(t, "system", gotReq.Messages[0].Role)
	assert.Contains(t, gotReq.Messages[0].Content, "Gaya Hidup")
	assert.Contains(t, gotReq.Messages[1].Content, "Tulis Golang")
	assert.Contains(t, gotReq.Messages[1].Content, "Kategori yang diinginkan: Teknologi")
	assert.Contains(t, gotReq.Messages[1].Content, "1. Snippet 1")
	assert.Contains(t, gotReq.Messages[1].Content, "## Referensi")
}

func TestBuildUserPrompt(t *testing.T) {
	plain := buildUserPrompt("Topik", "  ", nil)
	assert.Equal(t, "Topik", plain)
}

func TestOpenAiClient_GenerateBlog_Failures(t *testing.T) {
	serve := func(status int, body []byte) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write(body)
		}))
	}

	// upstream HTTP error
	s1 := serve(http.StatusInternalServerError, []byte("internal server error"))
	defer s1.Close()
	_, err := NewOpenAiClient(s1.URL, "k", "m").GenerateBlog("p", "", nil)
	assert.ErrorContains(t, err, "status 500")

	// empty choices
	s2 := serve(http.StatusOK, []byte(`{"choices":[]}`))
	defer s2.Close()
	_, err = NewOpenAiClient(s2.URL, "k", "m").GenerateBlog("p", "", nil)
	assert.ErrorContains(t, err, "empty response")

	// content is not JSON at all
	s3 := serve(http.StatusOK, chatBody("Not a json string at all"))
	defer s3.Close()
	_, err = NewOpenAiClient(s3.URL, "k", "m").GenerateBlog("p", "", nil)
	assert.ErrorContains(t, err, "failed to parse AI response")

	// response body itself is not JSON
	s4 := serve(http.StatusOK, []byte("<!DOCTYPE html><html>error</html>"))
	defer s4.Close()
	_, err = NewOpenAiClient(s4.URL, "k", "m").GenerateBlog("p", "", nil)
	assert.ErrorContains(t, err, "failed to parse OpenAI response")

	// network failure
	_, err = NewOpenAiClient("http://127.0.0.1:54399", "k", "m").GenerateBlog("p", "", nil)
	assert.ErrorContains(t, err, "failed to call OpenAI API")

	// invalid endpoint URL
	_, err = NewOpenAiClient("http://bad host", "k", "m").GenerateBlog("p", "", nil)
	assert.ErrorContains(t, err, "invalid OpenAI endpoint")
}

func TestParseGeneratedPost(t *testing.T) {
	// plain JSON
	p, err := parseGeneratedPost(`{"title":"A","content":"B"}`)
	require.NoError(t, err)
	assert.Equal(t, "A", p.Title)

	// surrounded by prose
	p, err = parseGeneratedPost("Berikut hasilnya: {\"title\":\"A\",\"content\":\"B\"} semoga membantu")
	require.NoError(t, err)
	assert.Equal(t, "B", p.Content)

	// braces but invalid JSON
	_, err = parseGeneratedPost("{ invalid }")
	assert.Error(t, err)

	// no braces
	_, err = parseGeneratedPost("tidak ada json")
	assert.Error(t, err)

	// missing mandatory fields
	_, err = parseGeneratedPost(`{"title":"  ","content":"B"}`)
	assert.ErrorContains(t, err, "missing title or content")
	_, err = parseGeneratedPost(`{"title":"A","content":""}`)
	assert.ErrorContains(t, err, "missing title or content")
}

func TestOpenAiClient_CheckAccess(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/models", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ok.Close()
	assert.NoError(t, NewOpenAiClient(ok.URL, "test-key", "m").CheckAccess())

	// missing key
	assert.ErrorContains(t, NewOpenAiClient(ok.URL, "  ", "m").CheckAccess(), "not configured")

	// unauthorized
	unauthorized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("unauthorized"))
	}))
	defer unauthorized.Close()
	assert.ErrorContains(t, NewOpenAiClient(unauthorized.URL, "bad", "m").CheckAccess(), "status 401")

	// unreachable and invalid endpoint
	assert.ErrorContains(t, NewOpenAiClient("http://127.0.0.1:59999", "k", "m").CheckAccess(), "failed to connect")
	assert.ErrorContains(t, NewOpenAiClient("http://bad host", "k", "m").CheckAccess(), "invalid OpenAI endpoint")
}

// ---------- search engine client (parsers) ----------

func TestSearchEngineClient(t *testing.T) {
	// Default client construction
	defClient := NewSearchEngineClient("")
	assert.NotNil(t, defClient)

	// Empty query returns empty
	emptyRes, err := defClient.Search("   ")
	assert.NoError(t, err)
	assert.Empty(t, emptyRes)

	// 1. Wikipedia OpenSearch parsing [term, titles, descriptions, urls]
	serverWikiOS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := `["golang", ["Golang", "Go Lang No Desc", "Only Title"], ["Bahasa pemrograman modern", "", ""], ["https://id.wikipedia.org/wiki/Golang", "https://id.wikipedia.org/wiki/Go", ""]]`
		_, _ = w.Write([]byte(raw))
	}))
	defer serverWikiOS.Close()

	clientWikiOS := NewSearchEngineClient(serverWikiOS.URL+"/wikipedia.org/api.php?custom=1", serverWikiOS.Client())
	resWikiOS, err := clientWikiOS.Search("golang")
	assert.NoError(t, err)
	assert.Len(t, resWikiOS, 3)
	assert.Contains(t, resWikiOS[0], "Bahasa pemrograman modern")
	assert.Contains(t, resWikiOS[1], "https://id.wikipedia.org/wiki/Go")
	assert.Equal(t, "Only Title", resWikiOS[2])

	// 2. Wikipedia Query parsing {"query": {"search": [...]}}
	serverWikiQuery := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := `{"query":{"search":[{"title":"React","snippet":"React is a <span>JS</span> library &amp; &quot;UI&quot; framework"}]}}`
		_, _ = w.Write([]byte(raw))
	}))
	defer serverWikiQuery.Close()

	clientWikiQuery := NewSearchEngineClient(serverWikiQuery.URL+"/wikipedia.org", serverWikiQuery.Client())
	resWikiQuery, err := clientWikiQuery.Search("React")
	assert.NoError(t, err)
	assert.Len(t, resWikiQuery, 1)
	assert.Contains(t, resWikiQuery[0], "React is a JS library & \"UI\" framework")

	// 3. DuckDuckGo parsing
	serverDDG := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := `{"Heading":"Go","AbstractText":"Open source programming language","AbstractURL":"https://duckduckgo.com/Go","RelatedTopics":[{"Text":"Go syntax","FirstURL":"https://duckduckgo.com/syntax"}]}`
		_, _ = w.Write([]byte(raw))
	}))
	defer serverDDG.Close()

	clientDDG := NewSearchEngineClient(serverDDG.URL+"/duckduckgo.com?lang=id", serverDDG.Client())
	resDDG, err := clientDDG.Search("Go")
	assert.NoError(t, err)
	assert.Len(t, resDDG, 2)
	assert.Contains(t, resDDG[0], "Open source programming language")

	// 4. Standard SearXNG / Delcom format with empty snippet and valid snippet
	serverStandard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := SearchEngineResponse{
			Results: []SearchResultItem{
				{Title: "Title 1", Snippet: "Snippet 1", URL: "https://example.com/1"},
				{Title: "Title 2", Snippet: "", URL: "https://example.com/2"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer serverStandard.Close()

	clientStandard := NewSearchEngineClient(serverStandard.URL+"?lang=en", serverStandard.Client())
	resStandard, err := clientStandard.Search("test")
	assert.NoError(t, err)
	assert.Len(t, resStandard, 2)
	assert.Contains(t, resStandard[0], "Snippet 1")
	assert.Contains(t, resStandard[1], "(https://example.com/2)")

	// 5. Server OK but empty results -> fallback information
	serverEmpty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := SearchEngineResponse{Results: []SearchResultItem{}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer serverEmpty.Close()

	clientEmpty := NewSearchEngineClient(serverEmpty.URL, serverEmpty.Client())
	resEmpty, err := clientEmpty.Search("unknown query")
	assert.NoError(t, err)
	assert.Len(t, resEmpty, 1)
	assert.Contains(t, resEmpty[0], "Informasi relevan mengenai: unknown query")

	// 6. Server Plain text body fallback
	serverNonJson := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Plain text search snippet &amp; notes"))
	}))
	defer serverNonJson.Close()

	clientNonJson := NewSearchEngineClient(serverNonJson.URL, serverNonJson.Client())
	resNonJson, err := clientNonJson.Search("plain")
	assert.NoError(t, err)
	assert.Equal(t, []string{"Plain text search snippet & notes"}, resNonJson)

	// 7. Server Status != 200
	serverError := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer serverError.Close()

	clientError := NewSearchEngineClient(serverError.URL, serverError.Client())
	_, err = clientError.Search("error query")
	assert.Error(t, err)

	// 8. Invalid URL / Request creation error (using invalid URL character)
	clientBadURL := NewSearchEngineClient("http://[fe80::1%en0]:invalid-url", &http.Client{})
	_, err = clientBadURL.Search("bad")
	assert.Error(t, err)

	// 9. Network connection failure
	clientNetFail := NewSearchEngineClient("http://127.0.0.1:54398", &http.Client{})
	_, err = clientNetFail.Search("fail")
	assert.Error(t, err)
}
