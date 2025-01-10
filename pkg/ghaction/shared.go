package ghaction

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

func NewGithubClientFromEnv() (*github.Client, error) {
	ghToken := GetInputOrDefault("GITHUB_TOKEN", "")
	if ghToken == "" {
		return nil, errors.New("missing input gh-token")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: ghToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc), nil
}

func GetInputOrDefault(name string, value string) string {
	envName := "INPUT_" + strings.ToUpper(strings.ReplaceAll(name, " ", "_"))

	env := os.Getenv(envName)
	if env == "" {
		return value
	}

	return env
}

func Repository() (string, string, string, error) {
	e := os.Getenv("GITHUB_REPOSITORY")
	if e == "" {
		return "", "", "", errors.New("missing env var GITHUB_REPOSITORY")
	}

	parts := strings.Split(e, "/")
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("expected 2 parts (owner + repo) in GITHUB_REPOSITORY, but got %v", len(parts))
	}

	sha := os.Getenv("GITHUB_SHA")
	if sha == "" {
		return "", "", "", errors.New("missing env var GITHUB_SHA")
	}

	return parts[0], parts[1], sha, nil
}

func SetOutput(output, value string) error {
	outputFile := os.Getenv("GITHUB_OUTPUT")
	if outputFile == "" {
		return errors.New("only output with a github output file supported. See https://github.blog/changelog/2022-10-11-github-actions-deprecating-save-state-and-set-output-commands/ for further details")
	}

	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("unable to open output file, due %w", err)
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "%v=%v\n", output, value)
	if err != nil {
		return fmt.Errorf("unable to write to output file, due %w", err)
	}

	return nil
}
