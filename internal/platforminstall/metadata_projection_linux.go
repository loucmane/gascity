package platforminstall

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// A directory alias may name only the synthetic directories already needed for
// explicit read-only file mounts. It adds neither a host mount nor import-read
// authority. Host and writer must resolve the declared projection identically.
func metadataValidateLinks(manifest Manifest) error {
	launch := manifest.Metadata
	links := make(map[string]string, len(launch.Links))
	for _, link := range launch.Links {
		if !filepath.IsAbs(link.Path) || filepath.Clean(link.Path) != link.Path || link.Path == "/" || link.Target == "" {
			return fmt.Errorf("invalid metadata link path or target")
		}
		if _, exists := links[link.Path]; exists {
			return fmt.Errorf("duplicate metadata link")
		}
		links[link.Path] = link.Target
	}
	for _, link := range launch.Links {
		// Inputs are mounted first. A later symlink must not replace a mount or
		// redirect where another declared mount/link is materialized.
		for _, pin := range append(append([]FilePin{}, launch.Inputs...), launch.Trees...) {
			if metadataPathsOverlap(link.Path, pin.Path) {
				return fmt.Errorf("metadata link overlaps readonly mount")
			}
		}
		for other := range links {
			if other != link.Path && metadataContains(other, link.Path) {
				return fmt.Errorf("nested metadata link materialization refused")
			}
		}
		target, err := os.Readlink(link.Path)
		if err != nil || target != link.Target {
			return fmt.Errorf("metadata symlink drift")
		}
		resolved, err := filepath.EvalSymlinks(link.Path)
		if err != nil {
			return err
		}
		projected, err := metadataProjectedPath(link.Path, links)
		if err != nil || projected != resolved {
			return fmt.Errorf("host and projected metadata link resolution differ")
		}
		for _, path := range []string{link.Path, resolved} {
			if path == "/" {
				return fmt.Errorf("metadata root alias refused")
			}
			for _, forbidden := range []string{"/proc", "/sys", "/dev", "/run"} {
				if metadataContains(forbidden, path) {
					return fmt.Errorf("metadata link exposes host authority")
				}
			}
			parents := []string{launch.Evidence}
			for _, output := range metadataOutputs(manifest) {
				parents = append(parents, filepath.Dir(output))
			}
			for _, parent := range launch.Parents {
				parents = append(parents, parent.Path)
			}
			for _, parent := range parents {
				if metadataPathsOverlap(path, parent) {
					return fmt.Errorf("metadata link overlaps writable parent")
				}
			}
			for _, tree := range launch.ProtectedTrees {
				if metadataPathsOverlap(path, tree.Pin.Path) {
					return fmt.Errorf("metadata link overlaps protected tree")
				}
			}
		}
		if !metadataReadCovered(launch, resolved) && !metadataSyntheticDirectory(launch, resolved) {
			return fmt.Errorf("metadata symlink escapes input closure")
		}
	}
	return nil
}

func metadataPathsOverlap(first, second string) bool {
	return metadataContains(first, second) || metadataContains(second, first)
}

func metadataSyntheticDirectory(launch *MetadataLaunch, path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	// This exception is for projected directories, not a new broad host tree.
	for _, tree := range launch.Trees {
		if metadataPathsOverlap(path, tree.Path) {
			return false
		}
	}
	for _, pin := range launch.Inputs {
		if pin.Path != path && metadataContains(path, pin.Path) {
			return true
		}
	}
	return false
}

// Resolve only declared symlinks, component by component. Cleaning a complete
// target before following intermediate links would misinterpret "link/..".
// Undeclared host aliases therefore cannot accidentally bless a different W path.
func metadataProjectedPath(path string, links map[string]string) (string, error) {
	parts := strings.Split(path, string(filepath.Separator))
	var current []string
	hops := 0
	for len(parts) > 0 {
		part := parts[0]
		parts = parts[1:]
		switch part {
		case "", ".":
			continue
		case "..":
			if len(current) == 0 {
				return "", fmt.Errorf("projected link escapes root")
			}
			current = current[:len(current)-1]
			continue
		}
		candidate := "/" + strings.Join(append(append([]string{}, current...), part), "/")
		if target, found := links[candidate]; found {
			hops++
			if hops > 64 {
				return "", fmt.Errorf("projected link cycle or depth bound")
			}
			if filepath.IsAbs(target) {
				current = nil
			}
			parts = append(strings.Split(target, "/"), parts...)
			continue
		}
		current = append(current, part)
	}
	return "/" + strings.Join(current, "/"), nil
}
