package main

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
)

func RunApi(chapi chan bool, MyID uint32) {
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

	// 200 OKを返す
	fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\n")
	// UTF-8指定
	fmt.Fprintf(conn, "Content-Type: application/json; charset=utf-8\r\n\r\n")

	// JSON形式で返す
	response := fmt.Sprintf(`{
		"VOLT": %f,
		"ISHOLDBALL": %t,
		"ERROR": %t,
		"ERRORCODE": %d,
		"ERRORMESSAGE": "%s"
	}`, float32(recvdata.Volt)/10.0, recvdata.IsDetectPhotosensor, isRobotError, RobotErrorCode, RobotErrorMessage)

	fmt.Fprint(conn, response)

}
