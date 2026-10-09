package world

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// ThreatManager manages a creature's threat table and victim selection heuristics.
// Reference: TrinityCore ThreatManager.h / ThreatManager.cpp.
type ThreatManager struct {
	mu             sync.Mutex
	ownerGUID      uint64
	currentVictim  uint64
	entries        map[uint64]float32
	lastClientSync time.Time
	// ownerCanHaveThreatList caches ThreatManager::CanHaveThreatList
	// (ThreatManager.cpp:156-179) for the owner at construction, mirroring
	// C++'s _ownerCanHaveThreatList (ThreatManager.cpp:196). Pets, totems,
	// triggers and player-summoned minions/guardians never create threat
	// refs — ThreatManager::AddThreat's !CanHaveThreatList() early leg
	// (ThreatManager.cpp:328-339) keeps combat state only.
	ownerCanHaveThreatList bool
	// updateTimerMs counts down to the next ThreatManager::Update
	// (ThreatManager.cpp:199-209, THREAT_UPDATE_INTERVAL = 1000ms).
	updateTimerMs int64
	// fixateGUID mirrors _fixateRef (ThreatManager.cpp:494-511): the
	// script-fixated victim, always preferred by victim selection.
	fixateGUID uint64
	// tauntExpiry mirrors the per-reference taunt state maintained by
	// ThreatManager::TauntUpdate (ThreatManager.cpp:439-458): victim GUID
	// -> taunt-aura expiry. A zero expiry means the aura carries no
	// duration and the state lives until the aura is removed.
	tauntExpiry map[uint64]time.Time
	// tempMods mirrors ThreatReference::_tempModifier (ThreatManager.h:299):
	// the additive-constant from SPELL_AURA_MOD_TOTAL_THREAT (aura 103)
	// effects on the victim, summed by ThreatManager::UpdateMyTempModifiers
	// (ThreatManager.cpp:699-718). entries holds the _baseAmount; every
	// threat read compares max(base + tempMods[victim], 0), the
	// ThreatReference::GetThreat floor (ThreatManager.h:260). Like C++ the
	// map only covers victims with an existing entry — a new entry starts
	// at 0 and picks the modifier up on the next aura apply/remove/change.
	tempMods map[uint64]int32
	// needClientUpdate mirrors ThreatManager::_needClientUpdate
	// (set at ThreatManager.cpp:49/61/141/793, consumed at 522-525):
	// any threat value change that does not switch the victim (AddThreat,
	// ScaleThreat, taunt-state swap, new threat ref) marks the list dirty,
	// and the next Update tick consumes it (SMSG_THREAT_UPDATE when the
	// victim is unchanged, SMSG_HIGHEST_THREAT_UPDATE when it switched).
	needClientUpdate bool
}

// motionCanHaveThreatList mirrors ThreatManager::CanHaveThreatList
// (ThreatManager.cpp:156-179): only creatures that are not pets, totems or
// triggers — and not player-summoned minions/guardians — keep a threat list.
// Go models pets as creatureMotions with a nonzero OwnerGUID (totems are not
// motions at all; triggers carry no type flag on the motion and their threat
// is never read); charmed creatures also carry OwnerGUID (charmCreature) but
// C++ keeps their threat lists, so they are carved out explicitly.
func motionCanHaveThreatList(motion *creatureMotion) bool {
	if motion == nil {
		return true
	}
	return motion.OwnerGUID == 0 || motion.Charmed
}

// NewThreatManager initializes a ThreatManager for a creature motion,
// caching the owner's CanHaveThreatList answer like C++'s Initialize()
// (ThreatManager.cpp:194-197). A nil motion keeps the historical default.
func NewThreatManager(motion *creatureMotion) *ThreatManager {
	var ownerGUID uint64
	if motion != nil {
		ownerGUID = motion.GUID
	}
	return &ThreatManager{
		ownerGUID:              ownerGUID,
		ownerCanHaveThreatList: motionCanHaveThreatList(motion),
		entries:                make(map[uint64]float32),
		tauntExpiry:            make(map[uint64]time.Time),
		tempMods:               make(map[uint64]int32),
	}
}

// effectiveThreatLocked returns the victim's effective threat —
// ThreatReference::GetThreat (ThreatManager.h:260): the base amount plus
// the SPELL_AURA_MOD_TOTAL_THREAT temp modifier, floored at zero. The
// modifier is an additive constant per victim (all of the victim's refs on
// this list share it — ThreatManager::UpdateMyTempModifiers sets every
// _threatenedByMe ref to the same mod), so it re-ranks victims only via
// the floor. Callers hold tm.mu.
func (tm *ThreatManager) effectiveThreatLocked(victim uint64) float32 {
	threat := tm.entries[victim] + float32(tm.tempMods[victim])
	if threat < 0 {
		return 0
	}
	return threat
}

