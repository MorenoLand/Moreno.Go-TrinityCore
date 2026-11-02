package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// bgWinnerTeamID translates a BG_EVENT_ON_END winner argument into the
// PvPTeamId BattleGroundMethods.h GetWinner returns (SharedDefines.h:
// PVP_TEAM_HORDE 0, PVP_TEAM_ALLIANCE 1, PVP_TEAM_NEUTRAL 2). The hook
// argument itself uses the Team convention (BattleGroundHooks.cpp OnBGEnd
// passes Team: ALLIANCE 0, HORDE 1, NEUTRAL 2), so Alliance and Horde swap;
// anything else (including the draw 2) reads Neutral. Non-END fires have no
// winner argument and read Neutral, the C++ _winnerTeamId default
// (Battleground.cpp:80).
func bgWinnerTeamID(event int, args []any) uint32 {
	if event != scripting.BGEventOnEnd || len(args) == 0 {
		return 2
	}
	var winner uint32
	switch value := args[0].(type) {
	case uint32:
		winner = value
	case uint:
		winner = uint32(value)
	case int:
		if value < 0 {
			return 2
		}
		winner = uint32(value)
	case int8:
		if value < 0 {
			return 2
		}
		winner = uint32(value)
	case int64:
		if value < 0 {
			return 2
		}
		winner = uint32(value)
	case float64:
		if value < 0 {
			return 2
		}
		winner = uint32(value)
	default:
		return 2
	}
	switch winner {
	case 0:
		return 1
	case 1:
		return 0
	default:
		return 2
	}
}

// bgScriptObject builds the Lua BattleGround object for a hook fire. The
// snapshot is taken here, before Trigger, so no handler method re-reads
// battleground state while the fire runs under a per-map state lock (the
// start/end legs): only the battleground_template store lock and the
// read-only DBC store are touched, neither held on the hook path. Name is
// the BattlemasterList.dbc Name[0] (BattlegroundMgr.cpp:503, default DBC
// locale); player counts come from the template row
// (BattlegroundMgr.cpp:505-508: totals are the per-team counts doubled),
// and levels come from the template row unless the pop banked a
// bracket range, which overwrites them (see below).
// A missing template row or DBC entry reads zero values, the same
// missing-row shape the template loader produces elsewhere. Status and
// EndTime are snapshot by fire point, because the live values are
// unreachable here (GetEndTime's EndedAt sits under the state lock the
// end legs hold): CREATE reads WAIT_JOIN (BattlegroundMgr.cpp:413 sets it
// at creation), START reads IN_PROGRESS (Battleground.cpp:453), END reads
// WAIT_LEAVE with m_EndTime just set to TIME_TO_AUTOREMOVE
// (Battleground.cpp:715-717, no _ProcessLeave tick has run yet), and
// PREDESTROY reads NONE — the instance is already out of the registry,
// and STATUS_NONE is defined as "bg is not instance" (Battleground.h:178).
// BracketID is the banked pop-time bracket (see bankBGBracket): the
// queue-pop simulation banks it at invite time, which precedes every
// CREATE fire, so CREATE/START/END/PREDESTROY all read the instance's
// bracket; an unbanked fire (a state created without a pop in this
// process lifetime, e.g. a restored WaitJoin slot) reads the C++ ctor
// default 0. MinLevel/MaxLevel are the banked bracket range when the
// pop banked one (C++ SetBracket overwrites the template-copied levels
// with the bracket row's, Battleground.cpp:1792), else the template
// row's overall range below.
func (srv *Server) bgScriptObject(mapID, bgTypeID, instanceID uint32, event int, args ...any) *scripting.Object {
	snap := scripting.BGSnapshot{Winner: bgWinnerTeamID(event, args)}
	switch event {
	case scripting.BGEventOnCreate:
		snap.Status = 2
	case scripting.BGEventOnStart:
		snap.Status = 3
	case scripting.BGEventOnEnd:
		snap.Status = 4
		snap.EndTime = uint32(bgWaitLeaveAutoRemove.Milliseconds())
	}
	if srv != nil {
		snap.BracketID = srv.bgBracketFor(mapID, instanceID)
		if tmpl := srv.bgTemplateForType(context.Background(), bgTypeID); tmpl != nil {
			snap.MinLevel = uint32(tmpl.minLevel)
			snap.MaxLevel = uint32(tmpl.maxLevel)
			if minLevel, maxLevel, ok := srv.bgBracketRangeFor(mapID, instanceID); ok {
				snap.MinLevel = minLevel
				snap.MaxLevel = maxLevel
			}
			snap.MinPlayersPerTeam = tmpl.minPlayersPerTeam
			snap.MaxPlayersPerTeam = tmpl.maxPlayersPerTeam
			snap.MinPlayers = tmpl.minPlayersPerTeam * 2
			snap.MaxPlayers = tmpl.maxPlayersPerTeam * 2
		}
		if srv.Data != nil {
			if file, err := srv.Data.File("BattlemasterList"); err == nil {
				if rec, found := file.Find(bgTypeID); found {
					if name, err := rec.String(11); err == nil {
						snap.Name = name
					}
				}
			}
		}
		snap.Map = scripting.NewMapObject(mapID, instanceID, srv.mapLive(mapID, instanceID))
	}
	return scripting.NewBattleGroundObject(bgTypeID, mapID, instanceID, scripting.BGLive{
		Load: func() (scripting.BGSnapshot, bool) { return snap, true },
	})
}

