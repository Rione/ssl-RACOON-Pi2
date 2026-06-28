//go:build rock5a

package rock5a

import (
	"fmt"

	"github.com/Rione/ssl-RACOON-Pi2/internal/state"
)

func ensureSendFrame() []byte {
	b := state.GetSendPayload()
	if len(b) < SPIPayloadSize {
		frame := make([]byte, SPIPayloadSize)
		frame[17] = state.InfoEmgStop
		return frame
	}
	if len(b) > SPIPayloadSize {
		return b[:SPIPayloadSize]
	}
	return b
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

func validateSPIFrame(rx []byte) error {
	if len(rx) < SPIFrameSize {
		return fmt.Errorf("short frame: got %d bytes, want %d", len(rx), SPIFrameSize)
	}
	if rx[0] != SPIFrameHeader {
		return fmt.Errorf("header: expected %02x, got %02x", SPIFrameHeader, rx[0])
	}
	if rx[SPIFrameSize-1] != SPIFrameFooter {
		return fmt.Errorf("footer: expected %02x, got %02x", SPIFrameFooter, rx[SPIFrameSize-1])
	}
	for i := 1 + SPIRecvSize; i < SPIFrameSize-1; i++ {
		if rx[i] != 0 {
			return fmt.Errorf("padding[%d]: expected 00, got %02x", i, rx[i])
		}
	}
	return nil
}
