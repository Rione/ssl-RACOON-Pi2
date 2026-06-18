package main

import (
	"fmt"
	"log"
	"time"

	"go.bug.st/serial"
)

// シリアル通信の状態管理
var (
	isSignalReceived     bool = false
	prevIsSignalReceived bool = false
)

// カメラ画像座標の一時的なゼロ対策用
var (
	prevImageX int = 0
	prevImageY int = 0
	zeroCountX int = 0
	zeroCountY int = 0
)

// ゼロ値の許容回数
const zeroTolerance = 5

// recvPacketSize は 0xFF プリアンブル直後に読むバイト数（フッタ 0xAA を含む）
const recvPacketSize = 12

// シリアル通信のプリアンブル（0xFF 1バイトのみ）
var serialPreamble = []byte{0xFF}

// 送信データのインデックス定数
const (
	idxVelXLow    = 1
	idxVelXHigh   = 2
	idxVelYLow    = 3
	idxVelYHigh   = 4
	idxVelAngLow  = 5
	idxVelAngHigh = 6
	idxDribble    = 7
	idxKick       = 8
	idxChip       = 9
	idxCamBallX   = 16
	idxCamBallY   = 17
	idxInfo       = 18
)

// RunSerial はシリアル通信によるデータ送受信を行う
func RunSerial(done <-chan struct{}, myID uint32) {
	port, err := serial.Open(SERIAL_PORT_NAME, &serial.Mode{})
	if err != nil {
		log.Fatal(err)
	}
	defer port.Close()

	recvdata = RecvStruct{}

	mode := &serial.Mode{
		BaudRate: BAUDRATE,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	if err := port.SetMode(mode); err != nil {
		log.Fatal(err)
	}

	lastRecvTime.Store(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	lastCmdRecvTime.Store(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))

	for {
		select {
		case <-done:
			return
		default:
			processSerialCommunication(port)
		}
	}
}

// processSerialCommunication はシリアル通信の1サイクルを処理する
func processSerialCommunication(port serial.Port) {
	// プリアンブルを待って受信
	recvbuf := waitForPreambleAndReceive(port)

	recvdata = parseRecvBuf(recvbuf)

	// ホイール回転数を100分の1に変換して rad/s の値にする
	flWheelSpeedRadS = float32(recvdata.FlWheelSpeed) / 100.0
	blWheelSpeedRadS = float32(recvdata.BlWheelSpeed) / 100.0
	brWheelSpeedRadS = float32(recvdata.BrWheelSpeed) / 100.0
	frWheelSpeedRadS = float32(recvdata.FrWheelSpeed) / 100.0

	if debugSerial {
		log.Printf("[Serial RX] Raw: % 02X", recvbuf)
		log.Printf("[Serial RX] Volt: %d (%.1fV), SensorInfo: 0b%08b, CapPower: %d, Footer: 0x%02X",
			recvdata.Volt, float32(recvdata.Volt)*0.1, recvdata.SensorInformation, recvdata.CapPower, recvdata.Footer)
		log.Printf("[Serial RX] Wheel(raw) FL: %d, BL: %d, BR: %d, FR: %d",
			recvdata.FlWheelSpeed, recvdata.BlWheelSpeed, recvdata.BrWheelSpeed, recvdata.FrWheelSpeed)
		log.Printf("[Serial RX] Wheel(rad/s) FL: %.2f, BL: %.2f, BR: %.2f, FR: %.2f",
			flWheelSpeedRadS, blWheelSpeedRadS, brWheelSpeedRadS, frWheelSpeedRadS)
	}

	// バッテリーエラーチェック
	checkBatteryStatus()

	// 送信データを準備
	sendbytes := prepareSendData()

	if debugSerial {
		logSendData(sendbytes)
	}

	port.Write(sendbytes)
	prevIsSignalReceived = isSignalReceived
}

// parseRecvBuf は STM 送信形式（パディングなし・リトルエンディアン）を手動でパースする
func parseRecvBuf(recvbuf []byte) RecvStruct {
	return RecvStruct{
		Volt:              recvbuf[0],
		SensorInformation: recvbuf[1],
		CapPower:          recvbuf[2],
		FlWheelSpeed:      int16(recvbuf[3]) | int16(recvbuf[4])<<8,
		BlWheelSpeed:      int16(recvbuf[5]) | int16(recvbuf[6])<<8,
		BrWheelSpeed:      int16(recvbuf[7]) | int16(recvbuf[8])<<8,
		FrWheelSpeed:      int16(recvbuf[9]) | int16(recvbuf[10])<<8,
		Footer:            recvbuf[11],
	}
}

// waitForPreambleAndReceive はプリアンブルを検出してデータを受信する
func waitForPreambleAndReceive(port serial.Port) []byte {
	buf := make([]byte, 1)
	recvbuf := make([]byte, recvPacketSize)

	port.ResetInputBuffer()

	// プリアンブル検出（0xFF 1バイトのみ）
	preambleIdx := 0
	for preambleIdx < len(serialPreamble) {
		port.Read(buf)
		if buf[0] == serialPreamble[preambleIdx] {
			preambleIdx++
		} else {
			preambleIdx = 0
		}
	}

	// データ受信: Volt(1) + Sensor(1) + Cap(1) + FL(2) + BL(2) + BR(2) + FR(2) + Footer(1)
	for i := 0; i < recvPacketSize; i++ {
		port.Read(buf)
		recvbuf[i] = buf[0]
	}

	return recvbuf
}

// checkBatteryStatus はバッテリー状態をチェックしてエラーフラグを設定する
func checkBatteryStatus() {
	if recvdata.Volt < uint8(BATTERY_CRITICAL_THRESHOLD) {
		isRobotError = true
		RobotErrorCode = 2
		RobotErrorMessage = "バッテリ電圧異常(回路故障の可能性)"
	} else if recvdata.Volt < uint8(BATTERY_LOW_THRESHOLD) {
		isRobotError = true
		RobotErrorCode = 2
		RobotErrorMessage = "バッテリ電圧異常"
	}
}

// prepareSendData は送信用バイト列を準備する
func prepareSendData() []byte {
	sendbytes := sendarray.Bytes()

	// 初回（データがない場合）は初期値を設定
	if len(sendbytes) < 19 {
		sendbytes = make([]byte, 19)
		sendbytes[0] = 0xFF // プリアンブル
		sendbytes[idxInfo] = 1
	}

	// カメラ座標を更新（一時的なゼロは無視）
	updateCameraCoordinates(sendbytes)

	// 受信タイムアウトチェック
	handleReceiveTimeout(sendbytes)

	// ロボット制御モードフラグ
	if isControlByRobotMode {
		sendbytes[idxInfo] |= INFO_CTRL_BY_ROBOT
	}

	// 長時間未受信の場合は充電停止
	if lastRecvTime.Since() > CHARGE_STOP_TIMEOUT {
		sendbytes[idxInfo] &= ^uint8(INFO_DO_CHARGE)
	}

	// 受信状態変化時の通知音
	handleReceiveStateChange()

	// キッカー値の更新
	if kickerEnable {
		sendbytes[idxKick] = kickerVal
	} else {
		sendbytes[idxKick] = 0
	}

	return sendbytes
}

// updateCameraCoordinates はカメラ座標を更新する（一時的なゼロは無視）
func updateCameraCoordinates(sendbytes []byte) {
	//imageDataがnilの場合は安全に処理をスキップ（または0をセット）する。
	if imageData == nil {
		sendbytes[idxCamBallX] = 0
		sendbytes[idxCamBallY] = 0
		return
	}
	// X座標
	if imageData.ImageX == 0 {
		zeroCountX++
		if zeroCountX <= zeroTolerance {
			sendbytes[idxCamBallX] = byte(prevImageX)
		} else {
			sendbytes[idxCamBallX] = 0
		}
	} else {
		scaledX := int(imageData.ImageX * 255 / 639)
		sendbytes[idxCamBallX] = byte(scaledX)
		prevImageX = scaledX
		zeroCountX = 0
	}

	// Y座標
	if imageData.ImageY == 0 {
		zeroCountY++
		if zeroCountY <= zeroTolerance {
			sendbytes[idxCamBallY] = byte(prevImageY)
		} else {
			sendbytes[idxCamBallY] = 0
		}
	} else {
		scaledY := int(imageData.ImageY / 10)
		sendbytes[idxCamBallY] = byte(scaledY)
		prevImageY = scaledY
		zeroCountY = 0
	}
}

// handleReceiveTimeout は受信タイムアウト時の処理を行う
func handleReceiveTimeout(sendbytes []byte) {
	if lastCmdRecvTime.Since() > NO_RECV_TIMEOUT && !isControlByRobotMode {
		// 速度・ドリブル・キック値をクリア
		for i := idxVelXLow; i <= idxChip; i++ {
			sendbytes[i] = 0
		}
		sendbytes[idxInfo] &= ^uint8(INFO_SIGNAL_RECEIVED)
		isSignalReceived = false
	} else {
		sendbytes[idxInfo] |= INFO_SIGNAL_RECEIVED
		isSignalReceived = true
	}
}

// handleReceiveStateChange は受信状態が変化した時の通知音を鳴らす
func handleReceiveStateChange() {
	if !isSignalReceived && prevIsSignalReceived {
		log.Println("No Data Recv")
		go ringBuzzer(3, 500*time.Millisecond, 0)
	}

	if isSignalReceived && !prevIsSignalReceived {
		go ringBuzzer(10, 500*time.Millisecond, 0)
	}
}

// logSendData はデバッグ用に送信データをログ出力する
func logSendData(sendbytes []byte) {
	velx := int16(sendbytes[idxVelXLow]) | int16(sendbytes[idxVelXHigh])<<8
	vely := int16(sendbytes[idxVelYLow]) | int16(sendbytes[idxVelYHigh])<<8
	velang := int16(sendbytes[idxVelAngLow]) | int16(sendbytes[idxVelAngHigh])<<8

	log.Printf("[Serial TX] VelX: %d, VelY: %d, VelAng: %d, Dribble: %d, Kick: %d, Chip: %d, Info: 0b%08b",
		velx, vely, velang, sendbytes[idxDribble], sendbytes[idxKick], sendbytes[idxChip], sendbytes[idxInfo])
	log.Printf("[Serial TX] CamBallX: %d, CamBallY: %d, Raw: %v",
		sendbytes[idxCamBallX], sendbytes[idxCamBallY], sendbytes)
	fmt.Println("---")
}
