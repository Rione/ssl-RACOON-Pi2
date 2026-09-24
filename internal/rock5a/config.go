//go:build rock5a

package rock5a

const DefaultHostname = "DietPi\n"

const (
	SPIDevPath = "/dev/spidev4.0"
	SPISpeedHz = 1_000_000
	// [ヘッダ 0xFF][ペイロード 19byte][フッタ 0xAA] = 21byte / トランザクション。
	// MainBoard 側の IMU 対応で 18 → 19byte に拡張された (ssl-RAVEN-Wing SPI_PROTOCOL.md 2026-09)。
	// 受信ペイロードは 19byte 全てを使うため、旧仕様のパディング(0埋め)領域は無い。
	SPIFrameSize   = 21
	SPIPayloadSize = 19
	SPIFrameHeader = 0xFF
	SPIFrameFooter = 0xAA
	// SPIPeriodMs は SPI トランザクションの周期。MainBoard 側 IMU の ODR が
	// 104Hz なので、取りこぼさないよう RAVEN-Wing と同じ 250Hz (4ms) にする。
	// 21byte @ 1MHz は転送だけなら約 0.17ms なので周期に対して十分短い。
	SPIPeriodMs     = 4
	WheelDiameterMm = 60.0

	//電圧の読み取り値が揺れる為、誤ってアラームが鳴らないよう、閾値を低めにしている
	BatteryLowThreshold      = 200 // 6s:20V
	BatteryCriticalThreshold = 190 // 6s:19V
)

const (
	PIN_LED1_BANK = 4
	PIN_LED1_PORT = 0
	PIN_LED1_PIN  = 1
	PIN_LED2_BANK = 4
	PIN_LED2_PORT = 1
	PIN_LED2_PIN  = 2

	PIN_BUTTON1_BANK = 4
	PIN_BUTTON1_PORT = 1
	PIN_BUTTON1_PIN  = 4
	PIN_BUTTON2_BANK = 1
	PIN_BUTTON2_PORT = 1
	PIN_BUTTON2_PIN  = 0

	PIN_DIP1_BANK = 1
	PIN_DIP1_PORT = 1
	PIN_DIP1_PIN  = 3
	PIN_DIP2_BANK = 1
	PIN_DIP2_PORT = 1
	PIN_DIP2_PIN  = 2
	PIN_DIP3_BANK = 1
	PIN_DIP3_PORT = 1
	PIN_DIP3_PIN  = 1
	PIN_DIP4_BANK = 1
	PIN_DIP4_PORT = 1
	PIN_DIP4_PIN  = 5
)

const (
	PWMChipPath = "/sys/class/pwm/pwmchip1"
	PWMChannel  = 0
)
