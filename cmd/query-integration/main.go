package main

import (
	"log"
	"strings"

	"github.com/grafana/sigma-rule-deployment/pkg/ghaction"
	"github.com/grafana/sigma-rule-deployment/pkg/integration"
)

func main() {
	logger := log.Default()
	gh, err := ghaction.NewGithubClientFromEnv()
	if err != nil {
		logger.Fatal("unable to create GitHub client from env")
	}

	ifs := ghaction.GetInputOrDefault("query_files", "")
	if ifs == "" {
		logger.Fatal("could not find a non-empty list of query_files")
	}
	ifa := strings.Split(ifs, ";") // TODO: make configurable?
	i := integration.NewIntegration(gh)

	err = i.Run(ifa)
	if err != nil {
		logger.Fatalf("failed processing files: %s", err.Error())
	}
}
