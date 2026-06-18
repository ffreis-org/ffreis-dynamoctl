# ffreis-dynamoctl

<!-- ffreis-badges:start -->
[![CI](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/FelipeFuhr/ffreis-badges/main/badges/ffreis-dynamoctl/ci.json)](https://github.com/FelipeFuhr/ffreis-dynamoctl/actions) [![License](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/FelipeFuhr/ffreis-badges/main/badges/ffreis-dynamoctl/license.json)](https://github.com/FelipeFuhr/ffreis-dynamoctl/blob/main/LICENSE)
<!-- ffreis-badges:end -->

`dynamoctl` is a Go CLI that manages encrypted secrets and configuration values in DynamoDB for the ffreis platform. Values are encrypted client-side with AES-256-GCM before storage, so no KMS or external key management is required — supply the same 64-character hex key at write and read time. Items are organized into a table and a namespace, support optional plaintext storage, in-place key rotation, and S3 backup/restore.

## What it does

- Stores key-value pairs in a DynamoDB table, encrypted by default with AES-256-GCM.
- Reads values back, decrypting transparently when an encryption key is supplied (or returns the raw ciphertext on request).
- Lists keys in a namespace (metadata only — values are never printed by `list`).
- Deletes keys, with an optional existence check.
- Rotates encryption keys: re-encrypts every encrypted item in a namespace from the old key to a new one, using a conditional update to avoid lost writes.
- Backs up items to a JSON file in S3 and restores them back into DynamoDB.
- Emits `text` (default) or `json` output and uses exit codes `0` success, `1` error, `2` not found.

Keys are scoped by `--table` (default `dynamoctl`) and `--namespace` (default `default`).

## Usage

Set the encryption key once (a 32-byte / 64-hex-char key) and select your AWS profile:

```bash
export DYNAMOCTL_KEY=$(openssl rand -hex 32)
export AWS_PROFILE=ffreis-platform   # or pass --profile
```

The CLI uses the standard AWS SDK credential chain. Pass `--profile`/`-p` (defaults to `$AWS_PROFILE`) and `--region`/`-r` (defaults to `$AWS_REGION`, else `us-east-1`).

### Global flags

| Flag | Env | Default | Description |
|---|---|---|---|
| `--table`, `-t` | `DYNAMOCTL_TABLE` | `dynamoctl` | DynamoDB table name |
| `--namespace`, `-n` | `DYNAMOCTL_NAMESPACE` | `default` | Key namespace |
| `--region`, `-r` | `AWS_REGION` | `us-east-1` | AWS region |
| `--profile`, `-p` | `AWS_PROFILE` | — | AWS CLI profile |
| `--encryption-key` | `DYNAMOCTL_KEY` | — | AES-256 key as 64-char hex string |
| `--output` | — | `text` | Output format: `text`, `json` |
| `--log-level` | — | `info` | `debug`, `info`, `warn`, `error` |

### Commands

```bash
# Store a value (encrypted by default; --no-encrypt for plaintext, --stdin to pipe)
dynamoctl set myapp/api-key ghp_xxxx
cat secret.txt | dynamoctl set myapp/api-key --stdin

# Retrieve + decrypt (--raw prints stored ciphertext without decrypting)
dynamoctl get myapp/api-key

# List keys in the namespace (metadata only)
dynamoctl list

# Delete a key (--force skips the existence check)
dynamoctl delete myapp/api-key

# Re-encrypt every item in the namespace with a new key
dynamoctl rotate --encryption-key $OLD_KEY --new-key $NEW_KEY

# Export items to / restore items from an S3 JSON backup
dynamoctl backup  --bucket my-backups [--all] [--prefix custom-prefix]
dynamoctl restore --bucket my-backups --key <s3-key> [--overwrite]

dynamoctl version [--output json]
```

`backup` preserves encrypted values as-is (no key required to back up); the S3 key follows `<prefix>/<table>/<namespace>/<YYYYMMDD-HHMMSS>.json` (default prefix `dynamoctl-backups`). `restore` skips existing items unless `--overwrite` is given. `rotate` skips plaintext items and counts (but does not abort on) per-item failures.

## Development

Requires Go 1.25+. Common Makefile targets:

```bash
make build          # build the ./dynamoctl binary (trimpath + version ldflags)
make test           # go test -race -shuffle=on ./...
make coverage-gate  # fail if total coverage < 80%
make lint           # golangci-lint run ./...
make fmt-check      # fail if any file is unformatted
make tidy           # go mod tidy + verify
make all            # fmt-check + lint + test + build
make help           # list all targets
```

Git hooks: `make setup` bootstraps and installs lefthook. CI can be run locally via `make ci-local` (act-based GitHub Actions fallback).

## License

MIT. See [LICENSE](LICENSE).
