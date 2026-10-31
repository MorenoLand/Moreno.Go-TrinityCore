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
	// suppressed mirrors ThreatReference ONLINE_STATE_SUPPRESSED for the
	// school-immunity apply path (ThreatManager::EvaluateSuppressed,
	// ThreatManager.cpp:290-304): victims marked here are skipped by
	// victim selection until a taunt re-evaluates them (a taunted victim
	// is never suppressed, ShouldBeSuppressed:112-115). The confuse /
	// breakable-stun legs of ShouldBeSuppressed and the offline leg
	// remain unmodeled.
	suppressed map[uint64]bool
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
		updateTimerMs:          threatUpdateIntervalMs,
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

// HasThreatEntry reports whether victim has a threat reference on this list,
// the ThreatManager::IsThreatenedBy twin (ThreatManager.h): the reference
// set, not the value, is the engaged test — a Unit::EngageWithTarget seed
// (0.0f) creates the entry, so GetThreat alone cannot tell a seeded entry
// from an absent victim (both read 0).
func (tm *ThreatManager) HasThreatEntry(victim uint64) bool {
	if tm == nil || victim == 0 {
		return false
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	_, ok := tm.entries[victim]
	return ok
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

// isSuppressedLocked reports whether the victim's reference is suppressed
// on this list. A taunted victim is never suppressed
// (ThreatReference::ShouldBeSuppressed, ThreatManager.cpp:112-115), so a
// live taunt exempts the victim even while the flag stands — the C++
// EvaluateSuppressed(true) tail of TauntUpdate re-evaluates the same way.
// Callers hold tm.mu.
func (tm *ThreatManager) isSuppressedLocked(victim uint64) bool {
	if tm.isTauntedLocked(victim) {
		return false
	}
	return tm.suppressed[victim]
}

// SuppressVictim marks the victim's reference suppressed on this list,
// the per-list half of ThreatManager::EvaluateSuppressed
// (ThreatManager.cpp:290-304) as called from
// AuraEffect::HandleAuraModSchoolImmunity apply
// (SpellAuraEffects.cpp:3192). Only an existing entry is marked; the
// current victim switches on the next Update tick, matching C++
// UpdateVictim cadence (no eager reselect here). A victim that is not
// damage-immune to the attacker's melee school is never marked — the
// caller gates on ShouldBeSuppressed's immunity leg.
func (tm *ThreatManager) SuppressVictim(victim uint64) {
	if tm == nil || victim == 0 {
		return
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if _, ok := tm.entries[victim]; !ok {
		return
	}
	if tm.isTauntedLocked(victim) {
		return
	}
	if tm.suppressed == nil {
		tm.suppressed = make(map[uint64]bool)
	}
	if !tm.suppressed[victim] {
		tm.suppressed[victim] = true
		tm.needClientUpdate = true
	}
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

// HasAnyTaunt reports whether ANY live taunt state exists on the creature,
// the creature-wide HasAuraType(SPELL_AURA_MOD_TAUNT) leg of
// Creature::CanCreatureAttack (Creature.cpp:2589): the C++ check is on the
// creature, not on the victim, so a taunt from one attacker suppresses the
// leash for every victim. Lazily expires elapsed auras the same way
// isTauntedLocked does.
func (tm *ThreatManager) HasAnyTaunt() bool {
	if tm == nil {
		return false
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	now := time.Now()
	for victim, exp := range tm.tauntExpiry {
		if exp.IsZero() || now.Before(exp) {
			return true
		}
		delete(tm.tauntExpiry, victim)
	}
	return false
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

	if !tm.ownerCanHaveThreatList || len(tm.entries) == 0 {
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
// (593-601). Callers hold tm.mu. The offline leg of the comparator has no
// Go model (refs are removed explicitly on death/evade); the suppressed
// leg is modeled for the school-immunity apply path (SuppressVictim) and
// skipped here unless the victim is taunted, matching IsAvailable. DETAUNT
// has no Go aura model, so the state precedence reduces to
// TAUNT > NONE; among equal states the 110%/130% + melee dance is exact,
// including the sorted-walk early-outs.
func (tm *ThreatManager) reselectVictimLocked(inMelee func(uint64) bool) uint64 {
	if len(tm.entries) == 0 {
		return 0
	}
	// Fixated target is always preferred (ReselectVictim:540-541); a stale
	// fixate whose entry is gone falls through like !IsAvailable().
	if tm.fixateGUID != 0 {
		if _, ok := tm.entries[tm.fixateGUID]; ok && !tm.isSuppressedLocked(tm.fixateGUID) {
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
		if tm.isSuppressedLocked(guid) {
			continue
		}
		cands = append(cands, candidate{guid: guid, threat: tm.effectiveThreatLocked(guid), taunted: tm.isTauntedLocked(guid)})
	}
	// C++ returns null when the highest reference is unavailable
	// (ReselectVictim:553-554); every entry suppressed means the same.
	if len(cands) == 0 {
		return 0
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
	if tm.isSuppressedLocked(old) {
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
// The "highest" is the heap's top (CompareReferencesLT, ThreatManager.cpp:593-601):
// taunt-state precedence first, then effective threat descending — with the
// taunt-skip leg (ThreatManager.cpp:424-429): a taunting highest yields to a
// next available ref with strictly more threat. Go entries have no
// online/suppressed/offline states, so every entry is available.
// The delta runs through the AddThreat entry legs
// (ThreatManager.cpp:379-410) with ignoreModifiers/ignoreRedirects, which
// keeps the redirect registry untouched: a missing entry is created
// (PutThreatListRef, ThreatManager.cpp:793, marks the client list dirty),
// then ThreatReference::AddThreat (ThreatManager.cpp:38-47) applies the delta
// — a zero delta is a no-op with no dirty mark, negatives floor at 0.
// Victim selection is the AddThreat legs verbatim: with a current victim the
// existing-entry leg selects nothing (the new-entry leg runs ProcessAIUpdates
// hooks only) — the 1s Update tick reselects, where the taunt-state
// precedence (ReselectVictim, ThreatManager.cpp:540-585) promotes the taunter
// while its bridged MOD_TAUNT state lives, so the victim switches on the tick
// exactly like C++ rather than inside the taunt. Only a missing current
// victim reselects immediately (UpdateVictim, ThreatManager.cpp:516-528).
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

	type candidate struct {
		guid    uint64
		threat  float32
		taunted bool
	}
	cands := make([]candidate, 0, len(tm.entries))
	for guid := range tm.entries {
		cands = append(cands, candidate{guid: guid, threat: tm.effectiveThreatLocked(guid), taunted: tm.isTauntedLocked(guid)})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].taunted != cands[j].taunted {
			return cands[i].taunted
		}
		return cands[i].threat > cands[j].threat
	})
	highest := cands[0]
	// The taunt-skip leg (ThreatManager.cpp:424-429): a taunting highest
	// yields to the next available ref with strictly more threat.
	if highest.taunted && len(cands) > 1 && cands[1].threat > highest.threat {
		highest = cands[1]
	}

	delta := highest.threat - tm.effectiveThreatLocked(victim)
	if _, ok := tm.entries[victim]; !ok {
		// PutThreatListRef (ThreatManager.cpp:793) marks the client list
		// dirty on every new ref — even when the delta below no-ops.
		tm.entries[victim] = 0
		tm.needClientUpdate = true
	}
	// ThreatReference::AddThreat (ThreatManager.cpp:38-47): a zero delta is
	// a no-op with no dirty mark; a negative delta floors the base at 0;
	// any nonzero write marks the client list dirty (ThreatManager.cpp:49).
	if delta != 0 {
		newThreat := tm.entries[victim] + delta
		if newThreat < 0 {
			newThreat = 0
		}
		tm.entries[victim] = newThreat
		tm.needClientUpdate = true
	}
	// Victim selection (ThreatManager.cpp:379-410): the AddThreat route
	// reselects only when there is no current victim; with one, the
	// victim stays until the Update tick's taunt-precedence reselect.
	if tm.currentVictim == 0 {
		if nv := tm.reselectVictimLocked(nil); nv != 0 && nv != tm.currentVictim {
			tm.currentVictim = nv
			return true, nv
		}
	}
	return false, tm.currentVictim
}

// RemoveThreat removes a victim from the threat table and re-evaluates the top
// victim through ReselectVictim (ThreatManager.cpp:531-585, reached via
// ClearThreat's UpdateVictim leg): taunt precedence first, then threat, with
// the fixated victim preferred — not a raw effective-threat max, which
// ignored both taunt state and fixate.
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
		tm.currentVictim = tm.reselectVictimLocked(nil)
		return true, tm.currentVictim
	}
	return false, tm.currentVictim
}

// ScaleThreat multiplies a victim's threat entry by factor.
// Reference: ThreatReference::ScaleThreat (ThreatManager.cpp:52-61) no-ops on a
// factor of 1 and clamps negative factors at 0; ThreatManager::ScaleThreat
// (ThreatManager.cpp:412-417) only touches an existing entry, so a victim with
// no entry is a no-op. C++ selects no victim here — reselect happens only in
// UpdateVictim on the 1s tick — so the returned victim is always the current
// one and the tick broadcasts any switch; the old eager max-pick ignored
// taunt precedence, fixate and the 110%/130% gates.
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
	return false, tm.currentVictim
}

// handleEffectModifyThreatPercent processes SPELL_EFFECT_MODIFY_THREAT_PERCENT (125).
// Reference: Spell::EffectModifyThreatPercent (SpellEffects.cpp:4915) runs at
// SPELL_EFFECT_HANDLE_HIT_TARGET per unit target:
// unitTarget->GetThreatManager().ModifyThreatByPercent(unitCaster, damage),
// where ModifyThreatByPercent (ThreatManager.h:143) no-ops on percent 0 and
// scales by 0.01*(100+percent). The CanHaveThreatList gate
// (ThreatManager.cpp:156-179) skips pets, totems, triggers and
// player-summoned minions/guardians; charmed creatures keep their lists,
// so they are not skipped — ScaleThreat only touches an existing entry,
// so the gate is outcome-equivalent to C++'s unguarded call.
func (s *session) handleEffectModifyThreatPercent(ctx context.Context, spell wotlk.Spell, eff wotlk.SpellEffect, hitTargets []uint64) {
	if s == nil || s.server == nil || s.player == nil {
		return
	}
	damage := eff.CalcValueForLevel(spell, uint32(s.player.Level))
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
		if motion == nil || motion.Evading || !motionCanHaveThreatList(motion) || motion.ThreatMgr == nil {
			s.server.motionMu.Unlock()
			continue
		}
		tm := motion.ThreatMgr
		s.server.motionMu.Unlock()

		// ScaleThreat selects no victim (ThreatManager.cpp:412-417); the
		// 1s Update tick reselects and broadcasts any switch.
		tm.ScaleThreat(s.playerGUID, factor)
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

// ResetThreat zeroes every threat value while keeping the entries, so the
// owner keeps its victim list and does not evade. It mirrors TC's
// ThreatReference::ScaleThreat(0.0f), which scales only _baseAmount — the Go
// entries map holds _baseAmount (tempMods mirrors _tempModifier), so temp
// modifiers, the current victim and taunt state are left untouched.
// Reference: TrinityCore ThreatManager::ResetAllThreat via
// ScriptedAI::ResetThreatList.
func (tm *ThreatManager) ResetThreat() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for guid := range tm.entries {
		tm.entries[guid] = 0
	}
}

// ClearThreat wipes the threat table on evade or death.
// Reference: TrinityCore ThreatManager::ClearAllThreat.
func (tm *ThreatManager) ClearThreat() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.entries = make(map[uint64]float32)
	tm.tempMods = make(map[uint64]int32)
	tm.suppressed = make(map[uint64]bool)
	tm.currentVictim = 0
	tm.fixateGUID = 0
	tm.tauntExpiry = make(map[uint64]time.Time)
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

// Threat broadcast helpers
// broadcastThreatUpdateInInstance fans SMSG_THREAT_UPDATE out to the
// visibility set around the threat owner (ThreatManager::SendThreatListToClients,
// ThreatManager.cpp:770-788: _owner->SendMessageToSet(&data, false)); the
// source is nil because the owner is a creature, so no player self leg exists.
func (s *Server) broadcastThreatUpdateInInstance(mapID, instanceID uint32, x, y, z float32, creatureGUID uint64, list []protocol.ThreatEntry) {
	payload := protocol.BuildThreatUpdate(creatureGUID, list)
	s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_THREAT_UPDATE), payload, mapID, instanceID, x, y, z, nil)
}

func (s *Server) broadcastHighestThreatUpdateInInstance(mapID, instanceID uint32, x, y, z float32, creatureGUID, highestGUID uint64, list []protocol.ThreatEntry) {
	payload := protocol.BuildHighestThreatUpdate(creatureGUID, highestGUID, list)
	s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_HIGHEST_THREAT_UPDATE), payload, mapID, instanceID, x, y, z, nil)
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

func (s *Server) broadcastThreatRemoveInInstance(mapID, instanceID uint32, x, y, z float32, creatureGUID, victimGUID uint64) {
	payload := protocol.BuildThreatRemove(creatureGUID, victimGUID)
	s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_THREAT_REMOVE), payload, mapID, instanceID, x, y, z, nil)
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
		s.broadcastThreatRemoveInInstance(mapID, instanceID, motion.X, motion.Y, motion.Z, ownerGUID, victimGUID)
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
// TELE_TO_SPELL still drops combat at Player.cpp:1753). The exit leg also
// carries CombatManager::UpdateOwnerCombatState (CombatManager.cpp): the
// in-combat flag clears and Unit::AtExitCombat (Unit.cpp:8964) strips
// AURA_INTERRUPT_FLAG_LEAVE_COMBAT auras, both gated on the flag being set
// (C++ runs AtExitCombat only on an actual combat-state flip).
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
	// CombatManager::UpdateOwnerCombatState (CombatManager.cpp) exit leg:
	// leaving combat clears UNIT_FLAG_IN_COMBAT and runs Unit::AtExitCombat
	// (Unit.cpp:8964), which strips AURA_INTERRUPT_FLAG_LEAVE_COMBAT auras
	// (RemoveAurasWithInterruptFlags). The flag itself is the flip gate —
	// C++ only runs AtExitCombat when the combat state actually changes
	// (combatState != IsInCombat()), so a non-combat logout/teleport strips
	// nothing. AtDisengage is a no-op virtual (Unit.h:1792).
	if s.player.UnitFlags&unitFlagInCombat != 0 {
		s.player.UnitFlags &^= unitFlagInCombat
		s.sendPlayerUpdate()
		s.removeAurasWithInterruptFlags(auraInterruptFlagLeaveCombat)
		// Player::AtExitCombat (Player.cpp:24573-24577): Death Knight rune
		// grace windows reset on leaving combat.
		if s.player.Class == classDeathKnight && s.runes != nil {
			for i := range s.runes.graceStartMs {
				s.runes.graceStartMs[i] = -1
			}
		}
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

// removeCreatureAttackers mirrors the RemoveAllAttackers half of
// Unit::CombatStop (Unit.cpp:5815, 5867-5878): every attacker of the given
// creature stops attacking it. Creature attackers (motion.TargetGUID ==
// guid, pets included — pets ride the motion map) drop their victim and
// get SMSG_ATTACK_STOP for the (attacker, victim) pair plus the visual
// target clear; player attackers get the session-side attack stop
// (sendAttackStop + attackTarget clear, the damage_shield death
// precedent). Attackers keep their own threat tables and InCombat here
// — the EndAllPvECombat legs (own-table clear, RemoveMeFromThreatLists)
// are the CombatStop caller's job, not this helper's, so
// RemoveAllAttackers stays usable on its own (freya's own JustDied calls
// it without CombatStop).
func (s *Server) removeCreatureAttackers(mapID, instanceID uint32, guid uint64) {
	if s == nil || guid == 0 {
		return
	}
	var attackers []uint64
	s.motionMu.Lock()
	for mGUID, m := range s.motionMapLocked(mapID, instanceID) {
		if m == nil || m.TargetGUID != guid {
			continue
		}
		attackers = append(attackers, mGUID)
	}
	s.motionMu.Unlock()
	for _, attackerGUID := range attackers {
		s.motionMu.Lock()
		m := s.findCreatureMotionLocked(mapID, instanceID, attackerGUID)
		if m == nil || m.TargetGUID != guid {
			s.motionMu.Unlock()
			continue
		}
		m.TargetGUID = 0
		s.motionMu.Unlock()
		stopPkt := buildAttackStop(attackerGUID, guid, false)
		s.broadcastToVisibleSetForCreature(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, mapID, instanceID, attackerGUID)
		if fresh := s.findCreatureMotion(mapID, instanceID, attackerGUID); fresh != nil {
			s.clearCreatureVisualTarget(fresh)
		}
	}
	s.sessionsMu.RLock()
	var players []*session
	for sess := range s.sessions {
		if sess == nil || !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID || sess.player.InstanceID != instanceID || sess.attackTarget != guid {
			continue
		}
		players = append(players, sess)
	}
	s.sessionsMu.RUnlock()
	for _, sess := range players {
		if sess.attackTarget != guid {
			continue
		}
		_ = sess.sendAttackStop(guid, false)
		sess.attackTarget = 0
	}
}

// zeroThreatOnAllLists mirrors the dungeon/non-player arm of
// Spell::EffectSanctuary (SpellEffects.cpp:3767-3770): every threat list in
// the map/instance carrying the unit scales its entry to 0
// (ThreatReference::ScaleThreat, ThreatManager.cpp:52-61 — the entry stays,
// the value zeroes); the victim reselect surfaces on the 1s Update tick
// (ThreatManager::UpdateVictim), which broadcasts the switch.
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
		// ScaleThreat selects no victim (ThreatManager.cpp:412-417); the
		// 1s Update tick reselects and broadcasts any switch.
		motion.ThreatMgr.ScaleThreat(targetGUID, 0)
	}
}

