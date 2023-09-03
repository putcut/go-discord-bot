# Go Discord Bot
Amazon EC2 上で動かすプライベート用 Minecraft Server のための Discord Bot

## Features
- Amazon EC2 Instance の起動停止

## Environment
- 環境変数 `INSTANCE_ID` を設定しておく。
- `~/.aws/credentials` に AWS の認証情報を書いておく。

## Docker
```
docker image build -t go-discord-bot .
docker container run --rm -v /home/vscode/.aws/credentials:/home/nonroot/.aws/credentials go-discord-bot
```

## Docker Compose
```
docker compose build
docker compose up
```