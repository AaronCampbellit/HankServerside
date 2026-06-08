package apps

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type PackagePreview struct {
	Manifest Manifest
	Warnings []string
}

func PreviewArchive(path string) (PackagePreview, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return PackagePreview{}, err
	}
	defer reader.Close()

	seen := make(map[string]struct{}, len(reader.File))
	files := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return PackagePreview{}, fmt.Errorf("archive contains symlink entry %q", file.Name)
		}
		cleaned, isDir, err := cleanArchivePath(file.Name)
		if err != nil {
			return PackagePreview{}, err
		}
		if file.FileInfo().IsDir() {
			isDir = true
		}
		if _, ok := seen[cleaned]; ok {
			return PackagePreview{}, fmt.Errorf("duplicate archive path %q", cleaned)
		}
		seen[cleaned] = struct{}{}
		if !isDir {
			files[cleaned] = file
		}
	}

	manifestFile, ok := files["app.json"]
	if !ok {
		return PackagePreview{}, fmt.Errorf("app.json is required")
	}

	manifest, err := decodeManifest(manifestFile)
	if err != nil {
		return PackagePreview{}, err
	}
	if err := ValidateManifest(manifest); err != nil {
		return PackagePreview{}, err
	}

	if _, ok := files[manifest.Runtime.Command]; !ok {
		return PackagePreview{}, fmt.Errorf("runtime command %q is missing from archive", manifest.Runtime.Command)
	}
	for _, schemaPath := range referencedSchemaPaths(manifest) {
		if _, ok := files[schemaPath]; !ok {
			return PackagePreview{}, fmt.Errorf("schema path %q is missing from archive", schemaPath)
		}
	}

	return PackagePreview{Manifest: manifest}, nil
}

func cleanArchivePath(value string) (string, bool, error) {
	trimmed := strings.TrimSuffix(value, "/")
	cleaned, ok := cleanPackagePath(trimmed)
	if !ok {
		return "", false, fmt.Errorf("unsafe archive path %q", value)
	}
	return cleaned, strings.HasSuffix(value, "/"), nil
}

func decodeManifest(file *zip.File) (Manifest, error) {
	reader, err := file.Open()
	if err != nil {
		return Manifest{}, fmt.Errorf("open app.json: %w", err)
	}
	defer reader.Close()

	var manifest Manifest
	if err := json.NewDecoder(reader).Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode app.json: %w", err)
	}
	return manifest, nil
}
