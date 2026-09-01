package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageLibraryWritesSingleRootLibrary(t *testing.T) {
	dir := t.TempDir()
	library := filepath.Join(dir, "stickyproxy.so")
	if err := os.WriteFile(library, []byte("plugin-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "stickyproxy_1.2.3_linux_amd64.zip")
	data, err := packageLibrary(library, archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("archive data is empty")
	}
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if len(reader.File) != 1 || reader.File[0].Name != "stickyproxy.so" {
		t.Fatalf("archive files = %#v", reader.File)
	}
}

func TestPackageChecksumUsesSha256sumFormat(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "stickyproxy_1.2.3_linux_amd64.zip")
	checksum := filepath.Join(dir, "stickyproxy_1.2.3_linux_amd64.zip.sha256")
	if err := writeChecksum(checksum, archive, []byte("archive-data")); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(checksum)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(raw), "  "+filepath.Base(archive)+"\n") {
		t.Fatalf("invalid checksum format: %q", raw)
	}
	if len(strings.Fields(string(raw))) != 2 || len(strings.Fields(string(raw))[0]) != 64 {
		t.Fatalf("invalid sha256sum line: %q", raw)
	}
}
