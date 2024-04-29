package main

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/stianeikeland/go-rpio/v4"
)

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

	// ここで音楽を鳴らす
	buzzer.Freq(1244 * 64)
	buzzer.DutyCycle(16, 32)
	time.Sleep(time.Millisecond * 100)
	buzzer.Freq(1108 * 64)
	time.Sleep(time.Millisecond * 100)
	buzzer.Freq(739 * 64)
	time.Sleep(time.Millisecond * 150)
	buzzer.DutyCycle(0, 32)
	time.Sleep(time.Millisecond * 100)
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

				if button1.Read()^1 == rpio.High || alarmIgnore {
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
