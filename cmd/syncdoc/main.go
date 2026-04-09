package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/NirajNair/syncdoc/internal/tunnel"
)

const TOKEN = "3C7iNOiAPdB8rsxC5SZgaDO0tIf_7AXLx1wxoQndC87eb5J7Q"

func main() {
	ctx := context.Background()
	addr := "http://localhost:8080"

	go startServer()

	tunnel, err := tunnel.Run(ctx, addr)
	if err != nil {
		fmt.Printf("Error starting ngrok tunnel: %v", err.Error())
	}
	// Close ngrok forwarder
	<-tunnel.Done()
}

func startServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ok"))
	})

	println("Statring server on port :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
