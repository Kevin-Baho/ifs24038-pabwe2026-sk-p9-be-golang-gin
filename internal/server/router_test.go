package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"app/internal/config"
	"app/internal/database"
)

const testKey = "test-secret"

type env struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func call(r http.Handler, method, path, token, body string) (*httptest.ResponseRecorder, env) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var e env
	_ = json.Unmarshal(w.Body.Bytes(), &e)
	return w, e
}

func newRouter(t *testing.T, openAI, search string) http.Handler {
	t.Helper()
	cfg := &config.Config{
		AppEnv:               "development",
		AppKey:               testKey,
		AdminUsername:        "admin",
		AdminPassword:        "admin",
		OpenAIEndpoint:       openAI,
		OpenAIKey:            "k",
		OpenAIModel:          "m",
		SearchEngineEndpoint: search,
	}
	db, err := database.ConnectTest()
	require.NoError(t, err)
	require.NoError(t, database.Seed(db, cfg))
	return SetupRouter(cfg, db)
}

func login(t *testing.T, r http.Handler, username, password string) string {
	t.Helper()
	w, e := call(r, http.MethodPost, "/api/auth/login", "", `{"username":"`+username+`","password":"`+password+`"}`)
	require.Equal(t, http.StatusOK, w.Code)
	var d struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(e.Data, &d))
	return d.Token
}

func TestSetupRouter_PureApi_Docs(t *testing.T) {
	cfg := &config.Config{AppEnv: "production", AppKey: testKey}
	db, err := database.ConnectTest()
	require.NoError(t, err)
	router := SetupRouter(cfg, db)

	// 404 handler
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodGet, "/random-unmatched-route", nil)
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusNotFound, w1.Code)
	assert.JSONEq(t, `{"status":"fail","message":"Endpoint not found"}`, w1.Body.String())

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/api/nonexistent", nil)
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusNotFound, w2.Code)

	// Swagger UI + initializer
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest(http.MethodGet, "/swagger/index.html", nil)
	req4.RequestURI = "/swagger/index.html"
	router.ServeHTTP(w4, req4)
	assert.Equal(t, http.StatusOK, w4.Code)
	assert.Contains(t, w4.Body.String(), "swagger-ui")

	w5 := httptest.NewRecorder()
	req5, _ := http.NewRequest(http.MethodGet, "/swagger/swagger-initializer.js", nil)
	req5.RequestURI = "/swagger/swagger-initializer.js"
	router.ServeHTTP(w5, req5)
	assert.Equal(t, http.StatusOK, w5.Code)
	assert.Contains(t, w5.Body.String(), `url: "/swagger/api.json"`)

	// Dynamic host + scheme
	w6 := httptest.NewRecorder()
	req6, _ := http.NewRequest(http.MethodGet, "/swagger/api.json", nil)
	req6.RequestURI = "/swagger/api.json"
	req6.Host = "api.myapp.test:8081"
	router.ServeHTTP(w6, req6)
	assert.Equal(t, http.StatusOK, w6.Code)
	assert.Contains(t, w6.Body.String(), `"swagger": "2.0"`)
	assert.Contains(t, w6.Body.String(), `"host": "api.myapp.test:8081"`)

	// Behind a proxy
	w7 := httptest.NewRecorder()
	req7, _ := http.NewRequest(http.MethodGet, "/swagger/api.json", nil)
	req7.RequestURI = "/swagger/api.json"
	req7.Header.Set("X-Forwarded-Host", "blog.del.ac.id")
	req7.Header.Set("X-Forwarded-Proto", "https")
	router.ServeHTTP(w7, req7)
	assert.Equal(t, http.StatusOK, w7.Code)
	assert.Contains(t, w7.Body.String(), `"host": "blog.del.ac.id"`)
	assert.Contains(t, w7.Body.String(), `"https"`)

	// doc.json backward compatibility
	w8 := httptest.NewRecorder()
	req8, _ := http.NewRequest(http.MethodGet, "/swagger/doc.json", nil)
	req8.RequestURI = "/swagger/doc.json"
	router.ServeHTTP(w8, req8)
	assert.Equal(t, http.StatusOK, w8.Code)

	// Root /api.json
	w9 := httptest.NewRecorder()
	req9, _ := http.NewRequest(http.MethodGet, "/api.json", nil)
	req9.Host = "localhost:8081"
	router.ServeHTTP(w9, req9)
	assert.Equal(t, http.StatusOK, w9.Code)
	assert.Contains(t, w9.Body.String(), `"host": "localhost:8081"`)

	// Redirects
	w10a := httptest.NewRecorder()
	req10a, _ := http.NewRequest(http.MethodGet, "/swagger", nil)
	router.ServeHTTP(w10a, req10a)
	assert.Equal(t, http.StatusMovedPermanently, w10a.Code)

	w10b := httptest.NewRecorder()
	req10b, _ := http.NewRequest(http.MethodGet, "/swagger/", nil)
	router.ServeHTTP(w10b, req10b)
	assert.Equal(t, http.StatusMovedPermanently, w10b.Code)

	// CORS preflight
	w11 := httptest.NewRecorder()
	req11, _ := http.NewRequest(http.MethodOptions, "/api/posts", nil)
	router.ServeHTTP(w11, req11)
	assert.Equal(t, http.StatusNoContent, w11.Code)
	assert.Equal(t, "*", w11.Header().Get("Access-Control-Allow-Origin"))
}

