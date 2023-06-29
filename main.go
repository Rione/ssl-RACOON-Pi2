package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/Rione-SSL/RACOON-Pi/proto/pb_gen"
	"github.com/stianeikeland/go-rpio/v4"
	"go.bug.st/serial"
	"google.golang.org/protobuf/proto"
)

// ボーレート
const BAUDRATE int = 9600

// シリアルポート名 ラズパイ4の場合、"/dev/serial0"
const SERIAL_PORT_NAME string = "/dev/serial0"

const IMU_TOOFAST_THRESHOULD float64 = 35.0

// ボールセンサー故障検知しきい値。L: OPEN故障、H: CLOSE故障
const BALLSENS_LBREAK_THRESHOULD int = 100
const BALLSENS_HBREAK_THRESHOULD int = 100

// ボールセンサーの反応しきい値。
const BALLSENS_LOW_THRESHOULD int = 100

// バッテリーの低下しきい値。 150 = 15.0V
const BATTERY_LOW_THRESHOULD int = 150
const BATTERY_CRITICAL_THRESHOULD int = 145

var sendarray bytes.Buffer //送信用バッファ

// 受信時の構造体
type RecvStruct struct {
	Volt        uint8
	PhotoSensor uint16
	IsHoldBall  bool
	ImuDir      int16
}

// 送信時の構造体
type SendStruct struct {
	preamble     byte
	motor        [4]uint8
	dribblePower uint8
	kickPower    uint8
	chipPower    uint8
	imuDir       uint8
	imuFlg       uint8
	emg          bool
}

// 受信データ構造体
var recvdata RecvStruct

// imu角度
var imudegree int16

// imu速度超過時のフラグ
var imuError bool = false

var last_recv_time time.Time = time.Now()

// ポート8080番で待ち受ける。
const PORT string = ":9191"

func RunApi(chapi chan bool, MyID uint32) {
	//ポートを開く
	listener, err := net.Listen("tcp", PORT)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	//接続を待つ
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal(err)
		}
		// log
		log.Println("Remote API Connected by ", conn.RemoteAddr())
		//接続があったら処理を行う
		go HandleRequest(conn)
	}
}

var isRobotError = false
var isRobotEmgError = false

var RobotErrorCode = 0
var RobotErrorMessage = ""

var ballSensLowCount = 0

var doBuzzer = false
var buzzerTone = 0
var buzzerTime time.Duration = 0 * time.Millisecond

var alarmIgnore = false

// 接続があったら処理を行う
func HandleRequest(conn net.Conn) {
	defer conn.Close()

	//リクエストを解析
	buf := make([]byte, 1024)
	_, err := conn.Read(buf)
	if err != nil {
		log.Println(err)
		return
	}

	// リクエストを解析
	// リクエストヘッダーの1行目を取得
	request := string(buf)
	// リクエストヘッダーの1行目をスペースで区切る
	requests := strings.Split(request, " ")
	// リクエストヘッダーの1行目からリクエストの種類を取得
	requestType := requests[0]

	// リクエストの種類がGETでなければエラーを返す
	if requestType != "GET" {
		fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\n")
		fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
		fmt.Fprintf(conn, "400 Bad Request\r\n")
		return
	}

	// リクエストのパスが"/buzzer"の場合
	if strings.Split(requests[1], "/")[1] == "buzzer" {
		// tone が指定されていない場合
		if len(requests) < 3 {
			fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "400 Bad Request\r\n")
			return
		}
		// buzzer/ の後に tone が指定されている場合
		log.Println(strings.Split(requests[1], "/")[1])
		tone, err := strconv.Atoi(strings.Split(requests[1], "/")[3])
		if err != nil {
			fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "400 Bad Request\r\n")
			return
		}
		duration, err := strconv.Atoi(strings.Split(requests[1], "/")[4])
		if err != nil {
			fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "400 Bad Request\r\n")
			return
		}
		// tone が 0 から 12 でない場合
		if tone < 0 || tone > 15 {
			fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "400 Bad Request\r\n")
			return
		}
		// duration が 50 から 3000 でない場合
		if duration < 50 || duration > 3000 {
			fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\n")
			fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
			fmt.Fprintf(conn, "400 Bad Request\r\n")
			return
		}

		// OK と表示
		fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\n")
		fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
		fmt.Fprintf(conn, "BUZZER OK\r\n")
		//ブザーを1秒鳴らす
		doBuzzer = true
		buzzerTone = tone
		buzzerTime = time.Duration(duration) * time.Millisecond
		return
	}

	if strings.Split(requests[1], "/")[1] == "ignorebatterylow" {
		// OK と表示
		fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\n")
		fmt.Fprintf(conn, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
		fmt.Fprintf(conn, "IGNORE BATTERY LOW OK\r\n")
		//アラーム無視をセットする
		alarmIgnore = true
		return
	}

	// 200 OKを返す
	fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\n")
	// UTF-8指定
	fmt.Fprintf(conn, "Content-Type: application/json; charset=utf-8\r\n\r\n")

	// JSON形式で返す
	response := fmt.Sprintf(`{
		"VOLT": %f,
		"PHOTOSENSOR": %d,
		"ISHOLDBALL": %t,
		"IMUDIR": %d,
		"ERROR": %t,
		"ERRORCODE": %d,
		"ERRORMESSAGE": "%s"
	}`, float32(recvdata.Volt)/10.0, recvdata.PhotoSensor, recvdata.IsHoldBall, recvdata.ImuDir, isRobotError, RobotErrorCode, RobotErrorMessage)

	fmt.Fprint(conn, response)

}

