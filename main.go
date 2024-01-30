package main

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/slack-go/slack"
)

const ohayoDir = "/ohayo"
const logDir = "/logs"
const currentLogFile = "/current.csv"
const envFile = "/.ohayo_env"

var homeDir = ""
var token = ""
var channelID = ""
var name = ""

func main() {
	// ユーザー情報を取得
	currentUser, err := user.Current()
	if err != nil {
		fmt.Println("ユーザー情報を取得できませんでした:", err)
		return
	}

	// ホームディレクトリのパス
	homeDir = currentUser.HomeDir
	dir := homeDir + ohayoDir + logDir
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.Mkdir(homeDir+ohayoDir, 0755)
		if err != nil {
			fmt.Println("ディレクトリの作成に失敗しました:", err)
			return
		}
		err = os.Mkdir(dir, 0755)
		if err != nil {
			fmt.Println("ディレクトリの作成に失敗しました:", err)
			return
		}
	}
	if !(len(os.Args) == 2 || (len(os.Args) == 3 && (os.Args[1] == "end" || os.Args[1] == "set-token" || os.Args[1] == "set-channel-id" || os.Args[1] == "set-name"))) {
		fmt.Println("Usage: go run main.go [start|pause|resume|end (memo)]")
		os.Exit(1)
	}

	command := os.Args[1]
	var note string
	if len(os.Args) == 3 {
		note = os.Args[2]
	}
	switch command {
	case "set-token":
		setEnv("SLACK_TOKEN", note)
		return
	case "set-channel-id":
		setEnv("SLACK_CHANNEL_ID", note)
		return
	case "set-name":
		setEnv("SLACK_NAME", note)
		return
	}

	token = getEnv("SLACK_TOKEN")
	if token == "" {
		fmt.Println("SLACK_TOKENが設定されていません")
		return
	}

	channelID = getEnv("SLACK_CHANNEL_ID")
	if channelID == "" {
		fmt.Println("SLACK_CHANNEL_IDが設定されていません")
		return
	}
	name = getEnv("SLACK_NAME")

	switch command {
	case "start":
		Start()
	case "pause":
		Pause()
	case "resume":
		Resume()
	case "end":
		End(note)
	default:
		fmt.Println("不明なコマンドです。[start|pause|resume|end] を指定してください。")
	}
}

func setEnv(key, value string) {
	// ファイルを開く（存在しない場合は新規作成）
	file, err := os.OpenFile(homeDir+ohayoDir+envFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		fmt.Println("ファイルを開く際にエラーが発生しました:", err)
		return
	}
	defer file.Close()

	// ファイルを読み込み、行ごとに処理
	scanner := bufio.NewScanner(file)
	var lines []string

	for scanner.Scan() {
		line := scanner.Text()

		// keyで始まる行があれば新しいvalueに置き換える
		if strings.HasPrefix(line, key) {
			line = key + "=" + value
		}

		lines = append(lines, line)
	}

	// エラーがあれば出力
	if err := scanner.Err(); err != nil {
		fmt.Println("ファイルを読み込む際にエラーが発生しました:", err)
		return
	}

	// ファイルの末尾に新しい行を追加（"TOKEN="で始まる行が見つからなかった場合）
	if !containsKey(lines, key) {
		lines = append(lines, key+"="+value)
	}

	// ファイルをリセットして新しい内容を書き込む
	if err := resetAndWriteFile(file, lines); err != nil {
		fmt.Println("ファイルに書き込む際にエラーが発生しました:", err)
		return
	}
}

func getEnv(key string) string {
	file, err := os.Open(homeDir + ohayoDir + envFile)
	if err != nil {
		fmt.Printf("open file err: %s\n", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		slice := strings.Split(line, "=")
		if len(slice) != 2 {
			fmt.Println("get env failed. len(slice) must be 2")
		}
		if key == slice[0] {
			return slice[1]
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("read file err: %s\n", err)
	}
	return ""
}

func containsKey(lines []string, key string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, key+"=") {
			return true
		}
	}
	return false
}

// ファイルをリセットして新しい内容を書き込む関数
func resetAndWriteFile(file *os.File, lines []string) error {
	// ファイルを先頭に戻してから中身を空にする
	if err := file.Truncate(0); err != nil {
		return err
	}

	// ファイルの先頭に戻す
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}

	// 新しい内容をファイルに書き込む
	writer := bufio.NewWriter(file)
	for _, line := range lines {
		_, err := writer.WriteString(line + "\n")
		if err != nil {
			return err
		}
	}

	// ファイルに変更を反映
	if err := writer.Flush(); err != nil {
		return err
	}

	return nil
}

// 勤務状況を表す構造体
type WorkStatus struct {
	StartTime   time.Time   `csv:"start_time"`
	PauseTimes  []time.Time `csv:"pause_times"`
	ResumeTimes []time.Time `csv:"resume_times"`
	EndTime     time.Time   `csv:"end_time"`
	IsPaused    bool        `csv:"is_paused"`
	IsEnd       bool        `csv:"is_end"`
}

