package mpq

import (
	"bytes"
	"compress/bzip2"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/JoshVarga/blast"
)

const (
	archiveMagic uint32 = 0x1A51504D
	fileImplode  uint32 = 0x00000100
	fileCompress uint32 = 0x00000200
	filePKWare   uint32 = 0x00000008
	fileSparse   uint32 = 0x00000020
	fileEncrypt  uint32 = 0x00010000
	fileFixKey   uint32 = 0x00020000
	fileSingle   uint32 = 0x01000000
	fileExists   uint32 = 0x80000000
	sectorSize   uint32 = 512
)

type header struct {
	HeaderSize         uint32
	ArchiveSize        uint32
	FormatVersion      uint16
	BlockSize          uint16
	HashTablePos       uint64
	BlockTablePos      uint64
	ExtendedBlockTable uint64
	ArchiveOffset      uint64
	HashEntries        uint32
	BlockEntries       uint32
}

type hashEntry struct {
	NameA    uint32
	NameB    uint32
	Locale   uint16
	Platform uint8
	Block    uint32
}

type blockEntry struct {
	RelativePos    uint64
	FilePos        uint64
	CompressedSize uint32
	FileSize       uint32
	Flags          uint32
}

type Archive struct {
	path   string
	file   *os.File
	header header
	hashes []hashEntry
	blocks []blockEntry
}

var cryptTable [0x500]uint32
var cryptOnce sync.Once