func (s *Server) broadcastThreatClearInInstance(mapID, instanceID uint32, x, y, z float32, creatureGUID uint64) {
	payload := protocol.BuildThreatClear(creatureGUID)
	s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_THREAT_CLEAR), payload, mapID, instanceID, x, y, z, nil)
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
		// Generic SPELL_AURA_MOD_THREAT (AuraType 10) — mirrors
		// Unit::GetTotalAuraMultiplierByMiscMask (Unit.cpp:4947-4956): only
		// effects whose MiscValue school mask overlaps the queried school
		// mask multiply in (AddPct per effect). Righteous Fury (25780) is
		// aura-10 with MiscValue=2 (holy) and amount 80 — the old ungated arm
		// applied its +80% to every school, and the hardcoded switch below
		// doubled it to x3.24 on holy; both deviations are fixed here.
		if a.AuraType == 10 {
			if a.MiscValue&int32(schoolMask) != 0 {
				mult *= (1.0 + float32(int32(a.Amount))/100.0)
			}
			continue
		}
		// Hardcoded spell-ID threat modifiers for spells whose 3.3.5a DBC
		// carries no SPELL_AURA_MOD_THREAT effect of its own (verified per
		// spell against the 3.3.5a Spell.dbc). The SameEffectStackRule
		// spell-group refinement (Unit.cpp:4871-4880) stays unmodeled — Go
		// has no spell-group model.
		switch a.SpellID {
		case 71: // Warrior: Defensive Stance (+45% threat)
			// The +45% lives on the separate 7376 (Defensive Stance Passive,
			// aura-10 MiscValue=127 amount 45), which Go never applies — the
			// hardcoded x1.45 replicates its exact net effect.
			mult *= 1.45
		case 5487, 9634: // Druid: Bear Form / Dire Bear Form (+30% threat)
			// No aura-10 effect on the stance spells in the 3.3.5a DBC; a
			// possible separate threat passive is unverified for 3.3.5a, so
			// the x1.30 stands as the approximation (revisit if 21178 or a
			// 3.3.5a equivalent is confirmed).
			mult *= 1.30
		case 1038: // Paladin: Hand of Salvation (-20% threat)
			// No aura-10 effect (per-tick -2% total-threat via triggered
			// casts, unmodeled); x0.80 approximates the 10s cumulative
			// (-2%/tick compounds to ~0.817).
			mult *= 0.80
		}
		// Removed: 2457/2458 Battle/Berserker Stance (x0.80) — TBC-era
		// deviation with no 3.3.5a DBC threat effect; C++ gives 1.0.
		// Removed: 25780 Righteous Fury — double-applied with the generic
		// arm; now handled exactly by the MiscValue-gated arm above.
	}
	if mult < 0.1 {
		mult = 0.1
	}
	return mult
}

