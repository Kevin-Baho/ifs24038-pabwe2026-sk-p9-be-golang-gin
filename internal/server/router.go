package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	"app/docs"
	"app/internal/config"
	"app/internal/features/ai"
	"app/internal/features/auth"
	"app/internal/features/post"
	"app/internal/middleware"
	"app/internal/response"
)

func SetupRouter(cfg *config.Config, db *gorm.DB) *gin.Engine {
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.CORS())
	r.Use(middleware.RateLimiter(1000, time.Second))

	// Repositories, services & handlers
	userRepo := auth.NewUserRepository(db)
	authSvc := auth.NewAuthService(userRepo, cfg.AppKey)
	authHdr := auth.NewAuthHandler(authSvc)

	postRepo := post.NewPostRepository(db)
	postSvc := post.NewPostService(postRepo)
	postHdr := post.NewPostHandler(postSvc)

	searchClient := ai.NewSearchEngineClient(cfg.SearchEngineEndpoint)
	aiClient := ai.NewOpenAiClient(cfg.OpenAIEndpoint, cfg.OpenAIKey, cfg.OpenAIModel, cfg.AITimeout)
	aiSvc := ai.NewAiService(aiClient, searchClient)
	aiHdr := ai.NewAiHandler(aiSvc)

	// Dynamic OpenAPI JSON adjusted to the current request URL
	getSwaggerData := func(c *gin.Context) []byte {
		host := c.GetHeader("X-Forwarded-Host")
		if host == "" {
			host = c.Request.Host
		}

		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}

		return docs.GetSwaggerJSON(host, scheme)
	}

	r.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})

	swaggerHandler := ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/swagger/api.json"))
	r.GET("/swagger/*any", func(c *gin.Context) {
		anyParam := c.Param("any")
		if anyParam == "" || anyParam == "/" {
			c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
			return
		}
		if anyParam == "/api.json" || anyParam == "/doc.json" {
			c.Data(http.StatusOK, "application/json; charset=utf-8", getSwaggerData(c))
			return
		}
		swaggerHandler(c)
	})

	r.GET("/api.json", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json; charset=utf-8", getSwaggerData(c))
	})

	api := r.Group("/api")
	{
		// Public: categories & published articles
		public := api.Group("/public")
		{
			public.GET("/categories", postHdr.GetCategories)
			public.GET("/posts", postHdr.GetPublicPosts)
			public.GET("/posts/:slug", postHdr.GetPublicPostBySlug)
		}

		// Auth: register & login are public
		authRoutes := api.Group("/auth")
		{
			authRoutes.POST("/register", authHdr.Register)
			authRoutes.POST("/login", authHdr.Login)

			authProtected := authRoutes.Group("")
			authProtected.Use(middleware.Auth(cfg.AppKey))
			{
				authProtected.POST("/logout", authHdr.Logout)
				authProtected.GET("/profile", authHdr.GetProfile)
				authProtected.PUT("/profile", authHdr.UpdateProfile)
			}
		}

		// Protected (JWT)
		protected := api.Group("")
		protected.Use(middleware.Auth(cfg.AppKey))
		{
			protected.GET("/posts", postHdr.GetMyPosts)
			protected.POST("/posts", postHdr.CreatePost)
			protected.GET("/posts/stats", postHdr.GetStats)
			protected.POST("/posts/ai-generate", aiHdr.GeneratePost)
			protected.GET("/posts/:id", postHdr.GetPostByID)
			protected.PUT("/posts/:id", postHdr.UpdatePost)
			protected.PATCH("/posts/:id/toggle-publish", postHdr.TogglePublish)
			protected.DELETE("/posts/:id", postHdr.DeletePost)

			// Admin only (role guard)
			adm := protected.Group("/admin")
			adm.Use(middleware.RequireRole("admin"))
			{
				adm.GET("/users", postHdr.AdminGetUsers)
				adm.GET("/posts", postHdr.AdminGetPosts)
				adm.GET("/users/:id/posts", postHdr.AdminGetUserPosts)
			}
		}
	}

	// Pure REST API 404 handler for unmatched routes
	r.NoRoute(func(c *gin.Context) {
		response.Fail(c, http.StatusNotFound, "Endpoint not found")
	})

	return r
}
