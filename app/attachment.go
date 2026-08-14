package app

import (
	"fmt"
	"net/http"
	"os"

	"github.com/Federicoand98/mani/core"
)

const maxImageBytes = 10 << 20 // 10MB

func LoadImage(path string) (core.ImageBlock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return core.ImageBlock{}, fmt.Errorf("image %s: %w", path, err)
	}

	if len(data) > maxImageBytes {
		return core.ImageBlock{}, fmt.Errorf("image %s: %d bytes exceeds the %d byte limit", path, len(data), maxImageBytes)
	}

	mt := http.DetectContentType(data)
	switch mt {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		return core.ImageBlock{}, fmt.Errorf("unsupported image type: %s", mt)
	}

	return core.ImageBlock{MediaType: mt, Data: data}, nil
}
