package apps

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const maxManifestBytes = 64 * 1024

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

	rootPrefix := archiveRootPrefix(reader.File)
	paths := make(map[string]bool, len(reader.File))
	files := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		name, keep := normalizeArchiveName(file.Name, rootPrefix)
		if !keep {
			continue
		}
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return PackagePreview{}, fmt.Errorf("archive contains symlink entry %q", file.Name)
		}
		cleaned, isDir, err := cleanArchivePath(name)
		if err != nil {
			return PackagePreview{}, err
		}
		if file.FileInfo().IsDir() {
			isDir = true
		}
		if _, ok := paths[cleaned]; ok {
			return PackagePreview{}, fmt.Errorf("duplicate archive path %q", cleaned)
		}
		if err := validateArchivePathCollision(paths, cleaned, isDir); err != nil {
			return PackagePreview{}, err
		}
		paths[cleaned] = isDir
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

func validateArchivePathCollision(paths map[string]bool, cleaned string, isDir bool) error {
	for existing, existingIsDir := range paths {
		if !existingIsDir && archivePathHasParent(cleaned, existing) {
			return fmt.Errorf("archive path collision: %q is inside file path %q", cleaned, existing)
		}
		if !isDir && archivePathHasParent(existing, cleaned) {
			return fmt.Errorf("archive path collision: %q contains existing path %q", cleaned, existing)
		}
	}
	return nil
}

func archivePathHasParent(value string, parent string) bool {
	return strings.HasPrefix(value, parent+"/")
}

func cleanArchivePath(value string) (string, bool, error) {
	trimmed := strings.TrimSuffix(value, "/")
	cleaned, ok := cleanPackagePath(trimmed)
	if !ok {
		return "", false, fmt.Errorf("unsafe archive path %q", value)
	}
	return cleaned, strings.HasSuffix(value, "/"), nil
}

// isIgnoredArchiveEntry reports entries that archivers add but that are not part
// of the app payload (macOS "Compress" artifacts). Skipping them lets a folder
// zipped from Finder be treated the same as a purpose-built package.
func isIgnoredArchiveEntry(name string) bool {
	trimmed := strings.TrimSuffix(name, "/")
	if trimmed == "" {
		return true
	}
	for _, part := range strings.Split(trimmed, "/") {
		if part == "__MACOSX" || part == ".DS_Store" || strings.HasPrefix(part, "._") {
			return true
		}
	}
	return false
}

// archiveRootPrefix returns a single top-level directory shared by every
// meaningful entry, e.g. "myapp/" when the archive was produced by compressing a
// folder ("myapp/app.json", "myapp/schemas/..."). It returns "" when entries
// already live at the archive root or span multiple top-level directories, so a
// purpose-built flat package is unaffected.
func archiveRootPrefix(files []*zip.File) string {
	prefix := ""
	for _, file := range files {
		if isIgnoredArchiveEntry(file.Name) {
			continue
		}
		trimmed := strings.TrimSuffix(file.Name, "/")
		idx := strings.Index(trimmed, "/")
		var top string
		switch {
		case idx >= 0:
			top = trimmed[:idx]
		case strings.HasSuffix(file.Name, "/"):
			// A bare top-level directory entry (e.g. "myapp/"): candidate wrapper.
			top = trimmed
		default:
			// A file sits directly at the archive root: no shared wrapper.
			return ""
		}
		// Only strip a wrapper whose own name is a safe path component; never let
		// prefix handling mask an unsafe entry such as "..", "C:", or ".".
		if cleaned, ok := cleanPackagePath(top); !ok || cleaned != top {
			return ""
		}
		switch prefix {
		case "":
			prefix = top
		case top:
		default:
			return ""
		}
	}
	if prefix == "" {
		return ""
	}
	return prefix + "/"
}

// normalizeArchiveName drops ignored entries and strips the shared root prefix.
// It returns ok=false for entries that must be skipped entirely (junk or the
// wrapper directory itself).
func normalizeArchiveName(name string, rootPrefix string) (string, bool) {
	if isIgnoredArchiveEntry(name) {
		return "", false
	}
	if rootPrefix != "" {
		name = strings.TrimPrefix(name, rootPrefix)
	}
	if strings.TrimSuffix(name, "/") == "" {
		return "", false
	}
	return name, true
}

func decodeManifest(file *zip.File) (Manifest, error) {
	if file.UncompressedSize64 > maxManifestBytes {
		return Manifest{}, manifestTooLargeError(file.UncompressedSize64)
	}
	reader, err := file.Open()
	if err != nil {
		return Manifest{}, fmt.Errorf("open app.json: %w", err)
	}
	defer reader.Close()

	var manifest Manifest
	limited := &io.LimitedReader{R: reader, N: maxManifestBytes + 1}
	decoder := json.NewDecoder(limited)
	if err := decoder.Decode(&manifest); err != nil {
		if limited.N == 0 {
			return Manifest{}, manifestTooLargeError(0)
		}
		return Manifest{}, fmt.Errorf("decode app.json: %w", err)
	}
	var trailing json.RawMessage
	switch err := decoder.Decode(&trailing); {
	case err == nil:
		return Manifest{}, fmt.Errorf("decode app.json: trailing JSON token after manifest object")
	case err == io.EOF:
		if limited.N == 0 {
			return Manifest{}, manifestTooLargeError(0)
		}
		return manifest, nil
	default:
		if limited.N == 0 {
			return Manifest{}, manifestTooLargeError(0)
		}
		return Manifest{}, fmt.Errorf("decode app.json: %w", err)
	}
}

func manifestTooLargeError(size uint64) error {
	if size > 0 {
		return fmt.Errorf("app.json too large: %d bytes exceeds %d", size, maxManifestBytes)
	}
	return fmt.Errorf("app.json too large: exceeds %d bytes", maxManifestBytes)
}
