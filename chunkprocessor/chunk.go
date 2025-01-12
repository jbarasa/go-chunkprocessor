package chunkprocessor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"runtime"
	"sync"
	"time"
)

// ChunkStatus represents the current status of a chunk upload
type ChunkStatus int

const (
	StatusPending    ChunkStatus = 1
	StatusUploading  ChunkStatus = 2
	StatusCompleted  ChunkStatus = 3
	StatusPaused     ChunkStatus = 4
	StatusError      ChunkStatus = 5
	StatusAssembling ChunkStatus = 6
)

// ChunkProcessor configuration
const (
	DefaultRetentionPeriod     = 48 * time.Hour
	DefaultIncompleteRetention = 336 * time.Hour // 14 days
	MaxConcurrentUploads       = 100
	MaxConcurrentChunksPerFile = 10
	MaxAssemblyWorkers         = 4
	MaxChunkSize               = 10 * 1024 * 1024 // 10MB maximum chunk size
)

// Chunk represents a single chunk of a file
type Chunk struct {
	ID          string
	FileID      string
	Index       int
	Size        int64
	Hash        string
	Status      ChunkStatus
	Data        []byte
	UploadedAt  time.Time
	LastUpdated time.Time
}

// FileUpload represents a file being uploaded in chunks
type FileUpload struct {
	ID              string
	Filename        string
	TotalSize       int64
	ChunkSize       int64
	TotalChunks     int
	CompletedChunks int
	Status          ChunkStatus
	Hash            string
	Chunks          map[int]*Chunk
	Metadata        map[string]interface{}
	mu              sync.RWMutex
	CreatedAt       time.Time
	LastUpdated     time.Time
}

// Storage defines the interface for chunk storage implementations
type Storage interface {
	SaveChunk(ctx context.Context, chunk *Chunk) error
	GetChunk(ctx context.Context, fileID string, chunkIndex int) (*Chunk, error)
	SaveFileMetadata(ctx context.Context, upload *FileUpload) error
	GetFileMetadata(ctx context.Context, fileID string) (*FileUpload, error)
	AssembleFile(ctx context.Context, fileID string, expectedHash string) (string, error)
	DeleteFile(ctx context.Context, fileID string) error
	CleanupSuccessfulUpload(ctx context.Context, fileID string) error
	CleanupUploads(ctx context.Context) error
}

// ChunkProcessor handles the chunk upload process
type ChunkProcessor struct {
	storage         Storage
	maxConcurrent   int
	uploadSemaphore chan struct{}
	assemblyQueue   chan assemblyJob
	workerPool      *WorkerPool
	wg              sync.WaitGroup
	quit            chan bool
}

type assemblyJob struct {
	fileID       string
	expectedHash string
	resultChan   chan assemblyResult
}

type assemblyResult struct {
	path string
	err  error
}

// WorkerPool manages a pool of workers for processing chunks
type WorkerPool struct {
	workers   chan chan work
	workQueue chan work
	quit      chan bool
	wg        sync.WaitGroup
}

type work struct {
	upload     *FileUpload
	chunk      *Chunk
	resultChan chan error
}

func NewChunkProcessor(storage Storage, maxConcurrent int) *ChunkProcessor {
	if maxConcurrent <= 0 {
		maxConcurrent = runtime.NumCPU()
	}
	if maxConcurrent > MaxConcurrentUploads {
		maxConcurrent = MaxConcurrentUploads
	}

	cp := &ChunkProcessor{
		storage:         storage,
		maxConcurrent:   maxConcurrent,
		uploadSemaphore: make(chan struct{}, maxConcurrent),
		assemblyQueue:   make(chan assemblyJob, maxConcurrent),
		workerPool:      newWorkerPool(maxConcurrent * MaxConcurrentChunksPerFile),
		quit:            make(chan bool),
	}

	// Start assembly workers
	for i := 0; i < MaxAssemblyWorkers; i++ {
		cp.wg.Add(1)
		go cp.assemblyWorker()
	}

	return cp
}

func newWorkerPool(maxWorkers int) *WorkerPool {
	pool := &WorkerPool{
		workers:   make(chan chan work, maxWorkers),
		workQueue: make(chan work),
		quit:      make(chan bool),
	}

	// Start the dispatcher
	go pool.dispatch()

	// Create workers
	for i := 0; i < maxWorkers; i++ {
		pool.wg.Add(1)
		go pool.startWorker()
	}

	return pool
}

