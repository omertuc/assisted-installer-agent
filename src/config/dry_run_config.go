package config

import (
	"flag"
	"fmt"
	"os"

	"github.com/kelseyhightower/envconfig"
)

// DryRunConfig defines configuration of the agent's dry-run mode
type DryRunConfig struct {
	DryRunEnabled    bool   `envconfig:"DRY_ENABLE"`
	ForcedHostID     string `envconfig:"DRY_HOST_ID"`
	ForcedMacAddress string `envconfig:"DRY_MAC_ADDRESS"`
}

var GlobalDryRunConfig DryRunConfig

var DefaultDryRunConfig DryRunConfig = DryRunConfig{
	DryRunEnabled:    false,
	ForcedHostID:     "",
	ForcedMacAddress: "",
}

func ProcessDryRunArgs() {
	err := envconfig.Process("dryconfig", &DefaultDryRunConfig)
	if err != nil {
		fmt.Printf("envconfig error: %v", err)
		os.Exit(1)
	}

	flag.BoolVar(&GlobalDryRunConfig.DryRunEnabled, "dry-run", DefaultDryRunConfig.DryRunEnabled, "Dry run avoids/fakes certain actions while communicating with the service")
	flag.StringVar(&GlobalDryRunConfig.ForcedHostID, "force-id", DefaultDryRunConfig.ForcedHostID, "The fake host ID to give to the host")
	flag.StringVar(&GlobalDryRunConfig.ForcedMacAddress, "force-mac", DefaultDryRunConfig.ForcedMacAddress, "The fake mac address to give to the first network interface")
	flag.Parse()
}
