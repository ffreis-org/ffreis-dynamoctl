package main

import (
	"os"

	"github.com/ffreis/dynamoctl/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
