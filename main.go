package main

import (
	"log"
	"os"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/basit/fileshare-backend/auth/middleware"
	"github.com/basit/fileshare-backend/graph"
	"github.com/basit/fileshare-backend/graph/resolvers"
	"github.com/basit/fileshare-backend/initializers"
	"github.com/basit/fileshare-backend/jobs"
	"github.com/basit/fileshare-backend/routes"
)

const defaultPort = "8080"

func main() {
	initializers.ConnectToDatabase()
	initializers.InitAWS()
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	srv := handler.New(graph.NewExecutableSchema(graph.Config{
		Resolvers: &resolvers.Resolver{},
	}))

	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})
	// Start cleanup job
	jobs.StartCleanupJob()

	router := gin.Default()
	// Add CORS middleware before other middleware
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	// Global middleware
	router.Use(
		middleware.RateLimitMiddleware(),
	)

	routes.RegisterFileRoutes(router)

	router.GET("/", func(c *gin.Context) {
		playground.Handler("GraphQL playground", "/query").ServeHTTP(c.Writer, c.Request)
	})

	router.POST("/graphql",
		middleware.AuthOptional(),
		middleware.GinContextToContextMiddleware(),
		func(c *gin.Context) {
			srv.ServeHTTP(c.Writer, c.Request)
		},
	)

	log.Printf("connect to http://localhost:%s/ for GraphQL playground", port)
	log.Fatal(router.Run(":" + port))
}
