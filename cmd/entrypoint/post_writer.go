package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/tektoncd/pipeline/pkg/entrypoint"
)

// realPostWriter actually writes files.
type realPostWriter struct{}

var _ entrypoint.PostWriter = (*realPostWriter)(nil)

func (*realPostWriter) Write(file string) {
	if file == "" {
		return
	}
	if _, err := os.Create(file); err != nil {
		log.Fatalf("Creating %q: %v", file, err)
	}
}

func (*realPostWriter) WriteFileContent(file string, content string) {
	if file == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(file), 0770); err != nil {
		log.Fatalf("Creating file path %q: %v", file, err)
	}
	f, err := os.Create(file)
	if err != nil {
		log.Fatalf("Creating %q: %v", file, err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		log.Fatalf("Writing %q: %v", file, err)
	}
}
