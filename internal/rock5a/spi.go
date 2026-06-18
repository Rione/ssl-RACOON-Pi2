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
	state.LastRecvTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

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

	state.Recvdata.Volt = rx[0]
	state.Recvdata.SensorInformation = rx[1]
	state.Recvdata.CapPower = rx[2]
	state.Recvdata.Reserved = rx[3]

	if state.DebugSerial {
		if frameErr != nil {
			log.Printf("[SPI RX] FRAME ERROR: %v", frameErr)
		}
		log.Printf("[SPI RX] Volt: %d (%.1fV), SensorInfo: 0b%08b, CapPower: %d, Reserved: %d",
			state.Recvdata.Volt, float32(state.Recvdata.Volt)*0.1, state.Recvdata.SensorInformation, state.Recvdata.CapPower, state.Recvdata.Reserved)
		log.Printf("[SPI RX] raw (%dB): % x", SPIFrameSize, rx)
		link.LogSendData(tx)
	}

	link.CheckBatteryStatus()
	link.FinishLinkCycle()
	prevSPIFrameValid = isSPIFrameValid
}

func validateRecvFrame(rx []byte) error {
	if len(rx) < SPIFrameSize {
		return fmt.Errorf("short frame: got %d bytes, want %d", len(rx), SPIFrameSize)
	}
	for i := 0; i < SPIRecvSize; i++ {
		if rx[i] != SPIExpectedRecvPayload[i] {
			return fmt.Errorf("payload[%d]: expected %02x, got %02x (want % x, got % x)",
				i, SPIExpectedRecvPayload[i], rx[i], SPIExpectedRecvPayload, rx[:SPIRecvSize])
		}
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
