package post

import (
	"fmt"
	"strings"
	"time"

	"app/internal/database"
)

const (
	wordsPerMinute   = 200
	maxSlugLength    = 80
	maxExcerptLength = 160
)

type PostService interface {
	// Public
	GetCategories() []string
	GetPublicPosts(q, category string) ([]PublicPostResponse, error)
	GetPublicPostBySlug(slug string) (*PublicPostResponse, error)

	// Author
	GetMyPosts(userID uint) ([]PostResponse, error)
	GetMyPost(userID, id uint) (*PostResponse, error)
	CreatePost(userID uint, req CreatePostRequest) (*PostResponse, error)
	UpdatePost(userID, id uint, req UpdatePostRequest) (*PostResponse, error)
	TogglePublish(userID, id uint) (*PostResponse, error)
	DeletePost(userID, id uint) error
	GetStats(userID uint) (*StatsResponse, error)

	// Admin
	GetAllUsers() ([]UserWithStats, error)
	GetAllPosts() ([]PostResponse, error)
	GetUserPosts(targetUserID uint) ([]PostResponse, error)
}

type postService struct {
	repo PostRepository
}

func NewPostService(repo PostRepository) PostService {
	return &postService{repo: repo}
}

// CalculateReadingTime returns total words / 200 wpm, with a minimum of 1 minute.
func CalculateReadingTime(content string) int {
	minutes := len(strings.Fields(content)) / wordsPerMinute
	if minutes < 1 {
		return 1
	}
	return minutes
}

// slugify builds a lowercase, ASCII, dash-separated slug ("post" when nothing usable remains).
func slugify(title string) string {
	var b strings.Builder
	pendingDash := false
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if pendingDash && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingDash = false
			b.WriteRune(r)
		} else {
			pendingDash = true
		}
	}

	slug := b.String()
	if slug == "" {
		return "post"
	}
	if len(slug) > maxSlugLength {
		slug = strings.TrimRight(slug[:maxSlugLength], "-")
	}
	return slug
}

// makeExcerpt derives a short plain-text excerpt from the content.
func makeExcerpt(content string) string {
	text := strings.Join(strings.Fields(content), " ")
	runes := []rune(text)
	if len(runes) <= maxExcerptLength {
		return text
	}
	return strings.TrimSpace(string(runes[:maxExcerptLength])) + "…"
}

func normalizeCategory(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return database.DefaultCategory, nil
	}
	for _, c := range database.PostCategories {
		if strings.EqualFold(c, input) {
			return c, nil
		}
	}
	return "", ErrInvalidCategory
}

// applyPublish switches the publication state and keeps published_at consistent.
func applyPublish(p *database.Post, publish bool) {
	if publish && !p.IsPublished {
		now := time.Now()
		p.PublishedAt = &now
	}
	if !publish {
		p.PublishedAt = nil
	}
	p.IsPublished = publish
}

