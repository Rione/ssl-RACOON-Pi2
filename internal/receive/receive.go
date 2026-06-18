package receive

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/Rione/ssl-RACOON-Pi2/internal/state"
	"github.com/Rione/ssl-RACOON-Pi2/internal/util"
	"github.com/Rione/ssl-RACOON-Pi2/proto/pb_gen"
	"google.golang.org/protobuf/proto"
)

const directKickThreshold float32 = 100

var playBallDetectedSound func()

func SetPlayBallDetectedSound(fn func()) {
	playBallDetectedSound = fn
}

func RunClient(done <-chan struct{}, myID uint32, ip string) {
	serverAddr := &net.UDPAddr{
		IP:   net.ParseIP(ip),
		Port: state.UDPRecvPort,
	}

	serverConn, err := net.ListenUDP("udp", serverAddr)
	util.CheckError(err)
	defer serverConn.Close()

	buf := make([]byte, 1024)

	for {
		select {
		case <-done:
			return
		default:
			n, _, _ := serverConn.ReadFromUDP(buf)
			state.LastRecvTime = time.Now()

			packet := &pb_gen.GrSim_Packet{}
			if err := proto.Unmarshal(buf[0:n], packet); err != nil {
				log.Fatal("Error: ", err)
			}

			processRobotCommands(packet, myID)
		}
	}
}

func processRobotCommands(packet *pb_gen.GrSim_Packet, myID uint32) {
	robotCmds := packet.Commands.GetRobotCommands()

	if state.DebugReceive {
		log.Printf("[AI RX] Received packet with %d robot commands", len(robotCmds))
	}

	for _, cmd := range robotCmds {
		if cmd.GetId() != myID {
			continue
		}

		if state.DebugReceive {
			logReceivedCommand(cmd)
		}

		processCommand(cmd)
	}
}

func logReceivedCommand(cmd *pb_gen.GrSim_Robot_Command) {
	log.Printf("[AI RX] === Robot ID: %d (Match) ===", cmd.GetId())
	log.Printf("[AI RX] VelTangent: %.3f m/s, VelNormal: %.3f m/s, VelAngular: %.3f rad/s",
		cmd.GetVeltangent(), cmd.GetVelnormal(), cmd.GetVelangular())
	log.Printf("[AI RX] KickSpeedX: %.1f, KickSpeedZ: %.1f, Spinner: %t, Wheel1(DribblePower): %.1f",
		cmd.GetKickspeedx(), cmd.GetKickspeedz(), cmd.GetSpinner(), cmd.GetWheel1())
	fmt.Println("---")
}

func processCommand(cmd *pb_gen.GrSim_Robot_Command) {
	kickSpeedX := cmd.GetKickspeedx()
	kickSpeedZ := cmd.GetKickspeedz()

	if kickSpeedX >= directKickThreshold {
		state.DoDirectKick = true
		kickSpeedX -= directKickThreshold
	}
	if kickSpeedZ >= directKickThreshold {
		state.DoDirectChipKick = true
		kickSpeedZ -= directKickThreshold
	}

	velTangent := float64(cmd.GetVeltangent())
	velNormal := float64(cmd.GetVelnormal())
	velAngular := float64(cmd.GetVelangular())
	spinner := cmd.GetSpinner()

	if kickSpeedX > 0 || kickSpeedZ > 0 {
		log.Printf("ID: %d, KickX: %.2f, KickZ: %.2f, VelT: %.2f, VelN: %.2f, VelA: %.2f, Spinner: %t",
			cmd.GetId(), cmd.GetKickspeedx(), cmd.GetKickspeedz(), velTangent, velNormal, velAngular, spinner)
	}

	payload := state.SendPayload{
		VelX:   int16(velTangent * 1000),
		VelY:   int16(velNormal * 1000),
		VelAng: int16(velAngular * 1000),
	}

	if spinner {
		spinnerVel := cmd.GetWheel1()
		if spinnerVel > 100 {
			spinnerVel = 100
		} else if spinnerVel < 0 {
			spinnerVel = 0
		}
		payload.DribblePower = uint8(spinnerVel)
	}

	if kickSpeedX > 0 {
		state.KickerVal = uint8(kickSpeedX * 10)
		state.KickerEnable = true
	}
	if state.KickerEnable {
		payload.KickPower = state.KickerVal
	}

	if kickSpeedZ > 0 {
		state.ChipVal = uint8(kickSpeedZ * 10)
		state.ChipEnable = true
	}
	if state.ChipEnable {
		payload.ChipPower = state.ChipVal
	}

	payload.Informations &= ^uint8(state.InfoEmgStop)
	if state.DoDirectKick {
		payload.Informations |= state.InfoDirectKick
	}
	if state.DoDirectChipKick {
		payload.Informations |= state.InfoDirectChip
	}
	payload.Informations |= state.InfoDoCharge

	state.SendArray = bytes.Buffer{}
	if err := binary.Write(&state.SendArray, binary.LittleEndian, payload); err != nil {
		log.Fatal(err)
	}
}

func ReceiveData(done <-chan struct{}, myID uint32, ip string) {
	serverAddr := &net.UDPAddr{
		IP:   net.ParseIP(ip),
		Port: state.UDPCameraPort,
	}

	serverConn, err := net.ListenUDP("udp", serverAddr)
	util.CheckError(err)
	defer serverConn.Close()

	buf := make([]byte, 20240)

	for {
		select {
		case <-done:
			return
		default:
			n, _, _ := serverConn.ReadFromUDP(buf)

			jsonData := &state.ImageData{}
			if err := json.Unmarshal(buf[0:n], jsonData); err != nil {
				log.Printf("JSON unmarshal error: %v", err)
				continue
			}

			state.ImageDataPtr = jsonData
			state.ImageResponseData.Frame = jsonData.Frame

			if jsonData.IsBallExit && !state.PrevBallDetected {
				if playBallDetectedSound != nil {
					go playBallDetectedSound()
				}
			}
			state.PrevBallDetected = jsonData.IsBallExit
		}
	}
}
