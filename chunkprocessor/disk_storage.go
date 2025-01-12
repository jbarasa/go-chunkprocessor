package chunkprocessor

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	chunkDirPerm  = 0755
	chunkFilePerm = 0644
	bufferSize    = 32 * 1024 // 32KB buffer size for file operations
)

type DiskStorage struct {
	baseDir     string
	chunksDir   string
	filesDir    string
	metadataDir string
	mu          sync.RWMutex
	// Per-file locks to allow parallel operations on different files
	fileLocks sync.Map
}

func NewDiskStorage(baseDir string) (*DiskStorage, error) {
	// Ensure baseDir is absolute
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %v", err)
	}

	ds := &DiskStorage{
		baseDir:     absBaseDir,
		chunksDir:   filepath.Join(absBaseDir, "chunks"),
		filesDir:    filepath.Join(absBaseDir, "files"),
		metadataDir: filepath.Join(absBaseDir, "metadata"),
	}

	// Create necessary directories with proper permissions
	dirs := []string{ds.chunksDir, ds.filesDir, ds.metadataDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}

	return ds, nil
}

// getFileLock returns a per-file mutex for concurrent operations
func (ds *DiskStorage) getFileLock(fileID string) *sync.RWMutex {
	value, _ := ds.fileLocks.LoadOrStore(fileID, &sync.RWMutex{})
	return value.(*sync.RWMutex)
}

func (ds *DiskStorage) SaveChunk(ctx context.Context, chunk *Chunk) error {
	// Get file-specific lock
	fileLock := ds.getFileLock(chunk.FileID)
	fileLock.Lock()
	defer fileLock.Unlock()

	// Create chunk directory if it doesn't exist
	chunkDir := filepath.Join(ds.chunksDir, chunk.FileID)
	if err := os.MkdirAll(chunkDir, chunkDirPerm); err != nil {
		return fmt.Errorf("failed to create chunk directory: %v", err)
	}

	// Write chunk data
	chunkPath := filepath.Join(chunkDir, fmt.Sprintf("%d", chunk.Index))
	f, err := os.OpenFile(chunkPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, chunkFilePerm)
	if err != nil {
		return fmt.Errorf("failed to create chunk file: %v", err)
	}
	defer f.Close()

	writer := bufio.NewWriterSize(f, bufferSize)
	if _, err := writer.Write(chunk.Data); err != nil {
		return fmt.Errorf("failed to write chunk data: %v", err)
	}

	return writer.Flush()
}

func (ds *DiskStorage) GetChunk(ctx context.Context, fileID string, chunkIndex int) (*Chunk, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	chunkPath := filepath.Join(ds.chunksDir, fileID, fmt.Sprintf("%d", chunkIndex))
	data, err := os.ReadFile(chunkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read chunk: %v", err)
	}

	return &Chunk{
		FileID: fileID,
		Index:  chunkIndex,
		Data:   data,
	}, nil
}

func (ds *DiskStorage) SaveFileMetadata(ctx context.Context, file *FileUpload) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	data, err := json.Marshal(file)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %v", err)
	}

	metadataPath := filepath.Join(ds.metadataDir, file.ID+".json")
	f, err := os.OpenFile(metadataPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create metadata file: %v", err)
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("failed to write metadata: %v", err)
	}

	return writer.Flush()
}

func (ds *DiskStorage) GetFileMetadata(ctx context.Context, fileID string) (*FileUpload, error) {
	ds.mu.RLock()
	metadataPath := filepath.Join(ds.metadataDir, fileID+".json")
	ds.mu.RUnlock()

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %v", err)
	}

	var upload FileUpload
	if err := json.Unmarshal(data, &upload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %v", err)
	}

	return &upload, nil
}

