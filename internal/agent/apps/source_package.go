package apps

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func packageSourceArchive(ctx context.Context, sourceArchivePath string, destination string) error {
	parent := filepath.Dir(destination)
	tempDir, err := os.MkdirTemp(parent, ".source-package-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if err := extractArchive(sourceArchivePath, tempDir); err != nil {
		return err
	}
	sourceRoot, err := findAppSourceRoot(tempDir)
	if err != nil {
		return err
	}
	manifest, err := readInstalledManifest(sourceRoot)
	if err != nil {
		return err
	}
	runtimePath, err := containedPath(sourceRoot, manifest.Runtime.Command)
	if err != nil {
		return err
	}
	if _, err := os.Stat(runtimePath); errors.Is(err, os.ErrNotExist) {
		if err := buildSourceRuntime(ctx, sourceRoot, manifest.Runtime.Command, runtimePath); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := os.Chmod(runtimePath, 0o700); err != nil {
		return err
	}
	return writePackagedSourceArchive(sourceRoot, manifest, destination)
}

func findAppSourceRoot(root string) (string, error) {
	if fileExists(filepath.Join(root, "app.json")) {
		return root, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	roots := make([]string, 0, 1)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(root, entry.Name())
		if fileExists(filepath.Join(candidate, "app.json")) {
			roots = append(roots, candidate)
		}
	}
	switch len(roots) {
	case 1:
		return roots[0], nil
	case 0:
		return "", fmt.Errorf("app.json is required at the source root")
	default:
		return "", fmt.Errorf("source archive contains multiple app roots")
	}
}

func buildSourceRuntime(ctx context.Context, sourceRoot string, runtimeCommand string, runtimePath string) error {
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("runtime command %q is missing and go is not available on the agent PATH", runtimeCommand)
	}
	if err := os.MkdirAll(filepath.Dir(runtimePath), 0o700); err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "go", "build", "-o", runtimePath, ".")
	command.Dir = sourceRoot
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		return fmt.Errorf("build runtime command %q: %w: %s", runtimeCommand, err, strings.TrimSpace(output.String()))
	}
	return nil
}

func writePackagedSourceArchive(sourceRoot string, manifest Manifest, destination string) error {
	paths, err := collectPackageSourcePaths(sourceRoot, manifest)
	if err != nil {
		return err
	}
	tempPath := destination + ".tmp"
	defer os.Remove(tempPath)

	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(file)
	for _, relative := range paths {
		if err := writePackageSourceEntry(archive, sourceRoot, relative, relative == manifest.Runtime.Command); err != nil {
			_ = archive.Close()
			_ = file.Close()
			return err
		}
	}
	if err := archive.Close(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, destination)
}

func collectPackageSourcePaths(sourceRoot string, manifest Manifest) ([]string, error) {
	paths := map[string]struct{}{
		"app.json":                {},
		manifest.Runtime.Command: {},
	}
	if fileExists(filepath.Join(sourceRoot, "README.md")) {
		paths["README.md"] = struct{}{}
	}
	for _, schemaPath := range referencedSchemaPaths(manifest) {
		paths[schemaPath] = struct{}{}
	}
	for _, directory := range []string{"schemas", "assets"} {
		if err := addPackageDirectoryPaths(sourceRoot, directory, paths); err != nil {
			return nil, err
		}
	}
	out := make([]string, 0, len(paths))
	for relative := range paths {
		cleaned, ok := cleanPackagePath(relative)
		if !ok {
			return nil, fmt.Errorf("source package path %q must be a relative clean path", relative)
		}
		absolute, err := containedPath(sourceRoot, cleaned)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(absolute)
		if err != nil {
			return nil, fmt.Errorf("source package path %q is missing: %w", cleaned, err)
		}
		if info.IsDir() {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("source package path %q is a symlink", cleaned)
		}
		out = append(out, cleaned)
	}
	sort.Strings(out)
	return out, nil
}

func addPackageDirectoryPaths(sourceRoot string, directory string, paths map[string]struct{}) error {
	absolute, err := containedPath(sourceRoot, directory)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absolute); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(absolute, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			relative, _ := filepath.Rel(sourceRoot, path)
			return fmt.Errorf("source package path %q is a symlink", filepath.ToSlash(relative))
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		paths[filepath.ToSlash(relative)] = struct{}{}
		return nil
	})
}

func writePackageSourceEntry(archive *zip.Writer, sourceRoot string, relative string, executable bool) error {
	absolute, err := containedPath(sourceRoot, relative)
	if err != nil {
		return err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("source package path %q is a symlink", relative)
	}
	mode := fileMode(info.Mode().Perm())
	if executable {
		mode = 0o700
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(relative)
	header.Method = zip.Deflate
	header.SetMode(mode)
	writer, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}
	file, err := os.Open(absolute)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(writer, file)
	return err
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