// SetTempModifier sets the victim's SPELL_AURA_MOD_TOTAL_THREAT additive
// modifier (ThreatReference::_tempModifier). Mirrors
// ThreatManager::UpdateMyTempModifiers (ThreatManager.cpp:699-718): it runs
// only over existing refs, so a victim with no entry keeps the constructor
// default 0 until an aura apply/remove/change recomputes it. C++ re-heapifies
// the sorted list on change (HeapNotifyIncreased/Decreased); Go's Update tick
// re-runs reselectVictimLocked every second, which is the same cadence as
// C++'s UpdateVictim-driven ReselectVictim, so no eager switch is needed.
func (tm *ThreatManager) SetTempModifier(victim uint64, mod int32) {
	if victim == 0 {
		return
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if _, ok := tm.entries[victim]; !ok {
		return
	}
	if mod == 0 {
		delete(tm.tempMods, victim)
		return
	}
	tm.tempMods[victim] = mod
}

// OwnerCanHaveThreatList reports the cached ThreatManager::CanHaveThreatList
// answer for the owner. Damage funnels consult it before consuming redirect
// registries or switching victims: C++ runs those arms inside AddThreat,
// after the !CanHaveThreatList() early return (ThreatManager.cpp:328-339).
func (tm *ThreatManager) OwnerCanHaveThreatList() bool {
	if tm == nil {
		return true
	}
	return tm.ownerCanHaveThreatList
}

// FixateTarget mirrors ThreatManager::FixateTarget (ThreatManager.cpp:494-506):
// a script-fixated victim already on the threat list becomes the preferred
// victim; a zero or unknown victim clears the fixate.
func (tm *ThreatManager) FixateTarget(victim uint64) {
	if tm == nil {
		return
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if victim != 0 {
		if _, ok := tm.entries[victim]; ok {
			tm.fixateGUID = victim
			return
		}
	}
	tm.fixateGUID = 0
}

// GetFixateTarget mirrors ThreatManager::GetFixateTarget (ThreatManager.cpp:508-513).
func (tm *ThreatManager) GetFixateTarget() uint64 {
	if tm == nil {
		return 0
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.fixateGUID
}

// ApplyTaunt records a MOD_TAUNT aura apply on the owner by the given caster,
// the Go half of AuraEffect::HandleModTaunt -> ThreatManager::TauntUpdate
// (SpellAuraEffects.cpp:2772-2781, ThreatManager.cpp:439-458). C++ keys the
// taunt state per threat reference and rebuilds it from the owner's live
// MOD_TAUNT aura effects; Go keys it per victim GUID with the aura's expiry,
// which the Update tick re-evaluates (the EvaluateSuppressed(true) tail of
// TauntUpdate has no Go model — suppressed/offline ref states are unmodeled).
// A non-positive duration means the aura carries no duration: the state lives
// until the aura is removed.
func (tm *ThreatManager) ApplyTaunt(victim uint64, durationMs uint32) {
	if tm == nil || victim == 0 {
		return
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.tauntExpiry == nil {
		tm.tauntExpiry = make(map[uint64]time.Time)
	}
	if durationMs > 0 {
		tm.tauntExpiry[victim] = time.Now().Add(time.Duration(durationMs) * time.Millisecond)
	} else {
		tm.tauntExpiry[victim] = time.Time{}
	}
	// The taunt-state swap marks the client list dirty
	// (ThreatReference::UpdateTauntState, ThreatManager.cpp:141).
	tm.needClientUpdate = true
}

// ClearTaunt drops one caster's taunt state, the remove arm of
// AuraEffect::HandleModTaunt -> ThreatManager::TauntUpdate.
func (tm *ThreatManager) ClearTaunt(victim uint64) {
	if tm == nil || victim == 0 {
		return
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	delete(tm.tauntExpiry, victim)
	// The taunt-state clear marks the client list dirty
	// (ThreatReference::UpdateTauntState, ThreatManager.cpp:141).
	tm.needClientUpdate = true
}

// applyCreatureTaunt bridges a MOD_TAUNT aura apply on a creature into its
// threat manager's taunt state (AuraEffect::HandleModTaunt,
// SpellAuraEffects.cpp:2772-2781 -> ThreatManager::TauntUpdate,
// ThreatManager.cpp:439-458). A creature with no threat manager is a no-op,
// matching C++ (TauntUpdate over an empty threat list records nothing).
func (s *Server) applyCreatureTaunt(key creatureAuraKey, casterGUID uint64, durationMs uint32) {
	if s == nil || key.GUID == 0 || casterGUID == 0 {
		return
	}
	motion := s.findCreatureMotion(key.Map, key.InstanceID, key.GUID)
	if motion == nil || motion.ThreatMgr == nil {
		return
	}
	motion.ThreatMgr.ApplyTaunt(casterGUID, durationMs)
}

// clearCreatureTaunt bridges a MOD_TAUNT aura removal into the threat
// manager, the remove arm of AuraEffect::HandleModTaunt.
func (s *Server) clearCreatureTaunt(key creatureAuraKey, casterGUID uint64) {
	if s == nil || key.GUID == 0 || casterGUID == 0 {
		return
	}
	motion := s.findCreatureMotion(key.Map, key.InstanceID, key.GUID)
	if motion == nil || motion.ThreatMgr == nil {
		return
	}
	motion.ThreatMgr.ClearTaunt(casterGUID)
}

// isTauntedLocked reports whether the victim holds a live taunt state,
// lazily expiring elapsed auras. Callers hold tm.mu.
func (tm *ThreatManager) isTauntedLocked(victim uint64) bool {
	exp, ok := tm.tauntExpiry[victim]
	if !ok {
		return false
	}
	if !exp.IsZero() && time.Now().After(exp) {
		delete(tm.tauntExpiry, victim)
		return false
	}
	return true
}

// IsTaunted reports whether the victim holds a live taunt state, the
// HasAuraType(SPELL_AURA_MOD_TAUNT) leg of Creature::CanCreatureAttack
// (Creature.cpp:2589-2595) as seen through the bridged taunt-state model.
func (tm *ThreatManager) IsTaunted(victim uint64) bool {
	if tm == nil || victim == 0 {
		return false
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.isTauntedLocked(victim)
}

// threatUpdateIntervalMs mirrors THREAT_UPDATE_INTERVAL (ThreatManager.h:86).
const threatUpdateIntervalMs = 1000

// Update mirrors ThreatManager::Update (ThreatManager.cpp:199-209): every
// THREAT_UPDATE_INTERVAL ms of accumulated combat time it re-runs victim
// selection (UpdateVictim -> ReselectVictim, ThreatManager.cpp:516-585).
// The tick is the only C++ path that re-gates the whole list, so it catches
// current-victim threat decay, taunt-aura expiry, and fixate changes that the
// eager AddThreat gate never revisits. inMelee classifies a candidate the way
// _owner->IsWithinMeleeRange does per candidate; a nil predicate treats every
// candidate as ranged (conservative: only the 130% gate can switch).
// Returns switched=true when the current victim changed.
func (tm *ThreatManager) Update(elapsedMs int64, inMelee func(uint64) bool) (switched bool, newVictim uint64, dirty bool) {
	if tm == nil {
		return false, 0, false
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if len(tm.entries) == 0 {
		return false, tm.currentVictim, false
	}
	tm.updateTimerMs -= elapsedMs
	if tm.updateTimerMs > 0 {
		return false, tm.currentVictim, false
	}
	tm.updateTimerMs = threatUpdateIntervalMs

	// TauntUpdate's EvaluateSuppressed(true) tail re-evaluates aura-driven
	// states; Go re-evaluates taunt expiries here (suppressed/offline ref
	// states have no Go model).
	now := time.Now()
	for victim, exp := range tm.tauntExpiry {
		if !exp.IsZero() && now.After(exp) {
			delete(tm.tauntExpiry, victim)
		}
	}

	newVictim = tm.reselectVictimLocked(inMelee)
	// ThreatManager::UpdateVictim (ThreatManager.cpp:522-525): the dirty
	// flag is consumed on the same cadence tick — broadcast
	// SMSG_HIGHEST_THREAT_UPDATE on a switch, SMSG_THREAT_UPDATE when the
	// victim is unchanged but the list moved since the last tick.
	dirty = tm.needClientUpdate
	tm.needClientUpdate = false
	if newVictim != tm.currentVictim {
		tm.currentVictim = newVictim
		return true, newVictim, dirty
	}
	return false, tm.currentVictim, dirty
}

// reselectVictimLocked mirrors ThreatManager::ReselectVictim
// (ThreatManager.cpp:531-585) with the CompareReferencesLT comparator
// (593-601). Callers hold tm.mu. The online/suppressed/offline leg of the
// comparator has no Go model (refs are removed explicitly on death/evade),
// and DETAUNT has no Go aura model, so the state precedence reduces to
// TAUNT > NONE; among equal states the 110%/130% + melee dance is exact,
// including the sorted-walk early-outs.
func (tm *ThreatManager) reselectVictimLocked(inMelee func(uint64) bool) uint64 {
	if len(tm.entries) == 0 {
		return 0
	}
	// Fixated target is always preferred (ReselectVictim:540-541); a stale
	// fixate whose entry is gone falls through like !IsAvailable().
	if tm.fixateGUID != 0 {
		if _, ok := tm.entries[tm.fixateGUID]; ok {
			return tm.fixateGUID
		}
		tm.fixateGUID = 0
	}

	type candidate struct {
		guid    uint64
		threat  float32
		taunted bool
	}
	cands := make([]candidate, 0, len(tm.entries))
	for guid := range tm.entries {
		cands = append(cands, candidate{guid: guid, threat: tm.effectiveThreatLocked(guid), taunted: tm.isTauntedLocked(guid)})
	}
	// Comparator order: taunt state precedence (TAUNT > NONE), then threat
	// descending — the heap order ReselectVictim walks.
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].taunted != cands[j].taunted {
			return cands[i].taunted
		}
		return cands[i].threat > cands[j].threat
	})

	old := tm.currentVictim
	if _, ok := tm.entries[old]; !ok {
		old = 0
	}
	highest := cands[0]
	// If we have no old victim, or the old victim is still highest, it wins.
	if old == 0 || highest.guid == old {
		return highest.guid
	}
	oldTaunted := tm.isTauntedLocked(old)
	// Taunt-state precedence (CompareReferencesLT:593-601): a taunted
	// highest beats a non-taunted old victim regardless of threat, and a
	// taunted old victim keeps aggro against non-taunted candidates.
	if highest.taunted != oldTaunted {
		if highest.taunted {
			return highest.guid
		}
		return old
	}
	oldThreat := tm.effectiveThreatLocked(old)
	// If the highest doesn't break 110% of the old victim, nothing below it
	// can either — old victim stays.
	if !(oldThreat*1.1 < highest.threat) {
		return old
	}
	// Above 130% it wins regardless of range.
	if oldThreat*1.3 < highest.threat {
		return highest.guid
	}
	// Between 110% and 130% it needs melee range.
	if inMelee != nil && inMelee(highest.guid) {
		return highest.guid
	}
	// The highest is ranged and below 130%: walk the sorted list for a
	// melee candidate above 110% beneath it.
	for _, c := range cands[1:] {
		if c.guid == old {
			return old
		}
		if c.taunted != oldTaunted {
			return old
		}
		if !(oldThreat*1.1 < c.threat) {
			return old
		}
		if inMelee != nil && inMelee(c.guid) {
			return c.guid
		}
	}
	// C++ ABORTs here ("manager desync"); Go keeps the old victim.
	return old
}

// AddThreat adds threat for a victim and evaluates target switching.
// Reference: TrinityCore ThreatManager::AddThreat (ThreatManager.cpp:308-410) /
// ReselectVictim (531-585) / CompareReferencesLT (593-601) / UpdateVictim (516-528).
// The 110%/130% switch gate matches C++ exactly, including the strict-greater
// boundary (CompareReferencesLT uses `<`, so equal threat never switches):
// a candidate breaks the current victim at >110% only in melee range,
// otherwise it needs >130%.
// Documented deltas: (1) C++ AddThreat never switches - selection happens in
// UpdateVictim->ReselectVictim on the AI tick (or when there is no current
// victim); Go re-evaluates eagerly inside AddThreat, outcome-equivalent for
// the adding victim but never re-evaluating non-adding candidates. (2) C++
// walks the sorted heap, so a ranged top below 130% does not block a melee
// candidate above 110% beneath it; Go tests only the adding victim against a
// single threshold, so that steal is missed.
// Bridged: ThreatReference::AddThreat negative amounts reduce the entry
// floored at 0 (ThreatManager.cpp:40-41); a zero add on an existing entry is a
// no-op, but on a NEW victim it still creates the entry and runs the victim
// leg (ThreatManager.cpp:308-410) — the EngageWithTarget 0.0f seed
// (Unit.cpp:8429-8438) registers the acquirer with zero threat. A decrease
// never clears the 110%/130% switch gate for the adding victim. Tick-based
// re-selection of a decreased non-current-victim entry stays under delta (1).
// The melee gate is per add, combat-reach-based via inMeleeThreatRange
// (Unit::IsWithinMeleeRange -> GetMeleeRange, Unit.cpp:599-618), matching the
// C++ per-candidate test positionally.
// Documented no-bridge: the online/suppressed/offline ref states
// (ShouldBeOffline/ShouldBeSuppressed: CanSeeOrDetect, _IsTargetAcceptable,
// CanCreatureAttack, immune flags, melee-school immunity, confuse,
// breakable stun) - Go refs are removed only via RemoveThreat/ClearThreat;
// DETAUNT (SPELL_AURA_MOD_DETAUNT, no Go aura model);
// ProcessAIUpdates/JustStartedThreateningMe (boss hooks ride the new-victim
// broadcast instead). FixateTarget and the taunt-state model are bridged:
// the eager gate above honors fixate preference and taunt precedence, and
// the Update tick runs the full ReselectVictim every second.
// Out of scope for this unit: the vehicle redirect leg of C++ AddThreat
// (ThreatManager.cpp:316-322) has no Go analog (Go models no vehicle
// threat); the threat-redirect registry itself is bridged on the session
// (spell_threat.go: redirectThreatRegistry, consumed by
// splitThreatRedirects at every AddThreat site whose C++ call leaves
// ignoreRedirects=false). getThreatMultiplier covers stance/aura
// SPELL_AURA_MOD_THREAT; the CalculateModifiedThreat spell-mod leg
// (SPELLMOD_THREAT) is bridged on the HandleThreatSpells cast-threat path
// (spell_threat.go: handleSpellInitialThreat).
func (tm *ThreatManager) AddThreat(victim uint64, amount float32, inMelee bool) (switched bool, newVictim uint64) {
	if victim == 0 {
		return false, tm.currentVictim
	}
	// ThreatManager::AddThreat (ThreatManager.cpp:328-339): a unit that
	// cannot have a threat list (pets, totems, triggers, player-summoned
	// minions/guardians) never creates threat refs — combat state only,
	// which the callers' engagement path already maintains. The redirect
	// consumption below is likewise skipped: in C++ it runs inside AddThreat
	// after this early return, so callers must also guard the redirect arm
	// (see OwnerCanHaveThreatList).
	if !tm.ownerCanHaveThreatList {
		return false, tm.currentVictim
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// ThreatManager::AddThreat (ThreatManager.cpp:308-410): the new-target leg
	// creates the ThreatReference and applies the amount through
	// ThreatReference::AddThreat even when the amount is 0 — Unit::EngageWithTarget
	// seeds 0.0f (Unit.cpp:8429-8438), and the zero add still registers the entry
	// and runs the victim leg. ThreatReference::AddThreat (ThreatManager.cpp:40-41)
	// no-ops on 0 and floors negatives at 0.
	oldThreat, exists := tm.entries[victim]
	var newThreat float32
	if !exists {
		if amount < 0 {
			amount = 0
		}
		newThreat = amount
		tm.entries[victim] = newThreat
		// ThreatManager::PutThreatListRef (ThreatManager.cpp:793) marks
		// the client list dirty on every new ref.
		tm.needClientUpdate = true
		// With no current victim the new ref becomes the victim outright
		// (C++ UpdateVictim); otherwise C++ runs ProcessAIUpdates (no
		// re-selection) — Go keeps its documented eager gate as delta (1).
		if tm.currentVictim == 0 {
			tm.currentVictim = victim
			return false, victim
		}
	} else {
		// ThreatReference::AddThreat: zero is a no-op on an existing entry;
		// negatives reduce, floored at zero. A decrease never clears the
		// 110%/130% switch gate for the adding victim. The switch gate
		// compares GetThreat — base plus the temp modifier, floored.
		if amount == 0 {
			return false, tm.currentVictim
		}
		// ThreatReference::AddThreat (ThreatManager.cpp:49) marks the
		// client list dirty on every nonzero add.
		tm.needClientUpdate = true
		newThreat = oldThreat + amount
		if newThreat < 0 {
			newThreat = 0
		}
		tm.entries[victim] = newThreat
		newThreat = tm.effectiveThreatLocked(victim)
	}

	// FixateTarget (ThreatManager.cpp:494-506) + ReselectVictim:540: the
	// fixated victim is always preferred — the eager gate never switches
	// away from it, and any add re-affirms it. A stale fixate whose entry
	// is gone is dropped like C++'s !IsAvailable() fall-through.
	if tm.fixateGUID != 0 {
		if _, ok := tm.entries[tm.fixateGUID]; ok {
			switched := tm.currentVictim != tm.fixateGUID
			tm.currentVictim = tm.fixateGUID
			return switched, tm.fixateGUID
		}
		tm.fixateGUID = 0
	}

	if tm.currentVictim == 0 || tm.currentVictim == victim {
		tm.currentVictim = victim
		return false, victim
	}

	// Taunt-state precedence (CompareReferencesLT, ThreatManager.cpp:593-601
	// — TAUNT > NONE; DETAUNT has no Go aura model): a taunted adding victim
	// outranks a non-taunted current victim regardless of the 110%/130%
	// gate, and a taunted current victim keeps aggro against non-taunted
	// adds — the aura-stickiness TauntUpdate maintains for the MOD_TAUNT
	// duration. The Update tick re-affirms this every second.
	addTaunted := tm.isTauntedLocked(victim)
	currTaunted := tm.isTauntedLocked(tm.currentVictim)
	if addTaunted != currTaunted {
		if addTaunted {
			tm.currentVictim = victim
			return true, victim
		}
		return false, tm.currentVictim
	}

	currThreat := tm.effectiveThreatLocked(tm.currentVictim)
	threshold := currThreat * 1.30
	if inMelee {
		threshold = currThreat * 1.10
	}

	if newThreat > threshold {
		tm.currentVictim = victim
		return true, victim
	}
	return false, tm.currentVictim
}

// SetThreat sets absolute threat for a victim (e.g. taunt).
func (tm *ThreatManager) SetThreat(victim uint64, amount float32) (switched bool, newVictim uint64) {
	if victim == 0 {
		return false, tm.currentVictim
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// SetThreat is a Go-only direct write (C++ has no ThreatManager::SetThreat);
	// the write marks the client list dirty the way every nonzero C++
	// threat write does (ThreatReference::AddThreat, ThreatManager.cpp:49).
	tm.entries[victim] = amount
	tm.needClientUpdate = true
	if tm.currentVictim == 0 || tm.effectiveThreatLocked(victim) > tm.effectiveThreatLocked(tm.currentVictim) {
		tm.currentVictim = victim
		return true, victim
	}
	return false, tm.currentVictim
}

// MatchUnitThreatToHighestThreat sets the victim's threat equal to the highest threat currently on the creature.
// Reference: TrinityCore ThreatManager::MatchUnitThreatToHighestThreat (ThreatManager.cpp:419-437).
// Documented deltas: C++ skips a highest that is itself taunting or unavailable;
// Go has no taunt-state model, so the raw highest wins. C++ returns early on an
// empty list; Go matches (no seed, no victim change). C++ routes the delta
// through AddThreat with ignoreModifiers/ignoreRedirects; Go writes the entry
// directly, which is value-equivalent for the missing-or-lower victim. Go always
// reports switched; C++ re-selects through the taunt-state comparator.
// The taunt-state machinery (TauntUpdate, comparator precedence) is
// documented at AddThreat; Go has no taunt-state model.
func (tm *ThreatManager) MatchUnitThreatToHighestThreat(victim uint64) (switched bool, newVictim uint64) {
	if victim == 0 {
		return false, tm.currentVictim
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// ThreatManager::MatchUnitThreatToHighestThreat (ThreatManager.cpp:419-437)
	// returns early on an empty list — no seed, no victim change.
	if len(tm.entries) == 0 {
		return false, tm.currentVictim
	}

	var highestThreat float32
	for guid := range tm.entries {
		if eff := tm.effectiveThreatLocked(guid); eff > highestThreat {
			highestThreat = eff
		}
	}
	current := tm.effectiveThreatLocked(victim)
	if highestThreat > current {
		// C++ adds the delta to the victim's _baseAmount
		// (ThreatManager.cpp:419-437 runs highest->GetThreat() -
		// GetThreat(target) through ThreatReference::AddThreat), which
		// marks the client list dirty (ThreatManager.cpp:49).
		tm.entries[victim] += highestThreat - current
		tm.needClientUpdate = true
	}
	tm.currentVictim = victim
	return true, victim
}

// RemoveThreat removes a victim from the threat table and re-evaluates top victim.
// Reference: TrinityCore ThreatManager::PurgeThreatListRef.
func (tm *ThreatManager) RemoveThreat(victim uint64) (switched bool, newVictim uint64) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	delete(tm.entries, victim)
	delete(tm.tempMods, victim)
	// ThreatManager.cpp:809-810: purging the fixated reference clears the
	// fixate; the victim's taunt state dies with its reference.
	if tm.fixateGUID == victim {
		tm.fixateGUID = 0
	}
	delete(tm.tauntExpiry, victim)
	if tm.currentVictim == victim {
		tm.currentVictim = 0
		var highestGUID uint64
		var highestThreat float32
		for guid := range tm.entries {
			if eff := tm.effectiveThreatLocked(guid); eff > highestThreat {
				highestThreat = eff
				highestGUID = guid
			}
		}
		tm.currentVictim = highestGUID
		return true, tm.currentVictim
	}
	return false, tm.currentVictim
}

// ScaleThreat multiplies a victim's threat entry by factor.
// Reference: ThreatReference::ScaleThreat (ThreatManager.cpp:52-61) no-ops on a
// factor of 1 and clamps negative factors at 0; ThreatManager::ScaleThreat
// (ThreatManager.cpp:412-417) only touches an existing entry, so a victim with
// no entry is a no-op. C++ reselects the victim lazily in UpdateVictim; Go
// reselects eagerly like RemoveThreat.
func (tm *ThreatManager) ScaleThreat(victim uint64, factor float32) (switched bool, newVictim uint64) {
	if victim == 0 || factor == 1 {
		return false, tm.currentVictim
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()

	threat, ok := tm.entries[victim]
	if !ok {
		return false, tm.currentVictim
	}
	if factor < 0 {
		factor = 0
	}
	tm.entries[victim] = threat * factor
	// ThreatReference::ScaleThreat (ThreatManager.cpp:61) marks the client
	// list dirty (the factor==1 early leg above is the C++ no-op).
	tm.needClientUpdate = true

	var highestGUID uint64
	var highestThreat float32
	for guid := range tm.entries {
		if eff := tm.effectiveThreatLocked(guid); eff > highestThreat {
			highestThreat = eff
			highestGUID = guid
		}
	}
	if highestGUID != tm.currentVictim {
		tm.currentVictim = highestGUID
		return true, highestGUID
	}
	return false, tm.currentVictim
}

// handleEffectModifyThreatPercent processes SPELL_EFFECT_MODIFY_THREAT_PERCENT (125).
// Reference: Spell::EffectModifyThreatPercent (SpellEffects.cpp:4915) runs at
// SPELL_EFFECT_HANDLE_HIT_TARGET per unit target:
// unitTarget->GetThreatManager().ModifyThreatByPercent(unitCaster, damage),
// where ModifyThreatByPercent (ThreatManager.h:143) no-ops on percent 0 and
// scales by 0.01*(100+percent). Entities that cannot hold a threat list
// (taunt precedent: totems, pets, triggers, player minions — OwnerGUID != 0)
// reject silently.
func (s *session) handleEffectModifyThreatPercent(ctx context.Context, eff wotlk.SpellEffect, hitTargets []uint64) {
	if s == nil || s.server == nil || s.player == nil {
		return
	}
	damage := eff.BasePoints + 1
	if damage == 0 {
		return
	}
	factor := float32(0.01 * float64(100+damage))
	for _, targetGUID := range hitTargets {
		if targetGUID == 0 {
			continue
		}
		if s.server.isTotemGUID(targetGUID) {
			continue
		}
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
		if motion == nil || motion.Evading || motion.OwnerGUID != 0 || motion.ThreatMgr == nil {
			s.server.motionMu.Unlock()
			continue
		}
		tm := motion.ThreatMgr
		s.server.motionMu.Unlock()

		switched, newVictim := tm.ScaleThreat(s.playerGUID, factor)
		if !switched {
			continue
		}
		// The taunt broadcast tail: announce the new highest-threat victim.
		s.server.motionMu.Lock()
		if m := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID); m != nil {
			m.TargetGUID = newVictim
			s.server.broadcastHighestThreatUpdateInInstance(m.Map, m.InstanceID, m.GUID, newVictim, tm.SortedEntries())
		}
		s.server.motionMu.Unlock()
	}
	_ = ctx
}

// HasVictim reports whether victim holds an entry in the threat table.
// Used by the RemoveMeFromThreatLists bridge (ThreatManager.cpp:690-697):
// removal tests entry existence, not a positive value.
func (tm *ThreatManager) HasVictim(victim uint64) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	_, ok := tm.entries[victim]
	return ok
}

// ClearThreat wipes the threat table on evade or death.
// Reference: TrinityCore ThreatManager::ClearAllThreat.
func (tm *ThreatManager) ClearThreat() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.entries = make(map[uint64]float32)
	tm.tempMods = make(map[uint64]int32)
	tm.currentVictim = 0
	tm.fixateGUID = 0
	tm.tauntExpiry = make(map[uint64]time.Time)
	tm.updateTimerMs = 0
}

// GetCurrentVictim returns the current primary threat target.
// Reference: TrinityCore ThreatManager::GetCurrentVictim
// (ThreatManager.cpp:210-217), which re-runs UpdateVictim when the current
// reference went offline — Go re-selects when the current victim no longer
// holds an entry (offline/suppressed ref states have no Go model; entries
// are removed explicitly on death/evade).
func (tm *ThreatManager) GetCurrentVictim() uint64 {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.currentVictim != 0 {
		if _, ok := tm.entries[tm.currentVictim]; !ok {
			tm.currentVictim = tm.reselectVictimLocked(nil)
		}
	}
	return tm.currentVictim
}

// GetThreat returns the threat value for a victim.
// ThreatReference::GetThreat (ThreatManager.h:260): the base amount plus the
// SPELL_AURA_MOD_TOTAL_THREAT temp modifier, floored at zero.
func (tm *ThreatManager) GetThreat(victim uint64) float32 {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.effectiveThreatLocked(victim)
}

// IsEmpty returns true if there are no targets threatening the creature.
func (tm *ThreatManager) IsEmpty() bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return len(tm.entries) == 0
}

// SortedEntries returns protocol-ready ThreatEntry slice sorted descending by threat.
// Reference: TrinityCore ThreatManager::SendThreatListToClients.
func (tm *ThreatManager) SortedEntries() []protocol.ThreatEntry {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	list := make([]protocol.ThreatEntry, 0, len(tm.entries))
	for guid := range tm.entries {
		// Client threat packets carry GetThreat (ThreatManager.cpp:212-213
		// SendThreatListToClients) — base plus the temp modifier.
		threat := tm.effectiveThreatLocked(guid)
		list = append(list, protocol.ThreatEntry{
			VictimGUID: guid,
			Threat:     uint32(threat * 100),
		})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Threat > list[j].Threat
	})
	return list
}

// Broadcast helpers for Threat Packets
func (s *Server) broadcastThreatUpdate(mapID uint32, creatureGUID uint64, list []protocol.ThreatEntry) {
	if s == nil {
		return
	}
	payload := protocol.BuildThreatUpdate(creatureGUID, list)
	s.broadcastToNearby(uint16(protocol.OpcodeSMSG_THREAT_UPDATE), payload, nil)
}

func (s *Server) broadcastThreatUpdateInInstance(mapID, instanceID uint32, creatureGUID uint64, list []protocol.ThreatEntry) {
	payload := protocol.BuildThreatUpdate(creatureGUID, list)
	s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_THREAT_UPDATE), payload, nil)
}

