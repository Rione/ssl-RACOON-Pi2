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

	// ブザーPWM初期化
	initBuzzerPWM()

	if checkInitialButtonState() {
		log.Println("Button1 is pressed. Start Robot Control Mode")
		isControlByRobotMode = true
	}

	hostname := getHostname()
	fmt.Println(hostname)

	// 初期ホスト名の場合、新しいホスト名を設定して再起動
	if hostname == "DietPi\n" {
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
	button1, err := openInputGPIO(PIN_BUTTON1_BANK, PIN_BUTTON1_PORT, PIN_BUTTON1_PIN)
	if err != nil {
		log.Fatalf("Button1 pin request failed: %v", err)
	}
	defer button1.Close()
	return isPressed(button1)
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

	// ホスト名を変更（D-Bus 不要: /etc/hostname 書き込み + hostname コマンド）
	exec.Command("sudo", "sh", "-c", fmt.Sprintf("echo '%s' > /etc/hostname", hostname)).Run()
	exec.Command("sudo", "hostname", hostname).Run()
	exec.Command("sudo", "sed", "-i", "s/DietPi/"+hostname+"/g", "/etc/hosts").Run()

	log.Println("=====Reboot=====")

	// 再起動音を鳴らす
	playRebootMelody()

	exec.Command("reboot").Run()
}

// playRebootMelody は再起動時のメロディを再生する
func playRebootMelody() {
	notes := []int{1175, 1396, 1760}
	for _, freq := range notes {
		ringBuzzerDirect(freq, 500*time.Millisecond)
	}
}

// handleLocalUserMode はローカルユーザーモードの初期化を行う
// ボタンが押されていればtrue、そうでなければfalseを返す
func handleLocalUserMode() bool {
	ringBuzzerDirect(1175, 1000*time.Millisecond)
	time.Sleep(1000 * time.Millisecond)

	button1, err := openInputGPIO(PIN_BUTTON1_BANK, PIN_BUTTON1_PORT, PIN_BUTTON1_PIN)
	if err != nil {
		log.Fatalf("Button1 pin request failed: %v", err)
	}
	defer button1.Close()

	if isPressed(button1) {
		isControlByRobotMode = true
		log.Println("Robot Control Mode is ON")
		// 確認音を2回鳴らす
		for i := 0; i < 2; i++ {
			ringBuzzerDirect(1244, 100*time.Millisecond)
			time.Sleep(100 * time.Millisecond)
		}
		return true
	}
	return false
}

// readRobotIDFromDIP はDIPスイッチからロボットIDを読み取る
func readRobotIDFromDIP() int {
	dip1, err := openInputGPIO(PIN_DIP1_BANK, PIN_DIP1_PORT, PIN_DIP1_PIN)
	if err != nil {
		log.Fatalf("DIP1 pin request failed: %v", err)
	}
	defer dip1.Close()
	dip2, err := openInputGPIO(PIN_DIP2_BANK, PIN_DIP2_PORT, PIN_DIP2_PIN)
	if err != nil {
		log.Fatalf("DIP2 pin request failed: %v", err)
	}
	defer dip2.Close()
	dip3, err := openInputGPIO(PIN_DIP3_BANK, PIN_DIP3_PORT, PIN_DIP3_PIN)
	if err != nil {
		log.Fatalf("DIP3 pin request failed: %v", err)
	}
	defer dip3.Close()
	dip4, err := openInputGPIO(PIN_DIP4_BANK, PIN_DIP4_PORT, PIN_DIP4_PIN)
	if err != nil {
		log.Fatalf("DIP4 pin request failed: %v", err)
	}
	defer dip4.Close()

	fmt.Println("DIP1:", readInverted(dip1))
	fmt.Println("DIP2:", readInverted(dip2))
	fmt.Println("DIP3:", readInverted(dip3))
	fmt.Println("DIP4:", readInverted(dip4))

	return int(readInverted(dip1) + readInverted(dip2)*2 + readInverted(dip3)*4 + readInverted(dip4)*8)
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
			if buzzerPWM != nil {
				buzzerPWM.close()
			}
			log.Println("Bye")
			os.Exit(0)
		}
	}()
}

// CheckError はエラーがあれば致命的エラーとしてログ出力して終了する
func CheckError(err error) {
	if err != nil {
		log.Println("oops")
		log.Fatal("Error: ", err)
	}
}
