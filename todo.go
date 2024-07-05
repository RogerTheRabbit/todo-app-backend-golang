package main

// https://go.dev/doc/tutorial/database-access

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

// album represents data about a record album.
type Todo struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

var dbpool *pgxpool.Pool
var LOG *log.Logger

func main() {

	dotEnvErr := godotenv.Load()

	LOG = log.Default()

	if dotEnvErr != nil {
		LOG.Println("Did not load .env file")
	}

	var err error
	dbpool, err = pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		LOG.Fatalf("Unable to create database connection pool: %v\n", err)
	}
	defer dbpool.Close()
	LOG.Println("Pininging Database")
	err = dbpool.Ping(context.Background())
	if err != nil {
		LOG.Fatalln(err)
	}
	LOG.Println("Database ok!")

	router := gin.Default()
	router.SetTrustedProxies(nil)
	config := cors.DefaultConfig()
	config.AllowOrigins = strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",")

	router.Use(cors.New(config))

	router.GET("/todos", getTodos)
	router.POST("/todos", createTodo)
	router.DELETE("/todos/:id", deleteTodo)

	router.Run(fmt.Sprintf("%s:%s", os.Getenv("SERVER_ADDRESS"), os.Getenv("SERVER_PORT")))
}

func getTodos(context *gin.Context) {
	todos, err := getAllTodosDB()
	if err != nil {
		LOG.Panic(err)
	}
	context.JSON(http.StatusOK, todos)
}

func createTodo(context *gin.Context) {
	var newTodo Todo

	if err := context.BindJSON(&newTodo); err != nil {
		return
	}

	createdTodo, err := createTodoDB(newTodo)
	if err != nil {
		LOG.Panic(err)
	}
	context.IndentedJSON(http.StatusCreated, createdTodo)
}

func deleteTodo(context *gin.Context) {
	id := context.Param("id")
	LOG.Printf("GOT DELETE REQUEST FOR: %s", id)

	if err := deleteTodoDB(id); err != nil {
		LOG.Panic(err)
	}
}

func getAllTodosDB() ([]Todo, error) {
	var todos []Todo

	rows, err := dbpool.Query(context.Background(), "SELECT * FROM todo")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var todo Todo
		if err := rows.Scan(&todo.ID, &todo.Title, &todo.Description); err != nil {
			return nil, fmt.Errorf("failed to extract todo from database query")
		}
		todos = append(todos, todo)
	}
	return todos, nil
}

func createTodoDB(todo Todo) (Todo, error) {
	var createdTodo Todo
	rows, err := dbpool.Query(context.Background(), "INSERT INTO todo (title, description) VALUES($1, $2)", todo.Title, todo.Description)
	if err != nil {
		return createdTodo, err
	}
	defer rows.Close()

	return createdTodo, nil
}

func deleteTodoDB(id string) error {
	rows, err := dbpool.Query(context.Background(), "DELETE FROM todo WHERE id=$1", id)
	if err != nil {
		return err
	}
	defer rows.Close()
	return nil
}
