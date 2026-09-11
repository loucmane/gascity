package platforminstall

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
)

const (
	metadataProtocol      = "gc.metadata-adoption-channel.v1"
	metadataReceiptSchema = "gc.platform-install-receipt.v2"
	metadataFrameLimit    = 128 * 1024
)

// MetadataManifestSchema versions the confined metadata-only installation path.
const MetadataManifestSchema = "gc.platform-install-manifest.v2"

// MetadataHost identifies the exact host process and configured TCP listener.
type MetadataHost struct {
	PID        int    `json:"pid"`
	Boot       string `json:"boot"`
	Start      string `json:"start"`
	Executable string `json:"executable"`
	Address    string `json:"address"`
	Listener   string `json:"listener"`
	City       string `json:"city"`
}

// MetadataPreimage pins an output before mutation; an empty digest means absent.
type MetadataPreimage struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
}

// MetadataLink binds an input symlink to its exact target.
type MetadataLink struct {
	Path   string `json:"path"`
	Target string `json:"target"`
}

// MetadataParent binds an output directory and its permitted contents.
type MetadataParent struct {
	Path    string   `json:"path"`
	Device  uint64   `json:"device"`
	Inode   uint64   `json:"inode"`
	UID     uint32   `json:"uid"`
	GID     uint32   `json:"gid"`
	Mode    uint32   `json:"mode"`
	Entries []string `json:"entries"`
}

// MetadataLaunch is the reviewed, immutable production mount and input contract.
type MetadataLaunch struct {
	Transaction   string             `json:"transaction"`
	Attempt       string             `json:"attempt"`
	Host          MetadataHost       `json:"host"`
	Namespaces    map[string]string  `json:"namespaces"`
	Writer        FilePin            `json:"writer"`
	Runtime       []FilePin          `json:"runtime"`
	Inputs        []FilePin          `json:"inputs"`
	Trees         []FilePin          `json:"trees"`
	Absent        []string           `json:"absent"`
	Links         []MetadataLink     `json:"links"`
	Parents       []MetadataParent   `json:"parents"`
	GCHome        string             `json:"gc_home"`
	CacheSHA256   string             `json:"cache_sha256"`
	ImportsSHA256 string             `json:"imports_sha256"`
	Preimages     []MetadataPreimage `json:"preimages"`
	Evidence      string             `json:"evidence"`
}

// MetadataBinding binds a live inherited channel and its immutable deadline.
type MetadataBinding struct {
	Protocol    string `json:"protocol"`
	Transaction string `json:"transaction"`
	Attempt     string `json:"attempt"`
	Nonce       string `json:"nonce"`
	Channel     string `json:"channel"`
	Request     string `json:"request"`
	Instance    string `json:"instance"`
	Deadline    int64  `json:"deadline"`
	LeaseEnd    int64  `json:"lease_end"`
	Observation bool   `json:"observation,omitempty"`
}

// MetadataCommit records bounded transaction evidence, not continuing liveness.
type MetadataCommit struct {
	Binding MetadataBinding `json:"binding"`
	Host    MetadataHost    `json:"host"`
}

type metadataRequest struct {
	Manifest Manifest        `json:"manifest"`
	Binding  MetadataBinding `json:"binding"`
	Proof    RuntimeProof    `json:"proof"`
	PlanOnly bool            `json:"plan_only"`
}

type metadataFrame struct {
	Binding  MetadataBinding `json:"binding"`
	Sequence int             `json:"sequence"`
	Kind     string          `json:"kind"`
	Digest   string          `json:"digest"`
	Receipt  *Receipt        `json:"receipt,omitempty"`
	Steps    []PlanStep      `json:"steps,omitempty"`
}

