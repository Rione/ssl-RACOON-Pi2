package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/Rione/ssl-RACOON-Pi2/proto/pb_gen"
	"google.golang.org/protobuf/proto"
)

func createStatus(robotid uint32, isdetectphotosensor bool, isdetectdribbler bool, isnewdribbler bool, batt uint32, cappower uint32, is_ball_exit bool, image_x float32, image_y float32, minthreshold string, maxthreshold string, balldetectradius int32, circularitythreshold float32) *pb_gen.PiToMw {
	return &pb_gen.PiToMw{
		RobotsStatus: &pb_gen.Robot_Status{
			RobotId:                &robotid,
			IsDetectPhotoSensor:    &isdetectphotosensor,
			IsDetectDribblerSensor: &isdetectdribbler,
			IsNewDribbler:          &isnewdribbler,
			BatteryVoltage:         &batt,
			CapPower:               &cappower,
		},
		BallStatus: &pb_gen.Ball_Status{
			IsBallExit:  &is_ball_exit,
			BallCameraX: &image_x,
			BallCameraY: &image_y,
		},
		Ball: &pb_gen.Ball{
			MinThreshold:         &minthreshold,
			MaxThreshold:         &maxthreshold,
			BallDetectRadius:     &balldetectradius,
			CircularityThreshold: &circularitythreshold,
		},
	}
}

// RACOON-MWにボールセンサ等の情報を送信するためのサーバ
func RunServer(chserver chan bool, MyID uint32) {
	ipv4 := "224.5.69.4"
	port := "16941"
	addr := ipv4 + ":" + port

	fmt.Println("Sender:", addr)
	conn, err := net.Dial("udp", addr)
	CheckError(err)
	defer conn.Close()

	if _, err := os.Stat("threshold.json"); os.IsNotExist(err) {
		//jsonファイルを作成
		file, err := os.Create("threshold.json")
		if err != nil {
			fmt.Println(err)
			return
		}
		defer file.Close()

		data := Adjustment{Min_Threshold: "1, 120, 100", Max_Threshold: "15, 255, 255", Ball_Detect_Radius: 150, Circularity_Threshold: 0.2}
		jsonData, err := json.Marshal(data)
		if err != nil {
			fmt.Println(err)
			return
		}
		_, err = file.Write(jsonData)
		if err != nil {
			fmt.Println(err)
			return
		}
	} else {
		var minThreshold string
		var maxThreshold string
		var ballDetectRadius int
		var circularityThreshold float32

		file, err := os.Open("threshold.json")
		if err != nil {
			fmt.Println(err)
			return
		}
		defer file.Close()

		decoder := json.NewDecoder(file)
		err = decoder.Decode(&Adjustment{Min_Threshold: minThreshold, Max_Threshold: maxThreshold, Ball_Detect_Radius: ballDetectRadius, Circularity_Threshold: circularityThreshold})
		if err != nil {
			fmt.Println(err)
			return
		}

		for {
			// 左から1ビットだけを取り出す
			detectPhotoSensor := 0b10000000&recvdata.SensorInformation != 0
			// 左から2ビット目だけを取り出す
			detectDribblerSensor := 0b01000000&recvdata.SensorInformation != 0
			// 左から3ビット目だけを取り出す
			isNewDribbler := 0b00100000&recvdata.SensorInformation != 0

			pe := createStatus(uint32(MyID), detectPhotoSensor, detectDribblerSensor, isNewDribbler, uint32(recvdata.Volt), uint32(recvdata.CapPower), imageData.Is_ball_exit, imageData.Image_x, imageData.Image_y, minThreshold, maxThreshold, int32(ballDetectRadius), circularityThreshold)
			Data, _ := proto.Marshal(pe)

			conn.Write([]byte(Data))

			time.Sleep(100 * time.Millisecond)
		}

	}
}
