package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppFileOps(t *testing.T) {
	dir := t.TempDir()
	svc := AppSvc{}

	sub, err := svc.NewFolder(dir, "子目录")
	if err != nil {
		t.Fatalf("NewFolder: %v", err)
	}
	if st, err := os.Stat(sub); err != nil || !st.IsDir() {
		t.Fatalf("NewFolder did not create a folder")
	}

	file := filepath.Join(sub, "a.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	renamed, err := svc.Rename(file, "b.txt")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := os.Stat(renamed); err != nil {
		t.Fatalf("Rename target missing: %v", err)
	}

	if err := svc.Delete(renamed, false); err != nil {
		t.Fatalf("Delete to recycle bin: %v", err)
	}
	if _, err := os.Stat(renamed); !os.IsNotExist(err) {
		t.Fatalf("file still present after recycle: %v", err)
	}

	if err := svc.Delete(sub, true); err != nil {
		t.Fatalf("permanent delete: %v", err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Fatalf("folder still present after delete")
	}

	if _, err := svc.Rename(filepath.Join(dir, "missing.txt"), `bad:name`); err == nil {
		t.Fatal("Rename accepted an invalid name")
	}
}
