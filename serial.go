package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"time"

	"go.bug.st/serial"
)

// シリアル通信部分

var isReceived bool = false
var pre_isReceived bool = false

// imageData.Image_x, Image_y の一瞬0対策用
var prevImageX int = 0
var prevImageY int = 0
var zeroCountX int = 0
var zeroCountY int = 0

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

		// デバッグモード: シリアル受信データの表示
		if debugSerial {
			log.Printf("[Serial RX] Volt: %d (%.1fV), SensorInfo: 0b%08b, CapPower: %d",
				recvdata.Volt, float32(recvdata.Volt)*0.1, recvdata.SensorInformation, recvdata.CapPower)
		}

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

		// image_x, image_y が一瞬0でも5回以内なら前の値を維持
		if imageData.Image_x == 0 {
			zeroCountX++
			if zeroCountX <= 5 {
				sendbytes[16] = byte(prevImageX)
			} else {
				sendbytes[16] = 0
			}
		} else {
			sendbytes[16] = byte(imageData.Image_x)
			prevImageX = int(imageData.Image_x)
			zeroCountX = 0
		}

		if imageData.Image_y == 0 {
			zeroCountY++
			if zeroCountY <= 5 {
				sendbytes[17] = byte(prevImageY)
			} else {
				sendbytes[17] = 0
			}
		} else {
			sendbytes[17] = byte(imageData.Image_y)
			prevImageY = int(imageData.Image_y)
			zeroCountY = 0
		}

		// log.Println("X:", int(sendbytes[16]), "Y:", int(sendbytes[17]))

		//受信しなかった場合に自動的にモーターOFFする(ロボット制御モードではない場合)
		if time.Since(last_recv_time) > 1*time.Second && !isControlByRobotMode {
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

		if isControlByRobotMode {
			sendbytes[18] = sendbytes[18] | 0b01000000 //ロボット制御モードのビットを立てる
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

		// デバッグモード: シリアル送信データの表示
		if debugSerial {
			velx := int16(sendbytes[1]) | int16(sendbytes[2])<<8
			vely := int16(sendbytes[3]) | int16(sendbytes[4])<<8
			velang := int16(sendbytes[5]) | int16(sendbytes[6])<<8
			log.Printf("[Serial TX] VelX: %d, VelY: %d, VelAng: %d, Dribble: %d, Kick: %d, Chip: %d, Info: 0b%08b",
				velx, vely, velang, sendbytes[7], sendbytes[8], sendbytes[9], sendbytes[18])
			log.Printf("[Serial TX] CamBallX: %d, CamBallY: %d, Raw: %v",
				sendbytes[16], sendbytes[17], sendbytes)
			fmt.Println("---")
		}

		port.Write(sendbytes) //書き込み

		pre_isReceived = isReceived

		//100ナノ秒待つ
		// time.Sleep(100 * time.Nanosecond)
	}
}
