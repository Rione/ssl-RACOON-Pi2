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

var cmd *exec.Cmd

func RunApi(chapi chan bool, MyID uint32) {
	// GitHubから最新のmain.pyを取得してPythonプロセスを開始
	err := restartPythonProcess()
	if err != nil {
		log.Printf("Pythonプロセス開始エラー（プログラムは継続します）: %v", err)
		// エラーがあってもAPIサーバーは継続
	}
	//ポートを開く
	listener, err := net.Listen("tcp", PORT)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	//接続を待つ
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal(err)
		}
		// log
		log.Println("Remote API Connected by ", conn.RemoteAddr())
		//接続があったら処理を行う
		go HandleRequest(conn)
	}
}

// 接続があったときの処理
func HandleRequest(conn net.Conn) {
	defer conn.Close()

	//リクエストを解析
	buf := make([]byte, 1024)
	_, err := conn.Read(buf)
	if err != nil {
		log.Println(err)
		return
	}

	// リクエストを解析
	// リクエストヘッダーの1行目を取得
	request := string(buf)
	// リクエストヘッダーの1行目をスペースで区切る
	requests := strings.Split(request, " ")
	// リクエストヘッダーの1行目からリクエストの種類を取得
	requestType := requests[0]

	// リクエストの種類がGETでなければエラーを返す
	if requestType != "GET" {
		fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\n")
		fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
		fmt.Fprintf(conn, "400 Bad Request\r\n")
		return
	}

	// リクエストのパスが"/buzzer"の場合
	if strings.Split(requests[1], "/")[1] == "buzzer" {
		// tone が指定されていない場合
		if len(requests) < 3 {
			fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "400 Bad Request\r\n")
			return
		}
		// buzzer/ の後に tone が指定されている場合
		log.Println(strings.Split(requests[1], "/")[1])
		tone, err := strconv.Atoi(strings.Split(requests[1], "/")[3])
		if err != nil {
			fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "400 Bad Request\r\n")
			return
		}
		duration, err := strconv.Atoi(strings.Split(requests[1], "/")[4])
		if err != nil {
			fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "400 Bad Request\r\n")
			return
		}
		// tone が 0 から 12 でない場合
		if tone < 0 || tone > 15 {
			fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "400 Bad Request\r\n")
			return
		}
		// duration が 50 から 3000 でない場合
		if duration < 50 || duration > 3000 {
			fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "400 Bad Request\r\n")
			return
		}

		// OK と表示
		fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\n")
		fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
		fmt.Fprintf(conn, "BUZZER OK\r\n")
		//ブザーを1秒鳴らす
		ringBuzzer(tone, time.Duration(duration)*time.Millisecond, 0)
		return
	}

	if strings.Split(requests[1], "/")[1] == "ignorebatterylow" {
		// OK と表示
		fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\n")
		fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
		fmt.Fprintf(conn, "IGNORE BATTERY LOW OK\r\n")
		//アラーム無視をセットする
		alarmIgnore = true
		return
	}

	if strings.Split(requests[1], "/")[1] == "image" {

		response, err := json.Marshal(imageResponse.Frame)
		if err != nil {
			fmt.Fprintf(conn, "HTTP/1.1 500 Internal Server Error\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "500 Internal Server Error\r\n")
			return
		}
		// HTTP レスポンスを返す
		fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\n")
		fmt.Fprintf(conn, "Content-Type: application/json\r\n")
		fmt.Fprintf(conn, "Content-Length: %d\r\n", len(response))
		fmt.Fprintf(conn, "\r\n")
		fmt.Fprintf(conn, "%s", response)

	}

	if strings.Split(requests[1], "/")[1] == "updatepython" {
		// OK と表示
		fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\n")
		fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
		fmt.Fprintf(conn, "UPDATE PYTHON OK\r\n")

		// Pythonプロセスを再起動（GitHubからの更新チェック付き）
		err := restartPythonProcess()
		if err != nil {
			log.Printf("Pythonプロセス再起動エラー: %v", err)
		}
		return
	}

	if strings.Split(requests[1], "/")[1] == "changeadjustment" {
		// /changeadjustment/1,120,100/15,255,255/150/0.2これを受け取る
		// /120,100,15をとる
		minThreshold := strings.Split(requests[1], "/")[2]
		maxThreshold := strings.Split(requests[1], "/")[3]
		ballDetectRadius, err := strconv.Atoi(strings.Split(requests[1], "/")[4])
		if err != nil {
			fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "400 Bad Request\r\n")
		}
		circularityThreshold, err := strconv.ParseFloat(strings.Split(requests[1], "/")[5], 32)
		if err != nil {
			fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "400 Bad Request\r\n")
		}
		// OK と表示
		fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\n")
		fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
		fmt.Fprintf(conn, "CHANGE ADJUSTMENT OK\r\n")
		// /changeadjustment/1,120,100/15,255,255/150/0.2を受け取ったら、jsonファイルを変更する
		// jsonファイルを変更する

		os.Remove("threshold.json")
		file, err := os.Create("threshold.json")
		if err != nil {
			log.Println(err)
		}
		defer file.Close()
		// jsonファイルに書き込む
		data := Adjustment{Min_Threshold: minThreshold, Max_Threshold: maxThreshold, Ball_Detect_Radius: ballDetectRadius, Circularity_Threshold: float32(circularityThreshold)}
		jsonData, err := json.Marshal(data)
		if err != nil {
			fmt.Fprintf(conn, "HTTP/1.1 500 Internal Server Error\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "500 Internal Server Error\r\n")
		}
		_, err = file.Write(jsonData)
		if err != nil {
			fmt.Fprintf(conn, "HTTP/1.1 500 Internal Server Error\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "500 Internal Server Error\r\n")
		}
		// jsonファイルを閉じる
		err = file.Close()
		if err != nil {
			fmt.Fprintf(conn, "HTTP/1.1 500 Internal Server Error\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "500 Internal Server Error\r\n")
		}

		// Pythonプロセスを再起動（GitHubからの更新チェック付き）
		err = restartPythonProcess()
		if err != nil {
			log.Printf("Pythonプロセス再起動エラー: %v", err)
		}

		return
	}

	// 200 OKを返す
	fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\n")
	// UTF-8指定
	fmt.Fprintf(conn, "Content-Type: application/json; charset=utf-8\r\n\r\n")

	// JSON形式で返す

	// 左から1ビットだけを取り出す
	detectPhotoSensor := 0b00000001&recvdata.SensorInformation != 0
	// 左から2ビット目だけを取り出す
	detectDribblerSensor := 0b00000010&recvdata.SensorInformation != 0
	// 左から3ビット目だけを取り出す
	isNewDribbler := 0b00000100&recvdata.SensorInformation != 0

	response := fmt.Sprintf(`{
		"VOLT": %f,
		"ISDETECTPHOTOSENSOR": %t,
		"ISDETECTDRIBBLERSENSOR": %t,
		"ISNEWDRIBBLER": %t,
		"ERROR": %t,
		"ERRORCODE": %d,
		"ERRORMESSAGE": "%s"
	}`, float32(recvdata.Volt)/10.0, detectPhotoSensor, detectDribblerSensor, isNewDribbler, isRobotError, RobotErrorCode, RobotErrorMessage)

	fmt.Fprint(conn, response)

}

// GitHubからmain.pyを取得して更新が必要かチェックし、必要に応じて更新する
func updateMainPyFromGitHub() error {
	const githubURL = "https://raw.githubusercontent.com/Rione/ssl-RACOON-Pi2/refs/heads/master/main.py"

	// タイムアウト付きHTTPクライアントを作成
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// GitHubからファイルを取得
	resp, err := client.Get(githubURL)
	if err != nil {
		return fmt.Errorf("gitHubからファイルを取得できませんでした: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("httpエラー: %d", resp.StatusCode)
	}

	// GitHubのファイル内容を読み取り
	githubContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("レスポンス読み取りエラー: %v", err)
	}

	// 現在のローカルファイルを読み取り
	localContent, err := os.ReadFile("main.py")
	if err != nil {
		// ファイルが存在しない場合は新規作成
		log.Println("ローカルのmain.pyが見つかりません。新規作成します。")
	} else {
		// ファイル内容を比較
		if string(localContent) == string(githubContent) {
			log.Println("main.pyは最新です。更新の必要はありません。")
			return nil
		}
	}

	// ファイルを更新
	err = os.WriteFile("main.py", githubContent, 0644)
	if err != nil {
		return fmt.Errorf("ファイル書き込みエラー: %v", err)
	}

	log.Println("main.pyが正常に更新されました。")
	return nil
}

// Pythonプロセスを停止して再起動する
func restartPythonProcess() error {
	// 既存のプロセスを停止
	if cmd != nil && cmd.Process != nil {
		log.Println("既存のPythonプロセスを停止します。")
		err := cmd.Process.Kill()
		if err != nil {
			log.Printf("プロセス停止エラー: %v", err)
		}
		cmd.Wait() // プロセスの終了を待つ
	}

	// GitHubから最新のmain.pyを取得・更新を試行
	err := updateMainPyFromGitHub()
	if err != nil {
		log.Printf("main.py更新エラー（ローカルファイルで継続）: %v", err)
		// エラーがあってもローカルのファイルで実行を継続
	}

	// 新しいプロセスを開始
	log.Println("Pythonプロセスを開始します。")
	cmd = exec.Command("python3", "main.py")
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("pythonプロセス開始エラー: %v", err)
	}

	log.Println("Pythonプロセスが正常に開始されました。")
	return nil
}