func (ds *DiskStorage) AssembleFile(ctx context.Context, fileID string, expectedHash string) (string, error) {
	// Get metadata first under read lock
	ds.mu.RLock()
	metadata, err := ds.GetFileMetadata(ctx, fileID)
	if err != nil {
		ds.mu.RUnlock()
		return "", fmt.Errorf("failed to get metadata: %v", err)
	}
	ds.mu.RUnlock()

	// Create final directory with proper permissions
	finalDir := filepath.Join(ds.filesDir, fileID)
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create final directory: %v", err)
	}

	// Create the final file directly
	finalPath := filepath.Join(finalDir, metadata.Filename)
	f, err := os.OpenFile(finalPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %v", err)
	}
	defer f.Close()

	// Use buffered writer for better performance
	writer := bufio.NewWriterSize(f, 32*1024)
	h := sha256.New()

	// Read and write chunks
	for i := 0; i < metadata.TotalChunks; i++ {
		select {
		case <-ctx.Done():
			os.Remove(finalPath) // Clean up if cancelled
			return "", ctx.Err()
		default:
		}

		chunk, err := ds.GetChunk(ctx, fileID, i)
		if err != nil {
			os.Remove(finalPath) // Clean up on error
			return "", fmt.Errorf("failed to get chunk %d: %v", i, err)
		}

		if _, err := writer.Write(chunk.Data); err != nil {
			os.Remove(finalPath) // Clean up on error
			return "", fmt.Errorf("failed to write chunk %d: %v", i, err)
		}
		h.Write(chunk.Data)
	}

	if err := writer.Flush(); err != nil {
		os.Remove(finalPath) // Clean up on error
		return "", fmt.Errorf("failed to flush data: %v", err)
	}

	// Verify hash
	finalHash := hex.EncodeToString(h.Sum(nil))
	if expectedHash != "" && finalHash != expectedHash {
		os.Remove(finalPath) // Clean up on hash mismatch
		return "", fmt.Errorf("hash verification failed: got %s, want %s", finalHash, expectedHash)
	}

	// Clean up only chunks first, keep metadata until file is transferred
	chunkDir := filepath.Join(ds.chunksDir, fileID)
	if err := os.RemoveAll(chunkDir); err != nil {
		log.Printf("Warning: failed to cleanup chunks for %s: %v", fileID, err)
	}

	return finalPath, nil
}

func (ds *DiskStorage) CleanupSuccessfulUpload(ctx context.Context, fileID string) error {
	// Clean up metadata
	metadataPath := filepath.Join(ds.metadataDir, fileID+".json")
	if err := os.Remove(metadataPath); err != nil {
		return fmt.Errorf("failed to remove metadata file: %v", err)
	}

	// Clean up file
	fileDir := filepath.Join(ds.filesDir, fileID)
	if err := os.RemoveAll(fileDir); err != nil {
		return fmt.Errorf("failed to remove file directory: %v", err)
	}

	return nil
}

func (ds *DiskStorage) DeleteFile(ctx context.Context, fileID string) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	return ds.deleteFileInternal(fileID)
}

func (ds *DiskStorage) deleteFileInternal(fileID string) error {
	// Remove chunks directory
	chunksDir := filepath.Join(ds.chunksDir, fileID)
	if err := os.RemoveAll(chunksDir); err != nil {
		return fmt.Errorf("failed to remove chunks directory: %v", err)
	}

	// Remove metadata file
	metadataFile := filepath.Join(ds.metadataDir, fileID+".json")
	if err := os.Remove(metadataFile); err != nil {
		return fmt.Errorf("failed to remove metadata file: %v", err)
	}

	return nil
}

func (ds *DiskStorage) CleanupUploads(ctx context.Context) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	entries, err := os.ReadDir(ds.metadataDir)
	if err != nil {
		return fmt.Errorf("failed to read metadata directory: %v", err)
	}

	now := time.Now()
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		fileID := entry.Name()[:len(entry.Name())-5] // remove .json extension

		// Release lock while reading metadata
		ds.mu.Unlock()
		upload, err := ds.GetFileMetadata(ctx, fileID)
		ds.mu.Lock()

		if err != nil {
			log.Printf("Warning: failed to read metadata for %s: %v", fileID, err)
			continue
		}

		// Remove completed uploads after DefaultRetentionPeriod
		if upload.Status == StatusCompleted && now.Sub(upload.LastUpdated) > DefaultRetentionPeriod {
			if err := ds.deleteFileInternal(fileID); err != nil {
				log.Printf("Warning: failed to cleanup completed upload %s: %v", fileID, err)
			}
			continue
		}

		// Remove incomplete uploads after DefaultIncompleteRetention
		if upload.Status != StatusCompleted && now.Sub(upload.LastUpdated) > DefaultIncompleteRetention {
			if err := ds.deleteFileInternal(fileID); err != nil {
				log.Printf("Warning: failed to cleanup incomplete upload %s: %v", fileID, err)
			}
		}
	}

	return nil
}

func (ds *DiskStorage) ListIncompleteUploads(ctx context.Context) ([]*FileUpload, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	files, err := os.ReadDir(ds.metadataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata directory: %v", err)
	}

	var incompleteUploads []*FileUpload
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		fileID := file.Name()[:len(file.Name())-5] // remove .json extension
		upload, err := ds.GetFileMetadata(ctx, fileID)
		if err != nil {
			continue
		}

		if upload.Status != StatusCompleted {
			incompleteUploads = append(incompleteUploads, upload)
		}
	}

	return incompleteUploads, nil
}
