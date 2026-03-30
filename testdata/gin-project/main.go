package main

import (
	"example.com/gin-demo/controllers"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	r.GET("/ping", controllers.Ping)

	v1 := r.Group("/api/v1")
	v1.GET("/users", controllers.ListUsers)
	v1.POST("/users", controllers.CreateUser)
	v1.GET("/users/:id", controllers.GetUser)
	v1.DELETE("/users/:id", controllers.DeleteUser)

	v2 := r.Group("/api/v2")
	v2.GET("/items", controllers.ListItems)

	r.Run(":8080")
}
