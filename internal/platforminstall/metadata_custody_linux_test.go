package platforminstall

import (
	"runtime/debug"
	"testing"
)

func metadataCustodyBuildFixture() *debug.BuildInfo {
	return &debug.BuildInfo{
		GoVersion: "go1.26.7", Path: "github.com/gastownhall/gascity/cmd/gc", Main: debug.Module{Path: "github.com/gastownhall/gascity"},
		Settings: []debug.BuildSetting{{Key: "-buildmode", Value: "exe"}, {Key: "-compiler", Value: "gc"}, {Key: "-trimpath", Value: "true"}, {Key: "CGO_ENABLED", Value: "0"}, {Key: "GOARCH", Value: "amd64"}, {Key: "GOOS", Value: "linux"}, {Key: "GOAMD64", Value: "v1"}},
		Deps:     []*debug.Module{{Path: "github.com/danielgtaylor/huma/v2", Version: "v2.37.3", Sum: "h1:6Av0Vj45Vk5lDxRVfoO2iPlEdvCvwLc7pl5nbqGOkYM="}},
	}
}

func TestMetadataCustodyBuildProfileRefusesUnauditedInputs(t *testing.T) {
	for _, failure := range []string{"none", "missing", "toolchain", "program", "module", "cgo", "race", "tags", "overlay", "missing-setting", "duplicate-setting", "replacement", "main-replacement", "huma-version", "huma-source", "missing-huma", "duplicate-huma"} {
		t.Run(failure, func(t *testing.T) {
			info := metadataCustodyBuildFixture()
			switch failure {
			case "missing":
				info = nil
			case "toolchain":
				info.GoVersion = "go1.26.6"
			case "program":
				info.Path += "-other"
			case "module":
				info.Main.Path += "-other"
			case "cgo":
				info.Settings[3].Value = "1"
			case "race", "tags", "overlay":
				info.Settings = append(info.Settings, debug.BuildSetting{Key: "-" + failure, Value: "true"})
			case "missing-setting":
				info.Settings = info.Settings[1:]
			case "duplicate-setting":
				info.Settings[0] = info.Settings[1]
			case "replacement":
				info.Deps[0].Replace = &debug.Module{Path: "/tmp/replacement"}
			case "main-replacement":
				info.Main.Replace = &debug.Module{Path: "/tmp/replacement"}
			case "huma-version":
				info.Deps[0].Version = "v2.0.0"
			case "huma-source":
				info.Deps[0].Sum = "other"
			case "missing-huma":
				info.Deps = nil
			case "duplicate-huma":
				info.Deps = append(info.Deps, info.Deps[0])
			}
			if err := validateMetadataCustodyBuild(info); (err == nil) != (failure == "none") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
