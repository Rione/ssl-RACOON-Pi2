package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"log"
	"net"
	"time"

	"github.com/Rione/ssl-RACOON-Pi2/proto/pb_gen"
	"google.golang.org/protobuf/proto"
)

// AIからの情報を受信するクライアント
func RunClient(chclient chan bool, MyID uint32, ip string) {

	serverAddr := &net.UDPAddr{
		IP:   net.ParseIP(ip),
		Port: 20011,
	}

	serverConn, err := net.ListenUDP("udp", serverAddr)
	CheckError(err)
	defer serverConn.Close()

	buf := make([]byte, 1024)

	for {
		n, _, _ := serverConn.ReadFromUDP(buf)
		last_recv_time = time.Now()
		packet := &pb_gen.GrSim_Packet{}
		err = proto.Unmarshal(buf[0:n], packet)

		if err != nil {
			log.Fatal("Error: ", err)
		}

		//受信元表示
		// log.Printf("Data received from %s", addr)

		robotcmd := packet.Commands.GetRobotCommands()

		for _, v := range robotcmd {
			// log.Printf("%d\n", int(v.GetId()))
			//ロボットIDが自分のIDと一致したら、受信した情報を反映する
			if v.GetId() == MyID {
				Id := v.GetId()
				Kickspeedx := v.GetKickspeedx()
				if Kickspeedx >= 100 {
					doDirectKick = true
					Kickspeedx -= 100
				}
				Kickspeedz := v.GetKickspeedz()
				if Kickspeedz >= 100 {
					doDirectChipKick = true
					Kickspeedz -= 100
				}
				Veltangent := float64(v.GetVeltangent())
				Velnormal := float64(v.GetVelnormal())
				Velangular := float64(v.GetVelangular())
				Spinner := v.GetSpinner()

				var SpinnerVel float32 = 0
				if Spinner {
					SpinnerVel = v.GetWheel1()
					if SpinnerVel > 100 {
						SpinnerVel = 100
					} else if SpinnerVel < 0 {
						SpinnerVel = 0
					}
					// log.Printf("SpinnerVel: %f", SpinnerVel)
				}

				if Kickspeedx > 0 || Kickspeedz > 0 {
					log.Printf("ID        : %d", Id)
					log.Printf("Kickspeedx: %f", v.GetKickspeedx())
					log.Printf("Kickspeedz: %f", v.GetKickspeedz())
					log.Printf("Veltangent: %f", Veltangent)
					log.Printf("Velnormal : %f", Velnormal)

					log.Printf("Velangular: %f", Velangular)
					log.Printf("Spinner   : %t", Spinner)
				}

				bytearray := SendStruct{}                   //送信用構造体
				bytearray.velx = int16(Veltangent * 1000)   //m/s
				bytearray.vely = int16(Velnormal * 1000)    //m/s
				bytearray.velang = int16(Velangular * 1000) // mrad/sに変換

				bytearray.preamble = 0xFF //プリアンブル

				if Spinner {
					bytearray.dribblePower = uint8(SpinnerVel) //ドリブラ情報
				} else {
					bytearray.dribblePower = 0 //ドリブラ情報
				}

				if Kickspeedx > 0 {
					kicker_val = uint8(Kickspeedx * 10)
					kicker_enable = true
				}
				if kicker_enable {
					bytearray.kickPower = kicker_val //キッカー情報
				} else {
					bytearray.kickPower = 0 //キッカー情報
				}
				if Kickspeedz > 0 {
					chip_val = uint8(Kickspeedz * 10)
					chip_enable = true
				}
				if chip_enable {
					bytearray.chipPower = chip_val //チップ情報
				} else {
					bytearray.chipPower = 0 //チップ情報
				}

				//相対位置情報
				bytearray.relativeX = 0
				bytearray.relativeY = 0
				bytearray.relativeTheta = 0

				//カメラ情報
				bytearray.cameraBallX = 0
				bytearray.cameraBallY = 0

				//informationsのemgStopを0にする
				bytearray.informations = bytearray.informations & 0b11111110

				//ダイレクトキックならば、doDirectKickを1にする
				if doDirectKick {
					bytearray.informations = bytearray.informations | 0b00000010
				}

				//ダイレクトチップならば、doDirectChipKickを1にする
				if doDirectChipKick {
					bytearray.informations = bytearray.informations | 0b00000100
				}

				// 充電を行う
				var doCharge bool = true
				if doCharge {
					bytearray.informations = bytearray.informations | 0b00010000
				}

				// //パリティビットを計算
				// parity := byte(0)
				// for i := 0; i < 7; i++ {
				// 	parity ^= byte(bytearray.velx >> uint(i) & 0x01)
				// 	parity ^= byte(bytearray.vely >> uint(i) & 0x01)
				// 	parity ^= byte(bytearray.velang >> uint(i) & 0x01)
				// 	parity ^= byte(bytearray.dribblePower >> uint(i) & 0x01)
				// 	parity ^= byte(bytearray.kickPower >> uint(i) & 0x01)
				// 	parity ^= byte(bytearray.chipPower >> uint(i) & 0x01)
				// 	parity ^= byte(bytearray.informations >> uint(i) & 0x01)
				// }
				// bytearray.informations = bytearray.informations | (parity << 7)

				//バイナリに変換
				sendarray = bytes.Buffer{}
				err := binary.Write(&sendarray, binary.LittleEndian, bytearray) //バイナリに変換
				if err != nil {
					log.Fatal(err)
				}
			}
		}
		// log.Println("======================================")
	}

}

func ReceiveData(chclient chan bool, MyID uint32, ip string) {
	serverAddr := &net.UDPAddr{
		IP:   net.ParseIP(ip),
		Port: 31133,
	}

	serverConn, err := net.ListenUDP("udp", serverAddr)
	CheckError(err)
	defer serverConn.Close()

	buf := make([]byte, 1024)
	n, _, _ := serverConn.ReadFromUDP(buf)

	jsonData := &ImageData{}
	err = json.Unmarshal(buf[0:n], jsonData)
	if err != nil {
		log.Fatal("Error: ", err)
	}

	imageData = *jsonData

}
