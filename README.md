# Example of game on TON token

# What is here:

1. Authorization
2. Top-up/withdraw
3. API + 2 games: Rock-Paper-Scissors/Max random

# Stack

1. Echo as API server
2. Postgres as database

# How to run

## Local

```bash
WALLET_SEED="...24words" TONPROOF_PAYLOAD_SIGNATURE_KEY="secret_key" APP_HOST="blala.ngrok-free.app" LOCAL=true WITH_DB_SCHEMA=true  BOT_TOKEN="TELEGRAM_BOT_TOKEN"  go run cmd/app/main.go
```

## Cloud

```bash
WALLET_SEED="...24words" TONPROOF_PAYLOAD_SIGNATURE_KEY="secret_key" DB_DSN="CONN_DSN" APP_HOST="balala.ngrok-free.app" WITH_DB_SCHEMA=true BOT_TOKEN="TELEGRAM_BOT_TOKEN" go run cmd/app/main.go
```

