package main

import (
	"flag"
	"fmt"

	"github.com/openshift/assisted-installer-agent/src/config"
	"github.com/openshift/assisted-installer-agent/src/inventory"
	"github.com/openshift/assisted-installer-agent/src/util"
)

type Config struct {
	ForceMac string
}

var executableConfig Config

func processArgs() {
	ret := &executableConfig
	flag.StringVar(&ret.ForceMac, "force-mac", "", "Force agent to report a particular mac address in its first network interface")
	flag.Parse()
}

func main() {
	processArgs()
	config.ProcessSubprocessArgs(config.DefaultLoggingConfig)

	util.SetLogging("inventory", config.SubprocessConfig.TextLogging, config.SubprocessConfig.JournalLogging)

	fmt.Print(string(inventory.CreateInventoryInfo(executableConfig.ForceMac)))
}
