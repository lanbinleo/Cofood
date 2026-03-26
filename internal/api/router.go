package api

import (
	"net/http"

	"cofood/internal/config"
	"cofood/internal/ratelimit"
	"cofood/internal/search"

	"github.com/gin-gonic/gin"
)

type Router struct {
	cfg       config.Config
	searchSvc *search.Service
}

func NewRouter(cfg config.Config, searchSvc *search.Service) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := &Router{
		cfg:       cfg,
		searchSvc: searchSvc,
	}

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(ratelimit.New().Handler())

	engine.GET("/healthz", router.healthz)

	// Serve frontend
	engine.StaticFile("/", "./web/index.html")
	engine.StaticFile("/index.html", "./web/index.html")

	v1 := engine.Group("/api/v1")
	v1.GET("/search/name", router.searchByName)
	v1.GET("/search/vector", router.searchByVector)

	return engine
}

func (r *Router) healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

func (r *Router) searchByName(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing query parameter: q"})
		return
	}

	result, err := r.searchSvc.SearchByName(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (r *Router) searchByVector(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing query parameter: q"})
		return
	}

	result, err := r.searchSvc.SearchByVector(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}
