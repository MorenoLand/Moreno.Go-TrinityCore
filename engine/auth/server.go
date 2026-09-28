package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/config"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/crypto"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/iplocation"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocoltrace"
)

const (
	logonChallenge        byte   = 0x00
	logonProof            byte   = 0x01
	reconnectChallenge    byte   = 0x02
	reconnectProof        byte   = 0x03
	realmList             byte   = 0x10
	wowSuccess            byte   = 0x00
	wowBanned             byte   = 0x03
	wowUnknownAccount     byte   = 0x04
	wowDBBusy             byte   = 0x08
	wowVersionInvalid     byte   = 0x09
	wowSuspended          byte   = 0x0c
	wowLockedEnforced     byte   = 0x10
	wowUnlockableLock     byte   = 0x19
	preBCMaxBuild         uint32 = 6141
	realmFlagOffline      uint32 = 0x02
	realmFlagSpecifyBuild uint32 = 0x04
)

var versionChallenge = [16]byte{0xBA, 0xA3, 0x1E, 0x99, 0xA0, 0x0B, 0x21, 0x57, 0xFC, 0x37, 0x3F, 0xB3, 0x69, 0xCD, 0xD2, 0xF1}

type Server struct {
	Store              *database.Store
	Logger             *slog.Logger
	RealmID            uint32
	RealmAddress       string
	WrongPassMaxCount  uint32
	WrongPassBanTime   uint32
	WrongPassBanType   bool
	WrongPassLogging   bool
	StrictVersionCheck bool
	ipLocations        *iplocation.Store
	TraceRecorder      *protocoltrace.Recorder
}

type account struct {
	ID           uint32
	Login        string
	Locked       bool
	LockCountry  string
	LastIP       string
	FailedLogins uint32
	Security     uint8
	Banned       bool
	PermanentBan bool
	TotpSecret   []byte
	Salt         [crypto.SRP6SaltLength]byte
	Verifier     [crypto.SRP6VerifierLength]byte
}

type buildInfo struct {
	Build       uint32
	Major       uint8
	Minor       uint8
	Bugfix      uint8
	WindowsHash []byte
	MacHash     []byte
}

type realm struct {
	ID                   uint32
	Name                 string
	Address              string
	Port                 uint16
	Icon                 uint8
	Flags                uint32
	Timezone             uint8
	AllowedSecurityLevel uint8
	Population           float32
	Build                uint32
}

type session struct {
	server         *Server
	conn           net.Conn
	status         byte
	account        account
	srp            *crypto.SRP6
	buildInfo      *buildInfo
	sessionKey     [crypto.SRP6SessionKeyLength]byte
	reconnectProof [16]byte
	build          uint32
	postBC         bool
	login          string
	remoteIP       string
	os             string
	locale         uint8
	totpRequired   bool
}

const (
	statusChallenge byte = iota
	statusProof
	statusReconnectProof
	statusAuthed
)

func NewServer(store *database.Store, logger *slog.Logger, realmID uint32, settings ...config.Config) *Server {
	address := ""
	server := &Server{Store: store, Logger: logger, RealmID: realmID, WrongPassBanTime: 600}
	if len(settings) != 0 {
		address = settings[0].RealmAddress
		server.WrongPassMaxCount = settings[0].WrongPassMaxCount
		server.WrongPassBanTime = settings[0].WrongPassBanTime
		server.WrongPassBanType = settings[0].WrongPassBanType
		server.WrongPassLogging = settings[0].WrongPassLogging
		server.StrictVersionCheck = settings[0].StrictVersionCheck
		if settings[0].IPLocationFile != "" {
			locations, err := iplocation.Load(settings[0].IPLocationFile)
			if err != nil {
				if logger != nil {
					logger.Error("IP location file load failed", "path", settings[0].IPLocationFile, "error", err)
				}
			} else {
				server.ipLocations = locations
			}
		}
	}
	if address == "" && store.Backend == database.BackendSQLite {
		address = "127.0.0.1"
	}
	server.RealmAddress = address
	return server
}

