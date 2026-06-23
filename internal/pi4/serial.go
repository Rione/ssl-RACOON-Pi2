//go:build pi4

package pi4

import (
	"log"
	"time"

	"github.com/Rione/ssl-RACOON-Pi2/internal/link"
	"github.com/Rione/ssl-RACOON-Pi2/internal/state"
	"go.bug.st/serial"
)

const recvPacketSize = 3

var serialPreamble = []byte{0xFF, 0x00, 0xFF, 0x00}

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

	past := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	state.LastRecvTime.Store(past)
	state.LastCmdRecvTime.Store(past)

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

	if state.DebugSerial {
		log.Printf("[Serial RX] Volt: %d (%.1fV), SensorInfo: 0b%08b, CapPower: %d",
			state.Recvdata.Volt, float32(state.Recvdata.Volt)*0.1, state.Recvdata.SensorInformation, state.Recvdata.CapPower)
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
