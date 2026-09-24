//go:build rock5a

package rock5a

import (
	"math"
	"testing"

	"github.com/Rione/ssl-RACOON-Pi2/internal/state"
)

// buildRxFrame は MainBoard が送ってくる 21byte フレームを組み立てる。
func buildRxFrame(volt, sensor, cap uint8, wheels [4]int16, ax, ay, yawRate, yaw int16) []byte {
	f := make([]byte, SPIFrameSize)
	f[0] = SPIFrameHeader
	f[rxVolt] = volt
	f[rxSensor] = sensor
	f[rxCapPower] = cap
	put := func(i int, v int16) {
		f[i] = byte(uint16(v))
		f[i+1] = byte(uint16(v) >> 8)
	}
	for i, w := range wheels {
		put(rxWheel0+2*i, w)
	}
	put(rxAccelX, ax)
	put(rxAccelY, ay)
	put(rxYawRate, yawRate)
	put(rxYawRad, yaw)
	f[SPIFrameSize-1] = SPIFrameFooter
	return f
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-4 }

func TestParseRecvBufAt(t *testing.T) {
	f := buildRxFrame(152, 0b101, 200, [4]int16{100, -200, 300, -400}, 1234, -500, -900, 15708)
	if err := validateSPIFrame(f); err != nil {
		t.Fatalf("validateSPIFrame: %v", err)
	}
	r := parseRecvBufAt(f, 0)
	if r.Volt != 152 || r.SensorInformation != 0b101 || r.CapPower != 200 {
		t.Errorf("header fields: %+v", r)
	}
	if r.FlWheelSpeed != 100 || r.BlWheelSpeed != -200 || r.BrWheelSpeed != 300 || r.FrWheelSpeed != -400 {
		t.Errorf("wheels: %+v", r)
	}

	imu := imuFromRecv(r)
	want := state.IMUData{AccelX: 1.234, AccelY: -0.5, YawRate: -1.0, Yaw: 1.5708}
	if !near(imu.AccelX, want.AccelX) || !near(imu.AccelY, want.AccelY) ||
		!near(imu.YawRate, want.YawRate) || !near(imu.Yaw, want.Yaw) {
		t.Errorf("imu = %+v, want %+v", imu, want)
	}
}

// ペイロードが 0 埋めされていなくても (IMU 値が入っていても) 有効と判定されること。
func TestValidateNoPaddingCheck(t *testing.T) {
	f := buildRxFrame(150, 0, 0, [4]int16{}, -1, -1, -1, -1)
	if err := validateSPIFrame(f); err != nil {
		t.Fatalf("validateSPIFrame: %v", err)
	}
}

// ビットずれでフレーム境界がずれても、2 フレーム分の窓から再同期できること。
func TestFindSPIFrameResync(t *testing.T) {
	f := buildRxFrame(150, 0, 0, [4]int16{1, 2, 3, 4}, 10, 20, 30, 40)

	// 1 周期目の後半 + 2 周期目の前半に 1 フレームがまたがる状況を作る。
	const shift = 5
	stream := append(append([]byte{}, f...), f...)
	var window [SPIFrameSize * 2]byte
	pushSPIRxWindow(window[:], stream[shift:shift+SPIFrameSize])
	pushSPIRxWindow(window[:], stream[shift+SPIFrameSize:])

	off := findSPIFrame(window[:])
	if off != SPIFrameSize-shift {
		t.Fatalf("findSPIFrame = %d, want %d", off, SPIFrameSize-shift)
	}
	r := parseRecvBufAt(window[:], off)
	if r.FrWheelSpeed != 4 || r.YawRaw != 40 {
		t.Errorf("parsed after resync: %+v", r)
	}
}

func TestEnsureSendFrameReservedByte(t *testing.T) {
	cmd := make([]byte, 18)
	for i := range cmd {
		cmd[i] = byte(i + 1)
	}
	state.SetSendPayload(cmd)
	defer state.SetSendPayload(nil)

	p := ensureSendFrame()
	if len(p) != SPIPayloadSize {
		t.Fatalf("len = %d, want %d", len(p), SPIPayloadSize)
	}
	if p[17] != 18 || p[18] != 0x00 {
		t.Errorf("payload tail = % x, want 12 00", p[17:])
	}
	tx := wrapSPIFrame(p)
	if len(tx) != SPIFrameSize || tx[0] != SPIFrameHeader || tx[SPIFrameSize-1] != SPIFrameFooter {
		t.Errorf("tx frame = % x", tx)
	}
}