func (s *Server) broadcastHighestThreatUpdate(mapID uint32, creatureGUID, highestGUID uint64, list []protocol.ThreatEntry) {
	if s == nil {
		return
	}
	payload := protocol.BuildHighestThreatUpdate(creatureGUID, highestGUID, list)
	s.broadcastToNearby(uint16(protocol.OpcodeSMSG_HIGHEST_THREAT_UPDATE), payload, nil)
}

func (s *Server) broadcastHighestThreatUpdateInInstance(mapID, instanceID uint32, creatureGUID, highestGUID uint64, list []protocol.ThreatEntry) {
	payload := protocol.BuildHighestThreatUpdate(creatureGUID, highestGUID, list)
	s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_HIGHEST_THREAT_UPDATE), payload, nil)
}

func (s *Server) broadcastThreatRemove(mapID uint32, creatureGUID, victimGUID uint64) {
	if s == nil {
		return
	}
	payload := protocol.BuildThreatRemove(creatureGUID, victimGUID)
	s.broadcastToNearby(uint16(protocol.OpcodeSMSG_THREAT_REMOVE), payload, nil)
}

// sumThreatTempModifier sums the SPELL_AURA_MOD_TOTAL_THREAT (aura 103)
// effect amounts over a unit's active auras — the mod computed by
// ThreatManager::UpdateMyTempModifiers (ThreatManager.cpp:699-704). The
// per-effect amounts ride aura.Amounts aligned with the spell's effect
// indices, the same alignment activeAuraHasEffect uses for its type check.
func sumThreatTempModifier(data *wotlk.Store, auras map[uint32]*activeAura) int32 {
	if data == nil {
		return 0
	}
	var mod int32
	for _, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if index >= len(aura.Amounts) {
				break
			}
			if aura.EffectMask&(1<<uint(index)) != 0 && effect.Aura == spellAuraModTotalThreat {
				mod += aura.Amounts[index]
			}
		}
	}
	return mod
}

