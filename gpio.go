package main

import (
	"fmt"
	"log"
	"time"

	"github.com/Yuzz1e/rock5a-gpio-go"
)

// LED点滅速度
const (
	LED_BLINK_NORMAL = 500 * time.Millisecond // 通常時の点滅間隔
	LED_BLINK_FAST   = 75 * time.Millisecond  // 高速点滅間隔
)

// openOutputGPIO は出力ピンを開き、初期値を設定する（rock5a-gpio-go）
func openOutputGPIO(bank, port, pin int, initialHigh bool) (*gpio.GPIO, error) {
	g, err := gpio.OpenGPIO(bank, port, pin)
	if err != nil {
		return nil, err
	}
	if err := g.SetDirection("out"); err != nil {
		g.Close()
		return nil, err
	}
	val := "0"
	if initialHigh {
		val = "1"
	}
	if err := g.Write(val); err != nil {
		g.Close()
		return nil, err
	}
	return g, nil
}

// openInputGPIO は入力ピンを開く（プルアップなし・アクティブロー想定）
// 必要なら呼び出し前に gpio.SetPull(bank, rune('A'+port), pin, gpio.Floating) で明示可能
func openInputGPIO(bank, port, pin int) (*gpio.GPIO, error) {
	g, err := gpio.OpenGPIO(bank, port, pin)
	if err != nil {
		return nil, err
	}
	if err := g.SetDirection("in"); err != nil {
		g.Close()
		return nil, err
	}
	return g, nil
}

// readInverted はアクティブローのピンを読み取り、押下時に1を返す（物理Low＝論理1）
func readInverted(g *gpio.GPIO) uint8 {
	v, err := g.Read()
	if err != nil {
		log.Printf("GPIO read error: %v", err)
		return 0
	}
	if v == "0" {
		return 1
	}
	return 0
}

// isPressed はアクティブローのボタンが押されているかを返す（Read()=="0" で押下）
func isPressed(g *gpio.GPIO) bool {
	v, err := g.Read()
	if err != nil {
		return false
	}
	return v == "0"
}

// setOutput は出力ピンの値を設定する（LED用: 1=点灯, 0=消灯）
func setOutput(g *gpio.GPIO, high bool) {
	if high {
		_ = g.Write("1")
	} else {
		_ = g.Write("0")
	}
}

// RunGPIO はGPIOの制御を行うメインループである
func RunGPIO(done <-chan struct{}) {
	led, err := openOutputGPIO(PIN_LED1_BANK, PIN_LED1_PORT, PIN_LED1_PIN, false)
	if err != nil {
		log.Printf("GPIO LED1 request failed (%d,%d,%d): %v", PIN_LED1_BANK, PIN_LED1_PORT, PIN_LED1_PIN, err)
		return
	}
	defer led.Close()

	led2, err := openOutputGPIO(PIN_LED2_BANK, PIN_LED2_PORT, PIN_LED2_PIN, false)
	if err != nil {
		log.Printf("GPIO LED2 request failed (%d,%d,%d): %v", PIN_LED2_BANK, PIN_LED2_PORT, PIN_LED2_PIN, err)
		return
	}
	defer led2.Close()

	button1, err := openInputGPIO(PIN_BUTTON1_BANK, PIN_BUTTON1_PORT, PIN_BUTTON1_PIN)
	if err != nil {
		log.Printf("GPIO Button1 request failed: %v", err)
		return
	}
	defer button1.Close()

	button2, err := openInputGPIO(PIN_BUTTON2_BANK, PIN_BUTTON2_PORT, PIN_BUTTON2_PIN)
	if err != nil {
		log.Printf("GPIO Button2 request failed: %v", err)
		return
	}
	defer button2.Close()

	// 起動メロディを再生
	playStartupMelody()

	// DIPスイッチの状態を表示（デバッグ用）
	printDIPStatus()

	// メインループ
	ledInterval := LED_BLINK_NORMAL
	alarmVoltage := BATTERY_LOW_THRESHOLD

	for {
		select {
		case <-done:
			return
		default:
			if recvdata.Volt <= uint8(alarmVoltage) {
				handleBatteryAlarm(led2, button1, &alarmVoltage)
			} else {
				ledInterval = handleNormalOperation(led, button1, button2, ledInterval)
			}
		}
	}
}

