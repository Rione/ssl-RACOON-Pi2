package main

import (
	"bytes"
	"encoding/binary"
	"log"
	"time"

	"go.bug.st/serial"
)

// シリアル通信部分
var isReceived bool = false
var pre_isReceived bool = false

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
	last_recv_time = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	for {
		//受信できるまで読み込む。バイトが0xFF, 0x00, 0xFF, 0x00のときは受信できると判断する
		buf := make([]byte, 1)
		recvbuf := make([]byte, 3)
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
							//合計 3バイト
							for i := 0; i < 3; i++ {
								port.Read(buf)      //読み込み
								recvbuf[i] = buf[0] //受信データを格納
							}
							break
						}
					}
				}
			}
		}
		// log.Println(recvbuf)
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

		// bytearray := SendStruct{}

		// bytearray.preamble = 0xFF
		// bytearray.velx = 0
		// bytearray.vely = 0
		// bytearray.velang = 0
		// bytearray.dribblePower = 0
		// bytearray.kickPower = 0
		// bytearray.chipPower = 0
		// bytearray.relativeX = 0
		// bytearray.relativeY = 0
		// bytearray.relativeTheta = 0
		// bytearray.cameraBallX = 0
		// bytearray.cameraBallY = 0
		// bytearray.informations = 0

		// sendarray = bytes.Buffer{}
		// err := binary.Write(&sendarray, binary.LittleEndian, bytearray) //バイナリに変換
		// if err != nil {
		// 	log.Fatal(err)
		// }

		//クライアントで受け取ったデータをバイト列に変更
		sendbytes := sendarray.Bytes()

		//バイト列がなかったら（初回受け取りを行っていない場合）、初期値を設定
		if len(sendbytes) <= 0 {
			sendbytes = []byte{0xFF, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
		}

		//受信しなかった場合に自動的にモーターOFFする
		if time.Since(last_recv_time) > 1*time.Second {
			// log.Println("No Data Recv")
			// disable velocity, dribble, kick
			for i := 1; i <= 9; i++ {
				sendbytes[i] = 0
			}

			//informations ビットの 5 番目を 0 にする
			sendbytes[18] = sendbytes[18] & 0b11011111
			isReceived = false
		} else {
			sendbytes[18] = sendbytes[18] | 0b00100000
			isReceived = true
		}

		if time.Since(last_recv_time) > 15*time.Second {
			//informations ビットの 4 番目を 0 にする
			sendbytes[18] = sendbytes[18] & 0b11101111
		}

		if !isReceived && pre_isReceived {
			log.Println("No Data Recv")
			go ringBuzzer(3, 500*time.Millisecond, 0)
		}

		if isReceived && !pre_isReceived {
			go ringBuzzer(10, 500*time.Millisecond, 0)
		}

		if kicker_enable {
			sendbytes[8] = kicker_val
		} else {
			sendbytes[8] = 0
		}

		//それぞれのデータを表示
		// log.Printf("VOLT: %f, BALLSENS: %t, IMUDEG: %d\n", float32(recvdata.Volt)*0.1, recvdata.IsHoldBall, recvdata.ImuDir)

		port.Write(sendbytes) //書き込み

		pre_isReceived = isReceived

		//100ナノ秒待つ
		// time.Sleep(100 * time.Nanosecond)
	}
}