func (s *Server) Handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closed:
		}
	}()
	defer close(closed)
	remoteIP := remoteAddress(conn)
	if banned, err := s.ipBanned(ctx, remoteIP); err != nil {
		s.debug("ip ban lookup failed", "remote", remoteIP, "error", err)
	} else if banned {
		s.debug("authentication connection rejected", "remote", remoteIP, "reason", "ip ban")
		_ = writePacket(conn, []byte{logonChallenge, 0, wowBanned})
		return
	}
	state := &session{server: s, conn: conn, status: statusChallenge, remoteIP: remoteIP}
	var traced *traceConn
	if s.TraceRecorder != nil {
		traced = &traceConn{Conn: conn, recorder: s.TraceRecorder}
		state.conn = traced
	}
	s.debug("authentication connection accepted", "remote", remoteIP)
	for {
		cmd := []byte{0}
		if _, err := io.ReadFull(conn, cmd); err != nil {
			return
		}
		if traced != nil {
			traced.begin(cmd[0])
		}
		var err error
		switch cmd[0] {
		case logonChallenge:
			err = state.handleLogonChallenge(ctx)
		case logonProof:
			err = state.handleLogonProof(ctx)
		case realmList:
			err = state.handleRealmList(ctx)
		case reconnectChallenge:
			err = state.handleReconnectChallenge(ctx)
		case reconnectProof:
			err = state.handleReconnectProof(ctx)
		default:
			if traced != nil {
				traced.end()
			}
			return
		}
		if traced != nil {
			traced.end()
		}
		if err != nil {
			if s.Logger != nil {
				s.Logger.Debug("authentication session closed", "error", err)
			}
			return
		}
	}
}

func (s *Server) ipBanned(ctx context.Context, ip string) (bool, error) {
	if s == nil || s.Store == nil {
		return false, nil
	}
	row, err := s.Store.QueryRowStatement(ctx, "LOGIN_SEL_IP_INFO", ip)
	if err != nil {
		return false, err
	}
	var banned bool
	var country sql.NullString
	if err := row.Scan(&banned, &country); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return banned, nil
}

