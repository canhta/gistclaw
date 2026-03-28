package web

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed static
var staticAssets embed.FS

func staticAssetFS() fs.FS {
	sub, err := fs.Sub(staticAssets, "static")
	if err != nil {
		panic(fmt.Sprintf("web: locate static assets: %v", err))
	}
	return sub
}
