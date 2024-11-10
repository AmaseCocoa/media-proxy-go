package main

import (
	"fmt"
	"github.com/davidbyttow/govips/v2/vips"
	"github.com/gin-gonic/gin"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/fcgi"
	"strings"
)

var proxyVersion = "0.1.0"

type ProxyConfig struct {
	Id int `json:"id"`
}

func isExistParams(c *gin.Context, paramName string) bool {
	queryParams := c.Request.URL.Query()
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
		image, _, err := img.ExportPng(&ep)
		if err != nil {
			return nil, err.Error()
		}
		return image, ""
	}
	ep := vips.WebpExportParams{Quality: 80}
	image, _, err := img.ExportWebp(&ep)
	if err != nil {
		return nil, err.Error()
	}
	return image, ""
}

func image(c *gin.Context) {
	url := c.Query("url")
	isStatic := isExistParams(c, "static")
	isBadge := isExistParams(c, "badge")
	isPreview := isExistParams(c, "preview")
	c.Header("Server", "Go, media-proxy-go/"+proxyVersion)
	c.Header("Cache-Control", "max-age=300")
	c.Header("Content-Security-Policy", "default-src 'none'; img-src 'self'; media-src 'self'; style-src 'unsafe-inline'")
	if url == "" {
		c.String(400, "url is required")
		return
	}
	res, err := http.Get(url)
	if err != nil {
		c.String(500, fmt.Sprintf("Error Fetching the media: %v", err))
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		c.String(500, fmt.Sprintf("Error Fetching the media: %v", res.StatusCode))
		return
	}
	data, err := io.ReadAll(res.Body)
	if res.Header.Get("Content-Type") == "" || res.Header.Get("Content-Type") == "application/octet-stream" || !strings.HasPrefix(res.Header.Get("Content-Type"), "image/") {
		c.Header("Cache-Control", "max-age=31536000, immutable")
		c.Data(200, res.Header.Get("Content-Type"), data)
		return
	}
	if err != nil {
		slog.Error(err.Error())
	}
	image, errorStr := processImage(data, isStatic, isPreview, isBadge)
	if errorStr != "" {
		slog.Error("Error decoding the image: " + errorStr)
		c.Header("Cache-Control", "max-age=300")
		c.Data(200, res.Header.Get("Content-Type"), data)
		return
	}
	c.Header("Cache-Control", "max-age=31536000, immutable")
	var ContentType string
	ContentType = "image/webp"
	if isBadge {
		ContentType = "image/png"
	}
	c.Data(200, ContentType, image)
}

func main() {
	vips.Startup(nil)
	defer vips.Shutdown()
	router := gin.Default()
	router.GET("/proxy/:type.webp", image)
	listener, err := net.Listen("tcp", ":9000")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	if err := fcgi.Serve(listener, router); err != nil {
		panic(err)
	}
}
