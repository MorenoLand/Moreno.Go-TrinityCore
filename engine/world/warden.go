package world

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rc4"
	"crypto/sha1"
	"database/sql"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Warden opcodes mirror TrinityCore WardenOpcodes.
const (
	// Client -> Server
	wardenCmsgModuleMissing     uint8 = 0
	wardenCmsgModuleOk          uint8 = 1
	wardenCmsgCheatChecksResult uint8 = 2
	wardenCmsgMemChecksResult   uint8 = 3
	wardenCmsgHashResult        uint8 = 4
	wardenCmsgModuleFailed      uint8 = 5

	// Server -> Client
	wardenSmsgModuleUse          uint8 = 0
	wardenSmsgModuleCache        uint8 = 1
	wardenSmsgCheatChecksRequest uint8 = 2
	wardenSmsgModuleInitialize   uint8 = 3
	wardenSmsgMemChecksRequest   uint8 = 4
	wardenSmsgHashRequest        uint8 = 5
)

// WardenActions defines the penalty action to execute upon check failure.
type WardenActions uint8

const (
	WardenActionLog  WardenActions = 0
	WardenActionKick WardenActions = 1
	WardenActionBan  WardenActions = 2
)

// WardenCheckCategory categorizes Warden checks into injection, lua sandbox, and client mod checks.
type WardenCheckCategory uint8

const (
	InjectCheckCategory WardenCheckCategory = 0
	LuaCheckCategory    WardenCheckCategory = 1
	ModdedCheckCategory WardenCheckCategory = 2
	NumCheckCategories  WardenCheckCategory = 3
)

// WardenCheckType defines the specific check mechanism.
type WardenCheckType uint8

const (
	NoneCheck    WardenCheckType = 0
	TimingCheck  WardenCheckType = 87
	DriverCheck  WardenCheckType = 113
	ProcCheck    WardenCheckType = 126
	LuaEvalCheck WardenCheckType = 139
	MpqCheck     WardenCheckType = 152
	PageCheckA   WardenCheckType = 178
	PageCheckB   WardenCheckType = 191
	ModuleCheck  WardenCheckType = 217
	MemCheck     WardenCheckType = 243
)

const (
	wardenMaxLuaCheckLength = 170
	wardenLuaEvalPrefix     = "local S,T,R=SendAddonMessage,function()"
	wardenLuaEvalMidfix     = " end R=S and T()if R then S('_TW',"
	wardenLuaEvalPostfix    = ",'GUILD')end"
)

func getWardenCheckCategory(t WardenCheckType) WardenCheckCategory {
	switch t {
	case DriverCheck, PageCheckA, PageCheckB, ModuleCheck:
		return InjectCheckCategory
	case LuaEvalCheck:
		return LuaCheckCategory
	case MpqCheck, MemCheck:
		return ModdedCheckCategory
	default:
		return NumCheckCategories
	}
}

// wardenCheck represents a single registered check from world.db `warden_checks`.
type wardenCheck struct {
	CheckId uint16
	Type    WardenCheckType
	Data    []byte
	Address uint32
	Length  uint8
	Str     string
	Comment string
	IdStr   [4]byte
	Action  WardenActions
}

// wardenCheckMgr holds the global registry of Warden checks and check results.
type wardenCheckMgr struct {
	mu           sync.RWMutex
	checks       []wardenCheck
	checkResults map[uint16][]byte
	pools        [NumCheckCategories][]uint16
}

func newWardenCheckMgr() *wardenCheckMgr {
	return &wardenCheckMgr{
		checkResults: make(map[uint16][]byte),
	}
}

