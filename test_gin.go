package main

import (
	"github.com/gin-gonic/gin"
	"log"
)

func main() {
	r := gin.Default()
	r.StaticFile("/test", "/does/not/exist.bin")
	log.Println("Gin static file setup passed")
}
