package main

import (
	"os"

	"github.com/ffreis/dynamoctl/cmd"
)

// exit is a package variable so tests can stub it out and observe the
// requested status code without terminating the test process.
var exit = os.Exit

func main() {
	exit(cmd.Execute())
}