func (m *wardenCheckMgr) loadChecks(ctx context.Context, worldDB *sql.DB, defaultAction uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var maxID sql.NullInt64
	row := worldDB.QueryRowContext(ctx, "SELECT MAX(id) FROM warden_checks")
	if err := row.Scan(&maxID); err != nil {
		return err
	}
	if !maxID.Valid || maxID.Int64 <= 0 {
		return nil
	}

	m.checks = make([]wardenCheck, maxID.Int64+1)
	for i := range m.pools {
		m.pools[i] = nil
	}

	rows, err := worldDB.QueryContext(ctx, "SELECT id, type, data, result, address, length, str, comment FROM warden_checks ORDER BY id ASC")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id      uint16
			chkType uint8
			data    []byte
			result  []byte
			address sql.NullInt64
			length  sql.NullInt64
			str     sql.NullString
			comment sql.NullString
		)

		if err := rows.Scan(&id, &chkType, &data, &result, &address, &length, &str, &comment); err != nil {
			return err
		}

		t := WardenCheckType(chkType)
		cat := getWardenCheckCategory(t)
		if cat == NumCheckCategories {
			continue
		}
		if t == LuaEvalCheck && id > 9999 {
			continue
		}

		chk := wardenCheck{
			CheckId: id,
			Type:    t,
			Action:  WardenActions(defaultAction),
		}

		if t == PageCheckA || t == PageCheckB || t == DriverCheck {
			chk.Data = data
		}
		if (t == MpqCheck || t == MemCheck) && len(result) > 0 {
			m.checkResults[id] = result
		}
		if t == MemCheck || t == PageCheckA || t == PageCheckB {
			if address.Valid {
				chk.Address = uint32(address.Int64)
			}
		}
		if t == PageCheckA || t == PageCheckB {
			if length.Valid {
				chk.Length = uint8(length.Int64)
			}
		}
		if t == MemCheck || t == MpqCheck || t == LuaEvalCheck || t == DriverCheck || t == ModuleCheck {
			if str.Valid {
				chk.Str = str.String
			}
		}
		if comment.Valid && comment.String != "" {
			chk.Comment = comment.String
		} else {
			chk.Comment = "Undocumented Check"
		}

		if t == LuaEvalCheck {
			if len(chk.Str) > wardenMaxLuaCheckLength {
				continue
			}
			idFormatted := fmt.Sprintf("%04d", id)
			copy(chk.IdStr[:], idFormatted)
		}

		if int(id) < len(m.checks) {
			m.checks[id] = chk
			m.pools[cat] = append(m.pools[cat], id)
		}
	}
	return rows.Err()
}

func (m *wardenCheckMgr) loadOverrides(ctx context.Context, charDB *sql.DB) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rows, err := charDB.QueryContext(ctx, "SELECT wardenId, action FROM warden_action")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var checkID uint16
		var action uint8
		if err := rows.Scan(&checkID, &action); err != nil {
			return err
		}
		if action <= uint8(WardenActionBan) && int(checkID) < len(m.checks) {
			m.checks[checkID].Action = WardenActions(action)
		}
	}
	return rows.Err()
}

func (m *wardenCheckMgr) getAvailableChecks(cat WardenCheckCategory) []uint16 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if cat >= NumCheckCategories {
		return nil
	}
	res := make([]uint16, len(m.pools[cat]))
	copy(res, m.pools[cat])
	return res
}

func (m *wardenCheckMgr) getCheckData(id uint16) (wardenCheck, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if int(id) >= len(m.checks) {
		return wardenCheck{}, false
	}
	chk := m.checks[id]
	return chk, chk.CheckId != 0
}

func (m *wardenCheckMgr) getCheckResult(id uint16) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res, ok := m.checkResults[id]
	return res, ok
}

// sessionKeyGenerator mirrors TrinityCore's SessionKeyGenerator<Trinity::Crypto::SHA1>.
type sessionKeyGenerator struct {
	o0  [20]byte
	o1  [20]byte
	o2  [20]byte
	idx int
}

func newSessionKeyGenerator(k []byte) *sessionKeyGenerator {
	gen := &sessionKeyGenerator{}
	half := len(k) / 2
	gen.o1 = sha1.Sum(k[:half])
	gen.o2 = sha1.Sum(k[half:])

	h0 := sha1.New()
	h0.Write(gen.o1[:])
	h0.Write(gen.o0[:])
	h0.Write(gen.o2[:])
	copy(gen.o0[:], h0.Sum(nil))
	gen.idx = 0
	return gen
}

func (g *sessionKeyGenerator) Generate(buf []byte) {
	for i := 0; i < len(buf); i++ {
		if g.idx >= 20 {
			h := sha1.New()
			h.Write(g.o1[:])
			h.Write(g.o0[:])
			h.Write(g.o2[:])
			copy(g.o0[:], h.Sum(nil))
			g.idx = 0
		}
		buf[i] = g.o0[g.idx]
		g.idx++
	}
}

// buildWardenChecksum mirrors Warden::BuildChecksum in TrinityCore.
// Calculates SHA-1 of data, and XORs the five 32-bit little-endian words.
func buildWardenChecksum(data []byte) uint32 {
	digest := sha1.Sum(data)
	var cs uint32
	for i := 0; i < 5; i++ {
		cs ^= binary.LittleEndian.Uint32(digest[i*4 : (i+1)*4])
	}
	return cs
}

