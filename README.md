# File Service v1.0.0

A high-performance, chunk-based file upload service written in Go that supports concurrent uploads, resumable transfers, and robust error handling. Features built-in cloud storage integration and SHA-256 hash verification.

## About the Author

👋 Hi, I'm Joseph Barasa, a passionate Go developer focused on building high-performance, scalable systems.

- 🌐 Portfolio: [jbarasa.com](https://jbarasa.com)
- 📧 Email: jbarasa.ke@gmail.com
- 💼 **Open to Work**: Looking for exciting Golang projects and opportunities
- 🚀 Specialized in: Distributed systems, microservices, and high-performance applications

### Work With Me

I'm currently available for Go/Golang positions and projects. I believe in practical problem-solving and writing clean, efficient code rather than theoretical puzzles. If you're looking for a developer who:

- Builds robust, production-ready systems
- Focuses on real-world solutions
- Values clean code and good documentation
- Has hands-on experience with concurrent systems
- Brings practical knowledge to the table

Let's connect! Reach out via:
- Email: jbarasa.ke@gmail.com
- Website: [jbarasa.com](https://jbarasa.com)

## Table of Contents
- [Features](#features)
- [Architecture](#architecture)
- [Models](#models)
- [Configuration](#configuration)
- [Security & Hashing](#security--hashing)
- [API Reference](#api-reference)
- [Client Integration Guide](#client-integration-guide)
- [Storage](#storage)
- [Cloud Integration](#cloud-integration)
- [Error Handling](#error-handling)
- [License](#license)
- [Framework Integration Examples](#framework-integration-examples)

## Features
- ✨ Chunked file uploads with configurable chunk sizes (default 256KB)
- 🚀 Concurrent chunk processing (up to 10 chunks per file)
- 💾 Resumable uploads with 14-day retention for incomplete uploads
- 🔒 File integrity verification using SHA-256 hashing
- 📊 Real-time upload progress tracking
- ⏸️ Pause/Resume upload functionality
- 🧹 Automatic cleanup of completed and stale uploads
- 💪 High performance with buffered I/O operations
- ☁️ Built-in cloud storage integration
- 🏷️ Rich metadata support for files

## Architecture

The service is built around four main components:

1. **ChunkProcessor**: Manages the upload process and chunk coordination
2. **Storage Interface**: Handles persistent storage of chunks and files
3. **Worker Pool**: Processes chunks concurrently with controlled resource usage
4. **Cloud Storage**: Handles final file storage in cloud providers

### Key Components
- `ChunkProcessor`: Orchestrates the entire upload process
- `Storage`: Interface for chunk and file storage implementations
- `WorkerPool`: Manages concurrent chunk processing
- `DiskStorage`: Reference implementation for file system storage
- `CloudStorage`: Interface for cloud provider integration

## Security & Hashing

### File Integrity
The service uses SHA-256 hashing to ensure file integrity throughout the upload process:

1. **Chunk-Level Hashing**
```go
// Calculate chunk hash
hash := sha256.New()
hash.Write(chunkData)
chunkHash := hex.EncodeToString(hash.Sum(nil))
```

2. **File-Level Verification**
```go
// Verify assembled file
hash := sha256.New()
file, _ := os.Open(filePath)
io.Copy(hash, file)
fileHash := hex.EncodeToString(hash.Sum(nil))
if fileHash != expectedHash {
    return errors.New("file hash mismatch")
}
```

### Hash Verification Process
1. Each chunk is hashed during upload
2. Server verifies chunk hash before storage
3. Final file is hashed after assembly
4. Hash is compared with expected hash before cloud storage

## Cloud Integration

The service includes built-in cloud storage integration:

```go
// Cloud Storage Interface
type CloudStorage interface {
    UploadFile(ctx context.Context, localPath, fileName string, metadata map[string]interface{}) error
}

// Example metadata
metadata := map[string]interface{}{
    "user_id":      "user123",
    "category":     "photos",
    "content_type": "image/jpeg",
    "tags":         []string{"vacation", "summer"},
}
```

### Automatic Cleanup
- Successful uploads are automatically cleaned up after cloud storage transfer
- Failed uploads are retained for troubleshooting
- Periodic cleanup runs every 24 hours for stale uploads

### Implementation Example
```go
func uploadToCloud(ctx context.Context, filePath, fileName string, metadata map[string]interface{}) error {
    // Initialize your cloud storage provider
    cloud := initCloudStorage()
    
    // Upload file with metadata
    err := cloud.UploadFile(ctx, filePath, fileName, metadata)
    if err != nil {
        return fmt.Errorf("cloud upload failed: %v", err)
    }
    
    // Cleanup local files after successful upload
    return storage.CleanupSuccessfulUpload(ctx, fileID)
}
```

## Models

### FileUpload
```go
type FileUpload struct {
    ID              string
    Filename        string
    TotalSize       int64
    ChunkSize       int64
    TotalChunks     int
    CompletedChunks int
    Status          ChunkStatus
    Hash            string
    Metadata        map[string]interface{}
    CreatedAt       time.Time
    LastUpdated     time.Time
}
```

### Chunk
```go
type Chunk struct {
    ID          string
    FileID      string
    Index       int
    Size        int64
    Hash        string
    Status      ChunkStatus
    UploadedAt  time.Time
    LastUpdated time.Time
}
```

## Installation

Install the latest version:
```bash
go get github.com/jbarasa/chunkprocessor@latest
```

Install a specific version:
```bash
go get github.com/jbarasa/chunkprocessor@v1.0.0
```

For Go modules (go.mod):
```go
require github.com/jbarasa/chunkprocessor v1.0.0
```

Minimum Go version required: 1.16

## Configuration

### System Constants
```go
const (
    DefaultRetentionPeriod     = 48 * time.Hour
    DefaultIncompleteRetention = 336 * time.Hour // 14 days
    MaxConcurrentUploads       = 100
    MaxConcurrentChunksPerFile = 10
    MaxAssemblyWorkers         = 4
    MaxChunkSize              = 10 * 1024 * 1024 // 10MB maximum chunk size
)
```

## API Reference

### Initialize Upload
```http
POST /upload/init
Content-Type: application/json

{
    "filename": "example.mp4",
    "totalSize": 1234567890,
    "chunkSize": 10485760,
    "metadata": {
        "contentType": "video/mp4",
        "userId": "user123"
    }
}
```

### Upload Chunk
```http
POST /upload/chunk
Content-Type: multipart/form-data

fileId: "file123"
chunkIndex: 1
data: [binary data]
hash: "sha256-hash-of-chunk"
```

### Get Upload Progress
```http
GET /upload/progress/{fileId}
```

### Pause Upload
```http
POST /upload/pause/{fileId}
```

### Resume Upload
```http
POST /upload/resume/{fileId}
```

## Client Integration Guide

### Required Headers
- `Content-Type`: `multipart/form-data` for chunk uploads
- `Content-Length`: Size of the chunk in bytes

### Chunk Upload Process

1. **Initialize Upload**
```javascript
async function initializeUpload(file) {
    const chunkSize = 10 * 1024 * 1024; // 10MB chunks
    const totalChunks = Math.ceil(file.size / chunkSize);
    
    const response = await fetch('/upload/init', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            filename: file.name,
            totalSize: file.size,
            chunkSize: chunkSize,
            metadata: {
                contentType: file.type
            }
        })
    });
    
    return await response.json();
}
```

2. **Upload Chunks**
```javascript
async function uploadChunk(fileId, chunk, index) {
    const formData = new FormData();
    formData.append('fileId', fileId);
    formData.append('chunkIndex', index);
    formData.append('data', chunk);
    
    // Calculate SHA-256 hash of chunk
    const hash = await calculateHash(chunk);
    formData.append('hash', hash);
    
    const response = await fetch('/upload/chunk', {
        method: 'POST',
        body: formData
    });
    
    return await response.json();
}
```

3. **Track Progress**
```javascript
async function trackProgress(fileId) {
    const response = await fetch(`/upload/progress/${fileId}`);
    const progress = await response.json();
    return progress.percentage;
}
```

4. **Handle Resumable Uploads**
```javascript
async function resumeUpload(fileId) {
    // Get current progress
    const progress = await trackProgress(fileId);
    
    // Resume from last successful chunk
    const lastChunkIndex = Math.floor(progress * totalChunks);
    for (let i = lastChunkIndex; i < totalChunks; i++) {
        await uploadChunk(fileId, chunks[i], i);
    }
}
```

## Storage

The service includes a disk-based storage implementation with the following features:

- Buffered I/O operations (32KB buffer)
- Concurrent access handling with file-level locks
- Automatic directory structure creation
- Cleanup of temporary chunks after successful assembly

### Directory Structure
```
base_dir/
├── chunks/
│   └── {fileId}/
│       ├── 0
│       ├── 1
│       └── ...
├── files/
│   └── {assembled files}
└── metadata/
    └── {fileId}.json
```

## Usage Example

Complete example of uploading a file:

```go
// Initialize storage and processor
storage := chunkprocessor.NewDiskStorage(baseDir)
processor := chunkprocessor.NewChunkProcessor(storage, 5)

// Initialize upload with metadata
metadata := map[string]interface{}{
    "user_id":      "user123",
    "category":     "photos",
    "content_type": "image/jpeg",
    "tags":         []string{"vacation", "summer"},
}

// Create upload session
upload, err := processor.InitializeUpload(ctx, filename, totalSize, 256000, metadata)
if err != nil {
    log.Fatalf("Failed to initialize upload: %v", err)
}

// Upload chunks
for i := 0; i < totalChunks; i++ {
    chunk := &chunkprocessor.Chunk{
        FileID: upload.ID,
        Index:  i,
        Data:   chunkData,
    }
    
    if err := processor.UploadChunk(ctx, chunk); err != nil {
        log.Fatalf("Chunk upload failed: %v", err)
    }
    
    progress := float64(i+1) / float64(totalChunks) * 100
    fmt.Printf("Progress: %.2f%%\n", progress)
}

// Get assembled file
filePath, err := processor.GetFileContent(ctx, upload.ID, upload.Hash)
if err != nil {
    log.Fatalf("Assembly failed: %v", err)
}

// Upload to cloud storage
err = uploadToCloud(ctx, filePath, upload.Filename, upload.Metadata)
```

## Error Handling

The service provides detailed error responses for various scenarios:

- Invalid chunk size
- Missing chunks
- Hash mismatch
- Concurrent upload limits exceeded
- Storage errors
- Assembly failures
- Cloud storage failures

Example error response:
```json
{
    "error": {
        "code": "INVALID_CHUNK",
        "message": "Chunk hash verification failed",
        "details": {
            "chunkIndex": 5,
            "expectedHash": "...",
            "receivedHash": "..."
        }
    }
}
```

## License

This project is licensed under the Mozilla Public License Version 2.0 - see the [LICENSE](LICENSE) file for details.

## Framework Integration Examples

### Fiber Example
```go
package main

import (
    "github.com/gofiber/fiber/v2"
    "github.com/yourusername/chunkprocessor"
)

func main() {
    app := fiber.New()
    storage, _ := chunkprocessor.NewDiskStorage("./uploads")
    processor := chunkprocessor.NewChunkProcessor(storage, 5)

    // Initialize upload
    app.Post("/upload/init", func(c *fiber.Ctx) error {
        var req struct {
            Filename string                 `json:"filename"`
            TotalSize int64                `json:"totalSize"`
            ChunkSize int64                `json:"chunkSize"`
            Metadata  map[string]interface{} `json:"metadata"`
        }
        if err := c.BodyParser(&req); err != nil {
            return c.Status(400).JSON(fiber.Map{"error": err.Error()})
        }

        upload, err := processor.InitializeUpload(c.Context(), req.Filename, req.TotalSize, req.ChunkSize, req.Metadata)
        if err != nil {
            return c.Status(500).JSON(fiber.Map{"error": err.Error()})
        }

        return c.JSON(upload)
    })

    // Upload chunk
    app.Post("/upload/chunk", func(c *fiber.Ctx) error {
        fileID := c.FormValue("fileId")
        index := c.FormValue("chunkIndex")
        file, err := c.FormFile("chunk")
        if err != nil {
            return c.Status(400).JSON(fiber.Map{"error": err.Error()})
        }

        // Read chunk data
        f, err := file.Open()
        if err != nil {
            return c.Status(500).JSON(fiber.Map{"error": err.Error()})
        }
        defer f.Close()

        data, err := io.ReadAll(f)
        if err != nil {
            return c.Status(500).JSON(fiber.Map{"error": err.Error()})
        }

        chunk := &chunkprocessor.Chunk{
            FileID: fileID,
            Index:  index,
            Data:   data,
        }

        if err := processor.UploadChunk(c.Context(), chunk); err != nil {
            return c.Status(500).JSON(fiber.Map{"error": err.Error()})
        }

        return c.SendStatus(200)
    })

    app.Listen(":3000")
}
```

### Echo Example
```go
package main

import (
    "github.com/labstack/echo/v4"
    "github.com/yourusername/chunkprocessor"
)

func main() {
    e := echo.New()
    storage, _ := chunkprocessor.NewDiskStorage("./uploads")
    processor := chunkprocessor.NewChunkProcessor(storage, 5)

    // Initialize upload
    e.POST("/upload/init", func(c echo.Context) error {
        var req struct {
            Filename  string                 `json:"filename"`
            TotalSize int64                 `json:"totalSize"`
            ChunkSize int64                 `json:"chunkSize"`
            Metadata  map[string]interface{} `json:"metadata"`
        }
        if err := c.Bind(&req); err != nil {
            return echo.NewHTTPError(400, err.Error())
        }

        upload, err := processor.InitializeUpload(c.Request().Context(), req.Filename, req.TotalSize, req.ChunkSize, req.Metadata)
        if err != nil {
            return echo.NewHTTPError(500, err.Error())
        }

        return c.JSON(200, upload)
    })

    // Upload chunk
    e.POST("/upload/chunk", func(c echo.Context) error {
        fileID := c.FormValue("fileId")
        index := c.FormValue("chunkIndex")
        file, err := c.FormFile("chunk")
        if err != nil {
            return echo.NewHTTPError(400, err.Error())
        }

        src, err := file.Open()
        if err != nil {
            return echo.NewHTTPError(500, err.Error())
        }
        defer src.Close()

        data, err := io.ReadAll(src)
        if err != nil {
            return echo.NewHTTPError(500, err.Error())
        }

        chunk := &chunkprocessor.Chunk{
            FileID: fileID,
            Index:  index,
            Data:   data,
        }

        if err := processor.UploadChunk(c.Request().Context(), chunk); err != nil {
            return echo.NewHTTPError(500, err.Error())
        }

        return c.NoContent(200)
    })

    e.Start(":3000")
}
```

### Gin Example
```go
package main

import (
    "github.com/gin-gonic/gin"
    "github.com/yourusername/chunkprocessor"
)

func main() {
    r := gin.Default()
    storage, _ := chunkprocessor.NewDiskStorage("./uploads")
    processor := chunkprocessor.NewChunkProcessor(storage, 5)

    // Initialize upload with metadata
    r.POST("/upload/init", func(c *gin.Context) {
        var req struct {
            Filename  string                 `json:"filename"`
            TotalSize int64                 `json:"totalSize"`
            ChunkSize int64                 `json:"chunkSize"`
            Metadata  map[string]interface{} `json:"metadata"`
        }
        if err := c.BindJSON(&req); err != nil {
            c.JSON(400, gin.H{"error": err.Error()})
            return
        }

        upload, err := processor.InitializeUpload(c.Request.Context(), req.Filename, req.TotalSize, req.ChunkSize, req.Metadata)
        if err != nil {
            c.JSON(500, gin.H{"error": err.Error()})
            return
        }

        c.JSON(200, upload)
    })

    // Upload chunk with progress tracking
    r.POST("/upload/chunk", func(c *gin.Context) {
        fileID := c.PostForm("fileId")
        index := c.PostForm("chunkIndex")
        file, err := c.FormFile("chunk")
        if err != nil {
            c.JSON(400, gin.H{"error": err.Error()})
            return
        }

        f, err := file.Open()
        if err != nil {
            c.JSON(500, gin.H{"error": err.Error()})
            return
        }
        defer f.Close()

        data, err := io.ReadAll(f)
        if err != nil {
            c.JSON(500, gin.H{"error": err.Error()})
            return
        }

        chunk := &chunkprocessor.Chunk{
            FileID: fileID,
            Index:  index,
            Data:   data,
        }

        if err := processor.UploadChunk(c.Request.Context(), chunk); err != nil {
            c.JSON(500, gin.H{"error": err.Error()})
            return
        }

        c.Status(200)
    })

    r.Run(":3000")
}
```

### Hertz Example
```go
package main

import (
    "context"
    "github.com/cloudwego/hertz/pkg/app"
    "github.com/cloudwego/hertz/pkg/app/server"
    "github.com/yourusername/chunkprocessor"
)

func main() {
    h := server.Default()
    storage, _ := chunkprocessor.NewDiskStorage("./uploads")
    processor := chunkprocessor.NewChunkProcessor(storage, 5)

    // Initialize upload
    h.POST("/upload/init", func(ctx context.Context, c *app.RequestContext) {
        var req struct {
            Filename  string                 `json:"filename"`
            TotalSize int64                 `json:"totalSize"`
            ChunkSize int64                 `json:"chunkSize"`
            Metadata  map[string]interface{} `json:"metadata"`
        }
        if err := c.BindJSON(&req); err != nil {
            c.JSON(400, map[string]interface{}{"error": err.Error()})
            return
        }

        upload, err := processor.InitializeUpload(ctx, req.Filename, req.TotalSize, req.ChunkSize, req.Metadata)
        if err != nil {
            c.JSON(500, map[string]interface{}{"error": err.Error()})
            return
        }

        c.JSON(200, upload)
    })

    // Upload chunk
    h.POST("/upload/chunk", func(ctx context.Context, c *app.RequestContext) {
        fileID := c.FormValue("fileId")
        index := c.FormValue("chunkIndex")
        file, err := c.FormFile("chunk")
        if err != nil {
            c.JSON(400, map[string]interface{}{"error": err.Error()})
            return
        }

        f, err := file.Open()
        if err != nil {
            c.JSON(500, map[string]interface{}{"error": err.Error()})
            return
        }
        defer f.Close()

        data, err := io.ReadAll(f)
        if err != nil {
            c.JSON(500, map[string]interface{}{"error": err.Error()})
            return
        }

        chunk := &chunkprocessor.Chunk{
            FileID: fileID,
            Index:  index,
            Data:   data,
        }

        if err := processor.UploadChunk(ctx, chunk); err != nil {
            c.JSON(500, map[string]interface{}{"error": err.Error()})
            return
        }

        c.Status(200)
    })

    h.Spin()
}
```

### Gorilla Mux Example
```go
package main

import (
    "encoding/json"
    "github.com/gorilla/mux"
    "github.com/yourusername/chunkprocessor"
    "net/http"
)

func main() {
    r := mux.NewRouter()
    storage, _ := chunkprocessor.NewDiskStorage("./uploads")
    processor := chunkprocessor.NewChunkProcessor(storage, 5)

    // Initialize upload
    r.HandleFunc("/upload/init", func(w http.ResponseWriter, r *http.Request) {
        var req struct {
            Filename  string                 `json:"filename"`
            TotalSize int64                 `json:"totalSize"`
            ChunkSize int64                 `json:"chunkSize"`
            Metadata  map[string]interface{} `json:"metadata"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }

        upload, err := processor.InitializeUpload(r.Context(), req.Filename, req.TotalSize, req.ChunkSize, req.Metadata)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(upload)
    }).Methods("POST")

    // Upload chunk
    r.HandleFunc("/upload/chunk", func(w http.ResponseWriter, r *http.Request) {
        if err := r.ParseMultipartForm(32 << 20); err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }

        fileID := r.FormValue("fileId")
        index := r.FormValue("chunkIndex")
        file, _, err := r.FormFile("chunk")
        if err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }
        defer file.Close()

        data, err := io.ReadAll(file)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        chunk := &chunkprocessor.Chunk{
            FileID: fileID,
            Index:  index,
            Data:   data,
        }

        if err := processor.UploadChunk(r.Context(), chunk); err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        w.WriteHeader(http.StatusOK)
    }).Methods("POST")

    http.ListenAndServe(":3000", r)
}
```

Each framework example includes:
- Upload initialization endpoint
- Chunk upload handling
- Error handling
- Progress tracking
- Proper context management
- File cleanup

Choose the framework that best suits your needs. All examples follow similar patterns but adapt to each framework's specific conventions and best practices.
