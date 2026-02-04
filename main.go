package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/stianeikeland/go-rpio/v4"
)

// kickCheck はキッカーパワーが入力された時に、一定時間後に値をリセットする関数である
// goroutineとして並列実行される
func kickCheck(done <-chan struct{}) {
	ticker := time.NewTicker(16 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// ストレートキックが入力されたとき
			if kickerEnable {
				time.Sleep(KICK_HOLD_DURATION)
				kickerEnable = false
				kickerVal = 0
				doDirectKick = false
			}
			// チップキックが入力されたとき
			if chipEnable {
				time.Sleep(KICK_HOLD_DURATION)
				chipEnable = false
				chipVal = 0
				doDirectChipKick = false
			}
			// IMUエラー状態のリセット
			if imuError {
				time.Sleep(KICK_HOLD_DURATION)
				imuError = false
			}
		}
	}
}

func main() {
	parseFlags()

	if checkInitialButtonState() {
		log.Println("Button1 is pressed. Start Robot Control Mode")
		isControlByRobotMode = true
	}

	hostname := getHostname()
	fmt.Println(hostname)

	// 初期ホスト名の場合、新しいホスト名を設定して再起動
	if hostname == "raspberrypi\n" {
		setupNewHostname()
		return
	}

	// ロボット制御モードまたはローカルユーザーの場合
	if isControlByRobotMode {
		log.Println("Robot Control Mode is ON")
		hostname = "localuser\n"
	}

	if hostname == "localuser\n" {
		if !handleLocalUserMode() {
			os.Exit(0)
		}
	}

	// 自動アップデート
	go confirmAndSelfUpdate()

	// GPIOの初期化
	if err := rpio.Open(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// DIPスイッチからロボットIDを読み取り
	robotID := readRobotIDFromDIP()
	fmt.Println("GOT ID FROM DIP SW:", robotID)

	// ローカルIPアドレスを取得
	ip := getLocalIP()
	ipCamera := "127.0.0.1"

	// シグナルハンドラの設定（Ctrl+C対応）
	setupSignalHandler()

	var myID uint32 = uint32(robotID)

	// 終了シグナル用チャネル
	done := make(chan struct{})

	// 各goroutineを起動
	go RunClient(done, myID, ip)
	go RunServer(done, myID)
	go RunSerial(done, myID)
	go kickCheck(done)
	go RunGPIO(done)
	go RunApi(done, myID)
	go ReceiveData(done, myID, ipCamera)

	// 終了を待機（無限ループ）
	select {}
}

// parseFlags はコマンドライン引数を解析する
func parseFlags() {
	flag.BoolVar(&debugSerial, "ds", false, "シリアル送受信のモニタリングを有効化")
	flag.BoolVar(&debugReceive, "dr", false, "AIからの受信結果表示を有効化")
	flag.Parse()

	if debugSerial {
		log.Println("Debug Mode: Serial monitoring enabled (-ds)")
	}
	if debugReceive {
		log.Println("Debug Mode: AI receive monitoring enabled (-dr)")
	}
}

// checkInitialButtonState は起動時のボタン状態を確認する
func checkInitialButtonState() bool {
	if err := rpio.Open(); err != nil {
		log.Fatal("Error: ", err)
	}
	defer rpio.Close()

	button1 := rpio.Pin(PIN_BUTTON1)
	button1.Input()
	button1.PullUp()

	return button1.Read()^1 == rpio.High
}

// getHostname は現在のホスト名を取得する
func getHostname() string {
	cmd := exec.Command("hostname")
	out, err := cmd.Output()
	if err != nil {
		panic(err)
	}
	return string(out)
}

// setupNewHostname は新しいホスト名を設定し、システムを再起動する
func setupNewHostname() {
	// UNIX時間の下5桁を使用してユニークなホスト名を生成
	unixtime := time.Now().UnixNano() % 100000
	hostname := fmt.Sprintf("racoon-%05d", unixtime)

	log.Printf("Unixtime is %d", time.Now().UnixNano())
	log.Println("Change Hostname To " + hostname)

	// ホスト名を変更
	exec.Command("hostnamectl", "set-hostname", hostname).Run()
	exec.Command("sudo", "sed", "-i", "/etc/hosts", "-e", "s/raspberrypi/"+hostname+"/g", "/etc/hosts").Run()

	log.Println("=====Reboot=====")

	// 再起動音を鳴らす
	if err := rpio.Open(); err == nil {
		playRebootMelody()
		rpio.Close()
	}

	exec.Command("reboot").Run()
}

// playRebootMelody は再起動時のメロディを再生する
func playRebootMelody() {
	buzzer := rpio.Pin(PIN_BUZZER)
	buzzer.Mode(rpio.Pwm)

	notes := []int{1175, 1396, 1760}
	for _, freq := range notes {
		buzzer.Freq(freq * 64)
		buzzer.DutyCycle(16, 32)
		time.Sleep(500 * time.Millisecond)
		buzzer.DutyCycle(0, 32)
	}
}

// handleLocalUserMode はローカルユーザーモードの初期化を行う
// ボタンが押されていればtrue、そうでなければfalseを返す
func handleLocalUserMode() bool {
	if err := rpio.Open(); err != nil {
		CheckError(err)
	}

	buzzer := rpio.Pin(PIN_BUZZER)
	buzzer.Mode(rpio.Pwm)
	buzzer.Freq(1175 * 64)
	buzzer.DutyCycle(16, 32)
	time.Sleep(1000 * time.Millisecond)
	buzzer.DutyCycle(0, 32)
	time.Sleep(1000 * time.Millisecond)

	button1 := rpio.Pin(PIN_BUTTON1)
	button1.Input()
	button1.PullUp()

	if button1.Read()^1 == rpio.High {
		isControlByRobotMode = true
		log.Println("Robot Control Mode is ON")
		// 確認音を2回鳴らす
		for i := 0; i < 2; i++ {
			buzzer.Freq(1244 * 64)
			buzzer.DutyCycle(16, 32)
			time.Sleep(100 * time.Millisecond)
			buzzer.DutyCycle(0, 32)
			time.Sleep(100 * time.Millisecond)
		}
		return true
	}
	return false
}

// readRobotIDFromDIP はDIPスイッチからロボットIDを読み取る
func readRobotIDFromDIP() int {
	dip1 := rpio.Pin(PIN_DIP1)
	dip1.Input()
	dip1.PullUp()
	dip2 := rpio.Pin(PIN_DIP2)
	dip2.Input()
	dip2.PullUp()
	dip3 := rpio.Pin(PIN_DIP3)
	dip3.Input()
	dip3.PullUp()
	dip4 := rpio.Pin(PIN_DIP4)
	dip4.Input()
	dip4.PullUp()

	fmt.Println("DIP1:", dip1.Read()^1)
	fmt.Println("DIP2:", dip2.Read()^1)
	fmt.Println("DIP3:", dip3.Read()^1)
	fmt.Println("DIP4:", dip4.Read()^1)

	return int(dip1.Read() ^ 1 + (dip2.Read()^1)*2 + (dip3.Read()^1)*4 + (dip4.Read()^1)*8)
}

// getLocalIP はローカルIPアドレスを取得する
func getLocalIP() string {
	netInterfaceAddresses, _ := net.InterfaceAddrs()

	for _, netInterfaceAddress := range netInterfaceAddresses {
		networkIP, ok := netInterfaceAddress.(*net.IPNet)
		if ok && !networkIP.IP.IsLoopback() && networkIP.IP.To4() != nil {
			return networkIP.IP.String()
		}
	}
	return "0.0.0.0"
}

// setupSignalHandler はCtrl+Cなどのシグナルハンドラを設定する
func setupSignalHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			rpio.Close()
			log.Println("Bye")
			os.Exit(0)
		}
	}()
}

// CheckError はエラーがあれば致命的エラーとしてログ出力して終了する
func CheckError(err error) {
	if err != nil {
		log.Fatal("Error: ", err)
	}
}