func isValidWardenChecksum(checksum uint32, data []byte) bool {
	return checksum == buildWardenChecksum(data)
}

// wardenSession handles the Warden state machine, cryptography, and check verification for a client session.
type wardenSession struct {
	session             *session
	mu                  sync.Mutex
	inputKey            [16]byte
	outputKey           [16]byte
	seed                [16]byte
	inputCrypto         *rc4.Cipher
	outputCrypto        *rc4.Cipher
	checkTimer          time.Duration
	clientResponseTimer time.Duration
	dataSent            bool
	initialized         bool
	serverTicks         uint32
	checks              [NumCheckCategories]struct {
		ids []uint16
		idx int
	}
	currentChecks []uint16
}

func newWardenSession(s *session, sessionKey []byte) (*wardenSession, error) {
	w := &wardenSession{
		session:    s,
		checkTimer: 10 * time.Second,
		seed:       wardenWinSeed,
	}

	gen := newSessionKeyGenerator(sessionKey)
	gen.Generate(w.inputKey[:])
	gen.Generate(w.outputKey[:])

	var err error
	w.inputCrypto, err = rc4.NewCipher(w.inputKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to init input rc4: %w", err)
	}
	w.outputCrypto, err = rc4.NewCipher(w.outputKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to init output rc4: %w", err)
	}

	// Initialize categories from check manager
	if s.server != nil && s.server.wardenCheckMgr != nil {
		for cat := WardenCheckCategory(0); cat < NumCheckCategories; cat++ {
			pool := s.server.wardenCheckMgr.getAvailableChecks(cat)
			if len(pool) > 0 {
				shuffled := make([]uint16, len(pool))
				copy(shuffled, pool)
				shuffleUint16(shuffled)
				w.checks[cat].ids = shuffled
				w.checks[cat].idx = 0
			}
		}
	}

	// Send initial WARDEN_SMSG_MODULE_USE
	if err := w.sendModuleUse(); err != nil {
		return nil, fmt.Errorf("failed to send module use: %w", err)
	}

	return w, nil
}

func shuffleUint16(a []uint16) {
	for i := len(a) - 1; i > 0; i-- {
		b := make([]byte, 2)
		_, _ = rand.Read(b)
		j := int(binary.LittleEndian.Uint16(b)) % (i + 1)
		a[i], a[j] = a[j], a[i]
	}
}

// sendModuleUse sends WARDEN_SMSG_MODULE_USE (0x00) with MD5, key, and module size.
func (w *wardenSession) sendModuleUse() error {
	moduleData := getWardenWinModuleData()
	buf := protocol.NewBuffer(1 + 16 + 16 + 4)
	buf.WriteU8(wardenSmsgModuleUse)
	buf.Write(wardenWinModuleID[:])
	buf.Write(wardenWinModuleKey[:])
	buf.WriteU32(uint32(len(moduleData)))

	payload := buf.Bytes()
	w.outputCrypto.XORKeyStream(payload, payload)
	return w.session.write(uint16(protocol.OpcodeSMSG_WARDEN_DATA), payload, w.session.authed)
}

// sendModuleToClient streams the module bytes in chunks of up to 500 bytes.
func (w *wardenSession) sendModuleToClient() error {
	moduleData := getWardenWinModuleData()
	sizeLeft := len(moduleData)
	pos := 0

	for sizeLeft > 0 {
		burst := sizeLeft
		if burst > 500 {
			burst = 500
		}

		buf := protocol.NewBuffer(1 + 2 + burst)
		buf.WriteU8(wardenSmsgModuleCache)
		buf.WriteU16(uint16(burst))
		buf.Write(moduleData[pos : pos+burst])

		payload := buf.Bytes()
		w.outputCrypto.XORKeyStream(payload, payload)
		if err := w.session.write(uint16(protocol.OpcodeSMSG_WARDEN_DATA), payload, w.session.authed); err != nil {
			return err
		}

		sizeLeft -= burst
		pos += burst
	}
	return nil
}