// triggerBGEvent dispatches an Eluna BG_EVENT_* hook (RegisterBGEvent) as
// (event, bg, bgId, instanceId, ...args), mirroring Eluna::BattleGroundHooks
// (BattleGroundHooks.cpp). Go's non-arena battleground states are per-map,
// not per-instance, so instanceId is 0: the per-map state carries no
// instance id to report. Arena legs fire through triggerArenaBGEvent
// instead: arena states are per-instance and carry a real instance id plus
// the per-map type id (NA/BE/RL/DS/RV), not battlegroundTypeForMap's shared
// queue type 4. These hooks never cancel: the C++ call sites use
// CallAllFunctions, not the Bool form. Callers fire only after releasing
// the battleground registry lock, except the start/end legs, which fire
// under the per-map state lock like the C++ EndBattleground fire point; the
// BG object is a pre-fire snapshot, so no handler method reaches that lock.
func (srv *Server) triggerBGEvent(mapID uint32, event int, args ...any) {
	if srv == nil || srv.Features == nil || srv.Features.Scripts == nil {
		return
	}
	bgTypeID, _, isArena, ok := battlegroundTypeForMap(mapID)
	if !ok || isArena {
		return
	}
	hookArgs := append([]any{srv.bgScriptObject(mapID, bgTypeID, 0, event, args...), bgTypeID, uint32(0)}, args...)
	if _, err := srv.Features.Scripts.TriggerBGEvent(context.Background(), event, hookArgs...); err != nil {
		srv.debug("lua bg event failed", "event", event, "error", err)
	}
}

// triggerArenaBGEvent is triggerBGEvent for arena instances: the caller
// passes the state's map id, per-map type id and real instance id. The winner
// argument on BGEventOnEnd is the Team convention (Gold = Alliance 0, Green
// = Horde 1, draw = 2), matching the non-arena END convention; the BG
// object's GetWinner translates it to PvPTeamId.
func (srv *Server) triggerArenaBGEvent(mapID, bgTypeID, instanceID uint32, event int, args ...any) {
	if srv == nil || srv.Features == nil || srv.Features.Scripts == nil {
		return
	}
	hookArgs := append([]any{srv.bgScriptObject(mapID, bgTypeID, instanceID, event, args...), bgTypeID, instanceID}, args...)
	if _, err := srv.Features.Scripts.TriggerBGEvent(context.Background(), event, hookArgs...); err != nil {
		srv.debug("lua bg event failed", "event", event, "error", err)
	}
}
