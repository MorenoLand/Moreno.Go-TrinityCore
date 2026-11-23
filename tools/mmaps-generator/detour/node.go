// A* search nodes, mirroring dep/recastnavigation/Detour/Source/DetourNode.cpp
// and Include/DetourNode.h. The pool is a hash table keyed by polygon
// reference plus a small per-poly "state" (used for tile-boundary crossing
// sides in findPath); the queue is a binary min-heap on total cost.
package detour

// Node flags (dtNodeFlags, DetourNode.h).
const (
	// DT_NODE_OPEN: node is in the open list.
	DT_NODE_OPEN uint8 = 0x01
	// DT_NODE_CLOSED: node is in the closed list.
	DT_NODE_CLOSED uint8 = 0x02
	// DT_NODE_PARENT_DETACHED: parent of the node is not adjacent (found using raycast).
	DT_NODE_PARENT_DETACHED uint8 = 0x04
)

// DT_NULL_IDX is the null node index (dtNodeIndex ~0, DetourNode.h).
const DT_NULL_IDX = uint16(0xffff)

// DT_NODE_PARENT_BITS / DT_NODE_STATE_BITS (DetourNode.h): pidx holds
// 24 bits, state 2 bits, so a pool holds at most (1<<24)-1 nodes and
// DT_MAX_STATES_PER_NODE states per polygon reference.
const (
	DT_NODE_PARENT_BITS    = 24
	DT_NODE_STATE_BITS     = 2
	DT_MAX_STATES_PER_NODE = 1 << DT_NODE_STATE_BITS
)

// PolyRef is a handle to a polygon within a navigation mesh tile
// (dtPolyRef, DetourNavMesh.h). DT_POLYREF64 is defined in this tree, so
// it is a uint64: salt 12 bits | tile 21 bits | poly 31 bits.
type PolyRef uint64

// TileRef is a handle to a tile within a navigation mesh
// (dtTileRef, DetourNavMesh.h).
type TileRef uint64

// Node mirrors dtNode (DetourNode.h). Pidx is the pool index of the
// parent node plus one (0 means no parent); State distinguishes nodes
// for the same polygon reference. idx is the node's own 0-based pool
// index, carried so GetNodeIdx stays O(1) without pointer arithmetic;
// it is set when the node is allocated and never changes within an
// allocation epoch.
type Node struct {
	Pos   Vec3
	Cost  float32
	Total float32
	Pidx  uint32
	State uint8
	Flags uint8
	ID    PolyRef
	idx   uint16
}

// hashRef mirrors the DT_POLYREF64 dtHashRef (DetourNode.cpp, Thomas Wang
// hash). Arithmetic is uint64 and wraps exactly as in C++.
func hashRef(a PolyRef) uint32 {
	x := uint64(a)
	x = (^x) + (x << 18)
	x = x ^ (x >> 31)
	x = x * 21
	x = x ^ (x >> 11)
	x = x + (x << 6)
	x = x ^ (x >> 22)
	return uint32(x)
}

// NodePool mirrors dtNodePool (DetourNode.cpp). Nodes live in a
// preallocated slice; GetNode returns pointers into it, which stay valid
// because the slice is never reallocated after construction.
type NodePool struct {
	nodes     []Node
	first     []uint16
	next      []uint16
	maxNodes  int
	hashSize  int
	nodeCount int
}

// NewNodePool mirrors dtNodePool::dtNodePool (DetourNode.cpp). maxNodes
// must be in (0, 65535]; hashSize must be a power of two. Callers pass
// dtNextPow2(maxNodes/4) as the hash size, as dtNavMeshQuery::init does.
func NewNodePool(maxNodes, hashSize int) *NodePool {
	p := &NodePool{
		nodes:    make([]Node, maxNodes),
		first:    make([]uint16, hashSize),
		next:     make([]uint16, maxNodes),
		maxNodes: maxNodes,
		hashSize: hashSize,
	}
	for i := range p.first {
		p.first[i] = DT_NULL_IDX
	}
	for i := range p.next {
		p.next[i] = DT_NULL_IDX
	}
	return p
}

// Clear mirrors dtNodePool::clear: the hash buckets are reset and the
// node count drops to zero. Node contents are left stale; GetNode
// re-initializes a node when it is (re)allocated.
func (p *NodePool) Clear() {
	for i := range p.first {
		p.first[i] = DT_NULL_IDX
	}
	p.nodeCount = 0
}

// FindNodes mirrors dtNodePool::findNodes: all pool nodes with the given
// id, up to maxNodes of them.
func (p *NodePool) FindNodes(id PolyRef, maxNodes int) []*Node {
	var out []*Node
	bucket := hashRef(id) & uint32(p.hashSize-1)
	for i := p.first[bucket]; i != DT_NULL_IDX; i = p.next[i] {
		if p.nodes[i].ID == id {
			if len(out) >= maxNodes {
				return out
			}
			out = append(out, &p.nodes[i])
		}
	}
	return out
}