// creatureThreatMultiplier mirrors getThreatMultiplier for a creature
// attacker: ThreatManager::CalculateModifiedThreat (ThreatManager.cpp:606-659)
// takes the attacker as its unit argument, so the school multipliers come
// from the attacker's own SPELL_AURA_MOD_THREAT (aura 10) auras, gated on
// the MiscValue school mask exactly like the player twin. Creature
// attackers carry no player stances, so the hardcoded stance arms of the
// player twin do not apply here; the 0.1 floor matches the player twin.
func (s *Server) creatureThreatMultiplier(mapID, instanceID uint32, guid uint64, schoolMask uint32) float32 {
	if s == nil {
		return 1.0
	}
	mult := float32(1.0)
	key := creatureAuraKey{Map: mapID, InstanceID: instanceID, GUID: guid}
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	if s.activeCreatureAuras != nil {
		if auras, ok := s.activeCreatureAuras[key]; ok {
			for _, a := range auras {
				if a == nil || a.Stopped {
					continue
				}
				if a.AuraType == 10 {
					if a.MiscValue&int32(schoolMask) != 0 {
						mult *= (1.0 + float32(int32(a.Amount))/100.0)
					}
				}
			}
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
		s.server.broadcastHighestThreatUpdateInInstance(motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.GUID, newVictim, entries)
	}
	motion.InCombat = true
	motion.Moving = true
}

// distributeHealingThreat calculates and splits healing threat among all creatures
// currently in combat with the healer or the heal target.
// Formula mirrors TrinityCore Unit::SendHealSpellLog and Unit::DoAttack (Unit.cpp:6550-6580):
// Base threat = effectiveHeal * 0.5 * healerThreatMultiplier.
// Threat is divided equally by the number of engaged creatures.
func (s *Server) distributeHealingThreat(ctx context.Context, healerGUID, targetGUID uint64, effectiveHeal uint32, ignoreModifiers bool, spellID uint32) {
	if s == nil || healerGUID == 0 || effectiveHeal == 0 {
		return
	}

	// ThreatManager::ForwardThreatForAssistingMe step 1
	// (ThreatManager.cpp:664-665): a SPELL_ATTR1_NO_THREAT spell generates no
	// assist threat at all.
	if spellHasNoThreatAttr(s.Data, spellID) {
		return
	}

	healerSess := s.findSessionByGUID(healerGUID)
	if healerSess == nil || healerSess.player == nil {
		return
	}

	// ThreatManager::AddThreat / ForwardThreatForAssistingMe take an
	// ignoreModifiers flag: energize gains (mana leech, innervate-style
	// obs-mod-power ticks) pass true, heal paths pass false.
	mult := float32(1.0)
	if !ignoreModifiers {
		mult = healerSess.getThreatMultiplier(2) // Holy/Healing school mask = 2
	}
	totalThreat := float32(effectiveHeal) * 0.5 * mult
	if totalThreat <= 0 {
		return
	}

	s.distributeHealingThreatLocked(healerSess, healerSess.player.Map, healerSess.player.InstanceID,
		healerSess.player.X, healerSess.player.Y, healerSess.player.Z, healerSess.player.CombatReach,
		healerGUID, targetGUID, totalThreat)

	if healerSess.player.UnitFlags&unitFlagInCombat == 0 {
		healerSess.player.UnitFlags |= unitFlagInCombat
		healerSess.sendPlayerUpdate()
	}
}

// distributeCreatureHealingThreat runs the same ForwardThreatForAssistingMe
// distribution for a creature healer (SpellAuraEffects.cpp:5411-5412 fires for
// creature casters too). Creature healers carry no healing-threat-multiplier
// model, so the factor is 1.0 and there is no in-combat flag leg.
func (s *Server) distributeCreatureHealingThreat(ctx context.Context, mapID, instanceID uint32, healerGUID, targetGUID uint64, effectiveHeal uint32, spellID uint32) {
	if s == nil || healerGUID == 0 || effectiveHeal == 0 {
		return
	}

	// ThreatManager::ForwardThreatForAssistingMe step 1
	// (ThreatManager.cpp:664-665): a SPELL_ATTR1_NO_THREAT spell generates no
	// assist threat at all.
	if spellHasNoThreatAttr(s.Data, spellID) {
		return
	}
	s.motionMu.Lock()
	healer := s.findCreatureMotionLocked(mapID, instanceID, healerGUID)
	// SpellAuraEffects.cpp:5576 — the threat forward fires for any
	// non-null caster (if (caster)); a dead creature caster still
	// forwards, so there is no alive gate here.
	if healer == nil {
		s.motionMu.Unlock()
		return
	}
	hx, hy, hz, hReach := healer.X, healer.Y, healer.Z, healer.CombatReach
	s.motionMu.Unlock()
	s.distributeHealingThreatLocked(nil, mapID, instanceID, hx, hy, hz, hReach, healerGUID, targetGUID, float32(effectiveHeal)*0.5)
}

// forwardPetAssistThreat runs the Spell::PreprocessSpellHit friendly-assist
// arm (Spell.cpp:2761-2765) for a pet cast: a hit on a friendly in-combat
// target by a spell with initial aggro forwards 0.0 threat from the original
// caster to every creature threatening the target, and a player-controlled
// original caster inherits the target's combat states. The Go pet path
// carries no originalCaster distinct from the caster (direct casts default
// it to the caster, Spell.cpp:566-568, and EffectTriggerSpell builds its
// Spell without an originalCaster argument, SpellEffects.cpp:2884), so the
// pet motion is the assistant. The 0.0 forward reuses
// distributeHealingThreatLocked: a zero total splits to zero shares and the
// per-recipient AddThreat still creates the threat ref and engages combat
// (ThreatManager.cpp:308-410), which is the arm's whole effect — unlike the
// heal forward, there is no zero-amount early return here. The inherit leg
// has no per-pair CombatManager model in Go (the player path's
// forwardInitialAssistThreatLocked uses the same flag approximation), so it
// sets the pet's InCombat flag; the forward's own AddThreats engage the pet
// with each creature recipient, and the InheritCombatStatesFrom immunity
// legs (CombatManager.cpp:270-290, IsImmuneToPC/NPC) have no Go model and
// are not replicated. The forward runs even when the inherit gate is off
// (a non-player-controlled caster still forwards; C++: "for creature v
// creature combat, the threat forward does it for us"). Caller holds no
// locks; the friendly and in-combat reads lock internally like the heal
// forward's snapshot, so no motionMu is held across them.
func (s *Server) forwardPetAssistThreat(caster *creatureMotion, targetGUID uint64, spell wotlk.Spell) {
	if s == nil || caster == nil || caster.GUID == 0 || targetGUID == 0 || targetGUID == caster.GUID {
		return
	}
	// SpellInfo::HasInitialAggro (SpellInfo.cpp:1261): NO_THREAT or
	// NO_INITIAL_AGGRO spells never reach the arm.
	if !spellHasInitialThreat(spell) {
		return
	}
	// The arm lives in the friendly branch of PreprocessSpellHit: a valid
	// attack target takes the interrupt-flags leg instead, never this one.
	if !s.creatureFriendlyToTarget(caster, targetGUID) {
		return
	}
	// unit->IsInCombat(): player targets read the session flag/attack
	// target, creature targets the motion flag — the unitInCombatLocked
	// shape without holding motionMu across the friendly read above.
	// C++ threat refs are map-local, so a player target on another
	// map/instance is not a hit target here.
	targetInCombat := false
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		if ts.player.Map == caster.Map && ts.player.InstanceID == caster.InstanceID {
			targetInCombat = ts.attackTarget != 0 || ts.player.UnitFlags&unitFlagInCombat != 0
		}
	} else if tm := s.findCreatureMotion(caster.Map, caster.InstanceID, targetGUID); tm != nil {
		targetInCombat = tm.InCombat
	}
	if !targetInCombat {
		return
	}
	s.motionMu.Lock()
	motion := s.findCreatureMotionLocked(caster.Map, caster.InstanceID, caster.GUID)
	if motion == nil {
		s.motionMu.Unlock()
		return
	}
	cx, cy, cz, cReach := motion.X, motion.Y, motion.Z, motion.CombatReach
	playerControlled := motion.UnitFlags&unitFlagPlayerControlled != 0
	// CombatManager::InheritCombatStatesFrom (CombatManager.cpp:270-290),
	// gated on the original caster's PLAYER_CONTROLLED flag: true pets
	// and player-charmed motions carry it, so the pet enters combat with
	// the target's attackers; the flag approximation stands in for the
	// per-pair refs Go does not model.
	if playerControlled {
		motion.InCombat = true
	}
	s.motionMu.Unlock()
	s.distributeHealingThreatLocked(nil, caster.Map, caster.InstanceID, cx, cy, cz, cReach, caster.GUID, targetGUID, 0)
}

