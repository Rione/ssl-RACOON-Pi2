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
	"go.bug.st/serial"
	"google.golang.org/protobuf/proto"
)

const BAUDRATE int = 9600
const SERIAL_PORT_NAME string = "/dev/serial0"

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
	port, err := serial.Open(SERIAL_PORT_NAME, &serial.Mode{})
	if err != nil {
		log.Fatal(err)
	}

	//シリアル通信のモードをセット
	mode := &serial.Mode{
		BaudRate: BAUDRATE,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	if err := port.SetMode(mode); err != nil {
		log.Fatal(err)
	}

	for {

		n, _ := port.Write(sendarray.Bytes()) //書き込み
		time.Sleep(16 * time.Millisecond)     //少し待つ
		log.Printf("Sent %v bytes\n", n)      //何バイト送信した？
		log.Println(sendarray.Bytes())        //送信済みのバイトを表示

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
		//バイナリから構造体に変換
		err = binary.Read(bytes.NewReader(recvbuf), binary.BigEndian, &recvdata)
		CheckError(err)
		log.Printf("VOLT: %f, BALLSENS: %t, IMUDEG: %d\n", float32(recvdata.Volt*10.0), recvdata.IsHoldBall, recvdata.ImuDir)
		//100ナノ秒待つ
		time.Sleep(100 * time.Nanosecond)
	}
}

var kicker_enable bool = false //キッカーの入力のON OFFを定義する
var kicker_val uint8 = 0       //キッカーの値
var chip_enable bool = false   //チップキックの入力のON OFFを定義する
var chip_val uint8 = 0         //チップキックの値

//キッカーパワーが入力された時に、値を固定する関数
//並列での処理が行われる
func kickCheck(chkicker chan bool) {
	for {
		//ストレートキックが入力されたとき
		if kicker_enable {
			//500ミリ秒待つ
			time.Sleep(500 * time.Millisecond)
			//ストレートキックをオフにし、値を0にする
			kicker_enable = false
			kicker_val = 0
		}
		//チップキックが入力されたとき
		if chip_enable {
			//500ミリ秒待つ
			time.Sleep(500 * time.Millisecond)
			//チップキックをオフにし、値を0にする
			chip_enable = false
			chip_val = 0
		}
		//ループを行うため、少し待機する
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
	//IPアドレスを表示
	fmt.Println("Resolved Host IP: " + ip)
	//IPアドレスの各数字部分を分解
	//例: 192.168.100.101 の場合、 192 が[0]、168 が[1]、100 が[2]、101 が[3]
	hostpart := strings.Split(ip, ".")
	//上記例の[3]なので、101の部分を取得
	iptoid, _ := strconv.Atoi(hostpart[3])
	// 100を引いてロボットIDを決定
	iptoid = iptoid - 100
	//上記推測の結果を表示
	fmt.Println("Estimated Robot ID: " + strconv.Itoa(iptoid))

	//MyIDで指定したロボットIDを取得
	var MyID uint32 = uint32(iptoid)

	chclient := make(chan bool)
	chapi := make(chan bool)
	chserver := make(chan bool)
	chserial := make(chan bool)
	chkick := make(chan bool)

	//各並列処理部分
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
	//WebAPIを起動
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
	//ポート 8080番でサーバを立ち上げる
	log.Fatal(http.ListenAndServe(":8080", nil))

	chapi <- true
}

func createStatus(robotid int32, infrared bool, flatkick bool, chipkick bool) *pb_gen.Robot_Status {
	//grSimとの互換性を確保するために用意。
	pe := &pb_gen.Robot_Status{
		RobotId: &robotid, Infrared: &infrared, FlatKick: &flatkick, ChipKick: &chipkick,
	}

	return pe
}

//RACOON-MWにボールセンサ等の情報を送信するためのサーバ
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

//AIからの情報を受信するクライアント
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

				//Velangular_deg がマイナス値のときは、マイナスであるという情報を付加(imuFlag)
				if Velangular_deg < 0 {
					Velangular_deg = Velangular_deg * -1
					bytearray.imuFlg = true
				} else {
					bytearray.imuFlg = false
				}
				bytearray.imuDir = uint8(Velangular_deg) //IMU情報
				bytearray.emg = false                    //EMG情報

				//log.Printf("Velnormalized: %f", Velnormalized)
				//log.Printf("Float64BeforeInt: %f", Motor)
				sendarray = bytes.Buffer{}
				err := binary.Write(&sendarray, binary.LittleEndian, bytearray) //バイナリに変換

				if err != nil {
					log.Fatal(err)
				}
			}

			//IDが255のときは、モーター動作させず緊急停止フェーズに移行
			if v.GetId() == 255 {
				bytearray := SendStruct{} //送信用構造体
				bytearray.emg = true      // 非常用モード
				bytearray.preamble = 0xFF //プリアンブル

				sendarray = bytes.Buffer{}
				err := binary.Write(&sendarray, binary.LittleEndian, bytearray) //バイナリに変換

				log.Println("EMERGANCY STOP MODE ACTIVATED")

				if err != nil {
					log.Fatal(err)
				}
			}
		}
		log.Println("======================================")
		fmt.Print("\033[H\033[2J")
	}

}

func CheckError(err error) {
	if err != nil {
		log.Fatal("Error: ", err)
	}
}
