module github.com/example/core-platform/backend/worker

go 1.25.9

require github.com/example/core-platform/packages/go/platformkit v0.0.0

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.10.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/opensearch-project/opensearch-go/v4 v4.7.3 // indirect
	github.com/stretchr/testify v1.12.0 // indirect
	go.temporal.io/sdk v1.48.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace github.com/example/core-platform/packages/go/platformkit => ../../packages/go/platformkit
