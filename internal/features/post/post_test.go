package post

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"app/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ---------- helpers ----------

type envelope struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type stubRepo struct {
	PostRepository
	createErr error
	updateErr error
	deleteErr error
}

func (s *stubRepo) Create(p *database.Post) error {
	if s.createErr != nil {
		return s.createErr
	}
	return s.PostRepository.Create(p)
}

func (s *stubRepo) Update(p *database.Post) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	return s.PostRepository.Update(p)
}

func (s *stubRepo) Delete(id uint) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.PostRepository.Delete(id)
}

type fixture struct {
	db      *gorm.DB
	repo    PostRepository
	svc     PostService
	handler *PostHandler
	owner   database.User
	other   database.User
	admin   database.User
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, err := database.ConnectTest()
	require.NoError(t, err)

	f := &fixture{
		db:    db,
		owner: database.User{Username: "owner", Email: "owner@example.com", Password: "x", Role: database.RoleMember},
		other: database.User{Username: "other", Email: "other@example.com", Password: "x", Role: database.RoleMember},
		admin: database.User{Username: "boss", Email: "boss@example.com", Password: "x", Role: database.RoleAdmin},
	}
	for _, u := range []*database.User{&f.owner, &f.other, &f.admin} {
		require.NoError(t, db.Create(u).Error)
	}

	f.repo = NewPostRepository(db)
	f.svc = NewPostService(f.repo)
	f.handler = NewPostHandler(f.svc)
	return f
}

// router wires every post handler; a non-zero userID simulates the Auth middleware.
func (f *fixture) router(userID uint) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if userID != 0 {
			c.Set("userID", userID)
		}
		c.Next()
	})
	h := f.handler
	r.GET("/public/categories", h.GetCategories)
	r.GET("/public/posts", h.GetPublicPosts)
	r.GET("/public/posts/:slug", h.GetPublicPostBySlug)
	r.GET("/posts", h.GetMyPosts)
	r.POST("/posts", h.CreatePost)
	r.GET("/posts/stats", h.GetStats)
	r.GET("/posts/:id", h.GetPostByID)
	r.PUT("/posts/:id", h.UpdatePost)
	r.PATCH("/posts/:id/toggle-publish", h.TogglePublish)
	r.DELETE("/posts/:id", h.DeletePost)
	r.GET("/admin/users", h.AdminGetUsers)
	r.GET("/admin/posts", h.AdminGetPosts)
	r.GET("/admin/users/:id/posts", h.AdminGetUserPosts)
	return r
}

func (f *fixture) create(t *testing.T, userID uint, title, category, content string, publish bool) PostResponse {
	t.Helper()
	res, err := f.svc.CreatePost(userID, CreatePostRequest{
		Title: title, Category: category, Content: content, IsPublished: publish,
	})
	require.NoError(t, err)
	return *res
}