// flagPetAssistContestedPvP runs the Spell::PreprocessSpellHit contested-PvP
// arm (Spell.cpp:2752-2757) for a pet cast: a hit on a friendly target
// carrying UNIT_STATE_ATTACK_PLAYER flags the caster's charmer/owner player
// contested (SetContestedPvP) and PvP-flagged (UpdatePvP(true)). Unlike the
// 2761 forward above it, this arm is NOT gated on initial aggro, so it runs
// its own path in the same per-target loop. Go has no per-unit
// ATTACK_PLAYER flag; the session flag+timer pair is its model
// (SetContestedPvP sets both together, ResetContestedPvP clears both,
// Player.cpp:20725-20766): a player target reads contestedPvPActive, and a
// creature target reads the flag through its charmer/owner session —
// SetContestedPvP fans ATTACK_PLAYER out to the player's controlled units,
// so a controlled creature of a contested player carries it, while a wild
// creature (no charmer, no owner session) never does. The caster-side player
// is the session running the cast (owner for true pets, charmer for
// possessed wild, matching GetCharmerOrOwnerPlayerOrPlayerItself). The
// guard-relocation notifier has no Go twin (contested-guard hostility reads
// contestedPvPActive live), and SetPvP's controlled-unit fan-out is the
// player flag alone (characters.go:1608 precedent). No caster==target gate:
// C++ has none for this leg, and a self-hit only refreshes the timer when
// the owner is already contested.
func (s *Server) flagPetAssistContestedPvP(owner *session, caster *creatureMotion, targetGUID uint64) {
	if s == nil || owner == nil || owner.player == nil || caster == nil || caster.GUID == 0 || targetGUID == 0 {
		return
	}
	// The arm lives in the friendly branch of PreprocessSpellHit.
	if !s.creatureFriendlyToTarget(caster, targetGUID) {
		return
	}
	now := time.Now()
	targetAttackPlayer := false
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		if ts.player.Map == caster.Map && ts.player.InstanceID == caster.InstanceID {
			targetAttackPlayer = ts.contestedPvPActive(now)
		}
	} else if tm := s.findCreatureMotion(caster.Map, caster.InstanceID, targetGUID); tm != nil {
		for _, guid := range []uint64{tm.CharmerGUID, tm.OwnerGUID} {
			if guid == 0 {
				continue
			}
			if os := s.findSessionByGUID(guid); os != nil && os.player != nil && os.player.Map == caster.Map && os.player.InstanceID == caster.InstanceID {
				targetAttackPlayer = os.contestedPvPActive(now)
				break
			}
		}
	}
	if !targetAttackPlayer {
		return
	}
	// Player::SetContestedPvP (Player.cpp:20725-20747): the 30s timer
	// refreshes unconditionally, the flag sets once.
	// Player::UpdatePvP(true) (Player.cpp:22196-22210): SetPvP(true) with
	// EndTimer = now, so the 5-minute toggle-off countdown runs from
	// this hit.
	changed := owner.player.PlayerFlags&playerFlagContestedPVP == 0 || owner.player.PVPFlags&pvpFlagPvP == 0
	owner.player.PlayerFlags |= playerFlagContestedPVP
	owner.contestedPVPEnd = now.Add(30 * time.Second)
	owner.player.PVPFlags |= pvpFlagPvP
	owner.pvpEnd = now
	if changed {
		owner.sendPlayerUpdate()
	}
}

