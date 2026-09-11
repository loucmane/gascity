package platforminstall

import (
	"fmt"
	"path/filepath"
)

// Protected siblings are preservation mounts, not additions to the import
// input closure. Ordinary Inputs/Trees must still never overlap writable paths.
func metadataProtectedChildren(manifest Manifest) (map[string]MetadataProtectedTree, error) {
	launch := manifest.Metadata
	children := map[string]MetadataProtectedTree{}
	if launch == nil || len(launch.ProtectedTrees) == 0 {
		return children, nil
	}
	if len(launch.ProtectedTrees) > 16 {
		return nil, fmt.Errorf("protected metadata tree inventory exceeds bound")
	}
	canonical := filepath.Dir(DefaultManifestPath(manifest.CityPath))
	var canonicalParent *MetadataParent
	for index := range launch.Parents {
		if launch.Parents[index].Path == canonical {
			if canonicalParent != nil {
				return nil, fmt.Errorf("duplicate canonical metadata parent")
			}
			canonicalParent = &launch.Parents[index]
		}
	}
	if canonicalParent == nil {
		return nil, fmt.Errorf("protected tree has no bound canonical parent")
	}
	for first, parent := range launch.Parents {
		for _, other := range launch.Parents[first+1:] {
			if metadataContains(parent.Path, other.Path) || metadataContains(other.Path, parent.Path) {
				return nil, fmt.Errorf("overlapping writable metadata parents")
			}
		}
	}
	identities := map[[2]uint64]bool{}
	for _, tree := range launch.ProtectedTrees {
		path := tree.Pin.Path
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Dir(path) != canonical {
			return nil, fmt.Errorf("protected tree must be a canonical direct child")
		}
		identity := [2]uint64{tree.Device, tree.Inode}
		if _, exists := children[path]; exists || identities[identity] || tree.Device == 0 || tree.Inode == 0 {
			return nil, fmt.Errorf("duplicate or unbound protected tree identity")
		}
		if tree.UID != canonicalParent.UID || tree.GID != canonicalParent.GID || tree.Pin.Mode & ^uint32(0o755) != 0 {
			return nil, fmt.Errorf("protected tree owner or mode refused")
		}
		if err := validateSHA256("protected tree", tree.Pin.SHA256); err != nil {
			return nil, err
		}
		conflicts := append(metadataOutputs(manifest), launch.Evidence, launch.Writer.Path, launch.GCHome)
		conflicts = append(conflicts, launch.Absent...)
		for _, output := range metadataOutputs(manifest) {
			conflicts = append(conflicts, metadataStage(manifest, output), metadataStage(manifest, output)+".rollback")
		}
		for _, parent := range launch.Parents {
			if parent.Path != canonical {
				conflicts = append(conflicts, parent.Path)
			}
		}
		for _, pins := range [][]FilePin{launch.Inputs, launch.Trees, launch.Runtime} {
			for _, pin := range pins {
				conflicts = append(conflicts, pin.Path)
			}
		}
		for _, link := range launch.Links {
			target := link.Target
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(link.Path), target)
			}
			conflicts = append(conflicts, link.Path, filepath.Clean(target))
		}
		for _, other := range conflicts {
			if other != "" && (metadataContains(path, other) || metadataContains(other, path)) {
				return nil, fmt.Errorf("protected tree overlaps another metadata authority")
			}
		}
		identities[identity] = true
		children[path] = tree
	}
	return children, nil
}
