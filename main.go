package main

import (
	"fmt"
	"os"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/slack-go/slack"
)

const logDir = "./logs"
const currentLogFile = "/current.csv"

var token = ""
var channelID = ""

func main() {
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		err := os.Mkdir(logDir, 0755)
		if err != nil {
			fmt.Println("ディレクトリの作成に失敗しました:", err)
			return
		}
	}

	token = os.Getenv("SLACK_TOKEN")
	if token == "" {
		fmt.Println("SLACK_TOKENが設定されていません")
		return
	}

	channelID = os.Getenv("SLACK_CHANNEL_ID")
	if channelID == "" {
		fmt.Println("SLACK_CHANNEL_IDが設定されていません")
		return
	}

	if !(len(os.Args) == 2 || (len(os.Args) == 3 && os.Args[1] == "end")) {
		fmt.Println("Usage: go run main.go [start|pause|resume|end (memo)]")
		os.Exit(1)
	}

	command := os.Args[1]
	var note string
	if len(os.Args) == 3 {
		note = os.Args[2]
	}
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

// 勤務状況を表す構造体
type WorkStatus struct {
	StartTime   time.Time   `csv:"start_time"`
	PauseTimes  []time.Time `csv:"pause_times"`
	ResumeTimes []time.Time `csv:"resume_times"`
	EndTime     time.Time   `csv:"end_time"`
	IsPaused    bool        `csv:"is_paused"`
	IsEnd       bool        `csv:"is_end"`
	Slacktoken  string      `csv:"-"`
}

func NewWorkStatus() *WorkStatus {
	return &WorkStatus{
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}
}

func NewCurrentWorkStatus() (*WorkStatus, error) {
	var ws []*WorkStatus
	err := readCSVFile(logDir+currentLogFile, &ws)
	if err != nil {
		fmt.Println("CSVファイルの読み込みに失敗しました:", err)
		return nil, err
	}
	if len(ws) != 1 {
		fmt.Println("CSVファイルの読み込みに失敗しました:", err)
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
		err = createCSVFile(logDir+currentLogFile, []*WorkStatus{ws})
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
	err = createCSVFile(logDir+currentLogFile, []*WorkStatus{currentWs})
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
	err = createCSVFile(logDir+currentLogFile, []*WorkStatus{currentWs})
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
	err = createCSVFile(logDir+currentLogFile, []*WorkStatus{currentWs})
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

	statusMessage := fmt.Sprintf("%s %s\n勤務開始：%s\n終了：%s\n休憩:%s\n%s",
		ws.StartTime.Format("2006/01/02"),
		fmt.Sprintf("%.0fh%.0fm", totalTime.Hours(), totalTime.Minutes()),
		ws.StartTime.Format("15:04:05"),
		fmt.Sprintf("%.0fh%.0fm", totalPaused.Hours(), totalPaused.Minutes()),
		ws.EndTime.Format("15:04:05"), memo)

	channelID, timestamp, err := api.PostMessage(channelID, slack.MsgOptionText(statusMessage, true))
	if err != nil {
		fmt.Printf("Slack通知エラー: %s\n", err)
		return
	}

	fmt.Printf("Slackに通知しました。Channel: %s, Timestamp: %s\n", channelID, timestamp)
}