func toPostResponse(p database.Post, withContent bool) PostResponse {
	res := PostResponse{
		ID:             p.ID,
		UserID:         p.UserID,
		Title:          p.Title,
		Slug:           p.Slug,
		Category:       p.Category,
		Excerpt:        p.Excerpt,
		CoverImagePath: p.CoverImagePath,
		IsPublished:    p.IsPublished,
		PublishedAt:    p.PublishedAt,
		ReadingTime:    CalculateReadingTime(p.Content),
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
	if withContent {
		res.Content = p.Content
	}
	if p.User != nil {
		res.Author = &AuthorResponse{ID: p.User.ID, Username: p.User.Username}
	}
	return res
}

// toPostResponses always returns a non-nil slice so that JSON encodes "[]", not "null".
func toPostResponses(posts []database.Post) []PostResponse {
	res := make([]PostResponse, 0, len(posts))
	for _, p := range posts {
		res = append(res, toPostResponse(p, false))
	}
	return res
}

func toPublicPostResponse(p database.Post, withContent bool) PublicPostResponse {
	res := PublicPostResponse{
		ID:             p.ID,
		UserID:         p.UserID,
		Title:          p.Title,
		Slug:           p.Slug,
		Category:       p.Category,
		Excerpt:        p.Excerpt,
		CoverImagePath: p.CoverImagePath,
		IsPublished:    p.IsPublished,
		PublishedAt:    p.PublishedAt,
		ReadingTime:    CalculateReadingTime(p.Content),
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
	if withContent {
		res.Content = p.Content
	}
	if p.User != nil {
		res.Author = &AuthorResponse{ID: p.User.ID, Username: p.User.Username}
	} else {
		res.Author = &AuthorResponse{ID: p.UserID, Username: ""}
	}
	return res
}

func toPublicPostResponses(posts []database.Post) []PublicPostResponse {
	res := make([]PublicPostResponse, 0, len(posts))
	for _, p := range posts {
		res = append(res, toPublicPostResponse(p, false))
	}
	return res
}

func (s *postService) uniqueSlug(title string) (string, error) {
	base := slugify(title)
	slug := base
	for i := 2; ; i++ {
		exists, err := s.repo.SlugExists(slug)
		if err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

// findOwned loads a post by ID; if not in DB -> ErrNotFound, if owned by another user -> ErrForbidden.
func (s *postService) findOwned(userID, id uint) (*database.Post, error) {
	p, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if p.UserID != userID {
		return nil, ErrForbidden
	}
	return p, nil
}

func (s *postService) save(p *database.Post) (*PostResponse, error) {
	if err := s.repo.Update(p); err != nil {
		return nil, err
	}
	res := toPostResponse(*p, true)
	return &res, nil
}

func (s *postService) GetCategories() []string {
	return append([]string{}, database.PostCategories...)
}

func (s *postService) GetPublicPosts(q, category string) ([]PublicPostResponse, error) {
	posts, err := s.repo.FindPublished(strings.TrimSpace(q), strings.TrimSpace(category))
	if err != nil {
		return nil, err
	}
	return toPublicPostResponses(posts), nil
}

func (s *postService) GetPublicPostBySlug(slug string) (*PublicPostResponse, error) {
	p, err := s.repo.FindPublishedBySlug(slug)
	if err != nil {
		return nil, err
	}
	res := toPublicPostResponse(*p, true)
	return &res, nil
}

func (s *postService) GetMyPosts(userID uint) ([]PostResponse, error) {
	posts, err := s.repo.FindByUserID(userID)
	if err != nil {
		return nil, err
	}
	return toPostResponses(posts), nil
}

func (s *postService) GetMyPost(userID, id uint) (*PostResponse, error) {
	p, err := s.findOwned(userID, id)
	if err != nil {
		return nil, err
	}
	res := toPostResponse(*p, true)
	return &res, nil
}

func (s *postService) CreatePost(userID uint, req CreatePostRequest) (*PostResponse, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" || strings.TrimSpace(req.Content) == "" {
		return nil, ErrInvalidInput
	}

	category, err := normalizeCategory(req.Category)
	if err != nil {
		return nil, err
	}

	slug, err := s.uniqueSlug(title)
	if err != nil {
		return nil, err
	}

	excerpt := strings.TrimSpace(req.Excerpt)
	if excerpt == "" {
		excerpt = makeExcerpt(req.Content)
	}

	p := &database.Post{
		UserID:         userID,
		Title:          title,
		Slug:           slug,
		Category:       category,
		Excerpt:        excerpt,
		Content:        req.Content,
		CoverImagePath: req.CoverImagePath,
	}
	applyPublish(p, req.IsPublished)

	if err := s.repo.Create(p); err != nil {
		return nil, err
	}

	res := toPostResponse(*p, true)
	return &res, nil
}

func (s *postService) UpdatePost(userID, id uint, req UpdatePostRequest) (*PostResponse, error) {
	p, err := s.findOwned(userID, id)
	if err != nil {
		return nil, err
	}

	if title := strings.TrimSpace(req.Title); title != "" {
		p.Title = title
	}
	if strings.TrimSpace(req.Category) != "" {
		category, err := normalizeCategory(req.Category)
		if err != nil {
			return nil, err
		}
		p.Category = category
	}
	if req.Excerpt != "" {
		p.Excerpt = req.Excerpt
	}
	if strings.TrimSpace(req.Content) != "" {
		p.Content = req.Content
	}
	if req.CoverImagePath != "" {
		if req.CoverImagePath == "DELETE" || req.CoverImagePath == "remove" {
			p.CoverImagePath = ""
		} else {
			p.CoverImagePath = req.CoverImagePath
		}
	}
	if req.IsPublished != nil {
		applyPublish(p, *req.IsPublished)
	}

	return s.save(p)
}

func (s *postService) TogglePublish(userID, id uint) (*PostResponse, error) {
	p, err := s.findOwned(userID, id)
	if err != nil {
		return nil, err
	}
	applyPublish(p, !p.IsPublished)
	return s.save(p)
}

func (s *postService) DeletePost(userID, id uint) error {
	p, err := s.findOwned(userID, id)
	if err != nil {
		return err
	}
	return s.repo.Delete(p.ID)
}

func (s *postService) GetStats(userID uint) (*StatsResponse, error) {
	total, published, err := s.repo.CountStats(userID)
	if err != nil {
		return nil, err
	}
	return &StatsResponse{
		TotalArticles: total,
		TotalSelesai:  published,
		BelumSelesai:  total - published,
	}, nil
}

func (s *postService) GetAllUsers() ([]UserWithStats, error) {
	users, err := s.repo.FindAllUsers()
	if err != nil {
		return nil, err
	}
	counts, err := s.repo.CountPostsPerUser()
	if err != nil {
		return nil, err
	}

	results := make([]UserWithStats, 0, len(users))
	for _, u := range users {
		results = append(results, UserWithStats{
			ID:         u.ID,
			Username:   u.Username,
			Email:      u.Email,
			Role:       u.Role,
			CreatedAt:  u.CreatedAt,
			TotalPosts: counts[u.ID],
		})
	}
	return results, nil
}

func (s *postService) GetAllPosts() ([]PostResponse, error) {
	posts, err := s.repo.FindAll()
	if err != nil {
		return nil, err
	}
	return toPostResponses(posts), nil
}

func (s *postService) GetUserPosts(targetUserID uint) ([]PostResponse, error) {
	posts, err := s.repo.FindByUserID(targetUserID)
	if err != nil {
		return nil, err
	}
	return toPostResponses(posts), nil
}
