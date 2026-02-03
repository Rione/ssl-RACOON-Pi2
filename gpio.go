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

	// ここで音楽を鳴らす
	// buzzer.Freq(1244 * 64)
	// buzzer.DutyCycle(16, 32)
	// time.Sleep(time.Millisecond * 100)
	// buzzer.Freq(1108 * 64)
	// time.Sleep(time.Millisecond * 100)
	// buzzer.Freq(739 * 64)
	// time.Sleep(time.Millisecond * 150)
	// buzzer.DutyCycle(0, 32)
	// time.Sleep(time.Millisecond * 100)
	// buzzer.Freq(1479 * 64)
	// buzzer.DutyCycle(16, 32)
	// time.Sleep(time.Millisecond * 100)
	// buzzer.DutyCycle(0, 32)
	// time.Sleep(time.Millisecond * 100)
	// buzzer.DutyCycle(16, 32)
	// time.Sleep(time.Millisecond * 100)
	// buzzer.DutyCycle(0, 32)

	ringBuzzer(13, 200*time.Millisecond,0)  //シ
	ringBuzzer(9, 200*time.Millisecond,0)  //ソ
	ringBuzzer(4, 200*time.Millisecond,0)  //レ
	ringBuzzer(9, 200*time.Millisecond,0)  //ソ
	ringBuzzer(11, 200*time.Millisecond,0) //ラ
	ringBuzzer(16, 200*time.Millisecond,0) //レ
	time.Sleep(50 * time.Millisecond)
	ringBuzzer(4, 200*time.Millisecond,0) //レ
	ringBuzzer(11, 200*time.Millisecond,0) //ラ
	ringBuzzer(13, 200*time.Millisecond,0)  //シ
	ringBuzzer(11, 200*time.Millisecond,0) //ラ
	ringBuzzer(4, 200*time.Millisecond,0) //レ
	ringBuzzer(9, 200*time.Millisecond,0)  //ソ

	// ringBuzzer(0, 1000*time.Millisecond)  //ラ#
	// ringBuzzer(1, 1000*time.Millisecond)  //シ
	// ringBuzzer(2, 1000*time.Millisecond)  //ド
	// ringBuzzer(3, 1000*time.Millisecond)  //ド#
	// ringBuzzer(4, 1000*time.Millisecond)  //レ
	// ringBuzzer(5, 1000*time.Millisecond)  //レ#
	// ringBuzzer(6, 1000*time.Millisecond)  //ミ
	// ringBuzzer(7, 1000*time.Millisecond)  //ファ
	// ringBuzzer(8, 1000*time.Millisecond)  //ファ#
	// ringBuzzer(9, 1000*time.Millisecond)  //ソ
	// ringBuzzer(10, 1000*time.Millisecond) //ソ#
	// ringBuzzer(11, 1000*time.Millisecond) //ラ
	// ringBuzzer(12, 1000*time.Millisecond) //ラ#
	// ringBuzzer(13, 1000*time.Millisecond) //シ
	// ringBuzzer(14, 1000*time.Millisecond) //ド
	// ringBuzzer(15, 1000*time.Millisecond) //ド#
	// ringBuzzer(16, 1000*time.Millisecond) //レ
	// ringBuzzer(17, 1000*time.Millisecond) //レ#
	// ringBuzzer(18, 1000*time.Millisecond) //ミ
	// ringBuzzer(19, 1000*time.Millisecond) //ファ
	// ringBuzzer(20, 1000*time.Millisecond) //ファ#
	// ringBuzzer(21, 1000*time.Millisecond) //ソ
	// ringBuzzer(22, 1000*time.Millisecond) //ソ#

	//GPIO 6, 25, 4, 5 を DIP 1, 2, 3, 4 に設定。入力
	dip1 := rpio.Pin(4)
	dip1.Input()
	dip1.PullUp()
	dip2 := rpio.Pin(5)
	dip2.Input()
	dip2.PullUp()
	dip3 := rpio.Pin(6)
	dip3.Input()
	dip3.PullUp()
	dip4 := rpio.Pin(25)
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
			log.Println("BATTERY ALARM")
			for {
				if recvdata.Volt <= uint8(BATTERY_CRITICAL_THRESHOULD) {
					ringBuzzer(25, 5000*time.Millisecond, 0)
					continue
				}

				led2.High()
				go ringBuzzer(25, 50*time.Millisecond, 0)
				time.Sleep(60 * time.Millisecond)
				go ringBuzzer(20, 90*time.Millisecond, 0)
				led2.Low()
				time.Sleep(120 * time.Millisecond)

				if button1.Read()^1 == rpio.High || alarmIgnore {
					//一時的にアラーム解除する
					log.Println("BATTERY ALARM IGNORED")
					alarmVoltage = BATTERY_CRITICAL_THRESHOULD
					time.Sleep(100 * time.Millisecond)
					go ringBuzzer(20, 20*time.Millisecond, 0)
					time.Sleep(300 * time.Millisecond)
					go ringBuzzer(20, 20*time.Millisecond, 0)
					time.Sleep(300 * time.Millisecond)
					go ringBuzzer(20, 20*time.Millisecond, 0)
					time.Sleep(300 * time.Millisecond)
					break
				}
			}
		} else {
			//通常チカチカ。ボタンが押されたら高速チカチカ
			//button2はkickボタン
			// buzzer.Freq(1479 * 64)
			time.Sleep(ledsec)
			led.Write(rpio.High)
			if button1.Read()^1 == rpio.High {
				ledsec = 75 * time.Millisecond
				go ringBuzzer(20, 20*time.Millisecond, 0)
			} else {
				ledsec = 500 * time.Millisecond
			}
			if button2.Read()^1 == rpio.High {
				go ringBuzzer(10, 50*time.Millisecond, 0)

				//log.Println(button2.Read())
				//kickする
				//buzzer.DutyCycle(16, 32)
				//time.Sleep(500 * time.Millisecond)
				//buzzer.DutyCycle(0, 32)
				//kicker_enable = true
				//kicker_val = 100
			}
			time.Sleep(ledsec)
			led.Write(rpio.Low)
		}
	}
}

func ringBuzzer(buzzerTone int, buzzerTime time.Duration, freq int) {
	//GPIO12をブザーPWMに設定。出力
	buzzer := rpio.Pin(13)
	buzzer.Mode(rpio.Pwm)

	if freq == 0 {
		freq = int(440*math.Pow(1.0595, float64(buzzerTone))) * 64
	} else {
		freq = freq * 64
	}

	buzzer.Freq(freq)
	buzzer.DutyCycle(16, 32)
	time.Sleep(buzzerTime)
	buzzer.DutyCycle(0, 32)

}
