//go:build rock5a

package rock5a

import (
	"fmt"
	"log"
	"time"

	"github.com/Rione/ssl-RACOON-Pi2/internal/link"
	"github.com/Rione/ssl-RACOON-Pi2/internal/state"
	"github.com/Rione/ssl-RACOON-Pi2/internal/util"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/conn/v3/spi"
	"periph.io/x/conn/v3/spi/spireg"
	"periph.io/x/host/v3"
)

var (
	isSPIFrameValid   bool = true
	prevSPIFrameValid bool = true
)

func RunSPI(done <-chan struct{}, myID uint32) {
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}

	port, err := spireg.Open(SPIDevPath)
	if err != nil {
		log.Fatal(err)
	}
	defer port.Close()

	conn, err := port.Connect(physic.Frequency(SPISpeedHz)*physic.Hertz, spi.Mode0, 8)
	if err != nil {
		log.Fatal(err)
	}

	state.Recvdata = state.RecvData{}
	past := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	state.LastRecvTime.Store(past)
	state.LastCmdRecvTime.Store(past)

	for {
		select {
		case <-done:
			return
		default:
			processSPICommunication(conn)
		}
	}
}

func processSPICommunication(conn spi.Conn) {
	tx := link.PrepareSendData()
	rx := make([]byte, len(tx))

	if err := conn.Tx(tx, rx); err != nil {
		util.CheckError(err)
	}

	frameErr := validateRecvFrame(rx)
	isSPIFrameValid = frameErr == nil
	handleSPIFrameValidationChange(frameErr)

	state.Recvdata = parseRecvBuf(rx)

	state.FlWheelSpeedRadS = motorRawToWheelMS(state.Recvdata.FlWheelSpeed)
	state.BlWheelSpeedRadS = motorRawToWheelMS(state.Recvdata.BlWheelSpeed)
	state.BrWheelSpeedRadS = motorRawToWheelMS(state.Recvdata.BrWheelSpeed)
	state.FrWheelSpeedRadS = motorRawToWheelMS(state.Recvdata.FrWheelSpeed)

	if state.DebugSerial {
		if frameErr != nil {
			log.Printf("[SPI RX] FRAME ERROR: %v", frameErr)
		}
		log.Printf("[SPI RX] Raw: % 02X", rx[:SPIRecvSize])
		log.Printf("[SPI RX] Volt: %d (%.1fV), SensorInfo: 0b%08b, CapPower: %d",
			state.Recvdata.Volt, float32(state.Recvdata.Volt)*0.1, state.Recvdata.SensorInformation, state.Recvdata.CapPower)
		log.Printf("[SPI RX] Wheel(raw) FL: %d, BL: %d, BR: %d, FR: %d",
			state.Recvdata.FlWheelSpeed, state.Recvdata.BlWheelSpeed, state.Recvdata.BrWheelSpeed, state.Recvdata.FrWheelSpeed)
		log.Printf("[SPI RX] Wheel(m/s) FL: %.3f, BL: %.3f, BR: %.3f, FR: %.3f",
			state.FlWheelSpeedRadS, state.BlWheelSpeedRadS, state.BrWheelSpeedRadS, state.FrWheelSpeedRadS)
		log.Printf("[SPI RX] full (%dB): % x", SPIFrameSize, rx)
		link.LogSendData(tx)
	}

	link.CheckBatteryStatus()
	link.FinishLinkCycle()
	prevSPIFrameValid = isSPIFrameValid
}

func parseRecvBuf(rx []byte) state.RecvData {
	return state.RecvData{
		Volt:              rx[0],
		SensorInformation: rx[1],
		CapPower:          rx[2],
		FlWheelSpeed:      int16(rx[3]) | int16(rx[4])<<8,
		BlWheelSpeed:      int16(rx[5]) | int16(rx[6])<<8,
		BrWheelSpeed:      int16(rx[7]) | int16(rx[8])<<8,
		FrWheelSpeed:      int16(rx[9]) | int16(rx[10])<<8,
	}
}

func motorRawToWheelMS(raw int16) float32 {
	wheelRadS := float32(raw) / 100.0
	wheelRadiusM := float32(WheelDiameterMm / 2000.0)
	return wheelRadS * wheelRadiusM
}

func validateRecvFrame(rx []byte) error {
	if len(rx) < SPIFrameSize {
		return fmt.Errorf("short frame: got %d bytes, want %d", len(rx), SPIFrameSize)
	}
	for i := SPIRecvSize; i < SPIFrameSize; i++ {
		if rx[i] != 0 {
			return fmt.Errorf("padding[%d]: expected 00, got %02x", i, rx[i])
		}
	}
	return nil
}

func handleSPIFrameValidationChange(frameErr error) {
	if frameErr != nil && prevSPIFrameValid {
		log.Printf("SPI recv frame mismatch: %v", frameErr)
		link.RingBuzzerAsync(5, 300*time.Millisecond, 0)
	}
	if frameErr == nil && !prevSPIFrameValid {
		log.Println("SPI recv frame recovered")
		link.RingBuzzerAsync(10, 200*time.Millisecond, 0)
	}
}
