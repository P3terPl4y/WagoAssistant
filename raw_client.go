package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	resp, err := http.Get("http://localhost:3000/")
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	fmt.Println("Response status:", resp.Status)
	for k, v := range resp.Header {
		fmt.Printf("%s: %v\n", k, v)
	}
}
