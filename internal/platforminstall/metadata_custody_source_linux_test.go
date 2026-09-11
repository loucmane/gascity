package platforminstall

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMetadataCustodyOwningSourceProfile(t *testing.T) {
	project := map[string]string{
		"cmd/gc/main.go":                           "e45fcd4514d3eb111afa6c0bcef9805dd9629e96e496db3f66008746c7c4f06b",
		"cmd/gc/cmd_supervisor.go":                 "67726fee27d167e5f1720c604bb9b73ec0dc848b6ee0abf6f0691b419bdec19c",
		"cmd/gc/dolt_start_managed.go":             "1050a984f884d09ce548dfce6341699d81e98a224f16b556b585a91020a1c1d2",
		"internal/api/supervisor.go":               "f513589dfa77d547a51280da68e13db5a46d5d660b0384b9149e4797e752141e",
		"internal/api/huma_handlers_supervisor.go": "01bec09f1006f0b1ef78cf65a9f405c7179c2d5a5a112a05eb13db088821f386",
		"internal/api/metadata_lease.go":           "22405e3c423172cfba4c5e9e025a852b44277554883733a65b73f3291aab9fa6",
		"internal/api/middleware.go":               "4668db0734a4ba08160409c47d2caa75f845e1cc32f41ce81207f10a3bcee104",
		"internal/api/writeauth.go":                "10fd588b3c171c4204ffaef4594a72f63e565db32371d3847acd2cf8a29894d8",
		"internal/api/readauth.go":                 "4c564340decbbc8e47fb0221ebdc6db3c64fa5a09df8ef6e700385ee2d5bbf0e",
		"internal/workspacesvc/proxy_process.go":   "c77ed35d81467d4d91824fd030954c00f0c343c1456e3dad9c4cbecd28dbb338",
	}
	toolchain := map[string]string{
		"internal/poll/sock_cloexec.go": "3ae8a5856c703f68eae5343f575727ed6123310c8d4bfc0ebf1814efa8028cfd",
		"net/http/server.go":            "3a085df91bb868c764119fc4a32bd901aff281be485d1559b6353e54d7232f45",
		"os/exec/exec.go":               "0bbfd988045f66e78e7416a1a0d47016ea966c65323d2c6ca7a18cdf4b7b5df0",
		"syscall/exec_linux.go":         "566314c12ef25bf80312c654b58648ce6f94cb427b8097b4b2f10993058d4366",
	}
	auditRoot := runtime.GOROOT() //nolint:staticcheck // Audit-only lookup; exact source hashes and pinned toolchain remain required.
	for root, pins := range map[string]map[string]string{filepath.Join("..", ".."): project, filepath.Join(auditRoot, "src"): toolchain} {
		for name, expected := range pins {
			data, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			if sha256Hex(data) != expected {
				t.Fatalf("accepted-socket custody source %s changed; owning review required", name)
			}
		}
	}
}
