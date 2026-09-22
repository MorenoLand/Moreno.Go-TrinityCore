package world

import "github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"

func readObjectGUID(payload []byte) (uint64, error) {
	reader := protocol.NewReader(payload)
	if len(payload) == 8 {
		return reader.ReadU64()
	}
	return reader.ReadPackedGUID()
}
