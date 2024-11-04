package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"github.com/davidbyttow/govips/v2/vips"
	"github.com/gofiber/fiber/v2"
)

var proxyVersion = "0.1.0"
var ctxt = context.Background()

func isExistParams(c *fiber.Ctx, paramName string) bool {
	queryParams := c.Queries()
	if _, ok := queryParams[paramName]; ok {
		return true
	}
	return false
}

func processImage(data []byte, isStatic bool, isPreview bool, isBadge bool) ([]byte, string) {
	importParams := vips.NewImportParams()
	if !isStatic {
		importParams.NumPages.Set(-1)
	}
	img, err := vips.LoadImageFromBuffer(data, importParams)
	if err != nil {
		return nil, err.Error()
	}
	if isPreview {
		width := img.Width()
		height := img.Height()
		maxWidth := 200
		maxHeight := 200
		scale := 1.0
		if width > maxWidth || height > maxHeight {
			scaleWidth := float64(maxWidth) / float64(width)
			scaleHeight := float64(maxHeight) / float64(height)
			scale = min(scaleWidth, scaleHeight)
		}
		if scale < 1.0 {
			if err := img.Resize(scale, vips.KernelAuto); err != nil {
				return nil, err.Error()
			}
		}
    }
	if isBadge {
		width := img.Width()
		height := img.Height()
		maxWidth := 96
		maxHeight := 96
		scale := 1.0
		if width > maxWidth || height > maxHeight {
			scaleWidth := float64(maxWidth) / float64(width)
			scaleHeight := float64(maxHeight) / float64(height)
			scale = min(scaleWidth, scaleHeight)
		}
		if scale < 1.0 {
			if err := img.Resize(scale, vips.KernelAuto); err != nil {
				return nil, err.Error()
			}
		}
		ep := vips.PngExportParams{}
		image, _, err := img.ExportPng(&ep);
		if err != nil {
			return nil, err.Error()
		}
		return image, ""
	}
	ep := vips.WebpExportParams{Quality: 80}
	image, _, err := img.ExportWebp(&ep);
	if err != nil {
		return nil, err.Error()
	}
	return image, ""
}

func image(c *fiber.Ctx) error {
	url := c.Query("url")
	isStatic := isExistParams(c, "static")
	isBadge := isExistParams(c, "badge")
	isPreview := isExistParams(c, "preview")
	c.Set(fiber.HeaderServer, "Go, media-proxy-go/" + proxyVersion)
	c.Set(fiber.HeaderCacheControl, "max-age=300")
	c.Set(fiber.HeaderContentSecurityPolicy, "default-src 'none'; img-src 'self'; media-src 'self'; style-src 'unsafe-inline'")
	if url == "" {
        return c.Status(400).SendString("url is required")
    }
	res, err := http.Get(url)
	if err != nil {
		return c.SendString(fmt.Sprintf("Error Fetching the media: %v", err))
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return c.SendString(fmt.Sprintf("Error Fetching the media: %v", res.StatusCode))
	}
	data, err := io.ReadAll(res.Body)
	if (res.Header.Get("Content-Type") == "" || res.Header.Get("Content-Type") == "application/octet-stream" || !strings.HasPrefix(res.Header.Get("Content-Type"), "image/")) {
		c.Set(fiber.HeaderCacheControl, "max-age=31536000, immutable")
		return c.Send(data)
	}
	if err != nil {
		slog.Error(err.Error())
	}
	image, errorStr := processImage(data, isStatic, isPreview, isBadge)
	if errorStr != "" {
		slog.Error("Error decoding the image: " + errorStr)
		c.Set(fiber.HeaderCacheControl, "max-age=31536000, immutable")
		return c.Send(data)
	}
	c.Set(fiber.HeaderContentType, "image/webp")
	if isBadge {
		c.Set(fiber.HeaderContentType, "image/png")
	}
	return c.Send(image)
}

func main() {
    vips.Startup(nil)
	defer vips.Shutdown()
	app := fiber.New()
	app.Get("/proxy/:type.webp", image)
	app.Listen(":3001")
}
