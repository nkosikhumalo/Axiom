package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func runInteractive() error {
	reader := bufio.NewReader(os.Stdin)

	readRequired := func(label string) (string, error) {
		fmt.Printf("%s: ", label)
		value, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return "", fmt.Errorf("%s is required", label)
		}
		return value, nil
	}

	legacy, err := readRequired("Legacy source file")
	if err != nil {
		return err
	}

	modern, err := readRequired("Modern source file")
	if err != nil {
		return err
	}

	// Set the package-level vars directly to avoid an initialization cycle
	// (rootCmd → run → runInteractive → rootCmd).
	legacyFile = legacy
	modernFile = modern

	return run(rootCmd, nil)
}
