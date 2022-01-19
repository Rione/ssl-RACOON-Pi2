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
	port = flag.String("p", "20021", "host: ip:port")
)

func main() {
	flag.Parse()

	switch *mode {
	case "server":
		RunServer()
	}
}

func RunServer() {
	serverAddr, err := net.ResolveUDPAddr("udp", ":"+*port)
	CheckError(err)

	serverConn, err := net.ListenUDP("udp", serverAddr)
	CheckError(err)
	defer serverConn.Close()

	buf := make([]byte, 1024)

	log.Println("Listening on port " + *port)
	for {
		n, addr, err := serverConn.ReadFromUDP(buf)
		packet := &pb_gen.GrSim_Packet{}
		err = proto.Unmarshal(buf[0:n], packet)
		log.Printf("Received %d from %s", *packet.Commands, addr)

		robotcmd := &pb_gen.GrSim_Commands{}
		*robotcmd = *packet.Commands.GetRobotCommands()
		if *robotcmd.GetId() == 0 {
			log.Printf("Robot 0 Data Received")
		}
		if err != nil {
			log.Fatal("Error: ", err)
		}
	}
}

func CheckError(err error) {
	if err != nil {
		log.Fatal("Error: ", err)
	}
}
