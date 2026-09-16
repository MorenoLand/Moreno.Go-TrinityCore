package protocoltrace

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const pktVersion uint16 = 0x0301
const pktHeaderSize = 66
const pktPacketPrefixSize = 20
const pktOptionalDataSize = 20
const pktMaxFieldSize = 64 * 1024 * 1024
const pktClientDirection uint32 = 0x47534d43
const pktServerDirection uint32 = 0x47534d53

func LoadPKT(r io.Reader, source string) (Trace, error) {
	if r == nil {
		return Trace{}, errors.New("PKT reader is nil")
	}
	header := make([]byte, pktHeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return Trace{}, fmt.Errorf("read PKT log header: %w", err)
	}
	if string(header[:3]) != "PKT" {
		return Trace{}, fmt.Errorf("invalid PKT signature %q", string(header[:3]))
	}
	if version := binary.LittleEndian.Uint16(header[3:5]); version != pktVersion {
		return Trace{}, fmt.Errorf("unsupported PKT version 0x%04x", version)
	}
	optionalSize := binary.LittleEndian.Uint32(header[62:66])
	if optionalSize > pktMaxFieldSize {
		return Trace{}, fmt.Errorf("PKT log optional header is too large: %d", optionalSize)
	}
	if err := discardPKTBytes(r, int64(optionalSize)); err != nil {
		return Trace{}, fmt.Errorf("read PKT log optional header: %w", err)
	}
	startTicks := binary.LittleEndian.Uint32(header[58:62])
	if source == "" {
		source = "trinitycore-pkt"
	}
	trace := Trace{Header: Header{Format: Format, Version: Version, Source: source}}
	for {
		packetHeader := make([]byte, pktPacketPrefixSize)
		n, err := io.ReadFull(r, packetHeader)
		if err == io.EOF && n == 0 {
			return trace, nil
		}
		if err != nil {
			return Trace{}, fmt.Errorf("read PKT packet header %d: %w", len(trace.Events)+1, err)
		}
		directionCode := binary.LittleEndian.Uint32(packetHeader[:4])
		var direction Direction
		switch directionCode {
		case pktClientDirection:
			direction = ClientToServer
		case pktServerDirection:
			direction = ServerToClient
		default:
			return Trace{}, fmt.Errorf("PKT packet %d has unknown direction 0x%08x", len(trace.Events)+1, directionCode)
		}
		connectionID := binary.LittleEndian.Uint32(packetHeader[4:8])
		arrivalTicks := binary.LittleEndian.Uint32(packetHeader[8:12])
		packetOptionalSize := binary.LittleEndian.Uint32(packetHeader[12:16])
		length := binary.LittleEndian.Uint32(packetHeader[16:20])
		if packetOptionalSize > pktMaxFieldSize {
			return Trace{}, fmt.Errorf("PKT packet %d optional data is too large: %d", len(trace.Events)+1, packetOptionalSize)
		}
		optionalData := make([]byte, packetOptionalSize)
		if _, err := io.ReadFull(r, optionalData); err != nil {
			return Trace{}, fmt.Errorf("read PKT packet %d optional data: %w", len(trace.Events)+1, err)
		}
		if length < 4 || length-4 > pktMaxFieldSize {
			return Trace{}, fmt.Errorf("PKT packet %d has invalid length %d", len(trace.Events)+1, length)
		}
		opcodeBytes := make([]byte, 4)
		if _, err := io.ReadFull(r, opcodeBytes); err != nil {
			return Trace{}, fmt.Errorf("read PKT packet %d opcode: %w", len(trace.Events)+1, err)
		}
		payload := make([]byte, length-4)
		if _, err := io.ReadFull(r, payload); err != nil {
			return Trace{}, fmt.Errorf("read PKT packet %d payload: %w", len(trace.Events)+1, err)
		}
		metadata := PacketMetadata{ConnectionID: connectionID}
		if len(optionalData) >= pktOptionalDataSize {
			metadata.RemoteIP = pktAddress(optionalData[:16])
			metadata.RemotePort = binary.LittleEndian.Uint32(optionalData[16:20])
		}
		trace.Events = append(trace.Events, Event{Sequence: uint64(len(trace.Events) + 1), TimeNS: int64(uint32(arrivalTicks-startTicks)) * int64(time.Millisecond), Direction: direction, Opcode: binary.LittleEndian.Uint32(opcodeBytes), Payload: base64.StdEncoding.EncodeToString(payload), ConnectionID: metadata.ConnectionID, RemoteIP: metadata.RemoteIP, RemotePort: metadata.RemotePort})
	}
}

