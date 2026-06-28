//go:build rock5a

package rock5a

import "github.com/Rione/ssl-RACOON-Pi2/internal/state"

func ensureSendFrame() []byte {
	b := state.GetSendPayload()
	if len(b) < SPIFrameSize {
		frame := make([]byte, SPIFrameSize)
		frame[17] = state.InfoEmgStop
		return frame
	}
	if len(b) > SPIFrameSize {
		return b[:SPIFrameSize]
	}
	return b
}
