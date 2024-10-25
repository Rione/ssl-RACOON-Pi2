package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"

	picturepb "github.com/Rione/ssl-RACOON-Pi2/proto/pb_gen"

	"gocv.io/x/gocv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	scanner *bufio.Scanner
	client  picturepb.ImageServiceClient
	window  *gocv.Window
	webcam  *gocv.VideoCapture
	mutex   = &sync.Mutex{}
)

func ClientStream() {
	runtime.LockOSThread()
	var err error
	webcam, err = gocv.OpenVideoCapture(0)
	if err != nil {
		log.Fatalf("error while opening video capture: %v", err)
	}
	stream, err := client.ClientStream(context.Background())
	if err != nil {
		log.Fatalf("error while creating stream: %v", err)
	}
	defer webcam.Close()

	fmt.Println("Camera initialized successfully.")

	img := gocv.NewMat()
	defer img.Close()

	for {
		ok := webcam.Read(&img)
		if !ok {
			log.Fatalf("cannot read device %v\n", 0)
		}

		params := []int{gocv.IMWriteJpegQuality, 25}
		buf, err := gocv.IMEncodeWithParams(".jpg", img, params)
		if err != nil {
			log.Fatalf("failed to encode image: %v", err)
		}

		if err := stream.Send(&picturepb.ImageRequest{
			Image: buf.GetBytes(),
			Id:    1,
		}); err != nil {
			log.Fatalf("failed to send image: %v", err)
			break
		}
	}
}

func Streaming(chstreaming chan bool) {
	fmt.Println("start gRPC Client.")

	// 1. 標準入力から文字列を受け取るスキャナを用意
	scanner = bufio.NewScanner(os.Stdin)

	// 2. gRPCサーバーとのコネクションを確立
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

	// 3. gRPCクライアントを生成
	client = picturepb.NewImageServiceClient(conn)

	for {
		ClientStream()
	}
}
