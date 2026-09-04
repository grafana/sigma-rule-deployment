module github.com/grafana/sigma-rule-deployment

go 1.26.6

require (
	github.com/google/uuid v1.6.0
	github.com/jarcoal/httpmock v1.4.2
	github.com/prometheus/common v0.71.0
	github.com/spaolacci/murmur3 v1.1.0
	github.com/stretchr/testify v1.12.1
	gopkg.in/yaml.v3 v3.0.1
)

require go.yaml.in/yaml/v3 v3.0.5 // indirect

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	golang.org/x/text v0.41.0
	google.golang.org/protobuf v1.36.12 // indirect
)
