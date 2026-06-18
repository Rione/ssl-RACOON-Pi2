package main

import (
	"bytes"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// シリアル通信設定
const (
	BAUDRATE         int    = 230400         // ボーレート
	SERIAL_PORT_NAME string = "/dev/serial0" // シリアルポート名（ラズパイ4用）
)

// バッテリー電圧しきい値（10倍値で格納：150 = 15.0V）
const (
	BATTERY_LOW_THRESHOLD      int = 140 // 低電圧警告しきい値
	BATTERY_CRITICAL_THRESHOLD int = 135 // 危険電圧しきい値
)

// GPIOピン番号定義
const (
	PIN_LED1    = 18 // LED1
	PIN_LED2    = 27 // LED2
	PIN_BUZZER  = 13 // ブザー（PWM）
	PIN_BUTTON1 = 22 // ボタン1（S2）
	PIN_BUTTON2 = 24 // ボタン2（S3）
	PIN_DIP1    = 4  // DIPスイッチ1
	PIN_DIP2    = 5  // DIPスイッチ2
	PIN_DIP3    = 6  // DIPスイッチ3
	PIN_DIP4    = 25 // DIPスイッチ4
)

// ネットワーク設定
const (
	PORT            string = ":9191"      // APIサーバーポート
	UDP_RECV_PORT   int    = 20011        // AIからの受信ポート
	UDP_CAMERA_PORT int    = 31133        // カメラデータ受信ポート
	MULTICAST_ADDR  string = "224.5.69.4" // マルチキャストアドレス
	PC_RECV_PORT    int    = 16941        // RAVENのロボット情報受信ポート
	MULTICAST_PORT  string = "16941"      // マルチキャスト送信ポート
)

// タイミング設定
const (
	KICK_HOLD_DURATION  = 500 * time.Millisecond // キック値保持時間
	NO_RECV_TIMEOUT     = 1 * time.Second        // 受信タイムアウト
	CHARGE_STOP_TIMEOUT = 15 * time.Second       // 充電停止までのタイムアウト
)

// ハンドシェイク中の状態管理変数
const (
	StateDiscovering = 0
	StateOffered     = 1
	StateConnected   = 2
)

var sendarray bytes.Buffer // 送信用バッファ

// RecvStruct はシリアル通信での受信データ構造体である
// ワイヤ形式（0xFF の直後 12 バイト、パディングなし）:
//   [0] Volt, [1] SensorInformation, [2] CapPower,
//   [3:4] FlWheelSpeed, [5:6] BlWheelSpeed, [7:8] BrWheelSpeed, [9:10] FrWheelSpeed (LE),
//   [11] Footer (0xAA)
// SensorInformationのビット構成:
//   - bit[0-4]: Reserved
//   - bit[5]: IsNewDribbler
//   - bit[6]: IsDetectDribbler
//   - bit[7]: IsDetectPhotoSensor
type RecvStruct struct {
	Volt              uint8
	SensorInformation uint8
	CapPower          uint8
	FlWheelSpeed      int16
	BlWheelSpeed      int16
	BrWheelSpeed      int16
	FrWheelSpeed      int16
	Footer            uint8
}

// ホイール回転数（rad/s）変換後の値
var (
	flWheelSpeedRadS float32
	blWheelSpeedRadS float32
	brWheelSpeedRadS float32
	frWheelSpeedRadS float32
)

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

// atomicTime は time.Time を複数goroutineからロックなしで読み書きするためのラッパ
// 内部は UnixNano(int64) を atomic で保持する。
type atomicTime struct {
	nano atomic.Int64
}

func (a *atomicTime) Store(t time.Time) { a.nano.Store(t.UnixNano()) }

// Since は保持時刻から現在までの経過時間を返す（time.Since 相当）
func (a *atomicTime) Since() time.Duration {
	return time.Duration(time.Now().UnixNano() - a.nano.Load())
}

// グローバル状態変数
var (
	StateMu         sync.Mutex   // 競合を防ぐための鍵（ロック）
	ConnectionState int          = StateDiscovering
	PcAddress       *net.UDPAddr // 接続先PCのIPアドレスを記憶

	recvdata RecvStruct         // 受信データ
	imuError bool       = false // IMU速度超過フラグ

	// lastRecvTime / lastCmdRecvTime は serial / receive / send_to_ai の複数goroutineから
	// （StateMu の内外問わず）読み書きされるため、データ競合を避けて atomic で保持する。
	lastRecvTime    atomicTime // PCからの最終受信時刻(OFFER/OK_PC/DATA/KEEP_ALIVE)。接続生存判定・充電停止用
	lastCmdRecvTime atomicTime // 最後にDATA(0x06)制御コマンドを受信した時刻。KEEP_ALIVEでは更新しない(速度クリアのフェイルセーフ用)
)

func init() {
	// RunSerial 起動時に 2000年へ上書きされる
	now := time.Now()
	lastRecvTime.Store(now)
	lastCmdRecvTime.Store(now)
}

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

var imageData *ImageData
var prevBallDetected bool = false //検知回数をカウントして警告音を制御するための変数

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
