package world

import (
	"errors"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func readObjectGUIDCandidates(payload []byte) ([]uint64, error) {
	reader := protocol.NewReader(payload)
	values := make([]uint64, 0, 2)
	if len(payload) >= 8 {
		if value, err := reader.ReadU64(); err == nil && value != 0 {
			values = append(values, value)
		}
	}
	packedReader := protocol.NewReader(payload)
	if value, err := packedReader.ReadPackedGUID(); err == nil && packedReader.Remaining() == 0 && value != 0 {
		for _, candidate := range values {
			if candidate == value {
				return values, nil
			}
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil, errors.New("object GUID is malformed")
	}
	return values, nil
}

func readObjectGUID(payload []byte) (uint64, error) {
	values, err := readObjectGUIDCandidates(payload)
	if err != nil {
		return 0, err
	}
	return values[0], nil
}
