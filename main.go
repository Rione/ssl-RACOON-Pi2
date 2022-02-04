package main

import (
	"github.com/Rione-SSL/RACOON-Pi/proto/pb_gen"
	"go.bug.st/serial"
	"github.com/golang/protobuf/proto"
	"github.com/stianeikeland/go-rpio/v4"
	"log"
	"net"
	"net/http"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"time"
	"os"
)

type RobotStatus struct {
	ID			int			`json:"id"`
	Battery		float32		`json:"battery"`
	Wireless	float32		`json:"wireless"`
	Health		string		`json:"health"`
	IsError		bool		`json:"is_error"`
	Code		int32		`json:"code"`
}

var robotstatus = []RobotStatus{{
	ID:			0,
	Battery:	12.15,
	Wireless:	66.0,
	Health:		"Good",
	IsError:	true,
	Code:		32,
}}

func main() {
	var MyID uint32 = 1

	chclient := make(chan bool)
	chapi := make(chan bool)
	chserver := make(chan bool)

	go WebAPI(chapi, MyID)
	go RunClient(chclient, MyID)
	go RunServer(chserver, MyID)

	<-chapi
	<-chclient
	<-chserver
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
	err := rpio.Open()
	if err != nil {
		os.Exit(1)
	}
	defer rpio.Close()

	IR := rpio.Pin(18)

	IR.Input()

	ipv4 := "224.5.23.2"
    port := "40000"
    addr := ipv4 + ":" + port

    fmt.Println("Sender:", addr)
    conn, err := net.Dial("udp", addr)
    CheckError(err)
    defer conn.Close()

	BeforeState := 0

    for {
        ReadState := IR.Read()

		if ReadState != BeforeState {
			Infrared := false
			if ReadState == 1{
				Infrared = true
			}
	
			pe := createStatus(int32(MyID), Infrared, false, false)
			Data, _ := proto.Marshal(pe)
	
			conn.Write([]byte(Data))
		}

		time.Sleep(2 * time.Millisecond)

		BeforeState = ReadState
    }

	chserver <- true
}

func RunClient(chclient chan bool, MyID uint32) {

	DR_PIN := 12
	
	err := rpio.Open()
	if err != nil {
		os.Exit(1)
	}
	defer rpio.Close()

	DR := rpio.Pin(DR_PIN)
	DR.Mode(rpio.Pwm)
	DR.Freq(50)
	DR.DutyCycle(0, 100) // DutyCycle(これからのDuty比, 100%まで)

	//ドリブラ初期化
	log.Printf("Initializing Dribbler...")
	DR.DutyCycle(0, 100)
	time.Sleep(time.Second * 1)
	DR.DutyCycle(4, 100)
	time.Sleep(time.Second * 1)
	log.Printf("Done.")

	port, err := serial.Open("/dev/ttyS0", &serial.Mode{})
	if err != nil{
		log.Fatal(err)
	}
	mode := &serial.Mode{
		BaudRate: 115200,
		Parity: serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	if err := port.SetMode(mode); err != nil {
		log.Fatal(err)
	}

	serverAddr := &net.UDPAddr{
		IP:   net.ParseIP("224.5.23.2"),
		Port: 20011,
	}

	serverConn, err := net.ListenMulticastUDP("udp", nil, serverAddr)
	CheckError(err)
	defer serverConn.Close()

	buf := make([]byte, 1024)

	for {
		n, addr, err := serverConn.ReadFromUDP(buf)
		packet := &pb_gen.GrSim_Packet{}
		err = proto.Unmarshal(buf[0:n], packet)

		if err != nil {
			log.Fatal("Error: ", err)
		}

		log.Printf("Data received from %s", addr)

		robotcmd := packet.Commands.GetRobotCommands()

		for _, v := range robotcmd {
			log.Println("%d", v.GetId())
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
				
				bytearray := make([]byte, 7) //7バイト領域を確保
				Motor := make([]float64, 4) //モータ信号用 Float64

				var Velnormalized float64 = math.Sqrt(math.Pow(Veltangent, 2) + math.Pow(Velnormal, 2))

				if Velnormalized > 1.0 {
					Velnormalized = 1.0
				} else if Velnormalized < 0.0 {
					Velnormalized = 0.0
				}

				Veltheta := math.Atan2(Veltangent, -Velnormal) - (math.Pi/2)

				if Veltheta < 0 {
					Veltheta = Veltheta + 2.0 * math.Pi
				}

				Veltheta = Veltheta * (180/math.Pi)

				if Spinner == true {
					DR.DutyCycle(8, 100)
				} else {
					DR.DutyCycle(4, 100)
				}

				Motor[0] = ((math.Sin((Veltheta - 60) * (math.Pi/180)) * Velnormalized) + Velangular) * 100
				Motor[1] = ((math.Sin((Veltheta - 135) * (math.Pi/180)) * Velnormalized) + Velangular) * 100
				Motor[2] = ((math.Sin((Veltheta - 225) * (math.Pi/180)) * Velnormalized) + Velangular) * 100
				Motor[3] = ((math.Sin((Veltheta - 300) * (math.Pi/180)) * Velnormalized) + Velangular) * 100

				for i := 0; i < 4; i++ {
					
					if Motor[i] > 100 {
						Motor[i] = 100
					} else if Motor[i] < -100 {
						Motor[i] = -100
					}

					Motor[i] = Motor[i] + 100
				}

				
				bytearray[0] = 0xFF	//プリアンブル
				for i := 0; i < 4; i++ {
					bytearray[i+1] = uint8(Motor[i]) // 1-4番のモータへの信号データ
				}

				if Spinner == true{
					bytearray[5] = 1 //ドリブラ情報
				} else {
					bytearray[5] = 0 //ドリブラ情報
				}
				bytearray[6] = uint8(Kickspeedx * 10) //キッカー情報

				log.Printf("Velnormalized: %f", Velnormalized)
				log.Printf("Float64BeforeInt: %f", Motor)
				log.Printf("Bytes: %b", bytearray)


				n, err := port.Write(bytearray) //書き込み
				if err != nil {
					log.Fatal(err)
				}
				log.Printf("Sent %v bytes\n", n) //何バイト送信した？
			}
		}
		
		log.Println("======================================")
	}

	chclient <- true
}

func CheckError(err error) {
	if err != nil {
		log.Fatal("Error: ", err)
	}
}
