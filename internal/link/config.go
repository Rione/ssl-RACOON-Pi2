package link

type FrameConfig struct {
	IdxVelXLow, IdxVelXHigh   int
	IdxVelYLow, IdxVelYHigh   int
	IdxVelAngLow, IdxVelAngHigh int
	IdxDribble, IdxKick, IdxChip int
	IdxCamBallX, IdxCamBallY, IdxInfo int
	EnsureSendFrame func() []byte
}

var frame FrameConfig

func ConfigureFrame(cfg FrameConfig) {
	frame = cfg
}