func (s *session) handleLogonChallenge(ctx context.Context) error {
	if s.status != statusChallenge {
		return errors.New("unexpected logon challenge")
	}
	build, login, osName, locale, err := readChallenge(s.conn)
	if err != nil {
		return err
	}
	login = strings.TrimSpace(strings.ToUpper(login))
	s.build = build
	s.postBC = s.build > preBCMaxBuild
	s.login = login
	s.os = osName
	s.locale = locale
	s.debug("logon challenge received", "account", login, "build", build, "remote", s.remoteIP)
	loaded, err := loadAccount(ctx, s.server.Store, s.login, s.remoteIP)
	if err != nil {
		return err
	}
	if loaded == nil {
		s.debug("logon rejected", "account", s.login, "reason", "unknown account")
		return writePacket(s.conn, []byte{logonChallenge, 0, wowUnknownAccount})
	}
	s.account = *loaded
	if s.account.Locked && s.account.LastIP != s.remoteIP {
		s.debug("logon rejected", "account", s.login, "reason", "ip lock")
		return writePacket(s.conn, []byte{logonChallenge, 0, wowLockedEnforced})
	}
	country := s.server.ipLocations.Country(s.remoteIP)
	if iplocation.CountryLockMismatch(s.account.Locked, s.account.LockCountry, country) {
		s.debug("logon rejected", "account", s.login, "reason", "country lock", "country", country)
		return writePacket(s.conn, []byte{logonChallenge, 0, wowUnlockableLock})
	}
	if s.account.Banned {
		s.debug("logon rejected", "account", s.login, "reason", "account ban", "permanent", s.account.PermanentBan)
		result := wowSuspended
		if s.account.PermanentBan {
			result = wowBanned
		}
		return writePacket(s.conn, []byte{logonChallenge, 0, result})
	}
	info, err := loadBuildInfo(ctx, s.server.Store, s.build)
	if err != nil {
		return err
	}
	if info == nil {
		s.debug("logon rejected", "account", s.login, "reason", "unsupported build", "build", s.build)
		return writePacket(s.conn, []byte{logonChallenge, 0, wowVersionInvalid})
	}
	s.buildInfo = info
	s.srp, err = crypto.NewSRP6(s.account.Login, s.account.Salt, s.account.Verifier)
	if err != nil {
		return err
	}
	packet := protocol.NewBuffer(96)
	packet.WriteU8(logonChallenge)
	packet.WriteU8(0)
	packet.WriteU8(wowSuccess)
	packet.Write(s.srp.B[:])
	packet.WriteU8(1)
	packet.Write(crypto.SRP6GeneratorBytes())
	packet.WriteU8(crypto.SRP6EphemeralLength)
	modulus := crypto.SRP6ModulusBytes()
	packet.Write(modulus[:])
	packet.Write(s.account.Salt[:])
	packet.Write(versionChallenge[:])
	if len(s.account.TotpSecret) != 0 {
		s.totpRequired = true
		packet.WriteU8(4)
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	s.status = statusProof
	return writePacket(s.conn, packet.Bytes())
}

func (s *session) handleLogonProof(ctx context.Context) error {
	if s.status != statusProof || s.srp == nil {
		return errors.New("unexpected logon proof")
	}
	data := make([]byte, 74)
	if _, err := io.ReadFull(s.conn, data); err != nil {
		return err
	}
	var A [crypto.SRP6EphemeralLength]byte
	var clientM [20]byte
	copy(A[:], data[:32])
	copy(clientM[:], data[32:52])
	if data[73]&0x04 != 0 {
		size := []byte{0}
		if _, err := io.ReadFull(s.conn, size); err != nil {
			return err
		}
		tokenData := make([]byte, size[0])
		if _, err := io.ReadFull(s.conn, tokenData); err != nil {
			return err
		}
		token, parseErr := strconv.ParseUint(strings.TrimSpace(string(tokenData)), 10, 32)
		if !s.totpRequired || parseErr != nil || !crypto.ValidateTOTP(s.account.TotpSecret, uint32(token), time.Now()) {
			s.debug("logon proof rejected", "account", s.account.Login, "reason", "invalid totp")
			_ = writePacket(s.conn, []byte{logonProof, wowUnknownAccount, 0, 0})
			return errors.New("invalid authentication token")
		}
	} else if s.totpRequired {
		s.debug("logon proof rejected", "account", s.account.Login, "reason", "missing totp")
		_ = writePacket(s.conn, []byte{logonProof, wowUnknownAccount, 0, 0})
		return errors.New("missing authentication token")
	}
	key, ok, err := s.srp.VerifyChallengeResponse(A, clientM)
	if err != nil {
		return err
	}
	if !ok {
		s.debug("logon proof rejected", "account", s.account.Login, "reason", "invalid srp6 proof")
		_ = writePacket(s.conn, []byte{logonProof, wowUnknownAccount, 0, 0})
		s.recordFailedLogin(ctx)
		return errors.New("invalid SRP6 proof")
	}
	if !s.verifyVersionProof(A[:], data[52:72], false) {
		s.debug("logon proof rejected", "account", s.account.Login, "reason", "invalid version proof")
		_ = writePacket(s.conn, []byte{logonProof, wowVersionInvalid})
		return errors.New("invalid version proof")
	}
	s.sessionKey = key
	if err := updateAuthenticatedAccount(ctx, s.server.Store, s.account.Login, s.sessionKey[:], s.remoteIP, s.locale, s.os); err != nil {
		return err
	}
	m2 := crypto.SessionVerifier(A, clientM, s.sessionKey)
	packet := protocol.NewBuffer(32)
	packet.WriteU8(logonProof)
	packet.WriteU8(wowSuccess)
	packet.Write(m2[:])
	packet.WriteU32(0x00800000)
	packet.WriteU32(0)
	packet.WriteU16(0)
	if err := writePacket(s.conn, packet.Bytes()); err != nil {
		return err
	}
	s.status = statusAuthed
	s.debug("logon authenticated", "account", s.account.Login, "remote", s.remoteIP)
	return nil
}

func (s *session) recordFailedLogin(ctx context.Context) {
	if s == nil || s.server == nil || s.server.Store == nil {
		return
	}
	if s.server.WrongPassLogging {
		_, _ = s.server.Store.ExecStatement(ctx, "LOGIN_INS_FALP_IP_LOGGING", s.account.ID, s.remoteIP, "Login to WoW Failed - Incorrect Password")
	}
	if s.server.WrongPassMaxCount == 0 {
		return
	}
	_, _ = s.server.Store.ExecStatement(ctx, "LOGIN_UPD_FAILEDLOGINS", s.account.Login)
	s.account.FailedLogins++
	if s.account.FailedLogins < s.server.WrongPassMaxCount {
		return
	}
	banTime := s.server.WrongPassBanTime
	if banTime == 0 {
		banTime = 600
	}
	if s.server.WrongPassBanType {
		_, _ = s.server.Store.ExecStatement(ctx, "LOGIN_INS_ACCOUNT_AUTO_BANNED", s.account.ID, banTime)
	} else {
		_, _ = s.server.Store.ExecStatement(ctx, "LOGIN_INS_IP_AUTO_BANNED", s.remoteIP, banTime)
	}
}

func (s *session) handleReconnectChallenge(ctx context.Context) error {
	if s.status != statusChallenge {
		return errors.New("unexpected reconnect challenge")
	}
	build, login, osName, locale, err := readChallenge(s.conn)
	if err != nil {
		return err
	}
	login = strings.TrimSpace(strings.ToUpper(login))
	s.build = build
	s.postBC = build > preBCMaxBuild
	s.login = login
	s.os = osName
	s.locale = locale
	row, err := s.server.Store.QueryRowStatement(ctx, "LOGIN_SEL_RECONNECTCHALLENGE", login)
	if err != nil {
		return err
	}
	var loaded account
	var locked, failed, banned, permanent, security uint64
	var sessionKey []byte
	if err := row.Scan(&loaded.ID, &loaded.Login, &locked, &loaded.LockCountry, &loaded.LastIP, &failed, &banned, &permanent, &security, &sessionKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return writePacket(s.conn, []byte{reconnectChallenge, wowUnknownAccount})
		}
		return err
	}
	loaded.Locked = locked != 0
	loaded.FailedLogins = uint32(failed)
	loaded.Banned = banned != 0
	loaded.PermanentBan = permanent != 0
	loaded.Security = uint8(security)
	if len(sessionKey) != crypto.SRP6SessionKeyLength {
		s.debug("reconnect rejected", "account", login, "reason", "missing session key")
		return writePacket(s.conn, []byte{reconnectChallenge, wowUnknownAccount})
	}
	copy(s.sessionKey[:], sessionKey)
	if _, err := io.ReadFull(rand.Reader, s.reconnectProof[:]); err != nil {
		return err
	}
	s.account = loaded
	packet := protocol.NewBuffer(34)
	packet.WriteU8(reconnectChallenge)
	packet.WriteU8(wowSuccess)
	packet.Write(s.reconnectProof[:])
	packet.Write(versionChallenge[:])
	s.status = statusReconnectProof
	return writePacket(s.conn, packet.Bytes())
}

