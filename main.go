package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Rione-SSL/RACOON-Pi/proto/pb_gen"
	"github.com/golang/protobuf/proto"
	"go.bug.st/serial"
)

type RobotStatus struct {
	ID       int     `json:"id"`
	Battery  float32 `json:"battery"`
	Wireless float32 `json:"wireless"`
	Health   string  `json:"health"`
	IsError  bool    `json:"is_error"`
	Code     int32   `json:"code"`
}

var robotstatus = []RobotStatus{{
	ID:       0,
	Battery:  12.15,
	Wireless: 66.0,
	Health:   "Good",
	IsError:  true,
	Code:     32,
}}

var sendarray bytes.Buffer //送信用バッファ

type RecvStruct struct {
	Volt        int8
	PhotoSensor int16
	IsHoldBall  bool
	ImuDir      int16
}

type SendStruct struct {
	preamble     byte
	motor        [4]uint8
	dribblePower uint8
	kickPower    uint8
	chipPower    uint8
	imuDir       uint8
	imuFlg       bool
	emg          bool
}

var recvdata RecvStruct

func RunSerial(chclient chan bool, MyID uint32) {
	port, err := serial.Open("/dev/serial0", &serial.Mode{})
	if err != nil {
		log.Fatal(err)
	}
	mode := &serial.Mode{
		BaudRate: 9600,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	if err := port.SetMode(mode); err != nil {
		log.Fatal(err)
	}

	for {

		//if sendarray has capacity
		// for i := 0; i < sendarray.Len(); i++ {
		// 	port.Write(sendarray.Bytes()[i : i+1])
		// 	log.Println(sendarray.Bytes()[i : i+1])
		// 	time.Sleep(1000 * time.Nanosecond)
		// }
		sendbytes := sendarray.Bytes()
		n, _ := port.Write(sendbytes) //書き込み
		time.Sleep(16 * time.Millisecond)
		log.Printf("Sent %v bytes\n", n) //何バイト送信した？
		log.Println(sendarray.Bytes())

		//time.Sleep(1000 * time.Nanosecond)
		buf := make([]byte, 1)
		recvbuf := make([]byte, 6)
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
							for i := 0; i < 6; i++ {
								port.Read(buf)      //読み込み
								recvbuf[i] = buf[0] //受信データを格納
							}
							break
						}
					}
				}
			}
		}
		recvdata := RecvStruct{}
		//構造体に変換
		log.Println(recvbuf)
		err = binary.Read(bytes.NewReader(recvbuf), binary.BigEndian, &recvdata)
		CheckError(err)
		time.Sleep(100 * time.Nanosecond)
	}
}

var kicker_enable bool = false
var kicker_val uint8 = 0
var chip_enable bool = false
var chip_val uint8 = 0

func kickCheck(chkicker chan bool) {
	for {
		if kicker_enable {
			time.Sleep(500 * time.Millisecond)
			kicker_enable = false
			kicker_val = 0
		}
		if chip_enable {
			time.Sleep(500 * time.Millisecond)
			chip_enable = false
			chip_val = 0
		}
		time.Sleep(16 * time.Millisecond)
	}
}

func main() {

	netInterfaceAddresses, _ := net.InterfaceAddrs()

	ip := "0.0.0.0"
	for _, netInterfaceAddress := range netInterfaceAddresses {
		networkIp, ok := netInterfaceAddress.(*net.IPNet)
		if ok && !networkIp.IP.IsLoopback() && networkIp.IP.To4() != nil {
			ip = networkIp.IP.String()
		}
	}
	fmt.Println("Resolved Host IP: " + ip)
	hostpart := strings.Split(ip, ".")
	iptoid, _ := strconv.Atoi(hostpart[3])
	iptoid = iptoid - 100

	fmt.Println("Estimated Robot ID: " + strconv.Itoa(iptoid))

	var MyID uint32 = uint32(iptoid)

	chclient := make(chan bool)
	chapi := make(chan bool)
	chserver := make(chan bool)
	chserial := make(chan bool)
	chkick := make(chan bool)

	go WebAPI(chapi, MyID)
	go RunClient(chclient, MyID, ip)
	go RunServer(chserver, MyID)
	go RunSerial(chserial, MyID)
	go kickCheck(chkick)

	<-chapi
	<-chclient
	<-chserver
	<-chserial
	<-chkick
}