func (t Trace) WritePKT(w io.Writer) error {
	if w == nil {
		return errors.New("PKT writer is nil")
	}
	if t.Header.Format != "" && (t.Header.Format != Format || t.Header.Version != Version) {
		return fmt.Errorf("unsupported protocol trace header %q version %d", t.Header.Format, t.Header.Version)
	}
	header := make([]byte, pktHeaderSize)
	copy(header[:3], []byte("PKT"))
	binary.LittleEndian.PutUint16(header[3:5], pktVersion)
	header[5] = 'T'
	binary.LittleEndian.PutUint32(header[6:10], 12340)
	copy(header[10:14], []byte("enUS"))
	now := time.Now()
	binary.LittleEndian.PutUint32(header[54:58], uint32(now.Unix()))
	binary.LittleEndian.PutUint32(header[58:62], uint32(now.UnixMilli()))
	if err := writePKTBytes(w, header); err != nil {
		return err
	}
	startTicks := binary.LittleEndian.Uint32(header[58:62])
	for index, event := range t.Events {
		payload, err := t.Payload(event)
		if err != nil {
			return err
		}
		if event.TimeNS < 0 || uint64(len(payload))+4 > uint64(^uint32(0)) {
			return fmt.Errorf("PKT event %d has invalid time or payload length", index+1)
		}
		var direction uint32
		switch event.Direction {
		case ClientToServer:
			direction = pktClientDirection
		case ServerToClient:
			direction = pktServerDirection
		default:
			return fmt.Errorf("PKT event %d has invalid direction %q", index+1, event.Direction)
		}
		optional := make([]byte, pktOptionalDataSize)
		if ip := net.ParseIP(event.RemoteIP); ip != nil {
			if ip4 := ip.To4(); ip4 != nil {
				copy(optional[:4], ip4)
			} else {
				copy(optional[:16], ip.To16())
			}
		}
		binary.LittleEndian.PutUint32(optional[16:20], event.RemotePort)
		packetHeader := make([]byte, 44)
		binary.LittleEndian.PutUint32(packetHeader[0:4], direction)
		binary.LittleEndian.PutUint32(packetHeader[4:8], event.ConnectionID)
		binary.LittleEndian.PutUint32(packetHeader[8:12], startTicks+uint32(event.TimeNS/int64(time.Millisecond)))
		binary.LittleEndian.PutUint32(packetHeader[12:16], pktOptionalDataSize)
		binary.LittleEndian.PutUint32(packetHeader[16:20], uint32(4+len(payload)))
		copy(packetHeader[20:40], optional)
		binary.LittleEndian.PutUint32(packetHeader[40:44], event.Opcode)
		if err := writePKTBytes(w, packetHeader); err != nil {
			return err
		}
		if err := writePKTBytes(w, payload); err != nil {
			return err
		}
	}
	return nil
}

func discardPKTBytes(r io.Reader, count int64) error {
	if count == 0 {
		return nil
	}
	_, err := io.CopyN(io.Discard, r, count)
	return err
}

func writePKTBytes(w io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := w.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

func pktAddress(data []byte) string {
	if len(data) < 16 {
		return ""
	}
	ipv4 := true
	for _, value := range data[4:16] {
		if value != 0 {
			ipv4 = false
			break
		}
	}
	if ipv4 {
		return net.IP(data[:4]).String()
	}
	return net.IP(data[:16]).String()
}
