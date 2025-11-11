package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/grafana/sigma-rule-deployment/internal/deploy"
	"github.com/grafana/sigma-rule-deployment/internal/integrate"
	"github.com/grafana/sigma-rule-deployment/internal/querytest"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: sigma-deployer <command> [args...]")
		fmt.Println("Commands:")
		fmt.Println("  integrate  - Integrate Sigma rules")
		fmt.Println("  deploy     - Deploy alert rules")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "integrate":
		integrator := integrate.NewIntegrator()
		if err := integrator.LoadConfig(); err != nil {
			fmt.Printf("Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		// Run integrator (conversions and cleanup)
		if err := integrator.Run(); err != nil {
			fmt.Printf("Error running integrator: %v\n", err)
			os.Exit(1)
		}

		// Run query testing if enabled
		config := integrator.Config()
		if config.IntegratorConfig.TestQueries {
			// Parse timeout from configuration
			timeoutDuration := 10 * time.Second // Default timeout
			if config.DeployerConfig.Timeout != "" {
				parsedTimeout, err := time.ParseDuration(config.DeployerConfig.Timeout)
				if err != nil {
					fmt.Printf("Warning: Invalid timeout format in config, using default: %v\n", err)
				} else {
					timeoutDuration = parsedTimeout
				}
			}

			queryTester := querytest.NewQueryTester(
				config,
				integrator.TestFiles(),
				timeoutDuration,
			)
			if err := queryTester.Run(); err != nil {
				if !config.IntegratorConfig.ContinueOnQueryTestingErrors {
					fmt.Printf("Error running query tests: %v\n", err)
					os.Exit(1)
				}
			}
		}
	case "deploy":
		ctx := context.Background()
		deployer := deploy.NewDeployer()

		if err := deployer.LoadConfig(ctx); err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		deployer.SetClient()

		var err error
		if deployer.IsFreshDeploy() {
			err = deployer.ConfigFreshDeployment(ctx)
		} else {
			err = deployer.ConfigNormalMode()
		}
		if err != nil {
			fmt.Printf("Error configuring deployment: %v\n", err)
			os.Exit(1)
		}

		// Deploy alerts
		alertsCreated, alertsUpdated, alertsDeleted, errDeploy := deployer.Deploy(ctx)

		// Write action outputs
		if err := deployer.WriteOutput(alertsCreated, alertsUpdated, alertsDeleted); err != nil {
			fmt.Printf("Error writing output: %v\n", err)
			os.Exit(1)
		}

		// We only check the deployment error AFTER writing the output so that
		// we still report the alerts that were created, updated and deleted before the error
		if errDeploy != nil {
			fmt.Printf("Error deploying: %v\n", errDeploy)
			os.Exit(1)
		}
	default:
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Usage: sigma-deployer <command> [args...]")
		fmt.Println("Commands:")
		fmt.Println("  integrate  - Integrate Sigma rules")
		fmt.Println("  deploy     - Deploy alert rules")
		os.Exit(1)
	}
}