// TestSetupRouter_AllEndpoints walks the 19 REST endpoints end to end with real JWTs.
func TestSetupRouter_AllEndpoints(t *testing.T) {
	openAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), "Konteks Penelusuran")
		content, _ := json.Marshal(map[string]string{
			"title": "AI Menulis", "category": "Teknologi", "excerpt": "Ringkas", "content": "## Isi\n\nFakta [1].",
		})
		resp, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": string(content)}}}})
		_, _ = w.Write(resp)
	}))
	defer openAI.Close()

	search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"title":"Go","snippet":"Bahasa","url":"https://go.dev"}]}`))
	}))
	defer search.Close()

	r := newRouter(t, openAI.URL, search.URL)

	// --- public ---
	w, e := call(r, http.MethodGet, "/api/public/categories", "", "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, string(e.Data), "Gaya Hidup")

	// --- auth ---
	w, e = call(r, http.MethodPost, "/api/auth/register", "", `{"username":"writer","email":"writer@example.com","password":"password123"}`)
	require.Equal(t, http.StatusCreated, w.Code)
	var reg struct {
		Token string `json:"token"`
		User  struct {
			ID   uint   `json:"id"`
			Role string `json:"role"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal(e.Data, &reg))
	assert.Equal(t, "member", reg.User.Role)

	token := login(t, r, "writer", "password123")
	memberToken := login(t, r, "member", "member")
	adminToken := login(t, r, "admin", "admin")

	w, _ = call(r, http.MethodGet, "/api/auth/profile", token, "")
	assert.Equal(t, http.StatusOK, w.Code)
	w, _ = call(r, http.MethodPut, "/api/auth/profile", token, `{"email":"writer2@example.com"}`)
	assert.Equal(t, http.StatusOK, w.Code)
	w, _ = call(r, http.MethodPost, "/api/auth/logout", token, "")
	assert.Equal(t, http.StatusOK, w.Code)

	// protected routes reject anonymous callers
	for _, p := range []struct{ m, path string }{
		{http.MethodPost, "/api/auth/logout"},
		{http.MethodGet, "/api/auth/profile"},
		{http.MethodGet, "/api/posts"},
		{http.MethodPost, "/api/posts/ai-generate"},
		{http.MethodGet, "/api/admin/users"},
	} {
		w, _ = call(r, p.m, p.path, "", "")
		assert.Equal(t, http.StatusUnauthorized, w.Code, p.path)
	}

	// --- author posts ---
	w, e = call(r, http.MethodPost, "/api/posts", token, `{"title":"Artikel Pertama","category":"Bisnis","content":"Isi artikel pertama","is_published":false}`)
	require.Equal(t, http.StatusCreated, w.Code)
	var created struct {
		ID   uint   `json:"id"`
		Slug string `json:"slug"`
	}
	require.NoError(t, json.Unmarshal(e.Data, &created))
	id := strconv.Itoa(int(created.ID))

	w, _ = call(r, http.MethodGet, "/api/posts", token, "")
	assert.Equal(t, http.StatusOK, w.Code)

	w, e = call(r, http.MethodGet, "/api/posts/stats", token, "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"total_articles":1,"total_selesai":0,"belum_selesai":1}`, string(e.Data))

	w, _ = call(r, http.MethodGet, "/api/posts/"+id, token, "")
	assert.Equal(t, http.StatusOK, w.Code)
	w, _ = call(r, http.MethodPut, "/api/posts/"+id, token, `{"title":"Artikel Pertama (Revisi)"}`)
	assert.Equal(t, http.StatusOK, w.Code)

	// draft is not public yet
	w, _ = call(r, http.MethodGet, "/api/public/posts/"+created.Slug, "", "")
	assert.Equal(t, http.StatusNotFound, w.Code)

	w, _ = call(r, http.MethodPatch, "/api/posts/"+id+"/toggle-publish", token, "")
	assert.Equal(t, http.StatusOK, w.Code)

	w, e = call(r, http.MethodGet, "/api/public/posts?q=pertama&category=Bisnis", "", "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, string(e.Data), created.Slug)
	w, e = call(r, http.MethodGet, "/api/public/posts/"+created.Slug, "", "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, string(e.Data), `"reading_time":1`)
	assert.Contains(t, string(e.Data), `"username":"writer"`)

	// --- AI generator with web search ---
	w, e = call(r, http.MethodPost, "/api/posts/ai-generate", token, `{"prompt":"Tulis tentang Go","include_web_search":true}`)
	require.Equal(t, http.StatusOK, w.Code)
	var gen struct {
		Title         string   `json:"title"`
		Content       string   `json:"content"`
		SearchSources []string `json:"search_sources"`
	}
	require.NoError(t, json.Unmarshal(e.Data, &gen))
	assert.Equal(t, "AI Menulis", gen.Title)
	assert.Len(t, gen.SearchSources, 1)
	assert.Contains(t, gen.Content, "## Referensi")

	// --- admin (role guard) ---
	w, _ = call(r, http.MethodGet, "/api/admin/users", memberToken, "")
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, e = call(r, http.MethodGet, "/api/admin/users", adminToken, "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, string(e.Data), `"total_posts":1`)
	w, _ = call(r, http.MethodGet, "/api/admin/posts", adminToken, "")
	assert.Equal(t, http.StatusOK, w.Code)
	w, _ = call(r, http.MethodGet, "/api/admin/users/"+strconv.Itoa(int(reg.User.ID))+"/posts", adminToken, "")
	assert.Equal(t, http.StatusOK, w.Code)

	// --- delete ---
	w, _ = call(r, http.MethodDelete, "/api/posts/"+id, token, "")
	assert.Equal(t, http.StatusOK, w.Code)
	w, _ = call(r, http.MethodGet, "/api/posts/"+id, token, "")
	assert.Equal(t, http.StatusNotFound, w.Code)
}
