package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/Rione/ssl-RACOON-Pi2/proto/pb_gen"
	"google.golang.org/protobuf/proto"
)

// ダイレクトキック判定用のしきい値
const directKickThreshold float32 = 100

// ハンドシェイク中の状態管理変数
const (
	StateDiscovering = 0
	StateOffered     = 1
	StateConnected   = 2
)

var (
	ConnectionState int = StateDiscovering
	PcAddress       *net.UDPAddr // 接続先PCのIPアドレスを記憶
)

// RunClient はAIからの制御コマンドを受信するUDPクライアントである
func RunClient(done <-chan struct{}, myID uint32, ip string) {
	serverAddr := &net.UDPAddr{
		IP:   net.ParseIP(ip),
		Port: UDP_RECV_PORT,
	}

	serverConn, err := net.ListenUDP("udp", serverAddr)
	CheckError(err)
	defer serverConn.Close()

	buf := make([]byte, 1024)
	log.Printf("[AI RX] Started listening for PC on port %d...", UDP_RECV_PORT)

	for {
		select {
		case <-done:
			return
		default:
			n, addr, _ := serverConn.ReadFromUDP(buf)
			if err != nil {
				log.Printf("[AI RX] Error reading UDP message: %v", err)
				continue
			}

			// 先頭1バイトのヘッダを解析
			header := buf[0]
			robotId := uint32((header >> 4) & 0x0F) // 上位4bit: ロボットID
			cmdId := header & 0x0F                  // 下位4bit: コマンドID

			// 自分宛てのパケットでなければ無視
			if robotId != myID {
				continue
			}

			switch cmdId {
			case 0x02: // OFFER
				PcAddress = addr // PCのIPアドレスを記憶
				ConnectionState = StateOffered
				log.Printf("[AI RX] Received OFFER from %s. State -> OFFERED", addr.IP.String())

			case 0x04: // OK_PC
				if ConnectionState == StateOffered {
					ConnectionState = StateConnected
					lastRecvTime = time.Now()
					log.Printf("[AI RX] Received OK_PC from %s. State -> CONNECTED", addr.IP.String())
				}

			case 0x06: // DATA (BotCmd)
				// OK_PC取りこぼし->DATAの場合CONNECTEDに強制昇格
				if ConnectionState != StateConnected {
					ConnectionState = StateConnected
					PcAddress = addr
					log.Printf("[AI RX] Received DATA before OK_PC. Implicit upgrade to CONNECTED")
				}
				
				lastRecvTime = time.Now() // DATA受信で寿命タイマーリセット

				if n > 1 {
					packet := &pb_gen.GrSim_Packet{}
					// 先頭1バイトを削って、残りをProtobufとしてパース
					if err := proto.Unmarshal(buf[1:n], packet); err != nil {
						log.Printf("Error unmarshaling DATA: %v", err)
						continue
					}
					processRobotCommands(packet, myID)
				}

			case 0x07: // KEEP_ALIVE
				// KEEP_ALIVEも生存確認なので、未接続ならCONNECTEDに昇格
				if ConnectionState != StateConnected {
					ConnectionState = StateConnected
					PcAddress = addr
					log.Printf("[AI RX] Received KEEP_ALIVE before OK_PC. Implicit upgrade to CONNECTED")
				}
				
				lastRecvTime = time.Now() // KEEP_ALIVE受信でも寿命リセット
			}
		}
	}
}

// processRobotCommands はロボットコマンドを処理する
func processRobotCommands(packet *pb_gen.GrSim_Packet, myID uint32) {
	robotCmds := packet.Commands.GetRobotCommands()

	if debugReceive {
		log.Printf("[AI RX] Received packet with %d robot commands", len(robotCmds))
	}

	for _, cmd := range robotCmds {
		if cmd.GetId() != myID {
			continue
		}

		if debugReceive {
			logReceivedCommand(cmd)
		}

		processCommand(cmd)
	}
}

// logReceivedCommand はデバッグ用にコマンドをログ出力する
func logReceivedCommand(cmd *pb_gen.GrSim_Robot_Command) {
	log.Printf("[AI RX] === Robot ID: %d (Match) ===", cmd.GetId())
	log.Printf("[AI RX] VelTangent: %.3f m/s, VelNormal: %.3f m/s, VelAngular: %.3f rad/s",
		cmd.GetVeltangent(), cmd.GetVelnormal(), cmd.GetVelangular())
	log.Printf("[AI RX] KickSpeedX: %.1f, KickSpeedZ: %.1f, Spinner: %t, Wheel1(DribblePower): %.1f",
		cmd.GetKickspeedx(), cmd.GetKickspeedz(), cmd.GetSpinner(), cmd.GetWheel1())
	fmt.Println("---")
}

