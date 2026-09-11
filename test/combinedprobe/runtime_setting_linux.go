package main

import (
	"fmt"
	"reflect"
)

func childEnvironment() []string {
	return []string{"GODEBUG=containermaxprocs=0"}
}

func validateChildLaunch(value plan) error {
	if !reflect.DeepEqual(value.ChildEnvironment, childEnvironment()) {
		return fmt.Errorf("child runtime environment drift/override refused")
	}
	if len(value.Argv) != 1 || !reflect.DeepEqual(value.Cases, cases) {
		return fmt.Errorf("child argv inventory differs")
	}
	if !reflect.DeepEqual(value.Argv["writer_template"], writerArgv("CASE")) {
		return fmt.Errorf("argv drift")
	}
	return nil
}
