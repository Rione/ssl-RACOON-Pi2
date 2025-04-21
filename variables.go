package main

import (
	"bytes"
	"time"
)

// ボーレート
const BAUDRATE int = 230400

// シリアルポート名 ラズパイ4の場合、"/dev/serial0"
const SERIAL_PORT_NAME string = "/dev/serial0"

// バッテリーの低下しきい値。 150 = 15.0V
const BATTERY_LOW_THRESHOULD int = 140
const BATTERY_CRITICAL_THRESHOULD int = 135

var sendarray bytes.Buffer //送信用バッファ

// 受信時の構造体
type RecvStruct struct {
	Volt                uint8
	IsDetectPhotosensor bool
	// IsTouchingDribbler  bool
	CapPower uint8
}

type SendStruct struct {
	preamble      byte
	velx          int16
	vely          int16
	velang        int16
	dribblePower  uint8
	kickPower     uint8
	chipPower     uint8
	relativeX     int16 // (mm)
	relativeY     int16 // (mm)
	relativeTheta int16 // (mrad)
	cameraBallX   uint8
	cameraBallY   uint8
	informations  uint8
	// informations の ビット構成
	// emgStop          bit[0]
	// doDirectKick     bit[1]
	// doDirectChip     bit[2]
	// 〜bit[3] Reserved〜
	// doCharge         bit[4] //コンデンサへの充電を行うかどうか
	// isSignalReceived bit[5] //Controllerから信号を受け取っているかどうか
	// isCtrlByRobot    bit[6] //ロボットからの位置制御を実行するかどうか
	// parity           bit[7] //velx から bit[6] までのパリティビット（偶数なら1）
}

// 受信データ構造体
var recvdata RecvStruct

// imu速度超過時のフラグ
var imuError bool = false

var last_recv_time time.Time = time.Now()

// ポート8080番で待ち受ける。
const PORT string = ":9191"

var isRobotError = false

var RobotErrorCode = 0
var RobotErrorMessage = ""

var doBuzzer = false
var buzzerTone = 0
var buzzerTime time.Duration = 0 * time.Millisecond

var alarmIgnore = false

var kicker_enable bool = false //キッカーの入力のON OFFを定義する
var kicker_val uint8 = 0       //キッカーの値
var chip_enable bool = false   //チップキックの入力のON OFFを定義する
var chip_val uint8 = 0         //チップキックの値

var doDirectChipKick bool = false
var doDirectKick bool = false

type ImageData struct {
	Is_ball_exit bool   `json:"isball"`
	Image_x      uint32 `json:"image_x"`
	Image_y      uint32 `json:"image_y"`
	Frame        string `json:"frame"`
}

var imageData ImageData
