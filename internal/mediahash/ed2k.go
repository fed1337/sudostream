package mediahash

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/md4" //nolint:gosec,staticcheck // G506/SA1019: AniDB ed2k identity requires MD4
)

// Ed2kChunkSize is the AniDB / eDonkey chunk size (9500 KiB).
const Ed2kChunkSize = 9728000

const (
	ed2kCopyBufSize   = 256 * 1024
	ed2kHashlistSlack = 1 // capacity padding for chunk digests
)

// Ed2k computes the AniDB ed2k hash (lowercase hex) and file size.
//
// AniDB "blue" method (wiki.anidb.net/AniDB:Ed2k-hash):
//   - size ≤ one chunk: MD4 of the whole file
//   - otherwise: MD4 of the concatenation of per-chunk MD4 digests
//
// Note: the older "red" / eMule variant appends MD4 of an empty chunk when
// size is an exact multiple of Ed2kChunkSize. AniDB blue does not.
func Ed2k(path string) (string, int64, error) {
	return Ed2kContext(context.Background(), path)
}

// Ed2kContext is like Ed2k but checks ctx between chunks.
func Ed2kContext(ctx context.Context, path string) (string, int64, error) {
	err := ctx.Err()
	if err != nil {
		return "", 0, fmt.Errorf("ed2k canceled: %w", err)
	}

	// Path is resolved via mediafs/indexer before hashing.
	file, err := os.Open(filepath.Clean(path)) // #nosec G304 -- path resolved via mediafs/indexer
	if err != nil {
		return "", 0, fmt.Errorf("open for ed2k: %w", err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("stat for ed2k: %w", err)
	}
	size := info.Size()

	if size <= Ed2kChunkSize {
		sum, hashErr := md4File(ctx, file, size)
		if hashErr != nil {
			return "", size, hashErr
		}

		return hex.EncodeToString(sum), size, nil
	}

	hashlist, hashErr := ed2kHashlist(ctx, file, size)
	if hashErr != nil {
		return "", size, hashErr
	}

	final := md4.New() //nolint:gosec // G406: AniDB ed2k requires MD4
	_, _ = final.Write(hashlist)

	return hex.EncodeToString(final.Sum(nil)), size, nil
}

func md4File(ctx context.Context, file *os.File, size int64) ([]byte, error) {
	hasher := md4.New() //nolint:gosec // G406: AniDB ed2k requires MD4
	buf := make([]byte, ed2kCopyBufSize)
	var read int64
	for read < size {
		err := ctx.Err()
		if err != nil {
			return nil, fmt.Errorf("ed2k canceled: %w", err)
		}
		n, err := file.Read(buf)
		if n > 0 {
			_, _ = hasher.Write(buf[:n])
			read += int64(n)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read for ed2k: %w", err)
		}
	}

	return hasher.Sum(nil), nil
}

func ed2kHashlist(ctx context.Context, file *os.File, size int64) ([]byte, error) {
	buf := make([]byte, Ed2kChunkSize)
	hashlist := make([]byte, 0, ((size/Ed2kChunkSize)+ed2kHashlistSlack)*md4.Size)

	for {
		err := ctx.Err()
		if err != nil {
			return nil, fmt.Errorf("ed2k canceled: %w", err)
		}

		n, err := io.ReadFull(file, buf)
		if n > 0 {
			hasher := md4.New() //nolint:gosec // G406: AniDB ed2k requires MD4
			_, _ = hasher.Write(buf[:n])
			hashlist = append(hashlist, hasher.Sum(nil)...)
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read ed2k chunk: %w", err)
		}
	}

	return hashlist, nil
}