// sendHashRequest sends WARDEN_SMSG_HASH_REQUEST (0x05) with the seed.
func (w *wardenSession) sendHashRequest() error {
	buf := protocol.NewBuffer(1 + 16)
	buf.WriteU8(wardenSmsgHashRequest)
	buf.Write(w.seed[:])

	payload := buf.Bytes()
	w.outputCrypto.XORKeyStream(payload, payload)
	return w.session.write(uint16(protocol.OpcodeSMSG_WARDEN_DATA), payload, w.session.authed)
}

// handleHashResult processes WARDEN_CMSG_HASH_RESULT (0x04) and re-keys the stream ciphers.
func (w *wardenSession) handleHashResult(data []byte) error {
	if len(data) < 20 {
		w.applyPenalty(nil, "warden hash result too short")
		return fmt.Errorf("hash result too short: %d", len(data))
	}

	var hash [20]byte
	copy(hash[:], data[:20])
	if hash != wardenWinClientKeySeedHash {
		w.applyPenalty(nil, "warden hash reply mismatch")
		return fmt.Errorf("hash reply mismatch")
	}

	// Re-key ciphers to ClientKeySeed and ServerKeySeed
	w.inputKey = wardenWinClientKeySeed
	w.outputKey = wardenWinServerKeySeed

	var err error
	w.inputCrypto, err = rc4.NewCipher(w.inputKey[:])
	if err != nil {
		return fmt.Errorf("failed to re-key input cipher: %w", err)
	}
	w.outputCrypto, err = rc4.NewCipher(w.outputKey[:])
	if err != nil {
		return fmt.Errorf("failed to re-key output cipher: %w", err)
	}

	w.initialized = true

	// Send WARDEN_SMSG_MODULE_INITIALIZE
	return w.sendModuleInitialize()
}

// sendModuleInitialize sends WARDEN_SMSG_MODULE_INITIALIZE (0x03) configuring client hooks.
func (w *wardenSession) sendModuleInitialize() error {
	part1 := make([]byte, 20)
	part1[0] = 1 // Unk1
	part1[1] = 0 // Unk2
	part1[2] = 1 // Type
	part1[3] = 0 // String_library1
	binary.LittleEndian.PutUint32(part1[4:8], 0x00024F80)
	binary.LittleEndian.PutUint32(part1[8:12], 0x000218C0)
	binary.LittleEndian.PutUint32(part1[12:16], 0x00022530)
	binary.LittleEndian.PutUint32(part1[16:20], 0x00022910)
	cs1 := buildWardenChecksum(part1)

	part2 := make([]byte, 8)
	part2[0] = 4 // Unk3
	part2[1] = 0 // Unk4
	part2[2] = 0 // String_library2
	binary.LittleEndian.PutUint32(part2[3:7], 0x00419210)
	part2[7] = 1 // Function2_set
	cs2 := buildWardenChecksum(part2)

	part3 := make([]byte, 8)
	part3[0] = 1 // Unk5
	part3[1] = 1 // Unk6
	part3[2] = 0 // String_library3
	binary.LittleEndian.PutUint32(part3[3:7], 0x0046AE20)
	part3[7] = 1 // Function3_set
	cs3 := buildWardenChecksum(part3)

	buf := protocol.NewBuffer(57)
	// Chunk 1
	buf.WriteU8(wardenSmsgModuleInitialize)
	buf.WriteU16(20)
	buf.WriteU32(cs1)
	buf.Write(part1)

	// Chunk 2
	buf.WriteU8(wardenSmsgModuleInitialize)
	buf.WriteU16(8)
	buf.WriteU32(cs2)
	buf.Write(part2)

	// Chunk 3
	buf.WriteU8(wardenSmsgModuleInitialize)
	buf.WriteU16(8)
	buf.WriteU32(cs3)
	buf.Write(part3)

	payload := buf.Bytes()
	w.outputCrypto.XORKeyStream(payload, payload)
	return w.session.write(uint16(protocol.OpcodeSMSG_WARDEN_DATA), payload, w.session.authed)
}

func getCheckPacketBaseSize(t WardenCheckType) int {
	switch t {
	case DriverCheck:
		return 1
	case LuaEvalCheck:
		return 1 + len(wardenLuaEvalPrefix) + len(wardenLuaEvalMidfix) + 4 + len(wardenLuaEvalPostfix)
	case MpqCheck:
		return 1
	case PageCheckA, PageCheckB:
		return 4 + 1
	case ModuleCheck:
		return 4 + sha1.Size
	case MemCheck:
		return 1 + 4 + 1
	default:
		return 0
	}
}

