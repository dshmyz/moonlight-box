package storage

import (
	"os"
	"strings"
	"testing"
)

func TestS3StorageUsesTransferManagerForPut(t *testing.T) {
	source, err := os.ReadFile("s3_storage.go")
	if err != nil {
		t.Fatalf("read s3 storage source: %v", err)
	}
	text := string(source)
	if !strings.Contains(text, "transfermanager.New") || !strings.Contains(text, "UploadObject") {
		t.Fatal("S3 Put should use transfermanager for streaming/multipart uploads")
	}
	if strings.Contains(text, "os.CreateTemp") {
		t.Fatal("S3 Put should not spool non-seekable readers to local temp files")
	}
}

func TestS3StorageProvidesServerSideMove(t *testing.T) {
	source, err := os.ReadFile("s3_storage.go")
	if err != nil {
		t.Fatalf("read s3 storage source: %v", err)
	}
	text := string(source)
	if !strings.Contains(text, "func (s *S3Storage) Move") {
		t.Fatal("S3Storage should implement Move so CAS can avoid Get-Put-Delete copies")
	}
	if !strings.Contains(text, "CopyObject") {
		t.Fatal("S3Storage Move should use server-side CopyObject")
	}
}
