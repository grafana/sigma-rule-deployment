package querytest

import (
	"fmt"
	"net/url"

	"github.com/grafana/sigma-rule-deployment/internal/model"
	"github.com/grafana/sigma-rule-deployment/shared"
)

// GenerateExploreLink creates a Grafana explore link based on the datasource type
func GenerateExploreLink(
	query, datasource, datasourceType string,
	config, defaultConf model.ConversionConfig,
	grafanaInstance, from, to string,
	orgID int64,
) (string, error) {
	customModel := shared.GetConfigValue(config.QueryModel, defaultConf.QueryModel, "")
	escapedQuery, err := shared.EscapeQueryJSON(query)
	if err != nil {
		return "", fmt.Errorf("could not escape provided query: %s", query)
	}

	var pane string
	switch {
	case customModel != "":
		pane = fmt.Sprintf(`{"yyz":{"datasource":"%[1]s","queries":[%[2]s],"range":{"from":"%[3]s","to":"%[4]s"}}}`, datasource, fmt.Sprintf(customModel, "A", datasource, escapedQuery), from, to)
	case datasourceType == shared.Loki:
		pane = fmt.Sprintf(`{"yyz":{"datasource":"%[1]s","queries":[{"refId":"A","expr":"%[2]s","queryType":"range","datasource":{"type":"loki","uid":"%[1]s"},"editorMode":"code","direction":"backward"}],"range":{"from":"%[3]s","to":"%[4]s"}}}`, datasource, escapedQuery, from, to)
	case datasourceType == shared.Elasticsearch:
		// For Elasticsearch, we need to include the full query structure with metrics and bucketAggs
		pane = fmt.Sprintf(`{"yyz":{"datasource":"%[1]s","queries":[{"refId":"A","datasource":{"type":"elasticsearch","uid":"%[1]s"},"query":"%[2]s","alias":"","metrics":[{"type":"count","id":"1"}],"bucketAggs":[{"type":"date_histogram","id":"2","settings":{"interval":"auto"},"field":"@timestamp"}],"timeField":"@timestamp"}],"range":{"from":"%[3]s","to":"%[4]s"},"compact":false}}`, datasource, escapedQuery, from, to)
	default:
		// Fallback to a generic structure
		pane = fmt.Sprintf(`{"yyz":{"datasource":"%[1]s","queries":[{"refId":"A","query":"%[2]s","datasource":{"type":"%[3]s","uid":"%[1]s"}}],"range":{"from":"%[4]s","to":"%[5]s"}}}`, datasource, escapedQuery, datasourceType, from, to)
	}

	return fmt.Sprintf("%s/explore?schemaVersion=1&panes=%s&orgId=%d", grafanaInstance, url.QueryEscape(pane), orgID), nil
}
