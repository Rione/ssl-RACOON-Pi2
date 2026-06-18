//go:build pi4

package pi4

import (
	"log"
	"time"

	"github.com/Rione/ssl-RACOON-Pi2/internal/link"
	"github.com/Rione/ssl-RACOON-Pi2/internal/state"
	"go.bug.st/serial"
)

const recvPacketSize = 12

var serialPreamble = []byte{0xFF}

func RunSerial(done <-chan struct{}, myID uint32) {
	port, err := serial.Open(SerialPortName, &serial.Mode{})
	if err != nil {
		log.Fatal(err)
	}
	defer port.Close()

	state.Recvdata = state.RecvData{}

	mode := &serial.Mode{
		BaudRate: Baudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	if err := port.SetMode(mode); err != nil {
		log.Fatal(err)
	}

	state.LastRecvTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	for {
		select {
		case <-done:
			return
		default:
			processSerialCommunication(port)
		}
	}
}

func processSerialCommunication(port serial.Port) {
	recvbuf := waitForPreambleAndReceive(port)

	state.Recvdata = parseRecvBuf(recvbuf)

	state.FlWheelSpeedRadS = float32(state.Recvdata.FlWheelSpeed) / 100.0
	state.BlWheelSpeedRadS = float32(state.Recvdata.BlWheelSpeed) / 100.0
	state.BrWheelSpeedRadS = float32(state.Recvdata.BrWheelSpeed) / 100.0
	state.FrWheelSpeedRadS = float32(state.Recvdata.FrWheelSpeed) / 100.0

	if state.DebugSerial {
		log.Printf("[Serial RX] Raw: % 02X", recvbuf)
		log.Printf("[Serial RX] Volt: %d (%.1fV), SensorInfo: 0b%08b, CapPower: %d, Footer: 0x%02X",
			state.Recvdata.Volt, float32(state.Recvdata.Volt)*0.1, state.Recvdata.SensorInformation, state.Recvdata.CapPower, state.Recvdata.Footer)
		log.Printf("[Serial RX] Wheel(raw) FL: %d, BL: %d, BR: %d, FR: %d",
			state.Recvdata.FlWheelSpeed, state.Recvdata.BlWheelSpeed, state.Recvdata.BrWheelSpeed, state.Recvdata.FrWheelSpeed)
		log.Printf("[Serial RX] Wheel(rad/s) FL: %.2f, BL: %.2f, BR: %.2f, FR: %.2f",
			state.FlWheelSpeedRadS, state.BlWheelSpeedRadS, state.BrWheelSpeedRadS, state.FrWheelSpeedRadS)
	}

	link.CheckBatteryStatus()

	sendbytes := link.PrepareSendData()

	if state.DebugSerial {
		link.LogSendData(sendbytes)
	}

	port.Write(sendbytes)
	link.FinishLinkCycle()
}

func parseRecvBuf(recvbuf []byte) state.RecvData {
	return state.RecvData{
		Volt:              recvbuf[0],
		SensorInformation: recvbuf[1],
		CapPower:          recvbuf[2],
		FlWheelSpeed:      int16(recvbuf[3]) | int16(recvbuf[4])<<8,
		BlWheelSpeed:      int16(recvbuf[5]) | int16(recvbuf[6])<<8,
		BrWheelSpeed:      int16(recvbuf[7]) | int16(recvbuf[8])<<8,
		FrWheelSpeed:      int16(recvbuf[9]) | int16(recvbuf[10])<<8,
		Footer:            recvbuf[11],
	}
}

func waitForPreambleAndReceive(port serial.Port) []byte {
	buf := make([]byte, 1)
	recvbuf := make([]byte, recvPacketSize)

	port.ResetInputBuffer()

	preambleIdx := 0
	for preambleIdx < len(serialPreamble) {
		port.Read(buf)
		if buf[0] == serialPreamble[preambleIdx] {
			preambleIdx++
		} else {
			preambleIdx = 0
		}
	}

	for i := 0; i < recvPacketSize; i++ {
		port.Read(buf)
		recvbuf[i] = buf[0]
	}

	return recvbuf
}