func (s *session) handleReconnectProof(ctx context.Context) error {
	if s.status != statusReconnectProof {
		return errors.New("unexpected reconnect proof")
	}
	data := make([]byte, 57)
	if _, err := io.ReadFull(s.conn, data); err != nil {
		return err
	}
	var r1 [16]byte
	var r2 [20]byte
	copy(r1[:], data[:16])
	copy(r2[:], data[16:36])
	h := sha1.New()
	_, _ = h.Write([]byte(s.account.Login))
	_, _ = h.Write(r1[:])
	_, _ = h.Write(s.reconnectProof[:])
	_, _ = h.Write(s.sessionKey[:])
	if subtle.ConstantTimeCompare(h.Sum(nil), r2[:]) != 1 {
		s.debug("reconnect proof rejected", "account", s.account.Login, "reason", "invalid proof")
		return errors.New("invalid reconnect proof")
	}
	if !s.verifyVersionProof(r1[:], data[36:56], true) {
		s.debug("reconnect proof rejected", "account", s.account.Login, "reason", "invalid version proof")
		_ = writePacket(s.conn, []byte{reconnectProof, wowVersionInvalid})
		return errors.New("invalid reconnect version proof")
	}
	if err := updateAuthenticatedAccount(ctx, s.server.Store, s.account.Login, s.sessionKey[:], s.remoteIP, s.locale, s.os); err != nil {
		return err
	}
	if err := writePacket(s.conn, []byte{reconnectProof, wowSuccess, 0, 0}); err != nil {
		return err
	}
	s.status = statusAuthed
	s.debug("reconnect authenticated", "account", s.account.Login, "remote", s.remoteIP)
	return nil
}

