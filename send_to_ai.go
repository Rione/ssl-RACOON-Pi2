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
	sendInterval   = 100 * time.Millisecond
)

// createStatus はRACOON-MWに送信するステータスメッセージを作成する
func createStatus(robotID uint32, detectPhotoSensor, detectDribbler, isNewDribbler bool,
	batteryVoltage, capPower uint32, isBallExit bool, imageX, imageY float32,
	minThreshold, maxThreshold string, ballDetectRadius int32, circularityThreshold float32) *pb_gen.PiToMw {
	return &pb_gen.PiToMw{
		RobotsStatus: &pb_gen.Robot_Status{
			RobotId:                &robotID,
			IsDetectPhotoSensor:    &detectPhotoSensor,
			IsDetectDribblerSensor: &detectDribbler,
			IsNewDribbler:          &isNewDribbler,
			BatteryVoltage:         &batteryVoltage,
			CapPower:               &capPower,
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
	addr := MULTICAST_ADDR + ":" + MULTICAST_PORT
	fmt.Println("Sender:", addr)

	conn, err := net.Dial("udp", addr)
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
			sendStatusToMW(conn, myID, adjustment)
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
func sendStatusToMW(conn net.Conn, myID uint32, adjustment Adjustment) {
	// センサー情報をビットマスクで取得
	detectPhotoSensor := recvdata.SensorInformation&SENSOR_PHOTO_MASK != 0
	detectDribblerSensor := recvdata.SensorInformation&SENSOR_DRIBBLER_MASK != 0
	isNewDribbler := recvdata.SensorInformation&SENSOR_NEW_DRIB_MASK != 0

	status := createStatus(
		myID,
		detectPhotoSensor,
		detectDribblerSensor,
		isNewDribbler,
		uint32(recvdata.Volt),
		uint32(recvdata.CapPower),
		imageData.IsBallExit,
		imageData.ImageX,
		imageData.ImageY,
		adjustment.MinThreshold,
		adjustment.MaxThreshold,
		int32(adjustment.BallDetectRadius),
		adjustment.CircularityThreshold,
	)

	data, err := proto.Marshal(status)
	if err != nil {
		log.Printf("Protobuf marshal error: %v", err)
		return
	}

	if _, err := conn.Write(data); err != nil {
		log.Printf("UDP send error: %v", err)
	}
}
