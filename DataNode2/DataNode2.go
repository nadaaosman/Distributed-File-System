package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"
	"strings"
	pb "Lab1/Services"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

)

const (
	port                   = ":50053"
	masterAddress          = "localhost:50051" // Address of the master node
	maxMessageLength       = 1024 * 1024 * 100 // 100 MB (adjust as needed)
	id               int32 = 1
)

type server struct {
	pb.UnimplementedFileServiceServer
}

// Implement FileService server methods.

func (s *server) UploadFile(ctx context.Context, in *pb.FileUploadRequest) (*pb.UploadResponse, error) {
	log.Printf("Received file upload request: %s", in.FileName)

// Retrieve metadata from the context
md, ok := metadata.FromIncomingContext(ctx)
if !ok {
    // No metadata found in the context
    log.Println("No metadata found in the context")
}
// Extract values from metadata
clientIP := md.Get("client-ip")
clientPort := md.Get("client-port")

fmt.Println("Context values:")
fmt.Printf("Client IP: %s\n",clientIP)
fmt.Printf("Client Port: %s\n",clientPort)


// md_ := metadata.Pairs("client-ip", clientIP, "client-port",clientPort)
// ctx_ := metadata.NewOutgoingContext(context.Background(), md_)

clientIPStr := strings.Join(clientIP, ",")
clientPortStr := strings.Join(clientPort, ",")
md_ := metadata.Pairs("client-ip", clientIPStr, "client-port", clientPortStr)
ctx_:= metadata.NewOutgoingContext(context.Background(), md_)

	// Create a directory if it doesn't exist
    // Convert id to string before formatting directory path
    dir := fmt.Sprintf("./uploaded_files_%d", id)

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.Mkdir(dir, 0755)
		if err != nil {
			return nil, fmt.Errorf("failed to create directory: %v", err)
		}
	}
	// Create a file to save the uploaded content
	filePath := fmt.Sprintf("%s/%s.mp4", dir, in.FileName)
	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	// Write the content to the file
	_, err = file.Write(in.FileContent)
	if err != nil {
		return nil, fmt.Errorf("failed to write content to file: %v", err)
	}

	log.Printf("File uploaded and saved at: %s", filePath)

	 // Asynchronously notify the master node about the upload
	 go func() {
        // Set up a connection to the master node
        masterConn, err := grpc.Dial(masterAddress, grpc.WithInsecure())
        if err != nil {
            log.Fatalf("did not connect to master node: %v", err)
        }
        defer masterConn.Close()
        masterClient := pb.NewFileServiceClient(masterConn)

        // Notify the master node about the upload
        _, err_ := masterClient.NotifyUploaded(ctx_, &pb.NotifyUploadedRequest{
            FileName: in.FileName,
            DataNode: id,
            FilePath: filePath,
        })
        if err_ != nil {
            log.Fatalf("could not handle notify uploaded request: %v", err)
        }
    }()

	return &pb.UploadResponse{Message: "File uploaded successfully"}, nil
}

func (s *server) DownloadFile(ctx context.Context, in *pb.FileDownloadRequest) (*pb.DownloadResponse, error) {
	log.Printf("Received file download request: %s", in.FileName)
	// Construct the directory path based on the ID
	dir := fmt.Sprintf("./uploaded_files_%d", id)

	// Construct the file path
	filePath := filepath.Join(dir, in.FileName+".mp4")

	// Read the content of the file
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}
	// Create and return the response with the file content
	response := &pb.DownloadResponse{
		FileContent: fileContent,
	}
	return response, nil
}

func (s *server) updatelive() {
	// Set up a connection to the master node
	masterConn, err := grpc.Dial(masterAddress, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect to master node: %v", err)
	}
	defer masterConn.Close()
	masterClient := pb.NewFileServiceClient(masterConn)

	for {
		time.Sleep( time.Second) // Sleep for 10 seconds as specified

		// Send keepalive message to master node
		keepAliveRequest := &pb.KeepAliveRequest{
			DataNode: fmt.Sprintf("%d", id), // Convert id to string
			IsAlive:  1,                     // Set the liveness status based on the data node's health check
			// Add the available ports
		}

		_, err := masterClient.KeepAlive(context.Background(), keepAliveRequest)
		if err != nil {
			log.Printf("Failed to send keepalive message: %v", err)
		}
	}
	// return &pb.KeepAliveResponse{}, nil
}
func (s *server) Replicate(ctx context.Context, in *pb.ReplicateRequest) (*pb.ReplicateResponse, error) {
	log.Printf("alooo333:")
    // Read the content of the file
    fileContent, err := os.ReadFile(in.FilePath)
    if err != nil {
        return nil, fmt.Errorf("failed to read file content: %v", err)
    }

    // Iterate over the provided IP addresses and ports
    for i, ip := range in.IpAddresses {
        // Establish connection with the data node
        conn, err := grpc.Dial(fmt.Sprintf("%s%d", ip, in.PortNumbers[i]), grpc.WithInsecure())
        if err != nil {
            log.Printf("failed to connect to data node %s:%d: %v", ip, in.PortNumbers[i], err)
            continue
        }
        defer conn.Close()

        // Create a client instance for the data node
        client := pb.NewFileServiceClient(conn)

        // Invoke the UploadFile service
        uploadResponse, err := client.UploadFile(ctx, &pb.FileUploadRequest{
            FileName:    in.FileName,
            FileContent: fileContent,
        })
        if err != nil {
            log.Printf("failed to upload file to data node %s:%d: %v", ip, in.PortNumbers[i], err)
            continue
        }

        log.Printf("Uploaded file %s to data node %s:%d successfully. Response: %s", in.FileName, ip, in.PortNumbers[i], uploadResponse.Message)
    }

    return &pb.ReplicateResponse{}, nil
}


func main() {
	// Initialize the data node
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}


	s := grpc.NewServer(grpc.MaxRecvMsgSize(maxMessageLength))
	pb.RegisterFileServiceServer(s, &server{})
	log.Printf("Server listening on port %s", port)
	go s.Serve(lis) // Start the gRPC server in a separate goroutine

	server := &server{} // Create an instance of the server

	go server.updatelive()
	// Block main goroutine
	select {}
}