func (s *session) handleRealmList(ctx context.Context) error {
	if s.status != statusAuthed {
		return errors.New("unexpected realm list request")
	}
	padding := make([]byte, 4)
	if _, err := io.ReadFull(s.conn, padding); err != nil {
		return err
	}
	realms, err := loadRealms(ctx, s.server.Store, s.server.RealmAddress)
	if err != nil {
		return err
	}
	counts, err := loadCharacterCounts(ctx, s.server.Store, s.account.ID)
	if err != nil {
		return err
	}
	builds, err := loadBuilds(ctx, s.server.Store)
	if err != nil {
		return err
	}
	payload := protocol.NewBuffer(256)
	count := 0
	for _, r := range realms {
		build, exists := builds[r.Build]
		compatible := (s.postBC && r.Build == s.build) || (!s.postBC && r.Build <= preBCMaxBuild)
		if !compatible && !exists {
			continue
		}
		flags := r.Flags
		if !compatible {
			flags |= realmFlagOffline | realmFlagSpecifyBuild
		}
		if !exists {
			flags &^= realmFlagSpecifyBuild
		}
		name := r.Name
		if !s.postBC && flags&realmFlagSpecifyBuild != 0 {
			name = fmt.Sprintf("%s (%d.%d.%d)", name, build.Major, build.Minor, build.Bugfix)
		}
		lock := uint8(0)
		if r.AllowedSecurityLevel > s.account.Security {
			lock = 1
		}
		address := net.JoinHostPort(r.Address, strconv.Itoa(int(r.Port)))
		payload.WriteU8(r.Icon)
		if s.postBC {
			payload.WriteU8(lock)
		}
		payload.WriteU8(uint8(flags))
		payload.WriteCString(name)
		payload.WriteCString(address)
		payload.WriteF32(r.Population)
		payload.WriteU8(uint8(counts[r.ID]))
		payload.WriteU8(r.Timezone)
		if s.postBC {
			payload.WriteU8(uint8(r.ID))
		} else {
			payload.WriteU8(0)
		}
		if s.postBC && flags&realmFlagSpecifyBuild != 0 {
			payload.WriteU8(build.Major)
			payload.WriteU8(build.Minor)
			payload.WriteU8(build.Bugfix)
			payload.WriteU16(uint16(build.Build))
		}
		count++
	}
	if s.postBC {
		payload.WriteU8(0x10)
		payload.WriteU8(0)
	} else {
		payload.WriteU8(0)
		payload.WriteU8(2)
	}
	header := protocol.NewBuffer(payload.Len() + 8)
	header.WriteU8(realmList)
	header.WriteU16(uint16(payload.Len() + 6))
	header.WriteU32(0)
	if s.postBC {
		header.WriteU16(uint16(count))
	} else {
		header.WriteU32(uint32(count))
	}
	header.Write(payload.Bytes())
	return writePacket(s.conn, header.Bytes())
}

