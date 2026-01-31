package main

// https://go.dev/doc/tutorial/database-access

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
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

type UserInfo struct {
	Sub      string `json:"sub"` // depending on your Authentik mappings
	UserName string `json:"preferred_username"`
	// add fields as needed based on Authentik property mappings
}

const AuthUserKey = "user"
const LOGIN_URL = "/auth/login"

var dbpool *pgxpool.Pool
var LOG *log.Logger
var oauthConfig *oauth2.Config

func main() {

	dotEnvErr := godotenv.Load("stack.env")

	oauthConfig = &oauth2.Config{
		ClientID:     os.Getenv("OAUTH_CLIENT_ID"),
		ClientSecret: os.Getenv("OAUTH_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("OAUTH_REDIRECT_URL"),
		Scopes:       []string{"openid", "profile"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  os.Getenv("OAUTH_AUTH_URL"),
			TokenURL: os.Getenv("OAUTH_TOKEN_URL"),
		},
	}

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
	config.AllowOrigins = []string{os.Getenv("APP_URL")}
	config.AllowCredentials = true
	store := cookie.NewStore(
		[]byte(getEnv("SESSION_SIGNING_KEY", string(genRandBytes(64)))),
		[]byte(getEnv("SESSION_ENCRYPTION_KEY", string(genRandBytes(32)))),
		[]byte(os.Getenv("SESSION_SIGNING_KEY_OLD")),
		[]byte(os.Getenv("SESSION_ENCRYPTION_KEY_OLD")),
	)
	store.Options(sessions.Options{HttpOnly: true, Path: "/", Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: 30 * 24 * 60 * 60})

	router.Use(sessions.Sessions("auth", store))
	router.Use(cors.New(config))

	authorized := router.Group("/")
	authorized.Use(AuthRequired())
	{
		authorized.GET("/todos", getTodos)
		authorized.POST("/todos", createTodo)
		authorized.DELETE("/todos/:id", deleteTodo)
		authorized.PUT("/todos", updateTodo)
		authorized.GET("/whoami", whoami)

		authorized.POST("/reminders", createReminder)
	}

	router.GET(LOGIN_URL, getLogin)
	router.GET("/auth/callback", getAuthCallback)

	router.Run(fmt.Sprintf("%s:%s", os.Getenv("SERVER_ADDRESS"), os.Getenv("SERVER_PORT")))
}

func AuthRequired() gin.HandlerFunc {
	return func(context *gin.Context) {
		session := sessions.Default(context)
		if session == nil || session.Get(AuthUserKey) == nil {
			LOG.Println("User Unauthenticated")
			context.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		context.Set(AuthUserKey, session.Get(AuthUserKey))
	}
}

func getLogin(context *gin.Context) {
	state := "randomCSRFToken" // TODO: generate and store in session
	url := oauthConfig.AuthCodeURL(state)
	context.Redirect(http.StatusFound, url)
}

func getAuthCallback(ginContext *gin.Context) {
	// state := c.Query("state")
	code := ginContext.Query("code")

	// TODO: validate `state` against what you stored

	ctx := context.Background()
	token, err := oauthConfig.Exchange(ctx, code)
	if err != nil {
		LOG.Println("token exchange error:", err)
		ginContext.JSON(http.StatusBadRequest, gin.H{"error": "token exchange failed"})
		return
	}

	// Use token to create an HTTP client or fetch userinfo
	client := oauthConfig.Client(ctx, token)

	resp, err := client.Get(os.Getenv("OAUTH_USER_INFO_URL"))
	if err != nil {
		LOG.Println("userinfo error:", err)
		ginContext.JSON(http.StatusBadRequest, gin.H{"error": "failed to fetch user info"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ginContext.String(resp.StatusCode, "userinfo responded with status %d", resp.StatusCode)
		return
	}

	var userInfo UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		log.Println("decode userinfo error:", err)
		ginContext.String(http.StatusInternalServerError, "decode userinfo failed")
		return
	}

	session := sessions.Default(ginContext)
	session.Set(AuthUserKey, userInfo.UserName)
	session.Save()

	ginContext.Redirect(http.StatusFound, os.Getenv("APP_URL"))
}

func whoami(context *gin.Context) {
	session := sessions.Default(context)
	user := session.Get(AuthUserKey)
	context.JSON(200, gin.H{"user": user})

}

func getTodos(context *gin.Context) {
	user := context.MustGet(AuthUserKey).(string)
	LOG.Println("Getting TODOs for USER: ", user)
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

func genRandBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		LOG.Fatalf("failed to read random bytes: %v", err)
	}
	return b
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