func getCheckPacketSize(c *wardenCheck) int {
	size := 1 + getCheckPacketBaseSize(c.Type)
	if c.Str != "" {
		size += len(c.Str) + 1
	}
	if len(c.Data) > 0 {
		size += len(c.Data)
	}
	return size
}

// requestChecks gathers checks from configured categories, builds the check request, and transmits it.
func (w *wardenSession) requestChecks() error {
	if w.session.server == nil || w.session.server.wardenCheckMgr == nil {
		return nil
	}
	mgr := w.session.server.wardenCheckMgr

	w.currentChecks = nil

	// Gather checks from category pools
	counts := [NumCheckCategories]uint32{
		InjectCheckCategory: w.session.server.Config.WardenNumInjectionChecks,
		LuaCheckCategory:    w.session.server.Config.WardenNumLuaSandboxChecks,
		ModdedCheckCategory: w.session.server.Config.WardenNumClientModChecks,
	}

	for cat := WardenCheckCategory(0); cat < NumCheckCategories; cat++ {
		pool := &w.checks[cat]
		n := counts[cat]
		for i := uint32(0); i < n; i++ {
			if pool.idx >= len(pool.ids) {
				if len(pool.ids) > 0 {
					shuffleUint16(pool.ids)
					pool.idx = 0
				} else {
					break
				}
			}
			w.currentChecks = append(w.currentChecks, pool.ids[pool.idx])
			pool.idx++
		}
	}

	shuffleUint16(w.currentChecks)

	// Filter checks exceeding 500 bytes
	expectedSize := 4 // 1 byte opcode + 1 byte timing unk + 1 byte timing type + 1 byte xor
	var filtered []uint16
	for _, id := range w.currentChecks {
		chk, ok := mgr.getCheckData(id)
		if !ok {
			continue
		}
		thisSize := getCheckPacketSize(&chk)
		if expectedSize+thisSize > 500 {
			continue
		}
		expectedSize += thisSize
		filtered = append(filtered, id)
	}
	w.currentChecks = filtered

	buf := protocol.NewBuffer(expectedSize)
	buf.WriteU8(wardenSmsgCheatChecksRequest)

	// 1. Strings block
	for _, id := range w.currentChecks {
		chk, _ := mgr.getCheckData(id)
		if chk.Type == LuaEvalCheck {
			strLen := len(wardenLuaEvalPrefix) + len(chk.Str) + len(wardenLuaEvalMidfix) + len(chk.IdStr) + len(wardenLuaEvalPostfix)
			buf.WriteU8(uint8(strLen))
			buf.WriteString(wardenLuaEvalPrefix)
			buf.WriteString(chk.Str)
			buf.WriteString(wardenLuaEvalMidfix)
			buf.Write(chk.IdStr[:])
			buf.WriteString(wardenLuaEvalPostfix)
		} else if chk.Str != "" {
			buf.WriteU8(uint8(len(chk.Str)))
			buf.WriteString(chk.Str)
		}
	}

	xorByte := w.inputKey[0]

	// 2. TIMING_CHECK
	buf.WriteU8(0x00)
	buf.WriteU8(uint8(TimingCheck) ^ xorByte)

	// 3. Checks block
	var index uint8 = 1
	for _, id := range w.currentChecks {
		chk, _ := mgr.getCheckData(id)
		buf.WriteU8(uint8(chk.Type) ^ xorByte)

		switch chk.Type {
		case MemCheck:
			res, _ := mgr.getCheckResult(id)
			buf.WriteU8(0x00)
			buf.WriteU32(chk.Address)
			buf.WriteU8(uint8(len(res)))

		case PageCheckA, PageCheckB:
			buf.Write(chk.Data)
			buf.WriteU32(chk.Address)
			buf.WriteU8(chk.Length)

		case MpqCheck, LuaEvalCheck:
			buf.WriteU8(index)
			index++

		case DriverCheck:
			buf.Write(chk.Data)
			buf.WriteU8(index)
			index++

		case ModuleCheck:
			var seed [4]byte
			_, _ = rand.Read(seed[:])
			buf.Write(seed[:])
			mac := hmac.New(sha1.New, seed[:])
			mac.Write([]byte(chk.Str))
			buf.Write(mac.Sum(nil))
		}
	}

	buf.WriteU8(xorByte)

	payload := buf.Bytes()
	w.outputCrypto.XORKeyStream(payload, payload)
	if err := w.session.write(uint16(protocol.OpcodeSMSG_WARDEN_DATA), payload, w.session.authed); err != nil {
		return err
	}

	w.dataSent = true
	w.serverTicks = uint32(time.Now().UnixMilli())
	return nil
}

