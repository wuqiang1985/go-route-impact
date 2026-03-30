package controllers

import (
	"example.com/gin-demo/services"
	"github.com/gin-gonic/gin"
)

func Ping(c *gin.Context) {
	c.JSON(200, gin.H{"message": "pong"})
}

func ListUsers(c *gin.Context) {
	users := services.GetAllUsers()
	c.JSON(200, users)
}

func CreateUser(c *gin.Context) {
	err := services.CreateUser(c)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"status": "created"})
}

func GetUser(c *gin.Context) {
	id := c.Param("id")
	user := services.GetUserByID(id)
	c.JSON(200, user)
}

func DeleteUser(c *gin.Context) {
	id := c.Param("id")
	services.DeleteUserByID(id)
	c.JSON(204, nil)
}

func ListItems(c *gin.Context) {
	items := services.GetAllItems()
	c.JSON(200, items)
}
