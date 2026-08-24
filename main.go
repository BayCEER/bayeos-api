package main

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "BayEOS API"})
	})

	// postgresURL, exists := os.LookupEnv("POSTGRES_DB_URL")
	// if !exists {
	//     panic("POSTGRES_DB_URL is required")
	// }

	port, exists := os.LookupEnv("BAYEOS_API_PORT")
	if !exists {
		panic("BAYEOS_API_PORT is required")
	}

	router.Run(":" + port)
}
