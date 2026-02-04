package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var pythonCmd *exec.Cmd

// RunApi はAPIサーバーを起動し、HTTPリクエストを処理する
func RunApi(done <-chan struct{}, myID uint32) {
	// GitHubから最新のmain.pyを取得してPythonプロセスを開始
	if err := restartPythonProcess(); err != nil {
		log.Printf("Pythonプロセス開始エラー（プログラムは継続します）: %v", err)
	}

	listener, err := net.Listen("tcp", PORT)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	log.Printf("API Server listening on %s", PORT)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}
		log.Println("Remote API Connected by", conn.RemoteAddr())
		go handleRequest(conn)
	}
}

// sendHTTPResponse はHTTPレスポンスを送信するヘルパー関数である
func sendHTTPResponse(conn net.Conn, statusCode int, contentType string, body string) {
	statusText := map[int]string{
		200: "OK",
		400: "Bad Request",
		500: "Internal Server Error",
	}
	fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\n", statusCode, statusText[statusCode])
	fmt.Fprintf(conn, "Content-Type: %s; charset=utf-8\r\n", contentType)
	fmt.Fprintf(conn, "Content-Length: %d\r\n", len(body))
	fmt.Fprintf(conn, "\r\n")
	fmt.Fprint(conn, body)
}

// sendErrorResponse はエラーレスポンスを送信するヘルパー関数である
func sendErrorResponse(conn net.Conn, statusCode int) {
	statusText := map[int]string{
		400: "Bad Request",
		500: "Internal Server Error",
	}
	body := fmt.Sprintf("%d %s\r\n", statusCode, statusText[statusCode])
	sendHTTPResponse(conn, statusCode, "text/plain", body)
}

// handleRequest はHTTPリクエストを処理する
func handleRequest(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 1024)
	if _, err := conn.Read(buf); err != nil {
		log.Printf("Read error: %v", err)
		return
	}

	request := string(buf)
	parts := strings.Split(request, " ")
	if len(parts) < 2 {
		sendErrorResponse(conn, 400)
		return
	}

	method := parts[0]
	path := parts[1]

	if method != "GET" {
		sendErrorResponse(conn, 400)
		return
	}

	pathParts := strings.Split(path, "/")
	if len(pathParts) < 2 {
		sendErrorResponse(conn, 400)
		return
	}

	endpoint := pathParts[1]

	switch endpoint {
	case "buzzer":
		handleBuzzer(conn, pathParts)
	case "ignorebatterylow":
		handleIgnoreBatteryLow(conn)
	case "image":
		handleImage(conn)
	case "updatepython":
		handleUpdatePython(conn)
	case "changeadjustment":
		handleChangeAdjustment(conn, pathParts)
	default:
		handleStatus(conn)
	}
}

// handleBuzzer はブザーAPIを処理する
func handleBuzzer(conn net.Conn, pathParts []string) {
	if len(pathParts) < 5 {
		sendErrorResponse(conn, 400)
		return
	}

	tone, err := strconv.Atoi(pathParts[3])
	if err != nil || tone < 0 || tone > 99 {
		sendErrorResponse(conn, 400)
		return
	}

	duration, err := strconv.Atoi(pathParts[4])
	if err != nil || duration < 50 || duration > 3000 {
		sendErrorResponse(conn, 400)
		return
	}

	sendHTTPResponse(conn, 200, "text/plain", "BUZZER OK\r\n")
	ringBuzzer(tone, time.Duration(duration)*time.Millisecond, 0)
}

// handleIgnoreBatteryLow はバッテリー低下警告無視APIを処理する
func handleIgnoreBatteryLow(conn net.Conn) {
	alarmIgnore = true
	sendHTTPResponse(conn, 200, "text/plain", "IGNORE BATTERY LOW OK\r\n")
}

// handleImage は画像取得APIを処理する
func handleImage(conn net.Conn) {
	response, err := json.Marshal(imageResponse.Frame)
	if err != nil {
		sendErrorResponse(conn, 500)
		return
	}
	sendHTTPResponse(conn, 200, "application/json", string(response))
}

// handleUpdatePython はPython更新APIを処理する
func handleUpdatePython(conn net.Conn) {
	sendHTTPResponse(conn, 200, "text/plain", "UPDATE PYTHON OK\r\n")
	if err := restartPythonProcess(); err != nil {
		log.Printf("Pythonプロセス再起動エラー: %v", err)
	}
}

