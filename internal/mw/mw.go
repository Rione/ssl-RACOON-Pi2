package mw

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/Rione/ssl-RACOON-Pi2/internal/state"
	"github.com/Rione/ssl-RACOON-Pi2/internal/util"
	"github.com/Rione/ssl-RACOON-Pi2/proto/pb_gen"
	"google.golang.org/protobuf/proto"
)

const (
	thresholdFile = "threshold.json"
	sendInterval  = 100 * time.Millisecond
)

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

func RunServer(done <-chan struct{}, myID uint32) {
	addr := state.MulticastAddr + ":" + state.MulticastPort
	fmt.Println("Sender:", addr)

	conn, err := net.Dial("udp", addr)
	util.CheckError(err)
	defer conn.Close()

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

func loadOrCreateThresholdConfig() state.Adjustment {
	if _, err := os.Stat(thresholdFile); os.IsNotExist(err) {
		if err := saveAdjustmentConfig(state.DefaultAdjustment); err != nil {
			log.Printf("しきい値ファイル作成エラー: %v", err)
		}
		return state.DefaultAdjustment
	}

	file, err := os.Open(thresholdFile)
	if err != nil {
		log.Printf("しきい値ファイル読み込みエラー: %v", err)
		return state.DefaultAdjustment
	}
	defer file.Close()

	var adjustment state.Adjustment
	if err := json.NewDecoder(file).Decode(&adjustment); err != nil {
		log.Printf("しきい値JSONデコードエラー: %v", err)
		return state.DefaultAdjustment
	}

	return adjustment
}

func saveAdjustmentConfig(adjustment state.Adjustment) error {
	jsonData, err := json.Marshal(adjustment)
	if err != nil {
		return fmt.Errorf("JSON変換エラー: %w", err)
	}
	return os.WriteFile(thresholdFile, jsonData, 0644)
}

func sendStatusToMW(conn net.Conn, myID uint32, adjustment state.Adjustment) {
	detectPhotoSensor := state.Recvdata.SensorInformation&state.SensorPhotoMask != 0
	detectDribblerSensor := state.Recvdata.SensorInformation&state.SensorDribblerMask != 0
	isNewDribbler := state.Recvdata.SensorInformation&state.SensorNewDribMask != 0

	var isBallExit bool
	var imageX, imageY float32
	if state.ImageDataPtr != nil {
		isBallExit = state.ImageDataPtr.IsBallExit
		imageX = state.ImageDataPtr.ImageX
		imageY = state.ImageDataPtr.ImageY
	}

	status := createStatus(
		myID,
		detectPhotoSensor,
		detectDribblerSensor,
		isNewDribbler,
		uint32(state.Recvdata.Volt),
		uint32(state.Recvdata.CapPower),
		isBallExit,
		imageX,
		imageY,
		adjustment.MinThreshold,
		adjustment.MaxThreshold,
		int32(adjustment.BallDetectRadius),
		adjustment.CircularityThreshold,
		state.FlWheelSpeedRadS,
		state.BlWheelSpeedRadS,
		state.BrWheelSpeedRadS,
		state.FrWheelSpeedRadS,
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

func SaveAdjustmentConfig(adjustment state.Adjustment) error {
	return saveAdjustmentConfig(adjustment)
}
