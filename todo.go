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

type Todo struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Username    *string `json:"username"`
}

type Reminder struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

var dbpool *pgxpool.Pool
var LOG *log.Logger

func main() {

	dotEnvErr := godotenv.Load("stack.env")

	LOG = log.Default()

	if dotEnvErr != nil {
		LOG.Println("Did not load .env file")
	}

	postgresAddress := fmt.Sprintf("postgres://%s:%s@%s", os.Getenv("POSTGRES_USERNAME"), os.Getenv("POSTGRES_PASSWORD"), os.Getenv("POSTGRES_ADDRESS"))

	var err error
	dbpool, err = pgxpool.New(context.Background(), postgresAddress)
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
	router.PUT("/todos", updateTodo)

	router.POST("/reminders", createReminder)

	router.Run(fmt.Sprintf("%s:%s", os.Getenv("SERVER_ADDRESS"), os.Getenv("SERVER_PORT")))
}

func getTodos(context *gin.Context) {
	todos, err := getAllTodosDB()
	if err != nil {
		LOG.Panic(err)
	}
	if todos == nil {
		todos = []Todo{}
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

	deltedCount, err := deleteTodoDB(id)
	if err != nil {
		LOG.Panic(err)
	}

	context.IndentedJSON(http.StatusOK, deltedCount)
}

func updateTodo(context *gin.Context) {
	var updatedTodo Todo

	if err := context.BindJSON(&updatedTodo); err != nil {
		LOG.Panic(err)
	}
	LOG.Printf("GOT PUT REQUEST FOR: %d", updatedTodo.ID)

	if err := updateTodoDB(updatedTodo); err != nil {
		LOG.Panic(err)
	}
}

func createReminder(context *gin.Context) {
	var newReminder Reminder

	if err := context.BindJSON(&newReminder); err != nil {
		return
	}

	createdReminder, err := createReminderDB(newReminder)
	if err != nil {
		LOG.Panic(err)
	}
	context.IndentedJSON(http.StatusCreated, createdReminder)
}

func getAllTodosDB() ([]Todo, error) {
	var todos []Todo

	rows, err := dbpool.Query(context.Background(), "SELECT todo.id, todo.title, todo.description, reminders.username FROM todo LEFT JOIN reminders ON todo.id=reminders.todo_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var todo Todo
		if err := rows.Scan(&todo.ID, &todo.Title, &todo.Description, &todo.Username); err != nil {
			return nil, fmt.Errorf("failed to extract todo from database query: %s", err)
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

	if rows.Next() {
		if err := rows.Scan(&createdTodo.ID, &createdTodo.Title, &createdTodo.Description, &createdTodo.Username); err != nil {
			return createdTodo, fmt.Errorf("failed to extract todo from database query: %s", err)
		}

	}

	return createdTodo, nil
}

func deleteTodoDB(id string) (int, error) {
	reminderRows, reminderErr := dbpool.Query(context.Background(), "DELETE FROM reminders WHERE todo_id=$1  RETURNING *", id)
	rows, err := dbpool.Query(context.Background(), "DELETE FROM todo WHERE id=$1 RETURNING *", id)
	if err != nil {
		return 0, err
	}
	if reminderErr != nil {
		return 0, reminderErr
	}
	defer rows.Close()
	defer reminderRows.Close()

	var deletedCount = 0
	var deletedReminderCount = 0

	for rows.Next() {
		deletedCount++
	}
	for reminderRows.Next() {
		deletedReminderCount++
	}

	LOG.Printf("Deleted %d todos and %d reminders", deletedCount, deletedReminderCount)

	return deletedCount + deletedReminderCount, nil
}

func updateTodoDB(todo Todo) error {
	rows, err := dbpool.Query(context.Background(), "UPDATE todo SET title=$2, description=$3 WHERE id=$1", todo.ID, todo.Title, todo.Description)
	if err != nil {
		return err
	}
	defer rows.Close()
	return nil
}

func createReminderDB(reminder Reminder) (Reminder, error) {
	var createdReminder Reminder
	rows, err := dbpool.Query(context.Background(), "INSERT INTO reminders (todo_id, username) VALUES($1, $2)", reminder.ID, reminder.Username)
	if err != nil {
		return createdReminder, err
	}
	defer rows.Close()

	return createdReminder, nil
}
