# BayEOS API

BayEOS API is the Go HTTP service for BayEOS V3. It stores application data in PostgreSQL and time-series data in QuestDB. On startup, it applies migrations to both databases and serves the API under a configurable URL context path.

## Requirements

- Go 1.27 or later
- PostgreSQL and QuestDB, reachable using PostgreSQL connection URLs
- The migration files and static assets included in this repository

## Configuration

The following environment variables are required when running the API directly:

| Variable                   | Description                                                      |
| -------------------------- | ---------------------------------------------------------------- |
| `PG_DB_URL`                | PostgreSQL connection URL for application data.                  |
| `QUEST_DB_URL`             | QuestDB PostgreSQL-wire connection URL.                          |
| `MIGRATIONS_PATH`          | Directory containing the `pg/` and `questdb/` migration folders. |
| `WS_API_PORT`              | HTTP port the API listens on.                                    |
| `WS_API_KEY`               | Secret key used to issue ID tokens.                              |
| `WS_LOG_LEVEL`             | Log level, such as `info` or `debug`.                            |
| `WS_GIN_MODE`              | Gin mode: `release`, `debug`, or `test`.                         |
| `WS_ID_TOKEN_EXPIRES_DAYS` | ID token lifetime in days.                                       |
| `WS_ASSETS_PATH`           | Directory containing static assets.                              |

`WS_BASE_PATH` is optional and defaults to `bayeos-api`. Set it to change the URL prefix. For example, the default version endpoint is `/bayeos-api/version`.

Optional settings include:

| Variable                      | Description                                                              |
| ----------------------------- | ------------------------------------------------------------------------ |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP gRPC endpoint for traces and metrics. Defaults to `localhost:4317`. |
| `WS_TRUSTED_PROXIES`          | Proxy addresses trusted for client IP detection.                         |
| `WS_CORS_HEADER`              | Set to `true` to enable the API's CORS middleware.                       |
| `WS_RESPONSE_DELAY`           | Artificial response delay in milliseconds, useful for testing.           |
| `WS_LDAP_ENABLED`             | Set to `true` to enable LDAP authentication; defaults to disabled.       |

When `WS_LDAP_ENABLED=true`, also provide `WS_LDAP_URL`, `WS_LDAP_BASE`, `WS_LDAP_SIGNIN_ATTRIBUTE`, `WS_LDAP_DISPLAY_ATTRIBUTE`, and `WS_LDAP_EMAIL_ATTRIBUTE`.

The Docker image sets `MIGRATIONS_PATH=/migrations` and `WS_ASSETS_PATH=/assets`. For a local run from this directory, these are typically `./migrations` and `./assets`. Database URLs contain credentials; URL-encode reserved characters and keep secrets out of version control.

## Run Locally

Set the required environment variables, ensure both databases are reachable, and run from this directory:

```sh
go run .
```

## Build and Test

Run the test suite:

```sh
go test ./...
```

Build the binary:

```sh
go build -o bayeos-api .
```

Build the Docker image:

```sh
docker build -t bayeos-api:local .
```

The image includes the API binary, migrations, and static assets. The Docker build accepts `VERSION`, `COMMIT`, and `BUILD_DATE` arguments for build metadata.

# How to release

```
git tag v0.2.0
git push origin v0.2.0
```
