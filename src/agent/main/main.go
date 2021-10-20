package main

import (
	"time"
	"fmt"

	"github.com/openshift/assisted-installer-agent/src/commands"
	"github.com/openshift/assisted-installer-agent/src/config"
	"github.com/openshift/assisted-installer-agent/src/util"
	log "github.com/sirupsen/logrus"
)

const defaultRetryDelay = 1 * time.Minute

func main() {
	config.ProcessArgs()
	util.SetLogging("agent_registration", config.GlobalAgentConfig.TextLogging, config.GlobalAgentConfig.JournalLogging)

	for {
		stepRunnerCommand := commands.RegisterHostWithRetry()
		if stepRunnerCommand == nil {
			log.Errorf("Incompatible server version, going to retry in %s", defaultRetryDelay)
			time.Sleep(defaultRetryDelay)
			continue
		}

        firstEnvArgIndex := -1
        for i, arg := range(stepRunnerCommand.Args) {
            if arg == "--env" {
                firstEnvArgIndex = i
            }
        }

        stepRunnerCommand.Args = append(
            stepRunnerCommand.Args[:firstEnvArgIndex],
            append([]string{fmt.Sprintf("--env=CONTAINERS_STORAGE_CONF=%s", config.GlobalAgentConfig.ContainerStorage)},
            stepRunnerCommand.Args[firstEnvArgIndex:]...)...
        )

		stepRunnerCommand.Args = append(stepRunnerCommand.Args, fmt.Sprintf("--force-mac=%s", config.GlobalAgentConfig.ForceMac))

		if err := commands.StartStepRunner(stepRunnerCommand.Command, stepRunnerCommand.Args...); err != nil {

			var reRegistrerDelay time.Duration
			if stepRunnerCommand.RetrySeconds > 0 {
				reRegistrerDelay = time.Duration(stepRunnerCommand.RetrySeconds) * time.Second
			} else {
				reRegistrerDelay = defaultRetryDelay
			}

			log.WithError(err).Errorf("Next step runner has crashed and will be restarted in %s", reRegistrerDelay)
			time.Sleep(reRegistrerDelay)
			continue
		}

		log.Info("Next step runner exited, going to re-register host")
	}
}
