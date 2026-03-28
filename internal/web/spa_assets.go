package web

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed all:appdist
var spaAssets embed.FS

func spaAssetsFS() fs.FS {
	sub, err := fs.Sub(spaAssets, "appdist")
	if err != nil {
		panic(fmt.Sprintf("web: locate spa assets: %v", err))
	}
	return sub
}

func readSPAAsset(name string) ([]byte, error) {
	return fs.ReadFile(spaAssetsFS(), name)
}
