package post

import "time"

type CreatePostRequest struct {
	Title          string `json:"title" binding:"required,max=200"`
	Category       string `json:"category"`
	Excerpt        string `json:"excerpt"`
	Content        string `json:"content" binding:"required"`
	CoverImagePath string `json:"cover_image_path"`
	IsPublished    bool   `json:"is_published"`
}

// UpdatePostRequest is a partial update: empty fields keep their current value.
// cover_image_path accepts "DELETE" or "remove" to clear the cover image.
type UpdatePostRequest struct {
	Title          string `json:"title" binding:"max=200"`
	Category       string `json:"category"`
	Excerpt        string `json:"excerpt"`
	Content        string `json:"content"`
	CoverImagePath string `json:"cover_image_path"`
	IsPublished    *bool  `json:"is_published"`
}

type AuthorResponse struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
}

type PostResponse struct {
	ID             uint            `json:"id"`
	UserID         uint            `json:"user_id"`
	Title          string          `json:"title"`
	Slug           string          `json:"slug"`
	Category       string          `json:"category"`
	Excerpt        string          `json:"excerpt"`
	Content        string          `json:"content,omitempty"`
	CoverImagePath string          `json:"cover_image_path,omitempty"`
	IsPublished    bool            `json:"is_published"`
	PublishedAt    *time.Time      `json:"published_at,omitempty"`
	ReadingTime    int             `json:"reading_time"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Author         *AuthorResponse `json:"author,omitempty"`
}

type PublicPostResponse struct {
	ID             uint            `json:"id"`
	UserID         uint            `json:"-"`
	Title          string          `json:"title"`
	Slug           string          `json:"slug"`
	Category       string          `json:"category"`
	Excerpt        string          `json:"excerpt"`
	Content        string          `json:"content,omitempty"`
	CoverImagePath string          `json:"cover_image_path,omitempty"`
	IsPublished    bool            `json:"is_published"`
	PublishedAt    *time.Time      `json:"published_at,omitempty"`
	ReadingTime    int             `json:"reading_time"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Author         *AuthorResponse `json:"author"`
}

// StatsResponse powers the statistics for the author dashboard.
type StatsResponse struct {
	TotalArticles int64 `json:"total_articles"`
	TotalSelesai  int64 `json:"total_selesai"`
	BelumSelesai  int64 `json:"belum_selesai"`
}

type UserWithStats struct {
	ID         uint      `json:"id"`
	Username   string    `json:"username"`
	Email      string    `json:"email"`
	Role       string    `json:"role"`
	CreatedAt  time.Time `json:"created_at"`
	TotalPosts int64     `json:"total_posts"`
}
