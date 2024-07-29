package main

import (
	pb "Lab1/Services"
	"bufio"
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	masterAddress = "172.28.104.85:50051" // Address of the master node
	clientAddress = "172.28.104.85:11111" // Address of the client server
)

// ClientServer represents the gRPC server running in the client
type ClientServer struct {
	pb.UnimplementedFileServiceServer
}

// Implement the SendNotification service handler
func (c *ClientServer) SendNotification(ctx context.Context, req *pb.SendNotificationRequest) (*pb.SendNotificationResponse, error) {
	// Print the message received in the request
	fmt.Printf("Received notification: %s\n", req.Message)
	return &pb.SendNotificationResponse{}, nil
}

func main() {
	// Start the gRPC server in the client
	go func() {
		lis, err := net.Listen("tcp", clientAddress)
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}
		defer lis.Close()

		grpcServer := grpc.NewServer()
		pb.RegisterFileServiceServer(grpcServer, &ClientServer{})

		log.Printf("Client gRPC server listening on %s", clientAddress)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	// Create a context with metadata including client IP and port
	md := metadata.Pairs("client-ip", "172.28.104.85", "client-port", "11111")
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	// Set up a connection to the master node
	masterConn, err := grpc.Dial(masterAddress, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect to master node: %v", err)
	}
	defer masterConn.Close()
	masterClient := pb.NewFileServiceClient(masterConn)

	// Loop to continuously prompt the user for actions
	for {
		fmt.Print("Enter 'upload', 'download', or 'exit' to exit: ")
		reader := bufio.NewReader(os.Stdin)
		choice, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalf("error reading input: %v", err)
		}

		// Remove newline character from input
		choice = strings.TrimSpace(choice)

		switch strings.ToLower(choice) {
		case "upload":
			// Prompt user for filename to upload
			var fileName string
			fmt.Print("Enter filename to upload (without extension): ")
			fmt.Scanln(&fileName)

			// Read file content from the specified path
			filePath := fmt.Sprintf("../uploading/%s.mp4", fileName)
			fileContent, err := os.ReadFile(filePath)
			if err != nil {
				log.Fatalf("could not read file: %v", err)
			}

			// Request port number from master node for uploading
			handleUploadFileResponse, err := masterClient.HandleUploadFile(ctx, &pb.HandleUploadFileRequest{})
			if err != nil {
				log.Fatalf("could not handle upload file request: %v", err)
			}

			// Connect to data node using the returned port number for uploading
			dataNodeAddress := fmt.Sprintf("localhost:%d", handleUploadFileResponse.PortNumber)
			dataNodeConn, err := grpc.Dial(dataNodeAddress, grpc.WithInsecure(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(1024*1024*100)))
			if err != nil {
				log.Fatalf("did not connect to data node: %v", err)
			}
			defer dataNodeConn.Close()
			dataNodeClient := pb.NewFileServiceClient(dataNodeConn)

			// Upload file to data node
			uploadFileResponse, err := dataNodeClient.UploadFile(ctx, &pb.FileUploadRequest{
				FileName:    fileName,
				FileContent: fileContent,
			})
			if err != nil {
				log.Fatalf("could not upload file to data node: %v", err)
			}
			fmt.Println("Upload File Response:", uploadFileResponse.Message)

		case "download":
			// Prompt user for filename to download
			var fileName string
			fmt.Print("Enter filename to download (without extension): ")
			fmt.Scanln(&fileName)

			// Request port numbers from master node for downloading
			handleDownloadFileResponse, err := masterClient.HandleDownloadFile(ctx, &pb.HandleDownloadFileRequest{
				FileName: fileName,
			})
			if err != nil {
				log.Fatalf("could not handle download file request: %v", err)
			}

			// Create a directory for storing downloaded files if it doesn't exist
			downloadDir := "./downloading"
			if _, err := os.Stat(downloadDir); os.IsNotExist(err) {
				err := os.Mkdir(downloadDir, 0755)
				if err != nil {
					log.Fatalf("could not create download directory: %v", err)
				}
			}

			// Convert []int32 to []int
			portNumbers := make([]int, len(handleDownloadFileResponse.PortNumbers))
			for i, v := range handleDownloadFileResponse.PortNumbers {
				portNumbers[i] = int(v)
			}
			// Print all IP addresses and port numbers returned
			for i := range handleDownloadFileResponse.IpAddress {
				fmt.Printf("IP: %s, Port: %d\n", handleDownloadFileResponse.IpAddress[i], handleDownloadFileResponse.PortNumbers[i])
			}

			// Connect to data nodes using a randomly selected IP address and port number
			index := rand.Intn(len(handleDownloadFileResponse.IpAddress))
			dataNodeAddress := fmt.Sprintf("%s%d", handleDownloadFileResponse.IpAddress[index], handleDownloadFileResponse.PortNumbers[index])
			dataNodeConn, err := grpc.Dial(dataNodeAddress, grpc.WithInsecure(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(1024*1024*100)))
			if err != nil {
				log.Fatalf("did not connect to data node: %v", err)
			}
			defer dataNodeConn.Close()
			dataNodeClient := pb.NewFileServiceClient(dataNodeConn)

			// Download file from data node
			downloadFileResponse, err := dataNodeClient.DownloadFile(ctx, &pb.FileDownloadRequest{
				FileName: fileName,
			})
			if err != nil {
				log.Fatalf("could not download file from data node: %v", err)
			}

			// Save downloaded file content to disk
			filePath := filepath.Join(downloadDir, fmt.Sprintf("%s.mp4", fileName))
			err = os.WriteFile(filePath, downloadFileResponse.FileContent, 0644)
			if err != nil {
				log.Fatalf("could not save downloaded file: %v", err)
			}
			fmt.Printf("Downloaded file '%s' saved successfully.\n", filePath)

		case "exit":
			fmt.Println("Exiting...")
			return

		default:
			fmt.Println("Invalid choice. Please enter 'upload', 'download', or 'exit'.")
		}
	}
}