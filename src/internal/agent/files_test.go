package agent

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	agentv1 "dockium/gen/agent/v1"
)

func newTestFiles(t *testing.T) (*fileService, string) {
	t.Helper()
	root := t.TempDir()
	manager := &Manager{dataRoot: root}
	files := newFileService(manager)
	directory := manager.InstanceDirectory(7)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create instance dir: %v", err)
	}
	return files, directory
}

func TestFileWriteReadList(t *testing.T) {
	files, directory := newTestFiles(t)

	write := files.handleWrite(1, &agentv1.WriteFile{
		InstanceId: 7,
		Path:       "nested/config.txt",
		Content:    []byte("привет dockium"),
		Create:     true,
	})
	if write.Error != "" {
		t.Fatalf("write error: %s", write.Error)
	}
	if data, err := os.ReadFile(filepath.Join(directory, "nested", "config.txt")); err != nil || string(data) != "привет dockium" {
		t.Fatalf("file content = %q, err = %v", data, err)
	}

	duplicate := files.handleWrite(1, &agentv1.WriteFile{
		InstanceId: 7,
		Path:       "nested/config.txt",
		Content:    []byte("x"),
		Create:     true,
	})
	if duplicate.Error == "" {
		t.Fatal("expected create on existing file to fail")
	}

	list := files.handleList(1, &agentv1.ListFiles{InstanceId: 7, Path: "nested"})
	if list.Error != "" || len(list.GetFiles().Items) != 1 {
		t.Fatalf("list = %+v", list)
	}

	read := files.handleRead(1, &agentv1.ReadFile{InstanceId: 7, Path: "nested/config.txt"})
	if read.Error != "" || string(read.GetFile().Content) != "привет dockium" {
		t.Fatalf("read = %+v", read)
	}
	if read.GetFile().Binary {
		t.Fatal("text file must not be binary")
	}
}

func TestFilePathTraversal(t *testing.T) {
	files, directory := newTestFiles(t)
	if err := os.WriteFile(filepath.Join(directory, "safe.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths := []string{"../secret", "../../etc/passwd", "nested/../../escape", "a/../../b", `..\\escape`}
	for _, path := range paths {
		result := files.handleRead(1, &agentv1.ReadFile{InstanceId: 7, Path: path})
		if result.Error == "" {
			t.Errorf("path %q must be rejected", path)
		}
	}
	if result := files.handleRead(1, &agentv1.ReadFile{InstanceId: 7, Path: "/safe.txt"}); result.Error != "" {
		t.Errorf("rooted path should map inside the root: %s", result.Error)
	}
}

func TestFileSymlinkEscape(t *testing.T) {
	files, directory := newTestFiles(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	result := files.handleRead(1, &agentv1.ReadFile{InstanceId: 7, Path: "link/secret.txt"})
	if result.Error == "" {
		t.Fatal("symlink escape must be rejected")
	}
}

func TestBinaryDetection(t *testing.T) {
	files, directory := newTestFiles(t)
	if err := os.WriteFile(filepath.Join(directory, "binary.bin"), []byte{0x00, 0xff, 0x10}, 0o644); err != nil {
		t.Fatal(err)
	}
	read := files.handleRead(1, &agentv1.ReadFile{InstanceId: 7, Path: "binary.bin"})
	if read.Error != "" {
		t.Fatalf("read error: %s", read.Error)
	}
	if !read.GetFile().Binary {
		t.Fatal("binary file must be flagged")
	}
	if len(read.GetFile().Content) != 0 {
		t.Fatal("binary content must not be returned")
	}
}

func TestRenameDeleteAndCompress(t *testing.T) {
	files, directory := newTestFiles(t)
	files.handleWrite(1, &agentv1.WriteFile{InstanceId: 7, Path: "raw/keep.txt", Content: []byte("keep"), Create: true})

	if result := files.handleRename(1, &agentv1.RenamePath{InstanceId: 7, OldPath: "raw/keep.txt", NewPath: "raw/renamed.txt"}); result.Error != "" {
		t.Fatalf("rename: %s", result.Error)
	}
	if _, err := os.Stat(filepath.Join(directory, "raw", "renamed.txt")); err != nil {
		t.Fatal("renamed file missing")
	}

	compress := files.handleCompress(1, &agentv1.CompressPath{InstanceId: 7, Path: "raw", Target: "raw.zip"})
	if compress.Error != "" {
		t.Fatalf("compress: %s", compress.Error)
	}
	archive, err := zip.OpenReader(filepath.Join(directory, "raw.zip"))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	if len(archive.File) == 0 {
		archive.Close()
		t.Fatal("archive is empty")
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	if result := files.handleDelete(1, &agentv1.DeletePath{InstanceId: 7, Path: "raw.zip"}); result.Error != "" {
		t.Fatalf("delete: %s", result.Error)
	}
	if _, err := os.Stat(filepath.Join(directory, "raw.zip")); !os.IsNotExist(err) {
		t.Fatal("file should be deleted")
	}
}

func TestMkdirAndChmod(t *testing.T) {
	files, directory := newTestFiles(t)
	if result := files.handleMkdir(1, &agentv1.CreateDirectory{InstanceId: 7, Path: "plugins"}); result.Error != "" {
		t.Fatalf("mkdir: %s", result.Error)
	}
	info, err := os.Stat(filepath.Join(directory, "plugins"))
	if err != nil || !info.IsDir() {
		t.Fatalf("plugins dir missing: %v", err)
	}
	if result := files.handleChmod(1, &agentv1.ChmodPath{InstanceId: 7, Path: "plugins", Mode: 0o750}); result.Error != "" {
		t.Fatalf("chmod: %s", result.Error)
	}
	if runtime.GOOS != "windows" {
		info, _ = os.Stat(filepath.Join(directory, "plugins"))
		if info.Mode().Perm() != 0o750 {
			t.Fatalf("mode = %o", info.Mode().Perm())
		}
	}
}

func TestCleanRelative(t *testing.T) {
	valid := map[string]string{
		"":         "",
		"/":        "",
		"a/b.txt":  "a/b.txt",
		"/a/b.txt": "a/b.txt",
		"a\\b.txt": "a/b.txt",
		"a/./b":    "a/b",
		"a//b":     "a/b",
	}
	for input, want := range valid {
		got, err := cleanRelative(input)
		if err != nil || got != want {
			t.Errorf("cleanRelative(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	invalid := []string{"..", "../a", "a/../../b", "a\x00b"}
	for _, input := range invalid {
		if _, err := cleanRelative(input); err == nil {
			t.Errorf("cleanRelative(%q) must fail", input)
		}
	}
}
