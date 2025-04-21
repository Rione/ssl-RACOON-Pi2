package main

import (
	"fmt"
	"net"
	"time"

	"github.com/Rione/ssl-RACOON-Pi2/proto/pb_gen"
	"google.golang.org/protobuf/proto"
)

func createStatus(robotid uint32, infrared bool, batt uint32, cappower uint32, is_ball_exit bool, image_x uint32, image_y uint32) *pb_gen.Robot_Status {
	pe := &pb_gen.Robot_Status{
		RobotId: &robotid, Infrared: &infrared, BatteryVoltage: &batt, CapPower: &cappower, IsBallExit: &is_ball_exit, ImageX: &image_x, ImageY: &image_y,
	}

	return pe
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

	for {
		pe := createStatus(uint32(MyID), recvdata.IsHoldBall, uint32(recvdata.Volt), uint32(recvdata.CapPower), imageData.Is_ball_exit, imageData.Image_x, imageData.Image_y)
		Data, _ := proto.Marshal(pe)

		conn.Write([]byte(Data))

		time.Sleep(100 * time.Millisecond)
	}

}