// FindNode mirrors dtNodePool::findNode: the node with the given id and
// state, or nil.
func (p *NodePool) FindNode(id PolyRef, state uint8) *Node {
	bucket := hashRef(id) & uint32(p.hashSize-1)
	for i := p.first[bucket]; i != DT_NULL_IDX; i = p.next[i] {
		if p.nodes[i].ID == id && p.nodes[i].State == state {
			return &p.nodes[i]
		}
	}
	return nil
}

// GetNode mirrors dtNodePool::getNode: the node with the given id and
// state, allocating a fresh zeroed node when none exists. It returns nil
// when the pool is full.
func (p *NodePool) GetNode(id PolyRef, state uint8) *Node {
	bucket := hashRef(id) & uint32(p.hashSize-1)
	for i := p.first[bucket]; i != DT_NULL_IDX; i = p.next[i] {
		if p.nodes[i].ID == id && p.nodes[i].State == state {
			return &p.nodes[i]
		}
	}

	if p.nodeCount >= p.maxNodes {
		return nil
	}

	i := uint16(p.nodeCount)
	p.nodeCount++

	node := &p.nodes[i]
	node.Pidx = 0
	node.Cost = 0
	node.Total = 0
	node.ID = id
	node.State = state
	node.Flags = 0
	node.idx = i

	p.next[i] = p.first[bucket]
	p.first[bucket] = i

	return node
}

// GetNodeIdx mirrors dtNodePool::getNodeIdx: the 1-based pool index of
// the node, or 0 for nil.
func (p *NodePool) GetNodeIdx(node *Node) uint32 {
	if node == nil {
		return 0
	}
	return uint32(node.idx) + 1
}

// GetNodeAtIdx mirrors dtNodePool::getNodeAtIdx: the node at 1-based
// index idx, or nil for 0.
func (p *NodePool) GetNodeAtIdx(idx uint32) *Node {
	if idx == 0 {
		return nil
	}
	return &p.nodes[idx-1]
}

// MaxNodes mirrors dtNodePool::getMaxNodes.
func (p *NodePool) MaxNodes() int { return p.maxNodes }

// NodeCount mirrors dtNodePool::getNodeCount.
func (p *NodePool) NodeCount() int { return p.nodeCount }

// NodeQueue mirrors dtNodeQueue (DetourNode.cpp): a binary min-heap on
// node total cost, 0-based with parent (i-1)/2.
type NodeQueue struct {
	heap     []*Node
	capacity int
	size     int
}

// NewNodeQueue mirrors dtNodeQueue::dtNodeQueue.
func NewNodeQueue(n int) *NodeQueue {
	return &NodeQueue{heap: make([]*Node, n+1), capacity: n}
}

// Clear mirrors dtNodeQueue::clear.
func (q *NodeQueue) Clear() { q.size = 0 }

// Empty mirrors dtNodeQueue::empty.
func (q *NodeQueue) Empty() bool { return q.size == 0 }

// Capacity mirrors dtNodeQueue::getCapacity.
func (q *NodeQueue) Capacity() int { return q.capacity }

func (q *NodeQueue) bubbleUp(i int, node *Node) {
	parent := (i - 1) / 2
	for i > 0 && q.heap[parent].Total > node.Total {
		q.heap[i] = q.heap[parent]
		i = parent
		parent = (i - 1) / 2
	}
	q.heap[i] = node
}

func (q *NodeQueue) trickleDown(i int, node *Node) {
	child := i*2 + 1
	for child < q.size {
		if child+1 < q.size && q.heap[child].Total > q.heap[child+1].Total {
			child++
		}
		q.heap[i] = q.heap[child]
		i = child
		child = i*2 + 1
	}
	q.bubbleUp(i, node)
}

// Pop mirrors dtNodeQueue::pop.
func (q *NodeQueue) Pop() *Node {
	result := q.heap[0]
	q.size--
	q.trickleDown(0, q.heap[q.size])
	return result
}

// Push mirrors dtNodeQueue::push.
func (q *NodeQueue) Push(node *Node) {
	q.size++
	q.bubbleUp(q.size-1, node)
}

// Modify mirrors dtNodeQueue::modify: re-heapify a node already in the
// queue after its total changed (linear scan for the node, then bubble
// up; totals only ever decrease here).
func (q *NodeQueue) Modify(node *Node) {
	for i := 0; i < q.size; i++ {
		if q.heap[i] == node {
			q.bubbleUp(i, node)
			return
		}
	}
}