// threatTempModifierForUnit computes the unit's current
// SPELL_AURA_MOD_TOTAL_THREAT additive modifier. Player casters resolve
// through their session's activeAuras (castMu); creature casters through the
// server's activeCreatureAuras (auraMu).
func (s *Server) threatTempModifierForUnit(unitGUID uint64, mapID, instanceID uint32) int32 {
	if s == nil || unitGUID == 0 {
		return 0
	}
	if sess := s.findSessionByGUID(unitGUID); sess != nil {
		sess.castMu.Lock()
		defer sess.castMu.Unlock()
		return sumThreatTempModifier(s.Data, sess.activeAuras)
	}
	motion := s.findCreatureMotion(mapID, instanceID, unitGUID)
	if motion == nil {
		return 0
	}
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	return sumThreatTempModifier(s.Data, s.activeCreatureAuras[creatureAuraKeyForMotion(motion)])
}

// refreshThreatTempModifier recomputes the unit's SPELL_AURA_MOD_TOTAL_THREAT
// additive modifier and pushes it onto every creature threat list holding the
// unit — the Go analog of AuraEffect::HandleAuraModTotalThreat
// (SpellAuraEffects.cpp:2746-2772) driving
// ThreatManager::UpdateMyTempModifiers (ThreatManager.cpp:699-718) over the
// caster's _threatenedByMe refs. C++ fires the handler on 103-aura
// apply/change-amount with a player target and updates the AURA CASTER's
// manager, so callers pass the caster GUID, not the target's. Like C++ the
// push touches only existing entries (new refs start at 0) and does not
// switch victims eagerly — the 1s Update tick reselects on the new values.
func (s *Server) refreshThreatTempModifier(mapID, instanceID uint32, unitGUID uint64) {
	if s == nil || unitGUID == 0 {
		return
	}
	mod := s.threatTempModifierForUnit(unitGUID, mapID, instanceID)
	var managers []*ThreatManager
	s.motionMu.Lock()
	for _, motion := range s.motionMapLocked(mapID, instanceID) {
		if motion == nil || motion.ThreatMgr == nil {
			continue
		}
		if motion.ThreatMgr.HasVictim(unitGUID) {
			managers = append(managers, motion.ThreatMgr)
		}
	}
	s.motionMu.Unlock()
	for _, tm := range managers {
		tm.SetTempModifier(unitGUID, mod)
	}
}

