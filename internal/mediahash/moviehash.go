// Package mediahash computes content identity hashes for media files (providers).
package mediahash

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const hashChunkSize = 64 * 1024

// MovieHashMinSize is the minimum file size required for OpenSubtitles moviehash.
const MovieHashMinSize = hashChunkSize

// ErrFileTooSmall is returned when the media file is under 64KiB.
var ErrFileTooSmall = errors.New("file too small for moviehash")

// MovieHash computes the OpenSubtitles moviehash for path and returns hash + filesize.
func MovieHash(path string) (string, int64, error) {
	// Path is resolved via mediafs/indexer before hashing.
	file, err := os.Open(filepath.Clean(path)) // #nosec G304 -- path resolved via mediafs/indexer
	if err != nil {
		return "", 0, fmt.Errorf("open for moviehash: %w", err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("stat for moviehash: %w", err)
	}
	size := info.Size()
	if size < hashChunkSize {
		return "", size, ErrFileTooSmall
	}

	var hash uint64
	hash += uint64(size) // OpenSubtitles algorithm treats size as uint64

	head := make([]byte, hashChunkSize)
	_, err = io.ReadFull(file, head)
	if err != nil {
		return "", size, fmt.Errorf("read moviehash head: %w", err)
	}
	hash = addHashChunk(hash, head)

	_, err = file.Seek(-hashChunkSize, io.SeekEnd)
	if err != nil {
		return "", size, fmt.Errorf("seek moviehash tail: %w", err)
	}
	tail := make([]byte, hashChunkSize)
	_, err = io.ReadFull(file, tail)
	if err != nil {
		return "", size, fmt.Errorf("read moviehash tail: %w", err)
	}
	hash = addHashChunk(hash, tail)

	return fmt.Sprintf("%016x", hash), size, nil
}

func addHashChunk(hash uint64, chunk []byte) uint64 {
	for i := 0; i+8 <= len(chunk); i += 8 {
		hash += binary.LittleEndian.Uint64(chunk[i : i+8])
	}

	return hash
}