func Archives(input string) ([]string, error) {
	info, err := os.Stat(input)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if !strings.EqualFold(filepath.Ext(input), ".mpq") {
			return nil, fmt.Errorf("input is not an MPQ archive: %s", input)
		}
		return []string{input}, nil
	}
	archives := make([]string, 0)
	err = filepath.WalkDir(input, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".mpq") {
			archives = append(archives, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(archives)
	return archives, nil
}

func Open(path string) (*Archive, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	archive := &Archive{path: path, file: file}
	if err := archive.readHeader(); err != nil {
		file.Close()
		return nil, err
	}
	if err := archive.readTables(); err != nil {
		file.Close()
		return nil, err
	}
	return archive, nil
}

func (a *Archive) Close() error {
	if a == nil || a.file == nil {
		return nil
	}
	err := a.file.Close()
	a.file = nil
	return err
}

func (a *Archive) readHeader() error {
	var raw [32]byte
	for offset := uint64(0); ; offset += 512 {
		if _, err := a.file.ReadAt(raw[:], int64(offset)); err != nil {
			return fmt.Errorf("%s is not an MPQ archive: %w", a.path, err)
		}
		if binary.LittleEndian.Uint32(raw[:4]) != archiveMagic {
			continue
		}
		a.header = header{HeaderSize: binary.LittleEndian.Uint32(raw[4:8]), ArchiveSize: binary.LittleEndian.Uint32(raw[8:12]), FormatVersion: binary.LittleEndian.Uint16(raw[12:14]), BlockSize: binary.LittleEndian.Uint16(raw[14:16]), HashTablePos: uint64(binary.LittleEndian.Uint32(raw[16:20])) + offset, BlockTablePos: uint64(binary.LittleEndian.Uint32(raw[20:24])) + offset, ArchiveOffset: offset, HashEntries: binary.LittleEndian.Uint32(raw[24:28]), BlockEntries: binary.LittleEndian.Uint32(raw[28:32])}
		if a.header.HeaderSize < 32 || a.header.HashEntries == 0 || a.header.BlockEntries == 0 {
			return fmt.Errorf("invalid MPQ header in %s", a.path)
		}
		if a.header.FormatVersion >= 1 {
			var extended [12]byte
			if _, err := a.file.ReadAt(extended[:], int64(offset)+32); err != nil {
				return fmt.Errorf("invalid MPQ extended header in %s: %w", a.path, err)
			}
			a.header.ExtendedBlockTable = binary.LittleEndian.Uint64(extended[:8]) + offset
			a.header.HashTablePos += uint64(binary.LittleEndian.Uint16(extended[8:10])) << 32
			a.header.BlockTablePos += uint64(binary.LittleEndian.Uint16(extended[10:12])) << 32
		}
		return nil
	}
}

func (a *Archive) readTables() error {
	cryptOnce.Do(initCryptTable)
	hashBytes, err := a.readAt(a.header.HashTablePos, uint64(a.header.HashEntries)*16)
	if err != nil {
		return err
	}
	decrypt(hashBytes, hashString("(hash table)", 3))
	a.hashes = make([]hashEntry, a.header.HashEntries)
	for index := range a.hashes {
		offset := index * 16
		a.hashes[index] = hashEntry{NameA: binary.LittleEndian.Uint32(hashBytes[offset:]), NameB: binary.LittleEndian.Uint32(hashBytes[offset+4:]), Locale: binary.LittleEndian.Uint16(hashBytes[offset+8:]), Platform: hashBytes[offset+10], Block: binary.LittleEndian.Uint32(hashBytes[offset+12:])}
	}
	blockBytes, err := a.readAt(a.header.BlockTablePos, uint64(a.header.BlockEntries)*16)
	if err != nil {
		return err
	}
	decrypt(blockBytes, hashString("(block table)", 3))
	a.blocks = make([]blockEntry, a.header.BlockEntries)
	for index := range a.blocks {
		offset := index * 16
		relative := uint64(binary.LittleEndian.Uint32(blockBytes[offset:]))
		a.blocks[index] = blockEntry{RelativePos: relative, FilePos: relative + a.header.ArchiveOffset, CompressedSize: binary.LittleEndian.Uint32(blockBytes[offset+4:]), FileSize: binary.LittleEndian.Uint32(blockBytes[offset+8:]), Flags: binary.LittleEndian.Uint32(blockBytes[offset+12:])}
	}
	if a.header.ExtendedBlockTable != 0 {
		extended, err := a.readAt(a.header.ExtendedBlockTable, uint64(a.header.BlockEntries)*2)
		if err != nil {
			return err
		}
		for index := range a.blocks {
			high := uint64(binary.LittleEndian.Uint16(extended[index*2:])) << 32
			a.blocks[index].RelativePos |= high
			a.blocks[index].FilePos |= high
		}
	}
	return nil
}

func (a *Archive) ListFiles() ([]string, error) {
	data, err := a.ReadFile("(listfile)")
	if err != nil {
		return nil, err
	}
	lines := strings.FieldsFunc(string(data), func(r rune) bool { return r == '\r' || r == '\n' })
	return lines, nil
}

func (a *Archive) ReadFile(name string) ([]byte, error) {
	index, ok := a.find(name)
	if !ok {
		return nil, os.ErrNotExist
	}
	if index >= uint32(len(a.blocks)) {
		return nil, errors.New("MPQ block index is out of range")
	}
	block := a.blocks[index]
	// HACK (mpq_libmpq.cpp:73): in patch.mpq some files report an unpacked
	// size of 0/1 and cannot be read; C++ treats them as missing and tries
	// the next archive, so do the same.
	if block.FileSize <= 1 {
		return nil, os.ErrNotExist
	}
	if block.Flags&fileImplode != 0 {
		data, err := a.readAt(block.FilePos, uint64(block.CompressedSize))
		if err != nil {
			return nil, err
		}
		key := uint32(0)
		if block.Flags&fileEncrypt != 0 {
			key = hashString(name, 3)
			if block.Flags&fileFixKey != 0 {
				// FIX_KEY adjusts by the absolute file position
				// (FilePos = RelativePos + ArchiveOffset), not the
				// header-relative offset.
				key = (key + uint32(block.FilePos)) ^ block.FileSize
			}
		}
		if key != 0 {
			decrypt(data, key)
		}
		return decompressImplode(data, block.FileSize)
	}
	key := uint32(0)
	if block.Flags&fileEncrypt != 0 {
		key = hashString(name, 3)
		if block.Flags&fileFixKey != 0 {
			key = (key + uint32(block.FilePos)) ^ block.FileSize
		}
	}
	if block.Flags&fileSingle != 0 {
		data, err := a.readAt(block.FilePos, uint64(block.CompressedSize))
		if err != nil {
			return nil, err
		}
		if key != 0 {
			decrypt(data, key)
		}
		return decompress(data, block.FileSize, block.Flags)
	}
	return a.readSectors(block, key)
}

func (a *Archive) readSectors(block blockEntry, key uint32) ([]byte, error) {
	sectorBytes := sectorSize << a.header.BlockSize
	sectorCount := (block.FileSize + sectorBytes - 1) / sectorBytes
	offsetBytes := (sectorCount + 1) * 4
	offsets, err := a.readAt(block.FilePos, uint64(offsetBytes))
	if err != nil {
		return nil, err
	}
	if key != 0 {
		decrypt(offsets, key-1)
	}
	result := bytes.NewBuffer(make([]byte, 0, block.FileSize))
	for index := uint32(0); index < sectorCount; index++ {
		start := binary.LittleEndian.Uint32(offsets[index*4:])
		end := binary.LittleEndian.Uint32(offsets[(index+1)*4:])
		if end < start || end > block.CompressedSize {
			return nil, errors.New("invalid MPQ sector offsets")
		}
		data, err := a.readAt(block.FilePos+uint64(start), uint64(end-start))
		if err != nil {
			return nil, err
		}
		if key != 0 {
			decrypt(data, key+index)
		}
		expected := sectorBytes
		if remaining := block.FileSize - index*sectorBytes; remaining < expected {
			expected = remaining
		}
		decoded, err := decompress(data, expected, block.Flags)
		if err != nil {
			return nil, err
		}
		result.Write(decoded)
	}
	return result.Bytes(), nil
}

func (a *Archive) find(name string) (uint32, bool) {
	if len(a.hashes) == 0 {
		return 0, false
	}
	name = normalize(name)
	start := hashString(name, 0) % uint32(len(a.hashes))
	hashA, hashB := hashString(name, 1), hashString(name, 2)
	for offset := uint32(0); offset < uint32(len(a.hashes)); offset++ {
		entry := a.hashes[(start+offset)%uint32(len(a.hashes))]
		if entry.NameA == 0xFFFFFFFF && entry.NameB == 0xFFFFFFFF {
			return 0, false
		}
		if entry.NameA == hashA && entry.NameB == hashB && (entry.Locale == 0 || entry.Locale == 0xFFFF) {
			return entry.Block, entry.Block != 0xFFFFFFFF
		}
	}
	return 0, false
}

func (a *Archive) readAt(offset, size uint64) ([]byte, error) {
	if size > uint64(^uint(0)>>1) {
		return nil, errors.New("MPQ read size exceeds addressable memory")
	}
	data := make([]byte, int(size))
	if _, err := a.file.ReadAt(data, int64(offset)); err != nil {
		return nil, err
	}
	return data, nil
}

func decompress(data []byte, expected, flags uint32) ([]byte, error) {
	if flags&fileImplode != 0 {
		return decompressImplode(data, expected)
	}
	if flags&fileCompress == 0 || uint32(len(data)) == expected {
		if uint32(len(data)) != expected {
			return nil, fmt.Errorf("MPQ file size mismatch: got %d, want %d", len(data), expected)
		}
		return data, nil
	}
	if len(data) == 0 {
		return nil, errors.New("empty compressed MPQ sector")
	}
	return decompressCompressed(data[1:], data[0], expected)
}

func decompressCompressed(data []byte, mask byte, expected uint32) ([]byte, error) {
	const supported = byte(0x01 | 0x02 | 0x08 | 0x10 | 0x20 | 0x40 | 0x80)
	if mask&^supported != 0 {
		return nil, errors.New("unsupported MPQ compression method")
	}
	methods := []byte{0x01, 0x02, 0x08, 0x10, 0x20, 0x40, 0x80}
	decoded := data
	used := false
	for _, method := range methods {
		if mask&method == 0 {
			continue
		}
		var err error
		decoded, err = decompressMethod(decoded, method, expected)
		if err != nil {
			return nil, err
		}
		used = true
	}
	if !used {
		return nil, errors.New("unsupported MPQ compression method")
	}
	if uint32(len(decoded)) != expected {
		return nil, fmt.Errorf("MPQ decompressed size mismatch: got %d, want %d", len(decoded), expected)
	}
	return decoded, nil
}

func decompressMethod(data []byte, method byte, expected uint32) ([]byte, error) {
	if method == 0x40 || method == 0x80 {
		return decompressWave(data, expected, map[byte]int{0x40: 1, 0x80: 2}[method])
	}
	var reader io.ReadCloser
	var err error
	switch method {
	case 0x01:
		return Decompress(data, int(expected))
	case 0x02:
		reader, err = zlib.NewReader(bytes.NewReader(data))
	case 0x08:
		reader, err = blast.NewReader(bytes.NewReader(data))
	case 0x10:
		reader = io.NopCloser(bzip2.NewReader(bytes.NewReader(data)))
	case 0x20:
		return decompressSparse(data, expected)
	default:
		return nil, errors.New("unsupported MPQ compression method")
	}
	if err != nil {
		return nil, err
	}
	decoded, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return decoded, nil
}

func decompressImplode(data []byte, expected uint32) ([]byte, error) {
	reader, err := blast.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	output, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if uint32(len(output)) != expected {
		return nil, fmt.Errorf("MPQ imploded size mismatch: got %d, want %d", len(output), expected)
	}
	return output, nil
}

var waveIndexAdjust = [...]int32{-1, 0, -1, 4, -1, 2, -1, 6, -1, 1, -1, 5, -1, 3, -1, 7, -1, 1, -1, 5, -1, 3, -1, 7, -1, 2, -1, 4, -1, 6, -1, 8}

var waveStepTable = [...]int32{7, 8, 9, 10, 11, 12, 13, 14, 16, 17, 19, 21, 23, 25, 28, 31, 34, 37, 41, 45, 50, 55, 60, 66, 73, 80, 88, 97, 107, 118, 130, 143, 157, 173, 190, 209, 230, 253, 279, 307, 337, 371, 408, 449, 494, 544, 598, 658, 724, 796, 876, 963, 1060, 1165, 1282, 1411, 1552, 1707, 1878, 2066, 2272, 2499, 2749, 3024, 3327, 3660, 4026, 4428, 4871, 5358, 5894, 6484, 7132, 7845, 8630, 9493, 10442, 11487, 12635, 13899, 15289, 16818, 18500, 20350, 22385, 24623, 27086, 29794, 32767}

func decompressWave(data []byte, expected uint32, channels int) ([]byte, error) {
	if channels < 1 || channels > 2 || len(data) < 2+channels*2 {
		return nil, errors.New("invalid MPQ ADPCM stream")
	}
	result := make([]byte, 0, expected)
	index := channels - 1
	indices := [2]int32{0x2C, 0x2C}
	samples := [2]int32{}
	appendSample := func(value int32) {
		if len(result)+2 > int(expected) {
			return
		}
		result = append(result, byte(value), byte(value>>8))
	}
	position := 2
	for channel := 0; channel < channels; channel++ {
		samples[channel] = int32(int16(binary.LittleEndian.Uint16(data[position:])))
		position += 2
		appendSample(samples[channel])
	}
	shift := uint(data[1])
	for position < len(data) && len(result) < int(expected) {
		value := data[position]
		position++
		if channels == 2 {
			if index == 0 {
				index = 1
			} else {
				index = 0
			}
		}
		if value&0x80 != 0 {
			switch value & 0x7F {
			case 0:
				if indices[index] > 0 {
					indices[index]--
				}
				appendSample(samples[index])
			case 1:
				indices[index] += 8
				if indices[index] > 0x58 {
					indices[index] = 0x58
				}
				if channels == 2 {
					if index == 0 {
						index = 1
					} else {
						index = 0
					}
				}
			case 2:
			default:
				indices[index] -= 8
				if indices[index] < 0 {
					indices[index] = 0
				}
				if channels == 2 {
					if index == 0 {
						index = 1
					} else {
						index = 0
					}
				}
			}
			continue
		}
		step := waveStepTable[indices[index]]
		delta := step >> shift
		if value&0x01 != 0 {
			delta += step
		}
		if value&0x02 != 0 {
			delta += step >> 1
		}
		if value&0x04 != 0 {
			delta += step >> 2
		}
		if value&0x08 != 0 {
			delta += step >> 3
		}
		if value&0x10 != 0 {
			delta += step >> 4
		}
		if value&0x20 != 0 {
			delta += step >> 5
		}
		if value&0x40 != 0 {
			samples[index] -= delta
			if samples[index] < -32768 {
				samples[index] = -32768
			}
		} else {
			samples[index] += delta
			if samples[index] > 32767 {
				samples[index] = 32767
			}
		}
		appendSample(samples[index])
		indices[index] += waveIndexAdjust[value&0x1F]
		if indices[index] < 0 {
			indices[index] = 0
		} else if indices[index] > 0x58 {
			indices[index] = 0x58
		}
	}
	if uint32(len(result)) != expected {
		return nil, fmt.Errorf("MPQ ADPCM decompressed size mismatch: got %d, want %d", len(result), expected)
	}
	return result, nil
}

func normalize(name string) string {
	return asciiUpper(strings.ReplaceAll(name, "/", "\\"))
}

// asciiUpper uppercases ASCII a-z only, matching C toupper on MPQ file
// names; strings.ToUpper would rewrite bytes >= 0x80 into U+FFFD and
// diverge from the C hash for non-ASCII names.
func asciiUpper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - ('a' - 'A')
		}
	}
	return string(b)
}