// handleCheckResult processes WARDEN_CMSG_CHEAT_CHECKS_RESULT (0x02) from the client.
func (w *wardenSession) handleCheckResult(data []byte) error {
	w.dataSent = false
	w.clientResponseTimer = 0

	r := protocol.NewReader(data)
	length, err := r.ReadU16()
	if err != nil {
		w.applyPenalty(nil, "invalid check result packet length header")
		return err
	}

	checksum, err := r.ReadU32()
	if err != nil {
		w.applyPenalty(nil, "invalid check result checksum header")
		return err
	}

	remaining := r.Bytes()[r.Position():]
	if int(length) != len(remaining) {
		w.applyPenalty(nil, "warden packet length manipulated")
		return fmt.Errorf("length manipulated: expected %d, got %d", length, len(remaining))
	}

	if !isValidWardenChecksum(checksum, remaining) {
		w.applyPenalty(nil, "warden packet checksum failed")
		return fmt.Errorf("warden packet checksum failed")
	}

	// 1. TIMING_CHECK
	timingResult, err := r.ReadU8()
	if err != nil || timingResult == 0 {
		w.applyPenalty(nil, "warden timing check failed")
		return fmt.Errorf("timing check failed: result=%d", timingResult)
	}
	_, _ = r.ReadU32() // clientTicks

	// 2. Validate individual checks
	mgr := w.session.server.wardenCheckMgr
	var checkFailed uint16

	for _, id := range w.currentChecks {
		chk, ok := mgr.getCheckData(id)
		if !ok {
			continue
		}

		switch chk.Type {
		case MemCheck:
			memRes, err := r.ReadU8()
			if err != nil || memRes != 0 {
				checkFailed = id
				continue
			}
			expected, _ := mgr.getCheckResult(id)
			got, err := r.Read(len(expected))
			if err != nil || !bytes.Equal(got, expected) {
				checkFailed = id
				continue
			}

		case PageCheckA, PageCheckB, DriverCheck, ModuleCheck:
			res, err := r.ReadU8()
			if err != nil || res != 0xE9 {
				checkFailed = id
				continue
			}

		case LuaEvalCheck:
			res, err := r.ReadU8()
			if err == nil && res == 0 {
				strLen, err := r.ReadU8()
				if err == nil && strLen > 0 {
					_ = r.SetPosition(r.Position() + int(strLen))
				}
			}

		case MpqCheck:
			mpqRes, err := r.ReadU8()
			if err != nil || mpqRes != 0 {
				checkFailed = id
				continue
			}
			gotSha, err := r.Read(sha1.Size)
			expected, _ := mgr.getCheckResult(id)
			if err != nil || !bytes.Equal(gotSha, expected) {
				checkFailed = id
				continue
			}
		}
	}

	if checkFailed > 0 {
		chk, _ := mgr.getCheckData(checkFailed)
		w.applyPenalty(&chk, fmt.Sprintf("failed check %d (%v)", checkFailed, chk.Type))
	}

	holdOff := w.session.server.Config.WardenClientCheckHoldOff
	if holdOff < 1 {
		holdOff = 1
	}
	w.checkTimer = time.Duration(holdOff) * time.Second
	return nil
}

// processLuaCheckResponse intercepts addon messages starting with "_TW\t" to handle Lua sandbox checks.
func (w *wardenSession) processLuaCheckResponse(msg string) bool {
	const wardenToken = "_TW\t"
	if !strings.HasPrefix(msg, wardenToken) {
		return false
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	idStr := strings.TrimPrefix(msg, wardenToken)
	// C++ std::from_chars (Warden.cpp:265) parses only the leading decimal
	// digits, yields 0 when none are present, and clamps to UINT16_MAX on
	// overflow — so "_TW\t42;" resolves to check 42 (check-specific action)
	// while strconv.ParseUint would reject the whole string (configured
	// action). Mirror the C++ parse semantics exactly. Note: a DB row with
	// id 0 stays shadowed by the sparse-slot gate in getCheckData (C++ would
	// return _checks[0]); ids effectively start at 1, so the gate is inert
	// here.
	digits := idStr
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			digits = digits[:i]
			break
		}
	}
	checkID := uint16(0)
	if digits != "" {
		if n, err := strconv.ParseUint(digits, 10, 16); err == nil {
			checkID = uint16(n)
		} else {
			checkID = 0xFFFF // from_chars saturates on overflow
		}
	}
	if w.session.server != nil && w.session.server.wardenCheckMgr != nil {
		if chk, ok := w.session.server.wardenCheckMgr.getCheckData(checkID); ok {
			if chk.Type == LuaEvalCheck {
				w.applyPenalty(&chk, fmt.Sprintf("failed Lua sandbox check %d", checkID))
				return true
			}
		}
	}

	w.applyPenalty(nil, "bogus Lua check response")
	return true
}