func WebAPI(chapi chan bool, MyID uint32) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		if err := enc.Encode(&robotstatus); err != nil {
			log.Fatal(err)
		}
		fmt.Println(buf.String())

		_, err := fmt.Fprint(w, buf.String())
		if err != nil {
			return
		}
	}

	// GET /robotstatus
	http.HandleFunc("/robotstatus", handler)
	log.Fatal(http.ListenAndServe(":8080", nil))

	chapi <- true
}

func createStatus(robotid int32, infrared bool, flatkick bool, chipkick bool) *pb_gen.Robot_Status {
	pe := &pb_gen.Robot_Status{
		RobotId: &robotid, Infrared: &infrared, FlatKick: &flatkick, ChipKick: &chipkick,
	}

	return pe
}

func RunServer(chserver chan bool, MyID uint32) {
	ipv4 := "224.5.23.2"
	port := "40000"
	addr := ipv4 + ":" + port

	fmt.Println("Sender:", addr)
	conn, err := net.Dial("udp", addr)
	CheckError(err)
	defer conn.Close()

	for {
		pe := createStatus(int32(MyID), recvdata.IsHoldBall, false, false)
		Data, _ := proto.Marshal(pe)

		conn.Write([]byte(Data))

		time.Sleep(100 * time.Millisecond)
	}

}

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
		packet := &pb_gen.GrSim_Packet{}
		err = proto.Unmarshal(buf[0:n], packet)

		if err != nil {
			log.Fatal("Error: ", err)
		}

		log.Printf("Data received from %s", addr)

		robotcmd := packet.Commands.GetRobotCommands()

		for _, v := range robotcmd {
			log.Printf("%d\n", int(v.GetId()))
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
					Motor[0] = (math.Sin((Veltheta-60)*(math.Pi/180)) * Velnormalized) * 100
					Motor[1] = (math.Sin((Veltheta-135)*(math.Pi/180)) * Velnormalized) * 100
					Motor[2] = (math.Sin((Veltheta-225)*(math.Pi/180)) * Velnormalized) * 100
					Motor[3] = (math.Sin((Veltheta-300)*(math.Pi/180)) * Velnormalized) * 100
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
				//100 times of Velangular_deg (ex. -90 degree -> -90.0 * 100 = -9000.0)
				//Velangular_deg = Velangular_deg * 100

				//Velangular_deg is negative
				if Velangular_deg < 0 {
					Velangular_deg = Velangular_deg * -1
					bytearray.imuFlg = true
				} else {
					bytearray.imuFlg = false
				}
				bytearray.imuDir = uint8(Velangular_deg) //IMU情報
				bytearray.emg = false                    //EMG情報

				log.Printf("Velnormalized: %f", Velnormalized)
				log.Printf("Float64BeforeInt: %f", Motor)
				sendarray = bytes.Buffer{}
				err := binary.Write(&sendarray, binary.LittleEndian, bytearray) //バイナリに変換

				if err != nil {
					log.Fatal(err)
				}
			}

			if v.GetId() == 255 {
				bytearray := SendStruct{} //送信用構造体
				bytearray.emg = true      // 非常用モード
				bytearray.preamble = 0xFF //プリアンブル

				sendarray = bytes.Buffer{}
				err := binary.Write(&sendarray, binary.LittleEndian, bytearray) //バイナリに変換

				if err != nil {
					log.Fatal(err)
				}
			}
		}
		log.Println("======================================")
	}

}

func CheckError(err error) {
	if err != nil {
		log.Fatal("Error: ", err)
	}
}
