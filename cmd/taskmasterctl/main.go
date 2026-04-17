package main

import (
	"fmt"
	"os"
)

func main() {

	if len(os.Args) < 1 {
		fmt.Println("Usage: taskmasterctl <command> [args]")
		os.Exit(1)
	}

}
