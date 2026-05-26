package server

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageExtFromContent(t *testing.T) {
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00}
	ext, ok := imageExtFromContent(png)
	if !ok || ext != ".png" {
		t.Fatalf("png: got %q %v", ext, ok)
	}
	jpg := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	ext, ok = imageExtFromContent(jpg)
	if !ok || ext != ".jpg" {
		t.Fatalf("jpg: got %q %v", ext, ok)
	}
	gif := []byte("GIF89a")
	ext, ok = imageExtFromContent(gif)
	if !ok || ext != ".gif" {
		t.Fatalf("gif: got %q %v", ext, ok)
	}
	if _, ok := imageExtFromContent([]byte("not an image")); ok {
		t.Fatal("expected invalid content")
	}
}

func TestSaveUploadedImage_rejectsNonImage(t *testing.T) {
	dir := t.TempDir()
	_, err := saveUploadedImage(bytes.NewReader([]byte("hello")), dir, "images", 1)
	if err == nil {
		t.Fatal("expected error for non-image")
	}
}

func TestSaveUploadedImage_writesPNG(t *testing.T) {
	dir := t.TempDir()
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x01}
	rel, err := saveUploadedImage(bytes.NewReader(png), dir, "images", 42)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rel, "images/") || !strings.HasSuffix(rel, ".png") {
		t.Fatalf("unexpected rel path: %q", rel)
	}
	base := filepath.Base(strings.TrimPrefix(rel, "images/"))
	full := filepath.Join(dir, base)
	if _, err := os.Stat(full); err != nil {
		t.Fatalf("file missing: %v", err)
	}
}
