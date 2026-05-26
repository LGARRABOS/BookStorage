package server

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxImageUploadBytes = 5 << 20 // 5 MiB

var (
	errImageTooLarge   = errors.New("image too large")
	errInvalidImage    = errors.New("invalid image content")
	errUnsafeImagePath = errors.New("unsafe image path")
)

// imageExtFromContent returns a normalized extension from magic bytes (.png, .jpg, .gif).
func imageExtFromContent(head []byte) (string, bool) {
	if len(head) >= 8 && bytes.HasPrefix(head, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return ".png", true
	}
	if len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF {
		return ".jpg", true
	}
	if len(head) >= 6 && (bytes.HasPrefix(head, []byte("GIF87a")) || bytes.HasPrefix(head, []byte("GIF89a"))) {
		return ".gif", true
	}
	return "", false
}

func uploadPathWithinDir(uploadDir, fullPath string) bool {
	cleanDir := filepath.Clean(uploadDir)
	cleanFull := filepath.Clean(fullPath)
	if cleanFull == cleanDir {
		return false
	}
	sep := string(os.PathSeparator)
	return strings.HasPrefix(cleanFull, cleanDir+sep)
}

// saveUploadedImage validates image content, writes under uploadDir, and returns the relative URL path segment.
func saveUploadedImage(file io.Reader, uploadDir, urlPath string, userID int) (string, error) {
	if file == nil || uploadDir == "" || userID <= 0 {
		return "", errInvalidImage
	}
	limited := io.LimitReader(file, maxImageUploadBytes+1)
	head := make([]byte, 512)
	n, err := io.ReadFull(limited, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", errInvalidImage
	}
	head = head[:n]
	if len(head) == 0 {
		return "", errInvalidImage
	}
	ext, ok := imageExtFromContent(head)
	if !ok {
		return "", errInvalidImage
	}

	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%d_%s%s", userID, hex.EncodeToString(rnd[:]), ext)
	full := filepath.Join(uploadDir, filename)
	if !uploadPathWithinDir(uploadDir, full) {
		return "", errUnsafeImagePath
	}

	dst, err := os.Create(full)
	if err != nil {
		return "", err
	}
	defer func() { _ = dst.Close() }()

	reader := io.MultiReader(bytes.NewReader(head), limited)
	written, err := io.Copy(dst, reader)
	if err != nil {
		_ = os.Remove(full)
		return "", err
	}
	if written > maxImageUploadBytes {
		_ = os.Remove(full)
		return "", errImageTooLarge
	}
	return buildMediaRelativePath(filename, urlPath), nil
}

func saveImageFromForm(r *http.Request, field, uploadDir, urlPath string, userID int) (string, error) {
	if r == nil {
		return "", errInvalidImage
	}
	file, header, err := r.FormFile(field)
	if err != nil || header == nil || strings.TrimSpace(header.Filename) == "" {
		return "", errInvalidImage
	}
	defer func() { _ = file.Close() }()
	return saveUploadedImage(file, uploadDir, urlPath, userID)
}
