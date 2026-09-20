// Command package-cross builds a real cross-platform Go sidecar and writes an unsigned DBXP review candidate.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type artifactMetadata struct {
	Target string `json:"target"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type packageFile struct {
	name string
	data []byte
	mode fs.FileMode
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	target := flag.String("target", "", "DBX package target")
	outputDir := flag.String("output-dir", "dist", "artifact output directory")
	flag.Parse()

	parts := strings.Split(*target, "-")
	if len(parts) != 2 || !contains([]string{"darwin", "windows", "linux"}, parts[0]) || !contains([]string{"arm64", "x64"}, parts[1]) {
		panic("unsupported target: " + *target)
	}

	manifestBytes, err := os.ReadFile("manifest.json")
	must(err)
	var manifest map[string]any
	must(json.Unmarshal(manifestBytes, &manifest))
	id, ok := manifest["id"].(string)
	if !ok || id == "" {
		panic("manifest id is required")
	}
	version, ok := manifest["version"].(string)
	if !ok || version == "" {
		panic("manifest version is required")
	}
	entrypoints := manifest["entrypoints"].(map[string]any)
	backend := entrypoints["backend"].(map[string]any)
	binaryName := path.Base(backend["executable"].(string))
	if parts[0] == "windows" {
		binaryName += ".exe"
	}
	executable := path.Join("bin", *target, binaryName)
	backend["executable"] = executable
	manifestBytes, err = json.MarshalIndent(manifest, "", "  ")
	must(err)
	manifestBytes = append(manifestBytes, '\n')

	tempDir, err := os.MkdirTemp("", "dbx-plugin-cross-")
	must(err)
	defer os.RemoveAll(tempDir)
	binaryPath := filepath.Join(tempDir, binaryName)
	cmd := exec.Command("go", "build", "-trimpath", "-o", binaryPath, ".")
	cmd.Dir = "backend"
	cmd.Env = append(os.Environ(), "GOOS="+parts[0], "GOARCH="+mapArch(parts[1]), "CGO_ENABLED=0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	must(cmd.Run())

	files := []packageFile{{name: executable, data: mustRead(binaryPath), mode: 0o755}, {name: "manifest.json", data: manifestBytes, mode: 0o644}}
	for _, root := range []string{"assets", "ui"} {
		must(filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("unsupported package file: %s", filePath)
			}
			files = append(files, packageFile{name: filepath.ToSlash(filePath), data: mustRead(filePath), mode: 0o644})
			return nil
		}))
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })

	checksums := struct {
		Algorithm string            `json:"algorithm"`
		Files     map[string]string `json:"files"`
	}{Algorithm: "sha256", Files: map[string]string{}}
	for _, file := range files {
		digest := sha256.Sum256(file.data)
		checksums.Files[file.name] = hex.EncodeToString(digest[:])
	}
	checksumBytes, err := json.MarshalIndent(checksums, "", "  ")
	must(err)
	checksumBytes = append(checksumBytes, '\n')
	files = append(files, packageFile{name: "checksums.json", data: checksumBytes, mode: 0o644})

	must(os.MkdirAll(*outputDir, 0o755))
	archiveName := fmt.Sprintf("%s-%s-%s.dbxp", id, version, *target)
	archivePath := filepath.Join(*outputDir, archiveName)
	archive := new(bytes.Buffer)
	zw := zip.NewWriter(archive)
	fixedTime := time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, file := range files {
		header := &zip.FileHeader{Name: file.name, Method: zip.Deflate, Modified: fixedTime}
		header.SetMode(file.mode)
		writer, createErr := zw.CreateHeader(header)
		must(createErr)
		_, writeErr := writer.Write(file.data)
		must(writeErr)
	}
	must(zw.Close())
	must(os.WriteFile(archivePath, archive.Bytes(), 0o644))

	digest := sha256.Sum256(archive.Bytes())
	metadata := artifactMetadata{Target: *target, URL: archiveName, SHA256: hex.EncodeToString(digest[:]), Size: int64(archive.Len())}
	metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
	must(err)
	metadataBytes = append(metadataBytes, '\n')
	must(os.WriteFile(strings.TrimSuffix(archivePath, ".dbxp")+".artifact.json", metadataBytes, 0o644))
	fmt.Printf("Success: cross-built %s\n", archivePath)
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func mapArch(arch string) string {
	if arch == "x64" {
		return "amd64"
	}
	return arch
}

func mustRead(name string) []byte {
	file, err := os.Open(name)
	must(err)
	defer file.Close()
	data, err := io.ReadAll(file)
	must(err)
	return data
}
