package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocoltrace"
)

func main() {
	expectedPath := flag.String("expected", "", "reference protocol trace JSONL")
	expectedPKTPath := flag.String("expected-pkt", "", "TrinityCore PKT 3.1 packet log")
	actualPath := flag.String("actual", "", "Go protocol trace JSONL")
	actualPKTPath := flag.String("actual-pkt", "", "Go or TrinityCore PKT 3.1 packet log")
	replayPath := flag.String("replay", "", "protocol trace JSONL to replay")
	outputPath := flag.String("output", "", "diff JSON output path; stdout when omitted")
	compareTiming := flag.Bool("compare-timing", false, "compare relative event timing")
	timingTolerance := flag.Duration("timing-tolerance", 0, "allowed relative timing difference")
	ignoreState := flag.Bool("ignore-state", false, "ignore non-wire state labels")
	selfCheck := flag.Bool("self-check", false, "validate PKT 3.1 parsing in memory")
	flag.Parse()
	if *selfCheck {
		if err := runSelfCheck(); err != nil {
			fail(err)
		}
		fmt.Println("PKT 3.1 self-check passed")
		return
	}
	if *replayPath != "" {
		if err := replay(*replayPath); err != nil {
			fail(err)
		}
		return
	}
	if (*expectedPath == "" && *expectedPKTPath == "") || (*actualPath == "" && *actualPKTPath == "") {
		fail(fmt.Errorf("an expected and actual trace are required unless -replay is used"))
	}
	if *expectedPath != "" && *expectedPKTPath != "" {
		fail(fmt.Errorf("-expected and -expected-pkt cannot be combined"))
	}
	if *actualPath != "" && *actualPKTPath != "" {
		fail(fmt.Errorf("-actual and -actual-pkt cannot be combined"))
	}
	var expected protocoltrace.Trace
	var err error
	if *expectedPKTPath != "" {
		expected, err = loadPKTFile(*expectedPKTPath)
	} else {
		expected, err = loadFile(*expectedPath)
	}
	if err != nil {
		fail(err)
	}
	var actual protocoltrace.Trace
	if *actualPKTPath != "" {
		actual, err = loadPKTFile(*actualPKTPath)
	} else {
		actual, err = loadFile(*actualPath)
	}
	if err != nil {
		fail(err)
	}
	differences, err := protocoltrace.Diff(expected, actual, protocoltrace.CompareOptions{CompareTiming: *compareTiming, TimingTolerance: *timingTolerance, IgnoreState: *ignoreState || *expectedPKTPath != "" || *actualPKTPath != ""})
	if err != nil {
		fail(err)
	}
	result := struct {
		Equal       bool                       `json:"equal"`
		Differences []protocoltrace.Difference `json:"differences"`
	}{Equal: len(differences) == 0, Differences: differences}
	var data []byte
	data, err = json.MarshalIndent(result, "", "  ")
	if err != nil {
		fail(err)
	}
	data = append(data, '\n')
	if *outputPath == "" {
		_, _ = os.Stdout.Write(data)
	} else if err := os.WriteFile(*outputPath, data, 0o644); err != nil {
		fail(err)
	}
	if !result.Equal {
		os.Exit(1)
	}
}

func loadFile(path string) (protocoltrace.Trace, error) {
	file, err := os.Open(path)
	if err != nil {
		return protocoltrace.Trace{}, err
	}
	defer file.Close()
	return protocoltrace.Load(file)
}

func loadPKTFile(path string) (protocoltrace.Trace, error) {
	file, err := os.Open(path)
	if err != nil {
		return protocoltrace.Trace{}, err
	}
	defer file.Close()
	return protocoltrace.LoadPKT(file, path)
}

func runSelfCheck() error {
	data := make([]byte, 66)
	copy(data[:3], []byte("PKT"))
	binary.LittleEndian.PutUint16(data[3:5], 0x0301)
	data[5] = 'T'
	binary.LittleEndian.PutUint32(data[6:10], 12340)
	copy(data[10:14], []byte("enUS"))
	binary.LittleEndian.PutUint32(data[58:62], 1000)
	appendPacket := func(direction, connection, arrival, port, opcode uint32, payload []byte) {
		packet := make([]byte, 44+len(payload))
		binary.LittleEndian.PutUint32(packet[0:4], direction)
		binary.LittleEndian.PutUint32(packet[4:8], connection)
		binary.LittleEndian.PutUint32(packet[8:12], arrival)
		binary.LittleEndian.PutUint32(packet[12:16], 20)
		binary.LittleEndian.PutUint32(packet[16:20], uint32(4+len(payload)))
		copy(packet[20:24], []byte{127, 0, 0, 1})
		binary.LittleEndian.PutUint32(packet[36:40], port)
		binary.LittleEndian.PutUint32(packet[40:44], opcode)
		copy(packet[44:], payload)
		data = append(data, packet...)
	}
	appendPacket(0x47534d43, 9, 1030, 3724, 0x00AA, []byte{1, 2, 3})
	appendPacket(0x47534d53, 9, 1070, 8085, 0x00BB, []byte{4, 5})
	trace, err := protocoltrace.LoadPKT(bytes.NewReader(data), "self-check")
	if err != nil {
		return err
	}
	if len(trace.Events) != 2 {
		return fmt.Errorf("expected two PKT events, got %d", len(trace.Events))
	}
	first, second := trace.Events[0], trace.Events[1]
	if first.Direction != protocoltrace.ClientToServer || second.Direction != protocoltrace.ServerToClient || first.ConnectionID != 9 || second.ConnectionID != 9 || first.RemoteIP != "127.0.0.1" || first.RemotePort != 3724 || first.TimeNS != 30*int64(1000000) || second.TimeNS != 70*int64(1000000) || first.Opcode != 0x00AA || second.Opcode != 0x00BB {
		return fmt.Errorf("unexpected PKT metadata: first=%+v second=%+v", first, second)
	}
	firstPayload, err := trace.Payload(first)
	if err != nil || !bytes.Equal(firstPayload, []byte{1, 2, 3}) {
		return fmt.Errorf("unexpected first PKT payload: %x: %w", firstPayload, err)
	}
	return nil
}

func replay(path string) error {
	trace, err := loadFile(path)
	if err != nil {
		return err
	}
	return trace.Replay(replayPrinter{})
}

type replayPrinter struct{}

func (replayPrinter) Handle(direction protocoltrace.Direction, opcode uint32, payload []byte, state string) error {
	fmt.Printf("%s opcode=%d payload=%x state=%s\n", direction, opcode, payload, state)
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}
