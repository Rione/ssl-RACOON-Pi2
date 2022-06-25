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
	emg          bool
}

var recvdata RecvStruct

func RunSerial(chclient chan bool, MyID uint32) {
	port, err := serial.Open("/dev/serial0", &serial.Mode{})
	if err != nil {
		log.Fatal(err)
	}
	mode := &serial.Mode{
		BaudRate: 115200,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	if err := port.SetMode(mode); err != nil {
		log.Fatal(err)
	}

	for {
		n, err := port.Write(sendarray.Bytes()) //書き込み
		log.Printf("Sent %v bytes\n", n)        //何バイト送信した？
		CheckError(err)
		buf := make([]byte, 1)
		recvbuf := make([]byte, 6)
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

	go WebAPI(chapi, MyID)
	go RunClient(chclient, MyID, ip)
	go RunServer(chserver, MyID)
	go RunSerial(chserial, MyID)

	<-chapi
	<-chclient
	<-chserver
	<-chserial
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

		time.Sleep(2 * time.Millisecond)
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
					Motor[0] = ((math.Sin((Veltheta-60)*(math.Pi/180)) * Velnormalized) + Velangular) * 100
					Motor[1] = ((math.Sin((Veltheta-135)*(math.Pi/180)) * Velnormalized) + Velangular) * 100
					Motor[2] = ((math.Sin((Veltheta-225)*(math.Pi/180)) * Velnormalized) + Velangular) * 100
					Motor[3] = ((math.Sin((Veltheta-300)*(math.Pi/180)) * Velnormalized) + Velangular) * 100
				}

				for i := 0; i < 4; i++ {

					if Motor[i] > 100 {
						Motor[i] = 100
					} else if Motor[i] < -100 {
						Motor[i] = -100
					}

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
				bytearray.kickPower = uint8(Kickspeedx * 10) //キッカー情報
				bytearray.chipPower = uint8(Kickspeedz * 10) //チップ情報
				bytearray.emg = false                        //EMG情報

				log.Printf("Velnormalized: %f", Velnormalized)
				log.Printf("Float64BeforeInt: %f", Motor)
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