// handleWardenData decrypts incoming CMSG_WARDEN_DATA and dispatches to appropriate handlers.
func (w *wardenSession) handleWardenData(ctx context.Context, payload []byte) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(payload) == 0 {
		return true
	}

	w.inputCrypto.XORKeyStream(payload, payload)
	opcode := payload[0]
	rest := payload[1:]

	switch opcode {
	case wardenCmsgModuleMissing:
		_ = w.sendModuleToClient()
	case wardenCmsgModuleOk:
		_ = w.sendHashRequest()
	case wardenCmsgCheatChecksResult:
		_ = w.handleCheckResult(rest)
	case wardenCmsgHashResult:
		_ = w.handleHashResult(rest)
	case wardenCmsgMemChecksResult, wardenCmsgModuleFailed:
		// Client warnings / failures
	default:
		w.session.server.Logger.Warn("unknown warden opcode received", "opcode", opcode, "size", len(payload))
	}
	return true
}

// update handles check timer ticks and client response timeouts.
func (w *wardenSession) update(diff time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.initialized {
		return
	}

	if w.dataSent {
		maxDelay := time.Duration(w.session.server.Config.WardenClientResponseDelay) * time.Second
		if maxDelay > 0 {
			if w.clientResponseTimer > maxDelay {
				w.session.server.Logger.Warn("exceeded Warden module response delay - disconnecting client",
					"account", w.session.accountName)
				// Warden::Update kicks unconditionally here (Warden::KickPlayer),
				// it is NOT routed through the configured check-fail action.
				if w.session.conn != nil {
					_ = w.session.conn.Close()
				}
				return
			}
			w.clientResponseTimer += diff
		}
	} else {
		if diff >= w.checkTimer {
			_ = w.requestChecks()
		} else {
			w.checkTimer -= diff
		}
	}
}

// applyPenalty executes the configured or check-specific penalty (Log, Kick, Ban).
func (w *wardenSession) applyPenalty(check *wardenCheck, reason string) {
	s := w.session
	if s == nil {
		return
	}

	action := WardenActionLog
	if check != nil {
		action = check.Action
	} else if s.server != nil {
		action = WardenActions(s.server.Config.WardenClientCheckFailAction)
	}

	checkID := uint16(0)
	if check != nil {
		checkID = check.CheckId
	}

	s.server.Logger.Warn("warden violation detected",
		"account", s.accountName,
		"checkId", checkID,
		"action", action,
		"reason", reason)

	switch action {
	case WardenActionKick:
		if s.conn != nil {
			_ = s.conn.Close()
		}
	case WardenActionBan:
		duration := s.server.Config.WardenBanDuration
		now := time.Now().Unix()
		unbandate := int64(0)
		if duration > 0 {
			unbandate = now + int64(duration)
		}
		banReason := "Warden Anticheat Violation"
		if check != nil && check.Comment != "" {
			banReason += ": " + check.Comment + fmt.Sprintf(" (CheckId: %d)", check.CheckId)
		}
		if s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
			_, _ = s.server.AuthStore.DB.ExecContext(context.Background(),
				"INSERT OR REPLACE INTO account_banned (id, bandate, unbandate, bannedby, banreason, active) VALUES (?, ?, ?, ?, ?, 1)",
				s.accountID, now, unbandate, "Server", banReason)
		}
		if s.conn != nil {
			_ = s.conn.Close()
		}
	case WardenActionLog:
	}
}

// forceChecks overrides the upcoming check cycle with specific check IDs for testing/debugging.
func (w *wardenSession) forceChecks(ids []uint16) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.currentChecks = make([]uint16, len(ids))
	copy(w.currentChecks, ids)
}

func (w *wardenSession) hasDataSent() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.dataSent
}
