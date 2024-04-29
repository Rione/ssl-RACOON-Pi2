package main

import (
	"bytes"
	"encoding/binary"
	"log"
	"math"
	"net"
	"time"

	"github.com/Rione-SSL/RACOON-Pi/proto/pb_gen"
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
		n, addr, _ := serverConn.ReadFromUDP(buf)
		last_recv_time = time.Now()
		packet := &pb_gen.GrSim_Packet{}
		err = proto.Unmarshal(buf[0:n], packet)

		if err != nil {
			log.Fatal("Error: ", err)
		}

		//受信元表示
		log.Printf("Data received from %s", addr)

		robotcmd := packet.Commands.GetRobotCommands()

		for _, v := range robotcmd {
			log.Printf("%d\n", int(v.GetId()))
			//ロボットIDが自分のIDと一致したら、受信した情報を反映する
			if v.GetId() == MyID {
				Id := v.GetId()
				Kickspeedx := v.GetKickspeedx()
				Kickspeedz := v.GetKickspeedz()
				Veltangent := float64(v.GetVeltangent())
				Velnormal := float64(v.GetVelnormal())
				Velangular := float64(v.GetVelangular())
				Spinner := v.GetSpinner()
				log.Printf("ID        : %d", Id)
				log.Printf("Kickspeedx: %f", Kickspeedx)
				log.Printf("Kickspeedz: %f", Kickspeedz)
				log.Printf("Veltangent: %f", Veltangent)
				log.Printf("Velnormal : %f", Velnormal)

				log.Printf("Velangular: %f", Velangular)
				log.Printf("Spinner   : %t", Spinner)
				bytearray := SendStruct{}   //送信用構造体
				Motor := make([]float64, 4) //モータ信号用 Float64

				var Velnormalized float64 = math.Sqrt(math.Pow(Veltangent, 2) + math.Pow(Velnormal, 2))

				if Velnormalized > 1.0 {
					Velnormalized = 1.0
				} else if Velnormalized < 0.0 {
					Velnormalized = 0.0
				}

				Veltheta := math.Atan2(Veltangent, -Velnormal) - (math.Pi / 2)

				if Veltheta < 0 {
					Veltheta = Veltheta + 2.0*math.Pi
				}

				Veltheta = Veltheta * (180 / math.Pi)

				if v.GetWheelsspeed() {
					Motor[0] = float64(v.GetWheel1())
					Motor[1] = float64(v.GetWheel2())
					Motor[2] = float64(v.GetWheel3())
					Motor[3] = float64(v.GetWheel4())
				} else {
					Motor[0] = (math.Sin((Veltheta-45)*(math.Pi/180)) * Velnormalized) * 100
					Motor[1] = (math.Sin((Veltheta-135)*(math.Pi/180)) * Velnormalized) * 100
					Motor[2] = (math.Sin((Veltheta-225)*(math.Pi/180)) * Velnormalized) * 100
					Motor[3] = (math.Sin((Veltheta-315)*(math.Pi/180)) * Velnormalized) * 100
				}

				//Limit Motor Value
				for i := 0; i < 4; i++ {

					if Motor[i] > 100 {
						Motor[i] = 100
					} else if Motor[i] < -100 {
						Motor[i] = -100
					}

					//Plus 100 for uint8
					Motor[i] = Motor[i] + 100
				}

				bytearray.preamble = 0xFF //プリアンブル
				for i := 0; i < 4; i++ {
					bytearray.motor[i] = uint8(Motor[i]) // 1-4番のモータへの信号データ
				}

				if Spinner {
					bytearray.dribblePower = 100 //ドリブラ情報
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

				// Velangular radian to degree
				Velangular_deg := Velangular * (180 / math.Pi)

				//Velangular_deg がマイナス値のときは、マイナスであるという情報を付加(imuFlag)
				if Velangular_deg < 0 {
					Velangular_deg = Velangular_deg * -1
					bytearray.imuFlg = 1
				} else {
					bytearray.imuFlg = 0
				}

				if v.GetWheelsspeed() {
					bytearray.imuFlg = 9 //IMU制御をしない
					bytearray.imuDir = 0 //IMU情報
				}
				bytearray.imuDir = uint8(Velangular_deg) //IMU情報
				bytearray.emg = false                    //EMG情報

				//log.Printf("Velnormalized: %f", Velnormalized)
				//log.Printf("Float64BeforeInt: %f", Motor)
				sendarray = bytes.Buffer{}
				if !imuResetPending {
					err := binary.Write(&sendarray, binary.LittleEndian, bytearray) //バイナリに変換
					if err != nil {
						log.Fatal(err)
					}
				}
			}
			//IDが255のときは、モーター動作させず緊急停止フェーズに移行
			if v.GetId() == 255 {
				bytearray := SendStruct{} //送信用構造体
				bytearray.emg = true      // 非常用モード
				bytearray.preamble = 0xFF //プリアンブル
				bytearray.motor[0] = 100
				bytearray.motor[1] = 100
				bytearray.motor[2] = 100
				bytearray.motor[3] = 100
				sendarray = bytes.Buffer{}
				err := binary.Write(&sendarray, binary.LittleEndian, bytearray) //バイナリに変換

				log.Println("EMERGANCY STOP MODE ACTIVATED")

				if err != nil {
					log.Fatal(err)
				}
			}

			//IMU全体リセット
			if v.GetId() == 254 {
				bytearray := SendStruct{} //送信用構造体
				bytearray.emg = false     // 非常用モード
				bytearray.imuFlg = 2
				bytearray.imuDir = 0
				bytearray.preamble = 0xFF //プリアンブル
				bytearray.motor[0] = 100
				bytearray.motor[1] = 100
				bytearray.motor[2] = 100
				bytearray.motor[3] = 100

				sendarray = bytes.Buffer{}
				err := binary.Write(&sendarray, binary.LittleEndian, bytearray) //バイナリに変換

				log.Println("=======IMU RESET=======")

				if err != nil {
					log.Fatal(err)
				}
			}

			//IMU単独リセット
			if v.GetId() == MyID+100 {
				bytearray := SendStruct{} //送信用構造体
				bytearray.emg = false     // 非常用モード
				bytearray.imuFlg = 3

				Velangular := float64(v.GetVelangular())
				// Velangular radian to degree
				Velangular_deg := Velangular * (180 / math.Pi)

				//Velangular_deg がマイナス値のときは、マイナスであるという情報を付加(imuFlag)
				if Velangular_deg < 0 {
					Velangular_deg = Velangular_deg * -1
					bytearray.imuFlg = 3
				} else {
					bytearray.imuFlg = 2
				}
				bytearray.imuDir = uint8(Velangular_deg) //IMU情報

				bytearray.preamble = 0xFF //プリアンブル
				bytearray.motor[0] = 100
				bytearray.motor[1] = 100
				bytearray.motor[2] = 100
				bytearray.motor[3] = 100

				sendarray = bytes.Buffer{}
				err := binary.Write(&sendarray, binary.LittleEndian, bytearray) //バイナリに変換

				log.Println("=======IMU RESET(RESET TO ANGLE)=======")

				//IMU Reset Pending フラグをたてる
				imuResetPending = true

				if err != nil {
					log.Fatal(err)
				}
			}
		}
		log.Println("======================================")
	}

}
