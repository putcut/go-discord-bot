# Go Discord Bot
プライベート用 Discord Bot

## AWS Authentication
`~/.aws/credentials` に認証情報を書いておく。
regionも忘れずに。

## Docker
### Build
`docker image build -t go-discord-bot .`
### Run
`docker container run --rm -v /home/vscode/.aws/credentials:/home/nonroot/.aws/credentials go-discord-bot`

## Docker Compose
```
docker compose build
docker compose up
```