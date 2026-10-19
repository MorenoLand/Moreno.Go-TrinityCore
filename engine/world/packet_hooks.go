package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// Packet-hook dispatch mirroring Eluna::OnPacketReceive / Eluna::OnPacketSend
// in the fork (src/server/game/LuaEngine/PacketHooks.cpp).
//
// Fork semantics, both directions:
//   - "any" bindings fire first, then per-opcode bindings; a false from the
//     any stage does NOT skip the per-opcode stage (OnPacketReceive/OnPacketSend
//     call Any then One unconditionally).
//   - Lua receives a COPY of the packet per stage (Push(new WorldPacket(packet)));
//     in-place Lua edits of that copy never reach the live packet. The C++
//     pushes one copy per stage and shares it across that stage's handlers
//     (lua_pushvalue of the same userdata in CallOneFunction), so the Go side
//     likewise passes one fresh copy per stage.
//   - On receive each stage captures 2 returns per handler: bool-false blocks
//     the opcode handler, and a Packet userdata second return REPLACES the
//     packet (packet = *data). On send each stage captures 1 return:
//     bool-false drops the packet, and the copy is discarded.
//
// FORK QUIRK (fidelity target, not upstream Eluna): the per-opcode RECEIVE
// bindings are looked up with the SEND event number
// (PacketHooks.cpp:117-119: START_HOOK_PACKET(PACKET_EVENT_ON_PACKET_SEND,
// packet.GetOpcode())), i.e. PACKET_EVENT_ON_PACKET_SEND = 7
// (Hooks.h:96). So on receive the engine fires per-opcode event-7 bindings
// and event-5 per-opcode bindings are DEAD — replicated below with a comment
// at the fire site.

// copyPacketForLua deep-copies a packet for one hook stage so in-place Lua
// edits (SetOpcode/Write*) cannot leak into the live packet, matching the
// fork's Push(new WorldPacket(packet)) copy (PacketHooks.cpp:95,120,68,44).
func copyPacketForLua(p *scripting.Packet) *scripting.Packet {
	return &scripting.Packet{
		Opcode:  p.Opcode,
		Data:    append([]byte(nil), p.Data...),
		ReadPos: p.ReadPos,
	}
}

// applyPacketReceiveReturns folds one stage's per-handler [first, second]
// returns into the fork's receive semantics (PacketHooks.cpp:93-115): a
// boolean false first return blocks the opcode handler (sticky), and a
// *scripting.Packet second return replaces the current packet by content
// copy (packet = *data), so later Lua edits of the returned userdata do not
// leak back.
func applyPacketReceiveReturns(values [][]any, cur *scripting.Packet) (blocked bool) {
	for _, vals := range values {
		if len(vals) > 0 {
			if allowed, ok := vals[0].(bool); ok && !allowed {
				blocked = true
			}
		}
		if len(vals) > 1 {
			if rep, ok := vals[1].(*scripting.Packet); ok && rep != nil {
				cur.Opcode = rep.Opcode
				cur.Data = append([]byte(nil), rep.Data...)
				cur.ReadPos = rep.ReadPos
			}
		}
	}
	return blocked
}

// packetHookBlocked folds one stage's single-return results into the fork's
// send semantics (PacketHooks.cpp:42-80): only a boolean false first return
// blocks; the per-stage copy is discarded, so in-place edits never leak.
func packetHookBlocked(values []any) bool {
	for _, value := range values {
		if allowed, ok := value.(bool); ok && !allowed {
			return true
		}
	}
	return false
}

// firePacketReceiveHooks runs the fork's OnPacketReceive binding sequence for
// one inbound packet: server-any bindings (SERVER_EVENT_ON_PACKET_RECEIVE = 5)
// then per-opcode bindings with the SEND event number 7 (the fork quirk,
// PacketHooks.cpp:117-119). It returns whether the opcode handler must be
// skipped and the packet (possibly replaced by a hook's second return) the
// handler should see. The returned packet is always a fresh copy; callers pass
// cur.Data to the handler. Handler selection stays on the ORIGINAL opcode,
// matching the C++ (opHandle is chosen before sEluna->OnPacketReceive runs).
func firePacketReceiveHooks(scripts *scripting.Runtime, ctx context.Context, opcode uint32, payload []byte, player *scripting.Object) (blocked bool, cur *scripting.Packet, hookErr error) {
	cur = &scripting.Packet{Opcode: opcode, Data: append([]byte(nil), payload...)}
	// Hook errors are logged by the runtime; the C++ keeps firing the
	// per-opcode stage regardless, so the first error is returned for the
	// caller's session-level debug log without aborting the sequence.
	anyVals, err := scripts.TriggerServerEvent2(ctx, scripting.ServerEventPacketReceive, copyPacketForLua(cur), player)
	if err != nil {
		hookErr = err
	} else if applyPacketReceiveReturns(anyVals, cur) {
		blocked = true
	}
	// FORK QUIRK: per-opcode receive bindings are keyed with the SEND event
	// number (PACKET_EVENT_ON_PACKET_SEND = 7); event-5 per-opcode bindings
	// never fire on receive in the fork (PacketHooks.cpp:117-119). A copy of
	// the (possibly any-stage-replaced) packet goes to this stage, matching
	// Push(new WorldPacket(packet)) after the any stage mutated packet.
	oneVals, err := scripts.TriggerPacketEvent2(ctx, int(opcode), scripting.ServerEventPacketSend, copyPacketForLua(cur), player)
	if err != nil {
		if hookErr == nil {
			hookErr = err
		}
	} else if applyPacketReceiveReturns(oneVals, cur) {
		blocked = true
	}
	return blocked, cur, hookErr
}

// firePacketSendHooks runs the fork's OnPacketSend binding sequence for one
// outbound packet: server-any bindings (SERVER_EVENT_ON_PACKET_SEND = 7) then
// per-opcode event-7 bindings, each with 1 result (PacketHooks.cpp:42-80).
// Each stage gets a throwaway copy; the live opcode/payload are never
// re-read, so in-place Lua edits cannot alter the wire packet.
func firePacketSendHooks(scripts *scripting.Runtime, ctx context.Context, opcode uint32, payload []byte, player *scripting.Object) (blocked bool, hookErr error) {
	probe := &scripting.Packet{Opcode: opcode, Data: append([]byte(nil), payload...)}
	anyVals, err := scripts.TriggerServerEvent(ctx, scripting.ServerEventPacketSend, copyPacketForLua(probe), player)
	if err != nil {
		hookErr = err
	} else if packetHookBlocked(anyVals) {
		return true, nil
	}
	oneVals, err := scripts.TriggerPacketEvent(ctx, int(opcode), scripting.ServerEventPacketSend, copyPacketForLua(probe), player)
	if err != nil {
		if hookErr == nil {
			hookErr = err
		}
	} else if packetHookBlocked(oneVals) {
		return true, hookErr
	}
	return false, hookErr
}