// flagCreatureAssistContestedPvP runs the Spell::PreprocessSpellHit
// contested-PvP arm (Spell.cpp:2752-2757) for a boss_ai creature cast: a
// hit on a friendly target carrying UNIT_STATE_ATTACK_PLAYER flags the
// caster's charmer/owner player contested (SetContestedPvP) and PvP-flagged
// (UpdatePvP(true)). GetCharmerOrOwnerPlayerOrPlayerItself
// (Object.cpp:2226-2233, Unit.h:1170) resolves to the charmer's player when
// charmed, else the owner's player, and to nil for a wild creature (ToPlayer
// on a creature) or a creature-owned creature (a non-player GUID fails
// IsPlayer) — so wild and creature-owned casters take no leg. The
// caster-side session must be online on the caster's map/instance, the
// map-local session model the target read already uses. The FFA arm
// (Object.cpp GetReactionTo: both player-controlled units FFA-flagged read
// HOSTILE) is corrected here: the shared creatureFriendlyToTarget gate
// carries no FFA term, so an FFA caster-side player against an FFA player
// target skips the leg. The m_caster != unit gate lives in the call-site
// loop (self-casts and landed reflects retarget to the caster). The flag
// application itself rides flagPetAssistContestedPvP: a pet motion casting
// through the creature path resolves to the same owner session, and the
// set is idempotent (the packet fires only on a newly set bit), so an
// overlap with the pet path's own call site only refreshes the timer.
func (s *Server) flagCreatureAssistContestedPvP(caster *creatureMotion, targetGUID uint64) {
	if s == nil || caster == nil || caster.GUID == 0 || targetGUID == 0 {
		return
	}
	ownerGUID := caster.CharmerGUID
	if ownerGUID == 0 {
		ownerGUID = caster.OwnerGUID
	}
	if ownerGUID == 0 {
		return
	}
	owner := s.findSessionByGUID(ownerGUID)
	if owner == nil || owner.player == nil || owner.player.Map != caster.Map || owner.player.InstanceID != caster.InstanceID {
		return
	}
	if owner.player.PVPFlags&pvpFlagFFA != 0 {
		if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil && ts.player.PVPFlags&pvpFlagFFA != 0 {
			return
		}
	}
	s.flagPetAssistContestedPvP(owner, caster, targetGUID)
}

