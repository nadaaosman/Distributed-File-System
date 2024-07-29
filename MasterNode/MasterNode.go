package main

import (
	pb "Lab1/Services"
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	port             = ":50060"
	keepAliveTimeout = 2* time.Second // Adjust timeout duration as needed
)

// FileRecord represents a record in the lookup table.
type FileRecord struct {
	FileName  string
	FilePaths []string
	DataNodes []int32
	// Liveness  []bool
}

// MachineRecord represents a record in the machines lookup table.
type MachineRecord struct {
	IPAddress      string
	AvailablePorts []int32
	Liveness       bool
}

type server struct {
	fileRecords      map[string]*FileRecord
	machineRecords   []*MachineRecord
	lastKeepAliveMap map[int]time.Time // Map to store the last keepalive timestamp for each data node
	mutex            sync.Mutex
	pb.UnimplementedFileServiceServer
}

func (s *server) PrintMachineRecords() {
	fmt.Println("Machine Records:")
	for i, record := range s.machineRecords {
		fmt.Printf("Machine %d:\n", i)
		fmt.Printf("  IPAddress: %s\n", record.IPAddress)
		fmt.Printf("  Liveness: %t\n", record.Liveness)
		fmt.Printf("  AvailablePorts: %v\n", record.AvailablePorts)
	}
}

func (s *server) PrintFileRecords() {
	fmt.Println("File Records:")
	for filename, record := range s.fileRecords {
		fmt.Printf("File: %s\n", filename)
		fmt.Printf("  FilePaths: %v\n", record.FilePaths)
		fmt.Printf("  DataNodes: %v\n", record.DataNodes)
		// fmt.Printf("  Liveness: %v\n", record.Liveness)
	}
}

// Implement FileService server methods.
func (s *server) HandleUploadFile(ctx context.Context, in *pb.HandleUploadFileRequest) (*pb.HandleUploadFileResponse, error) {
	// Lock the mutex to prevent concurrent access to shared data structures
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check if there are any alive machines
	aliveMachines := make([]*MachineRecord, 0)
	for _, machine := range s.machineRecords {
		if machine.Liveness {
			aliveMachines = append(aliveMachines, machine)
		}
	}
	// Check if there are no alive machines
	if len(aliveMachines) == 0 {
		return nil, errors.New("no alive machines available")
	}

	// Randomly select one alive machine
	selectedMachine := aliveMachines[rand.Intn(len(aliveMachines))]

	// Randomly select one available port from the selected machine
	selectedPort := selectedMachine.AvailablePorts[rand.Intn(len(selectedMachine.AvailablePorts))]
	selectedIP := selectedMachine.IPAddress

	// Construct the response with the selected IP address and port
	response := &pb.HandleUploadFileResponse{
		PortNumber: selectedPort,
		IpAddress:  selectedIP,
	}

	return response, nil
}

func (s *server) HandleDownloadFile(ctx context.Context, in *pb.HandleDownloadFileRequest) (*pb.HandleDownloadFileResponse, error) {
	// Simulate returning port numbers of data nodes that have the file
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Search for the filename in fileRecords
	fileRecord, ok := s.fileRecords[in.FileName]
	if !ok {
		return nil, errors.New("file not found")
	}

	// Select live datanodes and collect their IP addresses and available ports
	var ipAddresses []string
	var portNumbers []int32

	for i, datanode_ := range fileRecord.DataNodes {
		if s.machineRecords[datanode_].Liveness {
			datanode := s.machineRecords[fileRecord.DataNodes[i]]
			ipAddresses = append(ipAddresses, datanode.IPAddress)
			// Select a random port from available ports
			portNumbers = append(portNumbers, datanode.AvailablePorts[rand.Intn(len(datanode.AvailablePorts))])
		}
	}

	// Construct and return the response
	response := &pb.HandleDownloadFileResponse{
		IpAddress:   ipAddresses,
		PortNumbers: portNumbers,
	}

	return response, nil
}

