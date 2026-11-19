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

// StatsResponse powers the three statistic cards of the author dashboard.
type StatsResponse struct {
	Total       int64 `json:"total"`
	TotalTerbit int64 `json:"total_terbit"`
	Draft       int64 `json:"draft"`
}

type UserWithStats struct {
	ID         uint      `json:"id"`
	Username   string    `json:"username"`
	Email      string    `json:"email"`
	Role       string    `json:"role"`
	CreatedAt  time.Time `json:"created_at"`
	TotalPosts int64     `json:"total_posts"`
}
