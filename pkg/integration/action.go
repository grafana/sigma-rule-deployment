package integration

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/go-github/github"
)

type Integration struct {
	Client *github.Client
}

func NewIntegration(gh *github.Client) *Integration {
	return &Integration{
		Client: gh,
	}
}

func (a *Integration) Run(input_files []string) error {
	for _, input_file := range input_files {
		raw_queries, err := os.ReadFile(input_file)
		if err != nil {
			return err
		}

		queries := strings.Split(string(raw_queries), "\n\n") // Taken from the Sigma source code
		if len(queries) == 0 {
			return fmt.Errorf("no queries found in file: %s", input_file)
		}

		for i, query := range queries {

		}
	}
}