func (s *Server) broadcastThreatRemoveInInstance(mapID, instanceID uint32, creatureGUID, victimGUID uint64) {
	if s == nil {
		return
	}
	payload := protocol.BuildThreatRemove(creatureGUID, victimGUID)
	s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_THREAT_REMOVE), payload, nil)
}

// removeThreatVictimFromAllLists drops victimGUID from every other unit's
// threat table on the map/instance.
// Reference: TrinityCore ThreatManager::RemoveMeFromThreatLists
// (ThreatManager.cpp:690-697), reached on evade via CombatStop(true) ->
// CombatManager::EndAllPvECombat (CombatManager.cpp:344-350) and on death
// via Unit::setDeathState -> CombatStop (Unit.cpp:8901-8907) — the
// setDeathState pairing of RemoveMeFromThreatLists (the dead unit as
// victim in everyone else's table) with ClearAllThreat (the dead unit's
// own table). Per-victim removal sends SMSG_THREAT_REMOVE
// (SendRemoveToClients, ThreatManager.cpp:477) and re-evaluates the top
// victim (RemoveThreat / UpdateVictim).
func (s *Server) removeThreatVictimFromAllLists(mapID, instanceID uint32, victimGUID uint64) {
	if s == nil || victimGUID == 0 {
		return
	}
	var owners []uint64
	s.motionMu.Lock()
	for guid, motion := range s.motionMapLocked(mapID, instanceID) {
		if guid == victimGUID || motion == nil || motion.ThreatMgr == nil {
			continue
		}
		if motion.ThreatMgr.HasVictim(victimGUID) {
			owners = append(owners, guid)
		}
	}
	s.motionMu.Unlock()
	for _, ownerGUID := range owners {
		motion := s.findCreatureMotion(mapID, instanceID, ownerGUID)
		if motion == nil || motion.ThreatMgr == nil {
			continue
		}
		motion.ThreatMgr.RemoveThreat(victimGUID)
		s.broadcastThreatRemoveInInstance(mapID, instanceID, ownerGUID, victimGUID)
	}
}

