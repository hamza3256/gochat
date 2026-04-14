package persistence

import (
	"fmt"
	"os"
)

// Reader replays WAL segments in order, invoking the callback for
// each valid record. Corrupted records are skipped (logged to stderr).
type Reader struct {
	dir string
}

func NewReader(dir string) *Reader {
	return &Reader{dir: dir}
}

// Replay iterates over all segments in order, calling fn for each
// valid record. It returns the total number of records processed.
func (r *Reader) Replay(fn func(recordType byte, payload []byte) error) (int, error) {
	files, err := SegmentFiles(r.dir)
	if err != nil {
		return 0, fmt.Errorf("wal reader: list segments: %w", err)
	}

	total := 0
	for _, path := range files {
		n, err := r.replaySegment(path, fn)
		total += n
		if err != nil {
			return total, fmt.Errorf("wal reader: segment %s: %w", path, err)
		}
	}
	return total, nil
}

func (r *Reader) replaySegment(path string, fn func(byte, []byte) error) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	count := 0
	offset := 0
	for offset < len(data) {
		recordType, payload, consumed, err := DecodeRecord(data[offset:])
		if err != nil {
			// Skip corrupted tail (common after crash)
			break
		}
		offset += consumed

		if err := fn(recordType, payload); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
