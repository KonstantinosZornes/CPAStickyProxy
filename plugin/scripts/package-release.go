// package-release creates one CLIProxyAPI Plugin Store-compatible archive.
// The archive contains exactly the plugin library at its root and a companion
// sha256sum-format checksum file is written beside it.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	libraryPath := flag.String("library", "", "compiled plugin library")
	archivePath := flag.String("archive", "", "output ZIP archive")
	checksumPath := flag.String("checksum", "", "output SHA-256 checksum file")
	flag.Parse()
	if *libraryPath == "" || *archivePath == "" || *checksumPath == "" {
		fatalf("library, archive, and checksum are required")
	}

	data, err := packageLibrary(*libraryPath, *archivePath)
	if err != nil {
		fatalf("package library: %v", err)
	}
	if err := writeChecksum(*checksumPath, *archivePath, data); err != nil {
		fatalf("write checksum: %v", err)
	}
}

func writeChecksum(checksumPath, archivePath string, data []byte) error {
	checksum := sha256.Sum256(data)
	line := fmt.Sprintf("%s  %s\n", hex.EncodeToString(checksum[:]), filepath.Base(archivePath))
	return os.WriteFile(checksumPath, []byte(line), 0o644)
}

func packageLibrary(libraryPath, archivePath string) ([]byte, error) {
	library, err := os.Open(libraryPath)
	if err != nil {
		return nil, err
	}
	defer library.Close()
	info, err := library.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("library is not a regular file")
	}

	archive, err := os.Create(archivePath)
	if err != nil {
		return nil, err
	}
	writer := zip.NewWriter(archive)
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return nil, err
	}
	header.Name = filepath.Base(libraryPath)
	header.Method = zip.Deflate
	header.SetMode(0o755)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(entry, library); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return os.ReadFile(archivePath)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