// handleChangeAdjustment はしきい値変更APIを処理する
func handleChangeAdjustment(conn net.Conn, pathParts []string) {
	if len(pathParts) < 6 {
		sendErrorResponse(conn, 400)
		return
	}

	minThreshold := pathParts[2]
	maxThreshold := pathParts[3]

	ballDetectRadius, err := strconv.Atoi(pathParts[4])
	if err != nil {
		sendErrorResponse(conn, 400)
		return
	}

	circularityThreshold, err := strconv.ParseFloat(pathParts[5], 32)
	if err != nil {
		sendErrorResponse(conn, 400)
		return
	}

	// しきい値設定をJSONファイルに保存
	data := Adjustment{
		MinThreshold:         minThreshold,
		MaxThreshold:         maxThreshold,
		BallDetectRadius:     ballDetectRadius,
		CircularityThreshold: float32(circularityThreshold),
	}

	if err := saveAdjustmentToFile(data); err != nil {
		log.Printf("しきい値保存エラー: %v", err)
		sendErrorResponse(conn, 500)
		return
	}

	sendHTTPResponse(conn, 200, "text/plain", "CHANGE ADJUSTMENT OK\r\n")

	if err := restartPythonProcess(); err != nil {
		log.Printf("Pythonプロセス再起動エラー: %v", err)
	}
}

// saveAdjustmentToFile はしきい値設定をJSONファイルに保存する
func saveAdjustmentToFile(data Adjustment) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("JSON変換エラー: %w", err)
	}
	return os.WriteFile("threshold.json", jsonData, 0644)
}

// handleStatus はステータス取得APIを処理する
func handleStatus(conn net.Conn) {
	detectPhotoSensor := recvdata.SensorInformation&SENSOR_PHOTO_MASK != 0
	detectDribblerSensor := recvdata.SensorInformation&SENSOR_DRIBBLER_MASK != 0
	isNewDribbler := recvdata.SensorInformation&SENSOR_NEW_DRIB_MASK != 0

	response := fmt.Sprintf(`{
		"VOLT": %f,
		"ISDETECTPHOTOSENSOR": %t,
		"ISDETECTDRIBBLERSENSOR": %t,
		"ISNEWDRIBBLER": %t,
		"ERROR": %t,
		"ERRORCODE": %d,
		"ERRORMESSAGE": "%s"
	}`, float32(recvdata.Volt)/10.0, detectPhotoSensor, detectDribblerSensor, isNewDribbler, isRobotError, RobotErrorCode, RobotErrorMessage)

	sendHTTPResponse(conn, 200, "application/json", response)
}

const (
	pythonScriptURL   = "https://raw.githubusercontent.com/Rione/ssl-RACOON-Pi2/refs/heads/master/main.py"
	pythonScriptFile  = "main.py"
	httpClientTimeout = 5 * time.Second
)

// updateMainPyFromGitHub はGitHubからmain.pyを取得し、必要に応じて更新する
func updateMainPyFromGitHub() error {
	client := &http.Client{Timeout: httpClientTimeout}

	resp, err := client.Get(pythonScriptURL)
	if err != nil {
		return fmt.Errorf("GitHubからファイルを取得できませんでした: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTPエラー: %d", resp.StatusCode)
	}

	githubContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("レスポンス読み取りエラー: %w", err)
	}

	// ローカルファイルと比較
	localContent, err := os.ReadFile(pythonScriptFile)
	if err == nil && string(localContent) == string(githubContent) {
		log.Println("main.pyは最新です。更新の必要はありません。")
		return nil
	}

	if err != nil {
		log.Println("ローカルのmain.pyが見つかりません。新規作成します。")
	}

	if err := os.WriteFile(pythonScriptFile, githubContent, 0644); err != nil {
		return fmt.Errorf("ファイル書き込みエラー: %w", err)
	}

	log.Println("main.pyが正常に更新されました。")
	return nil
}

// restartPythonProcess はPythonプロセスを停止して再起動する
func restartPythonProcess() error {
	// 既存のプロセスを停止
	if pythonCmd != nil && pythonCmd.Process != nil {
		log.Println("既存のPythonプロセスを停止します。")
		if err := pythonCmd.Process.Kill(); err != nil {
			log.Printf("プロセス停止エラー: %v", err)
		}
		pythonCmd.Wait()
	}

	// GitHubから最新のmain.pyを取得・更新を試行
	if err := updateMainPyFromGitHub(); err != nil {
		log.Printf("main.py更新エラー（ローカルファイルで継続）: %v", err)
	}

	// 新しいプロセスを開始
	log.Println("Pythonプロセスを開始します。")
	pythonCmd = exec.Command("python3", pythonScriptFile)
	if err := pythonCmd.Start(); err != nil {
		return fmt.Errorf("Pythonプロセス開始エラー: %w", err)
	}

	log.Println("Pythonプロセスが正常に開始されました。")
	return nil
}
