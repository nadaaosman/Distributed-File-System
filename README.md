# Distributed File System

## Overview
Distributed systems are a cornerstone of modern computing, enabling a group of computers to work together as a single cohesive unit. Our Distributed File System (DFS) project showcases a simplified model that supports reading and writing MP4 files while ensuring fault tolerance through replication.

## Features
- **File Uploading**: Clients can upload MP4 files to the DFS.
- **File Downloading**: Clients can download MP4 files from the DFS.
- **Replication**: Ensures that each file is replicated across multiple nodes for fault tolerance.
- **Fault Tolerance**: The system can handle individual node failures without affecting overall uptime.

## Architecture
The DFS is designed with a centralized architecture comprising two main components:

### 1. Master Tracker Node
- Maintains a look-up table with columns for file name, Data Keeper node, file path on the data node, and the status of the data node (alive or not).
- Multi-threaded to handle multiple client requests simultaneously.
- Coordinates file replication and node status monitoring.

### 2. Data Keeper Nodes
- Store the actual data files.
- Send regular heartbeat signals to the Master Tracker to indicate their status.
- Multi-threaded to handle multiple client requests simultaneously.

## Communication
- All communication between the Master Tracker, Data Keepers, and Clients is done over gRPC.
- File transfers are conducted over TCP for reliable data transmission.

## Heartbeats
- Data Keeper nodes send a keepalive ping to the Master Tracker every second.
- The Master Tracker updates the look-up table based on these pings and marks nodes as alive or down accordingly.

## Protocols

### Uploading a File
1. The client contacts the Master Tracker.
2. The Master Tracker responds with the port number of a Data Keeper node.
3. The client uploads the file to the specified Data Keeper node.
4. The Data Keeper node notifies the Master Tracker upon successful upload.
5. The Master Tracker updates the look-up table and notifies the client of success.
6. The Master Tracker selects two additional nodes to replicate the file.

### Replication
- A dedicated thread in the Master Tracker checks for replication every 10 seconds.
- Files are replicated to ensure that each file exists on at least three alive Data Keeper nodes.

### Downloading a File
1. The client requests the Master Tracker for a specific file.
2. The Master Tracker provides a list of Data Keeper nodes storing the file.
3. The client downloads the file from the provided nodes (parallel downloading is optional).

## Technologies and Languages Used
- **Programming Language**: Go
- **Communication Protocol**: gRPC
- **Concurrency**: Go routines and channels
- **File Transfer**: TCP

## Project Structure
- `Client.go`: Handles client-side operations including file upload and download.
- `DataNode.go`: Implements the Data Keeper node functionality.
- `MasterNode.go`: Implements the Master Tracker node functionality.
- `services.proto`: Defines the gRPC services and messages.

## Setup and Running the Project
1. **Master Tracker Node**:
    ```bash
    go run MasterNode.go
    ```
2. **Data Keeper Nodes**:
    ```bash
    go run DataNode.go
    ```
3. **Client**:
    ```bash
    go run Client.go
    ```

Ensure that you have set up the correct IP addresses and ports in the respective files before running the project.
---