// forwardCreatureAssistThreat runs the Spell::PreprocessSpellHit
// friendly-assist forward (Spell.cpp:2761-2765) for a boss_ai creature
// cast: a hit on a friendly in-combat target by a spell with initial aggro
// forwards 0.0 threat from the original caster to every creature
// threatening the target, and a player-controlled original caster inherits
// the target's combat states. Go creature casts carry no originalCaster
// distinct from the caster (Spell ctor defaults it to the caster when no
// originalCasterGUID is supplied, Spell.cpp:565-575, and trigger builds
// pass none), so the motion itself is the assistant and its own
// PLAYER_CONTROLLED flag is the inherit gate (charm sets it,
// creaturemotion.go) — exactly the pet helper's model, reused here. The
// FFA correction mirrors flagCreatureAssistContestedPvP above: the shared
// creatureFriendlyToTarget gate carries no FFA term, so an FFA caster-side
// player against an FFA player target skips the leg, keeping both arms of
// the creature loop in agreement on the same target. The m_caster != unit
// gate lives in the call-site loop (self-casts and landed reflects
// retarget to the caster). A pet motion casting through the creature path
// as well as the pet path double-fires idempotently (0.0 adds, flag set).
func (s *Server) forwardCreatureAssistThreat(caster *creatureMotion, targetGUID uint64, spell wotlk.Spell) {
	if s == nil || caster == nil || caster.GUID == 0 || targetGUID == 0 {
		return
	}
	ownerGUID := caster.CharmerGUID
	if ownerGUID == 0 {
		ownerGUID = caster.OwnerGUID
	}
	if ownerGUID != 0 {
		if owner := s.findSessionByGUID(ownerGUID); owner != nil && owner.player != nil && owner.player.Map == caster.Map && owner.player.InstanceID == caster.InstanceID && owner.player.PVPFlags&pvpFlagFFA != 0 {
			if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil && ts.player.PVPFlags&pvpFlagFFA != 0 {
				return
			}
		}
	}
	s.forwardPetAssistThreat(caster, targetGUID, spell)
}

