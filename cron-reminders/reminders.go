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

func main() {

	dotEnvErr := godotenv.Load(".env")

	LOG = log.Default()

	if dotEnvErr != nil {
		LOG.Println("Did not load .env file")
	}

	if shouldSendReminder() {
		notify()
	}
}

func shouldSendReminder() bool {
	resp, err := http.Get(os.Getenv("USER_ONLINE_URI"))
	if err != nil {
		log.Fatalln("Failed to fetch online status", err)
		return true
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln("Failed to parse response body", err)
		return true
	}

	sb := string(body)
	return sb == "false"
}

func fetchTodos() ([]Todo, error) {
	resp, err := http.Get(os.Getenv("TODOS_URI"))
	if err != nil {
		log.Fatalln("Failed to fetch todos", err)
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln("Failed to parse todo response", err)
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

func notify() {
	var todos, err = fetchTodos()

	if err != nil || len(todos) == 0 {
		// Send Message notifying of failure (maybe we don't want that so we don't get spammed)
		return
	}

	LOG.Printf("User is offline and has %d todos, sending message now!", len(todos))

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
		return
	}

	// TOOD: Delete TODOS
}