func (p *WorkerPool) dispatch() {
	for {
		select {
		case job := <-p.workQueue:
			workerChan := <-p.workers
			workerChan <- job
		case <-p.quit:
			return
		}
	}
}

func (p *WorkerPool) startWorker() {
	defer p.wg.Done()

	jobChan := make(chan work)
	p.workers <- jobChan

	for {
		select {
		case job := <-jobChan:
			// Process the chunk
			err := processChunk(job.upload, job.chunk)
			job.resultChan <- err
			p.workers <- jobChan
		case <-p.quit:
			return
		}
	}
}

func (p *WorkerPool) Stop() {
	close(p.quit)
	p.wg.Wait()
}

func (cp *ChunkProcessor) Stop() {
	close(cp.quit)
	cp.workerPool.Stop()
	cp.wg.Wait()
}

func (cp *ChunkProcessor) assemblyWorker() {
	defer cp.wg.Done()

	for {
		select {
		case job := <-cp.assemblyQueue:
			// Create a new context with timeout for assembly
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			path, err := cp.storage.AssembleFile(ctx, job.fileID, job.expectedHash)
			cancel()

			select {
			case job.resultChan <- assemblyResult{path: path, err: err}:
			case <-cp.quit:
				return
			}
		case <-cp.quit:
			return
		}
	}
}

func (cp *ChunkProcessor) InitializeUpload(ctx context.Context, filename string, totalSize int64, chunkSize int64, metadata map[string]interface{}) (*FileUpload, error) {
	if totalSize <= 0 || chunkSize <= 0 {
		return nil, errors.New("invalid size parameters")
	}
	if chunkSize > MaxChunkSize {
		return nil, fmt.Errorf("chunk size exceeds maximum allowed size of %d bytes", MaxChunkSize)
	}

	totalChunks := int((totalSize + chunkSize - 1) / chunkSize)
	fileID := generateFileID(filename)

	// Check if upload already exists
	if existing, err := cp.storage.GetFileMetadata(ctx, fileID); err == nil && existing != nil {
		return existing, nil
	}

	upload := &FileUpload{
		ID:          fileID,
		Filename:    filename,
		TotalSize:   totalSize,
		ChunkSize:   chunkSize,
		TotalChunks: totalChunks,
		Status:      StatusPending,
		Chunks:      make(map[int]*Chunk),
		Metadata:    metadata,
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}

	if err := cp.storage.SaveFileMetadata(ctx, upload); err != nil {
		return nil, err
	}

	return upload, nil
}

func (cp *ChunkProcessor) UploadChunk(ctx context.Context, chunk *Chunk) error {
	select {
	case cp.uploadSemaphore <- struct{}{}:
		defer func() { <-cp.uploadSemaphore }()
	case <-ctx.Done():
		return ctx.Err()
	}

	upload, err := cp.storage.GetFileMetadata(ctx, chunk.FileID)
	if err != nil {
		return fmt.Errorf("failed to get metadata: %v", err)
	}

	upload.mu.Lock()
	defer upload.mu.Unlock()

	if upload.Status == StatusCompleted {
		return fmt.Errorf("upload already completed")
	}

	if chunk.Index >= upload.TotalChunks {
		return fmt.Errorf("invalid chunk index %d, total chunks: %d", chunk.Index, upload.TotalChunks)
	}

	if err := cp.storage.SaveChunk(ctx, chunk); err != nil {
		return fmt.Errorf("failed to save chunk: %v", err)
	}

	upload.CompletedChunks++

	// If this was the last chunk, calculate final hash and mark as completed
	if upload.CompletedChunks == upload.TotalChunks {
		// Calculate final hash synchronously to prevent race conditions
		if ds, ok := cp.storage.(*DiskStorage); ok {
			hash := sha256.New()
			for i := 0; i < upload.TotalChunks; i++ {
				chunk, err := ds.GetChunk(ctx, upload.ID, i)
				if err != nil {
					upload.mu.Unlock()
					return fmt.Errorf("failed to calculate final hash: %v", err)
				}
				hash.Write(chunk.Data)
			}
			upload.Hash = hex.EncodeToString(hash.Sum(nil))
			log.Printf("Final hash calculated: %s", upload.Hash)
		}

		upload.Status = StatusCompleted
		if err := cp.storage.SaveFileMetadata(ctx, upload); err != nil {
			return fmt.Errorf("failed to update metadata: %v", err)
		}

		// Queue assembly job
		resultChan := make(chan assemblyResult, 1)
		job := assemblyJob{
			fileID:       chunk.FileID,
			expectedHash: upload.Hash,
			resultChan:   resultChan,
		}

		select {
		case cp.assemblyQueue <- job:
		case <-ctx.Done():
			return ctx.Err()
		}
	} else {
		if err := cp.storage.SaveFileMetadata(ctx, upload); err != nil {
			return fmt.Errorf("failed to update metadata: %v", err)
		}
	}

	return nil
}

