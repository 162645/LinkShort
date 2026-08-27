package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

func main() {
	url := "http://localhost:8080/api/v1/shorten"
	payload := []byte(`{"long_url":"https://debug-test.com","user_id":"test_perf"}`)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		fmt.Printf("HTTP Request failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Response: %s\n", string(body))
}
