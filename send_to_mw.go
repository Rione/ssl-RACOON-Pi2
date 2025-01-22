package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"

	// "net"

	picturepb "github.com/Rione/ssl-RACOON-Pi2/proto/pb_gen"

	"gobot.io/x/gobot"                  //required to play picamera
	"gobot.io/x/gobot/platforms/opencv" //required to play picamera
	"gocv.io/x/gocv"                    //required to play picamera
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	// "google.golang.org/protobuf/proto"
)

var (
	scanner *bufio.Scanner
	client  picturepb.ImageServiceClient
	window  *gocv.Window
	webcam  *gocv.VideoCapture
	mutex          = &sync.Mutex{}
	id      uint32 = 1
)

func ClientStream(id uint32) {
	// ipv4 := "127.0.0.1"
	// port := "9999"
	// address := ipv4 + ":" + port

	// fmt.Println("Sender:", address)
	// conn, _:= net.Dial("udp", address)
	// // CheckError(err)
	// defer conn.Close()
	runtime.LockOSThread()
	window := opencv.NewWindowDriver()
	camera := opencv.NewCameraDriver(0)

	stream, err := client.ClientStream(context.Background())
	if err != nil {
		log.Fatalf("error while creating stream: %v", err)
	}

	work := func() {
		camera.On(opencv.Frame, func(data interface{}) {
			img := data.(gocv.Mat)
			params := []int{gocv.IMWriteJpegQuality, 25}
			buf, err := gocv.IMEncodeWithParams(".jpg", img, params)
			if err != nil {
				log.Fatalf("failed to encode image: %v", err)
			}
			if err := stream.Send(&picturepb.ImageRequest{
				Image: buf.GetBytes(),
				Id:    int32(id),
			}); err != nil {
				log.Fatalf("failed to send image: %v", err)
			}

			// pe := &picturepb.ImageRequest{
			// 	Image: buf.GetBytes(),
			// }
			// Data, _ := proto.Marshal(pe)

			// conn.Write([]byte(Data))

			// conn.Write([]byte(Data))

			// window.ShowImage(img)
			window.WaitKey(1)
		})
	}

	robot := gobot.NewRobot("cameraBot",
		[]gobot.Device{window, camera},
		work,
	)

	robot.Start()
}

// func ClientStream(id uint32) {
// 	// ipv4 := "127.0.0.1"
// 	// port := "9999"
// 	// address := ipv4 + ":" + port

// 	// fmt.Println("Sender:", address)
// 	// conn, _:= net.Dial("udp", address)
// 	// // CheckError(err)
// 	// defer conn.Close()

// 	runtime.LockOSThread()
// 	var err error
// 	webcam, err = gocv.OpenVideoCapture(0)
// 	if err != nil {
// 		log.Fatalf("error while opening video capture: %v", err)
// 	}
// 	stream, err := client.ClientStream(context.Background())
// 	if err != nil {
// 		log.Fatalf("error while creating stream: %v", err)
// 	}
// 	defer webcam.Close()

// 	fmt.Println("Camera initialized successfully.")

// 	img := gocv.NewMat()
// 	defer img.Close()

// 	for {
// 		ok := webcam.Read(&img)
// 		if !ok {
// 			log.Fatalf("cannot read device %v\n", 0)
// 		}

// 		params := []int{gocv.IMWriteJpegQuality, 25}
// 		buf, err := gocv.IMEncodeWithParams(".jpg", img, params)
// 		if err != nil {
// 			log.Fatalf("failed to encode image: %v", err)
// 		}

// 		if err := stream.Send(&picturepb.ImageRequest{
// 			Image: buf.GetBytes(),
// 			Id:    1,
// 		}); err != nil {
// 			log.Fatalf("failed to send image: %v", err)
// 			break
// 		}

// 		// pe := &picturepb.ImageRequest{
// 		// 	Image: buf.GetBytes(),
// 		// }
// 		// Data, _ := proto.Marshal(pe)

// 		// conn.Write([]byte(Data))

// 	}
// }

func Streaming(chstreaming chan bool, MyID uint32) {
	fmt.Println("start gRPC Client.")

	scanner = bufio.NewScanner(os.Stdin)

	address := "localhost:8081"
	conn, err := grpc.Dial(
		address,

		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Fatal("Connection failed.")
		return
	}
	defer conn.Close()

	client = picturepb.NewImageServiceClient(conn)

	for {
		id = MyID
		ClientStream(id)
	}
}
