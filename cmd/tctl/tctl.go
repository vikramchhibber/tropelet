package main

import (
	"fmt"

	"github.com/troplet/internal/tctl"
)

func main() {
	client := tctl.Client{}
	if err := client.Execute(); err != nil {
		panic(fmt.Sprintf("Failed initializing client: %v", err.Error()))
	}
}
