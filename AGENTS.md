# Agent Context

**This repo:** `ffreis-dynamoctl` — CLI for DynamoDB table inspection, scanning, and
management. Used primarily for Terraform lock inspection and debugging platform
configuration state.

## Non-obvious facts

- **Companion to `platform-configctl`** — dynamoctl is for raw table operations
  (listing, scanning, exporting); configctl is for structured config/secret management.
  They target different use cases on the same DynamoDB tables.

- **Used for Terraform lock management.** Lock keys are stored in
  `{org}-tf-locks-{env}` tables. Stale locks from interrupted applies must be
  removed via dynamoctl before Terraform can proceed.

- **Exit codes:** 0 = success, 1 = error, 2 = not found.

## Structure

```
cmd/dynamoctl/    ← Cobra CLI entry point
internal/         ← config, AWS SDK helpers, output formatting
```

## Build/run

```bash
make build
./bin/dynamoctl list --table ffreis-tf-locks-prod
./bin/dynamoctl get --table ffreis-tf-locks-prod --key <lock-id>
./bin/dynamoctl delete --table ffreis-tf-locks-prod --key <lock-id>
```