func NewWorkStatus() *WorkStatus {
	return &WorkStatus{
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}
}

func NewCurrentWorkStatus() (*WorkStatus, error) {
	var ws []*WorkStatus
	err := readCSVFile(homeDir+ohayoDir+logDir+currentLogFile, &ws)
	if err != nil {
		return nil, err
	}
	if len(ws) != 1 {
		return nil, err
	}

	return ws[0], nil
}

func createCSVFile(fileName string, ws []*WorkStatus) error {
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	err = gocsv.MarshalFile(ws, file)
	if err != nil {
		return err
	}

	return nil
}

func readCSVFile(fileName string, data interface{}) error {
	file, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	return gocsv.Unmarshal(file, data)
}

// 勤務開始
func Start() {
	currentWs, err := NewCurrentWorkStatus()
	if err != nil || currentWs.IsEnd {
		ws := NewWorkStatus()
		err = createCSVFile(homeDir+ohayoDir+logDir+currentLogFile, []*WorkStatus{ws})
		if err != nil {
			fmt.Println("CSVの書き込みに失敗しました。")
			return
		}
		fmt.Println("勤務を開始しました。")
		return
	}

	fmt.Println("勤務はすでに開始されています。")
}

// 勤務一時停止
func Pause() {
	currentWs, err := NewCurrentWorkStatus()
	if err != nil {
		fmt.Println("CSVの読み込みに失敗しました。")
		return
	}

	if currentWs.IsEnd {
		fmt.Println("勤務はすでに終了しています。")
		return
	}

	if currentWs.IsPaused {
		fmt.Println("勤務はすでに中断しています。")
		return
	}

	currentWs.IsPaused = true
	currentWs.PauseTimes = append(currentWs.PauseTimes, time.Now())
	err = createCSVFile(homeDir+ohayoDir+logDir+currentLogFile, []*WorkStatus{currentWs})
	if err != nil {
		fmt.Println("CSVの書き込みに失敗しました。")
		return
	}
	fmt.Println("勤務を中断しました。")
}

// 勤務再開
func Resume() {
	currentWs, err := NewCurrentWorkStatus()
	if err != nil {
		fmt.Println("CSVの読み込みに失敗しました。")
		return
	}

	if currentWs.IsEnd {
		fmt.Println("勤務はすでに終了しています。")
		return
	}

	if !currentWs.IsPaused {
		fmt.Println("勤務はすでに再開しています。")
		return
	}

	currentWs.IsPaused = false
	currentWs.ResumeTimes = append(currentWs.ResumeTimes, time.Now())
	err = createCSVFile(homeDir+ohayoDir+logDir+currentLogFile, []*WorkStatus{currentWs})
	if err != nil {
		fmt.Println("CSVの書き込みに失敗しました。")
		return
	}
	fmt.Println("勤務を一再開しました。")
}

// 勤務終了
func End(memo string) {
	currentWs, err := NewCurrentWorkStatus()
	if err != nil {
		fmt.Println("CSVの読み込みに失敗しました。")
		return
	}

	if currentWs.IsEnd {
		fmt.Println("勤務はすでに終了しています。")
		return
	}

	now := time.Now()
	if currentWs.IsPaused {
		currentWs.ResumeTimes = append(currentWs.ResumeTimes, now)
	}

	currentWs.IsPaused = false
	currentWs.IsEnd = true
	currentWs.EndTime = now
	err = createCSVFile(homeDir+ohayoDir+logDir+currentLogFile, []*WorkStatus{currentWs})
	if err != nil {
		fmt.Println("CSVの書き込みに失敗しました。")
		return
	}

	notifySlack(currentWs, memo)
	fmt.Println("勤務を終了しました。")
}

// Slackに勤務状況を通知
func notifySlack(ws *WorkStatus, memo string) {
	api := slack.New(token)

	var totalPaused time.Duration
	for i := range ws.PauseTimes {
		totalPaused += ws.ResumeTimes[i].Sub(ws.PauseTimes[i])
	}
	totalTime := ws.EndTime.Sub(ws.StartTime) - totalPaused

	statusMessage := fmt.Sprintf("%s\n%s %s\n勤務開始: %s\n勤務終了: %s\n休憩時間: %s\n%s",
		name,
		ws.StartTime.Format("2006/01/02"),
		durationToTimeString(totalTime),
		ws.StartTime.Format("15:04:05"),
		ws.EndTime.Format("15:04:05"),
		durationToTimeString(totalPaused),
		memo)

	channelID, timestamp, err := api.PostMessage(channelID, slack.MsgOptionText(statusMessage, true))
	if err != nil {
		fmt.Printf("Slack通知エラー: %s\n", err)
		return
	}

	fmt.Printf("Slackに通知しました。Channel: %s, Timestamp: %s\n", channelID, timestamp)
}

func durationToTimeString(duration time.Duration) string {
	m := uint(duration.Minutes())

	h := m / 60
	m = m % 60

	return fmt.Sprintf("%dh%dm", h, m)
}