// flagPlayerAssistContestedPvP runs the Spell::PreprocessSpellHit
// contested-PvP arm (Spell.cpp:2752-2757) for a player cast: a hit on a
// friendly target carrying UNIT_STATE_ATTACK_PLAYER flags the caster
// contested (SetContestedPvP) and PvP-flagged (UpdatePvP(true)).
// GetCharmerOrOwnerPlayerOrPlayerItself is the player itself here, so
// the caster side is this session. The arm has no initial-aggro,
// positivity, or Speed gate: it fires for any spell hit, which is why
// callers run it from the PreprocessTarget position ahead of the effect
// loops, over live hit targets only (misses carry no _spellHitTarget,
// Spell.cpp:2347-2351; a clean reflect retargets to the caster, dropped
// by the self gate). The friendly gate is isFriendlyLaunchTarget, the
// established PreprocessSpellHit-arm twin (breakHitBySpellAuras
// precedent); its same-raid cross-faction and forced-rank arms are
// unmodeled there, so those targets read non-friendly here too. FFA is
// corrected in this helper: both players FFA-flagged read HOSTILE in
// C++ (Object.cpp GetReactionTo), not friendly. The target
// ATTACK_PLAYER read reuses the pet helper's model: a player target
// reads its own contested flag+timer, a creature target reads it
// through its charmer/owner session, a wild creature never carries it.
func (s *session) flagPlayerAssistContestedPvP(ctx context.Context, targetGUID uint64) {
	if s == nil || s.server == nil || s.player == nil || targetGUID == 0 || targetGUID == s.playerGUID {
		return
	}
	if !s.isFriendlyLaunchTarget(ctx, targetGUID) {
		return
	}
	now := time.Now()
	targetAttackPlayer := false
	if ts := s.server.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		if ts.player.Map == s.player.Map && ts.player.InstanceID == s.player.InstanceID {
			if s.player.PVPFlags&pvpFlagFFA != 0 && ts.player.PVPFlags&pvpFlagFFA != 0 {
				return
			}
			targetAttackPlayer = ts.contestedPvPActive(now)
		}
	} else if tm := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, targetGUID); tm != nil {
		for _, guid := range []uint64{tm.CharmerGUID, tm.OwnerGUID} {
			if guid == 0 {
				continue
			}
			if os := s.server.findSessionByGUID(guid); os != nil && os.player != nil && os.player.Map == s.player.Map && os.player.InstanceID == s.player.InstanceID {
				targetAttackPlayer = os.contestedPvPActive(now)
				break
			}
		}
	}
	if !targetAttackPlayer {
		return
	}
	// Player::SetContestedPvP (Player.cpp:20725-20747): the 30s timer
	// refreshes unconditionally, the flag sets once.
	// Player::UpdatePvP(true) (Player.cpp:22196-22210): SetPvP(true) with
	// EndTimer = now, so the 5-minute toggle-off countdown runs from
	// this hit.
	changed := s.player.PlayerFlags&playerFlagContestedPVP == 0 || s.player.PVPFlags&pvpFlagPvP == 0
	s.player.PlayerFlags |= playerFlagContestedPVP
	s.contestedPVPEnd = now.Add(30 * time.Second)
	s.player.PVPFlags |= pvpFlagPvP
	s.pvpEnd = now
	if changed {
		s.sendPlayerUpdate()
	}
}