// シリアル通信部分
func RunSerial(chclient chan bool, MyID uint32) {
	port, err := serial.Open(SERIAL_PORT_NAME, &serial.Mode{})
	if err != nil {
		log.Fatal(err)
	}

	//構造体の宣言
	recvdata = RecvStruct{}

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

		//受信できるまで読み込む。バイトが0xFF, 0x00, 0xFF, 0x00のときは受信できると判断する
		buf := make([]byte, 1)
		recvbuf := make([]byte, 6)
		//ここで受信バッファをクリアする
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
							//合計6バイト
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

		//バイナリから構造体に変換
		err = binary.Read(bytes.NewReader(recvbuf), binary.BigEndian, &recvdata)
		CheckError(err)

		//////////////////////////////////
		///
		/// エラーチェック部分
		///
		//////////////////////////////////
		//ボールセンサーが低いときにカウントを増やす
		if recvdata.PhotoSensor < uint16(BALLSENS_LOW_THRESHOULD) {
			ballSensLowCount++
		} else {
			ballSensLowCount = 0
		}

		//ボールセンサーが低いときにカウントが一定値(10s)を超えたらエラー
		if ballSensLowCount > 600 {
			isRobotError = true
			RobotErrorCode = 1
			RobotErrorMessage = "ボールセンサ異常"
		}

		// //ボールセンサの値が極端に高いときはエラー
		// if recvdata.PhotoSensor > uint16(BALLSENS_HBREAK_THRESHOULD) {
		// 	isRobotError = true
		// 	RobotErrorCode = 1
		// 	RobotErrorMessage = "ボールセンサ異常(回路故障の可能性)"
		// }

		//ボールセンサの値が極端に低いときはエラー
		if recvdata.PhotoSensor < uint16(BALLSENS_LBREAK_THRESHOULD) {
			isRobotError = true
			RobotErrorCode = 1
			RobotErrorMessage = "ボールセンサ異常(回路故障の可能性)"
		}

		//バッテリの電圧が一定量を下回ったらエラー
		if recvdata.Volt < uint8(BATTERY_LOW_THRESHOULD) {
			isRobotError = true
			RobotErrorCode = 2
			RobotErrorMessage = "バッテリ電圧異常"
		}

		//バッテリの電圧が極端に低いとき（速度に影響を与えるレベル）はエラー
		if recvdata.Volt < uint8(BATTERY_CRITICAL_THRESHOULD) {
			isRobotError = true
			RobotErrorCode = 2
			RobotErrorMessage = "バッテリ電圧異常(回路故障の可能性)"

			isRobotEmgError = true //緊急停止
		}

		//TODO: Pendingをfalseにした瞬間にAIが受信し始める。Mutex必要か？
		if imuResetPending {
			imuResetPending = false
		}
		//クライアントで受け取ったデータをバイト列に変更
		sendbytes := sendarray.Bytes()

		//バイト列がなかったら（初回受け取りを行っていない場合）、初期値を設定
		if len(sendbytes) <= 0 {
			sendbytes = []byte{0xFF, 100, 100, 100, 100, 0, 0, 0, 0, 0, 0}
		}

		//受信しなかった場合に自動的にモーターOFFする
		if time.Since(last_recv_time) > 1*time.Second {
			// log.Println("No Data Recv")
			for i := 2; i <= 4; i++ {
				sendbytes[i] = 100
			}
		}

		if kicker_enable {
			sendbytes[6] = kicker_val
		} else {
			sendbytes[6] = 0
		}

		//それぞれのデータを表示
		// log.Printf("VOLT: %f, BALLSENS: %t, IMUDEG: %d\n", float32(recvdata.Volt)*0.1, recvdata.IsHoldBall, recvdata.ImuDir)

		//高速回転防止機能
		//フレームごとの角度が閾値を超えると, EMGをセットする
		if len(sendbytes) > 0 {
			if math.Abs(math.Abs(float64(imudegree))-math.Abs(float64(recvdata.ImuDir))) > IMU_TOOFAST_THRESHOULD {
				imuError = true
			}
		}
		// 角速度が大幅に超えた場合
		if imuError && len(sendbytes) > 0 {
			if sendbytes[9] == 0x00 || sendbytes[9] == 0x01 {
				//EMGをセット
				sendbytes[10] = 0x01
				log.Println("IMU DIFF OVER 35 DEGREE EMG STOPPING..")
			}
		}

		//imu角度リセットの動作部分
		if imuReset && len(sendbytes) > 0 {
			//EMGを解除
			sendbytes[10] = 0x00
			//imu角度フラグを2にセット
			sendbytes[9] = 0x02
			//imu角度を0にセット
			sendbytes[8] = 0x00
			log.Println("IMU RESET")
		}
		//前のフレームのimu角度を保持
		imudegree = recvdata.ImuDir

		port.Write(sendbytes)             //書き込み
		time.Sleep(16 * time.Millisecond) //少し待つ
		//log.Printf("Sent %v bytes\n", n)  //何バイト送信した？
		// log.Println(sendbytes) //送信済みのバイトを表示

		//100ナノ秒待つ
		time.Sleep(100 * time.Nanosecond)
	}
}