// stopPlayerCombat mirrors the combat half of Unit::CombatStop (Unit.cpp:5809-5825)
// reached on logout via Unit::CleanupBeforeRemoveFromMap (Unit.cpp:9845) and on
// teleport via Player::TeleportTo (Player.cpp:1753 same-map unless
// TELE_TO_NOT_LEAVE_COMBAT, 1803 far-teleport unconditional): drop the player as a
// victim from every creature/pet threat table on the map/instance
// (ThreatManager::RemoveMeFromThreatLists, ThreatManager.cpp:690-697, via
// CombatManager::EndAllPvECombat, CombatManager.cpp:344-350) and stop the player's
// own attack (AttackStop + SMSG_ATTACK_STOP). The ClearAllThreat leg is vacuous —
// Go players carry no ThreatMgr — and RemoveAllAttackers needs no bridge
// (attackers hold no separate attacker list in Go); the PvP suppression leg has
// no Go model outside sanctuary handling. No Go teleportTo caller maps to a
// TELE_TO_NOT_LEAVE_COMBAT (Player.h:666) site — blink/GO/transport/boss-mechanic
// teleports have no Go analog routing through teleportTo — so the stop is
// unconditional here, matching C++ for every Go caller (incl. Hearthstone, whose
// TELE_TO_SPELL still drops combat at Player.cpp:1753).
func (s *session) stopPlayerCombat() {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	s.server.removeThreatVictimFromAllLists(s.player.Map, s.player.InstanceID, s.playerGUID)
	if s.attackTarget != 0 {
		victim := s.attackTarget
		s.attackTarget = 0
		_ = s.sendAttackStop(victim, false)
	}
	// Unit::AttackStop (Unit.cpp:5774-5778) interrupts CURRENT_MELEE_SPELL:
	// a queued on-next-swing spell does not survive leaving combat.
	s.cancelNextSwingSpell()
}

