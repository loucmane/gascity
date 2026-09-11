package main

import (
	"context"
	"reflect"
	"testing"
)

func childLaunchPlan() plan {
	value := plan{ChildEnvironment: childEnvironment(), Argv: map[string][]string{"writer_template": writerArgv("CASE")}, Cases: append([]string{}, cases...)}
	return value
}

func TestChildRuntimeSettingExact(t *testing.T) {
	want := []string{"GODEBUG=containermaxprocs=0"}
	if !reflect.DeepEqual(childEnvironment(), want) {
		t.Fatal(childEnvironment())
	}
	cmd := command(context.Background(), []string{binaryPath, "verifier", "success"})
	if !reflect.DeepEqual(cmd.Env, want) || !reflect.DeepEqual(cmd.Environ(), want) {
		t.Fatal("child environment inherited overrides", cmd.Env)
	}
	changed := childEnvironment()
	changed[0] = "GODEBUG=containermaxprocs=1"
	if !reflect.DeepEqual(childEnvironment(), want) {
		t.Fatal("shared mutable child setting")
	}
}

func TestChildLaunchRefusesOverridesAndDrift(t *testing.T) {
	if err := validateChildLaunch(childLaunchPlan()); err != nil {
		t.Fatal(err)
	}
	for _, environment := range [][]string{nil, {}, {"GOMAXPROCS=1"}, {"GODEBUG=containermaxprocs=1"}, {"GODEBUG=containermaxprocs=0,other=1"}, {"GODEBUG=containermaxprocs=0", "GODEBUG=containermaxprocs=1"}, {"GODEBUG=containermaxprocs=0", "GOMAXPROCS=1"}, {"GODEBUG=containermaxprocs=0", "PATH=/unreviewed"}} {
		value := childLaunchPlan()
		value.ChildEnvironment = environment
		if validateChildLaunch(value) == nil {
			t.Fatal("unreviewed environment accepted", environment)
		}
	}
	for _, name := range []string{"writer_template"} {
		value := childLaunchPlan()
		args := value.Argv[name]
		setting := -1
		for index := 0; index+2 < len(args); index++ {
			if args[index] == "--setenv" && args[index+1] == "GODEBUG" {
				if setting != -1 {
					t.Fatal("duplicate setting")
				}
				setting = index
				if args[index+2] != "containermaxprocs=0" {
					t.Fatal(args)
				}
			}
		}
		if setting == -1 {
			t.Fatal("missing namespace runtime setting")
		}
		args[setting+2] = "containermaxprocs=1"
		if validateChildLaunch(value) == nil {
			t.Fatal("namespace setting drift accepted")
		}
		value = childLaunchPlan()
		value.Argv[name] = append(value.Argv[name], "--setenv", "GODEBUG", "containermaxprocs=1")
		if validateChildLaunch(value) == nil {
			t.Fatal("extra override accepted")
		}
	}
}
