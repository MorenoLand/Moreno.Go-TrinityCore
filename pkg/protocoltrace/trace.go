package protocoltrace

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

const Format = "moreno.protocol-trace"
const Version = 1

type Direction string

const (
	ClientToServer Direction = "client_to_server"
	ServerToClient Direction = "server_to_client"
)

type Header struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	Source  string `json:"source,omitempty"`
}

type Event struct {
	Sequence  uint64    `json:"sequence"`
	TimeNS    int64     `json:"time_ns"`
	Direction Direction `json:"direction"`
	Opcode    uint32    `json:"opcode"`
	Payload   string    `json:"payload_base64"`
	State     string    `json:"state,omitempty"`
}

type Trace struct {
	Header Header
	Events []Event
}

type Recorder struct {
	mu      sync.Mutex
	header  Header
	started time.Time
	events  []Event
}

func NewRecorder(source string) *Recorder {
	return &Recorder{header: Header{Format: Format, Version: Version, Source: source}, started: time.Now()}
}

func (r *Recorder) Record(direction Direction, opcode uint32, payload []byte, state string) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	sequence := uint64(len(r.events) + 1)
	data := make([]byte, len(payload))
	copy(data, payload)
	r.events = append(r.events, Event{Sequence: sequence, TimeNS: time.Since(r.started).Nanoseconds(), Direction: direction, Opcode: opcode, Payload: base64.StdEncoding.EncodeToString(data), State: state})
	return sequence
}

func (r *Recorder) Snapshot() Trace {
	r.mu.Lock()
	defer r.mu.Unlock()
	events := make([]Event, len(r.events))
	copy(events, r.events)
	return Trace{Header: r.header, Events: events}
}

func (t Trace) Write(w io.Writer) error {
	if w == nil {
		return errors.New("protocol trace writer is nil")
	}
	if t.Header.Format == "" {
		t.Header = Header{Format: Format, Version: Version}
	}
	if t.Header.Format != Format || t.Header.Version != Version {
		return fmt.Errorf("unsupported protocol trace header %q version %d", t.Header.Format, t.Header.Version)
	}
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(t.Header); err != nil {
		return err
	}
	for _, event := range t.Events {
		if err := encoder.Encode(event); err != nil {
			return err
		}
	}
	return nil
}

func Load(r io.Reader) (Trace, error) {
	if r == nil {
		return Trace{}, errors.New("protocol trace reader is nil")
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024), 32*1024*1024)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return Trace{}, err
		}
		return Trace{}, errors.New("protocol trace is empty")
	}
	var header Header
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return Trace{}, fmt.Errorf("decode protocol trace header: %w", err)
	}
	if header.Format != Format || header.Version != Version {
		return Trace{}, fmt.Errorf("unsupported protocol trace header %q version %d", header.Format, header.Version)
	}
	trace := Trace{Header: header}
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return Trace{}, fmt.Errorf("decode protocol trace event: %w", err)
		}
		if event.Sequence != uint64(len(trace.Events)+1) {
			return Trace{}, fmt.Errorf("protocol trace sequence %d is out of order", event.Sequence)
		}
		if event.Direction != ClientToServer && event.Direction != ServerToClient {
			return Trace{}, fmt.Errorf("protocol trace event %d has invalid direction %q", event.Sequence, event.Direction)
		}
		if _, err := base64.StdEncoding.DecodeString(event.Payload); err != nil {
			return Trace{}, fmt.Errorf("decode protocol trace payload %d: %w", event.Sequence, err)
		}
		trace.Events = append(trace.Events, event)
	}
	if err := scanner.Err(); err != nil {
		return Trace{}, err
	}
	return trace, nil
}

func (t Trace) Payload(event Event) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(event.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode event %d payload: %w", event.Sequence, err)
	}
	return data, nil
}

type CompareOptions struct {
	CompareTiming   bool
	TimingTolerance time.Duration
}

type Difference struct {
	Sequence uint64 `json:"sequence"`
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

func Diff(expected, actual Trace, options CompareOptions) ([]Difference, error) {
	if expected.Header.Format != Format || expected.Header.Version != Version {
		return nil, errors.New("expected trace has unsupported header")
	}
	if actual.Header.Format != Format || actual.Header.Version != Version {
		return nil, errors.New("actual trace has unsupported header")
	}
	differences := make([]Difference, 0)
	if len(expected.Events) != len(actual.Events) {
		differences = append(differences, Difference{Field: "event_count", Expected: fmt.Sprint(len(expected.Events)), Actual: fmt.Sprint(len(actual.Events))})
	}
	count := len(expected.Events)
	if len(actual.Events) < count {
		count = len(actual.Events)
	}
	for i := 0; i < count; i++ {
		expectedEvent, actualEvent := expected.Events[i], actual.Events[i]
		sequence := expectedEvent.Sequence
		if actualEvent.Sequence != sequence {
			differences = append(differences, Difference{Sequence: sequence, Field: "sequence", Expected: fmt.Sprint(sequence), Actual: fmt.Sprint(actualEvent.Sequence)})
		}
		if expectedEvent.Direction != actualEvent.Direction {
			differences = append(differences, Difference{Sequence: sequence, Field: "direction", Expected: string(expectedEvent.Direction), Actual: string(actualEvent.Direction)})
		}
		if expectedEvent.Opcode != actualEvent.Opcode {
			differences = append(differences, Difference{Sequence: sequence, Field: "opcode", Expected: fmt.Sprint(expectedEvent.Opcode), Actual: fmt.Sprint(actualEvent.Opcode)})
		}
		expectedPayload, err := expected.Payload(expectedEvent)
		if err != nil {
			return nil, err
		}
		actualPayload, err := actual.Payload(actualEvent)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(expectedPayload, actualPayload) {
			differences = append(differences, Difference{Sequence: sequence, Field: "payload_base64", Expected: expectedEvent.Payload, Actual: actualEvent.Payload})
		}
		if expectedEvent.State != actualEvent.State {
			differences = append(differences, Difference{Sequence: sequence, Field: "state", Expected: expectedEvent.State, Actual: actualEvent.State})
		}
		if options.CompareTiming && absDuration(time.Duration(expectedEvent.TimeNS-actualEvent.TimeNS)) > options.TimingTolerance {
			differences = append(differences, Difference{Sequence: sequence, Field: "time_ns", Expected: fmt.Sprint(expectedEvent.TimeNS), Actual: fmt.Sprint(actualEvent.TimeNS)})
		}
	}
	return differences, nil
}

type ReplaySink interface {
	Handle(direction Direction, opcode uint32, payload []byte, state string) error
}

func (t Trace) Replay(sink ReplaySink) error {
	if sink == nil {
		return errors.New("protocol trace replay sink is nil")
	}
	for _, event := range t.Events {
		payload, err := t.Payload(event)
		if err != nil {
			return err
		}
		if err := sink.Handle(event.Direction, event.Opcode, payload, event.State); err != nil {
			return fmt.Errorf("replay event %d: %w", event.Sequence, err)
		}
	}
	return nil
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
