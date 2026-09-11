package platforminstall

import (
	"bytes"
	"debug/buildinfo"
	"fmt"
	"os"
	"runtime/debug"
)

func validateMetadataCustodyBuild(info *debug.BuildInfo) error {
	if info == nil || info.GoVersion != "go1.26.7" || info.Path != "github.com/gastownhall/gascity/cmd/gc" || info.Main.Path != "github.com/gastownhall/gascity" || info.Main.Replace != nil {
		return fmt.Errorf("metadata accepted-socket custody build profile unavailable")
	}
	wanted := map[string]string{"-buildmode": "exe", "-compiler": "gc", "-trimpath": "true", "CGO_ENABLED": "0", "GOARCH": "amd64", "GOOS": "linux", "GOAMD64": "v1"}
	if len(info.Settings) != len(wanted) {
		return fmt.Errorf("metadata custody build settings not audited")
	}
	for _, setting := range info.Settings {
		value, exists := wanted[setting.Key]
		if !exists || value != setting.Value {
			return fmt.Errorf("metadata custody build setting %q refused", setting.Key)
		}
		delete(wanted, setting.Key)
	}
	if len(wanted) != 0 {
		return fmt.Errorf("metadata custody build settings incomplete")
	}
	framework := 0
	for _, dependency := range info.Deps {
		if dependency.Replace != nil {
			return fmt.Errorf("metadata custody dependency replacements refused")
		}
		if dependency.Path == "github.com/danielgtaylor/huma/v2" {
			if dependency.Version != "v2.37.3" || dependency.Sum != "h1:6Av0Vj45Vk5lDxRVfoO2iPlEdvCvwLc7pl5nbqGOkYM=" {
				return fmt.Errorf("metadata custody Huma implementation not audited")
			}
			framework++
		}
	}
	if framework != 1 {
		return fmt.Errorf("metadata custody Huma evidence incomplete")
	}
	return nil
}

func metadataCustodyImage(expected string) error {
	image, err := os.ReadFile("/proc/self/exe")
	if err != nil {
		return err
	}
	if sha256Hex(image) != expected {
		return fmt.Errorf("metadata custody requires the exact reviewed verifier and supervisor image")
	}
	info, err := buildinfo.Read(bytes.NewReader(image))
	if err != nil {
		return err
	}
	return validateMetadataCustodyBuild(info)
}
