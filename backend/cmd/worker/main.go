package main

import (
	"log"
	"time"
)

func main() {
	log.Println("worker started")
	for {
		time.Sleep(30 * time.Second)
	}
}
