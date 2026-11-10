// Command keygen prints a new random TOKEN_ENCRYPTION_KEY value.
package main

import (
	"fmt"
	"os"

	"contentfactory/internal/secrets"
)

func main() {
	key, err := secrets.GenerateKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(key)
}
