//go:build rock5a

package rock5a

import "github.com/Rione/ssl-RACOON-Pi2/internal/state"

func ensureSendFrame() []byte {
	b := state.GetSendPayload()
	if len(b) < 18 {
		b = make([]byte, 19)
		b[17] = state.InfoEmgStop
		return b
	}
	if len(b) == 18 {
		b = append(b, 0x00)
	}
	return b
}
