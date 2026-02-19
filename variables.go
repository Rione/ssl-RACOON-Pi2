package main

import (
	"bytes"
	"time"
)

// シリアル通信設定
const (
	BAUDRATE         int    = 230400       // ボーレート
	SERIAL_PORT_NAME string = "/dev/ttyS2" // Rock5A UART2（物理ピン8/10）
)

// バッテリー電圧しきい値（10倍値で格納：150 = 15.0V）
const (
	BATTERY_LOW_THRESHOLD      int = 140 // 低電圧警告しきい値
	BATTERY_CRITICAL_THRESHOLD int = 135 // 危険電圧しきい値
)

// GPIOピン定義（Rock5A 40ピンヘッダー）
// warthog618/go-gpiocdev 用: gpiochip 名と line offset を指定
//
// ピン対応表:
//
//	物理ピン  RPi BCM   Rock5A GPIO   Rock5A名       gpiochip  offset
//	Pin 12    GPIO18    129           GPIO4_A1 (LED1)  gpiochip4  1
//	Pin 13    GPIO27    138           GPIO4_B2 (LED2)  gpiochip4  10
//	Pin 11    GPIO13    --            GPIO4_B3 (ブザー: PWM)
//	Pin 15    GPIO22    140           GPIO4_B4 (ボタン1) gpiochip4 12
//	Pin 18    GPIO24    40            GPIO1_B0 (ボタン2) gpiochip1 8
//	Pin 7     GPIO4     43            GPIO1_B3 (DIP1)   gpiochip1 11
//	Pin 29    GPIO5     42            GPIO1_B2 (DIP2)   gpiochip1 10
//	Pin 31    GPIO6     41            GPIO1_B1 (DIP3)   gpiochip1 9
//	Pin 22    GPIO25    45            GPIO1_B5 (DIP4)   gpiochip1 13
//
// rock5a-gpio-go 用: bank (0-4), port (A=0, B=1, C=2, D=3), pin (0-7)
// OpenGPIO(bank, port, pin) / SetPull(bank, 'A'+port, pin, mode) で使用
const (
	// LED（出力）
	PIN_LED1_BANK = 4
	PIN_LED1_PORT = 0 // A
	PIN_LED1_PIN  = 1
	PIN_LED2_BANK = 4
	PIN_LED2_PORT = 1 // B
	PIN_LED2_PIN  = 2

	// ボタン（入力・プルアップなし・アクティブロー。押下時GNDで論理1）
	PIN_BUTTON1_BANK = 4
	PIN_BUTTON1_PORT = 1
	PIN_BUTTON1_PIN  = 4
	PIN_BUTTON2_BANK = 1
	PIN_BUTTON2_PORT = 1
	PIN_BUTTON2_PIN  = 0

	// DIPスイッチ（入力・プルアップなし・アクティブロー。ONでGND→論理1）
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

// PWM設定（/sys/class/pwm 経由でブザー制御）
// Rock5A Pin11 = GPIO4_B3 = PWM15_IR_M1
// デバイスツリーオーバーレイ rk3588-pwm15-m1 の有効化が必要
const (
	PWM_CHIP_PATH = "/sys/class/pwm/pwmchip1" // pwmchipのsysfsパス
	PWM_CHANNEL   = 0                         // PWMチャンネル番号
)

// ネットワーク設定
const (
	PORT            string = ":9191"      // APIサーバーポート
	UDP_RECV_PORT   int    = 20011        // AIからの受信ポート
	UDP_CAMERA_PORT int    = 31133        // カメラデータ受信ポート
	MULTICAST_ADDR  string = "224.5.69.4" // マルチキャストアドレス
	MULTICAST_PORT  string = "16941"      // マルチキャスト送信ポート
)

// タイミング設定
const (
	KICK_HOLD_DURATION  = 500 * time.Millisecond // キック値保持時間
	NO_RECV_TIMEOUT     = 1 * time.Second        // 受信タイムアウト
	CHARGE_STOP_TIMEOUT = 15 * time.Second       // 充電停止までのタイムアウト
)

var sendarray bytes.Buffer // 送信用バッファ

// RecvStruct はシリアル通信での受信データ構造体である
// SensorInformationのビット構成:
//   - bit[0-4]: Reserved
//   - bit[5]: IsNewDribbler
//   - bit[6]: IsDetectDribbler
//   - bit[7]: IsDetectPhotoSensor
type RecvStruct struct {
	Volt              uint8
	SensorInformation uint8
	CapPower          uint8
}

// センサー情報のビットマスク
const (
	SENSOR_PHOTO_MASK    = 0b00000001 // フォトセンサー検出ビット
	SENSOR_DRIBBLER_MASK = 0b00000010 // ドリブラーセンサー検出ビット
	SENSOR_NEW_DRIB_MASK = 0b00000100 // 新型ドリブラービット
)

var isControlByRobotMode = false // ロボット制御モードのフラグ

// SendStruct はシリアル通信での送信データ構造体である
// informationsのビット構成:
//   - bit[0]: emgStop（緊急停止）
//   - bit[1]: doDirectKick（ダイレクトキック）
//   - bit[2]: doDirectChip（ダイレクトチップ）
//   - bit[3]: Reserved
//   - bit[4]: doCharge（コンデンサ充電）
//   - bit[5]: isSignalReceived（信号受信フラグ）
//   - bit[6]: isCtrlByRobot（ロボット制御モード）
//   - bit[7]: parity（パリティビット）
type SendStruct struct {
	preamble      byte
	velx          int16
	vely          int16
	velang        int16
	dribblePower  uint8
	kickPower     uint8
	chipPower     uint8
	relativeX     int16 // 相対位置X (mm)
	relativeY     int16 // 相対位置Y (mm)
	relativeTheta int16 // 相対角度 (mrad)
	cameraBallX   uint8 // カメラのボールの左右情報 (0~255、元は0~639px)
	cameraBallY   uint8 // カメラからのボールまでの距離 (mm)
	informations  uint8
}

// informationsビットフラグ
const (
	INFO_EMG_STOP        = 0b00000001 // 緊急停止
	INFO_DIRECT_KICK     = 0b00000010 // ダイレクトキック
	INFO_DIRECT_CHIP     = 0b00000100 // ダイレクトチップ
	INFO_DO_CHARGE       = 0b00010000 // コンデンサ充電
	INFO_SIGNAL_RECEIVED = 0b00100000 // 信号受信中
	INFO_CTRL_BY_ROBOT   = 0b01000000 // ロボット制御モード
)

// グローバル状態変数
var (
	recvdata     RecvStruct              // 受信データ
	lastRecvTime time.Time  = time.Now() // 最終受信時刻
	imuError     bool       = false      // IMU速度超過フラグ
)

// エラー状態
var (
	isRobotError      = false
	RobotErrorCode    = 0
	RobotErrorMessage = ""
)

// アラーム制御
var alarmIgnore = false

// キッカー制御状態
var (
	kickerEnable     bool  = false // ストレートキック有効フラグ
	kickerVal        uint8 = 0     // ストレートキック強度
	chipEnable       bool  = false // チップキック有効フラグ
	chipVal          uint8 = 0     // チップキック強度
	doDirectChipKick bool  = false // ダイレクトチップキックフラグ
	doDirectKick     bool  = false // ダイレクトキックフラグ
)

// ImageData はカメラからの画像データ構造体である
type ImageData struct {
	IsBallExit bool    `json:"isball"`
	ImageX     float32 `json:"x"`
	ImageY     float32 `json:"y"`
	Frame      string  `json:"frame"`
}

var imageData ImageData

// ImageResponse はAPIレスポンス用の画像データ構造体である
type ImageResponse struct {
	Frame string `json:"frame"`
}

var imageResponse ImageResponse

// デバッグモードフラグ
var (
	debugSerial  bool = false // -ds: シリアル送受信のモニタリング
	debugReceive bool = false // -dr: AIからの受信結果表示
)

// Adjustment はボール検出のしきい値設定構造体である
type Adjustment struct {
	MinThreshold         string  `json:"minThreshold"`
	MaxThreshold         string  `json:"maxThreshold"`
	BallDetectRadius     int     `json:"ballDetectRadius"`
	CircularityThreshold float32 `json:"circularityThreshold"`
}

// デフォルトのしきい値設定
var defaultAdjustment = Adjustment{
	MinThreshold:         "1, 120, 100",
	MaxThreshold:         "15, 255, 255",
	BallDetectRadius:     150,
	CircularityThreshold: 0.2,
}
