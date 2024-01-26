## Setup

```bash
go install github.com/enkaism/ohayo

ohayo set-token $token
ohayo set-channel-id $channelID
```

## Usage

```bash
// 開始
ohayo start

// 休憩開始
ohayo pause

// 休憩終わり
ohayo resume

// 終了(休憩中でも実行可)
ohayo end
ohayo end "memo"
```
