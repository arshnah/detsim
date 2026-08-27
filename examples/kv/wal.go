package kv

import (
	"encoding/binary"
	"hash/crc32"

	"github.com/arshnah/detsim"
)

// Store is a WAL-based key-value store backed by FaultyStorage, with optional
// per-entry CRC32 checksums.
type Store struct {
	storage   *detsim.FaultyStorage
	checksums bool
	writeAt   int64
	data      map[string][]byte
}

// NewStore creates a Store. If checksums is true, each WAL entry includes a CRC32
// checksum that catches torn writes and byte corruption during recovery.
func NewStore(storage *detsim.FaultyStorage, checksums bool) *Store {
	return &Store{
		storage:   storage,
		checksums: checksums,
		data:      make(map[string][]byte),
	}
}

func encodeEntry(key, value []byte, withChecksum bool) []byte {
	header := 4 + 4
	if withChecksum {
		header += 4
	}
	buf := make([]byte, header+len(key)+len(value))
	off := 0
	binary.BigEndian.PutUint32(buf[off:], uint32(len(key)))
	off += 4
	binary.BigEndian.PutUint32(buf[off:], uint32(len(value)))
	off += 4
	if withChecksum {
		off += 4
	}
	copy(buf[off:], key)
	off += len(key)
	copy(buf[off:], value)

	if withChecksum {
		sum := crc32.ChecksumIEEE(buf[header:])
		binary.BigEndian.PutUint32(buf[8:12], sum)
	}
	return buf
}

// Put writes a key-value pair to the WAL. Returns false if the underlying storage
// is full (ErrDiskFull).
func (s *Store) Put(key, value string) (ok bool) {
	entry := encodeEntry([]byte(key), []byte(value), s.checksums)
	if _, err := s.storage.WriteAt(entry, s.writeAt); err != nil {
		return false
	}
	s.writeAt += int64(len(entry))
	s.data[key] = []byte(value)
	return true
}

// Sync commits all pending WAL writes to durable storage.
func (s *Store) Sync() { s.storage.Sync() }

// Get retrieves the value for key from the in-memory index.
func (s *Store) Get(key string) (string, bool) {
	v, ok := s.data[key]
	return string(v), ok
}

// Len returns the number of keys currently in the store.
func (s *Store) Len() int { return len(s.data) }

// Recover replays the WAL from storage up to maxOffset, rebuilding the in-memory
// index. Entries with valid checksums are accepted; torn or corrupted entries stop
// the scan.
func (s *Store) Recover(maxOffset int64) {
	s.data = make(map[string][]byte)
	s.writeAt = 0

	header := 8
	if s.checksums {
		header = 12
	}

	off := int64(0)
	for off+int64(header) <= maxOffset {
		hbuf := make([]byte, header)
		n, _ := s.storage.ReadAt(hbuf, off)
		if n < header {
			break
		}
		keyLen := binary.BigEndian.Uint32(hbuf[0:4])
		valLen := binary.BigEndian.Uint32(hbuf[4:8])
		var wantSum uint32
		if s.checksums {
			wantSum = binary.BigEndian.Uint32(hbuf[8:12])
		}

		total := int64(header) + int64(keyLen) + int64(valLen)
		if keyLen > 1<<20 || valLen > 1<<20 || off+total > maxOffset {
			break
		}

		body := make([]byte, keyLen+valLen)
		n, _ = s.storage.ReadAt(body, off+int64(header))
		if int64(n) < int64(keyLen+valLen) {
			break
		}

		if s.checksums {
			gotSum := crc32.ChecksumIEEE(body)
			if gotSum != wantSum {
				break
			}
		}

		key := string(body[:keyLen])
		value := make([]byte, valLen)
		copy(value, body[keyLen:])
		s.data[key] = value
		off += total
	}
	s.writeAt = off
}
