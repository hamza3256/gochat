package persistence

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	RecordTypeMessage    byte = 0x01
	RecordTypeAck        byte = 0x02
	RecordTypeCheckpoint byte = 0x03

	headerSize = 4 + 4 + 1 // CRC32 + Length + Type
)

// WALConfig controls segment rotation and sync behaviour.
type WALConfig struct {
	Dir             string
	SegmentMaxBytes int64
	SyncIntervalMs  int
	SyncBatchSize   int
}

// WAL provides a durable, sequential, append-only log organized into
// fixed-size segments. Records carry a CRC32 checksum for integrity.
type WAL struct {
	cfg     WALConfig
	mu      sync.Mutex
	seg     *os.File
	segSize int64
	segSeq  int
	pending int // records since last sync

	stopCh chan struct{}
	wg     sync.WaitGroup
}

func NewWAL(cfg WALConfig) (*WAL, error) {
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("wal: mkdir: %w", err)
	}

	w := &WAL{
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}

	seq, err := w.latestSegmentSeq()
	if err != nil {
		return nil, err
	}
	w.segSeq = seq + 1

	if err := w.rotate(); err != nil {
		return nil, err
	}

	w.wg.Add(1)
	go w.syncLoop()
	return w, nil
}

// Append writes a record with the given type and payload.
func (w *WAL) Append(recordType byte, payload []byte) error {
	rec := encodeRecord(recordType, payload)

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.segSize+int64(len(rec)) > w.cfg.SegmentMaxBytes {
		if err := w.rotate(); err != nil {
			return err
		}
	}

	n, err := w.seg.Write(rec)
	if err != nil {
		return fmt.Errorf("wal: write: %w", err)
	}
	w.segSize += int64(n)
	w.pending++

	if w.pending >= w.cfg.SyncBatchSize {
		w.syncLocked()
	}
	return nil
}

// Sync forces an fdatasync of the current segment.
func (w *WAL) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.syncLocked()
}

// Close stops the background sync loop and closes the segment file.
func (w *WAL) Close() error {
	close(w.stopCh)
	w.wg.Wait()

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.seg != nil {
		w.syncLocked()
		return w.seg.Close()
	}
	return nil
}

func (w *WAL) syncLocked() error {
	if w.seg == nil || w.pending == 0 {
		return nil
	}
	w.pending = 0
	return w.seg.Sync()
}

func (w *WAL) syncLoop() {
	defer w.wg.Done()
	ticker := time.NewTicker(time.Duration(w.cfg.SyncIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.mu.Lock()
			w.syncLocked()
			w.mu.Unlock()
		}
	}
}

func (w *WAL) rotate() error {
	if w.seg != nil {
		w.syncLocked()
		w.seg.Close()
	}
	name := filepath.Join(w.cfg.Dir, fmt.Sprintf("segment-%010d.wal", w.segSeq))
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("wal: open segment: %w", err)
	}
	w.seg = f
	w.segSize = 0
	w.segSeq++
	return nil
}

func (w *WAL) latestSegmentSeq() (int, error) {
	entries, err := os.ReadDir(w.cfg.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	maxSeq := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "segment-") || !strings.HasSuffix(e.Name(), ".wal") {
			continue
		}
		var seq int
		if _, err := fmt.Sscanf(e.Name(), "segment-%d.wal", &seq); err == nil {
			if seq > maxSeq {
				maxSeq = seq
			}
		}
	}
	return maxSeq, nil
}

func segmentPath(dir string) string {
	return dir
}

// encodeRecord builds a binary record: [CRC32(4) | Length(4) | Type(1) | Payload(N)]
func encodeRecord(recordType byte, payload []byte) []byte {
	plen := len(payload)
	rec := make([]byte, headerSize+plen)

	binary.LittleEndian.PutUint32(rec[4:8], uint32(plen))
	rec[8] = recordType
	copy(rec[9:], payload)

	checksum := crc32.ChecksumIEEE(rec[4:])
	binary.LittleEndian.PutUint32(rec[0:4], checksum)
	return rec
}

// DecodeRecord reads one record from buf, returning (type, payload, bytesConsumed, error).
func DecodeRecord(buf []byte) (byte, []byte, int, error) {
	if len(buf) < headerSize {
		return 0, nil, 0, fmt.Errorf("wal: short record header")
	}

	storedCRC := binary.LittleEndian.Uint32(buf[0:4])
	plen := binary.LittleEndian.Uint32(buf[4:8])
	recordType := buf[8]

	total := headerSize + int(plen)
	if len(buf) < total {
		return 0, nil, 0, fmt.Errorf("wal: short record payload")
	}

	computedCRC := crc32.ChecksumIEEE(buf[4:total])
	if storedCRC != computedCRC {
		return 0, nil, 0, fmt.Errorf("wal: CRC mismatch (stored=%x computed=%x)", storedCRC, computedCRC)
	}

	payload := make([]byte, plen)
	copy(payload, buf[headerSize:total])
	return recordType, payload, total, nil
}

// SegmentFiles returns all WAL segment paths in order.
func SegmentFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "segment-") && strings.HasSuffix(e.Name(), ".wal") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}