func metadataEncode(output io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > metadataFrameLimit {
		return fmt.Errorf("metadata frame size refused")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	if count, err := output.Write(header[:]); err != nil {
		return err
	} else if count != len(header) {
		return io.ErrShortWrite
	}
	count, err := output.Write(data)
	if err == nil && count != len(data) {
		return io.ErrShortWrite
	}
	return err
}

func metadataDecode(input io.Reader, target any) error {
	var header [4]byte
	if _, err := io.ReadFull(input, header[:]); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > metadataFrameLimit {
		return fmt.Errorf("metadata frame size refused")
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(input, data); err != nil {
		return err
	}
	if err := rejectDuplicateMetadataFields(data, target); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func metadataCheckFrame(frame metadataFrame, binding MetadataBinding, kind string, sequence int, now int64) error {
	if frame.Binding != binding || frame.Kind != kind || frame.Sequence != sequence || now < 0 || now >= binding.Deadline {
		return fmt.Errorf("metadata channel identity, order or deadline refused")
	}
	return nil
}

func validateMetadataBinding(binding MetadataBinding) error {
	if binding.Protocol != metadataProtocol || binding.Deadline <= 0 || binding.LeaseEnd <= binding.Deadline || binding.LeaseEnd-binding.Deadline < 2_000_000_000 {
		return fmt.Errorf("invalid metadata channel domain or deadline margin")
	}
	for name, value := range map[string]string{"transaction": binding.Transaction, "attempt": binding.Attempt, "nonce": binding.Nonce, "channel": binding.Channel, "request": binding.Request, "instance": binding.Instance} {
		if err := validateSHA256(name, value); err != nil {
			return err
		}
	}
	return nil
}

func metadataOutputs(manifest Manifest) []string {
	outputs := []string{DefaultManifestPath(manifest.CityPath), manifest.ReceiptPath}
	if manifest.PreviousMetadata != nil {
		outputs = append(outputs, manifest.PreviousMetadata.ManifestBackupPath, manifest.PreviousMetadata.ReceiptBackupPath)
	}
	return outputs
}

func validateMetadataLaunch(manifest Manifest) error {
	launch := manifest.Metadata
	if launch == nil || manifest.Schema != MetadataManifestSchema || manifest.Activation == nil {
		return fmt.Errorf("metadata-only adoption requires a versioned confined launch contract")
	}
	for name, value := range map[string]string{"transaction": launch.Transaction, "attempt": launch.Attempt, "cache": launch.CacheSHA256, "writer": launch.Writer.SHA256, "imports": launch.ImportsSHA256} {
		if err := validateSHA256(name, value); err != nil {
			return err
		}
	}
	if len(launch.Inputs) == 0 || len(launch.Inputs) > 2048 || len(launch.Runtime) != 6 || len(launch.Namespaces) != 6 {
		return fmt.Errorf("metadata runtime/input/namespace inventory incomplete")
	}
	if len(launch.Trees) == 0 || len(launch.Trees) > 128 || len(launch.Absent) > 2048 || len(launch.Links) > 2048 || len(launch.Parents) == 0 || len(launch.Parents) > 5 {
		return fmt.Errorf("metadata closure or parent inventory is incomplete")
	}
	for _, space := range []string{"user", "pid", "mnt", "net", "ipc", "time"} {
		if launch.Namespaces[space] == "" {
			return fmt.Errorf("missing metadata namespace %s", space)
		}
	}
	if launch.Host.PID <= 1 || launch.Host.Boot == "" || launch.Host.Start == "" || launch.Host.Listener == "" || launch.Host.City == "" {
		return fmt.Errorf("incomplete metadata host binding")
	}
	paths := append(metadataOutputs(manifest), launch.Evidence, launch.GCHome, launch.Host.Executable, launch.Writer.Path)
	paths = append(paths, launch.Absent...)
	for _, parent := range launch.Parents {
		paths = append(paths, parent.Path)
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
			return fmt.Errorf("noncanonical metadata path %q", path)
		}
	}
	absent := map[string]bool{}
	for _, path := range launch.Absent {
		if absent[path] {
			return fmt.Errorf("duplicate metadata absence")
		}
		absent[path] = true
	}
	outputs := metadataOutputs(manifest)
	if len(launch.Preimages) != len(outputs) {
		return fmt.Errorf("metadata preimage write set differs")
	}
	for index, output := range outputs {
		pin := launch.Preimages[index]
		if pin.Path != output {
			return fmt.Errorf("metadata output order differs")
		}
		if pin.SHA256 != "" {
			if err := validateSHA256("preimage", pin.SHA256); err != nil {
				return err
			}
		}
	}
	return nil
}
