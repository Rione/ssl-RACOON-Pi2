//go:build rock5a

// SPI送受信テスト用ツール（Rock5A Master → ロボットMCU Slave）
//
// ビルド:
//
//	go build -o spi_test ./cmd/spi_test
//
// 実行例:
//
//	sudo ./spi_test                          # 1秒間隔で連続送信
//	sudo ./spi_test -count 10 -velx 500      # VelX=500mm/s を10回送信
//	sudo ./spi_test -once -kick 50 -dribble 30
//	sudo ./spi_test -sweep -interval 16ms   # VelX: -1000→0→1000→0→-1000 を繰り返し
//	sudo ./spi_test -sweep -mismatch-only   # フレームずれ時のみ表示
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"periph.io/x/conn/v3/physic"
	"periph.io/x/conn/v3/spi"
	"periph.io/x/conn/v3/spi/spireg"
	"periph.io/x/host/v3"
)

const (
	spiDevPath   = "/dev/spidev4.0"
	spiSpeedHz   = 1_000_000
	spiFrameSize = 18
	spiRecvSize  = 4
)

var spiExpectedRecvPayload = []byte{0x64, 0x78, 0x82, 0x8c}

const (
	infoDoCharge       = 0b00010000
	infoSignalReceived = 0b00100000
)

// sendFrame は本番 SendStruct と同じ 18 バイトレイアウト（LittleEndian）
type sendFrame struct {
	VelX          int16
	VelY          int16
	VelAng        int16
	DribblePower  uint8
	KickPower     uint8
	ChipPower     uint8
	RelativeX     int16
	RelativeY     int16
	RelativeTheta int16
	CameraBallX   uint8
	CameraBallY   uint8
	Informations  uint8
}

// recvFrame は本番 RecvStruct と同じ 4 バイト（BigEndian）
type recvFrame struct {
	Volt              uint8
	SensorInformation uint8
	CapPower          uint8
	Reserved          uint8
}

// velXSweep は VelX を -max → 0 → max → 0 → -max と三角波スイープする
type velXSweep struct {
	value int
	step  int
	max   int
	dir   int // 1: 増加, -1: 減少
}

func newVelXSweep(max, step int) *velXSweep {
	if step <= 0 {
		step = 100
	}
	if max <= 0 {
		max = 1000
	}
	return &velXSweep{value: -max, step: step, max: max, dir: 1}
}

func (s *velXSweep) next() int {
	current := s.value
	if s.dir > 0 {
		if current >= s.max {
			s.dir = -1
			s.value = s.max - s.step
		} else {
			s.value = current + s.step
		}
	} else {
		if current <= -s.max {
			s.dir = 1
			s.value = -s.max + s.step
		} else {
			s.value = current - s.step
		}
	}
	return current
}