var imuReset bool = false

// GPIO処理部分
func RunGPIO(chgpio chan bool) {

	//ラズパイのGPIOのメモリを確保
	err := rpio.Open()
	CheckError(err)

	//GPIO18をLED1に設定。出力
	led := rpio.Pin(18)
	led.Output()

	//GPIO27をLED2に設定。出力
	led2 := rpio.Pin(27)
	led.Output()

	//GPIO22をbutton1に設定。入力(S2)
	button1 := rpio.Pin(22)
	button1.Input()
	button1.PullUp()

	//GPIO 24をbutton2に設定。入力(S3)
	button2 := rpio.Pin(24)
	button2.Input()
	button2.PullUp()

	//GPIO12をブザーPWMに設定。出力
	buzzer := rpio.Pin(12)
	buzzer.Mode(rpio.Pwm)
	buzzer.Freq(64000)
	buzzer.DutyCycle(0, 32)

	// buzzer.DutyCycle(16, 32)
	// time.Sleep(time.Millisecond * 100)
	// buzzer.DutyCycle(0, 32)
	// time.Sleep(time.Millisecond * 100)

	buzzer.Freq(1244 * 64)
	buzzer.DutyCycle(16, 32)
	time.Sleep(time.Millisecond * 100)
	buzzer.Freq(1108 * 64)
	time.Sleep(time.Millisecond * 100)
	buzzer.Freq(739 * 64)
	time.Sleep(time.Millisecond * 150)
	buzzer.DutyCycle(0, 32)
	time.Sleep(time.Millisecond * 100)

	//1479Hzにする
	buzzer.Freq(1479 * 64)
	buzzer.DutyCycle(16, 32)
	time.Sleep(time.Millisecond * 100)
	buzzer.DutyCycle(0, 32)
	time.Sleep(time.Millisecond * 100)
	buzzer.DutyCycle(16, 32)
	time.Sleep(time.Millisecond * 100)
	buzzer.DutyCycle(0, 32)

	//GPIO 6, 25, 4, 5 を DIP 1, 2, 3, 4 に設定。入力
	dip1 := rpio.Pin(6)
	dip1.Input()
	dip1.PullUp()
	dip2 := rpio.Pin(25)
	dip2.Input()
	dip2.PullUp()
	dip3 := rpio.Pin(4)
	dip3.Input()
	dip3.PullUp()
	dip4 := rpio.Pin(5)
	dip4.Input()
	dip4.PullUp()

	// DIP1, 2, 3 ,4 の状態を出力
	fmt.Println("DIP1:", dip1.Read()^1)
	fmt.Println("DIP2:", dip2.Read()^1)
	fmt.Println("DIP3:", dip3.Read()^1)
	fmt.Println("DIP4:", dip4.Read()^1)

	//DIP 1, 2, 3, 4からhexを作成
	hex := dip1.Read() ^ 1 + (dip2.Read()^1)*2 + (dip3.Read()^1)*4 + (dip4.Read()^1)*8

	//hexを表示
	fmt.Println("HEX:", int(hex))

	//Lチカ速度
	ledsec := 500 * time.Millisecond
	alarmVoltage := BATTERY_LOW_THRESHOULD
	for {
		//電圧降下検知
		if recvdata.Volt <= uint8(alarmVoltage) {
			for {
				buzzer.Freq(1200 * 64)
				buzzer.DutyCycle(16, 32)

				//高速チカチカ
				led2.High()
				time.Sleep(100 * time.Millisecond)

				buzzer.Freq(760 * 64)
				led2.Low()
				time.Sleep(150 * time.Millisecond)

				if button1.Read()^1 == rpio.High || alarmIgnore == true {
					//一時的にアラーム解除する
					log.Println("BATTERY ALARM IGNORED")
					alarmVoltage = BATTERY_CRITICAL_THRESHOULD
					break
				}
			}
		} else {
			//通常チカチカ。ボタンが押されたら高速チカチカ
			//ボタンが押されたら、imuをリセットする
			buzzer.Freq(1479 * 64)
			time.Sleep(ledsec)
			led.Write(rpio.High)
			if button1.Read()^1 == rpio.High {
				imuReset = true
				ledsec = 100 * time.Millisecond
				buzzer.DutyCycle(16, 32)
			} else {
				imuReset = false
				ledsec = 500 * time.Millisecond
			}
			if button2.Read()^1 == rpio.High {
				//kickする
				buzzer.DutyCycle(16, 32)
				time.Sleep(500 * time.Millisecond)
				buzzer.DutyCycle(0, 32)
				kicker_enable = true
				kicker_val = 100
			}
			time.Sleep(ledsec)
			led.Write(rpio.Low)
			buzzer.DutyCycle(0, 32)

			if doBuzzer {
				buzzer.Freq(int(440*math.Pow(1.0595, float64(buzzerTone))) * 64)
				buzzer.DutyCycle(16, 32)
				time.Sleep(buzzerTime)
				buzzer.DutyCycle(0, 32)
				doBuzzer = false
			}
		}
	}
}

