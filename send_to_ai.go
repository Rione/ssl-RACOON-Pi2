package main

import (
	"fmt"
	"net"
	"time"

	"github.com/Rione/ssl-RACOON-Pi/proto/pb_gen"
	"google.golang.org/protobuf/proto"
)

func createStatus(robotid int32, infrared bool, flatkick bool, chipkick bool) *pb_gen.Robot_Status {
	//grSimとの互換性を確保するために用意。
	pe := &pb_gen.Robot_Status{
		RobotId: &robotid, Infrared: &infrared, FlatKick: &flatkick, ChipKick: &chipkick,
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
		// log.Println(recvdata.IsHoldBall)
		pe := createStatus(int32(MyID), recvdata.IsHoldBall, false, false)
		Data, _ := proto.Marshal(pe)

		conn.Write([]byte(Data))

		time.Sleep(100 * time.Millisecond)
	}

}
