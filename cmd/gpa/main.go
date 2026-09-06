package main

import (
	"os"

	"github.com/soren-labs/gpt-account/internal/gpa"
)

func main() {
	os.Exit(gpa.Main(os.Args[1:]))
}