// playStartupMelody は起動時のメロディを再生する
func playStartupMelody() {
	// メロディ: シ-ソ-レ-ソ-ラ-レ（休符）レ-ラ-シ-ラ-レ-ソ
	melody := []struct {
		tone     int
		duration time.Duration
	}{
		{13, 150 * time.Millisecond}, // シ
		{9, 150 * time.Millisecond},  // ソ
		{4, 150 * time.Millisecond},  // レ
		{9, 150 * time.Millisecond},  // ソ
		{11, 150 * time.Millisecond}, // ラ
		{16, 300 * time.Millisecond}, // レ
	}

	for _, note := range melody {
		ringBuzzer(note.tone, note.duration, 0)
	}

	time.Sleep(75 * time.Millisecond)

	melody2 := []struct {
		tone     int
		duration time.Duration
	}{
		{4, 150 * time.Millisecond},  // レ
		{11, 150 * time.Millisecond}, // ラ
		{13, 150 * time.Millisecond}, // シ
		{11, 150 * time.Millisecond}, // ラ
		{4, 150 * time.Millisecond},  // レ
		{9, 300 * time.Millisecond},  // ソ
	}

	for _, note := range melody2 {
		ringBuzzer(note.tone, note.duration, 0)
	}
}

// printDIPStatus はDIPスイッチの状態を出力する
func printDIPStatus() {
	dip1, err := openInputGPIO(PIN_DIP1_BANK, PIN_DIP1_PORT, PIN_DIP1_PIN)
	if err != nil {
		log.Printf("GPIO DIP1 request failed: %v", err)
		return
	}
	defer dip1.Close()
	dip2, err := openInputGPIO(PIN_DIP2_BANK, PIN_DIP2_PORT, PIN_DIP2_PIN)
	if err != nil {
		log.Printf("GPIO DIP2 request failed: %v", err)
		return
	}
	defer dip2.Close()
	dip3, err := openInputGPIO(PIN_DIP3_BANK, PIN_DIP3_PORT, PIN_DIP3_PIN)
	if err != nil {
		log.Printf("GPIO DIP3 request failed: %v", err)
		return
	}
	defer dip3.Close()
	dip4, err := openInputGPIO(PIN_DIP4_BANK, PIN_DIP4_PORT, PIN_DIP4_PIN)
	if err != nil {
		log.Printf("GPIO DIP4 request failed: %v", err)
		return
	}
	defer dip4.Close()

	fmt.Println("DIP1:", readInverted(dip1))
	fmt.Println("DIP2:", readInverted(dip2))
	fmt.Println("DIP3:", readInverted(dip3))
	fmt.Println("DIP4:", readInverted(dip4))

	hex := readInverted(dip1) + readInverted(dip2)*2 + readInverted(dip3)*4 + readInverted(dip4)*8
	fmt.Println("HEX:", int(hex))
}

// handleBatteryAlarm はバッテリー低下アラームを処理する
func handleBatteryAlarm(led2, button1 *gpio.GPIO, alarmVoltage *int) {
	log.Println("BATTERY ALARM")

	for {
		// 危険電圧の場合は長時間アラーム
		if recvdata.Volt <= uint8(BATTERY_CRITICAL_THRESHOLD) {
			ringBuzzer(25, 5000*time.Millisecond, 0)
			continue
		}

		// 警告アラーム
		setOutput(led2, true)
		go ringBuzzer(25, 50*time.Millisecond, 0)
		time.Sleep(60 * time.Millisecond)
		go ringBuzzer(20, 90*time.Millisecond, 0)
		setOutput(led2, false)
		time.Sleep(120 * time.Millisecond)

		// ボタンでアラーム解除
		if isPressed(button1) || alarmIgnore {
			log.Println("BATTERY ALARM IGNORED")
			*alarmVoltage = BATTERY_CRITICAL_THRESHOLD
			playAlarmDismissSound()
			break
		}
	}
}

// playAlarmDismissSound はアラーム解除時の確認音を再生する
func playAlarmDismissSound() {
	for i := 0; i < 3; i++ {
		time.Sleep(100 * time.Millisecond)
		go ringBuzzer(20, 20*time.Millisecond, 0)
		time.Sleep(300 * time.Millisecond)
	}
}

// handleNormalOperation は通常のLED点滅とボタン処理を行う
func handleNormalOperation(led, button1, button2 *gpio.GPIO, ledInterval time.Duration) time.Duration {
	time.Sleep(ledInterval)
	setOutput(led, true)

	// ボタン1が押されたら高速点滅
	if isPressed(button1) {
		ledInterval = LED_BLINK_FAST
		go ringBuzzer(20, 20*time.Millisecond, 0)
	} else {
		ledInterval = LED_BLINK_NORMAL
	}

	// ボタン2が押されたら音を鳴らす
	if isPressed(button2) {
		go ringBuzzer(10, 50*time.Millisecond, 0)
	}

	time.Sleep(ledInterval)
	setOutput(led, false)

	return ledInterval
}
