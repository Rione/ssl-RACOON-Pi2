package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"sync"
	"time"
)

// SysfsPWM は /sys/class/pwm を通じてハードウェアPWMを制御する
type SysfsPWM struct {
	mu       sync.Mutex
	chipPath string // 例: "/sys/class/pwm/pwmchip0"
	pwmPath  string // 例: "/sys/class/pwm/pwmchip0/pwm0"
	channel  int
}

var buzzerPWM *SysfsPWM

// initBuzzerPWM はブザー用PWMチャンネルを初期化する
func initBuzzerPWM() {
	pwm := &SysfsPWM{
		chipPath: PWM_CHIP_PATH,
		pwmPath:  fmt.Sprintf("%s/pwm%d", PWM_CHIP_PATH, PWM_CHANNEL),
		channel:  PWM_CHANNEL,
	}

	// Export（すでにExportされている場合はエラーを無視）
	if _, err := os.Stat(pwm.pwmPath); os.IsNotExist(err) {
		log.Println("Exporting PWM...")
		if err := writeSysfs(pwm.chipPath+"/export", strconv.Itoa(PWM_CHANNEL)); err != nil {
			log.Printf("WARNING: Failed to export PWM (%v). Buzzer disabled.", err)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	writeSysfs(pwm.pwmPath+"/duty_cycle", "0")
	writeSysfs(pwm.pwmPath+"/enable", "0")

	buzzerPWM = pwm
	log.Printf("Buzzer PWM initialized: %s", pwm.pwmPath)
}

func writeSysfs(path string, val string) error {
	return os.WriteFile(path, []byte(val), 0644)
}

func (p *SysfsPWM) close() {
	writeSysfs(p.pwmPath+"/enable", "0")
	writeSysfs(p.pwmPath+"/duty_cycle", "0")
	writeSysfs(p.chipPath+"/unexport", strconv.Itoa(p.channel))
}

// ringBuzzer は指定されたトーンと長さでブザーを鳴らす
// buzzerTone: 音階（0から始まる半音単位、0=A#, 440Hzベース）
// buzzerTime: 再生時間
// freq: 直接周波数指定（0の場合はbuzzerToneから計算）
func ringBuzzer(buzzerTone int, buzzerTime time.Duration, freq int) {
	if buzzerPWM == nil {
		return
	}

	buzzerPWM.mu.Lock()
	defer buzzerPWM.mu.Unlock()

	const baseFrequency = 440.0
	const semitoneRatio = 1.0595

	var frequency float64
	if freq == 0 {
		frequency = baseFrequency * math.Pow(semitoneRatio, float64(buzzerTone))
	} else {
		frequency = float64(freq)
	}

	periodNs := int(1_000_000_000.0 / frequency)
	dutyNs := periodNs / 2

	writeSysfs(buzzerPWM.pwmPath+"/duty_cycle", "0")
	writeSysfs(buzzerPWM.pwmPath+"/period", strconv.Itoa(periodNs))
	writeSysfs(buzzerPWM.pwmPath+"/duty_cycle", strconv.Itoa(dutyNs))
	writeSysfs(buzzerPWM.pwmPath+"/enable", "1")

	time.Sleep(buzzerTime)

	writeSysfs(buzzerPWM.pwmPath+"/enable", "0")
	writeSysfs(buzzerPWM.pwmPath+"/duty_cycle", "0")
}

// ringBuzzerDirect は周波数を直接指定してブザーを鳴らす（メロディ再生用）
func ringBuzzerDirect(freq int, duration time.Duration) {
	ringBuzzer(0, duration, freq)
}