func (s *server) NotifyUploaded(ctx context.Context, in *pb.NotifyUploadedRequest) (*pb.NotifyUploadedResponse, error) {

	// Seed the random number generator to ensure different results each time
	rand.Seed(time.Now().UnixNano())

	// Lock the mutex to prevent concurrent access to shared data structures
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Record the file uploaded
	// Check if the file already exists in records
	if record, ok := s.fileRecords[in.FileName]; ok {
		// Modify the file record
		record.DataNodes = append(record.DataNodes, in.DataNode)
		record.FilePaths = append(record.FilePaths, in.FilePath)

		s.PrintFileRecords()
		// record.Liveness = append(record.Liveness, true)
		// Return the response
		return &pb.NotifyUploadedResponse{}, nil}else{

	// Add file record to the main look-up table
	s.fileRecords[in.FileName] = &FileRecord{
		FileName:  in.FileName,
		FilePaths: []string{in.FilePath},
		DataNodes: []int32{in.DataNode},
		// Liveness:  []bool{true}, // Assuming the uploaded node is initially alive
	}
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
	fmt.Printf("Client IP: %s\n", clientIP)
	fmt.Printf("Client Port: %s\n", clientPort)

	// Invoke a method on the client's gRPC service to send the notification
	go func() {
		// Create a gRPC client to connect to the client
		clientAddress := fmt.Sprintf("%s:%s", clientIP[0], clientPort[0])
		conn, err := grpc.Dial(clientAddress, grpc.WithInsecure())
		if err != nil {
			// return nil, fmt.Errorf("failed to connect to client: %v", err)
			log.Printf("failed to connect to client: %v", err)
			return // Return without any value
		}
		defer conn.Close()

		// Create a client instance
		client := pb.NewFileServiceClient(conn)

		_, err_ := client.SendNotification(context.Background(), &pb.SendNotificationRequest{
			Message: "Your file has been successfully uploaded!",
		})
		if err_ != nil {
			log.Printf("failed to send notification to client: %v", err_)
		}
	}()

	// Choose two datanodes for replicate the data from the source machine to other nodes
	// Source machine
	sourceMachineId := in.DataNode
	// Initialize slices to hold the IP addresses and port numbers of live machines
	var replicateIPs []string
	var replicatePorts []int32
	var replicateIds []int32

	// Replicate to the first and second nodes
	for i := 1; i <= 1; i++ {
		replicateId := (sourceMachineId + int32(i)) % int32(len(s.machineRecords))
		if s.machineRecords[replicateId].Liveness {
			replicateIPs = append(replicateIPs, s.machineRecords[replicateId].IPAddress)
			replicatePorts = append(replicatePorts, s.machineRecords[replicateId].AvailablePorts[rand.Intn(len(s.machineRecords[replicateId].AvailablePorts))])
			replicateIds = append(replicateIds, replicateId)
			log.Printf("Replicate machine %s.", s.machineRecords[replicateId].IPAddress)
		} else {
			log.Printf("Replicate machine %s is not alive. Skipping replication to this node.", s.machineRecords[replicateId].IPAddress)
		}
	}
	// Construct the request for replication
	replicateRequest := &pb.ReplicateRequest{
		FileName:    in.FileName,
		FilePath:    in.FilePath,
		IpAddresses: replicateIPs,
		PortNumbers: replicatePorts,
		Ids:         replicateIds,
	}

	if s.machineRecords[sourceMachineId].Liveness {
		log.Printf("alooo:")
		// Start the replication process in a separate goroutine
		go func() {
			log.Printf("alooo222:")
			clientAddress := fmt.Sprintf("%s%d", s.machineRecords[sourceMachineId].IPAddress, s.machineRecords[sourceMachineId].AvailablePorts[rand.Intn(len(s.machineRecords[sourceMachineId].AvailablePorts))])
			sourceConn, err := grpc.Dial(clientAddress, grpc.WithInsecure())
			if err != nil {
				log.Printf("failed to connect to source machine: %v", err)
				return
			}
			defer sourceConn.Close()

			// Create a client instance for the source machine
			sourceClient := pb.NewFileServiceClient(sourceConn)

			// Invoke the Replicate service on the source machine
			_, err = sourceClient.Replicate(context.Background(), replicateRequest)
			if err != nil {
				log.Printf("failed to execute Replicate service on source machine: %v", err)
				return
			}
		}()
	}
	s.PrintFileRecords()
	return &pb.NotifyUploadedResponse{}, nil
}
}

// }

