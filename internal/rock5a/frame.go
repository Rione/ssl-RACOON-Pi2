//go:build rock5a

package rock5a

import (
	"fmt"

	"github.com/Rione/ssl-RACOON-Pi2/internal/state"
)

// 受信フレーム (MainBoard → Rock5A) の各フィールドのフレーム内オフセット。
// 0 がヘッダなので、ペイロード先頭は 1。多バイト値はリトルエンディアン。
// 仕様は ssl-RAVEN-Wing の SPI_PROTOCOL.md 2節。
const (
	rxVolt     = 1
	rxSensor   = 2
	rxCapPower = 3
	rxWheel0   = 4 // wheel0(FL) 以降 2byte ずつ wheel1(BL), wheel2(BR), wheel3(FR)
	rxAccelX   = 12
	rxAccelY   = 14
	rxYawRate  = 16
	rxYawRad   = 18
)

// IMU フィールドのスケール係数。実値 = raw / スケール。
const (
	imuScaleAccel   = 1000.0  // g
	imuScaleYawRate = 900.0   // rad/s (±2000dps レンジで int16 が溢れないよう 900)
	imuScaleYaw     = 10000.0 // rad
)

func ensureSendFrame() []byte {
	b := state.GetSendPayload()
	frame := make([]byte, SPIPayloadSize)
	if len(b) == 0 {
		frame[17] = state.InfoEmgStop
		return frame
	}
	// RAVEN からの指令 (state.SendPayload) は 18byte。ペイロード末尾 1byte
	// (フレーム位置 19) は予約領域なので 0x00 のまま送る。
	copy(frame, b)
	return frame
}

func wrapSPIFrame(payload []byte) []byte {
	frame := make([]byte, SPIFrameSize)
	frame[0] = SPIFrameHeader
	n := len(payload)
	if n > SPIPayloadSize {
		n = SPIPayloadSize
	}
	copy(frame[1:], payload[:n])
	frame[SPIFrameSize-1] = SPIFrameFooter
	return frame
}

// validateSPIFrameAt は rx の offset 位置が有効なフレームかを調べる。
// ペイロード 19byte が全て使われるようになったため、判定材料はヘッダとフッタのみ。
func validateSPIFrameAt(rx []byte, offset int) error {
	if offset < 0 || offset+SPIFrameSize > len(rx) {
		return fmt.Errorf("frame out of range at offset %d", offset)
	}
	if rx[offset] != SPIFrameHeader {
		return fmt.Errorf("header: expected %02x, got %02x", SPIFrameHeader, rx[offset])
	}
	if rx[offset+SPIFrameSize-1] != SPIFrameFooter {
		return fmt.Errorf("footer: expected %02x, got %02x", SPIFrameFooter, rx[offset+SPIFrameSize-1])
	}
	return nil
}

func validateSPIFrame(rx []byte) error {
	if len(rx) < SPIFrameSize {
		return fmt.Errorf("short frame: got %d bytes, want %d", len(rx), SPIFrameSize)
	}
	return validateSPIFrameAt(rx, 0)
}

// findSPIFrame はバッファ内の最後 (最新) の有効フレーム位置を返す (見つからなければ -1)
func findSPIFrame(buf []byte) int {
	for i := len(buf) - SPIFrameSize; i >= 0; i-- {
		if validateSPIFrameAt(buf, i) == nil {
			return i
		}
	}
	return -1
}

func pushSPIRxWindow(window, chunk []byte) {
	copy(window, window[SPIFrameSize:])
	copy(window[SPIFrameSize:], chunk[:SPIFrameSize])
}

func int16At(b []byte, i int) int16 {
	return int16(uint16(b[i]) | uint16(b[i+1])<<8)
}

// parseRecvBufAt は frameOffset から始まるフレームを RecvData に展開する。
// 呼び出し前に findSPIFrame で位置が確定していること。
func parseRecvBufAt(rx []byte, frameOffset int) state.RecvData {
	f := rx[frameOffset : frameOffset+SPIFrameSize]
	return state.RecvData{
		Volt:              f[rxVolt],
		SensorInformation: f[rxSensor],
		CapPower:          f[rxCapPower],
		FlWheelSpeed:      int16At(f, rxWheel0+0),
		BlWheelSpeed:      int16At(f, rxWheel0+2),
		BrWheelSpeed:      int16At(f, rxWheel0+4),
		FrWheelSpeed:      int16At(f, rxWheel0+6),
		AccelXRaw:         int16At(f, rxAccelX),
		AccelYRaw:         int16At(f, rxAccelY),
		YawRateRaw:        int16At(f, rxYawRate),
		YawRaw:            int16At(f, rxYawRad),
	}
}

// imuFromRecv は RecvData の IMU 生値を物理量に直す。
func imuFromRecv(r state.RecvData) state.IMUData {
	return state.IMUData{
		AccelX:  float32(r.AccelXRaw) / imuScaleAccel,
		AccelY:  float32(r.AccelYRaw) / imuScaleAccel,
		YawRate: float32(r.YawRateRaw) / imuScaleYawRate,
		Yaw:     float32(r.YawRaw) / imuScaleYaw,
	}
}
