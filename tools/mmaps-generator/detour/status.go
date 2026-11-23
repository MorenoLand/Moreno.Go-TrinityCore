// Status codes for Detour navigation-mesh operations, mirroring
// dtStatus (dep/recastnavigation/Detour/Include/DetourStatus.h).
package detour

// Status is the dtStatus bitfield: one high-level flag (FAILURE, SUCCESS,
// IN_PROGRESS) plus detail bits.
type Status uint32

const (
	// DT_FAILURE: operation failed.
	DT_FAILURE Status = 1 << 31
	// DT_SUCCESS: operation succeeded.
	DT_SUCCESS Status = 1 << 30
	// DT_IN_PROGRESS: operation still in progress.
	DT_IN_PROGRESS Status = 1 << 29
	// DT_STATUS_DETAIL_MASK: detail bits of a status value.
	DT_STATUS_DETAIL_MASK Status = 0x0ffffff
	// DT_WRONG_MAGIC: input data is not recognized.
	DT_WRONG_MAGIC Status = 1 << 0
	// DT_WRONG_VERSION: input data is in wrong version.
	DT_WRONG_VERSION Status = 1 << 1
	// DT_OUT_OF_MEMORY: operation ran out of memory.
	DT_OUT_OF_MEMORY Status = 1 << 2
	// DT_INVALID_PARAM: an input parameter was invalid.
	DT_INVALID_PARAM Status = 1 << 3
	// DT_BUFFER_TOO_SMALL: result buffer was too small to store all results.
	DT_BUFFER_TOO_SMALL Status = 1 << 4
	// DT_OUT_OF_NODES: query ran out of nodes during search.
	DT_OUT_OF_NODES Status = 1 << 5
	// DT_PARTIAL_RESULT: query did not reach the end location, returning best guess.
	DT_PARTIAL_RESULT Status = 1 << 6
	// DT_ALREADY_OCCUPIED: a tile has already been assigned to the given x,y coordinate.
	DT_ALREADY_OCCUPIED Status = 1 << 7
)

// StatusSucceed mirrors dtStatusSucceed (DetourStatus.h).
func StatusSucceed(status Status) bool { return status&DT_SUCCESS != 0 }

// StatusFailed mirrors dtStatusFailed (DetourStatus.h).
func StatusFailed(status Status) bool { return status&DT_FAILURE != 0 }

// StatusInProgress mirrors dtStatusInProgress (DetourStatus.h).
func StatusInProgress(status Status) bool { return status&DT_IN_PROGRESS != 0 }

// StatusDetail mirrors dtStatusDetail (DetourStatus.h).
func StatusDetail(status Status, detail Status) bool { return status&detail != 0 }