func initCryptTable() {
	seed := uint32(0x00100001)
	for index := uint32(0); index < 0x100; index++ {
		for row := uint32(0); row < 5; row++ {
			seed = (seed*125 + 3) % 0x2AAAAB
			temp1 := (seed & 0xFFFF) << 16
			seed = (seed*125 + 3) % 0x2AAAAB
			temp2 := seed & 0xFFFF
			cryptTable[row*0x100+index] = temp1 | temp2
		}
	}
}

func hashString(name string, hashType uint32) uint32 {
	cryptOnce.Do(initCryptTable)
	seed1, seed2 := uint32(0x7FED7FED), uint32(0xEEEEEEEE)
	for index := 0; index < len(name); index++ {
		value := name[index]
		if value >= 'a' && value <= 'z' {
			value -= 'a' - 'A'
		}
		seed1 = cryptTable[(hashType<<8)+uint32(value)] ^ (seed1 + seed2)
		seed2 = uint32(value) + seed1 + seed2 + (seed2 << 5) + 3
	}
	return seed1
}

func decrypt(data []byte, key uint32) {
	cryptOnce.Do(initCryptTable)
	seed := key
	seed2 := uint32(0xEEEEEEEE)
	for offset := 0; offset+4 <= len(data); offset += 4 {
		seed2 += cryptTable[0x400+(seed&0xFF)]
		value := binary.LittleEndian.Uint32(data[offset:])
		value ^= seed + seed2
		seed = ((^seed << 21) + 0x11111111) | (seed >> 11)
		seed2 = value + seed2 + (seed2 << 5) + 3
		binary.LittleEndian.PutUint32(data[offset:], value)
	}
}