// processCommand はロボットコマンドを処理し、送信データを構築する
func processCommand(cmd *pb_gen.GrSim_Robot_Command) {
	kickSpeedX := cmd.GetKickspeedx()
	kickSpeedZ := cmd.GetKickspeedz()

	// ダイレクトキック判定（100以上の場合）
	if kickSpeedX >= directKickThreshold {
		doDirectKick = true
		kickSpeedX -= directKickThreshold
	}
	if kickSpeedZ >= directKickThreshold {
		doDirectChipKick = true
		kickSpeedZ -= directKickThreshold
	}

	velTangent := float64(cmd.GetVeltangent())
	velNormal := float64(cmd.GetVelnormal())
	velAngular := float64(cmd.GetVelangular())
	spinner := cmd.GetSpinner()

	// キック情報をログ出力
	if kickSpeedX > 0 || kickSpeedZ > 0 {
		log.Printf("ID: %d, KickX: %.2f, KickZ: %.2f, VelT: %.2f, VelN: %.2f, VelA: %.2f, Spinner: %t",
			cmd.GetId(), cmd.GetKickspeedx(), cmd.GetKickspeedz(), velTangent, velNormal, velAngular, spinner)
	}

	// 送信データを構築
	bytearray := SendStruct{
		preamble: 0xFF,
		velx:     int16(velTangent * 1000), // m/s → mm/s
		vely:     int16(velNormal * 1000),  // m/s → mm/s
		velang:   int16(velAngular * 1000), // rad/s → mrad/s
	}

	// ドリブラー設定
	if spinner {
		spinnerVel := cmd.GetWheel1()
		if spinnerVel > 100 {
			spinnerVel = 100
		} else if spinnerVel < 0 {
			spinnerVel = 0
		}
		bytearray.dribblePower = uint8(spinnerVel)
	}

	// キッカー設定
	if kickSpeedX > 0 {
		kickerVal = uint8(kickSpeedX * 10)
		kickerEnable = true
	}
	if kickerEnable {
		bytearray.kickPower = kickerVal
	}

	// チップキック設定
	if kickSpeedZ > 0 {
		chipVal = uint8(kickSpeedZ * 10)
		chipEnable = true
	}
	if chipEnable {
		bytearray.chipPower = chipVal
	}

	// informationsビットフラグを設定
	bytearray.informations &= ^uint8(INFO_EMG_STOP) // 緊急停止OFF
	if doDirectKick {
		bytearray.informations |= INFO_DIRECT_KICK
	}
	if doDirectChipKick {
		bytearray.informations |= INFO_DIRECT_CHIP
	}
	bytearray.informations |= INFO_DO_CHARGE // 充電ON

	// バイナリに変換
	sendarray = bytes.Buffer{}
	if err := binary.Write(&sendarray, binary.LittleEndian, bytearray); err != nil {
		log.Fatal(err)
	}
}

// ReceiveData はカメラからの画像データを受信する
func ReceiveData(done <-chan struct{}, myID uint32, ip string) {
	serverAddr := &net.UDPAddr{
		IP:   net.ParseIP(ip),
		Port: UDP_CAMERA_PORT,
	}

	serverConn, err := net.ListenUDP("udp", serverAddr)
	CheckError(err)
	defer serverConn.Close()

	buf := make([]byte, 20240)

	for {
		select {
		case <-done:
			return
		default:
			n, _, _ := serverConn.ReadFromUDP(buf)

			jsonData := &ImageData{}
			if err := json.Unmarshal(buf[0:n], jsonData); err != nil {
				log.Printf("JSON unmarshal error: %v", err)
				continue
			}

			imageData = jsonData
			imageResponse.Frame = jsonData.Frame

			// ボールを新しく検出した（前回は検出していなかった）場合のみ、音再生ループを開始
			if jsonData.IsBallExit && !prevBallDetected {
				go playBallDetectedSound()
			}
			prevBallDetected = jsonData.IsBallExit //時間放置による音の重なり阻止よりもタイミングが正確
		}
	}
}
