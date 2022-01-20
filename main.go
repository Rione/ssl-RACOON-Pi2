package main

import (
	"flag"
	"github.com/Rione-SSL/RACOON-Pi/proto/pb_gen"
	"github.com/golang/protobuf/proto"
	"log"
	"net"
)

var (
	mode = flag.String("m", "server", "mode: client or server")
	port = flag.String("p", "20011", "host: ip:port")
)

func main() {
	flag.Parse()

	switch *mode {
	case "server":
		RunServer()
	}
}

func RunServer() {
	var MyId uint32 = 0

	serverAddr := &net.UDPAddr{
		IP:   net.ParseIP("224.5.23.2"),
		Port: 20011,
	}

	serverConn, err := net.ListenMulticastUDP("udp", nil, serverAddr)
	CheckError(err)
	defer serverConn.Close()

	buf := make([]byte, 1024)

	log.Println("Listening on port " + *port)
	for {
		n, addr, err := serverConn.ReadFromUDP(buf)
		packet := &pb_gen.GrSim_Packet{}
		err = proto.Unmarshal(buf[0:n], packet)
		log.Printf("Data received from %s", addr)

		robotcmd := packet.Commands.GetRobotCommands()

		for _, v := range robotcmd {
			if v.GetId() == MyId {
				Id := v.GetId()
				Kickspeedx := v.GetKickspeedx()
				Kickspeedz := v.GetKickspeedz()
				Veltangent := v.GetVeltangent()
				Velnormal := v.GetVelnormal()
				Velangular := v.GetVelangular()
				Spinner := v.GetSpinner()
				log.Printf("ID        : %d", Id)
				log.Printf("Kickspeedx: %f", Kickspeedx)
				log.Printf("Kickspeedz: %f", Kickspeedz)
				log.Printf("Veltangent: %f", Veltangent)
				log.Printf("Velnormal : %f", Velnormal)
				log.Printf("Velangular: %f", Velangular)
				log.Printf("Spinner   : %t", Spinner)

			}
		}

		if err != nil {
			log.Fatal("Error: ", err)
		}
		log.Println("======================================")
	}
}

func CheckError(err error) {
	if err != nil {
		log.Fatal("Error: ", err)
	}
}