// distributeHealingThreatLocked fans totalThreat out to every engaged creature
// on the map. healerSess is the player healer's session (nil for creature
// healers, which never register threat redirects). Callers hold no locks.
func (s *Server) distributeHealingThreatLocked(healerSess *session, mapID, instanceID uint32, healerX, healerY, healerZ, healerReach float32, healerGUID, targetGUID uint64, totalThreat float32) {
	s.motionMu.Lock()
	defer s.motionMu.Unlock()

	var engaged, controlled []*creatureMotion
	now := time.Now()
	for _, m := range s.motionMapLocked(mapID, instanceID) {
		if m == nil || m.Health == 0 || !m.InCombat || m.Evading {
			continue
		}
		// ThreatManager::ForwardThreatForAssistingMe (ThreatManager.cpp:662-688):
		// the recipient set is the assisted unit's _threatenedByMe — the
		// creatures that threaten it — not every creature engaged with the
		// assistant. C++ iterates ALL _threatenedByMe pairs with no
		// threat-value filter: zero-threat refs are recipients too (AddThreat
		// creates the ref even at 0, ThreatManager.cpp:308-410; negatives
		// floor at 0 keeping the entry). Go approximates that set as
		// creatures whose current target is the assisted unit or that hold
		// a threat-table entry (HasVictim tests entry existence, not a
		// positive value) for it. Combat-end ref cleanup
		// (CombatManager.cpp:68-73,347) is approximated by the in-combat /
		// alive / non-evading gates.
		isEngaged := false
		if m.TargetGUID == targetGUID {
			isEngaged = true
		} else if m.ThreatMgr != nil && targetGUID != 0 && m.ThreatMgr.HasVictim(targetGUID) {
			isEngaged = true
		}
		if !isEngaged {
			continue
		}
		// ThreatManager::ForwardThreatForAssistingMe (ThreatManager.cpp:666-672):
		// recipients under UNIT_STATE_CONTROLLED (Unit.h:251 — CONFUSED |
		// STUNNED | FLEEING; fear lands here via the fleeing leg) take a
		// zero add — engagement without threat — and are excluded from the
		// even-split denominator. Go reads the CC state from the
		// creature's live confuse/stun/fear auras plus the fleeing timer.
		key := creatureAuraKey{Map: m.Map, InstanceID: m.InstanceID, GUID: m.GUID}
		if (!m.FleeingUntil.IsZero() && now.Before(m.FleeingUntil)) ||
			s.creatureHasAuraType(key, spellAuraModConfuse) ||
			s.creatureHasAuraType(key, spellAuraModStun) ||
			s.creatureHasAuraType(key, spellAuraModFear) {
			controlled = append(controlled, m)
			continue
		}
		engaged = append(engaged, m)
	}

	if len(engaged) == 0 && len(controlled) == 0 {
		return
	}

	addAssistThreat := func(m *creatureMotion, amount float32) {
		if m.ThreatMgr == nil {
			m.ThreatMgr = NewThreatManager(m)
		}
		dist := distance3D(healerX, healerY, healerZ, m.X, m.Y, m.Z)
		inMelee := inMeleeThreatRange(m.CombatReach, healerReach, dist)
		// The per-recipient AddThreat runs the redirect leg against the
		// assistant's registry (ThreatManager.cpp:344-373, ignoreRedirects
		// stays false on the ForwardThreatForAssistingMe adds); the nil
		// session guard inside splitThreatRedirects covers creature healers,
		// which never register redirects.
		share, rSwitched, rVictim := healerSess.splitThreatRedirects(m, amount)
		switched, newVictim := m.ThreatMgr.AddThreat(healerGUID, share, inMelee)
		if rSwitched {
			switched, newVictim = true, rVictim
		}
		if switched && newVictim != m.TargetGUID {
			m.TargetGUID = newVictim
			entries := m.ThreatMgr.SortedEntries()
			s.broadcastHighestThreatUpdateInInstance(m.Map, m.InstanceID, m.X, m.Y, m.Z, m.GUID, newVictim, entries)
		}
		m.Moving = true
	}

	if len(engaged) > 0 {
		threatPerCreature := totalThreat / float32(len(engaged))
		for _, m := range engaged {
			addAssistThreat(m, threatPerCreature)
		}
	}
	// The controlled leg mirrors C++'s AddThreat(assistant, 0.0f, spell,
	// true): the zero add registers the threat ref and engages combat
	// (ThreatManager.cpp:308-410) without adding threat.
	for _, m := range controlled {
		addAssistThreat(m, 0)
	}
}