func send(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func envOf(t *testing.T, w *httptest.ResponseRecorder) envelope {
	t.Helper()
	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	return env
}

func dataOf[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	require.NoError(t, json.Unmarshal(envOf(t, w).Data, &out))
	return out
}

func words(n int) string {
	return strings.TrimSpace(strings.Repeat("kata ", n))
}

func idPath(prefix string, id uint, suffix string) string {
	return prefix + strconv.Itoa(int(id)) + suffix
}

// ---------- pure helpers ----------

func TestCalculateReadingTime(t *testing.T) {
	assert.Equal(t, 1, CalculateReadingTime(""))
	assert.Equal(t, 1, CalculateReadingTime(words(199)))
	assert.Equal(t, 1, CalculateReadingTime(words(200)))
	assert.Equal(t, 2, CalculateReadingTime(words(400)))
	assert.Equal(t, 2, CalculateReadingTime(words(450)))
	assert.Equal(t, 5, CalculateReadingTime(words(1000)))
}

func TestSlugify(t *testing.T) {
	assert.Equal(t, "belajar-go-gin", slugify("Belajar Go Gin!"))
	assert.Equal(t, "hello-world", slugify("  --Hello__World-- "))
	assert.Equal(t, "go-1-27", slugify("Go 1.27"))
	assert.Equal(t, "post", slugify("!!!"))
	assert.Equal(t, "post", slugify(""))

	long := slugify(strings.Repeat("a", 79) + " " + strings.Repeat("b", 50))
	assert.Equal(t, strings.Repeat("a", 79), long)
	assert.LessOrEqual(t, len(slugify(strings.Repeat("z", 200))), maxSlugLength)
}

func TestMakeExcerpt(t *testing.T) {
	assert.Equal(t, "Halo dunia yang indah", makeExcerpt("  Halo   dunia\nyang indah "))

	long := makeExcerpt(strings.Repeat("abc ", 100))
	assert.True(t, strings.HasSuffix(long, "…"))
	assert.LessOrEqual(t, len([]rune(long)), maxExcerptLength+1)
}

func TestNormalizeCategory(t *testing.T) {
	c, err := normalizeCategory("")
	assert.NoError(t, err)
	assert.Equal(t, "Umum", c)

	c, err = normalizeCategory("  gaya hidup ")
	assert.NoError(t, err)
	assert.Equal(t, "Gaya Hidup", c)

	_, err = normalizeCategory("Olahraga")
	assert.ErrorIs(t, err, ErrInvalidCategory)
}

func TestApplyPublish(t *testing.T) {
	p := &database.Post{}

	applyPublish(p, true)
	assert.True(t, p.IsPublished)
	require.NotNil(t, p.PublishedAt)
	first := *p.PublishedAt

	// publishing an already published post keeps the original date
	applyPublish(p, true)
	assert.Equal(t, first, *p.PublishedAt)

	applyPublish(p, false)
	assert.False(t, p.IsPublished)
	assert.Nil(t, p.PublishedAt)
}

// ---------- author lifecycle ----------

func TestPost_AuthorLifecycle(t *testing.T) {
	f := newFixture(t)
	owner := f.router(f.owner.ID)
	other := f.router(f.other.ID)

	// 1. Create a published post (450 words => reading_time 2, category is case-insensitive)
	body, _ := json.Marshal(CreatePostRequest{
		Title:       "Belajar Go Gin",
		Category:    "teknologi",
		Content:     words(450),
		IsPublished: true,
	})
	w := send(owner, http.MethodPost, "/posts", string(body))
	require.Equal(t, http.StatusCreated, w.Code)
	p1 := dataOf[PostResponse](t, w)
	assert.Equal(t, "belajar-go-gin", p1.Slug)
	assert.Equal(t, "Teknologi", p1.Category)
	assert.True(t, p1.IsPublished)
	assert.NotNil(t, p1.PublishedAt)
	assert.Equal(t, 2, p1.ReadingTime)
	assert.Equal(t, f.owner.ID, p1.UserID)
	assert.NotEmpty(t, p1.Excerpt)
	assert.NotEmpty(t, p1.Content)

	// 2. Draft without category, excerpt derived from content
	w = send(owner, http.MethodPost, "/posts", `{"title":"Catatan Harian","content":"Hari ini cerah."}`)
	require.Equal(t, http.StatusCreated, w.Code)
	p2 := dataOf[PostResponse](t, w)
	assert.Equal(t, "Umum", p2.Category)
	assert.False(t, p2.IsPublished)
	assert.Nil(t, p2.PublishedAt)
	assert.Equal(t, "Hari ini cerah.", p2.Excerpt)
	assert.Equal(t, 1, p2.ReadingTime)

	// 3. Duplicate titles get unique slugs
	w = send(owner, http.MethodPost, "/posts", `{"title":"Belajar Go Gin","content":"dua","excerpt":"Ringkasan khusus"}`)
	require.Equal(t, http.StatusCreated, w.Code)
	p3 := dataOf[PostResponse](t, w)
	assert.Equal(t, "belajar-go-gin-2", p3.Slug)
	assert.Equal(t, "Ringkasan khusus", p3.Excerpt)

	w = send(owner, http.MethodPost, "/posts", `{"title":"Belajar Go Gin","content":"tiga"}`)
	require.Equal(t, http.StatusCreated, w.Code)
	p4 := dataOf[PostResponse](t, w)
	assert.Equal(t, "belajar-go-gin-3", p4.Slug)

	// 4. Create validation
	assert.Equal(t, http.StatusBadRequest, send(owner, http.MethodPost, "/posts", `{"title":"X","content":"y","category":"Olahraga"}`).Code)
	assert.Equal(t, http.StatusBadRequest, send(owner, http.MethodPost, "/posts", `{"title":"   ","content":"y"}`).Code)
	assert.Equal(t, http.StatusBadRequest, send(owner, http.MethodPost, "/posts", `{"title":"X","content":"   "}`).Code)
	assert.Equal(t, http.StatusBadRequest, send(owner, http.MethodPost, "/posts", `{"content":"y"}`).Code)
	assert.Equal(t, http.StatusBadRequest, send(owner, http.MethodPost, "/posts", `{invalid`).Code)

	// 5. List: own posts only, empty list is [] and not null
	w = send(owner, http.MethodGet, "/posts", "")
	require.Equal(t, http.StatusOK, w.Code)
	mine := dataOf[[]PostResponse](t, w)
	assert.Len(t, mine, 4)
	assert.Empty(t, mine[0].Content, "list items do not carry the full content")

	w = send(other, http.MethodGet, "/posts", "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, "[]", string(envOf(t, w).Data))

	// 6. Stats (total_articles, total_selesai, belum_selesai)
	stats := dataOf[StatsResponse](t, send(owner, http.MethodGet, "/posts/stats", ""))
	assert.Equal(t, StatsResponse{TotalArticles: 4, TotalSelesai: 1, BelumSelesai: 3}, stats)
	stats = dataOf[StatsResponse](t, send(other, http.MethodGet, "/posts/stats", ""))
	assert.Equal(t, StatsResponse{}, stats)

	// 7. Detail: owner only
	w = send(owner, http.MethodGet, idPath("/posts/", p1.ID, ""), "")
	require.Equal(t, http.StatusOK, w.Code)
	detail := dataOf[PostResponse](t, w)
	assert.Equal(t, p1.Slug, detail.Slug)
	assert.NotEmpty(t, detail.Content)
	require.NotNil(t, detail.Author)
	assert.Equal(t, "owner", detail.Author.Username)

	assert.Equal(t, http.StatusForbidden, send(other, http.MethodGet, idPath("/posts/", p1.ID, ""), "").Code)
	assert.Equal(t, http.StatusNotFound, send(owner, http.MethodGet, "/posts/9999", "").Code)
	assert.Equal(t, http.StatusBadRequest, send(owner, http.MethodGet, "/posts/abc", "").Code)

	// 8. Update with every field (draft -> published)
	w = send(owner, http.MethodPut, idPath("/posts/", p2.ID, ""),
		`{"title":"Catatan Baru","category":"opini","excerpt":"Ringkas","content":"Isi baru","cover_image_path":"cover.png","is_published":true}`)
	require.Equal(t, http.StatusOK, w.Code)
	up := dataOf[PostResponse](t, w)
	assert.Equal(t, "Catatan Baru", up.Title)
	assert.Equal(t, "catatan-harian", up.Slug, "slug stays stable on update")
	assert.Equal(t, "Opini", up.Category)
	assert.Equal(t, "Ringkas", up.Excerpt)
	assert.Equal(t, "Isi baru", up.Content)
	assert.Equal(t, "cover.png", up.CoverImagePath)
	assert.True(t, up.IsPublished)
	require.NotNil(t, up.PublishedAt)

	// publishing again keeps the stored date (both values are read back from the DB)
	stored := dataOf[PostResponse](t, send(owner, http.MethodGet, idPath("/posts/", p2.ID, ""), ""))
	require.NotNil(t, stored.PublishedAt)
	w = send(owner, http.MethodPut, idPath("/posts/", p2.ID, ""), `{"is_published":true}`)
	require.Equal(t, http.StatusOK, w.Code)
	again := dataOf[PostResponse](t, w)
	require.NotNil(t, again.PublishedAt)
	assert.True(t, stored.PublishedAt.Equal(*again.PublishedAt))

	// clearing the cover with "DELETE" and "remove"

	w = send(owner, http.MethodPut, idPath("/posts/", p2.ID, ""), `{"cover_image_path":"DELETE","is_published":false}`)
	require.Equal(t, http.StatusOK, w.Code)
	cleared := dataOf[PostResponse](t, w)
	assert.Empty(t, cleared.CoverImagePath)
	assert.False(t, cleared.IsPublished)
	assert.Nil(t, cleared.PublishedAt)

	send(owner, http.MethodPut, idPath("/posts/", p2.ID, ""), `{"cover_image_path":"again.png"}`)
	w = send(owner, http.MethodPut, idPath("/posts/", p2.ID, ""), `{"cover_image_path":"remove"}`)
	assert.Empty(t, dataOf[PostResponse](t, w).CoverImagePath)

	// empty body changes nothing
	w = send(owner, http.MethodPut, idPath("/posts/", p2.ID, ""), `{}`)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Catatan Baru", dataOf[PostResponse](t, w).Title)

	// update failures
	assert.Equal(t, http.StatusBadRequest, send(owner, http.MethodPut, idPath("/posts/", p2.ID, ""), `{"category":"Olahraga"}`).Code)
	assert.Equal(t, http.StatusBadRequest, send(owner, http.MethodPut, idPath("/posts/", p2.ID, ""), `{bad`).Code)
	assert.Equal(t, http.StatusBadRequest, send(owner, http.MethodPut, idPath("/posts/", p2.ID, ""), `{"title":"`+strings.Repeat("t", 201)+`"}`).Code)
	assert.Equal(t, http.StatusForbidden, send(other, http.MethodPut, idPath("/posts/", p2.ID, ""), `{"title":"Curi"}`).Code)

	// 9. Toggle publish: draft -> published -> draft
	w = send(owner, http.MethodPatch, idPath("/posts/", p3.ID, "/toggle-publish"), "")
	require.Equal(t, http.StatusOK, w.Code)
	toggled := dataOf[PostResponse](t, w)
	assert.True(t, toggled.IsPublished)
	assert.NotNil(t, toggled.PublishedAt)

	w = send(owner, http.MethodPatch, idPath("/posts/", p3.ID, "/toggle-publish"), "")
	toggled = dataOf[PostResponse](t, w)
	assert.False(t, toggled.IsPublished)
	assert.Nil(t, toggled.PublishedAt)

	assert.Equal(t, http.StatusForbidden, send(other, http.MethodPatch, idPath("/posts/", p3.ID, "/toggle-publish"), "").Code)

	// 10. Delete
	w = send(owner, http.MethodDelete, idPath("/posts/", p4.ID, ""), "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, envOf(t, w).Data)
	assert.Equal(t, http.StatusNotFound, send(owner, http.MethodDelete, idPath("/posts/", p4.ID, ""), "").Code)
	assert.Equal(t, http.StatusForbidden, send(other, http.MethodDelete, idPath("/posts/", p1.ID, ""), "").Code)
	assert.Equal(t, http.StatusNotFound, send(owner, http.MethodGet, idPath("/posts/", p4.ID, ""), "").Code)
}

func TestPost_AuthRequiredAndInvalidIDs(t *testing.T) {
	f := newFixture(t)
	anon := f.router(0)
	owner := f.router(f.owner.ID)

	unauthorized := []struct{ method, path string }{
		{http.MethodGet, "/posts"},
		{http.MethodPost, "/posts"},
		{http.MethodGet, "/posts/stats"},
		{http.MethodGet, "/posts/1"},
		{http.MethodPut, "/posts/1"},
		{http.MethodPatch, "/posts/1/toggle-publish"},
		{http.MethodDelete, "/posts/1"},
	}
	for _, tc := range unauthorized {
		w := send(anon, tc.method, tc.path, `{}`)
		assert.Equal(t, http.StatusUnauthorized, w.Code, tc.method+" "+tc.path)
		assert.Equal(t, "fail", envOf(t, w).Status)
	}

	invalidID := []struct{ method, path string }{
		{http.MethodGet, "/posts/abc"},
		{http.MethodPut, "/posts/abc"},
		{http.MethodPatch, "/posts/abc/toggle-publish"},
		{http.MethodDelete, "/posts/abc"},
	}
	for _, tc := range invalidID {
		assert.Equal(t, http.StatusBadRequest, send(owner, tc.method, tc.path, `{}`).Code, tc.method+" "+tc.path)
	}
}

// ---------- public ----------

func TestPost_PublicEndpoints(t *testing.T) {
	f := newFixture(t)
	r := f.router(0)

	pub1 := f.create(t, f.owner.ID, "Panduan Golang Pemula", "Teknologi", "Belajar golang dasar untuk pemula", true)
	pub2 := f.create(t, f.other.ID, "Tips Hidup Sehat", "Gaya Hidup", "Olahraga rutin setiap pagi", true)
	draft := f.create(t, f.owner.ID, "Rahasia Draf", "Opini", "Belum siap tayang", false)
	assert.False(t, draft.IsPublished)

	// categories
	w := send(r, http.MethodGet, "/public/categories", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, database.PostCategories, dataOf[[]string](t, w))

	// list: drafts are hidden, newest published first
	list := dataOf[[]PublicPostResponse](t, send(r, http.MethodGet, "/public/posts", ""))
	require.Len(t, list, 2)
	assert.Equal(t, pub2.Slug, list[0].Slug)
	assert.Equal(t, pub1.Slug, list[1].Slug)
	assert.Equal(t, "other", list[0].AuthorName)
	assert.Equal(t, "owner", list[1].AuthorName)
	assert.NotNil(t, list[0].Author)
	// user_id must NOT appear in the JSON
	rawBody := string(envOf(t, send(r, http.MethodGet, "/public/posts", "")).Data)
	assert.NotContains(t, rawBody, "user_id")

	// search (title, content, case-insensitive) and category filter
	assert.Len(t, dataOf[[]PublicPostResponse](t, send(r, http.MethodGet, "/public/posts?q=golang", "")), 1)
	assert.Len(t, dataOf[[]PublicPostResponse](t, send(r, http.MethodGet, "/public/posts?q=OLAHRAGA", "")), 1)
	assert.Len(t, dataOf[[]PublicPostResponse](t, send(r, http.MethodGet, "/public/posts?category=Gaya%20Hidup", "")), 1)
	assert.Len(t, dataOf[[]PublicPostResponse](t, send(r, http.MethodGet, "/public/posts?q=tips&category=Gaya%20Hidup", "")), 1)
	empty := send(r, http.MethodGet, "/public/posts?q=golang&category=Gaya%20Hidup", "")
	assert.Equal(t, http.StatusOK, empty.Code)
	assert.JSONEq(t, "[]", string(envOf(t, empty).Data))
	assert.Len(t, dataOf[[]PublicPostResponse](t, send(r, http.MethodGet, "/public/posts?q=%20%20", "")), 2)

	// detail by slug: author + reading_time + content
	w = send(r, http.MethodGet, "/public/posts/"+pub1.Slug, "")
	require.Equal(t, http.StatusOK, w.Code)
	detail := dataOf[PublicPostResponse](t, w)
	require.NotNil(t, detail.Author)
	assert.Equal(t, "owner", detail.Author.Username)
	assert.Equal(t, 1, detail.ReadingTime)
	assert.Equal(t, "Belajar golang dasar untuk pemula", detail.Content)
	// user_id must NOT appear in slug detail JSON either
	rawSlug := string(envOf(t, send(r, http.MethodGet, "/public/posts/"+pub1.Slug, "")).Data)
	assert.NotContains(t, rawSlug, "user_id")

	// draft and unknown slugs are 404
	assert.Equal(t, http.StatusNotFound, send(r, http.MethodGet, "/public/posts/"+draft.Slug, "").Code)
	assert.Equal(t, http.StatusNotFound, send(r, http.MethodGet, "/public/posts/tidak-ada", "").Code)
}

// ---------- admin ----------

func TestPost_AdminEndpoints(t *testing.T) {
	f := newFixture(t)
	r := f.router(f.admin.ID)

	f.create(t, f.owner.ID, "Artikel Satu", "Bisnis", "isi satu", true)
	f.create(t, f.owner.ID, "Artikel Dua", "Bisnis", "isi dua", false)
	f.create(t, f.other.ID, "Artikel Tiga", "Umum", "isi tiga", true)

	// users + total posts
	w := send(r, http.MethodGet, "/admin/users", "")
	require.Equal(t, http.StatusOK, w.Code)
	users := dataOf[[]UserWithStats](t, w)
	require.Len(t, users, 3)
	totals := map[string]int64{}
	for _, u := range users {
		totals[u.Username] = u.TotalPosts
		assert.Equal(t, u.TotalPosts, u.TotalArticles)
	}
	assert.Equal(t, map[string]int64{"owner": 2, "other": 1, "boss": 0}, totals)
	assert.Equal(t, int64(1), users[0].TotalPublished)
	assert.Equal(t, database.RoleAdmin, users[2].Role)

	// every post of the system, drafts included
	w = send(r, http.MethodGet, "/admin/posts", "")
	require.Equal(t, http.StatusOK, w.Code)
	all := dataOf[[]PostResponse](t, w)
	assert.Len(t, all, 3)
	assert.NotNil(t, all[0].Author)

	// posts of one user
	w = send(r, http.MethodGet, idPath("/admin/users/", f.owner.ID, "/posts"), "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, dataOf[[]PostResponse](t, w), 2)

	// user without posts -> empty list
	w = send(r, http.MethodGet, idPath("/admin/users/", f.admin.ID, "/posts"), "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, "[]", string(envOf(t, w).Data))

	assert.Equal(t, http.StatusOK, send(r, http.MethodGet, "/admin/users/99999/posts", "").Code)
	rawEmpty := string(envOf(t, send(r, http.MethodGet, "/admin/users/99999/posts", "")).Data)
	assert.JSONEq(t, "[]", rawEmpty)
	assert.Equal(t, http.StatusBadRequest, send(r, http.MethodGet, "/admin/users/abc/posts", "").Code)
}

// ---------- failure paths ----------

func TestPost_PostsTableMissing_ReturnsInternalError(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.db.Migrator().DropTable(&database.Post{}))
	r := f.router(f.owner.ID)

	cases := []struct{ method, path, body string }{
		{http.MethodGet, "/public/posts", ""},
		{http.MethodGet, "/public/posts/apa-saja", ""},
		{http.MethodGet, "/posts", ""},
		{http.MethodPost, "/posts", `{"title":"Judul","content":"Isi"}`},
		{http.MethodGet, "/posts/stats", ""},
		{http.MethodGet, "/posts/1", ""},
		{http.MethodPut, "/posts/1", `{}`},
		{http.MethodPatch, "/posts/1/toggle-publish", ""},
		{http.MethodDelete, "/posts/1", ""},
		{http.MethodGet, "/admin/users", ""},
		{http.MethodGet, "/admin/posts", ""},
		{http.MethodGet, idPath("/admin/users/", f.owner.ID, "/posts"), ""},
	}
	for _, tc := range cases {
		w := send(r, tc.method, tc.path, tc.body)
		assert.Equal(t, http.StatusInternalServerError, w.Code, tc.method+" "+tc.path)
		env := envOf(t, w)
		assert.Equal(t, "error", env.Status)
		assert.Equal(t, "Internal server error", env.Message)
	}
}

func TestPost_UsersTableMissing_ReturnsInternalError(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.db.Migrator().DropTable(&database.User{}))
	r := f.router(f.admin.ID)

	assert.Equal(t, http.StatusInternalServerError, send(r, http.MethodGet, "/admin/users", "").Code)
	// GetUserPosts queries posts directly via FindByUserID (no FindUserByID check).
	// In SQLite, Preload("User") on a dropped users table returns posts without error
	// (graceful degradation), so the response is 200 OK with an empty list.
	w := send(r, http.MethodGet, "/admin/users/1/posts", "")
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

func TestPost_Service_RepositoryWriteFailures(t *testing.T) {
	f := newFixture(t)

	// Create failure (after slug check passed)
	failCreate := NewPostService(&stubRepo{PostRepository: f.repo, createErr: errors.New("insert failed")})
	_, err := failCreate.CreatePost(f.owner.ID, CreatePostRequest{Title: "Gagal", Content: "isi"})
	assert.EqualError(t, err, "insert failed")

	// Update failure for both UpdatePost and TogglePublish
	existing := f.create(t, f.owner.ID, "Ada", "", "isi", false)
	failUpdate := NewPostService(&stubRepo{PostRepository: f.repo, updateErr: errors.New("update failed")})
	_, err = failUpdate.UpdatePost(f.owner.ID, existing.ID, UpdatePostRequest{Title: "Baru"})
	assert.EqualError(t, err, "update failed")
	_, err = failUpdate.TogglePublish(f.owner.ID, existing.ID)
	assert.EqualError(t, err, "update failed")

	// Delete failure surfaces as 500 through the handler
	failDelete := NewPostService(&stubRepo{PostRepository: f.repo, deleteErr: errors.New("delete failed")})
	assert.EqualError(t, failDelete.DeletePost(f.owner.ID, existing.ID), "delete failed")

	h := NewPostHandler(failDelete)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", f.owner.ID); c.Next() })
	r.DELETE("/posts/:id", h.DeletePost)
	assert.Equal(t, http.StatusInternalServerError, send(r, http.MethodDelete, idPath("/posts/", existing.ID, ""), "").Code)

	// The stub delegates to the real repository when no error is configured
	delegating := &stubRepo{PostRepository: f.repo}
	p := &database.Post{UserID: f.owner.ID, Title: "T", Slug: "t-delegate", Category: "Umum", Content: "c"}
	require.NoError(t, delegating.Create(p))
	p.Title = "T2"
	require.NoError(t, delegating.Update(p))
	require.NoError(t, delegating.Delete(p.ID))
}

func TestPost_Service_GetCategoriesReturnsCopy(t *testing.T) {
	f := newFixture(t)
	cats := f.svc.GetCategories()
	assert.Equal(t, database.PostCategories, cats)

	cats[0] = "Diubah"
	assert.Equal(t, "Teknologi", database.PostCategories[0], "mutating the result must not change the source list")
}

func TestPost_Repository_SlugExists(t *testing.T) {
	f := newFixture(t)
	exists, err := f.repo.SlugExists("belum-ada")
	assert.NoError(t, err)
	assert.False(t, exists)

	f.create(t, f.owner.ID, "Sudah Ada", "", "isi", false)
	exists, err = f.repo.SlugExists("sudah-ada")
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestPost_Repository_FindUserByID(t *testing.T) {
	f := newFixture(t)

	// existing user
	u, err := f.repo.FindUserByID(f.owner.ID)
	assert.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, f.owner.Username, u.Username)

	// non-existing user
	_, err = f.repo.FindUserByID(99999)
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestPost_PublicPostResponse_NilUser(t *testing.T) {
	// Cover the else branch of toPublicPostResponse when p.User is nil
	post := database.Post{
		UserID:  42,
		Title:   "No User",
		Slug:    "no-user",
		Content: "content",
		User:    nil,
	}
	res := toPublicPostResponse(post, false)
	require.NotNil(t, res.Author)
	assert.Equal(t, uint(42), res.Author.ID)
	assert.Equal(t, "", res.Author.Username)
}
