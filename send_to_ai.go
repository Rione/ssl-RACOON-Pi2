package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/Rione/ssl-RACOON-Pi2/proto/pb_gen"
	"google.golang.org/protobuf/proto"
)

const (
	thresholdFile  = "threshold.json"
	sendInterval     = time.Second / 60
	discoverInterval = 500 * time.Millisecond // DISCOVERは500ms間隔
	okInterval       = 100 * time.Millisecond // OK_ROBOTは100ms間隔
	robotTimeout     = 3 * time.Second        // PCから3s音沙汰がなければタイムアウト
)

// createStatus はRACOON-MWに送信するステータスメッセージを作成する
func createStatus(robotID uint32, detectPhotoSensor, detectDribbler, isNewDribbler bool,
	batteryVoltage, capPower uint32, isBallExit bool, imageX, imageY float32,
	minThreshold, maxThreshold string, ballDetectRadius int32, circularityThreshold float32,
	flWheelSpeed, blWheelSpeed, brWheelSpeed, frWheelSpeed float32) *pb_gen.PiToMw {
	return &pb_gen.PiToMw{
		RobotsStatus: &pb_gen.Robot_Status{
			RobotId:                &robotID,
			IsDetectPhotoSensor:    &detectPhotoSensor,
			IsDetectDribblerSensor: &detectDribbler,
			IsNewDribbler:          &isNewDribbler,
			BatteryVoltage:         &batteryVoltage,
			CapPower:               &capPower,
			FlWheelSpeed:           &flWheelSpeed,
			BlWheelSpeed:           &blWheelSpeed,
			BrWheelSpeed:           &brWheelSpeed,
			FrWheelSpeed:           &frWheelSpeed,
		},
		BallStatus: &pb_gen.Ball_Status{
			IsBallExit:  &isBallExit,
			BallCameraX: &imageX,
			BallCameraY: &imageY,
		},
		Ball: &pb_gen.Ball{
			MinThreshold:         &minThreshold,
			MaxThreshold:         &maxThreshold,
			BallDetectRadius:     &ballDetectRadius,
			CircularityThreshold: &circularityThreshold,
		},
	}
}

// RunServer はRACOON-MWにボールセンサ等の情報を送信するサーバーである
func RunServer(done <-chan struct{}, myID uint32) {
	// DISCOVER用マルチキャストアドレス
	mcastAddr, err := net.ResolveUDPAddr("udp", MULTICAST_ADDR+":"+MULTICAST_PORT)
	CheckError(err)

	conn, err := net.ListenUDP("udp", nil)
	CheckError(err)
	defer conn.Close()

	// しきい値設定を読み込む（存在しなければ作成）
	adjustment := loadOrCreateThresholdConfig()

	ticker := time.NewTicker(sendInterval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// PCから3s何も届かなかったら接続リセット
			if ConnectionState != StateDiscovering && time.Since(lastRecvTime) > robotTimeout {
				log.Println("[AI TX] PC connection timed out. Reverting to DISCOVERING.")
				ConnectionState = StateDiscovering
			}

			switch ConnectionState {
			case StateDiscovering:
				// DISCOVER(0x01)送信(マルチキャスト)
				if time.Since(lastDiscoverTime) > discoverInterval {
					header := []byte{byte((myID << 4) | 0x01)}
					if _, err := conn.WriteToUDP(header, mcastAddr); err != nil {
						log.Printf("Failed to send DISCOVER: %v", err)
					}
					lastDiscoverTime = time.Now()
				}

			case StateOffered:
				// OK_ROBOT(0x03)送信(ユニキャスト)
				if time.Since(lastOkTime) > okInterval && PcAddress != nil {
					header := []byte{byte((myID << 4) | 0x03)}
					if _, err := conn.WriteToUDP(header, PcAddress); err != nil {
						log.Printf("Failed to send OK_ROBOT: %v", err)
					}
					lastOkTime = time.Now()
				}

			case StateConnected:
				// DATA(0x05)を送信
				if PcAddress != nil {
					sendStatusToMW(conn, PcAddress, myID, adjustment)
				}
			}
		}
	}
}

// loadOrCreateThresholdConfig はしきい値設定を読み込むか、存在しなければデフォルト値で作成する
func loadOrCreateThresholdConfig() Adjustment {
	if _, err := os.Stat(thresholdFile); os.IsNotExist(err) {
		// ファイルが存在しない場合、デフォルト値で作成
		if err := saveAdjustmentConfig(defaultAdjustment); err != nil {
			log.Printf("しきい値ファイル作成エラー: %v", err)
		}
		return defaultAdjustment
	}

	// ファイルから読み込み
	file, err := os.Open(thresholdFile)
	if err != nil {
		log.Printf("しきい値ファイル読み込みエラー: %v", err)
		return defaultAdjustment
	}
	defer file.Close()

	var adjustment Adjustment
	if err := json.NewDecoder(file).Decode(&adjustment); err != nil {
		log.Printf("しきい値JSONデコードエラー: %v", err)
		return defaultAdjustment
	}

	return adjustment
}

// saveAdjustmentConfig はしきい値設定をファイルに保存する
func saveAdjustmentConfig(adjustment Adjustment) error {
	jsonData, err := json.Marshal(adjustment)
	if err != nil {
		return fmt.Errorf("JSON変換エラー: %w", err)
	}
	return os.WriteFile(thresholdFile, jsonData, 0644)
}

// sendStatusToMW はRACOON-MWにステータス情報を送信する
func sendStatusToMW(conn *net.UDPConn, targetAddr *net.UDPAddr, myID uint32, adjustment Adjustment) {
	// センサー情報をビットマスクで取得
	detectPhotoSensor := recvdata.SensorInformation&SENSOR_PHOTO_MASK != 0
	detectDribblerSensor := recvdata.SensorInformation&SENSOR_DRIBBLER_MASK != 0
	isNewDribbler := recvdata.SensorInformation&SENSOR_NEW_DRIB_MASK != 0

	var isBallExit bool
	var imageX, imageY float32
	if imageData != nil {
		isBallExit = imageData.IsBallExit
		imageX = imageData.ImageX
		imageY = imageData.ImageY
	}

	status := createStatus(
		myID,
		detectPhotoSensor,
		detectDribblerSensor,
		isNewDribbler,
		uint32(recvdata.Volt),
		uint32(recvdata.CapPower),
		isBallExit,
		imageX,
		imageY,
		adjustment.MinThreshold,
		adjustment.MaxThreshold,
		int32(adjustment.BallDetectRadius),
		adjustment.CircularityThreshold,
		flWheelSpeedRadS,
		blWheelSpeedRadS,
		brWheelSpeedRadS,
		frWheelSpeedRadS,
	)

	data, err := proto.Marshal(status)
	if err != nil {
		log.Printf("Protobuf marshal error: %v", err)
		return
	}

	// 先頭1バイトヘッダを付加(RobotID << 4 | 0x05)
	header := byte((myID << 4) | 0x05)
	sendData := append([]byte{header}, data...)

	if _, err := conn.WriteToUDP(sendData, targetAddr); err != nil {
		log.Printf("UDP send error: %v", err)
	}
}
