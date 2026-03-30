package services

import "github.com/gin-gonic/gin"

type User struct {
	ID   string
	Name string
}

type Item struct {
	ID   string
	Name string
}

func GetAllUsers() []User {
	return []User{{ID: "1", Name: "Alice"}, {ID: "2", Name: "Bob"}}
}

func CreateUser(c *gin.Context) error {
	return nil
}

func GetUserByID(id string) User {
	return User{ID: id, Name: "Alice"}
}

func DeleteUserByID(id string) {
}

func GetAllItems() []Item {
	return []Item{{ID: "1", Name: "Widget"}}
}