func loadAccount(ctx context.Context, store *database.Store, login, remoteIP string) (*account, error) {
	row, err := store.QueryRowStatement(ctx, "LOGIN_SEL_LOGONCHALLENGE", login)
	if err != nil {
		return nil, err
	}
	var result account
	var locked, failed, banned, permanent, security uint64
	var totp []byte
	var salt, verifier []byte
	if err := row.Scan(&result.ID, &result.Login, &locked, &result.LockCountry, &result.LastIP, &failed, &banned, &permanent, &security, &totp, &salt, &verifier); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	result.Locked = locked != 0
	result.FailedLogins = uint32(failed)
	result.Banned = banned != 0
	result.PermanentBan = permanent != 0
	result.Security = uint8(security)
	result.TotpSecret = totp
	result.Login = strings.ToUpper(result.Login)
	if len(salt) != crypto.SRP6SaltLength || len(verifier) != crypto.SRP6VerifierLength {
		return nil, errors.New("account SRP6 data has invalid length")
	}
	copy(result.Salt[:], salt)
	copy(result.Verifier[:], verifier)
	if result.Locked && result.LastIP != remoteIP {
		return &result, nil
	}
	return &result, nil
}

func accountBanned(ctx context.Context, store *database.Store, id uint32) (bool, error) {
	var count int64
	err := store.DB.QueryRowContext(ctx, "SELECT COUNT(1) FROM account_banned WHERE id = ? AND active = 1 AND (unbandate > ? OR unbandate = bandate)", id, time.Now().Unix()).Scan(&count)
	return count != 0, err
}

func updateAuthenticatedAccount(ctx context.Context, store *database.Store, login string, sessionKey []byte, ip string, locale uint8, osName string) error {
	_, err := store.ExecStatement(ctx, "LOGIN_UPD_LOGONPROOF", sessionKey, ip, locale, osName, login)
	return err
}

func loadBuildInfo(ctx context.Context, store *database.Store, build uint32) (*buildInfo, error) {
	var result buildInfo
	var winHash, macHash sql.NullString
	err := store.DB.QueryRowContext(ctx, "SELECT build, majorVersion, minorVersion, bugfixVersion, winChecksumSeed, macChecksumSeed FROM build_info WHERE build = ? LIMIT 1", build).Scan(&result.Build, &result.Major, &result.Minor, &result.Bugfix, &winHash, &macHash)
	if err != nil && (strings.Contains(strings.ToLower(err.Error()), "no such column") || strings.Contains(strings.ToLower(err.Error()), "unknown column")) {
		err = store.DB.QueryRowContext(ctx, "SELECT build, majorVersion, minorVersion, bugfixVersion FROM build_info WHERE build = ? LIMIT 1", build).Scan(&result.Build, &result.Major, &result.Minor, &result.Bugfix)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	result.WindowsHash = decodeVersionHash(winHash)
	result.MacHash = decodeVersionHash(macHash)
	return &result, err
}

func decodeVersionHash(value sql.NullString) []byte {
	if !value.Valid || value.String == "" {
		return nil
	}
	if decoded, err := hex.DecodeString(strings.TrimSpace(value.String)); err == nil && len(decoded) == sha1.Size {
		return decoded
	}
	if len(value.String) == sha1.Size {
		return []byte(value.String)
	}
	return nil
}

func (s *session) verifyVersionProof(input, proof []byte, reconnect bool) bool {
	if s == nil || s.server == nil || !s.server.StrictVersionCheck {
		return true
	}
	versionHash := make([]byte, sha1.Size)
	if !reconnect {
		if s.buildInfo == nil {
			return false
		}
		switch s.os {
		case "Win":
			copy(versionHash, s.buildInfo.WindowsHash)
		case "OSX":
			copy(versionHash, s.buildInfo.MacHash)
		default:
			return false
		}
		if len(s.buildInfo.WindowsHash) == 0 && len(s.buildInfo.MacHash) == 0 {
			return true
		}
		allZero := true
		for _, value := range versionHash {
			if value != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			return true
		}
	}
	hash := sha1.New()
	_, _ = hash.Write(input)
	_, _ = hash.Write(versionHash)
	return len(proof) == sha1.Size && subtle.ConstantTimeCompare(hash.Sum(nil), proof) == 1
}

func loadBuilds(ctx context.Context, store *database.Store) (map[uint32]buildInfo, error) {
	rows, err := store.DB.QueryContext(ctx, "SELECT build, majorVersion, minorVersion, bugfixVersion FROM build_info")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[uint32]buildInfo)
	for rows.Next() {
		var info buildInfo
		if err := rows.Scan(&info.Build, &info.Major, &info.Minor, &info.Bugfix); err != nil {
			return nil, err
		}
		result[info.Build] = info
	}
	return result, rows.Err()
}