func (s *server) replicationScheduler() {
	for {
		time.Sleep(10 * time.Second) // Sleep for 10 seconds as specified
		s.mutex.Lock()

		// Replicate files on nodes with less than 3 replicas
		for _, fileRecord := range s.fileRecords {
			trueCount := 0
			var liveNodeIndexes []int // Slice to store the indexes of live nodes
			for i, datanode := range fileRecord.DataNodes {
				if s.machineRecords[datanode].Liveness {
					trueCount++
					liveNodeIndexes = append(liveNodeIndexes, i) // Store the index of the live node
				}
			}
			if trueCount < 2 {

				// Choose a random live node for replication
				randomIndex := rand.Intn(len(liveNodeIndexes))
				chosenNodeIndex := liveNodeIndexes[randomIndex]
				// Replicate file to other nodes (Simulated)
				// Source machine
				sourceMachineId := fileRecord.DataNodes[chosenNodeIndex]
				// Initialize slices to hold the IP addresses and port numbers of live machines
				var replicateIPs []string
				var replicatePorts []int32
				var replicateIds []int32
				// Replicate to the first and second nodes
				for i := 1; i <= 2; i++ {
					replicateId := (sourceMachineId + int32(i)) % int32(len(s.machineRecords))
					alreadyExists := false
					// Check if replicateId already exists in DataNodes
					for _, node := range fileRecord.DataNodes {
						if node == replicateId {
							alreadyExists = true
							break
						}
					}
					// If replicateId is not already in DataNodes and the node is alive, add it for replication
					if !alreadyExists && s.machineRecords[replicateId].Liveness {
						replicateIPs = append(replicateIPs, s.machineRecords[replicateId].IPAddress)
						replicatePorts = append(replicatePorts, s.machineRecords[replicateId].AvailablePorts[rand.Intn(len(s.machineRecords[replicateId].AvailablePorts))])
						replicateIds = append(replicateIds, replicateId)
					} else {
						log.Printf("Replicate machine %s is not alive. Skipping replication to this node.", s.machineRecords[replicateId].IPAddress)
					}
				}
				// Construct the request for replication
				replicateRequest := &pb.ReplicateRequest{
					FileName:    fileRecord.FileName,
					FilePath:    fileRecord.FilePaths[chosenNodeIndex],
					IpAddresses: replicateIPs,
					PortNumbers: replicatePorts,
					Ids:         replicateIds,
				}
				if s.machineRecords[sourceMachineId].Liveness {
					// Connect to the source machine and execute the Replicate service
					sourceConn, err := grpc.Dial(fmt.Sprintf("%s%d", s.machineRecords[sourceMachineId].IPAddress, s.machineRecords[sourceMachineId].AvailablePorts[rand.Intn(len(s.machineRecords[sourceMachineId].AvailablePorts))]), grpc.WithInsecure())
					if err != nil {
						log.Printf("failed to connect to source machine: %v", err)
						continue
					}
					defer sourceConn.Close()

					// Create a client instance for the source machine
					sourceClient := pb.NewFileServiceClient(sourceConn)

					// Invoke the Replicate service on the source machine
					_, err = sourceClient.Replicate(context.Background(), replicateRequest)
					if err != nil {
						log.Printf("failed to execute Replicate service on source machine: %v", err)
						continue
					}
				}
			}
		}

		s.mutex.Unlock()
	}
}

// Implement a goroutine to periodically monitor keepalive messages
func (s *server) monitorKeepAlive() {
	ticker := time.NewTicker(keepAliveTimeout)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.mutex.Lock()
			for nodeID, lastTime := range s.lastKeepAliveMap {
				active := time.Since(lastTime) < keepAliveTimeout
				s.machineRecords[nodeID].Liveness = active

				if !active {
					log.Printf("Data node %d is inactive (no keepalive message received)", nodeID)
				} else {
					log.Printf("Data node %d is active (keepalive message received)", nodeID)
				}
			}
			s.mutex.Unlock()
		}
	}
}

// Update the lastKeepAliveMap when a keepalive message is received
func (s *server) KeepAlive(ctx context.Context, in *pb.KeepAliveRequest) (*pb.KeepAliveResponse, error) {
	// Convert the string DataNode to an integer
	dataNodeID, err := strconv.Atoi(in.DataNode)
	if err != nil {
		return nil, fmt.Errorf("failed to convert DataNode to int: %v", err)
	}
	log.Printf("Data node %d is sending", dataNodeID)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.lastKeepAliveMap[dataNodeID] = time.Now()
	return &pb.KeepAliveResponse{}, nil
}

func main() {
	// Initialize the master node
	s := grpc.NewServer()

	server := &server{
		fileRecords:      make(map[string]*FileRecord),
		machineRecords:   []*MachineRecord{},
		lastKeepAliveMap: make(map[int]time.Time),
	}

	// Create a new MachineRecord instance
	newMachineRecord := &MachineRecord{
		IPAddress:      "172.28.104.84:",
		AvailablePorts: []int32{50052}, // Example port number
		Liveness:       false,
	}

	// Create a new MachineRecord instance
	newMachineRecord2 := &MachineRecord{
		IPAddress:      "172.28.104.86:",
		AvailablePorts: []int32{50053}, // Example port number
		Liveness:       false,
	}

	// Create a new MachineRecord instance
	newMachineRecord3 := &MachineRecord{
		IPAddress:      "172.28.104.84:",
		AvailablePorts: []int32{50054}, // Example port number
		Liveness:       false,
	}

		// Create a new MachineRecord instance
		newMachineRecord4 := &MachineRecord{
			IPAddress:      "172.28.104.86:",
			AvailablePorts: []int32{50055}, // Example port number
			Liveness:       false,
		}
	// Insert the new MachineRecord into the machineRecords slice of the server struct
	server.machineRecords = append(server.machineRecords, newMachineRecord)
	server.machineRecords = append(server.machineRecords, newMachineRecord2)
	server.machineRecords = append(server.machineRecords, newMachineRecord3)
	server.machineRecords = append(server.machineRecords, newMachineRecord4)
	//Start a goroutine to monitor keepalive messages
	go server.monitorKeepAlive()

	// Start replication scheduler in a separate goroutine
	go server.replicationScheduler()

	// Register FileService server
	pb.RegisterFileServiceServer(s, server)

	// Start listening for incoming connections
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	log.Printf("Server listening on port %s", port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
