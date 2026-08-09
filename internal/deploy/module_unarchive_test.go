package deploy

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeTarGz(t *testing.T, name string, typeflag byte, link string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "bundle.tar.gz")
	output, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(output)
	tw := tar.NewWriter(gz)
	header := &tar.Header{Name: name, Typeflag: typeflag, Linkname: link, Mode: 0o644}
	if typeflag == tar.TypeReg {
		header.Size = 2
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if typeflag == tar.TypeReg {
		if _, err := tw.Write([]byte("ok")); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestUnarchiveRejectsDangerousDestination(t *testing.T) {
	module := &unarchiveModule{}
	for _, destination := range []string{"/", ".", "..", "../etc", "/opt/../etc", "~/bundle"} {
		t.Run(destination, func(t *testing.T) {
			node := &yaml.Node{}
			data, err := yaml.Marshal(map[string]any{"src": "bundle.zip", "dest": destination})
			if err != nil {
				t.Fatal(err)
			}
			if err := yaml.Unmarshal(data, node); err != nil {
				t.Fatal(err)
			}
			if _, err := module.DecodeArgs(node); err == nil {
				t.Fatalf("dangerous destination %q should be rejected", destination)
			}
		})
	}
}

func TestUnarchiveNormalizesDestinationBeforeStaging(t *testing.T) {
	module := &unarchiveModule{}
	node := &yaml.Node{}
	data, err := yaml.Marshal(map[string]any{"src": "bundle.zip", "dest": "/opt/app/"})
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, node); err != nil {
		t.Fatal(err)
	}
	decoded, err := module.DecodeArgs(node)
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.(*unarchiveArgs).Dest; got != "/opt/app" {
		t.Fatalf("normalized destination = %q", got)
	}
}

func TestParseRemoteTreeManifestPreservesTypesModesAndUnusualNames(t *testing.T) {
	fields := []string{
		"empty dir", "dir", "755", "-",
		"nested/file name", "file", "640", "abc123",
		"line\nbreak", "link", "777", "-",
	}
	manifest, err := parseRemoteTreeManifest(strings.Join(fields, "\x00") + "\x00")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 3 || manifest["empty dir"].Type != "dir" ||
		manifest["nested/file name"].Mode != "640" || manifest["line\nbreak"].Type != "link" {
		t.Fatalf("manifest = %+v", manifest)
	}
	if _, err := parseRemoteTreeManifest("truncated"); err == nil {
		t.Fatal("truncated manifest should fail closed")
	}
}

func TestValidateLocalArchiveAcceptsRegularTar(t *testing.T) {
	file := writeTarGz(t, "app/config.txt", tar.TypeReg, "")
	if err := validateLocalArchive(file, archiveKind(file)); err != nil {
		t.Fatalf("regular archive rejected: %v", err)
	}
}

func TestValidateLocalArchiveRejectsTarLinksAndTraversal(t *testing.T) {
	for _, tt := range []struct {
		name     string
		typeflag byte
		link     string
	}{
		{"absolute symlink", tar.TypeSymlink, "/etc/passwd"},
		{"hardlink", tar.TypeLink, "../../etc/passwd"},
		{"traversal", tar.TypeReg, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			name := "link"
			if tt.name == "traversal" {
				name = "../../escape"
			}
			file := writeTarGz(t, name, tt.typeflag, tt.link)
			if err := validateLocalArchive(file, archiveKind(file)); err == nil {
				t.Fatal("unsafe archive should be rejected")
			}
		})
	}
}

func TestValidateLocalArchiveRejectsZipSymlink(t *testing.T) {
	file := filepath.Join(t.TempDir(), "bundle.zip")
	output, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(output)
	header := &zip.FileHeader{Name: "absolute-link"}
	header.SetMode(os.ModeSymlink | 0o777)
	entry, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("/etc/passwd")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	if err := validateLocalArchive(file, archiveKind(file)); err == nil || !strings.Contains(err.Error(), "特殊文件") {
		t.Fatalf("zip symlink error = %v", err)
	}
}