func loadRealms(ctx context.Context, store *database.Store, advertisedAddress string) ([]realm, error) {
	rows, err := store.DB.QueryContext(ctx, "SELECT id, name, address, port, icon, flag, timezone, allowedSecurityLevel, population, gamebuild FROM realmlist ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]realm, 0)
	for rows.Next() {
		var r realm
		var population sql.NullFloat64
		if err := rows.Scan(&r.ID, &r.Name, &r.Address, &r.Port, &r.Icon, &r.Flags, &r.Timezone, &r.AllowedSecurityLevel, &population, &r.Build); err != nil {
			return nil, err
		}
		if population.Valid {
			r.Population = float32(population.Float64)
		}
		if advertisedAddress != "" {
			r.Address = advertisedAddress
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func loadCharacterCounts(ctx context.Context, store *database.Store, accountID uint32) (map[uint32]int64, error) {
	rows, err := store.DB.QueryContext(ctx, "SELECT realmid, COUNT(*) FROM realmcharacters WHERE acctid = ? GROUP BY realmid", accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[uint32]int64)
	for rows.Next() {
		var realmID uint32
		var count int64
		if err := rows.Scan(&realmID, &count); err != nil {
			return nil, err
		}
		result[realmID] = count
	}
	return result, rows.Err()
}

func writePacket(conn net.Conn, data []byte) error {
	for len(data) != 0 {
		n, err := conn.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

func readChallenge(conn net.Conn) (uint32, string, string, uint8, error) {
	prefix := make([]byte, 3)
	if _, err := io.ReadFull(conn, prefix); err != nil {
		return 0, "", "", 0, err
	}
	size := binary.LittleEndian.Uint16(prefix[1:])
	if size < 30 || size > 46 {
		return 0, "", "", 0, errors.New("invalid logon challenge size")
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(conn, body); err != nil {
		return 0, "", "", 0, err
	}
	if len(body) < 30 || int(body[29])+30 != int(size) {
		return 0, "", "", 0, errors.New("invalid logon challenge login length")
	}
	build := uint32(binary.LittleEndian.Uint16(body[7:9]))
	login := string(body[30 : 30+body[29]])
	osName := reverseCode(string(body[13:17]))
	locale := localeID(reverseCode(string(body[17:21])))
	return build, login, osName, locale, nil
}

func reverseCode(value string) string {
	runes := []rune(value)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return strings.TrimRight(string(runes), "\x00")
}

func localeID(value string) uint8 {
	for i, code := range []string{"enUS", "koKR", "frFR", "deDE", "zhCN", "zhTW", "esES", "esMX", "ruRU", "ptBR", "itIT"} {
		if value == code {
			return uint8(i)
		}
	}
	return 0
}

func remoteAddress(conn net.Conn) string {
	address := conn.RemoteAddr().String()
	if host, _, err := net.SplitHostPort(address); err == nil {
		return host
	}
	return strings.TrimSpace(address)
}

func (s *Server) debug(message string, args ...any) {
	if s.Logger != nil {
		s.Logger.Debug(message, args...)
	}
}

func (s *session) debug(message string, args ...any) {
	s.server.debug(message, args...)
}
