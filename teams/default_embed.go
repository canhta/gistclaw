package shippedteams

import (
	"embed"
	"io/fs"
)

//go:embed default/*
var defaultFiles embed.FS

var defaultFS fs.FS = func() fs.FS {
	sub, err := fs.Sub(defaultFiles, "default")
	if err != nil {
		panic(err)
	}
	return sub
}()

func Default() fs.FS {
	return defaultFS
}