func main() {
	dev := flag.String("dev", spiDevPath, "SPIデバイスパス")
	hz := flag.Int("hz", spiSpeedHz, "SPIクロック [Hz]")
	interval := flag.Duration("interval", time.Second, "送信間隔")
	count := flag.Int("count", 0, "送信回数（0で無限ループ）")
	once := flag.Bool("once", false, "1回だけ送信して終了")

	velX := flag.Int("velx", 0, "VelX [mm/s]")
	velY := flag.Int("vely", 0, "VelY [mm/s]")
	velAng := flag.Int("velang", 0, "VelAng [mrad/s]")
	dribble := flag.Int("dribble", 0, "ドリブルパワー [0-100]")
	kick := flag.Int("kick", 0, "キックパワー [0-255]")
	chip := flag.Int("chip", 0, "チップパワー [0-255]")
	camX := flag.Int("camx", 0, "カメラボールX [0-255]")
	camY := flag.Int("camy", 0, "カメラボールY [0-255]")
	charge := flag.Bool("charge", false, "コンデンサ充電フラグをONにする（デフォルトOFF=安全）")
	sweep := flag.Bool("sweep", false, "VelXを -max→0→max→0→-max とスイープする")
	sweepMax := flag.Int("sweep-max", 1000, "スイープ時のVelX最大絶対値 [mm/s]")
	sweepStep := flag.Int("sweep-step", 100, "スイープ時のVelX刻み幅 [mm/s]")
	mismatchOnly := flag.Bool("mismatch-only", false, "受信フレームずれ(NG)時のみ詳細を表示")

	flag.Parse()

	if *once {
		*count = 1
	}

	if _, err := host.Init(); err != nil {
		log.Fatalf("host.Init: %v", err)
	}

	port, err := spireg.Open(*dev)
	if err != nil {
		log.Fatalf("spireg.Open(%s): %v", *dev, err)
	}
	defer port.Close()

	conn, err := port.Connect(physic.Frequency(*hz)*physic.Hertz, spi.Mode0, 8)
	if err != nil {
		log.Fatalf("Connect: %v", err)
	}

	log.Printf("SPI test start: dev=%s speed=%dHz mode=0 frame=%dB", *dev, *hz, spiFrameSize)
	log.Printf("Expected RX: % x + %d zero bytes", spiExpectedRecvPayload, spiFrameSize-spiRecvSize)
	if *sweep {
		log.Printf("Sweep mode: VelX %d → 0 → %d → 0 → %d (step %d)", -*sweepMax, *sweepMax, -*sweepMax, *sweepStep)
	}
	if *mismatchOnly {
		log.Println("Mismatch-only mode: NG frames only")
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	var sweeper *velXSweep
	if *sweep {
		sweeper = newVelXSweep(*sweepMax, *sweepStep)
	}

	sent := 0
	prevFrameOK := true
	for {
		select {
		case <-sig:
			log.Println("Interrupted")
			return
		default:
		}

		velXNow := *velX
		if sweeper != nil {
			velXNow = sweeper.next()
		}

		tx := buildFrame(velXNow, *velY, *velAng, *dribble, *kick, *chip, *camX, *camY, *charge)
		rx := make([]byte, spiFrameSize)
		txCopy := append([]byte(nil), tx...)
		if err := conn.Tx(tx, rx); err != nil {
			log.Fatalf("Tx: %v", err)
		}

		sent++
		frameErr := validateRecvFrame(rx)
		frameOK := frameErr == nil
		if !frameOK && prevFrameOK {
			log.Printf("!! SPI RX FRAME MISMATCH: %v", frameErr)
		}
		if frameOK && !prevFrameOK {
			log.Println("SPI RX frame recovered")
		}
		prevFrameOK = frameOK

		if !*mismatchOnly || frameErr != nil {
			printResult(sent, txCopy, rx, frameErr)
		}

		if *count > 0 && sent >= *count {
			return
		}

		select {
		case <-sig:
			log.Println("Interrupted")
			return
		case <-ticker.C:
		}
	}
}

func buildFrame(velX, velY, velAng, dribble, kick, chip, camX, camY int, charge bool) []byte {
	info := uint8(infoSignalReceived)
	if charge {
		info |= infoDoCharge
	}

	frame := sendFrame{
		VelX:         int16(velX),
		VelY:         int16(velY),
		VelAng:       int16(velAng),
		DribblePower: clampUint8(dribble, 0, 100),
		KickPower:    clampUint8(kick, 0, 255),
		ChipPower:    clampUint8(chip, 0, 255),
		CameraBallX:  clampUint8(camX, 0, 255),
		CameraBallY:  clampUint8(camY, 0, 255),
		Informations: info,
	}

	buf := &bytes.Buffer{}
	if err := binary.Write(buf, binary.LittleEndian, frame); err != nil {
		log.Fatalf("binary.Write: %v", err)
	}
	return buf.Bytes()
}

func validateRecvFrame(rx []byte) error {
	if len(rx) < spiFrameSize {
		return fmt.Errorf("short frame: got %d bytes, want %d", len(rx), spiFrameSize)
	}
	for i := 0; i < spiRecvSize; i++ {
		if rx[i] != spiExpectedRecvPayload[i] {
			return fmt.Errorf("payload[%d]: expected %02x, got %02x", i, spiExpectedRecvPayload[i], rx[i])
		}
	}
	for i := spiRecvSize; i < spiFrameSize; i++ {
		if rx[i] != 0 {
			return fmt.Errorf("padding[%d]: expected 00, got %02x", i, rx[i])
		}
	}
	return nil
}

func printResult(n int, tx, rx []byte, frameErr error) {
	var recv recvFrame
	if err := binary.Read(bytes.NewReader(rx[:spiRecvSize]), binary.BigEndian, &recv); err != nil {
		log.Printf("[%d] RX parse error: %v", n, err)
		return
	}

	var sent sendFrame
	_ = binary.Read(bytes.NewReader(tx), binary.LittleEndian, &sent)

	status := "OK"
	if frameErr != nil {
		status = "NG"
	}

	fmt.Printf("[%d] %s TX vel=(%d,%d,%d) dribble=%d kick=%d chip=%d info=0b%08b\n",
		n, status, sent.VelX, sent.VelY, sent.VelAng,
		sent.DribblePower, sent.KickPower, sent.ChipPower, sent.Informations)
	fmt.Printf("     RX volt=%d (%.1fV) sensor=0b%08b cap=%d reserved=%d\n",
		recv.Volt, float32(recv.Volt)*0.1, recv.SensorInformation, recv.CapPower, recv.Reserved)
	fmt.Printf("     TX raw (%dB): % x\n", len(tx), tx)
	fmt.Printf("     RX raw (%dB): % x\n", len(rx), rx)
	if frameErr != nil {
		fmt.Printf("     !! %v\n", frameErr)
	}
}

func clampUint8(v, min, max int) uint8 {
	if v < min {
		v = min
	}
	if v > max {
		v = max
	}
	return uint8(v)
}
