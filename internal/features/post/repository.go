package post

import (
	"errors"
	"strings"

	"app/internal/database"
	"gorm.io/gorm"
)

var (
	ErrNotFound        = errors.New("post not found")
	ErrForbidden       = errors.New("Forbidden: bukan milik Anda")
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidCategory = errors.New("invalid category")
	ErrInvalidInput    = errors.New("title and content must not be blank")
)

type PostRepository interface {
	// Public
	FindPublished(q, category string) ([]database.Post, error)
	FindPublishedBySlug(slug string) (*database.Post, error)

	// Author
	FindByUserID(userID uint) ([]database.Post, error)
	FindByID(id uint) (*database.Post, error)
	SlugExists(slug string) (bool, error)
	Create(post *database.Post) error
	Update(post *database.Post) error
	Delete(id uint) error
	CountStats(userID uint) (total int64, published int64, err error)

	// Admin
	FindAll() ([]database.Post, error)
	FindAllUsers() ([]database.User, error)
	FindUserByID(id uint) (*database.User, error)
	CountPostsPerUser() (map[uint]int64, error)
}

type postRepository struct {
	db *gorm.DB
}

func NewPostRepository(db *gorm.DB) PostRepository {
	return &postRepository{db: db}
}

func mapNotFound(err, notFound error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound
	}
	return err
}

func (r *postRepository) FindPublished(q, category string) ([]database.Post, error) {
	query := r.db.Preload("User").Where("is_published = ?", true)
	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		query = query.Where("(LOWER(title) LIKE ? OR LOWER(excerpt) LIKE ? OR LOWER(content) LIKE ?)", like, like, like)
	}
	if category != "" {
		query = query.Where("category = ?", category)
	}

	var posts []database.Post
	err := query.Order("published_at desc, id desc").Find(&posts).Error
	return posts, err
}

func (r *postRepository) FindPublishedBySlug(slug string) (*database.Post, error) {
	var post database.Post
	err := r.db.Preload("User").Where("slug = ? AND is_published = ?", slug, true).First(&post).Error
	if err != nil {
		return nil, mapNotFound(err, ErrNotFound)
	}
	return &post, nil
}

func (r *postRepository) FindByUserID(userID uint) ([]database.Post, error) {
	var posts []database.Post
	err := r.db.Preload("User").Where("user_id = ?", userID).Order("created_at desc, id desc").Find(&posts).Error
	return posts, err
}

func (r *postRepository) FindByID(id uint) (*database.Post, error) {
	var post database.Post
	err := r.db.Preload("User").First(&post, id).Error
	if err != nil {
		return nil, mapNotFound(err, ErrNotFound)
	}
	return &post, nil
}

func (r *postRepository) SlugExists(slug string) (bool, error) {
	var count int64
	err := r.db.Model(&database.Post{}).Where("slug = ?", slug).Count(&count).Error
	return count > 0, err
}

func (r *postRepository) Create(post *database.Post) error {
	return r.db.Create(post).Error
}

func (r *postRepository) Update(post *database.Post) error {
	return r.db.Omit("User").Save(post).Error
}

func (r *postRepository) Delete(id uint) error {
	return r.db.Delete(&database.Post{}, id).Error
}

type statsRow struct {
	Total     int64
	Published int64
}

func (r *postRepository) CountStats(userID uint) (int64, int64, error) {
	var row statsRow
	err := r.db.Model(&database.Post{}).
		Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN is_published = ? THEN 1 ELSE 0 END), 0) AS published", true).
		Where("user_id = ?", userID).
		Scan(&row).Error
	return row.Total, row.Published, err
}

func (r *postRepository) FindAll() ([]database.Post, error) {
	var posts []database.Post
	err := r.db.Preload("User").Order("created_at desc, id desc").Find(&posts).Error
	return posts, err
}

func (r *postRepository) FindAllUsers() ([]database.User, error) {
	var users []database.User
	err := r.db.Order("id asc").Find(&users).Error
	return users, err
}

func (r *postRepository) FindUserByID(id uint) (*database.User, error) {
	var user database.User
	err := r.db.First(&user, id).Error
	if err != nil {
		return nil, mapNotFound(err, ErrUserNotFound)
	}
	return &user, nil
}

type userPostCount struct {
	UserID uint
	Total  int64
}

func (r *postRepository) CountPostsPerUser() (map[uint]int64, error) {
	var rows []userPostCount
	err := r.db.Model(&database.Post{}).
		Select("user_id, COUNT(*) AS total").
		Group("user_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	counts := make(map[uint]int64, len(rows))
	for _, row := range rows {
		counts[row.UserID] = row.Total
	}
	return counts, nil
}
