package world

import (
	"bytes"
	"compress/zlib"
	"io"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const maxSecureAddons = 25

func buildAddonInfoResponse(payload []byte) []byte {
	response := protocol.NewBuffer(4 + maxSecureAddons*8)
	addons := 0
	if len(payload) >= 4 {
		reader := protocol.NewReader(payload)
		uncompressedSize, err := reader.ReadU32()
		if err == nil && uncompressedSize > 0 && uncompressedSize <= 0xFFFFF && reader.Remaining() > 0 {
			compressed := bytes.NewReader(reader.Bytes()[reader.Position():])
			zreader, zerr := zlib.NewReader(compressed)
			if zerr == nil {
				decoded, readErr := io.ReadAll(io.LimitReader(zreader, int64(uncompressedSize)+1))
				_ = zreader.Close()
				if readErr == nil && len(decoded) <= int(uncompressedSize) {
					decodedReader := protocol.NewReader(decoded)
					count, countErr := decodedReader.ReadU32()
					if countErr == nil {
						if count > maxSecureAddons {
							count = maxSecureAddons
						}
						for i := uint32(0); i < count; i++ {
							if _, err := decodedReader.ReadCString(); err != nil {
								break
							}
							if _, err := decodedReader.ReadU8(); err != nil {
								break
							}
							if _, err := decodedReader.ReadU32(); err != nil {
								break
							}
							if _, err := decodedReader.ReadU32(); err != nil {
								break
							}
							addons++
						}
					}
				}
			}
		}
	}
	for i := 0; i < addons; i++ {
		response.WriteU8(2)  // SECURE_HIDDEN
		response.WriteU8(1)  // info provided
		response.WriteU8(0)  // public key already present
		response.WriteU32(0) // revision
		response.WriteU8(0)  // URL not provided
	}
	response.WriteU32(0) // no newly banned addon records
	return response.Bytes()
}