var kicker_enable bool = false //キッカーの入力のON OFFを定義する
var kicker_val uint8 = 0       //キッカーの値
var chip_enable bool = false   //チップキックの入力のON OFFを定義する
var chip_val uint8 = 0         //チップキックの値

// キッカーパワーが入力された時に、値を固定する関数
// 並列での処理が行われる
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
		if imuError {
			time.Sleep(500 * time.Millisecond)
			imuError = false
		}
		//ループを行うため、少し待機する
		time.Sleep(16 * time.Millisecond)
	}
}

func main() {

	//GPIOの初期化
	if err := rpio.Open(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer rpio.Close()

	//GPIO 6, 25, 4, 5 を DIP 1, 2, 3, 4 に設定。入力
	dip1 := rpio.Pin(6)
	dip1.Input()
	dip1.PullUp()
	dip2 := rpio.Pin(25)
	dip2.Input()
	dip2.PullUp()
	dip3 := rpio.Pin(4)
	dip3.Input()
	dip3.PullUp()
	dip4 := rpio.Pin(5)
	dip4.Input()
	dip4.PullUp()

	// HEXの値を表示
	hex := dip1.Read() ^ 1 + (dip2.Read()^1)*2 + (dip3.Read()^1)*4 + (dip4.Read()^1)*8
	fmt.Println("GOT ID FROM DIP SW:", int(hex))
	diptoid := int(hex)

	netInterfaceAddresses, _ := net.InterfaceAddrs()

	ip := "0.0.0.0"
	for _, netInterfaceAddress := range netInterfaceAddresses {
		networkIp, ok := netInterfaceAddress.(*net.IPNet)
		if ok && !networkIp.IP.IsLoopback() && networkIp.IP.To4() != nil {
			ip = networkIp.IP.String()
		}
	}

	//Ctrl+Cを押したときに終了するようにする
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			//終了時にGPIOを解放する
			rpio.Close()
			log.Println("Bye")
			os.Exit(0)
		}
	}()

	//Hostnameを取得する
	cmd := exec.Command("hostname")
	out, err := cmd.Output()
	if err != nil {
		panic(err)
	}
	fmt.Println(string(out))

	//もし初期値のraspberrypiだったら
	if string(out) == "raspberrypi\n" {
		//UNIX時間の下5桁を取得する
		unixtime := time.Now().UnixNano()
		log.Println("Unixtime is " + fmt.Sprintf("%d", unixtime))
		unixtime = unixtime % 100000

		//Hostnameをracoon-XXXXXに変更する
		hostname := "racoon-" + fmt.Sprintf("%05d", unixtime)

		log.Println("Change Hostname To " + hostname)
		//Change Hostname
		//hostnamectl set-hostname raspberrypi コマンド実行
		cmd = exec.Command("hostnamectl", "set-hostname", hostname)
		cmd.Run()

		//再起動
		log.Println("=====Reboot=====")

		buzzer := rpio.Pin(12)
		buzzer.Mode(rpio.Pwm)
		buzzer.Freq(1175 * 64)
		buzzer.DutyCycle(16, 32)
		time.Sleep(500 * time.Millisecond)
		buzzer.DutyCycle(0, 32)
		buzzer.Freq(1396 * 64)
		buzzer.DutyCycle(16, 32)
		time.Sleep(500 * time.Millisecond)
		buzzer.DutyCycle(0, 32)
		buzzer.Freq(1760 * 64)
		buzzer.DutyCycle(16, 32)
		time.Sleep(500 * time.Millisecond)
		buzzer.DutyCycle(0, 32)

		//reboot コマンド実行
		cmd = exec.Command("reboot")
		cmd.Run()

	}

	//MyIDで指定したロボットIDを取得
	var MyID uint32 = uint32(diptoid)

	chclient := make(chan bool)
	chserver := make(chan bool)
	chserial := make(chan bool)
	chkick := make(chan bool)
	chgpio := make(chan bool)
	chapi := make(chan bool)

	//各並列処理部分
	go RunClient(chclient, MyID, ip)
	go RunServer(chserver, MyID)
	go RunSerial(chserial, MyID)
	go kickCheck(chkick)
	go RunGPIO(chgpio)
	go RunApi(chapi, MyID)

	<-chclient
	<-chserver
	<-chserial
	<-chkick
	<-chgpio
	<-chapi
}

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

// IMU Resetを確実に行うためのフラグ
// 待機モードにうつり、これがセットされているときはAIから受け取らない
var imuResetPending bool = false

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
				if imuResetPending == false {
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

func CheckError(err error) {
	if err != nil {
		log.Fatal("Error: ", err)
	}
}