// evadeCreaturesTargeting mirrors the RemoveAllAttackers half of
// Unit::CombatStop (Unit.cpp:5815): every creature in the map/instance
// actively targeting the given unit drops combat through the evade path
// (attack-stop + threat-clear broadcasts, state reset).
func (s *Server) evadeCreaturesTargeting(ctx context.Context, mapID, instanceID uint32, targetGUID uint64) {
	if s == nil || targetGUID == 0 {
		return
	}
	var guids []uint64
	s.motionMu.Lock()
	for guid, motion := range s.motionMapLocked(mapID, instanceID) {
		if motion == nil || !motion.InCombat || motion.TargetGUID != targetGUID {
			continue
		}
		guids = append(guids, guid)
	}
	s.motionMu.Unlock()
	now := time.Now()
	for _, guid := range guids {
		s.motionMu.Lock()
		motion := s.findCreatureMotionLocked(mapID, instanceID, guid)
		if motion == nil || !motion.InCombat || motion.TargetGUID != targetGUID {
			s.motionMu.Unlock()
			continue
		}
		s.motionMu.Unlock()
		s.triggerCreatureEvade(ctx, motion, now)
	}
}

// zeroThreatOnAllLists mirrors the dungeon/non-player arm of
// Spell::EffectSanctuary (SpellEffects.cpp:3767-3770): every threat list in
// the map/instance carrying the unit scales its entry to 0
// (ThreatReference::ScaleThreat, ThreatManager.cpp:52-61 — the entry stays,
// the value zeroes), with the eager victim reselect Go uses for threat
// changes.
func (s *Server) zeroThreatOnAllLists(mapID, instanceID uint32, targetGUID uint64) {
	if s == nil || targetGUID == 0 {
		return
	}
	var owners []uint64
	s.motionMu.Lock()
	for guid, motion := range s.motionMapLocked(mapID, instanceID) {
		if motion == nil || motion.ThreatMgr == nil {
			continue
		}
		if motion.ThreatMgr.HasVictim(targetGUID) {
			owners = append(owners, guid)
		}
	}
	s.motionMu.Unlock()
	for _, ownerGUID := range owners {
		motion := s.findCreatureMotion(mapID, instanceID, ownerGUID)
		if motion == nil || motion.ThreatMgr == nil {
			continue
		}
		switched, newVictim := motion.ThreatMgr.ScaleThreat(targetGUID, 0)
		if !switched {
			continue
		}
		s.motionMu.Lock()
		if m := s.findCreatureMotionLocked(mapID, instanceID, ownerGUID); m != nil {
			m.TargetGUID = newVictim
			s.broadcastHighestThreatUpdateInInstance(m.Map, m.InstanceID, m.GUID, newVictim, motion.ThreatMgr.SortedEntries())
		}
		s.motionMu.Unlock()
	}
}

func (s *Server) broadcastThreatClear(mapID uint32, creatureGUID uint64) {
	if s == nil {
		return
	}
	payload := protocol.BuildThreatClear(creatureGUID)
	s.broadcastToNearby(uint16(protocol.OpcodeSMSG_THREAT_CLEAR), payload, nil)
}

func (s *Server) broadcastThreatClearInInstance(mapID, instanceID uint32, creatureGUID uint64) {
	if s == nil {
		return
	}
	payload := protocol.BuildThreatClear(creatureGUID)
	s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_THREAT_CLEAR), payload, nil)
}

// getThreatMultiplier calculates the session's current threat multiplier based on active stances and auras.
// Reference: TrinityCore Unit::GetTotalAuraMultiplierByMiscMask(SPELL_AURA_MOD_THREAT, mask).
func (s *session) getThreatMultiplier(schoolMask uint32) float32 {
	if s == nil || s.player == nil {
		return 1.0
	}
	mult := float32(1.0)
	s.castMu.Lock()
	defer s.castMu.Unlock()

	for _, a := range s.activeAuras {
		if a == nil || a.Stopped {
			continue
		}
		// Generic SPELL_AURA_MOD_THREAT (AuraType 10)
		if a.AuraType == 10 {
			mult *= (1.0 + float32(int32(a.Amount))/100.0)
			continue
		}
		// Specific stance / aura threat modifiers
		switch a.SpellID {
		case 71: // Warrior: Defensive Stance (+45% threat)
			mult *= 1.45
		case 2457, 2458: // Warrior: Battle / Berserker Stance (-20% threat)
			mult *= 0.80
		case 5487, 9634: // Druid: Bear Form / Dire Bear Form (+30% threat)
			mult *= 1.30
		case 25780: // Paladin: Righteous Fury (+80% threat on Holy spells, schoolMask & 0x02 != 0)
			if schoolMask&0x02 != 0 {
				mult *= 1.80
			}
		case 1038: // Paladin: Hand of Salvation (-20% threat)
			mult *= 0.80
		}
	}
	if mult < 0.1 {
		mult = 0.1
	}
	return mult
}

