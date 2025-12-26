package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Todo struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	Username    *string `json:"username"`
}

type DiscordMsg struct {
	Content string `json:"content"`
}

var LOG *log.Logger

const CHECKS_PER_MIN = 20

func main() {

	dotEnvErr := godotenv.Load(".env")

	LOG = log.Default()

	if dotEnvErr != nil {
		LOG.Println("Did not load .env file")
	}

	for range CHECKS_PER_MIN {
		if shouldSendReminder() {
			notifiedTodos := notify()
			if notifiedTodos != nil {
				deleteTodos(notifiedTodos)
			}
		}
		time.Sleep(60 / CHECKS_PER_MIN * time.Second)
	}
}

func shouldSendReminder() bool {
	resp, err := http.Get(os.Getenv("USER_ONLINE_URI"))
	if err != nil {
		log.Fatalf("Failed to fetch online status: %v\n", err)
		return true
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to parse response body: %v\n", err)
		return true
	}

	sb := string(body)
	return sb == "false"
}

func fetchTodos() ([]Todo, error) {
	resp, err := http.Get(os.Getenv("TODOS_URI"))
	if err != nil {
		log.Fatalf("Failed to fetch todos: %v\n", err)
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to parse todo response: %v\n", err)
		return nil, err
	}

	var todos []Todo
	if err := json.Unmarshal(body, &todos); err != nil {
		log.Fatal(err)
		return nil, err
	}

	var filteredTodos []Todo
	for _, todo := range todos {
		if todo.Username != nil {
			filteredTodos = append(filteredTodos, todo)
		}
	}

	return filteredTodos, nil
}

func notify() []Todo {
	var todos, err = fetchTodos()

	if err != nil || len(todos) == 0 {
		// Send Message notifying of failure (maybe we don't want that so we don't get spammed)
		return nil
	}

	LOG.Printf("User is offline and has %d todos, sending message now!\n", len(todos))

	var formattedTodos []string

	for _, todo := range todos {
		var fmtedDesc = ""
		if todo.Description != nil && len(*todo.Description) != 0 {
			fmtedDesc = ": " + *todo.Description
		}
		formattedTodos = append(formattedTodos, todo.Title+fmtedDesc)
	}

	var msg = DiscordMsg{Content: fmt.Sprintf("Hey, <@%s>, don't forget!\n%s", os.Getenv("DISCORD_USER_ID"), strings.Join(formattedTodos, ",\n"))}

	body, _ := json.Marshal(msg)

	_, err = http.Post(os.Getenv("DISCORD_WEBHOOK"), "application/json", bytes.NewBuffer(body))

	if err != nil {
		LOG.Fatal(err)
		return nil
	}

	return todos
}

func deleteTodos(todos []Todo) {

	for _, todo := range todos {
		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/%d", os.Getenv("TODOS_URI"), todo.ID), nil)
		if err != nil {
			log.Fatalf("Error creating request: %v\n", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatalf("Error sending request: %v\n", err)
		}
		defer resp.Body.Close()

		fmt.Printf("Deleted TODO w/ ID %d status: %s\n", todo.ID, resp.Status)
	}
}
