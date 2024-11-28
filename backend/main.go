package main

import (
	"backend/api"
	"backend/internal/db"
	"fmt"
	"log"
)

func main() {
	fmt.Println("Hello, World!")
	db.Init_DB()
	router := api.SetupRouter()
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
