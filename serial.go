package main

import (
	"bytes"
	"encoding/binary"
	"log"
	"time"

	"go.bug.st/serial"
)

// シリアル通信部分
func RunSerial(chclient chan bool, MyID uint32) {
	port, err := serial.Open(SERIAL_PORT_NAME, &serial.Mode{})
	if err != nil {
		log.Fatal(err)
	}

	//構造体の宣言
	recvdata = RecvStruct{}

	//シリアル通信のモードをセット
	mode := &serial.Mode{
		BaudRate: BAUDRATE,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	if err := port.SetMode(mode); err != nil {
		log.Fatal(err)
	}

	for {
		//受信できるまで読み込む。バイトが0xFF, 0x00, 0xFF, 0x00のときは受信できると判断する
		buf := make([]byte, 1)
		recvbuf := make([]byte, 6)
		//ここで受信バッファをクリアする
		port.ResetInputBuffer()
		for {
			port.Read(buf) //読み込み
			if bytes.Equal(buf, []byte{0xFF}) {
				port.Read(buf) //読み込み
				if bytes.Equal(buf, []byte{0x00}) {
					port.Read(buf) //読み込み
					if bytes.Equal(buf, []byte{0xFF}) {
						port.Read(buf) //読み込み
						if bytes.Equal(buf, []byte{0x00}) {
							//合計6バイト
							for i := 0; i < 6; i++ {
								port.Read(buf)      //読み込み
								recvbuf[i] = buf[0] //受信データを格納
							}
							break
						}
					}
				}
			}
		}

		//バイナリから構造体に変換
		err = binary.Read(bytes.NewReader(recvbuf), binary.BigEndian, &recvdata)
		CheckError(err)

		//////////////////////////////////
		///
		/// エラーチェック部分
		///
		//////////////////////////////////

		//バッテリの電圧が一定量を下回ったらエラー
		if recvdata.Volt < uint8(BATTERY_LOW_THRESHOULD) {
			isRobotError = true
			RobotErrorCode = 2
			RobotErrorMessage = "バッテリ電圧異常"
		}

		//バッテリの電圧が極端に低いとき（速度に影響を与えるレベル）はエラー
		if recvdata.Volt < uint8(BATTERY_CRITICAL_THRESHOULD) {
			isRobotError = true
			RobotErrorCode = 2
			RobotErrorMessage = "バッテリ電圧異常(回路故障の可能性)"

			// isRobotEmgError = true //緊急停止
		}

		//クライアントで受け取ったデータをバイト列に変更
		sendbytes := sendarray.Bytes()

		//バイト列がなかったら（初回受け取りを行っていない場合）、初期値を設定
		if len(sendbytes) <= 0 {
			sendbytes = []byte{0xFF, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
		}

		//受信しなかった場合に自動的にモーターOFFする
		if time.Since(last_recv_time) > 1*time.Second {
			// log.Println("No Data Recv")
			for i := 1; i <= 6; i++ {
				sendbytes[i] = 0
			}
		}

		if kicker_enable {
			sendbytes[8] = kicker_val
		} else {
			sendbytes[8] = 0
		}

		//それぞれのデータを表示
		// log.Printf("VOLT: %f, BALLSENS: %t, IMUDEG: %d\n", float32(recvdata.Volt)*0.1, recvdata.IsHoldBall, recvdata.ImuDir)

		port.Write(sendbytes)             //書き込み
		time.Sleep(16 * time.Millisecond) //少し待つ
		//log.Printf("Sent %v bytes\n", n)  //何バイト送信した？
		// log.Println(sendbytes) //送信済みのバイトを表示

		//100ナノ秒待つ
		// time.Sleep(100 * time.Nanosecond)
	}
}
