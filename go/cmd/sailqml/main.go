package main

import (
	"fmt"
	"os"
	"path/filepath"

	"harbour-freesailwallet/sailpkg"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "sailqml: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "patches":
		root, err := sailpkg.RepoRoot()
		if err != nil {
			return err
		}
		return sailpkg.UpdatePatches(filepath.Join(root, "qml"), filepath.Join(root, "go", "sailpkg", "patches"))
	case "checkout":
		force := false
		for _, a := range args[1:] {
			if a == "--force" {
				force = true
				continue
			}
			return fmt.Errorf("unknown argument %s", a)
		}
		root, err := sailpkg.RepoRoot()
		if err != nil {
			return err
		}
		return sailpkg.Checkout(filepath.Join(root, "qml"), force)
	case "install":
		dest, err := flagValue(args[1:], "--dest")
		if err != nil {
			return err
		}
		return sailpkg.Install(dest)
	case "prune":
		dest, err := flagValue(args[1:], "--dest")
		if err != nil {
			return err
		}
		return sailpkg.Prune(dest)
	default:
		return usage()
	}
}

func flagValue(args []string, name string) (string, error) {
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			return args[i+1], nil
		}
	}
	return "", fmt.Errorf("missing %s", name)
}

func usage() error {
	return fmt.Errorf("usage: sailqml patches | checkout [--force] | install --dest DIR | prune --dest DIR")
}
