package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/openshift/assisted-installer-agent/src/commands"
	"github.com/openshift/assisted-installer-agent/src/config"
	"github.com/openshift/assisted-installer-agent/src/session"
	"github.com/openshift/assisted-installer-agent/src/util"
	"github.com/openshift/assisted-service/models"
	log "github.com/sirupsen/logrus"
)

const mediaPreloadLoopDevice = "/dev/loop61"
const mediaPreloadLoopDeviceBackingFile = "/tmp/assisted-preloaded-metal-image"

const ostreeRepo = "/sysroot/ostree/repo"
const osmetFilesDir = "/run/coreos-installer/osmet"

// TODO: Dynamically calculate this by parsing the osmet file
// We can't make this too big, otherwise we'll run out of memory
// on the minimum allowed 8GB RAM machines. We'll need to figure out
// compression in that case.
const mediaPreloadDDBlockSize = "1M"
const mediaPreloadDDBlockCount = 5120

func findOsmetFilePath() (string, error) {
	// Find rhcos file
	stdout, stderr, exitCode := util.ExecutePrivileged("ls", osmetFilesDir)
	if exitCode != 0 {
		log.Errorf("Failed to find rhcos file: %s", stderr)
		return "", fmt.Errorf("Failed to find rhcos file: %s", stderr)
	}

	// Find metal (non metal4k) file
	var metalFile string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "metal") && !strings.Contains(line, "metal4k") {
			metalFile = line
			break
		}
	}

	if metalFile == "" {
		return "", errors.New("failed to find metal file")
	}

	return fmt.Sprintf("%s/%s", osmetFilesDir, metalFile), nil
}

func setupMediaPreloadRAMBlockDevice() error {
	// First check that loop device doesn't exist
	_, _, exitCode := util.ExecutePrivileged("ls", mediaPreloadLoopDevice)
	if exitCode == 0 {
		log.Debugf("loop device %s already exists", mediaPreloadLoopDevice)
	} else {
		if _, stderr, exitCode := util.ExecutePrivileged("dd", "if=/dev/zero", fmt.Sprintf("of=%s", mediaPreloadLoopDeviceBackingFile),
			fmt.Sprintf("bs=%s", mediaPreloadDDBlockSize), fmt.Sprintf("count=%d", mediaPreloadDDBlockCount)); exitCode != 0 {
			return fmt.Errorf("Failed to create backing file for loop device (%d): %s", exitCode, stderr)
		}

		if _, stderr, exitCode := util.ExecutePrivileged("losetup", mediaPreloadLoopDevice, mediaPreloadLoopDeviceBackingFile); exitCode != 0 {
			return fmt.Errorf("Failed to create loop device %s (%d): %s", mediaPreloadLoopDevice, exitCode, stderr)
		}
	}

	return nil
}

// coreosInstallerPreloadMedia creates the metal image on our RAM block device,
// this will be later used by the coreos-installer to install the OS without having
// to access the virtual media anymore.
func coreosInstallerPreloadMedia(osmetFilePath string) error {
	_, stderr, exitCode := util.ExecutePrivileged("coreos-installer", "dev", "extract", "osmet", "--osmet", osmetFilePath, ostreeRepo, mediaPreloadLoopDevice)

	if exitCode != 0 {
		return fmt.Errorf("Failed to extract osmet file %s (%d): %s", osmetFilePath, exitCode, stderr)
	}

	return nil
}

// preloadMedia tries to preload the media and updates the status for the service
func preloadMedia(ctx context.Context, cancel context.CancelFunc, agentConfig *config.AgentConfig) {
	log.Info("Media preloader goroutine started")

	api := commands.NewServiceAPI(agentConfig)
	inventorySession, err := session.New(agentConfig, agentConfig.TargetURL, agentConfig.PullSecretToken, log.StandardLogger())

	if err != nil {
		log.WithError(err).Error("Failed to create inventory session")
		return
	}

	if agentConfig.DryRunEnabled {
		api.PostPreloadStatus(inventorySession, models.NewPreloadStatus(models.PreloadStatusPreloaded))
		ctx.Done()
		return
	}

	api.PostPreloadStatus(inventorySession, models.NewPreloadStatus(models.PreloadStatusPreloading))

	for {
		select {
		case <-ctx.Done():
			log.Info("Media preloader goroutine exiting")
			return
		default:
			if err := setupMediaPreloadRAMBlockDevice(); err != nil {
				log.WithError(err).Error("Failed to setup media preload backing block device")
				api.PostPreloadStatus(inventorySession, models.NewPreloadStatus(models.PreloadStatusPreloadingFailed))
				continue
			}

			osmetFilePath, err := findOsmetFilePath()
			if err != nil {
				log.WithError(err).Errorf("Failed to find rhcos file")
				api.PostPreloadStatus(inventorySession, models.NewPreloadStatus(models.PreloadStatusPreloadingFailed))
				continue
			}

			if err := coreosInstallerPreloadMedia(osmetFilePath); err != nil {
				log.WithError(err).Error("Failed to preload media")
				api.PostPreloadStatus(inventorySession, models.NewPreloadStatus(models.PreloadStatusPreloadingFailed))
				continue
			}

			api.PostPreloadStatus(inventorySession, models.NewPreloadStatus(models.PreloadStatusPreloaded))
			break
		}
	}
}

func main() {
	agentConfig := config.ProcessArgs()
	config.ProcessDryRunArgs(&agentConfig.DryRunConfig)
	util.SetLogging("agent_next_step_runner", agentConfig.TextLogging, agentConfig.JournalLogging, agentConfig.StdoutLogging, agentConfig.ForcedHostID)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup
	wg.Add(1)
	toolRunnerFactory := commands.NewToolRunnerFactory()
	go commands.ProcessSteps(ctx, cancel, agentConfig, toolRunnerFactory, &wg, log.StandardLogger())
	go preloadMedia(ctx, cancel, agentConfig)

	if agentConfig.DryRunEnabled {
		log.Info(`Dry run enabled, will cancel goroutine on fake "reboot"`)
		for {
			if util.DryRebootHappened(&agentConfig.DryRunConfig) {
				log.Info("Dry reboot happened, exiting")
				cancel()
				break
			}

			time.Sleep(time.Second)
		}
	} else {
		// Nothing interesting to do, wait for the goroutine to finish naturally
		wg.Wait()
	}

	log.Info("next step runner exiting")
}
