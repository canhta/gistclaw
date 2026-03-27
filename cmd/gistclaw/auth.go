package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

var (
	authIsTerminal   = func(fd int) bool { return term.IsTerminal(fd) }
	authReadPassword = func(fd int) ([]byte, error) { return term.ReadPassword(fd) }
)

func runAuth(configPath string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "Usage: gistclaw auth set-password [--password-stdin]")
		return 1
	}

	switch args[0] {
	case "set-password":
		return runAuthSetPassword(configPath, args[1:], stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown auth subcommand: %s\n", args[0])
		return 1
	}
}

func runAuthSetPassword(configPath string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	usePasswordStdin := false
	for _, arg := range args {
		switch arg {
		case "--password-stdin":
			usePasswordStdin = true
		default:
			fmt.Fprintln(stderr, "Usage: gistclaw auth set-password [--password-stdin]")
			return 1
		}
	}

	password, err := readAuthPassword(stdin, stderr, usePasswordStdin)
	if err != nil {
		fmt.Fprintf(stderr, "auth set-password failed: %v\n", err)
		return 1
	}

	application, err := loadApp(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "bootstrap app: %v\n", err)
		return 1
	}
	defer func() { _ = application.Stop() }()

	if err := application.SetPassword(context.Background(), password, time.Now().UTC()); err != nil {
		fmt.Fprintf(stderr, "auth set-password failed: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "password updated")
	return 0
}

func readAuthPassword(stdin io.Reader, stderr io.Writer, fromStdin bool) (string, error) {
	if fromStdin {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read password from stdin: %w", err)
		}
		password := strings.TrimRight(string(data), "\r\n")
		if strings.TrimSpace(password) == "" {
			return "", fmt.Errorf("password is required")
		}
		return password, nil
	}

	fd := stdinFD(stdin)
	if !authIsTerminal(fd) {
		return "", fmt.Errorf("stdin is not a terminal; use --password-stdin")
	}
	first, err := promptSecret(stderr, fd, "New password: ")
	if err != nil {
		return "", err
	}
	second, err := promptSecret(stderr, fd, "Confirm password: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", fmt.Errorf("passwords do not match")
	}
	if strings.TrimSpace(first) == "" {
		return "", fmt.Errorf("password is required")
	}
	return first, nil
}

func promptSecret(stderr io.Writer, fd int, prompt string) (string, error) {
	fmt.Fprint(stderr, prompt)
	secret, err := authReadPassword(fd)
	fmt.Fprintln(stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(secret), nil
}

func stdinFD(stdin io.Reader) int {
	file, ok := stdin.(*os.File)
	if !ok {
		return -1
	}
	return int(file.Fd())
}
