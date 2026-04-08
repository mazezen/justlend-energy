package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/mazezen/justlend-energy/internel/router"
)

func main() {
	srv := &http.Server{
		Handler:      router.NewRouter(),
		Addr:         "0.0.0.0:8080",
		WriteTimeout: 60 * time.Second,
		ReadTimeout:  60 * time.Second,
	}

	fmt.Println("Listening on http://0.0.0.0:8080")
	log.Fatal(srv.ListenAndServe())
}