func (cp *ChunkProcessor) GetFileContent(ctx context.Context, fileID string, expectedHash string) (string, error) {
	upload, err := cp.storage.GetFileMetadata(ctx, fileID)
	if err != nil {
		return "", fmt.Errorf("failed to get metadata: %v", err)
	}

	if upload.Status != StatusCompleted {
		return "", fmt.Errorf("upload not completed")
	}

	if expectedHash != "" && upload.Hash != expectedHash {
		return "", fmt.Errorf("hash verification failed: got %s, want %s", upload.Hash, expectedHash)
	}

	// For single chunk files, assemble directly without using worker pool
	if upload.TotalChunks == 1 {
		return cp.storage.AssembleFile(ctx, fileID, expectedHash)
	}

	resultChan := make(chan assemblyResult, 1)
	job := assemblyJob{
		fileID:       fileID,
		expectedHash: expectedHash,
		resultChan:   resultChan,
	}

	select {
	case cp.assemblyQueue <- job:
	case <-ctx.Done():
		return "", ctx.Err()
	}

	select {
	case result := <-resultChan:
		return result.path, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (cp *ChunkProcessor) PauseUpload(ctx context.Context, fileID string) error {
	upload, err := cp.storage.GetFileMetadata(ctx, fileID)
	if err != nil {
		return err
	}

	upload.mu.Lock()
	defer upload.mu.Unlock()

	if upload.Status == StatusCompleted {
		return errors.New("cannot pause completed upload")
	}

	upload.Status = StatusPaused
	upload.LastUpdated = time.Now()
	return cp.storage.SaveFileMetadata(ctx, upload)
}

func (cp *ChunkProcessor) ResumeUpload(ctx context.Context, fileID string) error {
	upload, err := cp.storage.GetFileMetadata(ctx, fileID)
	if err != nil {
		return err
	}

	upload.mu.Lock()
	defer upload.mu.Unlock()

	if upload.Status != StatusPaused {
		return errors.New("upload is not paused")
	}

	upload.Status = StatusUploading
	upload.LastUpdated = time.Now()
	return cp.storage.SaveFileMetadata(ctx, upload)
}

func (cp *ChunkProcessor) GetProgress(ctx context.Context, fileID string) (float64, error) {
	upload, err := cp.storage.GetFileMetadata(ctx, fileID)
	if err != nil {
		return 0, err
	}

	upload.mu.RLock()
	defer upload.mu.RUnlock()

	if upload.TotalChunks == 0 {
		return 0, nil
	}

	return float64(upload.CompletedChunks) / float64(upload.TotalChunks) * 100, nil
}

func (cp *ChunkProcessor) GetFileInfo(ctx context.Context, fileID string) (*FileUpload, error) {
	return cp.storage.GetFileMetadata(ctx, fileID)
}

func (cp *ChunkProcessor) CleanupUploads(ctx context.Context) error {
	return cp.storage.CleanupUploads(ctx)
}

func (cp *ChunkProcessor) CleanupSuccessfulUpload(ctx context.Context, fileID string) error {
	upload, err := cp.storage.GetFileMetadata(ctx, fileID)
	if err != nil {
		return fmt.Errorf("failed to get upload metadata: %v", err)
	}

	if upload.Status != StatusCompleted {
		return fmt.Errorf("cannot cleanup incomplete upload: %s", fileID)
	}

	return cp.storage.CleanupSuccessfulUpload(ctx, fileID)
}

// Helper functions
func generateFileID(filename string) string {
	hash := sha256.New()
	io.WriteString(hash, filename)
	io.WriteString(hash, time.Now().String())
	return hex.EncodeToString(hash.Sum(nil))
}

func processChunk(upload *FileUpload, chunk *Chunk) error {
	// Validate chunk
	if chunk.Index >= upload.TotalChunks {
		return errors.New("invalid chunk index")
	}

	// Calculate chunk hash
	hash := sha256.New()
	hash.Write(chunk.Data)
	chunk.Hash = hex.EncodeToString(hash.Sum(nil))
	chunk.Status = StatusCompleted
	chunk.LastUpdated = time.Now()

	return nil
}
