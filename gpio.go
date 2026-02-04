package main

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/stianeikeland/go-rpio/v4"
)

// LED点滅速度
const (
	LED_BLINK_NORMAL = 500 * time.Millisecond // 通常時の点滅間隔
	LED_BLINK_FAST   = 75 * time.Millisecond  // 高速点滅間隔
)

// RunGPIO はGPIOの制御を行うメインループである
func RunGPIO(done <-chan struct{}) {
	if err := rpio.Open(); err != nil {
		CheckError(err)
	}

	// LED初期化
	led := rpio.Pin(PIN_LED1)
	led.Output()
	led2 := rpio.Pin(PIN_LED2)
	led2.Output()

	// ボタン初期化
	button1 := rpio.Pin(PIN_BUTTON1)
	button1.Input()
	button1.PullUp()
	button2 := rpio.Pin(PIN_BUTTON2)
	button2.Input()
	button2.PullUp()

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

	hex := dip1.Read() ^ 1 + (dip2.Read()^1)*2 + (dip3.Read()^1)*4 + (dip4.Read()^1)*8
	fmt.Println("HEX:", int(hex))
}

// handleBatteryAlarm はバッテリー低下アラームを処理する
func handleBatteryAlarm(led2, button1 rpio.Pin, alarmVoltage *int) {
	log.Println("BATTERY ALARM")

	for {
		// 危険電圧の場合は長時間アラーム
		if recvdata.Volt <= uint8(BATTERY_CRITICAL_THRESHOLD) {
			ringBuzzer(25, 5000*time.Millisecond, 0)
			continue
		}

		// 警告アラーム
		led2.High()
		go ringBuzzer(25, 50*time.Millisecond, 0)
		time.Sleep(60 * time.Millisecond)
		go ringBuzzer(20, 90*time.Millisecond, 0)
		led2.Low()
		time.Sleep(120 * time.Millisecond)

		// ボタンでアラーム解除
		if button1.Read()^1 == rpio.High || alarmIgnore {
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
func handleNormalOperation(led, button1, button2 rpio.Pin, ledInterval time.Duration) time.Duration {
	time.Sleep(ledInterval)
	led.Write(rpio.High)

	// ボタン1が押されたら高速点滅
	if button1.Read()^1 == rpio.High {
		ledInterval = LED_BLINK_FAST
		go ringBuzzer(20, 20*time.Millisecond, 0)
	} else {
		ledInterval = LED_BLINK_NORMAL
	}

	// ボタン2が押されたら音を鳴らす
	if button2.Read()^1 == rpio.High {
		go ringBuzzer(10, 50*time.Millisecond, 0)
	}

	time.Sleep(ledInterval)
	led.Write(rpio.Low)

	return ledInterval
}

// ringBuzzer は指定されたトーンと長さでブザーを鳴らす
// buzzerTone: 音階（0から始まる半音単位、0=A#, 440Hzベース）
// buzzerTime: 再生時間
// freq: 直接周波数指定（0の場合はbuzzerToneから計算）
func ringBuzzer(buzzerTone int, buzzerTime time.Duration, freq int) {
	buzzer := rpio.Pin(PIN_BUZZER)
	buzzer.Mode(rpio.Pwm)

	// 周波数の計算（PWMは64倍の値を使用）
	const pwmMultiplier = 64
	const baseFrequency = 440.0
	const semitoneRatio = 1.0595 // 12平均律の半音比

	var frequency int
	if freq == 0 {
		frequency = int(baseFrequency*math.Pow(semitoneRatio, float64(buzzerTone))) * pwmMultiplier
	} else {
		frequency = freq * pwmMultiplier
	}

	buzzer.Freq(frequency)
	buzzer.DutyCycle(16, 32)
	time.Sleep(buzzerTime)
	buzzer.DutyCycle(0, 32)
}
