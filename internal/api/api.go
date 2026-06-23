package api

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

	"github.com/Rione/ssl-RACOON-Pi2/internal/link"
	"github.com/Rione/ssl-RACOON-Pi2/internal/mw"
	"github.com/Rione/ssl-RACOON-Pi2/internal/state"
)

var pythonCmd *exec.Cmd

func Run(done <-chan struct{}, myID uint32) {
	if err := restartPythonProcess(); err != nil {
		log.Printf("Pythonプロセス開始エラー（プログラムは継続します）: %v", err)
	}

	listener, err := net.Listen("tcp", state.Port)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	log.Printf("API Server listening on %s", state.Port)

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

func sendErrorResponse(conn net.Conn, statusCode int) {
	statusText := map[int]string{
		400: "Bad Request",
		500: "Internal Server Error",
	}
	body := fmt.Sprintf("%d %s\r\n", statusCode, statusText[statusCode])
	sendHTTPResponse(conn, statusCode, "text/plain", body)
}

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
	link.RingBuzzerSync(tone, time.Duration(duration)*time.Millisecond, 0)
}

func handleIgnoreBatteryLow(conn net.Conn) {
	state.AlarmIgnore = true
	sendHTTPResponse(conn, 200, "text/plain", "IGNORE BATTERY LOW OK\r\n")
}

func handleImage(conn net.Conn) {
	response, err := json.Marshal(state.ImageResponseData.Frame)
	if err != nil {
		sendErrorResponse(conn, 500)
		return
	}
	sendHTTPResponse(conn, 200, "application/json", string(response))
}

func handleUpdatePython(conn net.Conn) {
	sendHTTPResponse(conn, 200, "text/plain", "UPDATE PYTHON OK\r\n")
	if err := restartPythonProcess(); err != nil {
		log.Printf("Pythonプロセス再起動エラー: %v", err)
	}
}

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

	data := state.Adjustment{
		MinThreshold:         minThreshold,
		MaxThreshold:         maxThreshold,
		BallDetectRadius:     ballDetectRadius,
		CircularityThreshold: float32(circularityThreshold),
	}

	if err := mw.SaveAdjustmentConfig(data); err != nil {
		log.Printf("しきい値保存エラー: %v", err)
		sendErrorResponse(conn, 500)
		return
	}

	sendHTTPResponse(conn, 200, "text/plain", "CHANGE ADJUSTMENT OK\r\n")

	if err := restartPythonProcess(); err != nil {
		log.Printf("Pythonプロセス再起動エラー: %v", err)
	}
}

func handleStatus(conn net.Conn) {
	detectPhotoSensor := state.Recvdata.SensorInformation&state.SensorPhotoMask != 0
	detectDribblerSensor := state.Recvdata.SensorInformation&state.SensorDribblerMask != 0
	isNewDribbler := state.Recvdata.SensorInformation&state.SensorNewDribMask != 0

	response := fmt.Sprintf(`{
		"VOLT": %f,
		"ISDETECTPHOTOSENSOR": %t,
		"ISDETECTDRIBBLERSENSOR": %t,
		"ISNEWDRIBBLER": %t,
		"ERROR": %t,
		"ERRORCODE": %d,
		"ERRORMESSAGE": "%s"
	}`, float32(state.Recvdata.Volt)/10.0, detectPhotoSensor, detectDribblerSensor, isNewDribbler, state.IsRobotError, state.RobotErrorCode, state.RobotErrorMessage)

	sendHTTPResponse(conn, 200, "application/json", response)
}

const (
	pythonScriptURL   = "https://raw.githubusercontent.com/Rione/ssl-RACOON-Pi2/refs/heads/master/main.py"
	pythonScriptFile  = "main.py"
	httpClientTimeout = 5 * time.Second
)

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

func restartPythonProcess() error {
	if pythonCmd != nil && pythonCmd.Process != nil {
		log.Println("既存のPythonプロセスを停止します。")
		if err := pythonCmd.Process.Kill(); err != nil {
			log.Printf("プロセス停止エラー: %v", err)
		}
		pythonCmd.Wait()
	}

	if err := updateMainPyFromGitHub(); err != nil {
		log.Printf("main.py更新エラー（ローカルファイルで継続）: %v", err)
	}

	log.Println("Pythonプロセスを開始します。")
	pythonCmd = exec.Command("python3", pythonScriptFile)
	if err := pythonCmd.Start(); err != nil {
		return fmt.Errorf("Pythonプロセス開始エラー: %w", err)
	}

	log.Println("Pythonプロセスが正常に開始されました。")
	return nil
}
