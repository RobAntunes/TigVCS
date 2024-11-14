// internal/safe/compression.go
package safe

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// CompressionOptions configures compression behavior
type CompressionOptions struct {
	// Minimum size in bytes before compressing
	MinSize int
	// Compression level (1=fastest, 3=best)
	Level int
	// File extensions to skip compression for
	SkipExtensions []string
	// Maximum file size for single-shot compression
	StreamingThreshold int64
}

// DefaultCompressionOptions provides sensible defaults
func DefaultCompressionOptions() CompressionOptions {
	return CompressionOptions{
		MinSize:           1024, // 1KB
		Level:             2,    // Balanced speed/compression
		StreamingThreshold: 50 * 1024 * 1024, // 50MB
		SkipExtensions: []string{
			".zip", ".gz", ".zst", ".xz", ".bz2",
			".png", ".jpg", ".jpeg", ".gif", ".webp",
			".mp3", ".mp4", ".avi", ".mkv",
			".pdf", ".docx", ".xlsx",
		},
	}
}

// compressionManager handles compression operations
type compressionManager struct {
	opts CompressionOptions
	
	// Encoder/decoder pools
	encoders sync.Pool
	decoders sync.Pool
	
	// Buffer pools for compression operations
	smallBufs sync.Pool
	largeBufs sync.Pool
}

func newCompressionManager(opts CompressionOptions) (*compressionManager, error) {
	// Create encoder/decoder for validation
	enc, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(opts.Level)),
		zstd.WithEncoderConcurrency(1),
	)
	if err != nil {
		return nil, fmt.Errorf("creating test encoder: %w", err)
	}
	enc.Close()

	dec, err := zstd.NewReader(nil,
		zstd.WithDecoderConcurrency(1),
	)
	if err != nil {
		return nil, fmt.Errorf("creating test decoder: %w", err)
	}
	dec.Close()

	cm := &compressionManager{
		opts: opts,
		encoders: sync.Pool{
			New: func() interface{} {
				enc, _ := zstd.NewWriter(nil,
					zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(opts.Level)),
					zstd.WithEncoderConcurrency(1),
				)
				return enc
			},
		},
		decoders: sync.Pool{
			New: func() interface{} {
				dec, _ := zstd.NewReader(nil,
					zstd.WithDecoderConcurrency(1),
				)
				return dec
			},
		},
		smallBufs: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, 32*1024)) // 32KB
			},
		},
		largeBufs: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, 1024*1024)) // 1MB
			},
		},
	}

	return cm, nil
}

// shouldCompress determines if content should be compressed
func (cm *compressionManager) shouldCompress(path string, size int) bool {
	// Check minimum size
	if size < cm.opts.MinSize {
		return false
	}

	// Check file extension
	ext := strings.ToLower(filepath.Ext(path))
	for _, skipExt := range cm.opts.SkipExtensions {
		if ext == skipExt {
			return false
		}
	}

	return true
}

// compress compresses content
func (cm *compressionManager) compress(path string, content []byte) ([]byte, error) {
	if !cm.shouldCompress(path, len(content)) {
		return content, nil
	}

	// Get encoder from pool
	enc := cm.encoders.Get().(*zstd.Encoder)
	defer cm.encoders.Put(enc)

	// Get appropriate buffer
	var buf *bytes.Buffer
	if len(content) < 32*1024 {
		buf = cm.smallBufs.Get().(*bytes.Buffer)
		defer cm.smallBufs.Put(buf)
	} else {
		buf = cm.largeBufs.Get().(*bytes.Buffer)
		defer cm.largeBufs.Put(buf)
	}
	buf.Reset()

	// Compress content
	if int64(len(content)) > cm.opts.StreamingThreshold {
		return cm.compressStream(enc, content)
	}

	return enc.EncodeAll(content, buf.Bytes()), nil
}

// compressStream handles large content compression
func (cm *compressionManager) compressStream(enc *zstd.Encoder, content []byte) ([]byte, error) {
	buf := cm.largeBufs.Get().(*bytes.Buffer)
	defer cm.largeBufs.Put(buf)
	buf.Reset()

	enc.Reset(buf)
	
	// Create reader for content
	reader := bytes.NewReader(content)
	
	// Stream compression
	_, err := io.Copy(enc, reader)
	if err != nil {
		return nil, fmt.Errorf("streaming compression: %w", err)
	}
	
	err = enc.Close()
	if err != nil {
		return nil, fmt.Errorf("finalizing compression: %w", err)
	}

	// Return compressed data
	return buf.Bytes(), nil
}

// decompress decompresses content
func (cm *compressionManager) decompress(content []byte) ([]byte, error) {
	// Get decoder from pool
	dec := cm.decoders.Get().(*zstd.Decoder)
	defer cm.decoders.Put(dec)

	// Check if content is actually compressed
	if len(content) > 4 && bytes.Equal(content[:4], []byte{0x28, 0xB5, 0x2F, 0xFD}) {
		// Content is zstd compressed
		if int64(len(content)) > cm.opts.StreamingThreshold {
			return cm.decompressStream(dec, content)
		}
		return dec.DecodeAll(content, nil)
	}

	// Content wasn't compressed
	return content, nil
}

// decompressStream handles large content decompression
func (cm *compressionManager) decompressStream(dec *zstd.Decoder, content []byte) ([]byte, error) {
	buf := cm.largeBufs.Get().(*bytes.Buffer)
	defer cm.largeBufs.Put(buf)
	buf.Reset()

	// Create reader for compressed content
	reader := bytes.NewReader(content)
	dec.Reset(reader)

	// Stream decompression
	_, err := io.Copy(buf, dec)
	if err != nil {
		return nil, fmt.Errorf("streaming decompression: %w", err)
	}

	// Return decompressed data
	return buf.Bytes(), nil
}

// close cleans up resources
func (cm *compressionManager) close() {
	// Close all pooled encoders/decoders
	for {
		if enc := cm.encoders.Get(); enc == nil {
			break
		} else {
			enc.(*zstd.Encoder).Close()
		}
	}
	for {
		if dec := cm.decoders.Get(); dec == nil {
			break
		} else {
			dec.(*zstd.Decoder).Close()
		}
	}
}