// isTauntSpell returns true if the spell is a Taunt ability.
// Reference: TrinityCore SPELL_EFFECT_ATTACK_ME (114) and SPELL_AURA_MOD_TAUNT (11).
func isTauntSpell(spellID uint32) bool {
	switch spellID {
	case 355, // Warrior: Taunt
		694,   // Warrior: Mocking Blow
		1161,  // Warrior: Challenging Shout
		6795,  // Druid: Growl
		5209,  // Druid: Challenging Roar
		56222, // Death Knight: Dark Command
		62124, // Paladin: Hand of Reckoning
		31789: // Paladin: Righteous Defense
		return true
	}
	return false
}

// handleEffectTaunt processes SPELL_EFFECT_ATTACK_ME (114) and Taunt spells against creature targets.
// Reference: TrinityCore Spell::EffectTaunt (SpellEffects.cpp:3131-3169).
func (s *session) handleEffectTaunt(ctx context.Context, targetGUID uint64, spellID uint32) {
	if s == nil || s.server == nil || targetGUID == 0 {
		return
	}
	// Spell::EffectTaunt (SpellEffects.cpp:3141-3143): totems are not valid
	// taunt targets (the DONT_REPORT cast result is silent). Checked before
	// motionMu: the totem lifecycle takes totemMu -> motionMu, so acquiring
	// them in the reverse order here would deadlock.
	if s.server.isTotemGUID(targetGUID) {
		return
	}
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
	// Spell::EffectTaunt (SpellEffects.cpp:3147-3149): Hand of Reckoning
	// (62124) casts triggered 67485 on non-player targets that are not
	// already targeting the caster, before the threat-list gate. The flag is
	// captured here and the cast runs below without motionMu held:
	// castSpellDirect re-enters the spell pipeline, whose initial-threat leg
	// (handleSpellInitialThreat) takes motionMu — holding it across the cast
	// would deadlock.
	horDamage := spellID == 62124 && motion != nil && !motion.Evading && motion.TargetGUID != s.playerGUID
	s.server.motionMu.Unlock()

	if horDamage && s.server.findSessionByGUID(targetGUID) == nil {
		s.castSpellDirect(ctx, 67485, targetGUID)
	}

	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()

	motion = s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
	if motion == nil || motion.Evading {
		return
	}
	// Spell::EffectTaunt (SpellEffects.cpp:3153-3157): entities that cannot
	// have a threat list (ThreatManager::CanHaveThreatList,
	// ThreatManager.cpp:156-179 — pets, totems, triggers, player-summoned
	// minions/guardians) reject the taunt silently. Charmed creatures keep
	// their lists, so they are not rejected.
	if !motionCanHaveThreatList(motion) {
		return
	}
	if motion.ThreatMgr == nil {
		motion.ThreatMgr = NewThreatManager(motion)
	}
	// Spell::EffectTaunt (SpellEffects.cpp:3155-3159): taunting a target
	// already attacking the caster is a silent no-op.
	if motion.ThreatMgr.GetCurrentVictim() == s.playerGUID {
		return
	}
	switched, newVictim := motion.ThreatMgr.MatchUnitThreatToHighestThreat(s.playerGUID)
	if switched || newVictim != motion.TargetGUID {
		motion.TargetGUID = newVictim
		entries := motion.ThreatMgr.SortedEntries()
		s.server.broadcastHighestThreatUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, newVictim, entries)
	}
	motion.InCombat = true
	motion.Moving = true
}

// distributeHealingThreat calculates and splits healing threat among all creatures
// currently in combat with the healer or the heal target.
// Formula mirrors TrinityCore Unit::SendHealSpellLog and Unit::DoAttack (Unit.cpp:6550-6580):
// Base threat = effectiveHeal * 0.5 * healerThreatMultiplier.
// Threat is divided equally by the number of engaged creatures.
func (s *Server) distributeHealingThreat(ctx context.Context, healerGUID, targetGUID uint64, effectiveHeal uint32) {
	if s == nil || healerGUID == 0 || effectiveHeal == 0 {
		return
	}

	healerSess := s.findSessionByGUID(healerGUID)
	if healerSess == nil || healerSess.player == nil {
		return
	}

	mult := healerSess.getThreatMultiplier(2) // Holy/Healing school mask = 2
	totalThreat := float32(effectiveHeal) * 0.5 * mult
	if totalThreat <= 0 {
		return
	}

	s.motionMu.Lock()
	defer s.motionMu.Unlock()

	var engaged []*creatureMotion
	for _, m := range s.motionMapLocked(healerSess.player.Map, healerSess.player.InstanceID) {
		if m == nil || m.Health == 0 || !m.InCombat || m.Evading {
			continue
		}
		isEngaged := false
		if m.TargetGUID == healerGUID || m.TargetGUID == targetGUID {
			isEngaged = true
		} else if m.ThreatMgr != nil {
			if m.ThreatMgr.GetThreat(healerGUID) > 0 || (targetGUID != 0 && m.ThreatMgr.GetThreat(targetGUID) > 0) {
				isEngaged = true
			}
		}
		if isEngaged {
			engaged = append(engaged, m)
		}
	}

	if len(engaged) == 0 {
		return
	}

	threatPerCreature := totalThreat / float32(len(engaged))
	for _, m := range engaged {
		if m.ThreatMgr == nil {
			m.ThreatMgr = NewThreatManager(m)
		}
		dist := distance3D(healerSess.player.X, healerSess.player.Y, healerSess.player.Z, m.X, m.Y, m.Z)
		inMelee := inMeleeThreatRange(m.CombatReach, healerSess.player.CombatReach, dist)
		switched, newVictim := m.ThreatMgr.AddThreat(healerGUID, threatPerCreature, inMelee)
		if switched && newVictim != m.TargetGUID {
			m.TargetGUID = newVictim
			entries := m.ThreatMgr.SortedEntries()
			s.broadcastHighestThreatUpdateInInstance(m.Map, m.InstanceID, m.GUID, newVictim, entries)
		}
		m.Moving = true
	}

	if healerSess.player.UnitFlags&unitFlagInCombat == 0 {
		healerSess.player.UnitFlags |= unitFlagInCombat
		healerSess.sendPlayerUpdate()
	}
}
