package world

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// BossAI defines the interface for scripted boss encounters.
// Reference: TrinityCore BossAI.h / ScriptedCreature.h.
//
// Callback audit vs BossAI::_Reset/_JustDied/_JustReachedHome/_JustEngagedWith
// (ScriptedCreature.cpp:499-544), per implemented BossAI (item 6):
// _Reset (499-510) vs OnReset — fired at respawn (kill.go:865, mirroring
// ai->Reset() in Creature::Respawn, Creature.cpp:2198) and at evade via
// OnEvade (mirroring Reset() at the tail of CreatureAI::_EnterEvadeMode,
// CreatureAI.cpp:237); C++ runs no Reset at first spawn either, so Go's
// alive-only call sites match. The !IsAlive() early return is vacuous.
// SetCombatPulseDelay(0) is bridged (creature:SetCombatPulseDelay,
// lua_creature_events.go); ResetLootMode() has no Go model — no bridge.
// events.Reset() is bridged per-AI: every Go-native implementation re-arms
// its own flags/timers in OnReset (rhahkZor slamTimer, taragaman 2 timers,
// kresh shieldUsed); luaBossAI is a no-op — its Lua script owns state via
// the Lua On_Reset event (23) the engine fires (vancleef/mr_smite are
// Lua-driven; their former Go AIs were deleted as unreachable).
// summons.DespawnAll() for Lua-tracked summons rides the Lua event-4/23
// fires — vancleef's blackguards despawn on evade/death and Reset
// re-summons them via its SummonBlackguards arm (boss_vancleef.cpp:86-90,
// ported through the creature:SummonCreature bridge). scheduler.CancelAll()
// needs no bridge — the AIs are tick-driven and OnReset re-arms their
// timers. The instance SetBossState(NOT_STARTED) arm has no boss-state
// model; Go's analog is clearInstanceEncounter on evade
// (creaturemotion.go:626).
// _JustDied (512-518) vs OnDied — bridged: BossAI._JustDied's events.Reset()
// is covered per-AI (flags/timers linger until the respawn OnReset re-arms
// them; stale timers stay inert via the OnUpdate !InCombat guard the kill
// paths set), scheduler.CancelAll() needs no bridge (tick-driven AIs),
// and summons.DespawnAll() rides the Lua event-4 fire for Lua-tracked
// summons (vancleef's blackguards). The instance SetBossState(DONE) arm
// has no state model; the kill sites clear encounter-active tracking
// (combat.go:566/879, spells.go:7446/12646) — the pet owner-gone branch
// fires the Lua 23+4 pair itself before the native OnDied, so Lua summon
// cleanup runs there too. Lua keeps
// Eluna parity (fireCreatureDied fires Lua event 23 On_Reset then event 4
// On_Died) with the native hook fired right after, matching the
// Eluna-first/native-second order used at respawn (kill.go:865).
// _JustReachedHome (520-523) = me->setActive(false): no active model in Go;
// already documented at creaturemotion.go:659-671 (Eluna event 24 fires, the
// veto discarded as a provable no-op since only BossAI overrides
// JustReachedHome). Go-native AIs carry no home-arrival model.
// _JustEngagedWith (525-539) vs OnAggro (creaturemotion.go:443/1301):
// CheckRequiredBosses SEQUENCE_BREAK documented no-bridge; instance
// SetBossState(IN_PROGRESS) has no boss-state model — beginInstanceEncounter
// is raid-gated (IsRaid) and a no-op for all five Go bosses (Deadmines and
// Wailing Caverns are dungeons; only Lua-ported raid bosses use it);
// SetCombatPulseDelay(5) is bridged (creature:SetCombatPulseDelay,
// lua_creature_events.go); setActive(true) has no Go model — no bridge;
// DoZoneInCombat() has no engine analog — no bridge (the C++ vancleef
// summons.DoZoneInCombat() arm likewise; the Reset-summoned blackguards
// aggro via the proximity scan). ScheduleTasks() needs no aggro-time
// bridge: every
// Go AI arms its timers in its constructor, and getBossAIForCreature runs
// lazily at first engagement (creaturemotion.go:461 and the combat.go /
// spells.go combat sites), with OnUpdate decrementing only while InCombat —
// outcome-equivalent to C++ spawn-time arming. C++ boss_mr_smite's
// JustEngagedWith is empty (boss_mr_smite.cpp:105-107), so the Lua port
// likewise schedules nothing on engage. mr_smite's timers mirror
// Initialize() (boss_mr_smite.cpp:70-80): trash urand(5000,9000), slam
// 9000; repeat arms mirror UpdateAI (:125-139): trash urand(6000,15500),
// slam 11000, with the bCheckChances 84% cast gate (:109-114) bridged —
// the timer re-arms even when the cast is skipped, matching C++.
// rhahkZor (644) and
// taragaman (11520) have no script files in this C++ tree — their logic is
// TDB smart_scripts rows — so their constructor values (slam 12s; fire
// nova 8s init/9s repeat, uppercut 12s) stand as transcribed; SmartAI's
// AGGRO-event processing (SmartAI.cpp:562) cannot be verified against the
// missing rows, so no OnAggro re-arm is applied for them either.
type BossAI interface {
	OnReset(ctx context.Context, s *Server, motion *creatureMotion)
	OnAggro(ctx context.Context, s *Server, motion *creatureMotion, victim uint64)
	// OnDamageTaken mirrors UnitAI::DamageTaken (UnitAI.h:304): fired from
	// Unit::DealDamage (Unit.cpp:701) on the victim's AI for any attacker,
	// ahead of the tap/health/threat/rage legs. C++ passes damage as
	// uint32&, so the hook may adjust it — hence the pointer.
	OnDamageTaken(ctx context.Context, s *Server, motion *creatureMotion, attacker uint64, damage *uint32)
	// DamageDealt mirrors UnitAI::DamageDealt (UnitAI.h:300): fired from
	// Unit::DealDamage (Unit.cpp:704) on the attacker's AI after DamageTaken,
	// on the same mutable damage. No script in this tree overrides it, so
	// every AI keeps the base no-op.
	DamageDealt(ctx context.Context, s *Server, motion *creatureMotion, victimGUID uint64, damage *uint32)
	// HealReceived mirrors UnitAI::HealReceived (UnitAI.h:307): fired from
	// Unit::DealHeal (Unit.cpp:6302) on the victim's AI ahead of the
	// health gain, on the pre-clamp addhealth. C++ passes it as uint32&,
	// so the hook may adjust the amount — hence the pointer.
	HealReceived(ctx context.Context, s *Server, motion *creatureMotion, healerGUID uint64, addhealth *uint32)
	// HealDone mirrors UnitAI::HealDone (UnitAI.h:310): fired from
	// Unit::DealHeal (Unit.cpp:6304) on the healer's AI, same mutable
	// addhealth arm.
	HealDone(ctx context.Context, s *Server, motion *creatureMotion, victimGUID uint64, addhealth *uint32)
	OnKillPlayer(ctx context.Context, s *Server, motion *creatureMotion, victim uint64)
	OnEvade(ctx context.Context, s *Server, motion *creatureMotion)
	OnDied(ctx context.Context, s *Server, motion *creatureMotion)
	OnUpdate(ctx context.Context, s *Server, motion *creatureMotion, diff time.Duration, players []playerPos, now time.Time)
}

var (
	bossAIRegistry = make(map[string]func(*creatureMotion) BossAI)
	bossAIByEntry  = make(map[uint32]func(*creatureMotion) BossAI)
)

func RegisterBossAI(scriptName string, entry uint32, factory func(*creatureMotion) BossAI) {
	if scriptName != "" {
		bossAIRegistry[scriptName] = factory
	}
	if entry != 0 {
		bossAIByEntry[entry] = factory
	}
}

// fireDealHealAIHooks mirrors the AI-hook arm of Unit::DealHeal
// (Unit.cpp:6299-6304): the victim AI's HealReceived fires ahead of the
// health gain, then the healer AI's HealDone — in that C++ order, on the
// same pre-clamp addhealth. Both fire on a self-heal (C++ has no
// healer==victim filter). A nil motion or a motion with no BossAI carries
// no AI model, so the hook is skipped there (C++ always has an AI).
func fireDealHealAIHooks(ctx context.Context, s *Server, victim, healer *creatureMotion, healerGUID, victimGUID uint64, addhealth *uint32) {
	if victim != nil && victim.BossAI != nil {
		victim.BossAI.HealReceived(ctx, s, victim, healerGUID, addhealth)
	}
	if healer != nil && healer.BossAI != nil {
		healer.BossAI.HealDone(ctx, s, healer, victimGUID, addhealth)
	}
	// Lua-scripted bosses have no BossAI, so they observe heals only through
	// the Go-side heal event, fired after the native hooks so existing hook
	// order is undisturbed. Read-only: the event cannot rewrite addhealth.
	if victim != nil && addhealth != nil {
		s.fireCreatureHealReceived(ctx, victim, healer, *addhealth)
	}
}

// fireDealDamageAIHooks mirrors the AI-hook arm of Unit::DealDamage
// (Unit.cpp:701-704): the victim AI's DamageTaken fires first, then the
// attacker's DamageDealt — in that C++ order, on the same mutable damage,
// ahead of the health change. A nil motion or a motion with no BossAI
// carries no AI model, so the hook is skipped there (C++ always has an
// AI; players and pet motions have none in Go).
func fireDealDamageAIHooks(ctx context.Context, s *Server, victim, attacker *creatureMotion, attackerGUID, victimGUID uint64, damage *uint32) {
	if victim != nil && victim.BossAI != nil {
		victim.BossAI.OnDamageTaken(ctx, s, victim, attackerGUID, damage)
	}
	if attacker != nil && attacker.BossAI != nil {
		attacker.BossAI.DamageDealt(ctx, s, attacker, victimGUID, damage)
	}
}

func getBossAIForCreature(motion *creatureMotion, scriptName string) BossAI {
	if motion == nil {
		return nil
	}
	if scriptName != "" {
		if factory, ok := bossAIRegistry[scriptName]; ok {
			return factory(motion)
		}
	}
	if factory, ok := bossAIByEntry[motion.Entry]; ok {
		return factory(motion)
	}
	return nil
}

// evadeDespawn records a BossAI::_DespawnAtEvade removal awaiting its
// delayed respawn (ScriptedCreature.cpp:596-617: DespawnOrUnsummon(0s,
// delayToRespawn)). The motion is deleted from the world; the motion sweep
// skips the GUID while the record is pending so the spawn query does not
// recreate it early, and processEvadeDespawns re-arms it at expiry.
type evadeDespawn struct {
	GUID       uint64 // motion GUID (creatureWorldGUID)
	DBGuid     uint32 // creature.guid
	Entry      uint32
	Map        uint32
	InstanceID uint32
	At         time.Time
}

// despawnAtEvade ports the DespawnOrUnsummon(0s, delayToRespawn) half of
// BossAI::_DespawnAtEvade (ScriptedCreature.cpp:596-617): the creature
// leaves the world now and respawns after delay. The instance
// SetBossState(FAIL) arm has no Go model (no instance boss-state —
// documented at boss_ai.go:17-66). Threat teardown already ran in the evade
// funnel ahead of OnEvade.
func (s *Server) despawnAtEvade(ctx context.Context, m *creatureMotion, delay time.Duration) {
	if s == nil || m == nil {
		return
	}
	rec := evadeDespawn{
		GUID:       m.GUID,
		DBGuid:     uint32(m.GUID & 0x00FFFFFF),
		Entry:      uint32((m.GUID >> 24) & 0x00FFFFFF),
		Map:        m.Map,
		InstanceID: m.InstanceID,
		At:         time.Now().Add(delay),
	}
	s.motionMu.Lock()
	delete(s.motionMapLocked(m.Map, m.InstanceID), m.GUID)
	if s.evadeDespawns == nil {
		s.evadeDespawns = make(map[uint64]evadeDespawn)
	}
	s.evadeDespawns[m.GUID] = rec
	s.motionMu.Unlock()
	// The client drops the object now; processEvadeDespawns re-creates it
	// at expiry.
	destroyBuf := protocol.NewBuffer(9)
	destroyBuf.WriteU64(m.GUID)
	destroyBuf.WriteU8(0)
	s.broadcastToInstance(m.Map, m.InstanceID, uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroyBuf.Bytes(), nil)
	// Eluna::OnRemoveFromWorld (CreatureHooks.cpp:80-85,
	// CREATURE_EVENT_ON_REMOVE (37)) fires from Creature::RemoveFromWorld
	// (Creature.cpp:314) when a creature leaves the world.
	s.fireCreatureLuaEvent(ctx, m, scripting.CreatureEventOnRemove)
}

// processEvadeDespawns re-arms expired _DespawnAtEvade removals; called
// from the world tick next to processCreatureRespawns.
func (s *Server) processEvadeDespawns(ctx context.Context, now time.Time) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	s.motionMu.Lock()
	var due []evadeDespawn
	for guid, rec := range s.evadeDespawns {
		if !now.Before(rec.At) {
			due = append(due, rec)
			delete(s.evadeDespawns, guid)
		}
	}
	s.motionMu.Unlock()
	for _, rec := range due {
		s.respawnEvadedCreature(ctx, rec)
	}
}

// respawnEvadedCreature re-materializes a despawnAtEvade removal at its
// spawn position at full health — the DespawnOrUnsummon respawn half —
// with a create block so clients re-see the boss, then the Eluna spawn
// hook and the native AI reset (Creature::Respawn, Creature.cpp:2156-2215:
// ai->Reset()).
func (s *Server) respawnEvadedCreature(ctx context.Context, rec evadeDespawn) {
	var moveType, model, npcFlags int64
	var faction, dynFlags uint32
	var wander, walkSpeed, runSpeed, x, y, z, orientation, scale, mana float64
	err := s.WorldStore.DB.QueryRowContext(ctx,
		`SELECT c.MovementType, c.wander_distance, COALESCE(NULLIF(t.speed_walk, 0), 1.0), COALESCE(NULLIF(t.speed_run, 0), 1.14286),
			c.position_x, c.position_y, c.position_z, c.orientation,
			COALESCE(NULLIF(c.modelid, 0), NULLIF(t.modelid1, 0), NULLIF(t.modelid2, 0), NULLIF(t.modelid3, 0), NULLIF(t.modelid4, 0), 1),
			t.npcflag, t.scale, c.curmana, COALESCE(t.faction, 0), COALESCE(t.dynamicflags, 0)
			FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.guid = ?`,
		rec.DBGuid).Scan(&moveType, &wander, &walkSpeed, &runSpeed, &x, &y, &z, &orientation, &model, &npcFlags, &scale, &mana, &faction, &dynFlags)
	if err != nil {
		return
	}
	motion := s.motionFor(ctx, rec.DBGuid, rec.Entry, rec.Map, rec.InstanceID,
		float32(x), float32(y), float32(z), float32(orientation),
		uint32(moveType), wander, float32(walkSpeed), float32(runSpeed), 0)
	if motion == nil {
		return
	}
	motion.Health = motion.MaxHealth
	// Re-create the object client-side: build the spawn block from the DB
	// row overlaid with the live motion state.
	spawn := creatureSpawn{GUID: rec.DBGuid, Entry: rec.Entry, Map: rec.Map,
		X: float32(x), Y: float32(y), Z: float32(z), Orientation: float32(orientation),
		Model: uint32(model), NPCFlags: uint32(npcFlags), Scale: float32(scale), Mana: uint32(mana)}
	stats := s.loadCreatureStats(ctx, rec.Entry)
	spawn.MaxHealth, spawn.Health = stats.MaxHealth, stats.MaxHealth
	spawn.Faction, spawn.DynamicFlags = faction, dynFlags
	spawn.UnitFlags = stats.UnitFlags
	spawn.Level = stats.Level
	s.applyCreatureMotionToSpawn(playerState{Map: rec.Map, InstanceID: rec.InstanceID}, &spawn)
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(buildCreatureUpdate(ctx, spawn, nil))
	if packet, err := updates.BuildPacket(0); err == nil && packet != nil {
		s.broadcastToInstance(rec.Map, rec.InstanceID, packet.Opcode, packet.Payload.Bytes(), nil)
	}
	// motionFor already fired the Eluna spawn pair (23+5) when it created
	// the motion; the native AI reset still runs after, matching the
	// ElunaCreatureAI::JustAppeared sEluna-before-ScriptedAI order.
	if motion.BossAI != nil {
		motion.BossAI.OnReset(ctx, s, motion)
	}
}

// -------------------------------------------------------------
// Rhahk'Zor (Entry 644)
// Reference: smart_scripts for entry 644
// -------------------------------------------------------------
type rhahkZorAI struct {
	motion    *creatureMotion
	slamTimer time.Duration
}

func newRhahkZorAI(m *creatureMotion) BossAI {
	return &rhahkZorAI{
		motion:    m,
		slamTimer: 12 * time.Second,
	}
}

func (ai *rhahkZorAI) OnReset(ctx context.Context, s *Server, m *creatureMotion) {
	ai.slamTimer = 12 * time.Second
}

func (ai *rhahkZorAI) OnAggro(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
	// SAY_AGGRO = 0: "VanCleef pay big for you heads!"
	s.broadcastCreatureTalk(ctx, m.Map, m.GUID, m.Entry, "Rhahk'Zor", 0, 0)
}

func (ai *rhahkZorAI) OnDamageTaken(ctx context.Context, s *Server, m *creatureMotion, attacker uint64, damage *uint32) {
}

// UnitAI::HealReceived/HealDone base no-ops (UnitAI.h:307-310).
func (ai *rhahkZorAI) HealReceived(ctx context.Context, s *Server, m *creatureMotion, healerGUID uint64, addhealth *uint32) {
}

func (ai *rhahkZorAI) HealDone(ctx context.Context, s *Server, m *creatureMotion, victimGUID uint64, addhealth *uint32) {
}

// UnitAI::DamageDealt base no-op (UnitAI.h:300): fired on the attacker's
// AI from Unit::DealDamage (Unit.cpp:704). No script in this tree overrides
// it, so this stays a no-op like the heal legs above.
func (ai *rhahkZorAI) DamageDealt(ctx context.Context, s *Server, m *creatureMotion, victimGUID uint64, damage *uint32) {
}

func (ai *rhahkZorAI) OnKillPlayer(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
}

func (ai *rhahkZorAI) OnEvade(ctx context.Context, s *Server, m *creatureMotion) {
	ai.OnReset(ctx, s, m)
}

// OnDied mirrors BossAI::_JustDied (ScriptedCreature.cpp:512-518) — no
// per-AI cleanup needed (this AI tracks no summons).
func (ai *rhahkZorAI) OnDied(ctx context.Context, s *Server, m *creatureMotion) {
}

func (ai *rhahkZorAI) OnUpdate(ctx context.Context, s *Server, m *creatureMotion, diff time.Duration, players []playerPos, now time.Time) {
	if !m.InCombat || m.TargetGUID == 0 {
		return
	}
	ai.slamTimer -= diff
	if ai.slamTimer <= 0 {
		ai.slamTimer = 12 * time.Second
		s.castCreatureSpell(ctx, m, 6304, m.TargetGUID) // Rhahk'Zor Slam
	}
}

// -------------------------------------------------------------
// Taragaman the Hungerer (Entry 11520 - Ragefire Chasm)
// -------------------------------------------------------------
type taragamanAI struct {
	motion        *creatureMotion
	fireNovaTimer time.Duration
	uppercutTimer time.Duration
}

func newTaragamanAI(m *creatureMotion) BossAI {
	return &taragamanAI{
		motion:        m,
		fireNovaTimer: 8 * time.Second,
		uppercutTimer: 12 * time.Second,
	}
}

func (ai *taragamanAI) OnReset(ctx context.Context, s *Server, m *creatureMotion) {
	ai.fireNovaTimer = 8 * time.Second
	ai.uppercutTimer = 12 * time.Second
}

func (ai *taragamanAI) OnAggro(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
}

func (ai *taragamanAI) OnDamageTaken(ctx context.Context, s *Server, m *creatureMotion, attacker uint64, damage *uint32) {
}

// UnitAI::HealReceived/HealDone base no-ops (UnitAI.h:307-310).
func (ai *taragamanAI) HealReceived(ctx context.Context, s *Server, m *creatureMotion, healerGUID uint64, addhealth *uint32) {
}

func (ai *taragamanAI) HealDone(ctx context.Context, s *Server, m *creatureMotion, victimGUID uint64, addhealth *uint32) {
}

// UnitAI::DamageDealt base no-op (UnitAI.h:300): fired on the attacker's
// AI from Unit::DealDamage (Unit.cpp:704). No script in this tree overrides
// it, so this stays a no-op like the heal legs above.
func (ai *taragamanAI) DamageDealt(ctx context.Context, s *Server, m *creatureMotion, victimGUID uint64, damage *uint32) {
}

func (ai *taragamanAI) OnKillPlayer(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
}

func (ai *taragamanAI) OnEvade(ctx context.Context, s *Server, m *creatureMotion) {
	ai.OnReset(ctx, s, m)
}

// OnDied mirrors BossAI::_JustDied (ScriptedCreature.cpp:512-518) — no
// per-AI cleanup needed (this AI tracks no summons).
func (ai *taragamanAI) OnDied(ctx context.Context, s *Server, m *creatureMotion) {
}

func (ai *taragamanAI) OnUpdate(ctx context.Context, s *Server, m *creatureMotion, diff time.Duration, players []playerPos, now time.Time) {
	if !m.InCombat || m.TargetGUID == 0 {
		return
	}
	ai.fireNovaTimer -= diff
	if ai.fireNovaTimer <= 0 {
		ai.fireNovaTimer = 9 * time.Second
		s.castCreatureSpell(ctx, m, 11970, m.GUID) // Fire Nova
	}

	ai.uppercutTimer -= diff
	if ai.uppercutTimer <= 0 {
		ai.uppercutTimer = 12 * time.Second
		s.castCreatureSpell(ctx, m, 18072, m.TargetGUID) // Uppercut
	}
}

// -------------------------------------------------------------
// Kresh (Entry 3653 - Wailing Caverns)
// -------------------------------------------------------------
type kreshAI struct {
	motion     *creatureMotion
	shieldUsed bool
}

func newKreshAI(m *creatureMotion) BossAI {
	return &kreshAI{motion: m}
}

func (ai *kreshAI) OnReset(ctx context.Context, s *Server, m *creatureMotion) {
	ai.shieldUsed = false
}

func (ai *kreshAI) OnAggro(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
}

func (ai *kreshAI) OnDamageTaken(ctx context.Context, s *Server, m *creatureMotion, attacker uint64, damage *uint32) {
	if m.MaxHealth == 0 {
		return
	}
	hpPct := float32(m.Health) / float32(m.MaxHealth) * 100.0
	if !ai.shieldUsed && hpPct <= 20.0 {
		ai.shieldUsed = true
		s.castCreatureSpell(ctx, m, 8269, m.GUID) // Shell Shield
	}
}

// UnitAI::HealReceived/HealDone base no-ops (UnitAI.h:307-310).
func (ai *kreshAI) HealReceived(ctx context.Context, s *Server, m *creatureMotion, healerGUID uint64, addhealth *uint32) {
}

func (ai *kreshAI) HealDone(ctx context.Context, s *Server, m *creatureMotion, victimGUID uint64, addhealth *uint32) {
}

// UnitAI::DamageDealt base no-op (UnitAI.h:300): fired on the attacker's
// AI from Unit::DealDamage (Unit.cpp:704). No script in this tree overrides
// it, so this stays a no-op like the heal legs above.
func (ai *kreshAI) DamageDealt(ctx context.Context, s *Server, m *creatureMotion, victimGUID uint64, damage *uint32) {
}

func (ai *kreshAI) OnKillPlayer(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
}

func (ai *kreshAI) OnEvade(ctx context.Context, s *Server, m *creatureMotion) {
	ai.OnReset(ctx, s, m)
}

// OnDied mirrors BossAI::_JustDied (ScriptedCreature.cpp:512-518) — no
// per-AI cleanup needed (this AI tracks no summons).
func (ai *kreshAI) OnDied(ctx context.Context, s *Server, m *creatureMotion) {
}

func (ai *kreshAI) OnUpdate(ctx context.Context, s *Server, m *creatureMotion, diff time.Duration, players []playerPos, now time.Time) {
}

// castCreatureSpell casts a spell from creature to target or self
func (s *Server) castCreatureSpell(ctx context.Context, m *creatureMotion, spellID uint32, targetGUID uint64) {
	s.castCreatureSpellInternal(ctx, m, spellID, targetGUID, false)
}

// castCreatureSpellTriggered is the TRIGGERED_IGNORE_POWER_AND_REAGENT_COST
// path (Spell.cpp:3449-3453): a triggered cast skips CheckPower/TakePower.
// The AICOND_DIE death cast (CombatAI.cpp:68-73, kill.go) rides this.
func (s *Server) castCreatureSpellTriggered(ctx context.Context, m *creatureMotion, spellID uint32, targetGUID uint64) {
	s.castCreatureSpellInternal(ctx, m, spellID, targetGUID, true)
}

const (
	unitFlagPossessed   uint32 = 0x01000000 // UNIT_FLAG_POSSESSED (UnitDefines.h:148)
	creaturePowerHealth uint32 = 0xFFFFFFFE // POWER_HEALTH (-2 as signed, Powers enum)
	creaturePowerMana   uint32 = 0          // POWER_MANA
	creaturePowerRage   uint32 = 1          // POWER_RAGE
	creaturePowerFocus  uint32 = 2          // POWER_FOCUS
	creaturePowerEnergy uint32 = 3          // POWER_ENERGY
	creaturePowerRune   uint32 = 5          // POWER_RUNE
	creatureMaxPowers   uint32 = 7          // MAX_POWERS
)

// creaturePower reads the motion's current power for a 0..MAX_POWERS type.
func creaturePower(m *creatureMotion, powerType uint32) uint32 {
	if m == nil || powerType >= uint32(len(m.Powers)) {
		return 0
	}
	return m.Powers[powerType]
}

// creatureSpellPowerCost mirrors SpellInfo::CalcPowerCost
// (SpellInfo.cpp:3154-3241) for creature casters: base ManaCost, the
// ManaCostPercentage of the power's max (POWER_HEALTH from max health,
// POWER_MANA from create mana, rage/focus/energy/happiness from max power),
// and the SPELL_ATTR0_LEVEL_DAMAGE_CALCULATION GtNPCManaCostScaler for
// non-player-controlled casters. The UNIT_FIELD_POWER_COST_MODIFIER flat add,
// the Shiv energy leg, SPELLMOD_COST, the cost multiplier, and the npcbot
// legs are player/aura-side and stay 0 for creatures; RUNE/RUNIC_POWER pct
// costs are unimplemented in C++ too ("Not implemented yet!").
func (s *Server) creatureSpellPowerCost(spell wotlk.Spell, m *creatureMotion) int32 {
	powerType := spell.PowerType
	if spell.AttributesEx&spellAttr1DrainAllPower != 0 {
		if powerType == creaturePowerHealth {
			if m == nil {
				return 0
			}
			return int32(m.Health)
		}
		if powerType < creatureMaxPowers {
			return int32(creaturePower(m, powerType))
		}
		return 0
	}
	powerCost := int32(spell.ManaCost)
	if spell.ManaCostPct != 0 && m != nil {
		var base uint32
		switch powerType {
		case creaturePowerHealth:
			base = m.MaxHealth
		case creaturePowerMana:
			if len(m.MaxPowers) > 0 {
				base = m.MaxPowers[0]
			}
		case creaturePowerRage, creaturePowerFocus, creaturePowerEnergy, 4: // POWER_HAPPINESS
			if powerType < uint32(len(m.MaxPowers)) {
				base = m.MaxPowers[powerType]
			}
		}
		powerCost += int32(uint64(base) * uint64(spell.ManaCostPct) / 100)
	}
	// GtNPCManaCostScaler (SpellInfo.cpp:3228-3235): NPC spells carrying
	// SPELL_ATTR0_LEVEL_DAMAGE_CALCULATION scale by the caster/spell level
	// ratio. C++ gates on !IsControlledByPlayer — pets and charmed
	// creatures skip it.
	if !creatureIsPlayerControlled(m) && spell.Attributes&spellAttr0LevelDamageCalculation != 0 && s != nil && s.Data != nil && m != nil {
		if spellScaler, ok, err := s.Data.GtNPCManaCostScaler(spell.SpellLevel); err == nil && ok && spellScaler != 0 {
			if casterScaler, ok, err := s.Data.GtNPCManaCostScaler(uint32(m.Level)); err == nil && ok {
				powerCost = int32(float32(powerCost) * (casterScaler / spellScaler))
			}
		}
	}
	if powerCost < 0 {
		powerCost = 0
	}
	return powerCost
}

// creatureIsPlayerControlled mirrors Unit::IsControlledByPlayer for
// the CalcPowerCost NPC gate: pets (OwnerGUID) and charmed creatures are
// player-controlled.
func creatureIsPlayerControlled(m *creatureMotion) bool {
	return m != nil && (m.OwnerGUID != 0 || m.Charmed)
}

// checkCreatureSpellPower mirrors Spell::CheckPower (Spell.cpp:6640-6674)
// for creature casters: POWER_HEALTH needs health strictly above the cost
// (SPELL_FAILED_CASTER_AURASTATE), unknown power types fail, POWER_RUNE is
// OK (CheckRuneCost returns SPELL_CAST_OK for non-players, Spell.cpp:4914),
// otherwise the power pool must cover the cost (SPELL_FAILED_NO_POWER).
// Creature casters never carry a cast item.
func checkCreatureSpellPower(spell wotlk.Spell, m *creatureMotion, cost int32) bool {
	if m == nil {
		return true
	}
	powerType := spell.PowerType
	if powerType == creaturePowerHealth {
		return int32(m.Health) > cost
	}
	if powerType >= creatureMaxPowers {
		return false
	}
	if powerType == creaturePowerRune {
		return true
	}
	return int32(creaturePower(m, powerType)) >= cost
}

// takeCreatureSpellPower mirrors Spell::TakePower (Spell.cpp:4779-4873) for
// creature casters: POWER_HEALTH drains health, other powers drain the pool
// (floored at 0 like Unit::ModifyPower), POWER_RUNE is a no-op (TakeRunePower
// is player-only). The five-second mana timer (Spell.cpp:4868-4870) is
// vacuous on this path — UNIT_FLAG2_REGENERATE_POWER is set only on pets
// (Pet.cpp:1868) and players (Player.cpp:544), never on world creatures, so
// Creature::Regenerate early-returns here; pets stamp the timer in the pet
// spell drain (pet_combat.go) and regen via the pet runtime
// (pet_runtime.go).
func takeCreatureSpellPower(spell wotlk.Spell, m *creatureMotion, cost int32) {
	if m == nil || cost <= 0 {
		return
	}
	powerType := spell.PowerType
	if powerType == creaturePowerRune {
		return
	}
	if powerType == creaturePowerHealth {
		if uint32(cost) >= m.Health {
			m.Health = 0
		} else {
			m.Health -= uint32(cost)
		}
		return
	}
	if powerType < uint32(len(m.Powers)) {
		if uint32(cost) >= m.Powers[powerType] {
			m.Powers[powerType] = 0
		} else {
			m.Powers[powerType] -= uint32(cost)
		}
	}
}

// checkCreatureSpellRange mirrors the CheckRange leg of Spell::CheckCast
// (Spell.cpp:5496) for creature casters. A next-melee-swing spell returns
// {0, 100} (Spell.cpp:6588-6589), disabling the model; a missing DBC
// range row stays permissive (unknown-data convention). The unit-target
// arm is skipped when the target is absent or the caster itself
// (Spell.cpp:6553). The DBC hostile/friendly pair keys on caster-vs-target
// hostility (WorldObject::GetSpellMinRangeForTarget/
// GetSpellMaxRangeForTarget, Object.cpp:2277-2303); the combat-reach arms
// mirror Spell::GetMinMaxRange (Spell.cpp:6594-6625) — melee-flagged
// entries collapse to the melee range (Unit::GetMeleeRange,
// Unit.cpp:614-618), SPELL_RANGE_RANGED entries add the melee range to the
// DBC min, and the caster+target reach rangeMod lands on the max (and on
// the min when it is positive and the entry is not ranged-flagged). The
// GO-target (Spell.cpp:6565-6569) and dest (6571-6577) arms are vacuous —
// creature casts carry no GO or dest targets in Go — and the
// FacingCasterFlags INFRONT arm (6563-6565) is player-only. Standing
// no-bridges: the both-moving +8/3 term (no movement state on this path),
// the REQ_AMMO ranged-weapon arm (player-only, Spell.cpp:6628),
// pet/charmed owner SPELLMOD_RANGE (Unit::GetSpellModOwner needs the
// owner's live session spellmods), and npcbot range mods.
func (s *Server) checkCreatureSpellRange(m *creatureMotion, spellInfo wotlk.Spell, targetGUID uint64, modOwner *session) bool {
	if s == nil || s.Data == nil || m == nil {
		return true
	}
	if isNextMeleeSwingSpell(spellInfo) {
		return true
	}
	rangeEntry, ok, err := s.Data.SpellRange(spellInfo.RangeIndex)
	if err != nil || !ok {
		return true
	}
	if targetGUID == 0 || targetGUID == m.GUID {
		return true
	}
	x, y, z, reach, _, found := s.aiThreatUnitPos(m, targetGUID, nil, true)
	if !found {
		return true
	}
	cReach := m.combatReachOrDefault()
	targetHostile := s.creatureRangeTargetHostile(m, targetGUID)
	var minRange, maxRange float64
	if rangeEntry.Flags&spellRangeFlagMelee != 0 {
		maxRange = calcMeleeRange(cReach, reach)
	} else {
		minRange = float64(rangeEntry.MinFriendly)
		maxRange = float64(rangeEntry.MaxFriendly)
		if targetHostile {
			minRange = float64(rangeEntry.MinHostile)
			maxRange = float64(rangeEntry.MaxHostile)
		}
		if rangeEntry.Flags&spellRangeFlagRanged != 0 {
			minRange += calcMeleeRange(cReach, reach)
		} else if minRange > 0 {
			minRange += float64(cReach) + float64(reach)
		}
		// Spell::GetMinMaxRange (Spell.cpp:6632-6635): SPELLMOD_RANGE
		// applies to the DBC max before the reach term, via
		// GetSpellModOwner (Object.cpp:2246-2268) — the pet/totem owner.
		if modOwner != nil {
			maxRange = modOwner.applySpellModFloat(spellInfo, spellModRange, maxRange)
		}
		maxRange += float64(cReach) + float64(reach)
	}
	if dist := distance3D(m.X, m.Y, m.Z, x, y, z); dist > maxRange {
		return false
	} else if minRange > 0 && dist < minRange {
		return false
	}
	return true
}

// creatureRangeTargetHostile mirrors the hostility key of
// WorldObject::GetSpellMinRangeForTarget/GetSpellMaxRangeForTarget
// (Object.cpp:2277-2303) for a creature caster: hostile only when the
// caster's faction template is hostile (and not friendly) to the target's,
// matching the player path's spellRangeTargetHostile convention
// (spells.go:3219). Unresolvable targets key the friendly pair, like C++'s
// null-target arm. Duel hostility is player-vs-player only and does not
// make a creature hostile to a duelist.
func (s *Server) creatureRangeTargetHostile(m *creatureMotion, targetGUID uint64) bool {
	if s == nil || m == nil {
		return false
	}
	hostileTo := func(tp playerPos) bool {
		return !s.isFriendlyFaction(m.Faction, tp) && s.isHostileFaction(m.Faction, tp)
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		tp := playerPos{
			FactionTemplate: ts.effectiveFactionTemplate(ts.player.Race),
			Race:            ts.player.Race,
			Class:           ts.player.Class,
			Sess:            ts,
			Reputations:     playerReputationMap(ts.player.Reputations),
		}
		return hostileTo(tp)
	}
	if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil {
		return hostileTo(playerPos{FactionTemplate: tm.Faction})
	}
	return false
}

// checkCreatureSpellBehindFacing mirrors the behind/facing arms of
// Spell::CheckCast (Spell.cpp:5379-5384) for creature casters (both the
// wild-creature AI path and the pet path). With
// SPELL_ATTR0_CU_REQ_CASTER_BEHIND_TARGET the caster must be outside the
// target's frontal M_PI arc (HasInArc(M_PI), Position.cpp:120), failing
// SPELL_FAILED_NOT_BEHIND; with SPELL_ATTR0_CU_REQ_TARGET_FACING_CASTER
// the caster must be inside it, failing SPELL_FAILED_NOT_INFRONT. Both
// arms run for triggered casts (no C++ triggered exemption) and are
// skipped when the target is absent or the caster itself (Spell.cpp:5377).
// An unresolvable target is fail-open — Go cannot prove the facing
// violation without the target's orientation.
func (s *Server) checkCreatureSpellBehindFacing(m *creatureMotion, spellInfo wotlk.Spell, targetGUID uint64) bool {
	if s == nil || m == nil {
		return true
	}
	customAttr := s.getSpellCustomAttr(spellInfo.ID)
	if customAttr&(SpellCustomAttrReqCasterBehindTarget|SpellCustomAttrReqTargetFacingCaster) == 0 {
		return true
	}
	if targetGUID == 0 || targetGUID == m.GUID {
		return true
	}
	var o, tx, ty float32
	found := false
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		o, tx, ty, found = ts.player.Orientation, ts.player.X, ts.player.Y, true
	} else if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil {
		o, tx, ty, found = tm.Orientation, tm.X, tm.Y, true
	}
	if !found {
		return true
	}
	casterInFrontArc := hasInArc(o, tx, ty, m.X, m.Y, math.Pi)
	if customAttr&SpellCustomAttrReqCasterBehindTarget != 0 && casterInFrontArc {
		return false
	}
	if customAttr&SpellCustomAttrReqTargetFacingCaster != 0 && !casterInFrontArc {
		return false
	}
	return true
}

// creatureFriendlyToTarget mirrors WorldObject::IsFriendlyTo
// (Object.cpp:2847-2850, GetReactionTo >= REP_FRIENDLY) for a creature
// caster against a unit target, reusing the faction-template model of
// creatureRangeTargetHostile: player targets resolve through their live
// session's faction state, creature targets through a synthetic playerPos
// carrying the target's faction template. Self is always friendly
// (GetReactionTo's always-friendly-to-self arm, Object.cpp:2711). Inherited
// degradations: no duel arm (player-vs-player only; C++ also needs a player
// caster), no charmer/owner term (Go has no creature charm model; pets and
// totems cast through session paths, never here), no forced-rank/FFA-PvP
// terms (GetAffectingPlayer is null for these motions, so C++ skips that
// block too — Object.cpp:2735-2737).
func (s *Server) creatureFriendlyToTarget(m *creatureMotion, targetGUID uint64) bool {
	if s == nil || m == nil {
		return false
	}
	if targetGUID == m.GUID {
		return true
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		return s.isFriendlyFaction(m.Faction, playerPos{
			FactionTemplate: ts.effectiveFactionTemplate(ts.player.Race),
			Race:            ts.player.Race,
			Class:           ts.player.Class,
			Sess:            ts,
			Reputations:     playerReputationMap(ts.player.Reputations),
		})
	}
	if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil {
		return s.isFriendlyFaction(m.Faction, playerPos{FactionTemplate: tm.Faction})
	}
	return false
}

// creatureCastTargetResolves mirrors the lost-explicit-target arm of
// Spell::_cast (Spell.cpp:3288-3293) for the creature path: after
// UpdatePointers, an explicit object target that no longer resolves
// cancels the cast. A player target resolves through its live session on
// the caster's map/instance (C++ only sees in-map objects); any other unit
// resolves through the creature motion map. Self casts and target-less
// casts never reach this helper (GetObjectTargetGUID()==0 skips the arm).
func (s *Server) creatureCastTargetResolves(m *creatureMotion, targetGUID uint64) bool {
	if s == nil || m == nil {
		return true
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		return ts.player.Map == m.Map && ts.player.InstanceID == m.InstanceID
	}
	return s.findCreatureMotion(m.Map, m.InstanceID, targetGUID) != nil
}

// creatureUnitTargetPos resolves a unit GUID to its X/Y on the motion's
// map/instance for the SetInFront leg: live player session first (same
// map/instance, mirroring the lost-target arm's resolution), then the
// creature motion map — the ObjectAccessor::GetUnit(*caster, guid) model.
func (s *Server) creatureUnitTargetPos(m *creatureMotion, targetGUID uint64) (float32, float32, bool) {
	if s == nil || m == nil || targetGUID == 0 {
		return 0, 0, false
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil && ts.player.Map == m.Map && ts.player.InstanceID == m.InstanceID {
		return ts.player.X, ts.player.Y, true
	}
	if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil {
		return tm.X, tm.Y, true
	}
	return 0, 0, false
}

// creatureSpellTargetInCombat is the Server-level twin of unitTargetInCombat
// for wild-creature casters (SpellInfo::CheckTarget, SpellInfo.cpp:1650):
// a target player's session combat state, or a target creature's
// motion.InCombat. Resolves nothing — returns false when the guid maps to
// no unit. The UNIT_FLAG_PET_IN_COMBAT half of the C++ OR has no Go model.
func (s *Server) creatureSpellTargetInCombat(m *creatureMotion, targetGUID uint64) bool {
	if s == nil {
		return false
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		return ts.attackTarget != 0 || ts.player.UnitFlags&unitFlagInCombat != 0
	}
	if m == nil {
		return false
	}
	s.motionMu.Lock()
	motion := s.findCreatureMotionLocked(m.Map, m.InstanceID, targetGUID)
	s.motionMu.Unlock()
	return motion != nil && motion.InCombat
}

// creatureSpellTargetDead reports whether the explicit unit target is dead
// (SpellInfo::CheckTarget, SpellInfo.cpp:1715): player health or
// creature-motion health. False when the guid resolves to nothing
// (fail-open, like the LOS arms).
func (s *Server) creatureSpellTargetDead(m *creatureMotion, targetGUID uint64) bool {
	if s == nil {
		return false
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		return ts.player.Health == 0
	}
	if m == nil {
		return false
	}
	s.motionMu.Lock()
	motion := s.findCreatureMotionLocked(m.Map, m.InstanceID, targetGUID)
	s.motionMu.Unlock()
	return motion != nil && motion.Health == 0
}

// creatureSpellTargetTypePasses is the Server-level twin of
// session.checkTargetCreatureType for wild-creature casters
// (SpellInfo::CheckTargetCreatureType, SpellInfo.cpp:1868-1886, as reached
// from SpellInfo::CheckTarget, SpellInfo.cpp:1728): the spell's
// TargetCreatureType mask (field 17) must intersect the target's
// GetCreatureTypeMask, or the cast fizzles (the C++
// SPELL_FAILED_TARGET_IS_PLAYER / SPELL_FAILED_BAD_TARGETS arms map to a
// silent fail on this path). The Curse-of-Doom arm (SPELLFAMILY_WARLOCK +
// category 1179) rejects player targets outright while creatures always
// pass. The magnet (Grounding Totem) redirect has no Go model, so the check
// always applies — documented gap, like the session twin.
func (s *Server) creatureSpellTargetTypePasses(ctx context.Context, m *creatureMotion, spell wotlk.Spell, targetGUID uint64) bool {
	if s == nil || m == nil || targetGUID == 0 {
		return true
	}
	mask, isPlayer := s.creatureTargetCreatureTypeMask(ctx, m, targetGUID)
	if spell.SpellFamilyName == spellFamilyWarlock && spell.Category == 1179 {
		return !isPlayer
	}
	return spell.TargetCreatureType == 0 || mask == 0 || mask&spell.TargetCreatureType != 0
}

// creatureTargetCreatureTypeMask is the Server-level twin of
// session.targetCreatureTypeMask (Unit::GetCreatureTypeMask, Unit.cpp:9153):
// mask = (type >= 1) ? 1 << (type-1) : 0. Player targets resolve through
// their live session; creature targets resolve their template entry from
// the motion map (falling back to the entry embedded in the runtime GUID,
// like session.targetCreatureEntry) and the creature_template type row. A
// zero mask means the type could not be resolved — like C++'s !creatureType
// short-circuit, an unknown type does not fail the check.
func (s *Server) creatureTargetCreatureTypeMask(ctx context.Context, m *creatureMotion, targetGUID uint64) (uint32, bool) {
	if s == nil || m == nil {
		return 0, false
	}
	if targetSess := s.findSessionByGUID(targetGUID); targetSess != nil && targetSess.player != nil {
		return s.creatureTypeMaskOfPlayer(targetSess.player), true
	}
	var entry uint32
	if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil {
		entry = tm.Entry
	} else if e := uint32(targetGUID>>24) & 0xFFFFFF; e != 0 {
		entry = e
	}
	if entry == 0 || s.WorldStore == nil || s.WorldStore.DB == nil {
		return 0, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var creatureType int64
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT type FROM creature_template WHERE entry = ?", entry).Scan(&creatureType); err != nil {
		return 0, false
	}
	if creatureType < 1 {
		return 0, false
	}
	return uint32(1) << (uint32(creatureType) - 1), false
}

// creatureTypeMaskOfPlayer is the Server-level twin of
// session.playerCreatureTypeMask (Unit::GetCreatureType, Unit.cpp:9124-9150):
// the shapeshift form's CreatureType when set (> 0), else the race's
// CreatureType (ChrRaces.dbc field 8).
func (s *Server) creatureTypeMaskOfPlayer(p *playerState) uint32 {
	if p == nil || s == nil || s.Data == nil {
		return 0
	}
	if form := p.ShapeshiftForm; form != 0 {
		if shape, found, err := s.Data.ShapeshiftForm(uint32(form)); err == nil && found && shape.CreatureType > 0 {
			return uint32(1) << (uint32(shape.CreatureType) - 1)
		}
	}
	if race, found, err := s.Data.Race(uint32(p.Race)); err == nil && found && race.CreatureType >= 1 {
		return uint32(1) << (race.CreatureType - 1)
	}
	return 0
}

// creatureSpellTargetHasAuraState is the Server-level twin of
// session.targetHasAuraState for wild-creature casters (Unit::HasAuraState
// as reached from SpellInfo::CheckTarget, SpellInfo.cpp:1757-1766). The
// SPELL_AURA_ABILITY_IGNORE_AURASTATE bypass reads the caster's own auras
// (Unit.cpp:5946-5954), so it runs first for every target kind, consulting
// the caster motion's attributed auras (the presence map carries no caster
// info — the session twin likewise reads active auras only). Player targets
// resolve through their live session; creature targets consult the server
// creature aura maps keyed off the caster's map/instance. Per-caster states
// (Conflagrate, Deadly Poison) need an aura applied by the casting motion
// itself (CasterGUID == m.GUID, Unit.cpp:5957-5965). An unresolvable target
// returns false, like the session twin's getCombatTarget miss.
func (s *Server) creatureSpellTargetHasAuraState(m *creatureMotion, spell wotlk.Spell, targetGUID uint64, state uint32) bool {
	if s == nil || m == nil || state == auraStateNone || targetGUID == 0 {
		return false
	}
	if s.creatureCasterIgnoresAuraState(m, spell) {
		return true
	}
	bit := uint32(1) << (state - 1)
	perCaster := bit&perCasterAuraStateMask != 0
	if targetSess := s.findSessionByGUID(targetGUID); targetSess != nil && targetSess.player != nil {
		if perCaster {
			for _, aura := range targetSess.loadedAuras() {
				if aura == nil || aura.CasterGUID != m.GUID {
					continue
				}
				if spellAuraStateOf(s, aura.SpellID) == state {
					return true
				}
			}
			return false
		}
		return targetSess.unitAuraStateMask()&bit != 0
	}
	target := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID)
	if target == nil {
		return false
	}
	key := creatureAuraKey{Map: m.Map, InstanceID: m.InstanceID, GUID: targetGUID}
	mask := auraStateHealthMask(target.Health, target.MaxHealth)
	s.auraMu.Lock()
	for spellID := range s.creatureAuras[key] {
		if spellAuraStateOf(s, spellID) == state && !perCaster {
			mask |= bit
		}
	}
	for spellID, aura := range s.activeCreatureAuras[key] {
		if spellAuraStateOf(s, spellID) != state {
			continue
		}
		if perCaster && (aura == nil || aura.CasterGUID != m.GUID) {
			continue
		}
		mask |= bit
	}
	s.auraMu.Unlock()
	return mask&bit != 0
}

// creatureSpellTargetHasAura is the Server-level twin of
// session.targetHasAura for wild-creature casters (SpellInfo::CheckTarget,
// SpellInfo.cpp:1769-1772): whether the target currently carries auraSpell.
// Player targets resolve to their session's aura set; creature targets
// consult the server creature aura maps. The C++ GetSpellIdForDifficulty
// remap is omitted — documented standing delta shared with the session
// twin.
func (s *Server) creatureSpellTargetHasAura(m *creatureMotion, targetGUID uint64, auraSpell uint32) bool {
	if s == nil || m == nil || auraSpell == 0 || targetGUID == 0 {
		return false
	}
	if targetSess := s.findSessionByGUID(targetGUID); targetSess != nil && targetSess.player != nil {
		return targetSess.hasAura(auraSpell)
	}
	key := creatureAuraKey{Map: m.Map, InstanceID: m.InstanceID, GUID: targetGUID}
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	if _, found := s.creatureAuras[key][auraSpell]; found {
		return true
	}
	_, found := s.activeCreatureAuras[key][auraSpell]
	return found
}

// checkCreatureSpellCheckTarget is the wild-creature-caster twin of the
// explicit-target SpellInfo::CheckTarget block in Spell::CheckCast
// (Spell.cpp:5372 — CheckTarget(m_caster, target, implicit=false) for a
// creature caster). C++ relative order: between the conditions block
// (5337-5348) and the behind/facing arms (5379-5384); runs for triggered
// casts too (no C++ triggered exemption). Returns false when the cast must
// fizzle. Bridged: self, in-combat, only-players, dead, creature-type,
// GM/invis, in-flight, aura-state, aura-spell; no-bridge documented inline
// (visibility, ONLY_TARGET_GHOSTS, tapped/pickpocket/disarm vacuous,
// corpses, CC implicit-only, PREVENT_RESURRECTION).
func (s *Server) checkCreatureSpellCheckTarget(ctx context.Context, m *creatureMotion, spell wotlk.Spell, targetGUID uint64) bool {
	if s == nil || m == nil {
		return true
	}
	// C++ only enters the block for m_targets.GetUnitTarget() != null.
	if targetGUID == 0 {
		return true
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1637-1638):
	// SPELL_ATTR1_CANT_TARGET_SELF with caster == target.
	if spell.AttributesEx&spellAttr1CantTargetSelf != 0 && targetGUID == m.GUID {
		return false
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1641-1642): the CanSeeOrDetect
	// visibility gate is unbridged — Go has no stealth/invisibility-level
	// model on either side (documented no-bridge, like the player path).
	// SpellInfo::CheckTarget (SpellInfo.cpp:1650-1651):
	// SPELL_ATTR1_CANT_TARGET_IN_COMBAT with an in-combat target.
	if spell.AttributesEx&spellAttr1CantTargetInCombat != 0 && s.creatureSpellTargetInCombat(m, targetGUID) {
		return false
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1654-1662):
	// SPELL_ATTR3_ONLY_TARGET_GHOSTS — no Go model (target-side ghost aura
	// state is unmodeled); documented no-bridge.
	// SpellInfo::CheckTarget (SpellInfo.cpp:1667-1692): the CANT_TARGET_TAPPED,
	// PICKPOCKET and DISARM arms sit inside C++'s
	// `caster->GetTypeId() == TYPEID_PLAYER` block — vacuous for a creature
	// caster, so they are correctly absent here.
	// SpellInfo::CheckTarget (SpellInfo.cpp:1697-1710): corpse targets —
	// no Go corpse-target model on the creature path; documented no-bridge.
	// SpellInfo::CheckTarget (SpellInfo.cpp:1712-1713):
	// SPELL_ATTR3_ONLY_TARGET_PLAYERS with a non-player target — a guid
	// with a live session is a player.
	if spell.AttributesEx3&spellAttr3OnlyTargetPlayers != 0 && s.findSessionByGUID(targetGUID) == nil {
		return false
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1715-1716): dead targets fail
	// unless the spell allows them (IsAllowingDeadTarget, 1177).
	if s.creatureSpellTargetDead(m, targetGUID) && !spellAllowsDeadTarget(spell) {
		return false
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1718-1720): the implicit-only
	// crowd-control gate is vacuous — C++ passes implicit=false for
	// creature casters. (SpellInfo.cpp:1723 UNIT_FLAG_NOT_SELECTABLE is
	// commented out in C++; vacant.)
	// SpellInfo::CheckTarget (SpellInfo.cpp:1728): target creature-type
	// gate, ahead of the aura-state/aura-spell gates in C++ order.
	if !s.creatureSpellTargetTypePasses(ctx, m, spell, targetGUID) {
		return false
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1736-1743): GM-mode /
	// GM-invisibility. A wild caster has no affecting player, so C++ fires
	// this only for non-positive spells (GetAffectingPlayer() ||
	// !IsPositive()); charmed/owned casters keep the owner leg via
	// m.OwnerGUID. Self-targets are exempt (unitTarget != caster).
	if targetGUID != m.GUID && (m.OwnerGUID != 0 || !spellIsPositive(spell)) && s.gmAttackTargetBlocked(targetGUID) {
		return false
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1745-1747): in-flight targets
	// (only players fly); the ALLOW_INFLIGHT_TARGET custom attribute
	// exempts, like the player path.
	if ts := s.findSessionByGUID(targetGUID); ts != nil {
		if s.getSpellCustomAttr(spell.ID)&SpellCustomAttrAllowInflightTarget == 0 && ts.inFlight {
			return false
		}
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1757-1766): target aura-state
	// requirements. C++ skips both arms for vehicle casters and when the
	// target is the caster's charmer/owner (creature casters are never
	// vehicles); the charmer/owner exemption mirrors the pet path.
	charmerExempt := targetGUID == m.OwnerGUID || (m.CharmOwnerGUID != 0 && targetGUID == m.CharmOwnerGUID)
	if !charmerExempt {
		if spell.TargetAuraState != 0 && !s.creatureSpellTargetHasAuraState(m, spell, targetGUID, spell.TargetAuraState) {
			return false
		}
		if spell.ExcludeTargetAuraState != 0 && s.creatureSpellTargetHasAuraState(m, spell, targetGUID, spell.ExcludeTargetAuraState) {
			return false
		}
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1769-1773): target aura-spell
	// requirements (no charmer exemption in C++).
	if spell.TargetAuraSpell != 0 && !s.creatureSpellTargetHasAura(m, targetGUID, spell.TargetAuraSpell) {
		return false
	}
	if spell.ExcludeTargetAuraSpell != 0 && s.creatureSpellTargetHasAura(m, targetGUID, spell.ExcludeTargetAuraSpell) {
		return false
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1776-1778):
	// SPELL_AURA_PREVENT_RESURRECTION — unbridged on the player path too;
	// documented no-bridge parity.
	return true
}

// spellFirstEffectTargetsUnitPet reports whether any effect carries
// TargetA == TARGET_UNIT_PET (5) — the Spell::CheckCast pet-presence arm
// (Spell.cpp:5410-5432) scans all effects for the first such effect; the
// scan order is irrelevant since any hit fizzles. Shared by the creature
// and pet cast paths.
func spellFirstEffectTargetsUnitPet(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.ImplicitTargetA == spellImplicitTargetUnitPet {
			return true
		}
	}
	return false
}

func (s *Server) castCreatureSpellInternal(ctx context.Context, m *creatureMotion, spellID uint32, targetGUID uint64, triggered bool) {
	if s == nil || m == nil || spellID == 0 {
		return
	}
	now := time.Now()
	castID := uint8(1)
	castFlags := uint32(spellCastFlagGo)
	castTimeStamp := uint32(now.UnixMilli())
	hitTargets := []uint64{targetGUID}
	spellTarget := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnitWireMask, UnitGUID: targetGUID}
	// The resolved dest-family destination: evaluated ahead of the LOS
	// gates below and reused for the GO packet write, so the dest-LOS arm
	// and the packet carry the same value. Go creature casts carry no
	// caller-supplied dest, so the resolution is the server-side
	// SelectSpellTargets twin. hasDest is false when the spell carries no
	// dest-family target.
	var destX, destY, destZ float32
	hasDest := false
	var spellInfo wotlk.Spell
	spellKnown := false
	if s != nil && s.Data != nil {
		if si, found, err := s.Data.Spell(spellID); err == nil && found {
			spellInfo, spellKnown = si, true
		}
	}
	sendGoPacket := true
	// Reflect verdicts collected by the AddUnitTarget arm below; read by
	// the GO send and the per-hit legs after the spellKnown block.
	var reflectedTargets map[uint64]uint8
	var evadedTargets map[uint64]bool
	if spellKnown {
		spellTarget = spellGoPacketTarget(spellInfo, spellTarget)
		// Spell::CheckCast unit-cooldown arm (Spell.cpp:5207-5211): a
		// creature whose spell history still holds the spell — or, for
		// non-triggered casts, its DBC category — fizzles. The arm runs
		// for triggered casts too (with the category ignored, via
		// IsIgnoringCooldowns = TRIGGERED_IGNORE_SPELL_AND_CATEGORY_CD,
		// Spell.cpp:7506-7509), so it sits ahead of the !triggered block,
		// matching C++ order (5207 precedes the shapeshift block at 5247).
		if !s.creatureSpellHistoryReady(m, spellInfo, triggered) {
			return
		}
		// Spell::CheckPower (Spell.cpp:5502) + Spell::TakePower
		// (Spell.cpp:3449, "Powers have to be taken before SendSpellGo"):
		// a creature that cannot afford the spell fizzles — no packet, no
		// effects — and a paying creature drains the pool. Triggered casts
		// (TRIGGERED_IGNORE_POWER_AND_REAGENT_COST) skip both. The
		// caster-state block runs first, matching C++ relative order
		// (Spell.cpp:5298-5314 precedes CheckRange at 5496 and CheckPower
		// at 5502).
		if !triggered {
			// Spell::CheckCast shapeshift leg (Spell.cpp:5247-5277) and
			// SPELL_AURA_BLOCK_SPELL_FAMILY leg (Spell.cpp:5277-5278)
			// precede the caster-state block, matching C++ order.
			if !s.checkCreatureSpellShapeshift(m, spellInfo) {
				return
			}
			if !s.checkCreatureSpellFamilyBlocked(m, spellInfo) {
				return
			}
			if !s.checkCreatureSpellCasterState(m, spellInfo) {
				return
			}
		}
		// Spell::CheckCast conditions block (Spell.cpp:5337-5348): the
		// creature-caster twin of the player-path arm — CONDITION_SOURCE_TYPE_SPELL
		// rows with the caster as target 0 and the explicit unit target as
		// target 1. C++ carries no triggered exemption on this block, so it
		// sits outside the !triggered gate, between the caster-state block
		// (5294-5314) and the range arm (5496). A failing set fizzles the
		// cast (the caster has no client to report a fail code to).
		if !s.checkCreatureSpellCastConditions(ctx, m, spellID, targetGUID) {
			return
		}
		// SpellInfo::CheckExplicitTarget null-target arm (SpellInfo.cpp:1784-1790):
		// a spell whose explicit target mask needs a target fizzles when
		// none was provided. C++ relative order: CheckExplicitTarget (5365)
		// precedes CheckTarget (5372).
		if targetGUID == 0 && spellNeedsExplicitUnitTarget(spellInfo) {
			return
		}
		// SpellInfo::CheckExplicitTarget faction arms (SpellInfo.cpp:1793-1811):
		// the enemy/ally/party/raid/minipet/passenger verdict on the explicit
		// unit target. C++ relative order: CheckExplicitTarget (5365) ahead
		// of CheckTarget (5372). The affecting player is the charm/owner
		// session when one resolves, else nil (wild caster).
		var explicitAttacker *session
		if os := s.findSessionByGUID(m.CharmOwnerGUID); m.CharmOwnerGUID != 0 && os != nil {
			explicitAttacker = os
		} else if os := s.findSessionByGUID(m.OwnerGUID); m.OwnerGUID != 0 && os != nil {
			explicitAttacker = os
		}
		if targetGUID != 0 && !s.checkCreatureExplicitTargetFaction(m, explicitAttacker, spellInfo, targetGUID) {
			return
		}
		// SpellInfo::CheckTarget on the explicit unit target (Spell.cpp:5372),
		// between the conditions block and the behind/facing arms in C++
		// relative order. Runs for triggered casts too (no C++ triggered
		// exemption).
		if !s.checkCreatureSpellCheckTarget(ctx, m, spellInfo, targetGUID) {
			return
		}
		// Spell::CheckCast behind/facing arms (Spell.cpp:5379-5384): after
		// CheckTarget (5372) and ahead of the unit-target LOS arm (5391) in
		// C++ relative order. Runs for triggered casts too (no C++
		// triggered exemption).
		if !s.checkCreatureSpellBehindFacing(m, spellInfo, targetGUID) {
			return
		}
		// Spell::CheckCast unit-target line-of-sight arm (Spell.cpp:5391-5401):
		// a creature caster whose spell has an explicit unit target fails
		// when the target is not within LOS, unless the spell carries
		// SPELL_ATTR2_CAN_TARGET_NOT_IN_LOS or
		// SPELL_ATTR5_SKIP_CHECKCAST_LOS_CHECK. C++ runs this for triggered
		// casts too (no triggered exemption on this arm); the dynObj
		// losTarget origin arm is near-vacuous (needs a triggered-by-aura
		// cast with a live dynobj of the aura spell) and the
		// GO-caster exemption is vacuous (caster is always a creature here).
		// DisableMgr::SPELL_DISABLE_LOS has no Go model (no DisableMgr).
		// C++ relative order: inside the unit-target block, after the
		// behind/facing arms, ahead of CheckRange (5496).
		destX, destY, destZ, hasDest = s.resolveCreatureSpellDestination(m, spellInfo, targetGUID)
		if targetGUID != 0 && targetGUID != m.GUID &&
			spellInfo.AttributesEx1&spellAttr2CanTargetNotInLOS == 0 &&
			spellInfo.AttributesEx5&spellAttr5SkipCheckcastLosCheck == 0 {
			if x, y, z, _, _, found := s.aiThreatUnitPos(m, targetGUID, nil, true); found {
				if !s.hasLineOfSightSpellCheck(m.Map, m.X, m.Y, m.Z, x, y, z) {
					return
				}
			}
		}
		// Spell::CheckCast dest line-of-sight arm (Spell.cpp:5400-5408): a
		// creature cast whose resolved destination is not within LOS from
		// the caster fails, unless the spell carries
		// SPELL_ATTR2_CAN_TARGET_NOT_IN_LOS or
		// SPELL_ATTR5_SKIP_CHECKCAST_LOS_CHECK. C++ reads m_targets at
		// CheckCast time (seeded by InitExplicitTargets before CheckCast);
		// Go creature casts carry no caller-supplied dest, so the
		// server-side resolved destination is evaluated at the C++
		// relative position — after the unit-target LOS arm (5391), ahead
		// of the pet-presence arm (5410). DisableMgr::SPELL_DISABLE_LOS
		// has no Go model (no DisableMgr). Runs for triggered casts too
		// (no C++ triggered exemption).
		if hasDest &&
			spellInfo.AttributesEx1&spellAttr2CanTargetNotInLOS == 0 &&
			spellInfo.AttributesEx5&spellAttr5SkipCheckcastLosCheck == 0 {
			if !s.hasLineOfSightSpellCheck(m.Map, m.X, m.Y, m.Z, destX, destY, destZ) {
				return
			}
		}
		// Spell::CheckCast pet-presence arm (Spell.cpp:5410-5432): any effect
		// with TargetA == TARGET_UNIT_PET (5) needs the caster's guardian pet
		// (Unit::GetGuardianPet). The npcbot arm is vacuous (no npcbot model
		// in Go) and creature motions carry no guardian pet, so the first
		// such effect fizzles the cast — SPELL_FAILED_NO_PET, or
		// SPELL_FAILED_DONT_REPORT when triggered by an aura; both fizzle
		// here since the creature path has no client to report to. C++
		// relative order: after the dest-LOS arm, ahead of the battleground
		// arm. Runs for triggered casts too (no C++ triggered exemption).
		if spellFirstEffectTargetsUnitPet(spellInfo) {
			return
		}
		// Spell::CheckCast arena arm (Spell.cpp:5439-5447):
		// ATTR4_NOT_USABLE_IN_ARENA, or a recovery time over 10 minutes
		// without ATTR4_USABLE_IN_ARENA, fails on battle-arena maps.
		// GetRecoveryTime = max(RecoveryTime, CategoryRecoveryTime)
		// (SpellInfo.cpp:3149-3152); the map-entry-must-exist shape mirrors
		// the session twin and the C++ sMapStore lookup. Runs for triggered
		// casts too (no C++ triggered exemption).
		recoveryTime := spellInfo.RecoveryTime
		if spellInfo.CategoryRecoveryTime > recoveryTime {
			recoveryTime = spellInfo.CategoryRecoveryTime
		}
		if spellInfo.AttributesEx4&spellAttr4NotUsableInArena != 0 ||
			(recoveryTime > 10*60*1000 && spellInfo.AttributesEx4&spellAttr4UsableInArena == 0) {
			if entry, found, err := s.Data.Map(m.Map); err == nil && found && entry.IsBattleArena() {
				return
			}
		}
		// Spell::CheckCast zone arm (Spell.cpp:5448-5462,
		// SpellInfo::CheckLocation, SpellInfo.cpp:1505-1567): the shared
		// motion-caster twin (checkMotionSpellCheckLocation,
		// creature_spell_area.go) — AreaGroupId + NOT_IN_RAID_INSTANCE +
		// spell_area DB leg with null-player IsFitToRequirements. Runs for
		// triggered casts too (no C++ triggered exemption).
		if !s.checkMotionSpellCheckLocation(ctx, m, spellInfo) {
			return
		}
		// Spell::CheckCast spell-focus arm (Spell.cpp:5476-5482): a spell
		// with RequiresSpellFocus fizzles without a matching focus object
		// in range (motionSpellFocusFound, the SearchSpellFocus twin
		// anchored at the creature motion). Runs for triggered casts too
		// (no C++ triggered exemption).
		if !s.motionSpellFocusFound(ctx, m, spellInfo) {
			return
		}
		// Spell::CheckRange (Spell.cpp:5496): C++ runs the range arm for
		// triggered casts too (5494-5496, "Triggered spells also have
		// range check"), after the unit-target LOS arm and before
		// CheckPower (5502)/CheckCasterAuras (5509).
		if !s.checkCreatureSpellRange(m, spellInfo, targetGUID, nil) {
			return
		}
		// Spell::CheckPower (Spell.cpp:5502): the affordability check runs in
		// the prepare-phase position, but the drain itself lands in _cast
		// after SelectSpellTargets (Spell.cpp:3449), so the cost is stashed
		// for the TakePower arm below — a lost explicit target (3288-3293)
		// or failed CheckCast never drains in C++.
		var powerCost int32
		if !triggered {
			powerCost = s.creatureSpellPowerCost(spellInfo, m)
			if !checkCreatureSpellPower(spellInfo, m, powerCost) {
				return
			}
			// Spell::CheckCasterAuras CC-state gate (Spell.cpp:5509,
			// gated by TRIGGERED_IGNORE_CASTER_AURAS): after CheckPower,
			// matching the C++ CheckCast gate order; Go's triggered
			// models the full mask, so triggered casts skip all three.
			if !s.checkCreatureCasterAuras(m, spellInfo) {
				return
			}
		}
		// SPELL_EFFECT_DISPEL arm (Spell.cpp:5517-5562): a pure-dispel spell
		// with a non-empty mask fizzles on a target with nothing dispellable
		// (SPELL_FAILED_NOTHING_TO_DISPEL). Placed before the steal arm,
		// matching C++ CheckCast order (the dispel scan precedes the
		// per-effect switch holding STEAL_BENEFICIAL_BUFF at 5957). Runs for
		// non-triggered casts only — the !IsTriggered() gate at 5540.
		if !s.creatureDispelPreCast(m, spellInfo, targetGUID, triggered) {
			return
		}
		// Summon-family arms (Spell.cpp:5805-5946): SUMMON_PLAYER /
		// SUMMON_RAF_FRIEND fail for non-player casters; CREATE_TAMED_PET
		// needs a player target without an existing pet. The SUMMON /
		// SUMMON_PET pet+charm gates are vacuous for creature casters (no
		// pet-ownership or charm model). Placed after the dispel arm and
		// before the steal arm, matching C++ CheckCast case order.
		if !s.creatureSummonPreCast(m, spellInfo, targetGUID) {
			return
		}
		// SPELL_EFFECT_STEAL_BENEFICIAL_BUFF arm (Spell.cpp:5957-5984):
		// no !IsTriggered() gate, so it runs for triggered casts too.
		// Placed after the caster-aura gate and before the cooldown
		// record, matching C++ CheckCast order (5509 -> per-effect
		// switch 5617+ -> SendSpellCooldown at 3463): a failed CheckCast
		// never starts a cooldown and never arms cast state.
		if !s.creatureStealPreCast(m, spellInfo, targetGUID) {
			return
		}
		// Spell::prepare stealth-break arm (Spell.cpp:3180-3191, gated by
		// TRIGGERED_IGNORE_AURA_INTERRUPT_FLAGS 0x100): a non-triggered
		// creature cast of a stealth-breaking spell (SpellInfo::IsBreakingStealth,
		// SpellInfo.cpp:1244-1247 == !SPELL_ATTR1_NOT_BREAK_STEALTH) drops
		// the caster's AURA_INTERRUPT_FLAG_CAST auras at cast start, ahead
		// of SendSpellStart/TriggerGlobalCooldown in C++; any unit-target
		// effect additionally drops AURA_INTERRUPT_FLAG_SPELL_ATTACK auras
		// (GetUsedTargetObjectType() == TARGET_OBJECT_TYPE_UNIT, mirrored
		// here by the player path's spellEffectTargetsUnit approximation,
		// spells.go:3076). Go's triggered models TRIGGERED_FULL_MASK
		// (0x0007FFFF), which carries both TRIGGERED_CAST_DIRECTLY (0x80,
		// skipping the arm's else-branch) and
		// TRIGGERED_IGNORE_AURA_INTERRUPT_FLAGS, so triggered casts skip
		// the arm. C++ relative order: after CheckCast (the steal arm
		// above), ahead of the _cast arms (the lost-explicit-target arm
		// below).
		if !triggered && spellInfo.AttributesEx&spellAttr1NotBreakStealth == 0 {
			s.removeCreatureAurasWithInterruptFlags(m, auraInterruptFlagCast)
			for _, eff := range spellInfo.Effects {
				if spellEffectTargetsUnit(eff.Effect) {
					s.removeCreatureAurasWithInterruptFlags(m, auraInterruptFlagSpellAttack)
					break
				}
			}
		}
		// Spell::_cast lost-explicit-target arm (Spell.cpp:3288-3293): an
		// explicit unit target whose object no longer resolves cancels the
		// cast — no GO packet, no cooldown (SelectSpellTargets at 3410
		// precedes SendSpellCooldown at 3463). Checked before the redirect:
		// C++ tests the loss before SelectSpellTargets. Self casts and
		// target-less casts are exempt (GetObjectTargetGUID()==0 skips the
		// arm). Runs for triggered casts too — the arm has no triggered
		// gate.
		if targetGUID != 0 && targetGUID != m.GUID && !s.creatureCastTargetResolves(m, targetGUID) {
			return
		}
		// Spell::_cast creature SetInFront leg (Spell.cpp:3404-3409,
		// Unit::SetInFront, Unit.cpp:13663-13667): a creature caster with a
		// resolving non-empty unit target that isn't possessed faces it
		// before SelectSpellTargets. Keys on the creature's combat target
		// (GetTarget), not the spell target. The UNIT_STATE_CANNOT_TURN
		// gate has no Go model (standing no-bridge, same as the other
		// unit-state arms); the angle mirrors Unit::GetAbsoluteAngle
		// (atan2 normalized to [0, 2pi), same as the distract bridge).
		if m.TargetGUID != 0 && m.UnitFlags&unitFlagPossessed == 0 {
			if tx, ty, ok := s.creatureUnitTargetPos(m, m.TargetGUID); ok {
				angle := float32(math.Atan2(float64(ty-m.Y), float64(tx-m.X)))
				if angle < 0 {
					angle += 2 * math.Pi
				}
				m.Orientation = angle
			}
		}
		// Spell::SelectExplicitTargets magic-redirect arm (Spell.cpp:724-757):
		// no caster-type gate — SelectSpellTargets (761) runs it for
		// creature casts in _cast (3410), after CheckCast (the steal arm
		// above) and before the cooldown record. A live Grounding Totem
		// magnet on the victim pulls the cast onto the totem; the GO
		// packet below carries the redirected target.
		if redirectGUID, ok := s.creatureGroundingTotemRedirect(ctx, m, spellInfo, targetGUID); ok {
			targetGUID = redirectGUID
			spellTarget.UnitGUID = redirectGUID
			hitTargets = []uint64{redirectGUID}
			// The redirect changes the anchor for the target-dest arms
			// (SelectImplicitTargetDestTargets, Spell.cpp:1433-1462): the
			// packet destination re-resolves on the redirected target,
			// matching C++ arm order inside SelectSpellTargets (the
			// 724-757 redirect precedes the 900-975 dest arms). The
			// dest-LOS gate above keeps the pre-redirect resolution — C++
			// evaluates that arm at CheckCast time, before the redirect.
			destX, destY, destZ, hasDest = s.resolveCreatureSpellDestination(m, spellInfo, targetGUID)
		}
		// Spell::prepare aura-scaling arm (Spell.cpp:3040-3056) + the
		// AddUnitTarget min-level arm (Spell.cpp:2115-2150) + the
		// SelectTargets removal leg (Spell.cpp:807-830) for a creature
		// caster. The fill arm gates on IsControlledByPlayer(), which holds
		// exactly for charmed motions — charmCreature's only call site
		// passes a player GUID (spells.go:20929), mirroring the charm legs
		// (Unit.cpp:6219/6259/6265); wild motions keep m_auraScaleMask at 0
		// and skip the arm entirely. The cast row is the pristine DBC row,
		// so the basepoint-equality gate is exact via
		// spellAuraScaleMask(spellInfo, spellInfo, true). Triggered creature
		// casts model TRIGGERED_FULL_MASK, which carries
		// TRIGGERED_IGNORE_AURA_SCALING (0x10), so the arm is skipped for
		// triggered casts. Failing targets drop out of the hit list ahead
		// of the GO packet (SelectSpellTargets at Spell.cpp:3410 precedes
		// SendSpellGo at 4338); with none left the cast fails with
		// SPELL_FAILED_LOWLEVEL (807-830) — silent here, the creature path
		// has no client to report to. No downrank application: the creature
		// path has no aura-application site at cast completion (damage lands
		// in the combat tick), so the PreprocessSpellHit scaleAura leg
		// (Spell.cpp:2780-2795) has nothing to hook into — documented
		// no-bridge.
		if !triggered && m.Charmed {
			if mask := spellAuraScaleMask(spellInfo, spellInfo, true); mask != 0 {
				kept, failed := s.applyCreatureAuraScaling(m, spellInfo, mask, hitTargets)
				hitTargets = kept
				if failed {
					return
				}
			}
		}
		// Spell::SelectImplicitChainTargets (Spell.cpp:1582-1624) for a
		// creature caster: chain selection runs inside SelectSpellTargets
		// (Spell.cpp:3410) with no caster-type gate, so creature-cast chain
		// spells (Black Stalker 31717 via creature:CastSpell) jump. The
		// selection is inert for area/cone/caster-object effects
		// (chainSelectionEligibleTarget, Spell.cpp:1173/1575), and the
		// SPELLMOD_JUMP_TARGETS fold is a pass-through with no session.
		// Jumps join hitTargets ahead of the reflect arm below so each
		// jump rolls its own reflect/evade verdict like the primary
		// target (Spell::AddUnitTarget, Spell.cpp:2152), and ride the GO
		// packet plus the Eluna/trigger legs with it. Triggered casts run
		// the arm too — SelectSpellTargets carries no IsTriggered gate.
		if spellKnown && targetGUID != 0 {
			if jumps, isChainHeal, checkType, effectMask := chainSpellJumps(nil, spellInfo); jumps > 0 {
				hitTargets = append(hitTargets, s.spellSearchChainTargetsForMotion(ctx, m, spellInfo, spellID, targetGUID, jumps, isChainHeal, checkType, effectMask)...)
			}
		}
		// Spell::AddUnitTarget reflect arm (Spell.cpp:2152, 2173-2188) on
		// this path: a hit target whose reflect roll succeeds leaves the
		// hit list and becomes a REFLECT miss in the GO packet below
		// (SelectSpellTargets at 3410 precedes SendSpellGo at 4338), with
		// ProcReflectDelayed armed for player reflectors (2181). Victims
		// here can be players or creatures. Unlike the inline AI victim
		// path (creaturemotion.go), this path applies no damage legs at
		// cast completion, so there is no return-trip damage funnel — C++
		// would deal the spell back to the caster via
		// DoEffectOnLaunchTarget (7740-7743), a documented delta shared
		// with this path's no-damage model. The per-hit legs below
		// (SpellHitTarget hook, trigger consumer) skip reflected targets
		// the same way the inline path skips their per-hit legs.
		if len(hitTargets) > 0 {
			for _, guid := range hitTargets {
				reflected, evaded, status := s.serverCastCreatureReflectVerdict(ctx, m, spellInfo, guid)
				if evaded {
					if evadedTargets == nil {
						evadedTargets = map[uint64]bool{}
					}
					evadedTargets[guid] = true
					continue
				}
				if reflected {
					if reflectedTargets == nil {
						reflectedTargets = map[uint64]uint8{}
					}
					reflectedTargets[guid] = status
					s.armCastCreatureReflectProcDelayed(ctx, m, guid, spellInfo)
				}
			}
		}
		// Spell::SelectSpellTargets dest-family arms (Spell.cpp:3410, ahead
		// of TakePower at 3449): a creature casting a spell with a
		// dest-family implicit target resolves the destination server-side
		// and carries it in the GO packet — C++ seeds m_targets at the
		// caster (CheckDst, Spell.cpp:6523) and SendSpellGo writes it into
		// the packet (Spell.cpp:4338). The destination was resolved ahead of
		// the LOS gates above (destX/destY/destZ); the GO packet carries the
		// same value. Placed after the redirect so the target-dest arms
		// anchor on the redirected target, matching the C++ arm order
		// inside SelectSpellTargets.
		if spellKnown && hasDest {
			spellTarget.Flags |= protocol.SpellTargetFlagDestLocation
			spellTarget.Destination = protocol.SpellTargetLocation{X: destX, Y: destY, Z: destZ}
		}
		if !triggered {
			// Spell::TakePower (Spell.cpp:3449-3456, "Powers have to be
			// taken before SendSpellGo"): drained after the fizzle arms,
			// matching C++ order (SelectSpellTargets at 3410 precedes the
			// drain at 3449). TakeReagents (3455) is player-only
			// (Spell.cpp:5048), vacuous for creatures.
			takeCreatureSpellPower(spellInfo, m, powerCost)
			// Spell::SendSpellCooldown (Spell.cpp:3463, 3780-3791):
			// non-triggered casts start the spell-history cooldown
			// (HandleCooldowns returns early for IsIgnoringCooldowns,
			// SpellHistory.cpp:165-166). Recorded after the fizzle gates:
			// a failed CheckCast never starts a cooldown in C++.
			s.recordCreatureSpellCooldown(m, spellInfo)
		}
		// Unit::CastSpell arms UNIT_STATE_CASTING for a non-triggered
		// cast's cast time; the combat tick's early-out
		// (CombatAI.cpp:97, 159) reads it via motion.CastingUntil.
		// Stamped after the fizzle gates: a failed CheckCast never arms
		// cast state in C++. Every castCreatureSpell caller models a
		// non-triggered cast (the kill.go AICOND_DIE death cast is
		// C++-triggered, but its motion is dead and never ticks again),
		// so the triggered stamp below is inert.
		if castMs := s.creatureAISpellCastTimeMs(creatureAuraKeyForMotion(m), spellInfo); castMs > 0 {
			m.CastingUntil = now.Add(time.Duration(castMs) * time.Millisecond)
		}
		// Spell::handle_immediate channel arms (Spell.cpp:3569-3596):
		// a channeled creature spell holds UNIT_STATE_CASTING for the
		// channel duration — Unit::CastSpell arms it at cast start for
		// CURRENT_CHANNELED_SPELL (Unit.cpp:3107, no triggered gate), and
		// CombatAI::UpdateAI's early-out (CombatAI.cpp:97) skips the
		// spell event and the melee swing for the whole channel. Go's
		// cast-time-only stamp left the channel unmodeled: the creature
		// could fire the next spell event or melee on the very next
		// tick. The channel extends the stamp by the DBC duration on top
		// of the cast time (C++ runs cast time first, then the channel).
		// Deltas: the SPELLMOD_DURATION / ModSpellDurationTime
		// (Object.cpp:2475) fold is unmodeled — GetSpellModOwner is null
		// for creature motions; the cast-speed factor of the channel's cast
		// time now comes from creatureCastingSpeedCastFactor
		// (creatureAISpellCastTimeMs), but ModSpellDurationTime's haste
		// fold on the duration leg itself stays unmodeled;
		// a -1 (infinite) duration stamps nothing, since C++ holds the
		// state until the channel is interrupted and Go has no
		// creature-channel-interrupt model — an infinite stamp would
		// soft-lock the motion. SendChannelStart (Spell.cpp:4669) stays
		// packet-only downstream: no SMSG_SPELL_START_CHANNEL is sent on
		// the creature path.
		if castMs := s.creatureAISpellCastTimeMs(creatureAuraKeyForMotion(m), spellInfo); isChanneledSpell(spellInfo) && s != nil && s.Data != nil {
			if dur, ok, derr := s.Data.SpellDuration(spellInfo.DurationIndex, 1); derr == nil && ok && dur > 0 {
				if channelUntil := now.Add(time.Duration(castMs+uint32(dur)) * time.Millisecond); channelUntil.After(m.CastingUntil) {
					m.CastingUntil = channelUntil
				}
			}
		}
		// Spell::SendSpellGo cast flags (Spell.cpp:4283-4330) for creature
		// casters: base UNKNOWN_9 (spellCastFlagGo); PENDING for triggered
		// non-auto-repeat casts (4292, same predicate as the player
		// triggered path); NO_GCD when the spell has no start recovery
		// time (4325, no caster gate). POWER_LEFT_SELF stays off — the
		// 4297 arm needs a player or pet caster. AMMO (4294) needs the
		// UNIT_VIRTUAL_ITEM_SLOT_ID model — no bridge. RUNE_LIST via
		// ACTIVATE_RUNE fills player-side only (C++ @todo notes a
		// creature crash there) — no bridge. ADJUST_MISSILE needs the
		// target elevation/delay-moment model — no bridge.
		if triggered && spellInfo.AttributesEx1&spellAttr2AutorepeatFlag == 0 {
			castFlags |= spellCastFlagPending
		}
		if spellInfo.StartRecoveryTime == 0 {
			castFlags |= protocol.SpellCastFlagNoGCD
		}
		// Spell::IsNeedSendToClient (Spell.cpp:7543-7547): a triggered
		// cast with no visual, not channeled, and no speed sends nothing.
		// Non-triggered casts always send (the final || term). Runs after
		// the drain/cooldown/stamp: C++ runs those before SendSpellGo's
		// early return. Only the packet is skipped — the rest of _cast
		// (including the spell_linked tail below) still runs, so this is a
		// flag, not an early return.
		sendGoPacket = true
		if triggered && spellInfo.SpellVisual[0] == 0 && spellInfo.SpellVisual[1] == 0 && !isChanneledSpell(spellInfo) && spellInfo.Speed <= 0 {
			sendGoPacket = false
		}
	}
	if sendGoPacket {
		packetHits := hitTargets
		var missStatus []protocol.SpellMissStatus
		if len(reflectedTargets) > 0 || len(evadedTargets) > 0 {
			packetHits = make([]uint64, 0, len(hitTargets))
			for _, guid := range hitTargets {
				if _, ok := reflectedTargets[guid]; ok {
					continue
				}
				if evadedTargets[guid] {
					continue
				}
				packetHits = append(packetHits, guid)
			}
			for guid, status := range reflectedTargets {
				missStatus = append(missStatus, protocol.SpellMissStatus{TargetGUID: guid, Reason: protocol.SpellMissReflect, ReflectStatus: status})
			}
			for guid := range evadedTargets {
				missStatus = append(missStatus, protocol.SpellMissStatus{TargetGUID: guid, Reason: protocol.SpellMissEvade})
			}
		}
		goPkt := protocol.BuildSpellGo(m.GUID, m.GUID, castID, spellID, castFlags, castTimeStamp, packetHits, missStatus, spellTarget)

		if targetSess := s.findSessionByGUID(targetGUID); targetSess != nil {
			_ = targetSess.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, true)
			s.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, targetSess)
		} else {
			s.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, nil)
		}
	}
	// Spell::EffectTransmitted (SpellEffects.cpp:4927-5045) for a creature
	// caster: the generic arm runs once per cast at SPELL_EFFECT_HANDLE_HIT
	// (not per hit target), spawning the effect's MiscValue gameobject
	// entry. Placed ahead of the spell_linked tail, matching C++ order
	// (effect handlers run inside the hit phase; the linked tail fires at
	// _cast completion). Gated on spellKnown like the tail: C++ never
	// reaches _cast without a SpellInfo.
	if spellKnown {
		for i := range spellInfo.Effects {
			if eff := spellInfo.Effects[i]; eff.Effect == spellEffectTransmitted {
				s.creatureEffectTransmitted(ctx, m, spellTarget, spellInfo, eff)
			}
		}
	}
	// Spell::EffectInstaKill (SpellEffects.cpp:274-296) for a creature
	// caster: the per-effect HIT_TARGET arm — the only spell-effect arm
	// bridged on this path, which otherwise runs the no-damage model (all
	// other effects deal nothing here). Each effect-1 effect kills its hit
	// targets; reflected/evaded targets are skipped like every other
	// per-hit leg above. The m_caster == unitTarget finish() leg is vacuous
	// (no client-side interrupt message on this path). Gated on spellKnown
	// like the Transmitted arm.
	if spellKnown {
		for i := range spellInfo.Effects {
			if spellInfo.Effects[i].Effect == spellEffectInstaKill {
				s.creatureEffectInstaKill(ctx, m, spellID, hitTargets, reflectedTargets, evadedTargets)
			}
		}
	}
	// Spell::EffectSummonType (SpellEffects.cpp:2194-2440) for a creature
	// caster: the HIT-mode arm spawning SPELL_EFFECT_SUMMON creature
	// entries. Placed after the InstaKill leg, matching the other
	// effect-grouped arms above (C++ runs effects in index order;
	// documented approximation). Gated on spellKnown like the tail.
	if spellKnown {
		s.creatureEffectSummonType(ctx, m, spellInfo, hasDest, destX, destY, destZ)
	}
	// Spell::_cast spell_linked tail (Spell.cpp:3503-3512): type-0 rows
	// fire at cast completion for creature casters too — after SendSpellGo
	// (3467), and even when IsNeedSendToClient skipped the packet. Gated
	// on spellKnown: C++ never reaches _cast without a SpellInfo.
	if spellKnown {
		s.fireCreatureSpellLinkedTriggers(ctx, m, spellID, targetGUID)
	}
	// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2609-2617): a
	// non-friendly non-positive (or dispel-effect) spell hit stands the
	// target up. Placed ahead of the ev14/ev15 legs, matching C++ order
	// (2609 precedes 2629). The EVADE-miss arm is vacuous (evaded targets
	// are skipped below; PreprocessTarget, Spell.cpp:2347, nulls
	// _spellHitTarget on any miss, and this path models no plain-miss
	// verdict, so every remaining hit target is live). A bounced reflect
	// return trip (IMMUNE) nulls _spellHitTarget (Spell.cpp:2346-2350) and
	// stands nothing; a LANDED reflect keeps the ORIGINAL target for this
	// arm — the stand-up leg fires on TargetInfo::unit (the reflector),
	// not on _spellHitTarget (the caster). The AtTargetAttacked half is
	// covered by the combat tick's aggro when this path's no-damage-model
	// damage lands.
	if spellKnown && (!spellIsPositive(spellInfo) || spellHasEffect(spellInfo, spellEffectDispel)) {
		for _, guid := range hitTargets {
			if status, reflected := reflectedTargets[guid]; reflected && status != protocol.SpellMissNone {
				continue
			}
			if evadedTargets[guid] {
				continue
			}
			if guid == 0 || s.creatureFriendlyToTarget(m, guid) {
				continue
			}
			if ts := s.findSessionByGUID(guid); ts != nil && ts.player != nil {
				if ts.player.StandState != 0 {
					ts.player.StandState = 0
					ts.sendPlayerUpdate()
				}
				continue
			}
			if tm := s.findCreatureMotion(m.Map, m.InstanceID, guid); tm != nil {
				s.standCreatureMotionUp(tm)
			}
		}
	}
	// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2629-2650): the
	// target-side SpellHit Eluna hook — CREATURE_EVENT_ON_HIT_BY_SPELL
	// (event 14), args (event, creature, caster, spellid) — fires once
	// per hit target, immediately followed by the caster-side
	// SpellHitTarget leg (event 15, 2639-2650, args (caster, target,
	// spellid); ElunaCreatureAI.h:231-238 — the return only vetoes the
	// empty ScriptedAI arm, so it is discarded; the gameobject-caster
	// arm has no Go model). DoProcessTargetContainer (Spell.cpp:3555-3566)
	// runs the per-effect DoTargetSpellHit loop (Spell.cpp:2384) for
	// damage first and DoDamageAndTriggers once per target after — every
	// TargetInfo (primary, chain jumps, AoE fan-out) takes both legs in
	// container order; the per-effect loop carries no AI hook (the earlier
	// per-effect firing misattributed DoTargetSpellHit). The boolean veto
	// gates only the empty ScriptedAI::SpellHit base (CreatureAI.h:143),
	// so it is discarded. A miss nulls _spellHitTarget in PreprocessTarget
	// (Spell.cpp:2347), so the hook never fires there: SPELL_MISS_EVADE
	// verdicts are skipped below like the reflect verdicts (the trigger
	// consumer skips both too). The Lua signature exposes no effect index,
	// so firing once per target is exact, not an approximation — same as
	// the player path (spells.go).
	if spellKnown {
		for _, guid := range hitTargets {
			if status, reflected := reflectedTargets[guid]; reflected {
				// Reflect return trip (Spell.cpp:2350): a landed reflect
				// (ReflectResult == SPELL_MISS_NONE) retargets
				// _spellHitTarget to the caster itself, so the caster's
				// own AI takes the target-side SpellHit leg right here
				// (ev14, creature=caster, caster=caster) plus the
				// caster-side SpellHitTarget leg (ev15, target=caster)
				// — once per reflected target, in target-container
				// order. The reflector itself never fires
				// (_spellHitTarget points away from it). A bounced
				// return trip (IMMUNE) nulls _spellHitTarget — no fire
				// at all. The return-trip damage stays out (no-damage
				// model delta); only the AI notification legs are
				// modeled.
				if status == protocol.SpellMissNone {
					s.fireCreatureLuaEvent(ctx, m, scripting.CreatureEventOnHitBySpell, s.luaMotionCreature(m), spellID)
					s.fireCreatureSpellHitTarget(ctx, m, m.GUID, spellID)
				}
				continue
			}
			if evadedTargets[guid] {
				continue
			}
			if tm := s.findCreatureMotion(m.Map, m.InstanceID, guid); tm != nil {
				s.fireCreatureLuaEvent(ctx, tm, scripting.CreatureEventOnHitBySpell, s.luaMotionCreature(m), spellID)
			}
			// The ev15 leg rides the same target iteration, matching C++
			// order (2639-2650 follow 2629-2638 per TargetInfo) — chain
			// jumps take it too, not just the primary target. The guid != 0
			// gate keeps the old targetGUID != 0 gate (C++ never has a null
			// _spellHitTarget here).
			if guid != 0 {
				s.fireCreatureSpellHitTarget(ctx, m, guid, spellID)
			}
		}
	}
	// Spell::DoTriggersOnSpellHit (Spell.cpp:2913-2957): the per-hit trigger
	// consumer — ADD_TARGET_TRIGGER snapshot plus the spell_linked
	// SPELL_LINK_HIT (type 1) rows — fires per hit target. Go's creature
	// path applies spell damage in the combat tick, so this runs at cast
	// completion next to the type-0 tail above, the same placement that
	// tail uses. Gated on spellKnown like the tail: C++ never reaches
	// _cast without a SpellInfo.
	if spellKnown {
		unreflectedHits := hitTargets
		if len(reflectedTargets) > 0 || len(evadedTargets) > 0 {
			unreflectedHits = make([]uint64, 0, len(hitTargets))
			for _, guid := range hitTargets {
				if _, ok := reflectedTargets[guid]; ok {
					continue
				}
				if evadedTargets[guid] {
					continue
				}
				unreflectedHits = append(unreflectedHits, guid)
			}
		}
		s.fireCreatureHitTriggers(ctx, m, spellInfo, spellID, unreflectedHits)
	}
	// Spell::_cast "Handle procs on cast" tail (Spell.cpp:3525-3545): the
	// PHASE_CAST proc leg fires after the spell_linked tail for creature
	// casters too. The modOwner block (3514-3522) is vacuous on this path —
	// GetSpellModOwner returns null for wild and charmed creatures (only
	// Pet objects carry UNIT_MASK_PET, Pet.cpp:49), so neither the
	// spell-mod-taking reset nor the cheat-cooldown reset applies.
	if spellKnown {
		s.fireCreatureCastPhaseAuraProcs(ctx, m, spellID, triggered)
	}
}

// serverAuraScaleTargetLevel resolves a hit target's level for the
// aura-scaling min-level check (Spell::AddUnitTarget, Spell.cpp:2123/2143)
// on the creature path, where no player session exists: online players
// resolve via their session, tracked creatures via the motion store.
func (s *Server) serverAuraScaleTargetLevel(m *creatureMotion, targetGUID uint64) (uint8, bool) {
	if s == nil || m == nil {
		return 0, false
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil && ts.player.Level > 0 {
		return ts.player.Level, true
	}
	if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil && tm.Level > 0 {
		return uint8(tm.Level), true
	}
	return 0, false
}

// applyCreatureAuraScaling mirrors the AddUnitTarget ScaleAura arm
// (Spell.cpp:2115-2150) and the SelectTargets removal leg (Spell.cpp:807-830)
// for a charmed creature caster. For every hit target other than the caster
// whose effect mask is exactly the aura-scale mask, the target passes the
// min-level check when targetLevel+10 reaches the first rank's SpellLevel;
// failing targets drop from the hit list, and when every target is dropped
// the cast fails with SPELL_FAILED_LOWLEVEL (Spell.cpp:827-830), reported
// via failed. Targets whose level cannot be resolved are kept unscaled. The
// downrank application has no consumer on this path (no aura-application
// site at cast completion), so only the removal leg is modeled.
func (s *Server) applyCreatureAuraScaling(m *creatureMotion, spell wotlk.Spell, mask uint8, hitTargets []uint64) ([]uint64, bool) {
	var targetMask uint8
	for i := range spell.Effects {
		if spell.Effects[i].Effect != 0 {
			targetMask |= 1 << uint(i)
		}
	}
	var firstLevel uint32
	if s != nil && s.Data != nil {
		if fr, found, _ := s.Data.Spell(s.spellFirstRank(spell.ID)); found {
			firstLevel = fr.SpellLevel
		}
	}
	kept := make([]uint64, 0, len(hitTargets))
	for _, guid := range hitTargets {
		if guid == 0 || guid == m.GUID || targetMask != mask {
			kept = append(kept, guid)
			continue
		}
		level, ok := s.serverAuraScaleTargetLevel(m, guid)
		if !ok || uint32(level)+10 < firstLevel {
			continue
		}
		kept = append(kept, guid)
	}
	if len(hitTargets) > 0 && len(kept) == 0 {
		return nil, true
	}
	return kept, false
}

// fireCreatureSpellLinkedTriggers applies the spell_linked_spell tail of
// Spell::_cast (Spell.cpp:3503-3512) for a creature caster: rows of
// spell_linked_spell with type 0 fire at cast completion — negative ids
// RemoveAurasDueToSpell the caster's own auras of -id (the
// unitCaster->ToUnit() arm covers creatures), positive ids are cast
// triggered by the caster on the effective unit target
// (m_targets.GetUnitTarget(), falling back to the caster when null).
func (s *Server) fireCreatureSpellLinkedTriggers(ctx context.Context, m *creatureMotion, spellID uint32, targetGUID uint64) {
	if s == nil || m == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT spell_effect FROM spell_linked_spell WHERE spell_trigger = ? AND type = 0", int32(spellID))
	if err != nil {
		if !missingTable(err) {
			s.debug("spell_linked_spell query failed", "spell", spellID, "err", err)
		}
		return
	}
	var effects []int32
	for rows.Next() {
		var effect int32
		if err := rows.Scan(&effect); err == nil {
			effects = append(effects, effect)
		}
	}
	rows.Close()
	for _, id := range effects {
		if id < 0 {
			s.removeCreatureAura(creatureAuraKeyForMotion(m), uint32(-id))
			continue
		}
		if id == 0 {
			continue
		}
		tgt := targetGUID
		if tgt == 0 {
			tgt = m.GUID
		}
		s.castCreatureSpellInternal(ctx, m, uint32(id), tgt, true)
	}
}

// fireCreatureSpellHitTarget fires Eluna CREATURE_EVENT_ON_SPELL_HIT_TARGET
// (15) on the caster creature's bindings when its spell hits a unit target,
// mirroring the caster arm of Spell::TargetInfo::DoDamageAndTriggers
// (Spell.cpp:2643-2648). The target resolves to a player object for session
// GUIDs and a creature object for creature GUIDs (the
// ObjectAccessor::GetUnit(*caster, guid) model); an unresolvable target
// passes nil, the way a despawned WorldObject would.
func (s *Server) fireCreatureSpellHitTarget(ctx context.Context, m *creatureMotion, targetGUID uint64, spellID uint32) {
	if s == nil || m == nil {
		return
	}
	var target *scripting.Object
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		target = ts.luaPlayer()
	} else if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil {
		target = s.luaMotionCreature(tm)
	}
	s.fireCreatureLuaEvent(ctx, m, scripting.CreatureEventOnSpellHitTarget, target, spellID)
}

// creatureEffectInstaKill mirrors Spell::EffectInstaKill
// (SpellEffects.cpp:274-296) for a creature caster: the per-hit-target arm
// of each effect-1 effect. Gates (dead/ghost, god-cheat) run first, then
// SMSG_SPELLINSTAKILLLOG fans out to the caster's visibility set
// (m_caster->SendMessageToSet, 100.0f), then the target dies — C++ sends the
// packet before DealDamage, so the ordering matches. The m_caster ==
// unitTarget finish() leg is vacuous (no client-side interrupt message on
// this path). Player victims die through the server-side creature-kill
// funnel: health zeroed, killPlayer with the charmer/owner player session as
// killer (nil for wild creatures — GetCharmerOrOwnerPlayerOrPlayerItself),
// the KilledByCreature hook and BossAI OnKillPlayer — the same funnel as the
// creature aura-damage kill. C++ EffectInstaKill's DealDamage passes
// durabilityLoss=false, so the kill carries no durability roll. Creature
// victims die through creatureInstaKillVictim: the NODAMAGE DealDamage legs
// that survive the damagetype guard (AI DamageTaken/DamageDealt hooks, the
// tap/loot-recipient block) plus the Unit::Kill chain. The skipped
// damagetype-guarded legs (split/share damage, aura interrupts, damage
// procs) stay skipped per the 13:4x audit. The npcbot legs stay structural
// (third-party mod, no Go model) and sScriptMgr->OnDamage is a no-op in this
// tree (ScriptMgr.h:430 base no-op; the bundled LuaEngine binds no server
// OnDamage hook). The vehicle-mounted duel arm stays unbridged (no vehicle
// model); the plain duel arm and the OwnerAttackedBy signal are bridged in
// the player-victim path, the signal also in creatureInstaKillVictim.
func (s *Server) creatureEffectInstaKill(ctx context.Context, m *creatureMotion, spellID uint32, hitTargets []uint64, reflected map[uint64]uint8, evaded map[uint64]bool) {
	if s == nil || m == nil {
		return
	}
	for _, guid := range hitTargets {
		if guid == 0 || evaded[guid] {
			continue
		}
		if _, isReflected := reflected[guid]; isReflected {
			continue
		}
		if vs := s.findSessionByGUID(guid); vs != nil && vs.player != nil {
			if vs.player.Health == 0 || vs.isDeadOrGhost() || vs.godCheatActive() {
				continue
			}
		} else if tm := s.findCreatureMotion(m.Map, m.InstanceID, guid); tm == nil || tm.Health == 0 {
			continue
		}
		packet := protocol.NewBuffer(20)
		packet.WriteU64(m.GUID)
		packet.WriteU64(guid)
		packet.WriteU32(spellID)
		s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_SPELLINSTAKILLLOG), packet.Bytes(), m.Map, m.InstanceID, m.X, m.Y, m.Z, nil)
		if vs := s.findSessionByGUID(guid); vs != nil && vs.player != nil {
			// Unit::DealDamage (Unit.cpp:728-733): the victim's controlled
			// creatures are signaled OwnerAttackedBy on any non-DoT damage
			// (NODAMAGE qualifies). Fires before the duel arm, matching C++
			// order.
			s.triggerPetDefensive(ctx, vs.player.Map, vs.player.InstanceID, vs.playerGUID, m.GUID, scripting.CreatureEventOnOwnerAttackedAt)
			// Unit::DealDamage duel arm (Unit.cpp:825-853, 957-973): lethal
			// damage on a duelist ends the duel — the duel opponent's hit is
			// clamped to health-1, so the victim survives at 1 HP and no kill
			// follows. The vehicle-mounted arm stays unbridged (no vehicle
			// model); a non-opponent kill falls through to killPlayer, whose
			// interruptDuel mirrors Unit::Kill's DUEL_INTERRUPTED arm
			// (Unit.cpp:11359-11365).
			if duelDefeatOnDamage(vs, vs.controllingPlayerGUID(m.GUID), vs.player.Health, vs.player.Health) {
				continue
			}
			vs.player.Health = 0
			var creatureKiller *session
			if m.OwnerGUID != 0 {
				creatureKiller = s.findSessionByGUID(m.OwnerGUID)
			}
			vs.killPlayer(ctx, creatureKiller, false, false)
			s.fireKilledByCreatureHook(ctx, m, vs)
			if m.BossAI != nil {
				m.BossAI.OnKillPlayer(ctx, s, m, guid)
			}
			continue
		}
		s.creatureInstaKillVictim(ctx, m, guid)
	}
}

// creatureEffectSummonType mirrors Spell::EffectSummonType
// (SpellEffects.cpp:2194-2440) for a creature caster: the HIT-mode arm that
// spawns the effect's MiscValue creature entry at the cast's dest position
// (the Go creature path's resolved dest, else the caster's position). Like
// the Transmitted/InstaKill legs above, C++ executes effects in effect-index
// order; this path groups by effect instead (documented approximation).
// Only the default WILD/ALLY/UNK leg (SpellEffects.cpp:2319-2341) is
// bridged: the SummonGuardian (pet/guardian/minion titles, Flags&512),
// charm (PUPPET), vehicle, totem/lightwell, and minipet arms have no Go
// model — the multi-summon property-ID list (SpellEffects.cpp:2225-2242)
// keys on the effect's damage value, which this path does not model, so
// numSummons is always 1 (documented delta). The timed/dead summon-type
// choice (SpellEffects.cpp:2322) uses the bridge's 0-based scheme; the
// ALLY owner/faction stamp (2333-2337), the Eluna::JustSummoned fire
// (CreatureHooks.cpp:165-171 — the shared summonTempCreature funnel fires
// CREATURE_EVENT_ON_JUST_SUMMONED_CREATURE (19) on the caster), and the
// ExecuteLogEffectSummonObject SMSG_SPELLLOGEXECUTE write (SpellEffects.cpp
// spell log convention, Transmitted-leg format) are twinned.
func (s *Server) creatureEffectSummonType(ctx context.Context, m *creatureMotion, spell wotlk.Spell, hasDest bool, dx, dy, dz float32) {
	if s == nil || m == nil || s.Data == nil {
		return
	}
	lvl := m.Level
	if lvl == 0 {
		lvl = 80
	}
	var duration int32
	if spell.DurationIndex > 0 {
		if dur, ok, derr := s.Data.SpellDuration(spell.DurationIndex, uint32(lvl)); derr == nil && ok && dur > 0 {
			duration = dur
		}
	}
	for i := range spell.Effects {
		eff := spell.Effects[i]
		if eff.Effect != spellEffectSummon {
			continue
		}
		entry := uint32(0)
		if eff.MiscValue > 0 {
			entry = uint32(eff.MiscValue)
		}
		if entry == 0 {
			continue
		}
		props, found, err := s.Data.SummonProperties(uint32(eff.MiscValueB))
		if err != nil || !found {
			continue
		}
		switch props.Control {
		case summonCategoryWild, summonCategoryAlly, summonCategoryUnk:
		default:
			continue // PET/PUPPET/VEHICLE: guardian, charm, vehicle arms unmodeled
		}
		if props.Flags&512 != 0 {
			continue // SummonGuardian arm (SpellEffects.cpp:2248-2251)
		}
		switch props.Title {
		case summonTitlePet, summonTitleGuardian, summonTitleMinion, summonTitleGuardian2:
			continue // SummonGuardian arms (SpellEffects.cpp:2255-2259)
		case summonTitleTotem, summonTitleLightwell, summonTitleMinipet, summonTitleVehicle, summonTitleVehicle2:
			continue // totem/lightwell/minipet/vehicle arms unmodeled
		}
		// Default leg (SpellEffects.cpp:2319-2341): timed when the spell
		// duration is positive, dead-despawn otherwise.
		summonType := uint8(tempSummonDeadDespawn)
		var despawnMs uint32
		if duration > 0 {
			summonType = uint8(tempSummonTimedDespawn)
			despawnMs = uint32(duration)
		}
		x, y, z := m.X, m.Y, m.Z
		if hasDest {
			x, y, z = dx, dy, dz
		}
		summoned := s.summonTempCreature(ctx, s.summonOriginFromMotion(m), entry, x, y, z, m.Orientation, summonType, despawnMs)
		if summoned == nil {
			continue
		}
		if props.Control == summonCategoryAlly {
			s.motionMu.Lock()
			summoned.OwnerGUID = m.GUID
			summoned.Faction = m.Faction
			s.motionMu.Unlock()
		}
		log := protocol.NewBuffer(32)
		log.WritePackedGUID(m.GUID)
		log.WriteU32(spell.ID)
		log.WriteU32(1)
		log.WriteU32(spellEffectSummon)
		log.WriteU32(1)
		log.WritePackedGUID(summoned.GUID)
		s.broadcastToInstance(m.Map, m.InstanceID, uint16(protocol.OpcodeSMSG_SPELLLOGEXECUTE), log.Bytes(), nil)
	}
}

// creatureInstaKillVictim runs the creature-victim legs of a creature
// caster's EffectInstaKill: Unit::DealDamage with damagetype NODAMAGE
// (Unit.cpp:696) followed by Unit::Kill (Unit.cpp:11153). Only the legs
// outside the damagetype != NODAMAGE guard survive: the AI DamageTaken /
// DamageDealt hooks, the tap + loot-recipient + damage-requirement block
// (Unit.cpp:872-876), and the Kill chain. The skipped legs (damage log,
// split/share, aura interrupts, DoDamageAndTriggers procs) were verified
// absent for NODAMAGE in the 13:4x audit. With a charmer/owner player the
// full Kill chain runs on the owner's session (the pet-kill path's model:
// party-kill log, Eluna events, KILL/KILLED/DEATH procs, killing-blow
// criteria, XP/loot, respawn); without one the minimal death registration
// runs, including the no-owner KILL/KILLED/DEATH proc loops
// (Unit.cpp:11257-11279) ahead of the death-aura strip. The OwnerAttackedBy
// signal (Unit.cpp:728-733) fires after the motion unlock below.
func (s *Server) creatureInstaKillVictim(ctx context.Context, m *creatureMotion, guid uint64) {
	if s == nil || m == nil || guid == 0 {
		return
	}
	var owner *session
	if m.OwnerGUID != 0 {
		owner = s.findSessionByGUID(m.OwnerGUID)
	}
	var tapPlayer, tapGroup uint64
	if owner != nil && owner.player != nil {
		tapPlayer, tapGroup = owner.playerGUID, owner.groupID
	}
	s.motionMu.Lock()
	tm := s.findCreatureMotionLocked(m.Map, m.InstanceID, guid)
	if tm == nil || tm.Health == 0 {
		s.motionMu.Unlock()
		return
	}
	if tm.BossAI == nil {
		tm.BossAI = getBossAIForCreature(tm, tm.ScriptName)
	}
	// Unit::DealDamage (Unit.cpp:701-704): the victim's DamageTaken and the
	// attacker's DamageDealt AI hooks fire for NODAMAGE too.
	damage := tm.Health
	fireDealDamageAIHooks(ctx, s, tm, m, m.GUID, guid, &damage)
	// Unit::DealDamage tap block (Unit.cpp:872-876): the killing blow taps
	// for the caster's charmer/owner player
	// (GetCharmerOrOwnerPlayerOrPlayerItself) and lowers the player-damage
	// requirement. A wild caster taps nothing — C++ sets the killer
	// creature itself as loot recipient, which has no Go player-tap model
	// — and the requirement stands, so no reward follows.
	if tapPlayer != 0 {
		s.recordCreatureTap(tm, tapPlayer, tapGroup, damage, tm.Health, true)
	}
	killedTarget := combatTarget{
		GUID:       tm.GUID,
		Map:        tm.Map,
		InstanceID: tm.InstanceID,
		X:          tm.X,
		Y:          tm.Y,
		Z:          tm.Z,
		Health:     tm.Health,
		MaxHealth:  tm.MaxHealth,
		Level:      uint8(tm.Level),
	}
	s.clearInstanceEncounter(tm)
	tm.Health = 0
	tm.DynamicFlags |= unitDynFlagLootable
	tm.InCombat = false
	tm.TargetGUID = 0
	tm.Moving = false
	if tm.ThreatMgr != nil {
		tm.ThreatMgr.ClearThreat()
	}
	s.motionMu.Unlock()
	// Unit::DealDamage (Unit.cpp:728-733): the victim's controlled creatures
	// are signaled OwnerAttackedBy on any non-DoT damage (NODAMAGE
	// qualifies). Fires outside the motion lock — the Eluna hook re-enters
	// it on demand.
	s.triggerPetDefensive(ctx, tm.Map, tm.InstanceID, tm.GUID, m.GUID, scripting.CreatureEventOnOwnerAttackedAt)
	s.stopCreatureMotionInInstance(tm.Map, tm.InstanceID, tm.GUID, tm.X, tm.Y, tm.Z)
	s.broadcastCreatureValuesUpdateInInstance(tm.Map, tm.InstanceID, tm.GUID, map[int]uint32{
		unitFieldHealth:       0,
		unitFieldDynamicFlags: tm.DynamicFlags,
	})
	s.broadcastThreatClearInInstance(tm.Map, tm.InstanceID, tm.GUID)
	if owner != nil && owner.player != nil {
		// Unit::Kill's full chain on the charmer/owner player session.
		owner.onCreatureKilled(ctx, killedTarget, m)
		return
	}
	// No charmer/owner player: the minimal death registration. Unit::Kill's
	// KILL/KILLED/DEATH proc loops (Unit.cpp:11257-11279) fire first, while
	// the victim's auras are still present — "Proc auras on death - must be
	// before aura/combat remove" — then the death-aura strip (Unit.cpp:11320)
	// runs ahead of the hooks. The KILL leg runs on the attacker's auras and
	// the KILLED leg on the victim's, both gated on the victim not being a
	// critter; the DEATH leg is unconditional. The pet/totem-owner arm
	// (Unit.cpp:11257-11263) is vacuous here: a no-owner attacker has no
	// player owner unit, and a creature-owned pet/totem has no Go owner
	// motion to proc — kill.go's owner-path arm covers the player-owned
	// case, and the pet's own aura leg stays unmodeled there too.
	if tm.CreatureType != creatureTypeCritter {
		s.fireCreatureMeleeAuraProcs(ctx, m, tm.GUID, procEventInfo{
			typeMask:       procFlagKill,
			spellTypeMask:  procSpellTypeMaskAll,
			spellPhaseMask: procSpellPhaseNone,
			hitMask:        procHitNone,
			actorGUID:      m.GUID,
		})
		s.fireCreatureMeleeAuraProcs(ctx, tm, m.GUID, procEventInfo{
			typeMask:       procFlagKilled,
			spellTypeMask:  procSpellTypeMaskAll,
			spellPhaseMask: procSpellPhaseNone,
			hitMask:        procHitNone,
			actorGUID:      tm.GUID,
		})
	}
	s.fireCreatureMeleeAuraProcs(ctx, tm, tm.GUID, procEventInfo{
		typeMask:       procFlagDeath,
		spellTypeMask:  procSpellTypeMaskAll,
		spellPhaseMask: procSpellPhaseNone,
		hitMask:        procHitNone,
		actorGUID:      tm.GUID,
	})
	deathKey := creatureAuraKeyForMotion(tm)
	s.auraMu.Lock()
	var deathAuras []*activeAura
	for _, aura := range s.activeCreatureAuras[deathKey] {
		deathAuras = append(deathAuras, aura)
	}
	s.auraMu.Unlock()
	s.grantChannelDeathItems(ctx, deathAuras, tm.Level, true, tm.TapPlayerGUID, tm.TapGroupID)
	s.clearCreatureAurasOnDeath(deathKey)
	s.removeThreatVictimFromAllLists(tm.Map, tm.InstanceID, tm.GUID)
	// Unit::Kill's KilledUnit branch arm (Unit.cpp:11385-11387): a
	// non-pet creature attacker fires only the branch arm — the pet-arm
	// double dispatch is pet-specific.
	s.fireCreatureTargetDied(ctx, m, s.luaMotionCreature(tm))
	// BossAI::_JustDied (ScriptedCreature.cpp:512-518).
	if tm.BossAI != nil {
		tm.BossAI.OnDied(ctx, s, tm)
	}
	// Creature::RemoveCorpse schedules the respawn for every death; the
	// owner path gets it from onCreatureKilled.
	if tm.InstanceID == 0 {
		s.scheduleCreatureRespawn(ctx, uint32(tm.GUID&0x00FFFFFF), uint32(math.Max(float64(tm.MaxHealth), 1)), time.Now())
	} else {
		s.scheduleInstanceCreatureRespawn(ctx, killedTarget, uint32(math.Max(float64(tm.MaxHealth), 1)), time.Now())
	}
}

// prepareCreatureHitTriggerSpells mirrors Spell::PrepareTriggersExecutedOnHit
// (Spell.cpp:8176-8206) for a creature caster: it snapshots the caster
// creature's SPELL_AURA_ADD_TARGET_TRIGGER aura effects present at cast
// completion, so triggered auras gained mid-cast cannot affect the caster and
// proc chance uses the combo-point-independent base amount. Same arms as the
// player-path snapshot (AuraEffect::IsAffectedOnSpell family gate,
// TriggerSpell DBC-known gate, base-amount chance times stack amount — the
// CalculateSpellDamage fold stays a documented delta, as on that path).
func (s *Server) prepareCreatureHitTriggerSpells(m *creatureMotion, spell wotlk.Spell) []spellHitTrigger {
	if s == nil || m == nil || s.Data == nil {
		return nil
	}
	key := creatureAuraKeyForMotion(m)
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	var triggers []spellHitTrigger
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		auraSpell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if effect.Aura != spellAuraAddTargetTrigger || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			// AuraEffect::IsAffectedOnSpell (SpellAuraEffects.cpp:848).
			if !spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
				continue
			}
			if effect.TriggerSpell == 0 {
				continue
			}
			if _, found, err := s.Data.Spell(effect.TriggerSpell); err != nil || !found {
				continue
			}
			chance := aura.BaseAmounts[index]
			if chance == 0 {
				chance = effect.BasePoints
			}
			chance *= int32(aura.StackAmount)
			triggers = append(triggers, spellHitTrigger{
				TriggerSpellID:       effect.TriggerSpell,
				TriggeredByAuraSpell: aura.SpellID,
				Chance:               chance,
			})
		}
	}
	return triggers
}

// canExecuteHitTriggers mirrors Spell::CanExecuteTriggersOnHit
// (Spell.cpp:8164-8174) for either caster path: an effect bit in the hit
// mask fires the trigger unless the triggering aura carries
// SPELL_ATTR4_PROC_ONLY_ON_CASTER (SharedDefines.h:561), in which case only
// effects whose implicit target A is TARGET_UNIT_CASTER
// (SharedDefines.h:1442) qualify.
func (s *Server) canExecuteHitTriggers(spell wotlk.Spell, effMask uint8, triggeredByAuraSpell uint32) bool {
	onlyOnCaster := false
	if triggeredByAuraSpell != 0 && s != nil && s.Data != nil {
		if auraSpell, found, err := s.Data.Spell(triggeredByAuraSpell); err == nil && found {
			onlyOnCaster = auraSpell.AttributesEx4&spellAttr4ProcOnlyOnCaster != 0
		}
	}
	for i, eff := range spell.Effects {
		if effMask&(1<<uint(i)) == 0 {
			continue
		}
		if !onlyOnCaster || eff.ImplicitTargetA == targetUnitCaster {
			return true
		}
	}
	return false
}

// fireCreatureHitTriggers mirrors Spell::DoTriggersOnSpellHit
// (Spell.cpp:2913-2957) for a creature caster, per hit target. C++ calls it
// inside DoTargetSpellHit after damage/healing is dealt (Spell.cpp:2650-2652);
// Go's creature path applies spell damage in the combat tick
// (creaturemotion.go), so the consumer runs at cast completion next to the
// spell_linked tail, the same placement that tail uses. Two legs:
//   - ADD_TARGET_TRIGGER snapshot: per trigger, the CanExecuteTriggersOnHit
//     effMask gate plus the roll_chance_i chance fires
//     m_caster->CastSpell(unit, triggeredSpell->Id, true) — a triggered cast
//     by the caster creature on the hit target; a triggered aura with no
//     duration (-1) inherits the remaining duration of the cast spell's own
//     aura on the target from the caster.
//   - spell_linked_spell hit rows (type 1, SPELL_LINK_HIT, SpellMgr.h:106):
//     negative ids RemoveAurasDueToSpell on the hit target, positive ids are
//     cast by the hit target on itself with the caster as original caster.
func (s *Server) fireCreatureHitTriggers(ctx context.Context, m *creatureMotion, spell wotlk.Spell, spellID uint32, hitTargets []uint64) {
	if s == nil || m == nil {
		return
	}
	// effMask is the mask of the spell's non-zero effects: Go applies every
	// non-zero effect to every hit target, so there is no per-target effect
	// filtering like C++'s per-target EffectMask.
	var hitEffMask uint8
	for effectIndex, eff := range spell.Effects {
		if eff.Effect != 0 {
			hitEffMask |= 1 << uint(effectIndex)
		}
	}
	triggers := s.prepareCreatureHitTriggerSpells(m, spell)
	for _, targetGUID := range hitTargets {
		if targetGUID == 0 {
			continue
		}
		// DoTargetSpellHit runs per unit target; GO guids flow through
		// hitTargets like C++'s AddGOTarget but are never hit targets.
		if high := uint16(targetGUID >> 48); high != 0 && high != 0xF130 {
			continue
		}
		for _, t := range triggers {
			if !s.canExecuteHitTriggers(spell, hitEffMask, t.TriggeredByAuraSpell) {
				continue
			}
			// roll_chance_i(chance): urand(0, 99) < chance.
			if rand.Float64()*100 >= float64(t.Chance) {
				continue
			}
			// m_caster->CastSpell(unit, triggeredSpell->Id, true): triggered
			// cast by the caster creature on the hit target.
			s.castCreatureSpellInternal(ctx, m, t.TriggerSpellID, targetGUID, true)
			s.inheritCreatureHitTriggerAuraDuration(ctx, m.GUID, spellID, t.TriggerSpellID, targetGUID, m.Map, m.InstanceID)
		}
		s.fireCreatureSpellLinkedHitTriggers(ctx, m, spellID, targetGUID)
	}
}

// fireCreatureSpellLinkedHitTriggers mirrors the spell_linked tail of
// Spell::DoTriggersOnSpellHit (Spell.cpp:2951-2957) for a creature caster:
// rows of spell_linked_spell with type 1 (id + SPELL_LINK_HIT,
// SpellMgr.h:106) fire per hit target — negative ids RemoveAurasDueToSpell
// on the hit target, positive ids are cast by the hit target on itself
// (unit->CastSpell(unit, *i, m_caster->GetGUID()) — TRIGGERED_FULL_MASK via
// CastSpellExtraArgs(ObjectGuid), SpellDefines.h:167) with the creature
// caster as original caster. (Spell.cpp carries a @todo to remove this
// table; Go mirrors the cast leg in fireCreatureSpellLinkedTriggers, so the
// hit leg is mirrored here for the same reason.)
func (s *Server) fireCreatureSpellLinkedHitTriggers(ctx context.Context, m *creatureMotion, spellID uint32, targetGUID uint64) {
	if s == nil || m == nil || s.WorldStore == nil || s.WorldStore.DB == nil || targetGUID == 0 {
		return
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT spell_effect FROM spell_linked_spell WHERE spell_trigger = ? AND type = 1", int32(spellID))
	if err != nil {
		if !missingTable(err) {
			s.debug("spell_linked_spell hit query failed", "spell", spellID, "err", err)
		}
		return
	}
	var effects []int32
	for rows.Next() {
		var effect int32
		if err := rows.Scan(&effect); err == nil {
			effects = append(effects, effect)
		}
	}
	rows.Close()
	for _, id := range effects {
		if id == 0 {
			continue
		}
		if id < 0 {
			// Unit::RemoveAurasDueToSpell on the hit target.
			linkedSpellID := uint32(-id)
			if ts := s.findSessionByGUID(targetGUID); ts != nil {
				ts.removeAura(linkedSpellID)
				continue
			}
			if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil {
				s.removeCreatureAura(creatureAuraKeyForMotion(tm), linkedSpellID)
			}
			continue
		}
		// unit->CastSpell(unit, id, m_caster->GetGUID()): the hit target
		// casts on itself, triggered. Player targets cast through their
		// own session; creature targets cast through the creature path.
		if ts := s.findSessionByGUID(targetGUID); ts != nil {
			ts.castSpellDirect(ctx, uint32(id), targetGUID)
			continue
		}
		if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil {
			s.castCreatureSpellInternal(ctx, tm, uint32(id), tm.GUID, true)
		}
	}
}

// fireCreatureMeleeAuraProcs evaluates real aura procs on the done side of
// a creature melee swing: the actor leg of Unit::ProcSkillsAndAuras
// (Unit.cpp:5356-5365) via Unit::TriggerAurasProcOnEvent
// (Unit.cpp:10385-10408). The attacker-side ProcSkillsAndReactives arm
// (Unit.cpp:10253-10327) is vacuous for wild creatures — its non-victim
// legs need a warrior player (Overpower), a hunter pet (Wolverine Bite),
// or an npcbot, so nothing fires for a normal creature attacker. The
// modOwner arm (Cobra Strikes, Unit.cpp:10395-10408) has no Go model
// (no GetSpellModOwner on the creature path).
//
// The caller builds the event: melee swings carry
// typeMask = PROC_FLAG_DONE_MELEE_AUTO_ATTACK | PROC_FLAG_DONE_MAINHAND_ATTACK
// (Unit.cpp:1194 — creatures swing BASE_ATTACK only), PROC_SPELL_TYPE_NONE,
// PROC_SPELL_PHASE_NONE, and the outcome-derived hit mask; the kill leg
// carries PROC_FLAG_KILL / PROC_SPELL_TYPE_MASK_ALL / PROC_HIT_NONE
// (Unit.cpp:11283-11290). The trigger spell targets the victim (melee) or
// the kill victim (kill).
func (s *Server) fireCreatureMeleeAuraProcs(ctx context.Context, m *creatureMotion, victimGUID uint64, ev procEventInfo) {
	if s == nil || m == nil || victimGUID == 0 {
		return
	}
	s.fireCreatureAuraProcLoop(ctx, m, victimGUID, ev)
}

// fireCreatureAuraProcLoop runs one aura-proc pass over the caster creature's
// own auras: the shared body of Unit::TriggerAurasProcOnEvent
// (Unit.cpp:10385-10408) for the creature path — each aura with a generated
// spell_proc entry runs the CanSpellTriggerProcOnEvent gate against the
// event; on pass, the chance roll fires each eligible aura effect's trigger
// spell on triggerTargetGUID. A zero trigger target mirrors C++'s null
// _procTarget (Unit::ProcSkillsAndAuras passes actionTarget as _procTarget,
// Unit.cpp:10388): the trigger spell resolves its own implicit targets, and
// the CheckExplicitTarget twin fizzles the spells that need an explicit unit
// target (SpellInfo.cpp:1784-1790). The modOwner arm
// (Unit.cpp:10395-10408) has no Go model — GetSpellModOwner returns null for
// wild and charmed creatures (only Pet objects carry UNIT_MASK_PET,
// Pet.cpp:49), and the creature path never runs pets or totems. The
// SetCantProc evaluation window (Unit.cpp:10424-10428) has no creature-side
// reader yet (no per-motion m_procDeep model); the _cast proc leg carries no
// CanProc gate in C++, so the omission is inert there.
func (s *Server) fireCreatureAuraProcLoop(ctx context.Context, m *creatureMotion, triggerTargetGUID uint64, ev procEventInfo) {
	if s == nil || m == nil {
		return
	}
	key := creatureAuraKeyForMotion(m)
	s.auraMu.Lock()
	auras := make([]*activeAura, 0, len(s.activeCreatureAuras[key]))
	for _, aura := range s.activeCreatureAuras[key] {
		auras = append(auras, aura)
	}
	s.auraMu.Unlock()
	for _, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		entry, auraSpell, ok := s.serverSpellProcEntryFor(aura.SpellID)
		if !ok {
			continue
		}
		// Session-side twin of the Aura::GetProcEffectMask self-trigger
		// guard (SpellAuras.cpp:2055-2057, Spell::IsTriggeredByAura,
		// Spell.h:544): inert on the creature path — the creature trigger
		// cast (castCreatureSpellInternal) carries no triggering-aura
		// attribution, so ev.triggeredByAuraSpell is always 0 here.
		if ev.eventSpell != nil && ev.triggeredByAuraSpell != 0 && ev.triggeredByAuraSpell == auraSpell.ID {
			continue
		}
		if entry.Charges > 0 && aura.RemainingCharges == 0 {
			continue
		}
		// C++ Aura::GetProcEffectMask (SpellAuras.cpp:2089-2091): a proc on
		// cooldown (Aura::IsProcOnCooldown, SpellAuras.cpp:2010-2013) gates
		// before the chance roll, per aura.
		if time.Now().Before(aura.ProcCooldownUntil) {
			continue
		}
		// C++ Aura::GetProcEffectMask (SpellAuras.cpp:2082-2086): a
		// charge-using aura with PROC_ATTR_REQ_SPELLMOD only procs when the
		// triggering spell's applied-mods registry holds the aura. Creature
		// melee/kill events carry no spell info and the creature path has
		// no spell-mod taking model, so the registry is empty and the check
		// always fails — skip, matching an empty m_appliedMods.
		if entry.AttributesMask&procAttrReqSpellmod != 0 {
			continue
		}
		if !canSpellTriggerProcOnEvent(entry, ev, auraSpell.AttributesEx3&spellAttr3CanProcWithTriggered != 0) {
			continue
		}
		// C++ GetProcEffectMask/TriggerProcOnEvent (SpellAuras.cpp:2045-2129,
		// 2192-2212): every eligible effect fires its own trigger spell.
		eligible := make([]int, 0, len(auraSpell.Effects))
		for i := range auraSpell.Effects {
			eff := &auraSpell.Effects[i]
			if eff.Effect == 0 || !isProcTriggerAuraType(eff.Aura) {
				continue
			}
			if aura.EffectMask&(1<<uint(i)) == 0 {
				continue
			}
			if !s.checkCreatureEffectProc(aura, eff, ev) {
				continue
			}
			eligible = append(eligible, i)
		}
		if len(eligible) == 0 {
			continue
		}
		if !s.rollCreatureProcChance(entry) {
			continue
		}
		// C++ PrepareProcToTrigger (SpellAuras.cpp:2032): a charge is taken
		// when the triggering spell is absent (melee/kill events carry no
		// spell info) or lacks SPELL_ATTR6_DONT_CONSUME_PROC_CHARGES.
		spent := false
		if entry.Charges > 0 && !procEventKeepsProcCharges(ev) {
			s.auraMu.Lock()
			aura.RemainingCharges--
			spent = aura.RemainingCharges == 0
			s.auraMu.Unlock()
		}
		// C++ Aura::PrepareProcToTrigger (SpellAuras.cpp:2042):
		// AddProcCooldown(now + procEntry->Cooldown), set after the chance
		// roll succeeded.
		if entry.CooldownMs > 0 {
			s.auraMu.Lock()
			aura.ProcCooldownUntil = time.Now().Add(time.Duration(entry.CooldownMs) * time.Millisecond)
			s.auraMu.Unlock()
		}
		for _, i := range eligible {
			eff := &auraSpell.Effects[i]
			if eff.Aura == spellAuraProcTriggerDamage {
				s.creatureProcTriggerDamageAuraProc(ctx, m, aura, auraSpell, i, triggerTargetGUID)
				continue
			}
			if eff.TriggerSpell == 0 {
				// C++ HandleProcTriggerSpellAuraProc warns and returns when
				// the effect carries no triggered spell
				// (SpellAuraEffects.cpp:5654-5661).
				continue
			}
			if eff.Aura == spellAuraProcTriggerSpellWithValue {
				// HandleProcTriggerSpellWithValueAuraProc passes the
				// triggering effect's own amount (SpellAuraEffects.cpp:5726-5728)
				// as SPELLVALUE_BASE_POINT0. castCreatureSpellInternal has
				// no base-point override, so this leg stays unbridged.
				continue
			}
			// m_caster->CastSpell(unit, triggeredSpell->Id, true): the
			// triggered cast by the creature on the trigger target.
			s.castCreatureSpellInternal(ctx, m, eff.TriggerSpell, triggerTargetGUID, true)
		}
		// Aura::TriggerProcOnEvent (SpellAuras.cpp:2209-2211): remove only
		// after all proc effects complete.
		if spent {
			s.removeCreatureAura(key, aura.SpellID)
		}
	}
}

// fireCreatureCastPhaseAuraProcs bridges the on-cast proc leg at the end of
// Spell::_cast (Spell.cpp:3525-3545) for a creature caster: after the
// spell_linked_spell tail, the caster's auras evaluate one proc event with
// PROC_SPELL_PHASE_CAST, PROC_SPELL_TYPE_MASK_ALL and the normalized hit
// mask — Unit::ProcSkillsAndAuras(m_originalCaster, nullptr, procAttacker,
// PROC_FLAG_NONE, ...). The type mask reuses spellFinishPhaseProcTypeMask:
// C++ runs the same m_procAttacker fallback (the prepareDataForTriggerSystem
// fill, else the magic/none positivity split) at both _cast legs
// (Spell.cpp:3529-3542 vs 3764-3776). The hit mask seeds PROC_HIT_NORMAL:
// C++ normalizes m_hitMask to NORMAL when no target crit (3537-3538), and Go
// applies no target effects at cast time on the creature path, so no crit
// info exists — the value the C++ normalization lands on for any
// non-critical cast. The event's triggered state is the cast's own (Go's
// triggered models TRIGGERED_FULL_MASK, which carries
// TRIGGERED_DISALLOW_PROC_EVENTS). The m_originalCaster null gate is vacuous
// (the motion is always present); SetExecutedCurrently has no Go model (no
// cast Spell object); CreatureAI::OnSpellCastFinished has no bridge (no AI
// classes in Go). Generated proc entries only carry PROC_SPELL_PHASE_HIT
// (serverSpellProcEntryFor, SpellMgr.cpp:1828) — like a C++ build with no
// spell_proc DB rows bearing PHASE_CAST, the event currently triggers
// nothing; the bridge is structural.
func (s *Server) fireCreatureCastPhaseAuraProcs(ctx context.Context, m *creatureMotion, spellID uint32, triggered bool) {
	if s == nil || m == nil || s.Data == nil {
		return
	}
	spell, found, err := s.Data.Spell(spellID)
	if err != nil || !found {
		return
	}
	typeMask := spellFinishPhaseProcTypeMask(spell)
	if typeMask == procFlagNone {
		return
	}
	schoolMask := spell.SchoolMask
	if schoolMask == 0 {
		schoolMask = 1
	}
	spellCopy := spell
	s.fireCreatureAuraProcLoop(ctx, m, 0, procEventInfo{
		typeMask:       typeMask,
		schoolMask:     schoolMask,
		spellTypeMask:  procSpellTypeMaskAll,
		spellPhaseMask: procSpellPhaseCast,
		hitMask:        procHitNormal,
		triggered:      triggered,
		eventSpell:     &spellCopy,
		actorGUID:      m.GUID,
	})
}

// fireCreaturePeriodicDamageTickProcs mirrors the Unit::ProcSkillsAndAuras
// tail of AuraEffect::HandlePeriodicDamageAurasTick
// (SpellAuraEffects.cpp:5210-5225) — and the health-leech tick's identical
// tail (SpellAuraEffects.cpp:5295-5305) — for creature actor/victim: the
// caster motion's own auras evaluate DONE_PERIODIC, the victim motion's own
// auras evaluate TAKEN_PERIODIC | (TAKEN_DAMAGE when damage landed), both
// PROC_SPELL_TYPE_DAMAGE / PROC_SPELL_PHASE_HIT with the absorb/resist-
// derived hit mask. A nil caster or victim skips that leg (dead victim,
// offline caster); the trigger spells target the other side, matching C++.
// The event is not marked triggered (C++ passes no Spell on the tick).
func (s *Server) fireCreaturePeriodicDamageTickProcs(ctx context.Context, caster, victim *creatureMotion, targetGUID, casterGUID uint64, tickSpell wotlk.Spell, schoolMask uint32, crit bool, absorbed, resisted, damage uint32) {
	if s == nil {
		return
	}
	hitMask := spellDamageProcHitMask(true, false, resisted > 0 && damage == 0, absorbed > 0 && damage == 0, crit, absorbed, false)
	if caster != nil {
		spellCopy := tickSpell
		s.fireCreatureAuraProcLoop(ctx, caster, targetGUID, procEventInfo{
			typeMask:       procFlagDonePeriodic,
			schoolMask:     schoolMask,
			spellTypeMask:  procSpellTypeDamage,
			spellPhaseMask: procSpellPhaseHit,
			hitMask:        hitMask,
			triggered:      false,
			eventSpell:     &spellCopy,
			actorGUID:      casterGUID,
			damage:         damage,
		})
	}
	if victim != nil {
		typeMask := procFlagTakenPeriodic
		if damage > 0 {
			typeMask |= procFlagTakenDamage
		}
		spellCopy := tickSpell
		s.fireCreatureAuraProcLoop(ctx, victim, casterGUID, procEventInfo{
			typeMask:       typeMask,
			schoolMask:     schoolMask,
			spellTypeMask:  procSpellTypeDamage,
			spellPhaseMask: procSpellPhaseHit,
			hitMask:        hitMask,
			triggered:      false,
			eventSpell:     &spellCopy,
			actorGUID:      casterGUID,
			damage:         damage,
		})
	}
}

// fireCreaturePeriodicHealTickProcs mirrors the Unit::ProcSkillsAndAuras
// tail of AuraEffect::HandlePeriodicHealAurasTick
// (SpellAuraEffects.cpp:5423-5433) for creature actor/victim: the caster
// motion's own auras evaluate DONE_PERIODIC, the victim motion's own auras
// evaluate TAKEN_PERIODIC, both PROC_SPELL_TYPE_HEAL / PROC_SPELL_PHASE_HIT
// with the crit-conditional hit mask. The caller carries the C++ gates:
// %-based (obs-mod-health) and item-cast ticks never proc.
func (s *Server) fireCreaturePeriodicHealTickProcs(ctx context.Context, caster, victim *creatureMotion, targetGUID, casterGUID uint64, tickSpell wotlk.Spell, schoolMask uint32, crit bool) {
	if s == nil {
		return
	}
	hitMask := procHitNormal
	if crit {
		hitMask = procHitCritical
	}
	if caster != nil {
		spellCopy := tickSpell
		s.fireCreatureAuraProcLoop(ctx, caster, targetGUID, procEventInfo{
			typeMask:       procFlagDonePeriodic,
			schoolMask:     schoolMask,
			spellTypeMask:  procSpellTypeHeal,
			spellPhaseMask: procSpellPhaseHit,
			hitMask:        hitMask,
			triggered:      false,
			eventSpell:     &spellCopy,
			actorGUID:      casterGUID,
		})
	}
	if victim != nil {
		spellCopy := tickSpell
		s.fireCreatureAuraProcLoop(ctx, victim, casterGUID, procEventInfo{
			typeMask:       procFlagTakenPeriodic,
			schoolMask:     schoolMask,
			spellTypeMask:  procSpellTypeHeal,
			spellPhaseMask: procSpellPhaseHit,
			hitMask:        hitMask,
			triggered:      false,
			eventSpell:     &spellCopy,
			actorGUID:      casterGUID,
		})
	}
}

// fireCreaturePeriodicPowerBurnTickProcs mirrors the Unit::ProcSkillsAndAuras
// tail of AuraEffect::HandlePeriodicPowerBurnAuraTick
// (SpellAuraEffects.cpp:5583-5623) for creature victims: the victim motion's
// own auras evaluate TAKEN_PERIODIC | (TAKEN_DAMAGE when damage landed) with
// PROC_SPELL_TYPE_NO_DMG_HEAL | (PROC_SPELL_TYPE_DAMAGE when damage landed) /
// PROC_SPELL_PHASE_HIT and the absorb/resist-derived hit mask (no crit model
// on the burn path, matching C++). The done side needs no motion arm: the Go
// burn tick only runs with a live caster session (burnCaster), which fires the
// session-side done leg. The event is not marked triggered (C++ passes no
// Spell on the tick).
func (s *Server) fireCreaturePeriodicPowerBurnTickProcs(ctx context.Context, victim *creatureMotion, targetGUID, casterGUID uint64, burnSpell wotlk.Spell, schoolMask uint32, absorbed, resisted, damage uint32) {
	if s == nil || victim == nil {
		return
	}
	typeMask := procFlagTakenPeriodic
	spellTypeMask := procSpellTypeNoDmgHeal
	if damage > 0 {
		typeMask |= procFlagTakenDamage
		spellTypeMask |= procSpellTypeDamage
	}
	spellCopy := burnSpell
	s.fireCreatureAuraProcLoop(ctx, victim, casterGUID, procEventInfo{
		typeMask:       typeMask,
		schoolMask:     schoolMask,
		spellTypeMask:  spellTypeMask,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        spellDamageProcHitMask(true, false, resisted > 0 && damage == 0, absorbed > 0 && damage == 0, false, absorbed, false),
		triggered:      false,
		eventSpell:     &spellCopy,
		actorGUID:      casterGUID,
		damage:         damage,
	})
}

// creatureProcTriggerDamageAuraProc mirrors
// AuraEffect::HandleProcTriggerDamageAuraProc (SpellAuraEffects.cpp:5738-5762)
// for a creature caster: the effect's own amount as direct spell damage of
// the aura spell's school on the proc target — no hit roll, no crit roll.
// C++ goes SpellDamageBonusDone -> CalculateSpellDamageTaken -> damage mods
// -> non-melee damage log -> DealSpellDamage. The done-bonus arm
// (SpellDamageBonusDone) has no creature-side model (no spell-power or
// spell mods on wild creatures), so the pipeline starts at the taken leg,
// mirroring the creature spell-damage victim block (creaturemotion.go).
func (s *Server) creatureProcTriggerDamageAuraProc(ctx context.Context, m *creatureMotion, aura *activeAura, auraSpell wotlk.Spell, effIndex int, victimGUID uint64) {
	if s == nil || m == nil || aura == nil || victimGUID == 0 {
		return
	}
	basePoint := aura.Amount
	if effIndex >= 0 && effIndex < len(aura.Amounts) && aura.Amounts[effIndex] > 0 {
		basePoint = uint32(aura.Amounts[effIndex])
	}
	if basePoint == 0 {
		return
	}
	schoolMask := uint32(1)
	if auraSpell.SchoolMask != 0 {
		schoolMask = auraSpell.SchoolMask
	}
	vs := s.findSessionByGUID(victimGUID)
	if vs == nil || vs.player == nil || vs.player.Health == 0 {
		return
	}
	damage := basePoint
	absorbed := uint32(0)
	// Unit::DealDamage (Unit.cpp:735-737): CHEAT_GOD victims take no damage.
	if vs.godCheatActive() {
		damage = 0
	} else if damage > 0 {
		absorbed, damage = vs.applyAbsorptionShields(damage, uint8(schoolMask))
	}
	overkill := uint32(0)
	now := time.Now()
	if !vs.godCheatActive() && damage > 0 {
		// Unit::CalcAbsorbResist (Unit.cpp:1977-2060): split-damage auras
		// (81/153) redirect part of the damage to the aura's caster, ahead
		// of the 300 share leg.
		damage = s.splitDamageToCaster(ctx, victimGUID, true, creatureAuraKey{}, m.GUID, damage, schoolMask)
		// Unit::DealDamage (Unit.cpp:766-788): SPELL_AURA_SHARE_DAMAGE_PCT.
		vs.splitShareDamagePct(ctx, victimGUID, true, creatureAuraKey{}, m.GUID, damage, schoolMask)
		// Unit::DealDamage (Unit.cpp:869-870): HIGHEST_HIT_RECEIVED.
		vs.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, damage)
		victimHealth := vs.player.Health
		if damage >= victimHealth {
			overkill = damage - victimHealth
			vs.player.Health = 0
			var creatureKiller *session
			if m.OwnerGUID != 0 {
				creatureKiller = s.findSessionByGUID(m.OwnerGUID)
			}
			vs.killPlayer(ctx, creatureKiller, false, true)
			// Eluna PLAYER_EVENT_ON_KILLED_BY_CREATURE (8): Unit::Kill tail
			// (Unit.cpp:11466-11480), attacker is the creature.
			s.fireKilledByCreatureHook(ctx, m, vs)
			if m.BossAI != nil {
				m.BossAI.OnKillPlayer(ctx, s, m, victimGUID)
			}
		} else {
			vs.player.Health -= damage
			// Unit::DealDamage (Unit.cpp:815-924): rage from damage
			// received — damage + absorbed for the conversion.
			vs.grantRageFromDamageTaken(ctx, damage+absorbed)
			// Reference Unit::DealDamage -> Spell::Delayed / DelayedChannel.
			vs.delayCurrentCast()
			vs.delayCurrentChannel()
			vs.procSpellDirectDamageAuras(auraSpell.ID, damage)
		}
		vs.lastCombatTime = now
		if vs.player.UnitFlags&unitFlagInCombat == 0 {
			vs.player.UnitFlags |= unitFlagInCombat
		}
	}
	logPkt := buildSpellNonMeleeDamageLog(victimGUID, m.GUID, auraSpell.ID, damage, overkill, uint8(schoolMask))
	_ = vs.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, true)
	s.broadcastToInstance(m.Map, m.InstanceID, uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, vs)
	vs.sendPlayerUpdate()
	if s != nil {
		s.triggerPetDefensive(ctx, vs.player.Map, vs.player.InstanceID, victimGUID, m.GUID, scripting.CreatureEventOnOwnerAttackedAt)
	}
}

// inheritCreatureHitTriggerAuraDuration mirrors the no-duration leg of
// Spell::DoTriggersOnSpellHit (Spell.cpp:2935-2949) for a creature caster:
// SPELL_AURA_ADD_TARGET_TRIGGER auras must not trigger auras without
// duration, so when the triggered spell's raw duration is -1
// (SpellInfo::GetDuration, SpellInfo.cpp:3077) the triggered aura — applied
// to the hit target by the caster creature — inherits the remaining duration
// of the cast spell's own aura on that target from the caster.
// Aura::GetDuration is the live remaining duration.
func (s *Server) inheritCreatureHitTriggerAuraDuration(ctx context.Context, casterGUID uint64, spellID, triggeredSpellID uint32, targetGUID uint64, mapID, instanceID uint32) {
	if s == nil || s.Data == nil {
		return
	}
	duration, found, err := s.Data.SpellDurationBase(triggeredSpellID)
	if err != nil || !found || duration != -1 {
		return
	}
	remainingOf := func(remainingMs uint32, updatedAt time.Time) uint32 {
		remaining := int64(remainingMs)
		if !updatedAt.IsZero() {
			if elapsed := time.Since(updatedAt).Milliseconds(); elapsed > 0 {
				if uint64(elapsed) < uint64(remaining) {
					remaining -= elapsed
				} else {
					remaining = 0
				}
			}
		}
		if remaining < 0 {
			remaining = 0
		}
		return uint32(remaining)
	}
	// unit->GetAura(spellId, casterGUID) on the hit target: player targets
	// carry their auras on their session.
	if ts := s.findSessionByGUID(targetGUID); ts != nil {
		var base, triggered *activeAura
		for _, aura := range ts.loadedAuras() {
			if aura == nil || aura.Stopped || aura.CasterGUID != casterGUID {
				continue
			}
			switch aura.SpellID {
			case spellID:
				base = aura
			case triggeredSpellID:
				triggered = aura
			}
		}
		if base == nil || triggered == nil {
			return
		}
		remaining := remainingOf(base.RemainingMs, base.DurationUpdatedAt)
		triggered.DurationMs = remaining
		triggered.RemainingMs = remaining
		triggered.DurationUpdatedAt = time.Now()
		// Aura::SetDuration pushes the client update
		// (SpellAuras.cpp:902); the triggered aura was applied as
		// permanent, so refresh the client's duration here.
		ts.sendAuraUpdate(triggered.Slot, triggeredSpellID, false, triggered.Positive, remaining, remaining)
		return
	}
	// Creature hit target: the server-side creature aura records.
	if tm := s.findCreatureMotion(mapID, instanceID, targetGUID); tm != nil {
		key := creatureAuraKeyForMotion(tm)
		s.auraMu.Lock()
		defer s.auraMu.Unlock()
		auras := s.activeCreatureAuras[key]
		if auras == nil {
			return
		}
		base, ok := auras[spellID]
		if !ok || base == nil || base.Stopped || base.CasterGUID != casterGUID {
			return
		}
		triggered, ok := auras[triggeredSpellID]
		if !ok || triggered == nil || triggered.Stopped || triggered.CasterGUID != casterGUID {
			return
		}
		remaining := remainingOf(base.RemainingMs, base.DurationUpdatedAt)
		triggered.DurationMs = remaining
		triggered.RemainingMs = remaining
		triggered.DurationUpdatedAt = time.Now()
	}
}

// creatureSummonPreCast mirrors the summon-family arms of Spell::CheckCast
// for a creature caster (Spell.cpp:5805-5946). The SUMMON pet/charm gates
// (5820-5823) and the whole SUMMON_PET arm (5839-5904) read the caster's
// owned-pet and charm GUIDs — Go's creature motions carry no pet-ownership
// or charm model (creatureMotion.PetID marks the motion as being a pet), so
// GetPetGUID()/GetCharmedGUID() are always zero here and C++ can never fire
// those arms for a creature caster: vacuous, structurally exact. The
// player-only arms are real fizzles:
//   - SPELL_EFFECT_SUMMON_PLAYER (5905) and SPELL_EFFECT_SUMMON_RAF_FRIEND
//     (5935): a non-player caster fails with SPELL_FAILED_BAD_TARGETS.
//   - SPELL_EFFECT_CREATE_TAMED_PET (5825): a non-player unit target fails
//     with SPELL_FAILED_BAD_TARGETS; a target that already has a pet fails
//     with SPELL_FAILED_ALREADY_HAVE_SUMMON unless the spell carries
//     SPELL_ATTR1_DISMISS_PET.
//
// Creature casts have no failure-packet path, so the SPELL_FAILED_* codes
// are internal only — false means fizzle.
func (s *Server) creatureSummonPreCast(m *creatureMotion, spell wotlk.Spell, targetGUID uint64) bool {
	if m == nil {
		return true
	}
	for _, eff := range spell.Effects {
		switch eff.Effect {
		case spellEffectSummonPlayer, spellEffectSummonRafFriend:
			return false
		case spellEffectCreateTamedPet:
			if targetGUID == 0 {
				continue
			}
			ts := s.findSessionByGUID(targetGUID)
			if ts == nil || ts.player == nil {
				return false
			}
			if spell.AttributesEx&spellAttr1DismissPet == 0 && ts.player.PetGUID != 0 {
				return false
			}
		}
	}
	return true
}

// creatureStealPreCast mirrors the SPELL_EFFECT_STEAL_BENEFICIAL_BUFF arm
// of Spell::CheckCast (Spell.cpp:5957-5984) for a creature caster: a null
// or self unit target fizzles (SPELL_FAILED_BAD_TARGETS); a target carrying
// no stealable aura under the effect's dispel mask fizzles
// (SPELL_FAILED_NOTHING_TO_STEAL). Creature casts have no failure-packet
// path, so the SPELL_FAILED_* codes are internal only — false means fizzle.
func (s *Server) creatureStealPreCast(m *creatureMotion, spell wotlk.Spell, targetGUID uint64) bool {
	if m == nil {
		return true
	}
	for _, eff := range spell.Effects {
		if eff.Effect != 126 { // SPELL_EFFECT_STEAL_BENEFICIAL_BUFF
			continue
		}
		if targetGUID == 0 || targetGUID == m.GUID {
			return false
		}
		if !s.unitHasStealableAura(m, targetGUID, getDispelMask(uint32(eff.MiscValue))) {
			return false
		}
	}
	return true
}

// unitHasStealableAura scans the target's auras the way the CheckCast steal
// arm scans Unit::GetVisibleAuras(): positive auras only, dispel-mask match,
// skipping passive auras and SPELL_ATTR4_NOT_STEALABLE spells — the
// EffectStealBeneficialBuff skip list (SpellEffects.cpp:5172). Unlike the
// dispel-list scan there is no hostility filter: the C++ arm checks
// AuraApplication::IsPositive() directly, and no dispel-resist or Unholy
// Blight terms (those live in GetDispellableAuraList, which the arm does
// not call).
func (s *Server) unitHasStealableAura(caster *creatureMotion, targetGUID uint64, dispelMask uint32) bool {
	if s == nil || caster == nil {
		return false
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		ts.castMu.Lock()
		defer ts.castMu.Unlock()
		for _, aura := range ts.activeAuras {
			if stealableAuraRecord(s, aura, dispelMask) {
				return true
			}
		}
		return false
	}
	tm := s.findCreatureMotion(caster.Map, caster.InstanceID, targetGUID)
	if tm == nil {
		return false
	}
	key := creatureAuraKeyForMotion(tm)
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	for _, aura := range s.activeCreatureAuras[key] {
		if stealableAuraRecord(s, aura, dispelMask) {
			return true
		}
	}
	return false
}

// stealableAuraRecord is the per-aura predicate of the CheckCast steal arm
// (Spell.cpp:5957-5984): positive, dispel-mask overlap, not passive, not
// SPELL_ATTR4_NOT_STEALABLE. Stopped auras are skipped like the other
// aura-list scans.
func stealableAuraRecord(s *Server, aura *activeAura, dispelMask uint32) bool {
	if aura == nil || aura.Stopped || !aura.Positive {
		return false
	}
	var sp wotlk.Spell
	var found bool
	if s != nil && s.Data != nil {
		sp, found, _ = s.Data.Spell(aura.SpellID)
	}
	dispelType := aura.DispelType
	if dispelType == 0 && found {
		dispelType = sp.DispelType
	}
	if getDispelMask(dispelType)&dispelMask == 0 {
		return false
	}
	if found {
		if sp.Attributes&spellAttributePassive != 0 {
			return false
		}
		if sp.AttributesEx4&spellAttr4NotStealable != 0 {
			return false
		}
	}
	return true
}

// creatureSpellHistoryReady mirrors the unit-cooldown arm of
// Spell::CheckCast (Spell.cpp:5207-5211) for creature casters via
// SpellHistory::IsReady/HasCooldown (SpellHistory.cpp:187-196, 473-487): the
// cast fizzles while the motion's spell-history map still holds the spell's
// own cooldown end, or — unless ignoreCategoryCooldown (triggered casts via
// IsIgnoringCooldowns, Spell.cpp:7506-7509) — its DBC category's end. The
// PreventionType SILENCE school-lockout arm of IsReady has no creature model
// (no interrupt school-lockout state) — documented no-bridge.
func (s *Server) creatureSpellHistoryReady(m *creatureMotion, spell wotlk.Spell, ignoreCategoryCooldown bool) bool {
	if m == nil {
		return true
	}
	now := time.Now()
	if end, ok := m.SpellCooldowns[spell.ID]; ok && now.Before(end) {
		return false
	}
	if !ignoreCategoryCooldown && spell.Category != 0 {
		if end, ok := m.SpellCategoryCooldowns[spell.Category]; ok && now.Before(end) {
			return false
		}
	}
	return true
}

// recordCreatureSpellCooldown mirrors Spell::SendSpellCooldown for a creature
// caster (Spell.cpp:3780-3791, called at 3463) via SpellHistory::StartCooldown
// (SpellHistory.cpp:283-355): a non-triggered cast starts a RecoveryTime
// cooldown on the spell and a CategoryRecoveryTime cooldown on its DBC
// category (SpellInfo::GetCategory, SpellInfo.cpp:877-880). Passive spells and
// cooldown-started-on-event spells are exempt (SpellHistory.cpp:181-184);
// triggered casts never record (HandleCooldowns returns early for
// IsIgnoringCooldowns, SpellHistory.cpp:165-166). The ranged-attack-time
// fallback (category 76 / auto-repeat, SpellHistory.cpp:307-309) and the
// SPELLMOD_COOLDOWN / SPELL_AURA_MOD_COOLDOWN applications have no creature
// model — documented no-bridges.
func (s *Server) recordCreatureSpellCooldown(m *creatureMotion, spell wotlk.Spell) {
	if s == nil || m == nil {
		return
	}
	if spell.Attributes&spellAttr0Passive != 0 || s.spellIsCooldownStartedOnEvent(spell) {
		return
	}
	cooldownMs := int64(spell.RecoveryTime)
	if cooldownMs < 0 {
		cooldownMs = 0
	}
	categoryMs := int64(spell.CategoryRecoveryTime)
	if categoryMs < 0 {
		categoryMs = 0
	}
	if cooldownMs == 0 && categoryMs == 0 {
		return
	}
	now := time.Now()
	if m.SpellCooldowns == nil {
		m.SpellCooldowns = make(map[uint32]time.Time)
	}
	// AddCooldown (SpellHistory.cpp:395): recTime is the spell cooldown, or
	// the category end when the spell has none; the per-spell entry exists
	// in both cases.
	if cooldownMs > 0 {
		m.SpellCooldowns[spell.ID] = now.Add(time.Duration(cooldownMs) * time.Millisecond)
	} else {
		m.SpellCooldowns[spell.ID] = now.Add(time.Duration(categoryMs) * time.Millisecond)
	}
	if categoryMs > 0 && spell.Category != 0 {
		catEnd := now.Add(time.Duration(categoryMs) * time.Millisecond)
		if m.SpellCategoryCooldowns == nil {
			m.SpellCategoryCooldowns = make(map[uint32]time.Time)
		}
		if m.SpellCooldownCategories == nil {
			m.SpellCooldownCategories = make(map[uint32]uint32)
		}
		if m.SpellCooldownCategoryEnds == nil {
			m.SpellCooldownCategoryEnds = make(map[uint32]time.Time)
		}
		m.SpellCategoryCooldowns[spell.Category] = catEnd
		m.SpellCooldownCategories[spell.ID] = spell.Category
		m.SpellCooldownCategoryEnds[spell.ID] = catEnd
	}
}

// checkCreatureSpellCasterState mirrors the caster-state block of
// Spell::CheckCast (Spell.cpp:5298-5314) for creature casters: the
// CasterAuraState / CasterAuraStateNot / CasterAuraSpell /
// ExcludeCasterAuraSpell gates and the reqCombat SPELL_FAILED_AFFECTING_COMBAT
// arm (SPELL_ATTR0_CANT_USED_IN_COMBAT, SpellInfo::CanBeUsedInCombat —
// SpellInfo.cpp:1200-1203). Triggered casts skip the whole block
// (TRIGGERED_IGNORE_CASTER_AURASTATE, SpellDefines.h:134), so every gate is
// non-triggered only. A gate failure fizzles the cast before SendSpellGo.
// Order follows C++: CasterAuraState, CasterAuraStateNot, CasterAuraSpell,
// ExcludeCasterAuraSpell, then the in-combat arm.
func (s *Server) checkCreatureSpellCasterState(m *creatureMotion, spell wotlk.Spell) bool {
	if spell.CasterAuraSpell != 0 && !s.creatureHasAura(m, spell.CasterAuraSpell) {
		return false
	}
	if spell.ExcludeCasterAuraSpell != 0 && s.creatureHasAura(m, spell.ExcludeCasterAuraSpell) {
		return false
	}
	if spell.CasterAuraState != 0 && !s.creatureCasterHasAuraState(m, spell, spell.CasterAuraState) {
		return false
	}
	if spell.ExcludeCasterAuraState != 0 && s.creatureCasterHasAuraState(m, spell, spell.ExcludeCasterAuraState) {
		return false
	}
	if m != nil && m.InCombat && spell.Attributes&spellAttr0CantUsedInCombat != 0 &&
		!s.creatureCasterAuraStateReqCombatExempt(m, spell) {
		return false
	}
	return true
}

// creatureHasAura reports whether the creature carries the aura spell,
// mirroring the creature-target branch of targetHasAura (spells.go): both
// the presence map and the full active-creature-aura records count,
// like C++ Unit::HasAura.
func (s *Server) creatureHasAura(m *creatureMotion, spellID uint32) bool {
	if s == nil || m == nil || spellID == 0 {
		return false
	}
	key := creatureAuraKeyForMotion(m)
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	if _, found := s.creatureAuras[key][spellID]; found {
		return true
	}
	_, found := s.activeCreatureAuras[key][spellID]
	return found
}

// creatureCasterHasAuraState mirrors the session hasAuraState
// (aura_state.go) for a creature caster, used by the caster-state block of
// Spell::CheckCast (Spell.cpp:5298-5304). The SPELL_AURA_ABILITY_IGNORE_AURASTATE
// bypass runs first (Unit.cpp:5946-5954); per-caster states need an aura the
// creature itself applied (CasterGUID == motion GUID, Unit.cpp:5957-5965),
// otherwise the unit-wide mask (health thresholds + spell-granted states)
// decides. The GetSpellIdForDifficulty wrapper on the C++ side is omitted:
// the player-side bridge does the same.
func (s *Server) creatureCasterHasAuraState(m *creatureMotion, spell wotlk.Spell, state uint32) bool {
	if state == auraStateNone || s == nil || m == nil {
		return false
	}
	if s.creatureCasterIgnoresAuraState(m, spell) {
		return true
	}
	if uint32(1)<<(state-1)&perCasterAuraStateMask != 0 {
		key := creatureAuraKeyForMotion(m)
		s.auraMu.Lock()
		defer s.auraMu.Unlock()
		for _, aura := range s.activeCreatureAuras[key] {
			if aura == nil || aura.CasterGUID != m.GUID {
				continue
			}
			if spellAuraStateOf(s, aura.SpellID) == state {
				return true
			}
		}
		return false
	}
	bit := uint32(1) << (state - 1)
	return s.creatureCasterAuraStateMask(m)&bit != 0
}

// creatureCasterAuraStateMask builds the creature caster's unit-wide aura
// state mask: health thresholds plus one bit per aura-granted state from the
// server creature aura maps (Unit::ModifyAuraState, Unit.cpp:5880).
func (s *Server) creatureCasterAuraStateMask(m *creatureMotion) uint32 {
	if s == nil || m == nil {
		return 0
	}
	mask := auraStateHealthMask(m.Health, m.MaxHealth)
	key := creatureAuraKeyForMotion(m)
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	for spellID := range s.creatureAuras[key] {
		if state := spellAuraStateOf(s, spellID); state != auraStateNone {
			mask |= uint32(1) << (state - 1)
		}
	}
	for spellID := range s.activeCreatureAuras[key] {
		if state := spellAuraStateOf(s, spellID); state != auraStateNone {
			mask |= uint32(1) << (state - 1)
		}
	}
	return mask
}

// checkCreatureCasterAuras mirrors the CC-state half of
// Spell::CheckCasterAuras (Spell.cpp:6257-6389) for a creature caster,
// called from Spell::CheckCast at Spell.cpp:5509 — after CheckPower,
// matching the gate order in castCreatureSpellInternal. The ATTR6 pass,
// the ATTR5 usable-while-CC spells (with the Pain Suppression glyph
// carve-out), the cancel compositions (stun: MOD_STUN + STRANGULATE;
// silence: MOD_SILENCE + MOD_PACIFY_SILENCE; pacify: MOD_PACIFY +
// MOD_PACIFY_SILENCE; fear/confuse: single type), the mechanicCheck
// lambda, and the immune-shield banish arm all follow the player-side
// bridge (checkCasterAuras, spells.go).
//
// Delta: creatures carry no UNIT_FIELD_FLAGS CC bits — nothing sets
// UNIT_FLAG_STUNNED / SILENCED / PACIFIED / FLEEING / CONFUSED on a
// creatureMotion — so the CC state is derived from the creature's
// applied aura effects by type, the exact inverse of how C++ raises
// the flags (HandleAuraModStun/Fear/Confuse via SetControlled, and
// HandleAuraModSilence/Pacify directly, SpellAuraEffects.cpp). A
// fizzle returns false; creature casts have no failure-packet path, so
// the SPELL_FAILED_* codes and the PREVENTED_BY_MECHANIC param are
// internal only.
func (s *Server) checkCreatureCasterAuras(m *creatureMotion, spell wotlk.Spell) bool {
	if spell.AttributesEx6&spellAttr6IgnoreCasterAuras != 0 {
		return true
	}
	usableWhileStunned := spell.AttributesEx5&spellAttr5UsableWhileStunned != 0
	usableWhileFeared := spell.AttributesEx5&spellAttr5UsableWhileFeared != 0
	usableWhileConfused := spell.AttributesEx5&spellAttr5UsableWhileConfused != 0
	if spell.ID == 33206 && !s.creatureHasAura(m, 63248) { // Pain Suppression without Glyph of Pain Suppression
		usableWhileStunned = false
	}
	allowedMask := spellAllowedMechanicMask(spell)
	var result uint8
	var mechanic uint32
	switch {
	case len(s.creatureCasterAuraEffectsByType(m, spellAuraModStun)) > 0:
		if usableWhileStunned {
			result, mechanic = s.creatureCasterAurasMechanic(m, spell, allowedMask, spellAuraModStun)
		} else if !(s.creatureSpellCancelsAuraEffect(m, spell, spellAuraModStun, &mechanic) &&
			s.creatureSpellCancelsAuraEffect(m, spell, spellAuraStrangulate, &mechanic)) {
			result = spellFailedStunned
		} else if spell.Mechanic&29 != 0 && s.creatureHasAuraMechanic(m, 1<<18) {
			// Immune-shield arm (Spell.cpp:6327-6329): C++ ANDs the
			// mechanic enum value itself against MECHANIC_IMMUNE_SHIELD
			// (29); a banish mechanic on the caster re-fails the cast.
			result = spellFailedStunned
		}
	case len(s.creatureCasterAuraEffectsByType(m, spellAuraModSilence)) > 0 && spell.PreventionType == spellPreventionTypeSilence:
		if !(s.creatureSpellCancelsAuraEffect(m, spell, spellAuraModSilence, &mechanic) &&
			s.creatureSpellCancelsAuraEffect(m, spell, spellAuraModPacifySilence, &mechanic)) {
			result = spellFailedSilenced
		}
	case len(s.creatureCasterAuraEffectsByType(m, spellAuraModPacify)) > 0 && spell.PreventionType == spellPreventionTypePacify:
		if !(s.creatureSpellCancelsAuraEffect(m, spell, spellAuraModPacify, &mechanic) &&
			s.creatureSpellCancelsAuraEffect(m, spell, spellAuraModPacifySilence, &mechanic)) {
			result = spellFailedPacified
		}
	case len(s.creatureCasterAuraEffectsByType(m, spellAuraModFear)) > 0:
		if usableWhileFeared {
			result, mechanic = s.creatureCasterAurasMechanic(m, spell, allowedMask, spellAuraModFear)
		} else if !s.creatureSpellCancelsAuraEffect(m, spell, spellAuraModFear, &mechanic) {
			result = spellFailedFleeing
		}
	case len(s.creatureCasterAuraEffectsByType(m, spellAuraModConfuse)) > 0:
		if usableWhileConfused {
			result, mechanic = s.creatureCasterAurasMechanic(m, spell, allowedMask, spellAuraModConfuse)
		} else if !s.creatureSpellCancelsAuraEffect(m, spell, spellAuraModConfuse, &mechanic) {
			result = spellFailedConfused
		}
	}
	if result != 0 && mechanic != 0 {
		// SPELL_FAILED_PREVENTED_BY_MECHANIC (Spell.cpp:6385-6387) —
		// still a fizzle on the creature path.
		return false
	}
	return result == 0
}

// creatureCasterAuraEffectsByType mirrors the session
// casterAuraEffectsByType (spells.go) for a creature caster: the
// creature's applied aura effects of auraType, flattened per effect
// index, from the server's activeCreatureAuras under auraMu. Per-effect
// aura types resolve from the DBC spell row (the record's single
// AuraType carries only the first effect); unresolvable rows fall back
// to the record's AuraType, matching the grouped model. This feeds the
// CheckCasterAuras helpers the way Unit::GetAuraEffectsByType feeds
// them in C++ (Spell.cpp:6257+).
func (s *Server) creatureCasterAuraEffectsByType(m *creatureMotion, auraType uint32) []casterAuraEffectRef {
	if s == nil || m == nil {
		return nil
	}
	key := creatureAuraKeyForMotion(m)
	s.auraMu.Lock()
	auras := make(map[uint32]*activeAura, len(s.activeCreatureAuras[key]))
	for id, aura := range s.activeCreatureAuras[key] {
		auras[id] = aura
	}
	s.auraMu.Unlock()
	var out []casterAuraEffectRef
	for id, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		for i := 0; i < 3; i++ {
			if aura.EffectMask&(1<<uint(i)) == 0 {
				continue
			}
			t := aura.AuraType
			if s.Data != nil {
				if sp, found, err := s.Data.Spell(id); err == nil && found && i < len(sp.Effects) {
					t = sp.Effects[i].Aura
				}
			}
			if t != auraType {
				continue
			}
			out = append(out, casterAuraEffectRef{spellID: id, effIndex: i})
		}
	}
	return out
}

// creatureSpellCancelsAuraEffect mirrors the session
// checkSpellCancelsAuraEffect (Spell.cpp:6391-6415) for a creature
// caster: every creature aura effect of auraType must be cancelled by
// the casting spell; an empty effect list passes outright; on the
// first non-cancelled effect *mechanic takes the effect's mechanic,
// else the aura's spell mechanic. Unresolvable spell rows are
// permissive (the player-side convention).
func (s *Server) creatureSpellCancelsAuraEffect(m *creatureMotion, spell wotlk.Spell, auraType uint32, mechanic *uint32) bool {
	effects := s.creatureCasterAuraEffectsByType(m, auraType)
	if len(effects) == 0 {
		return true
	}
	for _, ref := range effects {
		if s.Data == nil {
			continue
		}
		auraSpell, found, err := s.Data.Spell(ref.spellID)
		if err != nil || !found {
			continue
		}
		if spellCancelsAuraEffect(spell, auraSpell, ref.effIndex) {
			continue
		}
		if mechanic != nil {
			*mechanic = auraSpell.Effects[ref.effIndex].Mechanic
			if *mechanic == 0 {
				*mechanic = auraSpell.Mechanic
			}
		}
		return false
	}
	return true
}

// creatureCasterAurasMechanic mirrors the mechanicCheck lambda of
// Spell::CheckCasterAuras (Spell.cpp:6294-6334) for a creature caster,
// the same semantics as the session checkCasterAurasMechanic
// (spells.go): a usable-while-CC spell still fizzles when a CC aura
// effect's whole-spell mechanic mask (spellMechanicMask, the
// GetAllEffectsMechanicMask analog) falls outside the casting spell's
// allowed mask. The first blocker wins; its mechanic (effect mechanic,
// else the aura's spell mechanic) is the would-be packet param. The
// C++ ABORT() default is unreachable.
func (s *Server) creatureCasterAurasMechanic(m *creatureMotion, spell wotlk.Spell, allowedMask, auraType uint32) (uint8, uint32) {
	var failure uint8
	switch auraType {
	case spellAuraModStun:
		failure = spellFailedStunned
	case spellAuraModFear:
		failure = spellFailedFleeing
	case spellAuraModConfuse:
		failure = spellFailedConfused
	default:
		return 0, 0
	}
	for _, ref := range s.creatureCasterAuraEffectsByType(m, auraType) {
		if s.Data == nil {
			continue
		}
		auraSpell, found, err := s.Data.Spell(ref.spellID)
		if err != nil || !found {
			continue
		}
		mechanicMask := spellMechanicMask(auraSpell)
		if mechanicMask == 0 || mechanicMask&allowedMask != 0 {
			continue
		}
		mechanic := auraSpell.Effects[ref.effIndex].Mechanic
		if mechanic == 0 {
			mechanic = auraSpell.Mechanic
		}
		return failure, mechanic
	}
	return 0, 0
}

// creatureHasAuraMechanic mirrors Unit::HasAuraWithMechanic (Unit.cpp)
// for a creature caster: true when any of the creature's live auras
// carries one of the mechanics in mask. Feeds the immune-shield arm of
// CheckCasterAuras (Spell.cpp:6327-6329).
func (s *Server) creatureHasAuraMechanic(m *creatureMotion, mask uint32) bool {
	if s == nil || s.Data == nil || m == nil {
		return false
	}
	key := creatureAuraKeyForMotion(m)
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	for id, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		if sp, found, err := s.Data.Spell(id); err == nil && found && spellMechanicMask(sp)&mask != 0 {
			return true
		}
	}
	return false
}

// checkCreatureSpellShapeshift mirrors the shapeshift leg of
// Spell::CheckCast (Spell.cpp:5247-5277, strict=true from prepare,
// Spell.cpp:3100) for a creature caster. Creatures carry no shapeshift
// form (no MOD_SHAPESHIFT apply path on creatureMotion), so the form is
// 0 and SpellInfo::CheckShapeshift (SpellInfo.cpp:1450-1493, the
// checkShapeshiftCastForm core) reduces to the Stances != 0 arm:
// SPELL_FAILED_ONLY_SHAPESHIFT unless ATTR2_NOT_NEED_SHAPESHIFT. The
// MOD_IGNORE_SHAPESHIFT exemption (IsAffectedOnSpell,
// SpellAuraEffects.cpp:848) and the ATTR0_ONLY_STEALTHED arm (needs a
// live STEALTH aura effect, the HasStealthAura analog, Unit.cpp) still
// apply. Triggered casts skip the leg (TRIGGERED_IGNORE_SHAPESHIFT);
// Go's triggered models the full mask, so the caller gates it.
func (s *Server) checkCreatureSpellShapeshift(m *creatureMotion, spell wotlk.Spell) bool {
	if s.creatureHasIgnoreShapeshiftAura(m, spell) {
		return true
	}
	if checkShapeshiftCastForm(s.Data, spell, 0) != 0 {
		return false
	}
	if spell.Attributes&spellAttr0OnlyStealthed != 0 &&
		len(s.creatureCasterAuraEffectsByType(m, spellAuraStealth)) == 0 {
		return false
	}
	return true
}

// creatureHasIgnoreShapeshiftAura mirrors the session
// hasIgnoreShapeshiftAura (shapeshift.go, Spell.cpp:5250-5260) for a
// creature caster: the shapeshift check is skipped when any
// SPELL_AURA_MOD_IGNORE_SHAPESHIFT aura effect of the creature is
// affected on the spell.
func (s *Server) creatureHasIgnoreShapeshiftAura(m *creatureMotion, spell wotlk.Spell) bool {
	if s == nil || s.Data == nil || m == nil {
		return false
	}
	key := creatureAuraKeyForMotion(m)
	s.auraMu.Lock()
	auras := make(map[uint32]*activeAura, len(s.activeCreatureAuras[key]))
	for id, aura := range s.activeCreatureAuras[key] {
		auras[id] = aura
	}
	s.auraMu.Unlock()
	for id, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		auraSpell, found, err := s.Data.Spell(id)
		if err != nil || !found {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if effect.Aura != spellAuraModIgnoreShapeshift || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
				return true
			}
		}
	}
	return false
}

// checkCreatureSpellFamilyBlocked mirrors the
// SPELL_AURA_BLOCK_SPELL_FAMILY leg of Spell::CheckCast
// (Spell.cpp:5277-5278): a creature carrying a BLOCK_SPELL_FAMILY aura
// effect whose MiscValue matches the spell's SpellFamilyName cannot
// cast it (SPELL_FAILED_SPELL_UNAVAILABLE). MiscValue resolves from
// the DBC effect row, not the record's single MiscValue.
func (s *Server) checkCreatureSpellFamilyBlocked(m *creatureMotion, spell wotlk.Spell) bool {
	if s == nil || s.Data == nil || m == nil {
		return true
	}
	key := creatureAuraKeyForMotion(m)
	s.auraMu.Lock()
	auras := make(map[uint32]*activeAura, len(s.activeCreatureAuras[key]))
	for id, aura := range s.activeCreatureAuras[key] {
		auras[id] = aura
	}
	s.auraMu.Unlock()
	for id, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		auraSpell, found, err := s.Data.Spell(id)
		if err != nil || !found {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if effect.Aura != spellAuraBlockSpellFamily || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if uint32(effect.MiscValue) == spell.SpellFamilyName {
				return false
			}
		}
	}
	return true
}

// creatureCasterIgnoresAuraState mirrors casterIgnoresAuraState for a
// creature caster: a SPELL_AURA_ABILITY_IGNORE_AURASTATE (262) effect on the
// creature's own auras whose family mask covers the spell makes every aura
// state query succeed (Unit.cpp:5946-5954). Presence-map entries carry no
// effect records, so only the full active-creature-aura records participate
// (noted gap — Lua-applied auras never grant 262).
func (s *Server) creatureCasterIgnoresAuraState(m *creatureMotion, spell wotlk.Spell) bool {
	if s == nil || s.Data == nil || m == nil {
		return false
	}
	key := creatureAuraKeyForMotion(m)
	s.auraMu.Lock()
	auras := make([]*activeAura, 0, len(s.activeCreatureAuras[key]))
	for _, aura := range s.activeCreatureAuras[key] {
		auras = append(auras, aura)
	}
	s.auraMu.Unlock()
	data := s.Data
	return auraStateBypassApplies(auras, func(spellID uint32) (wotlk.Spell, bool) {
		granting, found, err := data.Spell(spellID)
		return granting, err == nil && found
	}, spell, -1)
}

// creatureCasterAuraStateReqCombatExempt mirrors the session
// auraStateReqCombatExempt for a creature caster: a SPELL_AURA_ABILITY_IGNORE_AURASTATE
// (262) effect with MiscValue == 1 lifts the in-combat
// SPELL_ATTR0_CANT_USED_IN_COMBAT gate (Spell.cpp:5280-5292).
func (s *Server) creatureCasterAuraStateReqCombatExempt(m *creatureMotion, spell wotlk.Spell) bool {
	if s == nil || s.Data == nil || m == nil {
		return false
	}
	key := creatureAuraKeyForMotion(m)
	s.auraMu.Lock()
	auras := make([]*activeAura, 0, len(s.activeCreatureAuras[key]))
	for _, aura := range s.activeCreatureAuras[key] {
		auras = append(auras, aura)
	}
	s.auraMu.Unlock()
	data := s.Data
	return auraStateBypassApplies(auras, func(spellID uint32) (wotlk.Spell, bool) {
		granting, found, err := data.Spell(spellID)
		return granting, err == nil && found
	}, spell, 1)
}

// spellAuraStateOf is the server-level twin of the session lookupAuraState:
// it resolves a spell's aura state through the DBC store, returning
// auraStateNone when the spell is unknown.
func spellAuraStateOf(s *Server, spellID uint32) uint32 {
	if s == nil || s.Data == nil {
		return auraStateNone
	}
	if spell, found, err := s.Data.Spell(spellID); err == nil && found {
		return spellAuraState(spell)
	}
	return auraStateNone
}

// -------------------------------------------------------------
// Lua-driven bosses (fight logic in lua_scripts/, e.g. Karazhan).
// Reference: boss scripts under src/server/scripts/ ported to Lua.
// -------------------------------------------------------------
// luaBossAI is the no-op BossAI slot for bosses whose encounter logic lives
// in a Lua script. The Lua creature-event hooks (enter combat, leave combat,
// target died, died, reset) drive the fight; this shim exists so the engine
// treats the creature as a boss — instance-encounter admission locks on
// aggro (beginInstanceEncounter) and clears on evade/death, mirroring the
// C++ BossAI constructor's instance binding (e.g. DATA_MAIDEN_OF_VIRTUE).
// Register one entry per Lua-ported boss via RegisterLuaBoss.
type luaBossAI struct{}

func newLuaBossAI(m *creatureMotion) BossAI { return &luaBossAI{} }

func (ai *luaBossAI) OnReset(ctx context.Context, s *Server, m *creatureMotion) {}
func (ai *luaBossAI) OnAggro(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
}
func (ai *luaBossAI) OnDamageTaken(ctx context.Context, s *Server, m *creatureMotion, attacker uint64, damage *uint32) {
}

// UnitAI::HealReceived/HealDone base no-ops (UnitAI.h:307-310): Lua bosses
// receive heal legs through creature Lua events (event-driven), so the Go
// AI side stays a no-op like OnDamageTaken.
func (ai *luaBossAI) HealReceived(ctx context.Context, s *Server, m *creatureMotion, healerGUID uint64, addhealth *uint32) {
}

func (ai *luaBossAI) HealDone(ctx context.Context, s *Server, m *creatureMotion, victimGUID uint64, addhealth *uint32) {
}

// UnitAI::DamageDealt base no-op (UnitAI.h:300): fired on the attacker's
// AI from Unit::DealDamage (Unit.cpp:704). No script in this tree overrides
// it, so this stays a no-op like the heal legs above.
func (ai *luaBossAI) DamageDealt(ctx context.Context, s *Server, m *creatureMotion, victimGUID uint64, damage *uint32) {
}
func (ai *luaBossAI) OnKillPlayer(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
}
func (ai *luaBossAI) OnEvade(ctx context.Context, s *Server, m *creatureMotion) {}

// OnDied mirrors BossAI::_JustDied (ScriptedCreature.cpp:512-518) — no-op:
// Lua-scripted fights keep their death handling in the script (Eluna event
// 4, fired by fireCreatureDied just ahead of this hook).
func (ai *luaBossAI) OnDied(ctx context.Context, s *Server, m *creatureMotion) {}
func (ai *luaBossAI) OnUpdate(ctx context.Context, s *Server, m *creatureMotion, diff time.Duration, players []playerPos, now time.Time) {
}

// RegisterLuaBoss marks a creature entry as a Lua-scripted boss: its BossAI
// slot carries the no-op luaBossAI so engine boss handling (instance
// encounter admission) applies, while the Lua script drives the fight.
func RegisterLuaBoss(scriptName string, entry uint32) {
	RegisterBossAI(scriptName, entry, newLuaBossAI)
}

func init() {
	RegisterBossAI("boss_rhahkzor", 644, newRhahkZorAI)
	RegisterBossAI("boss_taragaman", 11520, newTaragamanAI)
	RegisterBossAI("boss_kresh", 3653, newKreshAI)
	// Maiden of Virtue (Karazhan) — fight logic in
	// lua_scripts/karazhan/boss_maiden_of_virtue.lua.
	RegisterLuaBoss("boss_maiden_of_virtue", 16457)
	// Moroes (Karazhan) — fight logic in
	// lua_scripts/karazhan/boss_moroes.lua.
	RegisterLuaBoss("boss_moroes", 15687)
	// Midnight (Karazhan) — fight logic in
	// lua_scripts/karazhan/boss_midnight.lua.
	RegisterLuaBoss("boss_midnight", 16151)
	// Attumen the Huntsman, unmounted (15550) and mounted (16152) —
	// fight logic in lua_scripts/karazhan/boss_midnight.lua.
	RegisterLuaBoss("boss_attumen", 15550)
	RegisterLuaBoss("boss_attumen", 16152)
	// Netherspite (Karazhan) — fight logic in
	// lua_scripts/karazhan/boss_netherspite.lua.
	RegisterLuaBoss("boss_netherspite", 15689)
	// The Curator (Karazhan) — fight logic in
	// lua_scripts/karazhan/boss_curator.lua.
	RegisterLuaBoss("boss_curator", 15691)
	// Nightbane (Karazhan) — fight logic in
	// lua_scripts/karazhan/boss_nightbane.lua.
	RegisterLuaBoss("boss_nightbane", 17225)
	// Prince Malchezaar (Karazhan) — fight logic in
	// lua_scripts/karazhan/boss_prince_malchezaar.lua.
	RegisterLuaBoss("boss_malchezaar", 15690)
	// Shade of Aran (Karazhan) — fight logic in
	// lua_scripts/karazhan/boss_shade_of_aran.lua (water elemental
	// 17167 AI "npc_aran_elemental" in the same file).
	RegisterLuaBoss("boss_shade_of_aran", 16524)
	// Terestian Illhoof (Karazhan) — fight logic in
	// lua_scripts/karazhan/boss_terestian_illhoof.lua (Kil'rek 17229 AI
	// "npc_kilrek" and fiendish imp 17267 AI "npc_fiendish_imp" in the
	// same file; demon chains 17248 and fiendish portal 17265 have no
	// Lua-modelable hooks).
	RegisterLuaBoss("boss_terestian_illhoof", 15688)
	// Opera Event (Karazhan) — fight logic in
	// lua_scripts/karazhan/bosses_opera.lua (Tito 17548 AI "npc_tito" in
	// the same file; cyclone 18412 and grandmother 17603 have no
	// Lua-modelable hooks).
	RegisterLuaBoss("boss_dorothee", 17535)
	RegisterLuaBoss("boss_strawman", 17543)
	RegisterLuaBoss("boss_tinhead", 17547)
	RegisterLuaBoss("boss_roar", 17546)
	RegisterLuaBoss("boss_crone", 18168)
	RegisterLuaBoss("boss_bigbadwolf", 17521)
	RegisterLuaBoss("boss_julianne", 17534)
	RegisterLuaBoss("boss_romulo", 17533)
	// Mana Feeder (Karazhan) — fight logic in
	// lua_scripts/karazhan/npc_mana_feeder.lua (the C++ Reset self-cast;
	// the school immunities have no bridge).
	RegisterLuaBoss("npc_mana_feeder", 16491)
	// Arcane Protector (Karazhan) — fight logic in
	// lua_scripts/karazhan/npc_arcane_protector.lua.
	RegisterLuaBoss("npc_arcane_protector", 16504)
	// Karazhan instance script (instance_karazhan.cpp, map 532) —
	// InstanceMapScript only: GUID latches on creature/GO create (Kil'rek,
	// Terestian, Moroes, Nightbane; 13 GO doors incl. stage doors,
	// side entrance, dust-covered chest, blackened urn), optional-boss
	// trash-death counter spawning Hyakiss/Shadikith/Rokad, Oz death
	// counter via SetData, opera-DONE door/flag unlock, chess-DONE
	// dust-covered-chest respawn, GetData OperaEvent (urand OZ..RAJ at
	// construction, never altered) and GetGuidData for 18 keys. No Lua
	// InstanceMapScript bridge (instance_deadmines, instance_blackrock_spire,
	// instance_gnomeregan precedent) — documented only.
	// Aki'lzon (Zul'Aman) — fight logic in
	// lua_scripts/zulaman/boss_akilzon.lua (soaring eagle 24858 AI
	// "npc_akilzon_eagle" in the same file).
	RegisterLuaBoss("boss_akilzon", 23574)
	// Halazzi (Zul'Aman) — fight logic in
	// lua_scripts/zulaman/boss_halazzi.lua (spirit lynx 24143 AI
	// "npc_halazzi_lynx" in the same file).
	RegisterLuaBoss("boss_halazzi", 23577)
	// Nalorakk (Zul'Aman) — fight logic in
	// lua_scripts/zulaman/boss_nalorakk.lua.
	RegisterLuaBoss("boss_nalorakk", 23576)
	// Jan'alai (Zul'Aman) — fight logic in
	// lua_scripts/zulaman/boss_janalai.lua (fire bomb 23920 AI
	// "npc_janalai_firebomb", dragonhawk hatchling 23598 AI
	// "npc_janalai_hatchling", and egg 23817 AI "npc_janalai_egg" in
	// the same file; hatcher 23818 AI "npc_janalai_hatcher" has no
	// Lua-modelable hooks and is not registered).
	RegisterLuaBoss("boss_janalai", 23578)
	// Hex Lord Malacrass (Zul'Aman) — fight logic in
	// lua_scripts/zulaman/boss_hexlord.lua (the eight add AIs — Thurg,
	// Alyson Antille, Slither, Lord Raadan, Gazakroth, Fenstalker,
	// Darkheart, Koragg — in the same file; the 23920 siphon-soul
	// trigger has no C++ script and the
	// spell_hexlord_unstable_affliction AuraScript is not modeled).
	RegisterLuaBoss("boss_hexlord_malacrass", 24239)
	// Zul'jin (Zul'Aman) — fight logic in
	// lua_scripts/zulaman/boss_zuljin.lua (feather vortex 24136 AI
	// "npc_zuljin_vortex" in the same file; the four animal spirits and
	// the column of fire have no C++ CreatureScript in boss_zuljin.cpp
	// and are not registered).
	RegisterLuaBoss("boss_zuljin", 23863)
	// Arlokk (Zul'Gurub) — fight logic in
	// lua_scripts/zulgurub/boss_arlokk.lua (zulian prowler 15101 AI
	// "npc_zulian_prowler" in the same file; the panther trigger 15091
	// has no C++ script and the go_gong_of_bethekk AI has no
	// Lua-modelable hooks — neither is registered).
	RegisterLuaBoss("boss_arlokk", 14515)
	// High Priestess Jeklik (Zul'Gurub) — fight logic in
	// lua_scripts/zulgurub/boss_jeklik.lua (frenzied bat 14965 AI
	// "npc_batrider" in the same file; the bloodseeker bat 11368 has no
	// C++ CreatureScript in boss_jeklik.cpp and is not registered).
	RegisterLuaBoss("boss_jeklik", 14517)
	// Gahz'ranka (Zul'Gurub) — fight logic in
	// lua_scripts/zulgurub/boss_gahzranka.lua. The fished-up summon has
	// no C++ CreatureScript in boss_gahzranka.cpp and is not registered.
	RegisterLuaBoss("boss_gahzranka", 15114)
	// Gri'lek of the Iron Blade (Zul'Gurub, Edge of Madness) — fight
	// logic in lua_scripts/zulgurub/boss_grilek.lua. The brazier summon
	// has no C++ CreatureScript in boss_grilek.cpp and is not registered.
	RegisterLuaBoss("boss_grilek", 15082)
	// Hazza'rah (Zul'Gurub, Edge of Madness) — fight logic in
	// lua_scripts/zulgurub/boss_hazzarah.lua. The brazier summon has no
	// C++ CreatureScript in boss_hazzarah.cpp and is not registered.
	RegisterLuaBoss("boss_hazzarah", 15083)
	// Hakkar the Soulflayer (Zul'Gurub, end boss) — fight logic in
	// lua_scripts/zulgurub/boss_hakkar.lua. The at_zulgurub_entrance
	// OnlyOnceAreaTriggerScript in boss_hakkar.cpp has no Lua
	// area-trigger bridge and is not registered.
	RegisterLuaBoss("boss_hakkar", 14834)
	// Jin'do the Hexxer (Zul'Gurub, optional boss) — fight logic in
	// lua_scripts/zulgurub/boss_jindo.lua. The npc_healing_ward and
	// npc_shade_of_jindo trash AIs from the same C++ file register
	// their own creature events in the Lua file and are not bosses.
	RegisterLuaBoss("boss_jindo", 11380)
	// Bloodlord Mandokir (Zul'Gurub, optional boss) — fight logic in
	// lua_scripts/zulgurub/boss_mandokir.lua. The npc_ohgan and
	// npc_vilebranch_speaker trash AIs from the same C++ file register
	// their own creature events in the Lua file and are not bosses.
	RegisterLuaBoss("boss_mandokir", 11382)
	// High Priestess Mar'li (Zul'Gurub, main boss) — fight logic in
	// lua_scripts/zulgurub/boss_marli.lua. The npc_spawn_of_marli trash
	// AI from the same C++ file registers its own creature events in the
	// Lua file and is not a boss; the gob_spider_egg gameobject script
	// and the spell_hatch_spiders SpellScript have no Lua bridge and are
	// not registered.
	RegisterLuaBoss("boss_marli", 14510)
	// Renataki of the Thousand Blades (Zul'Gurub, Edge of Madness) —
	// fight logic in lua_scripts/zulgurub/boss_renataki.lua. The
	// brazier summon has no C++ CreatureScript in boss_renataki.cpp
	// and is not registered.
	RegisterLuaBoss("boss_renataki", 15084)
	// High Priest Thekal (Zul'Gurub, main boss) — fight logic in
	// lua_scripts/zulgurub/boss_thekal.lua. The npc_zealot_lorkhan and
	// npc_zealot_zath add AIs from the same C++ file register their own
	// creature events in the Lua file and are not bosses.
	RegisterLuaBoss("boss_thekal", 14509)
	// High Priest Venoxis (Zul'Gurub, main boss) — fight logic in
	// lua_scripts/zulgurub/boss_venoxis.lua. The parasitic serpent 14884
	// has no C++ CreatureScript in boss_venoxis.cpp and is not registered.
	RegisterLuaBoss("boss_venoxis", 14507)
	// Wushoolay the Storm Witch (Zul'Gurub, Edge of Madness) — fight logic in
	// lua_scripts/zulgurub/boss_wushoolay.lua. The brazier summon has no
	// bearer (no summon model) — she is assumed already spawned.
	RegisterLuaBoss("boss_wushoolay", 15085)
	// Razorgore the Untamed (Blackwing Lair, first boss) — fight logic
	// in lua_scripts/blackwinglair/boss_razorgore.lua. The
	// go_orb_of_domination GameObjectScript and the spell_egg_event
	// SpellScript from the same C++ file have no Lua bridges and are
	// not registered; the phase-two DoAction trigger has no bridge.
	RegisterLuaBoss("boss_razorgore", 12435)
	// Vaelastrasz the Corrupt (Blackwing Lair, second boss) — fight
	// logic in lua_scripts/blackwinglair/boss_vaelastrasz.lua. The
	// spell_vael_burning_adrenaline AuraScript from the same C++ file
	// has no Lua bridge and is not registered; the pre-combat speech
	// machine runs through the gossip hooks in the same file.
	RegisterLuaBoss("boss_vaelastrasz", 13020)
	// Broodlord Lashlayer (Blackwing Lair, third boss) — fight logic
	// in lua_scripts/blackwinglair/boss_broodlord_lashlayer.lua. The
	// go_suppression_device GameObjectScript from the same C++ file
	// has no Lua bridge and is not registered; the leash-check evade
	// arm and the threat-cut arm have no bridges.
	RegisterLuaBoss("boss_broodlord", 12017)
	// Firemaw (Blackwing Lair, fourth boss) — fight logic in
	// lua_scripts/blackwinglair/boss_firemaw.lua. The wingbuffet
	// threat-cut arm has no bridge (no threat model).
	RegisterLuaBoss("boss_firemaw", 11983)
	// Ebonroc (Blackwing Lair, fifth boss) — fight logic in
	// lua_scripts/blackwinglair/boss_ebonroc.lua. The wingbuffet
	// threat-cut arm has no bridge (no threat model).
	RegisterLuaBoss("boss_ebonroc", 14601)
	// Flamegor (Blackwing Lair, sixth boss) — fight logic in
	// lua_scripts/blackwinglair/boss_flamegor.lua. The wingbuffet
	// threat-cut arm has no bridge (no threat model).
	RegisterLuaBoss("boss_flamegor", 11981)
	// Chromaggus (Blackwing Lair, seventh boss) — fight logic in
	// lua_scripts/blackwinglair/boss_chromaggus.lua. The
	// go_chromaggus_lever GameObjectScript from the same C++ file has
	// no Lua bridge and is not registered; the fight starts on pull
	// instead of on the lever, and the UpdateAI casting gates and the
	// threat model have no bridges.
	RegisterLuaBoss("boss_chromaggus", 14020)
	// Victor Nefarius (Blackwing Lair, eighth boss) — fight logic in
	// lua_scripts/blackwinglair/boss_nefarian.lua. The drakonid
	// summons, the Nefarian summon, the bone-construct transform, the
	// UBRS-only SetData/path/gameobject arms, the threat-reset arm and
	// the MovementInform zone-in-combat arm have no bridges; the
	// encounter-state bookkeeping is blocked on the instance-script
	// model.
	RegisterLuaBoss("boss_victor_nefarius", 10162)
	// Nefarian (Blackwing Lair, eighth boss) — fight logic in
	// lua_scripts/blackwinglair/boss_nefarian.lua. The UpdateAI
	// casting gates, the sub-20% bone-construct respawn loop and the
	// MovementInform arm have no bridges (no UNIT_STATE, no summon
	// model, no movement model); only the Talk arm of phase 3 is
	// modeled.
	RegisterLuaBoss("boss_nefarian", 11583)
	// instance_blackwing_lair (AddSC_instance_blackwing_lair; map 469 —
	// InstanceMapScript only, zero creature/at_ scripts): encounter gate
	// CheckRequiredBosses, Razorgore egg-event machine (EGG_EVENT SetData,
	// EVENT_RAZOR_SPAWN wave loop, EVENT_RAZOR_PHASE_TWO,
	// OnCreatureCreate summon-forward, OnUnitDeath charm-hack),
	// SetBossState egg-phase + Nefarius fail/respawn arms. Every arm needs
	// the instance-script model — the Lua surface has no InstanceMapScript
	// bridge, so this unit is documented-only (precedent:
	// instance_blackrock_spire).
	// Lucifron (Molten Core, first boss) — fight logic in
	// lua_scripts/moltencore/boss_lucifron.lua. The UpdateAI casting
	// gates have no bridge (no UNIT_STATE model); the encounter-state
	// bookkeeping is blocked on the instance-script model.
	// Lucifron (Molten Core, first boss) — fight logic in
	// lua_scripts/moltencore/boss_lucifron.lua. The UpdateAI casting
	// gates have no bridge (no UNIT_STATE model); the encounter-state
	// bookkeeping is blocked on the instance-script model.
	RegisterLuaBoss("boss_lucifron", 12118)
	// Magmadar (Molten Core, second boss) — fight logic in
	// lua_scripts/moltencore/boss_magmadar.lua. The UpdateAI casting
	// gates have no bridge (no UNIT_STATE model); the encounter-state
	// bookkeeping is blocked on the instance-script model.
	RegisterLuaBoss("boss_magmadar", 11982)
	// Gehennas (Molten Core, third boss) — fight logic in
	// lua_scripts/moltencore/boss_gehennas.lua. The UpdateAI casting
	// gates have no bridge (no UNIT_STATE model); the encounter-state
	// bookkeeping is blocked on the instance-script model.
	RegisterLuaBoss("boss_gehennas", 12259)
	// Garr (Molten Core, fourth boss) — fight logic in
	// lua_scripts/moltencore/boss_garr.lua. The UpdateAI casting
	// gates have no bridge (no UNIT_STATE model); the encounter-state
	// bookkeeping is blocked on the instance-script model; the
	// npc_firesworn AI in the same C++ file is not registered (its
	// creature entry is DB-side, unverifiable from the C++ tree).
	RegisterLuaBoss("boss_garr", 12057)
	// Shazzrah (Molten Core, fifth boss) — fight logic in
	// lua_scripts/moltencore/boss_shazzrah.lua. The UpdateAI casting
	// gates have no bridge (no UNIT_STATE model); the encounter-state
	// bookkeeping is blocked on the instance-script model; the Gate
	// threat-reset and teleport SpellScript arms have no bridges (no
	// threat model, no SpellScript model).
	RegisterLuaBoss("boss_shazzrah", 12264)
	// Baron Geddon (Molten Core, sixth boss) — fight logic in
	// lua_scripts/moltencore/boss_baron_geddon.lua. The UpdateAI casting
	// gates have no bridge (no UNIT_STATE model); the encounter-state
	// bookkeeping is blocked on the instance-script model; the
	// armageddon InterruptNonMeleeSpells arm is bridged (no-op twin) and ported; the
	// spell_baron_geddon_inferno AuraScript has no bridge (no AuraScript
	// model).
	RegisterLuaBoss("boss_baron_geddon", 12056)
	// RegisterLuaBoss wires lua_scripts/moltencore/boss_sulfuron_harbinger.lua.
	// The UpdateAI casting gates have no bridge (no UNIT_STATE model); the
	// encounter-state bookkeeping is blocked on the instance-script model; the
	// inspire random-friendly-missing-buff arm has no bridge (no
	// friendly-creature lookup) — only the self-cast is kept; the
	// npc_flamewaker_priest add AI is not registered (no entry constant in
	// the C++ tree, DB-side ScriptName binding).
	RegisterLuaBoss("boss_sulfuron", 12098)
	// RegisterLuaBoss wires lua_scripts/moltencore/boss_golemagg.lua.
	// The UpdateAI casting gates have no bridge (no UNIT_STATE model); the
	// encounter-state bookkeeping is blocked on the instance-script model; the
	// HasAura(SPELL_ENRAGE) once-guard is kept as per-GUID Lua state (no
	// HasAura bridge); the npc_core_rager add AI is not registered (no
	// entry constant in the C++ tree, DB-side ScriptName binding).
	RegisterLuaBoss("boss_golemagg", 11988)
	// RegisterLuaBoss wires lua_scripts/moltencore/boss_majordomo_executus.lua.
	// The UpdateAI casting gates have no bridge (no UNIT_STATE model); the
	// encounter-state bookkeeping is blocked on the instance-script model;
	// the defeat/outro arms (defeat trigger, faction change, teleport,
	// Ragnaros summon, gossip flags, DoAction) have no bridges — only the
	// combat timers, slay/aggro Talk lines, the teleport-victim exclusion,
	// the sub-50% triggered Aegis of Ragnaros, and the gossip-select Talk
	// arms are modeled.
	RegisterLuaBoss("boss_majordomo", 12018)
	// RegisterLuaBoss wires lua_scripts/moltencore/boss_ragnaros.lua.
	// The UpdateAI casting gates have no bridge (no UNIT_STATE model); the
	// encounter-state bookkeeping (DATA_RAGNAROS_ADDS) and the
	// intro/majordomo-kill arms are blocked on the instance-script model;
	// the intro, submerge and emerge react/emote/flag/faction/attack arms
	// have no bridges; the 8 sons of flame summon loop is PORTED
	// 2026-10-24 (SummonCreature(12143, target pos, type 1, 15min);
	// AttackStart relay unbridged, engine aggro instead) with the
	// Lua-side adds-death counter (event 21) driving the >8-adds
	// early-emerge arm; the magma blast IsWithinMeleeRange gate has no bridge;
	// the npc_son_of_flame add AI is not registered (no entry constant in
	// the C++ tree, DB-side ScriptName binding).
	RegisterLuaBoss("boss_ragnaros", 11502)
	// instance_molten_core.cpp audit (map 409, "MC"): InstanceMapScript only,
	// no creature/at_ scripts — and there is no Lua InstanceMapScript bridge,
	// so this unit is documented-only. C++ arms: OnCreatureCreate latches the
	// Golemagg (11988) and Majordomo Executus (12018) GUIDs; OnGameObjectCreate
	// latches the Cache of the Firelord GO (179703); DATA_RAGNAROS_ADDS death
	// counter (feeds the >8-adds early-emerge arm in boss_ragnaros.lua, but
	// no summon bridge means the 8 sons of flame never spawn); GetGuidData for
	// the two boss GUIDs; SetBossState spawns Majordomo + 4 healer (11663) +
	// 4 elite (11664) flamewakers once all 8 prior bosses are DONE (deferred
	// across map loads via ReadSaveDataMore), respawns the cache 7 days after
	// Majordomo dies, and re-summons Majordomo at the tele pos with
	// ACTION_START_RAGNAROS_ALT to trigger the intro if he is already DONE.
	// None of it is bridgeable without an instance-script model.
	// RegisterLuaBoss wires lua_scripts/blacktemple/boss_warlord_najentus.lua.
	// The UpdateAI casting gates have no bridge (no UNIT_STATE model); the
	// encounter-state bookkeeping is blocked on the instance-script model; the
	// impaling-spine GO summon (185584) IS ported (creature:SummonGameObject
	// bridged, target position via GetX/GetY/GetZ/GetO); the spine-removal
	// gossip arm stays out (the GO handler needs the instance-creature
	// lookup); the needle-spine targeting SpellScript filter has no
	// SpellScript bridge.
	RegisterLuaBoss("boss_najentus", 22887)
	// RegisterLuaBoss wires lua_scripts/blacktemple/boss_supremus.lua.
	// The UpdateAI casting gates have no bridge (no UNIT_STATE model); the
	// encounter-state bookkeeping is blocked on the instance-script model; the
	// ResetThreatList/AddThreat arms have no threat bridge (the hateful strike
	// target pick is approximated by the highest-health melee-range player);
	// the ApplySpellImmune taunt arms stay out on no-bearer grounds (the
	// immunity bridge itself exists: creature:ApplySpellImmune — see
	// boss_supremus.lua); the volcano and molten
	// flame add AIs are not registered (ScriptName bindings are DB-side); the
	// volcanic summon/volcano adds have no summon model.
	RegisterLuaBoss("boss_supremus", 22898)
	// RegisterLuaBoss wires lua_scripts/blacktemple/boss_shade_of_akama.lua.
	// The UpdateAI casting gates have no bridge (no UNIT_STATE model); the
	// encounter-state bookkeeping is blocked on the instance-script model
	// (DATA_SHADE_OF_AKAMA/DATA_AKAMA_SHADE lookups, SetBossState DONE);
	// the chase-complete phase-two gate has no MovementInform bridge, so the
	// threat 41602 arm and the phase-one immune/non-selectable/emotestate
	// arms have no bridge; the channeler/sorcerer/defender/rogue/
	// elementalist/spiritbinder/broken companion AIs are not registered
	// (ScriptName bindings are DB-side) and have no summon model; the shade
	// soul-channel AuraScripts have no AuraScript bridge.
	RegisterLuaBoss("boss_shade_of_akama", 22841)
	// RegisterLuaBoss wires lua_scripts/blacktemple/boss_teron_gorefiend.lua.
	// The UpdateAI casting gates have no bridge (no UNIT_STATE model); the
	// encounter-state bookkeeping is blocked on the instance-script model
	// (DATA_TERON_GOREFIEND/DATA_TERON_GOREFIEND_INTRO, SetBossState); the
	// intro arms (area trigger ACTION_START_INTRO, Talk(SAY_INTRO),
	// NON_ATTACKABLE/NOT_SELECTABLE/REACT_PASSIVE flags, 20s
	// EVENT_FINISH_INTRO) have no area-trigger or flag bridges — the fight
	// starts on pull; the doom blossom summon (23123), the shadow of death
	// summon arms and the EnterEvadeMode summons.DespawnAll/_DespawnAtEvade
	// arms have no summon/despawn model; the shadow of death remove
	// (41999) cast's effect lives in the unmodeled SpellScript; the doom
	// blossom and shadowy construct AIs are not registered (ScriptName
	// bindings are DB-side) and the construct's instance-creature lookup,
	// ResetThreatList/AddThreat and ApplySpellImmune arms stay out — the
	// immunity bridge exists (creature:ApplySpellImmune) but the recv is
	// cross-creature via the instance lookup, with no GUID-to-creature
	// bridge (see boss_teron_gorefiend.lua);
	// the shadow of death / spiritual vengeance AuraScripts have no
	// AuraScript bridge.
	RegisterLuaBoss("boss_teron_gorefiend", 22871)
	// RegisterLuaBoss wires lua_scripts/blacktemple/boss_gurtogg_bloodboil.lua.
	// The UpdateAI casting gates have no bridge (no UNIT_STATE model); the
	// encounter-state bookkeeping is blocked on the instance-script model
	// (DATA_GURTOGG_BLOODBOIL); the GetThreat/ModifyThreatByPercent/AddThreat
	// and AttackStart(oldTarget) victim-restore arms have no threat bridge
	// (the fel rage GUID is followed via per-GUID Lua state); the
	// ApplySpellImmune taunt/attack-me arms are PORTED (creature:ApplySpellImmune
	// bridge — see boss_gurtogg_bloodboil.lua) and the bewildering-strike
	// CanAIAttack gate has no bridge; the fel acid breath combat-reach
	// pick is approximated at 10 yd (no combat-reach bridge); the player
	// self-casts 40617/40603 have no player-side CastSpell bridge and are
	// applied via AddAura (their whole effect is the aura); the berserk and
	// death DoPlaySoundToSet arms are ported via creature:PlayDirectSound; the npc_fel_geyser
	// add AI is not registered (ScriptName bindings are DB-side, no entry
	// constant in the C++ tree) and its summons have no summon model; the
	// bloodboil/insignificance target-filter SpellScripts have no
	// SpellScript bridge; the EnterEvadeMode _DespawnAtEvade arm has no
	// despawn bridge (evade-side cleanup is engine-side).
	RegisterLuaBoss("boss_gurtogg_bloodboil", 22948)
	// Reliquary of Souls (Black Temple) — fight logic in
	// lua_scripts/blacktemple/boss_reliquary_of_souls.lua.
	// The reliquary models only the ACTION_START_COMBAT arm (10s
	// submerge visual + summon-essence cast, phase pinned at
	// suffering); the essence-death phase advances, the
	// HandleSpirits world-trigger scheduling and the enslaved-soul
	// DoAction(ACTION_KILL_SELF) propagation have no
	// instance/DoAction bridge, and the trigger's MoveInLineOfSight
	// start is modeled as pull; the three essence boss AIs are not
	// registered (no essence entry constants in the C++ tree —
	// ScriptName bindings are DB-side); the enslaved-soul
	// REACT/DoZoneInCombat arms and the 500ms KillSelf arm have no
	// bridges (no react/zone-combat/despawn model); the combat
	// trigger's sight arm has no bridge and it is unkillable
	// (damage rewritten to 0, C++-exact); the aura_of_desire,
	// submerge and spite AuraScripts and the frenzy SpellScript
	// have no script bridges.
	RegisterLuaBoss("boss_reliquary_of_souls", 22856)
	// Enslaved Soul (Black Temple) — fight logic in
	// lua_scripts/blacktemple/boss_reliquary_of_souls.lua.
	RegisterLuaBoss("npc_enslaved_soul", 23469)
	// Reliquary combat trigger (Black Temple) — invulnerability
	// hook in lua_scripts/blacktemple/boss_reliquary_of_souls.lua.
	RegisterLuaBoss("npc_reliquary_combat_trigger", 23417)
	// Mother Shahraz (Black Temple) — fight logic in
	// lua_scripts/blacktemple/boss_mother_shahraz.lua.
	// The enrage (10%, triggered 40867), taunt and fatal-attraction
	// arms are modeled; the encounter-state bookkeeping is blocked
	// on the instance-script model (DATA_MOTHER_SHAHRAZ); the
	// fatal-attraction SPELLVALUE_MAX_TARGETS=3 clamp and the
	// teleport target filter have no bridge (the 40869 target
	// filter SpellScript has no SpellScript bridge); the
	// 40863/40865/40866/40862 beam-trigger arms and the 40867
	// random-beam periodic arm have no AuraScript bridge; the
	// saber lash aura script (40816) has no bridge; the EnterEvadeMode
	// _DespawnAtEvade arm has no despawn bridge (evade-side cleanup
	// is engine-side).
	RegisterLuaBoss("boss_mother_shahraz", 22947)
	// Illidari Council (Black Temple) — fight logic in
	// lua_scripts/blacktemple/boss_illidari_council.lua.
	// The controller (23426) arms the equivalency cycle and the
	// 15min berserk one-shot; the member casts, the Talk arms on
	// other members and the JustDied quiet-suicide relay have no
	// cross-creature bridge; the SetBossState(DONE) and
	// SendEncounterUnit arms are blocked on the instance-script
	// model. Gathios (22949) runs the bless/consecration/hammer/
	// judgement/aura cycle — the bless friendly-target scan has
	// no friendly enumeration bridge, so the cast is skipped and
	// only the cycle kept; the hammer picks a random alive player
	// 10-40 yd excluding the victim (C++-exact selector).
	// Zerevor (22950) runs flamestrike/blizzard/arcane-explosion
	// with the 5s gate and a one-time dampen magic cast — the
	// recurring dampen arm has no bearer (its AuraScript DoAction
	// has no bridge), and the arcane-bolt filler has no
	// spell-attack bridge. Malande (22951) runs circle of
	// healing/reflective shield/divine wrath — the HealReceived
	// shared-rule arm has no heal hook bridge and the empowered-
	// smite filler has no spell-attack bridge. Veras (22952) runs
	// the vanish+deadly-strike cycle — the CanSeeAlways arm has no
	// bridge. All members get the lethal-damage rewrite
	// (health-1, C++-exact) and Talk(SAY_COUNCIL_SLAY/DEATH).
	// The npc_veras_vanish_effect AI is not registered (no entry
	// constant in the C++ tree — ScriptName binds DB-side); the
	// ten spell/aura scripts have no script bridges.
	RegisterLuaBoss("boss_illidari_council", 23426)
	RegisterLuaBoss("boss_gathios_the_shatterer", 22949)
	RegisterLuaBoss("boss_high_nethermancer_zerevor", 22950)
	RegisterLuaBoss("boss_lady_malande", 22951)
	RegisterLuaBoss("boss_veras_darkshadow", 22952)
	// Illidan Stormrage (Black Temple) — fight logic in
	// lua_scripts/blacktemple/boss_illidan.lua.
	// Illidan (22917) runs the phase-1 schedule, the 25min berserk
	// and the taunt cycle on pull (the intro arms have no
	// gossip/instance bridges); the health-based transitions arm
	// the minions weave (90% Talk + 30s weave timer ported; the weave
	// fires empty — the illidari-elite summon stays out: summon-side
	// AttackStart/AddThreat on Akama via instance GUID + 23226
	// unregistered, no bearer), the
	// air-phase timer chain (65%, positions unmodeled — no movement
	// bridge; the both-flames-dead finalize arm has no bearer, so
	// the pillar loop repeats), the demon-form cycle (60s one-shot
	// in the phase-3/4 schedule, 72s cancel, 15s demon spells) and
	// the phase-4 shadow-prison/maiev sequence (30%); lethal damage
	// is rewritten to health-1 with the demon-cancel or the death
	// outro arm (C++-exact). Akama (23089) runs the healing-potion
	// loop and the lethal-damage rewrite; the intro/minions/outro
	// chains have no gossip/movement/DoAction bridges. Flame of
	// Azzinoth (22997) runs the engage/charge/flame-blast cycle —
	// the JustDied ACTION_FLAME_DEAD relay to illidan has no
	// cross-creature bridge. Maiev (23197) runs cage-trap/shadow-
	// strike/throw-dagger/taunt with the down-arm (health-1 +
	// 40409, once-guard never reset — no AuraScript bridge); the
	// appear/outro chains have no summon/DoAction bridges. Blade of
	// Azzinoth (22996), the db target (23070) and the generic fire
	// entries (23069/23259/23336) get their Reset self-casts. The
	// parasitic shadowfiend (23498) and illidari elite (23226) AIs
	// have no modelable arm without engine bridges; the shadow
	// demon and cage-trap-trigger AIs have no entry constants in
	// the C++ tree (ScriptName binds DB-side); the twenty-two
	// spell/aura scripts have no script bridges.
	RegisterLuaBoss("boss_illidan_stormrage", 22917)
	RegisterLuaBoss("npc_akama_illidan", 23089)
	RegisterLuaBoss("npc_flame_of_azzinoth", 22997)
	RegisterLuaBoss("npc_illidan_db_target", 23070)
	RegisterLuaBoss("npc_maiev", 23197)
	RegisterLuaBoss("npc_blade_of_azzinoth", 22996)
	RegisterLuaBoss("npc_illidan_generic_fire", 23069)
	RegisterLuaBoss("npc_illidan_generic_fire", 23259)
	RegisterLuaBoss("npc_illidan_generic_fire", 23336)
	// Kalecgos (Sunwell Plateau) — fight logic in
	// lua_scripts/sunwellplateau/boss_kalecgos.lua.
	// Kalecgos (24850) runs the six-arm pull schedule (arcane buffet
	// with the 20% Talk, frost breath, tail lash, wild magic, spectral
	// blast), the 1s check timer with the enrage and banish arms, the
	// lethal-damage-to-0 rewrite and the 50% kill Talk; the human
	// summon, the sathrovarr banish cross-check and the ACTION_START_
	// OUTRO relay have no cross-creature/summon bridges, and the outro
	// movement chain has no movement/faction bridge. Kalecgos human
	// (24891) runs revitalize/heroic strike with the 75%/50%/10% say-
	// phase Talks and the death Talk; the sathrovarr-only damage-source
	// arm has no instance-creature bridge. Sathrovarr (24892) runs the
	// shadowbolt/corruption-strike Talk+schedule arms, the spectral-
	// realm-aura-filtered agony curse pick, the enrage/banish check
	// timer, the lethal-damage-to-0 rewrite, the kill Talk and the
	// tap-check SpellHit arm (kill-caster skipped — no kill bridge);
	// its kalecgos enrage/banish/outro relays have no cross-creature
	// bridge. The spectral-rift GO script and the five
	// spell/aura scripts have no script bridges.
	RegisterLuaBoss("boss_kalecgos", 24850)
	RegisterLuaBoss("boss_kalecgos_human", 24891)
	RegisterLuaBoss("boss_sathrovarr", 24892)
	// Brutallus (Sunwell Plateau) — fight logic in
	// lua_scripts/sunwellplateau/boss_brutallus.lua.
	// Brutallus (24882) runs the pull Talk, the meteor-slash/stomp
	// (stomp Talk)/burn/berserk schedule, the unguarded kill Talk,
	// the death Talk and the Reset dual-wield arm; the burn target
	// self-cast is modeled as AddAura (no player-side CastSpell
	// bridge); the Felmyst summon and SetBossState arms have no
	// summon/instance-script bridges, and the Madrigosa intro
	// chain has no instance-creature/cross-creature/flag/movement
	// bridges.
	RegisterLuaBoss("boss_brutallus", 24882)
	// Felmyst (Sunwell Plateau) — fight logic in
	// lua_scripts/sunwellplateau/boss_felmyst.lua.
	// Felmyst (25038) runs the pull auras, the
	// cleave/corrosion/gas-nova/encapsulate ground-phase schedule,
	// the berserk chain (10min, then every 10s, C++-exact), the
	// unguarded kill Talk, the death Talk and timer cleanup; the
	// flight phase, DamageTaken invulnerability, FOG_INFORM SpellHit
	// summon, vapor-trail despawn summon and JustAppeared Talk arms
	// have no movement/summon/kill/spawn bridges, and the
	// SetBossState arms are blocked on the instance-script model.
	RegisterLuaBoss("boss_felmyst", 25038)
	// Vapor trail (25267) runs its constructor triggered self-cast
	// of trail trigger 45399; the flag/combat-reach arms have no
	// bridges. The vapor (25265) has no bridgeable AI arms (flag/
	// speed/target-selection only) and the Blazing Dead (25268)
	// has no C++ AI class, so neither is registered yet — entries
	// verified in sunwell_plateau.h for future registration.
	RegisterLuaBoss("npc_felmyst_trail", 25267)
	// Eredar Twins (25165/25166) run their pull schedules (shadow
	// blades/shadow nova/confounding blow for Sacrolash; conflagration/
	// flame sear/pyrogenics/blaze for Alythess), the 6min enrage
	// one-shots, the 25% kill Talks and timer cleanup. The sister-death
	// detection (cross-creature instance lookup), the SpellHitTarget
	// touched-spell chaining, the shadow-image summons and the
	// SetBossState arms have no cross-creature/SpellHitTarget/summon/
	// instance-script bridges.
	RegisterLuaBoss("boss_sacrolash", 25165)
	RegisterLuaBoss("boss_alythess", 25166)
	// Shadow Image (25214) runs its constructor triggered self-cast of
	// image visual 45263 plus the shadow-fury/dark-strike schedule; the
	// NOT_SELECTABLE flag and 15s KillSelf arms have no flag/kill
	// bridges, and its sacrolash-side DoSpawnCreature summon is
	// summon-blocked — registered for its own verifiable AI arms
	// (felmyst-trail convention).
	RegisterLuaBoss("npc_shadow_image", 25214)
	// Muru (25741) / Entropius (25840): Muru runs the engage
	// triggered periodic self-casts (open portal 45994, darkness
	// 45998, negative energy 46009), the 10s one-shot blood-elves
	// summon script/periodic casts, the 10min non-triggered enrage,
	// and the lethal-damage rewrite (health-1, PHASE_TWO transition,
	// triggered 46177 open-all-portals, 6s-delayed triggered 46217
	// summon-entropius). Entropius runs the reset triggered
	// cosmetic-spawn 46223, the 2s one-shot triggered negative-
	// energy 46284 and the 15s-repeat triggered darkness 46269 +
	// blackhole 46282 schedule. The muru RemoveAllAuras/NOT_
	// SELECTABLE flag arms, the cross-creature enrage/evade/death
	// relays, the entropius DoResetPortals grid scan and
	// JustSummoned dark-fiend/darkness arms, and the SetBossState
	// arms have no aura/flag/cross-creature/creature-enumeration/
	// summon/kill/instance-script bridges.
	RegisterLuaBoss("boss_muru", 25741)
	RegisterLuaBoss("boss_entropius", 25840)
	// Muru Portal Target (25770) runs its SpellHit arms (event 14):
	// 46177 open-all-portals -> triggered 45977 open-portal +
	// triggered 46205 transform-visual-missile; 45976 open-portal-2
	// -> triggered 45977 + 6s-delayed triggered 45978 summon-void-
	// sentinel-summoner. The void-spawn summon event and the actual
	// creature summons are summon-blocked — registered for its own
	// verifiable AI arms (felmyst-trail convention).
	RegisterLuaBoss("npc_muru_portal", 25770)
	// Dark Fiend (25744) runs its constructor triggered self-cast
	// of darkfiend skin 45934; the 2s/3s react/target/proximity
	// scheduler arms and the dispel OnRemove arm have no react/
	// flag/cross-creature/movement/kill/despawn/AuraScript bridges
	// — registered for its own verifiable AI arm (felmyst-trail
	// convention).
	RegisterLuaBoss("npc_dark_fiend", 25744)
	// Void Sentinel (25772) runs the engage triggered shadow-pulse-
	// periodic 46086 self-cast plus the 45s-repeat non-triggered
	// void-blast 46161 on the victim; its death 6x void-spawn
	// summon arm is summon-blocked.
	RegisterLuaBoss("npc_void_sentinel", 25772)
	// Darkness / Black Hole (25879) runs its Reset visual chain:
	// non-triggered 46242 self-cast, then 1s -> 46247, 1.2s ->
	// 46242, 2s -> 46228 + 46235, then the chain ends. The 15s
	// DisappearAndDie and the REACT/AttackStart arms have no
	// despawn/react/instance-player/movement bridges.
	RegisterLuaBoss("npc_blackhole", 25879)
	// Kil'jaeden (25315) runs the C++ Timer[10] scheduler as a
	// 100ms Lua pump: soul flay 45442+47106, legion lightning
	// 45664, fire bloom 45641, shadow spike 46680 (+30s wait),
	// flame dart 45737, darkness 46605/45657 (+9s wait), with the
	// C++-exact 85/55/25% phase transitions and the shield-orb /
	// orbs-empower timer dynamics (summons and the kalecgos
	// cross-creature arms skipped). The speech chain's kalecgos/
	// anveena rows, the SetBossState/DoZoneInCombat arms and the
	// armageddon-target summons have no cross-creature/instance-
	// script/summon bridges.
	RegisterLuaBoss("boss_kiljaeden", 25315)
	// Hand of the Deceiver (25588) runs its shadow-bolt-volley
	// 45770 schedule, the 20% shadow-infusion 45772 upkeep and the
	// out-of-combat shadow-channeling 46757 upkeep. The felfire-
	// portal summon, the controller death-count relay and the
	// SetBossState arms have no summon/cross-creature/instance-
	// script bridges — registered for its own verifiable AI arms
	// (felmyst-trail convention).
	RegisterLuaBoss("npc_hand_of_the_deceiver", 25588)
	// Volatile Felfire Fiend (25598) casts felfire fission 45779
	// (triggered) on lethal damage and runs the 2s-primed 3yd
	// proximity detonation; the KillSelf and AddThreat arms have
	// no kill/threat bridges.
	RegisterLuaBoss("npc_volatile_felfire_fiend", 25598)
	// Armageddon Target (25735) runs its Reset visual chain:
	// triggered 45911, 9s -> triggered 45914, 5s -> triggered
	// 45909. The DespawnOrUnsummon arm has no despawn bridge.
	RegisterLuaBoss("npc_armageddon", 25735)
	// Sinister Reflection (25708) runs the C++-exact per-victim-
	// class spell rotations (druid/hunter/mage/warlock/warrior/
	// paladin/priest/shaman/rogue). The summon-side setup and the
	// SetCanDualWield arms have no summon/dual-wield bridges.
	// Note the C++ ScriptName typo "npc_sinster_reflection" is
	// preserved 1:1.
	RegisterLuaBoss("npc_sinster_reflection", 25708)

	// lua_scripts/gruulslair/boss_high_king_maulgar.lua (boss_high_
	// king_maulgar + the four ogre councillors, first boss of
	// Gruul's Lair; gruuls_lair.h DATA_MAULGAR = 0).
	// High King Maulgar (18831) runs the C++-exact phase-1 timer
	// set (arcing smash / whirlwind / mighty blow) and the <50%
	// phase-2 machine: Talk(SAY_ENRAGE) + triggered self-cast
	// dual wield 29651 + the charging and roar arms (initial 0s).
	// The virtual-item-slot zeroing arms ride
	// creature:SetEquipmentSlots (0, 0, EQUIP_NO_CHANGE skip);
	// the charging AttackStart victim-switch arm has no
	// bridge; the instance/SetBossState/DoZoneInCombat arms are
	// blocked on the instance-script model.
	RegisterLuaBoss("boss_high_king_maulgar", 18831)
	// Krosh Firehand (18832) runs the fireball 2s floor + the
	// within-30-yd (3D) distance poller (IsWithinDist bridged),
	// the spell-shield arm and blast wave 60s (random alive
	// player within 15 yd 2D — IsWithinDist2d bridged; only the
	// threat-list half of the pick has no bridge; the
	// InterruptNonMeleeSpells arms are bridged (no-op twin) and ported).
	RegisterLuaBoss("boss_krosh_firehand", 18832)
	// Olm the Summoner (18834) runs dark decay / summon WFH
	// 33131 / death coil. The AttackStart MoveChase/threat arm has
	// no movement/threat bridges; the WFH summon effect is
	// engine-side (no AI class in the C++ file).
	RegisterLuaBoss("boss_olm_the_summoner", 18834)
	// Kiggler the Crazed (18835) runs the C++-exact
	// DoCastVictim arms: greater polymorph / lightning bolt /
	// arcane shock / arcane explosion.
	RegisterLuaBoss("boss_kiggler_the_crazed", 18835)
	// Blindeye the Seer (18836) runs the three self-cast healing
	// arms: greater PW shield / heal / prayer of healing.
	RegisterLuaBoss("boss_blindeye_the_seer", 18836)
	// Gruul the Dragonkiller (19044) runs the growth / cave-in /
	// ground-slam+shatter / hurtful-strike / reverberation arms;
	// the shatter performing-clear rides bridged event 15 (caster-
	// side bindings); the ground-slam knockback arms are unmodeled
	// (no hit-enumeration bridge for the AoE hit players).
	RegisterLuaBoss("boss_gruul", 19044)
	// Magtheridon (17257) runs the break-free / cleave / blast
	// nova / blaze / quake / berserk arms plus the 30% collapse
	// chain; the channeler-driven phase machine is cross-creature
	// blocked so the fight starts on pull. Hellfire Channeler
	// (17256) runs shadow-bolt-volley / fear / abyssal; its
	// magtheridon DoAction relay is unmodeled. Magtheridon Room
	// (17516) runs the debris visual/damage chain. Fight logic in
	// lua_scripts/hellfirecitadel/boss_magtheridon.lua.
	RegisterLuaBoss("boss_magtheridon", 17257)
	RegisterLuaBoss("npc_hellfire_channeler", 17256)
	RegisterLuaBoss("npc_magtheridon_room", 17516)
	// Fathom-Lord Karathress (21214) runs cataclysmic bolt /
	// sear nova / enrage / blessing of the tides; the three
	// Fathom-Guards run their C++ timers: Sharkkis (21966)
	// leeching throw / multishot / beast within, Tidalvess
	// (21965) windfury + frost shock / spitfire / poison
	// cleansing / earthbind totems, Caribdis (21964) water bolt
	// volley / tidal surge / heal. Instance/summon/cross-creature
	// arms are unmodeled. Fight logic in
	// lua_scripts/serpentshrinecavern/boss_fathomlord_karathress.lua.
	RegisterLuaBoss("boss_fathomlord_karathress", 21214)
	RegisterLuaBoss("boss_fathomguard_sharkkis", 21966)
	RegisterLuaBoss("boss_fathomguard_tidalvess", 21965)
	RegisterLuaBoss("boss_fathomguard_caribdis", 21964)

	// Hydross the Unstable (21216) — dual-form (clean/corrupted)
	// switch on 2.5s position poll vs (-239.439,-363.481)/18yd,
	// escalating mark stacks (15s), water tomb 7s / vile sludge
	// 7s->15s, enrage 10min->60s. Display/immunity/threat/summon
	// arms are unmodeled. Fight logic in
	// lua_scripts/serpentshrinecavern/boss_hydross_the_unstable.lua.
	RegisterLuaBoss("boss_hydross_the_unstable", 21216)

	// Lady Vashj (21212) — shock blast / static charge / entangle
	// + shoot-or-multishot melee-range arm in phase 1, Talk on the
	// once-guarded <70% phase-2 switch, teleport + shield-channel
	// summons + forked lightning + enchanted/tainted/Coilfang
	// summons in phase 2, intro Talk + NON_ATTACKABLE flag arms.
	// The DATA_CANSTARTPHASE3 poll, phase 3, the shield-barrier
	// upkeep and the enchanted waypoint machine are unmodeled.
	// Tainted Elemental (22009) runs poison bolt {5s,10s} + 30s
	// despawn + the JustDied 50s-clamp relay. Fight logic in
	// lua_scripts/serpentshrinecavern/boss_lady_vashj.lua.
	RegisterLuaBoss("boss_lady_vashj", 21212)
	RegisterLuaBoss("npc_tainted_elemental", 22009)

	// Leotheras the Blind (21215) — whirlwind / chaos blast /
	// nightelf-demon form switching / berserk / 15% final form.
	// The banish pre-phase, CheckBanish poll, channeler summons,
	// inner-demon summons, demon-copy summon, display/item/threat/
	// movement arms are unmodeled. Demon Form (21875) runs chaos
	// blast with the range-gated re-arm; Greyheart Spellbinder
	// (21806) runs mind blast ({3s,8s} then {10s,15s}) — the
	// earthshock caster scan has no bridge. Inner Demon (21857)
	// runs soul link 1s / demonic alignment upkeep / shadow bolt
	// 10s — the victim binding and damage immunity are unmodeled.
	// Fight logic in
	// lua_scripts/serpentshrinecavern/boss_leotheras_the_blind.lua.
	RegisterLuaBoss("boss_leotheras_the_blind", 21215)
	RegisterLuaBoss("boss_leotheras_the_blind_demonform", 21875)
	RegisterLuaBoss("npc_greyheart_spellbinder", 21806)
	RegisterLuaBoss("npc_inner_demon", 21857)

	// Morogrim Tidewalker (21213) — tidal wave 10s/20s,
	// earthquake 40s/10s two-stage (Talk + Talk(EMOTE) on the
	// summon stage), watery grave 30s in phase 1 (talks only),
	// watery globules 0/25s in phase 2 (Talk(EMOTE) only) on the
	// once-guarded <25% switch. The murloc (21920) summons, the
	// waterfall teleport, the watery-grave/globule target picks
	// and the player-side triggered casts are unmodeled. Water
	// Globule (21913) runs the 5-yd check -> explosion 37871 +
	// despawn; the LOS aggro machine and flag/faction arms are
	// unmodeled. Fight logic in
	// lua_scripts/serpentshrinecavern/boss_morogrim_tidewalker.lua.
	RegisterLuaBoss("boss_morogrim_tidewalker", 21213)
	RegisterLuaBoss("npc_water_globule", 21913)

	// The Lurker Below (21217): spout (45s) -> Talk(EMOTE_SPOUT)
	// + 20s rotation; whirl (18s, 20s post-spout); geyser
	// ({15s,20s}, non-victim pick w/ victim fallback, triggered
	// 37478); waterbolt (triggered 37138 when no player in melee
	// range); 120s -> submerge (non-triggered 37550), 60s ->
	// emerge (triggered 20568) + spout in 3s. The fishing/GO
	// pre-phase, the 9x ambusher/guardian summons, the rotating
	// arc-spout machine, the LOS aggro and the instance-side
	// encounter bookkeeping are unmodeled (no
	// instance/GO/summon/movement/LOS/arc/evade bridges). The
	// Coilfang Ambusher (21865): multishot (triggered 37790 on
	// victim) + shoot bow (triggered 37770 on a random player;
	// the 1100 BP0 arg has no bridge), 1500ms GCD arms. Fight
	// logic in
	// lua_scripts/serpentshrinecavern/boss_lurker_below.lua.
	RegisterLuaBoss("boss_the_lurker_below", 21217)
	RegisterLuaBoss("npc_coilfang_ambusher", 21865)
	// Hydromancer Thespia + Coilfang Water Elemental (The Steamvault):
	// lightning cloud (random player <=30yd, 15s then 15-25s; the
	// Heroic second cast has no difficulty bridge), lung burst
	// (random player <=40yd, 7s then 7-12s), enveloping winds
	// (random player <=35yd, 9s then 10-15s; Heroic second cast
	// unmodeled); the elemental self-casts water bolt volley
	// (34449), 3-6s init then 7-12s. Fight logic in
	// lua_scripts/steamvault/boss_hydromancer_thespia.lua.
	RegisterLuaBoss("boss_hydromancer_thespia", 17797)
	RegisterLuaBoss("npc_coilfang_waterelemental", 17917)
	// Mekgineer Steamrigger (The Steamvault): super shrink ray
	// (victim, 20s), saw blade (random non-victim player with
	// victim fallback, 15s), electrified net (victim, 10s),
	// SAY_MECHANICS at the 75/50/25 thresholds (the mechanic
	// summons are summon-blocked); the npc_steamrigger_mechanic
	// AI is entirely instance-gated and stays unregistered.
	// Fight logic in
	// lua_scripts/steamvault/boss_mekgineer_steamrigger.lua.
	RegisterLuaBoss("boss_mekgineer_steamrigger", 17796)
	// Warlord Kalithresh (The Steamvault): spell reflection
	// (self, 10s then 15-25s), impale (random player, nil pick
	// casts nothing, 7-14s init then 7.5-12.5s); the rage arm
	// is FindNearestCreature(17954)+cross-creature gated and
	// kept as timer bookkeeping only; the npc_naga_distiller
	// AI is entirely flag/cross-creature/instance gated and
	// stays unregistered. Fight logic in
	// lua_scripts/steamvault/boss_warlord_kalithresh.lua.
	RegisterLuaBoss("boss_warlord_kalithresh", 17798)
	// Mennu the Betrayer (The Slave Pens): tainted stoneskin
	// totem (self, 30s, HealthBelowPct(100) gate), tainted
	// earthgrab totem (self, 20s one-shot), corrupted nova
	// totem (self, 60s one-shot), Mennu's healing ward (self,
	// 14-25s), lightning bolt (triggered DoCastVictim, 14-19s
	// init then 14-25s); the _Reset/_JustDied instance arms
	// stay blocked on the instance-script model. Fight logic
	// in lua_scripts/theslavepens/boss_mennu_the_betrayer.lua.
	RegisterLuaBoss("boss_mennu_the_betrayer", 17941)
	// Rokmar the Crackler (The Slave Pens): grievous wound
	// (triggered DoCastVictim, 10s then 20-30s), ensnaring
	// moss (DoCastAOE self-cast, 20s then 20-30s), water spit
	// (DoCastAOE self-cast, 14s then 14-18s), frenzy self-cast
	// once-guarded on HealthBelowPct(10); no Talk lines and no
	// KilledUnit body in C++, so no event 3; the _Reset/
	// _JustDied instance arms stay blocked on the
	// instance-script model. Fight logic in
	// lua_scripts/theslavepens/boss_rokmar_the_crackler.lua.
	RegisterLuaBoss("boss_rokmar_the_crackler", 17991)
	// Quagmirran (The Slave Pens): acid spray (DoCastAOE
	// self-cast, 25s then 20-25s), cleave (triggered
	// DoCastVictim, 9s then 18-34s), uppercut (random alive
	// player in 10 yd excluding the victim, 20s then 22s),
	// poison bolt volley (DoCast(me), 31s then 24s); no Talk
	// lines and no KilledUnit body in C++, so no event 3; the
	// _Reset/_JustDied instance arms stay blocked on the
	// instance-script model. Fight logic in
	// lua_scripts/theslavepens/boss_quagmirran.lua.
	RegisterLuaBoss("boss_quagmirran", 17942)
	// Ahune (The Slave Pens, Midsummer event): the 4ms
	// initial-emerge self casts (stand, spanky hands, shield,
	// fired on engage) plus the 3s synch-health timer kept as
	// bookkeeping only (cross-creature gated, no bridge); the
	// whole retreat/emerge phase machine is cross-creature/
	// summon/GO driven and unreachable, documented only.
	// Frozen core (25865): the ctor Initialize self casts via
	// the engine-fired OnSpawn event, and the JustDied self
	// casts (the cross-creature Kill arm stays blocked). The
	// bunny/flamecaller/ice-stone/SpellScript-AuraScript arms
	// stay unregistered. Fight logic in
	// lua_scripts/theslavepens/boss_ahune.lua and
	// lua_scripts/theslavepens/npc_frozen_core.lua.
	RegisterLuaBoss("boss_ahune", 25740)
	RegisterLuaBoss("npc_frozen_core", 25865)
	// Hungarfen (The Underbog): foul-spores once-guard at <=20%
	// health, the acid-geyser target casts, and the mushroom-
	// summon arm PORTED 2026-10-22 (SummonCreature(17990, target
	// pos + offsets, type 2, 22s)).
	// Underbog mushroom (17990): the Reset triggered self casts
	// via the engine-fired OnSpawn event, and the grow/shrink
	// timer machine (the RemoveAurasDueToSpell shrink arm has no
	// aura-removal bridge). Fight logic in
	// lua_scripts/theunderbog/boss_hungarfen.lua and
	// lua_scripts/theunderbog/npc_underbog_mushroom.lua.
	RegisterLuaBoss("boss_hungarfen", 17770)
	RegisterLuaBoss("npc_underbog_mushroom", 17990)

	// boss_the_black_stalker (17882): levitate (the followup
	// LevitatedTarget/LevitatedTarget_Timer/InAir machine needs
	// an ObjectAccessor/cross-creature bridge, bookkeeping only),
	// chain lightning, and static charge target casts; the
	// heroic-only spore-strider summon and the 60-yd evade arm
	// have no difficulty/home-position bridges (bookkeeping only).
	// Fight logic in lua_scripts/theunderbog/boss_the_black_
	// stalker.lua.
	RegisterLuaBoss("boss_the_black_stalker", 17882)

	// boss_broggok (17380): slime spray and poison bolt victim
	// casts plus the poison cloud self-cast; the C++ event
	// schedule fires only from the unbridgeable ACTION_ACTIVATE_
	// BROGGOK DoAction (lever gossip -> prisoner waves ->
	// ActivateCell), so the activate schedule (10s/7s/5s) is
	// applied at OnEnterCombat, the only engagement hook this
	// model has. The whole activate/prepare/reset DoAction
	// machine (flag/react/immune flips, DoZoneInCombat, summon
	// arms) and the instance lever/GO choreography have no
	// instance/GO/summon/flag/react/zone bridges (bookkeeping
	// only). The prisoner adds (17398 nascent fel orc, 17429
	// fel orc neophyte) cast concussion blow/stomp and
	// charge/frenzy on their own ScheduleEvents schedules.
	// Fight logic in lua_scripts/hellfirecitadel/boss_broggok.
	// lua, lua_scripts/hellfirecitadel/npc_nascent_fel_orc.lua,
	// and lua_scripts/hellfirecitadel/npc_fel_orc_neophyte.lua.
	RegisterLuaBoss("boss_broggok", 17380)
	RegisterLuaBoss("npc_nascent_fel_orc", 17398)
	RegisterLuaBoss("npc_fel_orc_neophyte", 17429)

	// boss_kelidan_the_breaker (17377): shadow bolt volley and
	// corruption self-casts plus the burning-nova -> fire-nova
	// machine (the C++ Firenova early return is C++-exact: nothing
	// else fires in the 5s window); Talk on pull/nova/death and a
	// C++-exact 50% kill line. The whole channeler summon/
	// activation machine (SummonChannelers, ChannelerEngaged/
	// ChannelerDied/GetChanneled), the Reset passive/
	// non-attackable/immune arms and the pre-combat evocation arm
	// have no summon/cross-creature/flag/react/immune/unit-state
	// bridges; heroic teleport, heroic spell ids and the
	// burning-nova AddAura have no difficulty/teleport/aura
	// bridges (timer bookkeeping exact). The add
	// npc_shadowmoon_channeler (17653) casts mark of shadow on a
	// random player and shadow bolt on the victim; its pre-combat
	// GetChanneled channeling machine and its Kelidan relays are
	// cross-creature gated (bookkeeping only).
	// Fight logic in lua_scripts/hellfirecitadel/boss_kelidan_the_
	// breaker.lua and lua_scripts/hellfirecitadel/npc_shadowmoon_
	// channeler.lua.
	RegisterLuaBoss("boss_kelidan_the_breaker", 17377)
	RegisterLuaBoss("npc_shadowmoon_channeler", 17653)

	// boss_the_maker (17381): acid spray and knockdown victim
	// casts plus exploding breaker on a random player within 30
	// yd and domination on a random player (nil picks cast
	// nothing; re-arms fire regardless — C++-exact); Talk on
	// pull (SAY_AGGRO), a player-type-gated kill line
	// (SAY_KILL, 3-arg handler convention) and death (SAY_DIE).
	// The BossAI ctor DATA_THE_MAKER bookkeeping has no
	// instance-script bridge (timer bookkeeping exact).
	// Fight logic in lua_scripts/hellfirecitadel/boss_the_
	// maker.lua.
	RegisterLuaBoss("boss_the_maker", 17381)

	// boss_grand_warlock_nethekurse (16807): death coil and
	// shadow fissure on random players (nil picks cast
	// nothing; re-arms fire regardless — C++-exact), the
	// below-20%-health flip to the dark-spin + shadow-cleave
	// phase (cleave timer decrements only in the phase-2
	// branch — C++-exact bookkeeping); Talk on pull
	// (SAY_AGGRO, 4), an ungated kill line (SAY_SLAY, 5) and
	// death (SAY_DIE, 6). The intro/peon SetData machine
	// (SAY_INTRO/PEON_ATTACKED/PEON_DIES/TAUNT) has no
	// instance/cross-creature/flag bridges — the Initialize()
	// schedule lands on OnEnterCombat (broggok precedent);
	// heroic 30741/35953 unmodeled (thespia precedent).
	// npc_fel_orc_convert (17083): hemorrhage on a 3s/15s
	// EventMap rhythm; the SetData peon relays have no
	// cross-creature bridge.
	// npc_lesser_shadow_fissure documented only (all four AI
	// overrides empty — ahune bunny empty-skeleton
	// precedent).
	// Fight logic in lua_scripts/shatteredhalls/boss_nethe
	// kurse.lua and lua_scripts/shatteredhalls/npc_fel_orc_
	// convert.lua.
	RegisterLuaBoss("boss_grand_warlock_nethekurse", 16807)
	RegisterLuaBoss("npc_fel_orc_convert", 17083)

	// Fight logic in
	// lua_scripts/shatteredhalls/boss_warbringer_omrogg.lua.
	RegisterLuaBoss("boss_warbringer_omrogg", 16809)

	// Fight logic in
	// lua_scripts/shatteredhalls/boss_warchief_kargath_
	// bladefist.lua.
	RegisterLuaBoss("boss_warchief_kargath_bladefist", 16808)

	// boss_shattered_executioner (17301, NPC_SHATTERED_
	// EXECUTIONER): cleave-only BossAI; the zone-file
	// area-trigger and the two script hooks stay
	// unregistered (no area-trigger/AuraScript/SpellScript
	// bridges); the loot-mode/immune/quest/SetData arms
	// have no bridges — see lua_scripts/shatteredhalls/
	// boss_shattered_executioner.lua.
	RegisterLuaBoss("boss_shattered_executioner", 17301)

	// boss_watchkeeper_gargolmar (17306): mortal wound /
	// surge / retaliation / heal-yell boss; the
	// MoveInLineOfSight SAY_TAUNT arm has no LoS-aggro
	// bridge — see lua_scripts/hellfireramparts/
	// boss_watchkeeper_gargolmar.lua.
	RegisterLuaBoss("boss_watchkeeper_gargolmar", 17306)

	// boss_omor_the_unscarred (17308): orbital strike /
	// shadow whip / treacherous aura / demonic shield /
	// fiendish-hound summon boss; the summon, whip-pullback
	// and movement-flag arms have no bridges — see
	// lua_scripts/hellfireramparts/
	// boss_omor_the_unscarred.lua.
	RegisterLuaBoss("boss_omor_the_unscarred", 17308)

	// boss_vazruden_the_herald (17307): the herald's whole
	// phase machine is movement/summon-blocked — only the
	// SAY_INTRO engage arm is registered; the adds (Nazan
	// 17536, Vazruden 17537) and the hellfire sentry (17517)
	// carry the combat arms — see lua_scripts/
	// hellfireramparts/boss_vazruden_the_herald.lua.
	RegisterLuaBoss("boss_vazruden_the_herald", 17307)

	// boss_nazan (17536): fireball / cone-of-fire boss; the
	// flight waypoints, gravity/walk arms, early-land
	// condition, heroic bellowing roar and liquid-fire
	// summon have no bridges — see lua_scripts/
	// hellfireramparts/boss_nazan.lua.
	RegisterLuaBoss("boss_nazan", 17536)

	// boss_vazruden (17537): revenge / wipe-yell boss; the
	// heroic revenge variant and the DisappearAndDie arm
	// have no bridges — see lua_scripts/hellfireramparts/
	// boss_vazruden.lua.
	RegisterLuaBoss("boss_vazruden", 17537)

	// npc_hellfire_sentry (17517): kidney-shot trash; the
	// JustDied -> herald SentryDownBy relay has no
	// cross-creature bridge — see lua_scripts/
	// hellfireramparts/npc_hellfire_sentry.lua.
	RegisterLuaBoss("npc_hellfire_sentry", 17517)

	// npc_millhouse_manastorm (20977): the Arcatraz zone
	// script; the pre-combat intro machine runs on a
	// spawn-started pump and the combat arms on an
	// enter-combat pump — see lua_scripts/arcatraz/
	// npc_millhouse_manastorm.lua. npc_warden_mellichar
	// (20904) is documented there but not registered: its
	// whole event machine is instance/summon driven.
	RegisterLuaBoss("npc_millhouse_manastorm", 20977)

	// boss_zereketh_the_unbound (20870): void zone / shadow
	// nova / seed of corruption boss; the UNIT_STATE_CASTING
	// early-return gates have no cast-state bridge — see
	// lua_scripts/arcatraz/boss_zereketh_the_unbound.lua.
	RegisterLuaBoss("boss_zereketh_the_unbound", 20870)

	// boss_dalliah_the_doomsayer (20885): gift of the
	// doomsayer / whirlwind / heal boss; the Soccothrates
	// cross-creature arms and the out-of-combat
	// EVENT_SOCCOTHRATES_DEATH machine have no
	// cross-creature / data-set bridges — see lua_scripts/
	// arcatraz/boss_dalliah_the_doomsayer.lua.
	RegisterLuaBoss("boss_dalliah_the_doomsayer", 20885)

	// boss_wrath_scryer_soccothrates (20886): felfire shock /
	// knock away boss; the MoveInLineOfSight prefight
	// machine, the Dalliah cross-creature arms (ME_FIRST,
	// the 25% taunt, the JustDied SetData relay) and the
	// SetData / EVENT_DALLIAH_DEATH machine have no
	// LoS / instance-data / cross-creature / data-set
	// bridges — see lua_scripts/arcatraz/
	// boss_wrath_scryer_soccothrates.lua.
	RegisterLuaBoss("boss_wrath_scryer_soccothrates", 20886)

	// boss_harbinger_skyriss (20912): intro machine / image
	// splits / mind rend / fear / domination boss; the
	// wardens-shield / mellichar-kill intro arms, the DoSplit
	// illusion summons (spell-cast summons — type/duration live
	// in spell data, unverifiable — plus the JustSummoned
	// AttackStart relay has no bridge and the illusion AI
	// (21466/21467) is unregistered), the heroic mana-burn arm
	// is PORTED (IsHeroic bridged) and the cast-state gates have
	// no instance / cross-creature / cast-state bridges — see
	// lua_scripts/arcatraz/boss_harbinger_skyriss.lua. Arcatraz
	// boss roster 4/4 COMPLETE.
	RegisterLuaBoss("boss_harbinger_skyriss", 20912)

	// The Botanica (Tempest Keep) boss roster 1/5:
	// boss_high_botanist_freywinn (17975): seedling / tree-form
	// boss; the summon-frayer arms, the tree-form aura /
	// movement / cast-state arms have no summon / aura /
	// movement / cast-state bridges — see lua_scripts/
	// botanica/boss_high_botanist_freywinn.lua. instance_the_
	// botanica.cpp stays blocked on the instance-script model.
	RegisterLuaBoss("boss_high_botanist_freywinn", 17975)

	// The Botanica (Tempest Keep) boss roster 2/5:
	// boss_laj (17980): teleport / allergic-reaction boss; the
	// TRIGGERED summon lasher/flayer casts ride spell data (the
	// summoned entry IDs live in the spell effects, unverifiable)
	// and the JustSummoned AttackStart relay has no bridge —
	// unmodeled; the DoTransform display/immune arms are PORTED
	// (2026-10-20) — see lua_scripts/botanica/boss_laj.lua.
	// instance_the_botanica.cpp stays blocked on the
	// instance-script model.
	RegisterLuaBoss("boss_laj", 17980)

	// The Botanica (Tempest Keep) boss roster 3/5:
	// boss_warp_splinter (17977): war-stomp / arcane-volley /
	// summon-treants boss; the six SummonTreants summon casts
	// have no summon bridge, the treant (19949) heal/follow AI
	// has no movement / cross-creature / cast-bp0 / suicide
	// bridges and is documented only — see lua_scripts/
	// botanica/boss_warp_splinter.lua. instance_the_
	// botanica.cpp stays blocked on the instance-script model.
	RegisterLuaBoss("boss_warp_splinter", 17977)

	// The Botanica (Tempest Keep):
	// boss_thorngrin_the_tender (17978): sacrifice / hellfire /
	// enrage boss with 50%/20% HP talk latches; the heroic
	// hellfire schedule and the UNIT_STATE_CASTING gates have
	// no difficulty / cast-state bridges — see lua_scripts/
	// botanica/boss_thorngrin_the_tender.lua. instance_the_
	// botanica.cpp stays blocked on the instance-script model.
	RegisterLuaBoss("boss_thorngrin_the_tender", 17978)

	// The Botanica (Tempest Keep) boss roster 5/5 COMPLETE:
	// boss_commander_sarannis (17976): arcane resonance /
	// arcane devastation boss with a 50% HP summon-talk latch;
	// the SPELL_SUMMON_REINFORCEMENTS summons have no summon
	// bridge and the spell_commander_sarannis_summon_
	// reinforcements SpellScript has no SpellScript bridge —
	// see lua_scripts/botanica/boss_commander_sarannis.lua.
	// instance_the_botanica.cpp stays blocked on the
	// instance-script model.
	RegisterLuaBoss("boss_commander_sarannis", 17976)

	// The Eye (Tempest Keep) — first boss in the set per
	// outland_script_loader.cpp order: boss_alar (19514):
	// two-phase phoenix — phase-1 platform cycle with a 20%
	// flame-quills branch, a lethal-hit DamageTaken latch into
	// the 5s WE_DIE -> WE_REVIVE phase-2 sequence, and phase-2
	// charge / melt armor / dive-bomb chain / berserk arms;
	// the platform/dive MovePoints, ember and flame-patch
	// summons, and the flame-quills AuraScript have no movement
	// / summon / AuraScript bridges — see lua_scripts/the_eye/
	// boss_alar.lua. instance_the_eye.cpp stays blocked on the
	// instance-script model.
	RegisterLuaBoss("boss_alar", 19514)
	// boss_kaelthas (19622): only the two standalone Talk arms
	// are bridgeable — KilledUnit SAY_SLAY and JustDied
	// SAY_DEATH; the whole phase machine (DoAction chains,
	// 7-weapon / phoenix summons, phase-5 transition MovePoints,
	// gravity lapse SpellScript) has no instance / summon /
	// movement / SpellScript bridges — see
	// lua_scripts/the_eye/boss_kaelthas.lua. The four advisor
	// scripts port the bridgeable arms: boss_thaladred_the_
	// darkener (20064) — SAY_THALADRED_AGGRO + silence / rend /
	// psychic-blow pump (gaze arm: no threat / targeted-Talk
	// bridges); boss_lord_sanguinar (20060) — SAY_SANGUINAR_
	// AGGRO + bellowing-roar pump; boss_grand_astromancer_
	// capernian (20062) — SAY_CAPERNIAN_AGGRO + fireball /
	// conflagration pump (arcane-explosion melee-range scan and
	// MoveChase AttackStart: no threat-list / movement
	// bridges); boss_master_engineer_telonicus (20063) —
	// SAY_TELONICUS_AGGRO + bomb / remote-toy pump. The
	// advisor fake-death / SPELL_RESSURECTION resurrection
	// machine and the kael DoAction hooks have no display /
	// stand / flag / cross-creature / instance bridges (death
	// talks never fire in this model); npc_kael_flamestrike,
	// npc_phoenix_tk, npc_phoenix_egg_tk sit behind the
	// unmodeled summon machines; spell_kael_gravity_lapse has
	// no SpellScript bridge.
	RegisterLuaBoss("boss_kaelthas", 19622)
	RegisterLuaBoss("boss_thaladred_the_darkener", 20064)
	RegisterLuaBoss("boss_lord_sanguinar", 20060)
	RegisterLuaBoss("boss_grand_astromancer_capernian", 20062)
	RegisterLuaBoss("boss_master_engineer_telonicus", 20063)

	// boss_void_reaver (19516): pounding / arcane-orb / knock-
	// away / 10-minute berserk boss; the arcane-orb threat-list
	// leg picks over alive players in the instance (no threat-
	// list bridge), the knock-away 25% threat drop has no
	// ModifyThreatByPercent bridge, the cast-state gates have
	// no cast-state bridge, and the BossAI DATA_VOID_REAVER =
	// 3 ctor / reset / death arms are instance-blocked — see
	// lua_scripts/the_eye/boss_void_reaver.lua.
	RegisterLuaBoss("boss_void_reaver", 19516)

	// npc_crystalcore_devastator (20040 — wowhead-verified; the
	// C++ carries no NPC_ constant): knock-away 22893 TRIGGERED
	// DoCastVictim 25s/23s with the C++ GetVictim() nil guard
	// (the ResetThreat victim leg has no single-target threat
	// bridge) + countercharge 35035 non-triggered DoCast(me)
	// self-cast 9s/45s; no Talk lines in the C++; cast-state
	// gates have no cast-state bridge — see lua_scripts/the_eye/
	// npc_crystalcore_devastator.lua.
	RegisterLuaBoss("npc_crystalcore_devastator", 20040)

	// boss_high_astromancer_solarian (18805): the bridgeable
	// Phase 1 arms — the blinding-light latch, the two wrath
	// timers, the arcane-missiles arm — plus the 20% health
	// phase-4 latch into fear / void-bolt arms; the Phase 2/3
	// portal machine (teleports, spotlight/agent/priest
	// summons, visibility/flag arms), the wrath AuraScript,
	// and the npc_solarium_priest AI have no
	// movement/summon/visibility/display/AuraScript/cross-
	// creature bridges — see lua_scripts/the_eye/
	// boss_high_astromancer_solarian.lua.
	RegisterLuaBoss("boss_high_astromancer_solarian", 18805)

	// boss_gatewatcher_iron_hand (19710 — wowhead-verified;
	// mechanar.h carries no NPC_ constant; DATA_GATEWATCHER_
	// IRON_HAND = 1 is instance-side): stream of machine fluid
	// 35311 TRIGGERED DoCastVictim 55s/randtime(35s,50s),
	// jackhammer 35326 TRIGGERED DoCastVictim 45s/30s with
	// EMOTE_HAMMER + 50% SAY_HAMMER talks, shadow power 35322
	// non-triggered DoCast(me) self-cast 25s/randtime(20s,28s)
	// (the H_SPELL_* heroic variants are never fired by the
	// C++ UpdateAI — not IsHeroic-gated), KilledUnit 50%
	// SAY_SLAY, JustDied SAY_DEATH; cast-state gates have no
	// cast-state bridge — see lua_scripts/mechanar/
	// boss_gatewatcher_iron_hand.lua. First unit of the
	// Mechanar block per outland_script_loader.cpp order;
	// instance_mechanar.cpp stays blocked on the
	// instance-script model.
	RegisterLuaBoss("boss_gatewatcher_iron_hand", 19710)
	// boss_gatewatcher_gyrokill (19218; no NPC_ constant in the
	// C++ tree — DB-side ScriptName binding, entry per standard
	// TBC creature data; DATA_GATEWATCHER_GYROKILL = 0 is
	// instance-side): stream of machine fluid 35311 TRIGGERED
	// DoCastVictim 10s/randtime(13s,17s), saw blade 35318
	// non-triggered DoCast(me) self-cast 20s/randtime(20s,30s)
	// with SAY_SAW_BLADEs (the H_SPELL_* heroic variants are
	// never fired by the C++ UpdateAI — not IsHeroic-gated),
	// shadow power 35322 non-triggered DoCast(me) self-cast
	// 25s one-shot — C++-exact quirk: the EVENT_SHADOW_POWER
	// arm schedules EVENT_SAW_BLADE at randtime(25s,35s)
	// instead of re-arming itself (modeled as a second pending
	// saw-blade countdown); KilledUnit SAY_SLAY, JustDied
	// SAY_DEATH; cast-state gates have no cast-state bridge —
	// see lua_scripts/mechanar/boss_gatewatcher_gyrokill.lua.
	RegisterLuaBoss("boss_gatewatcher_gyrokill", 19218)
	// boss_nethermancer_sepethrea (17271; DB-side ScriptName
	// binding, no NPC_ constant — entry per standard TBC
	// creature data; DATA_NETHERMANCER_SEPRETHREA = 3 is
	// instance-side): summon ragin flames 35275 non-triggered
	// DoCastVictim + SAY_SUMMON on engage, frost attack 35263
	// TRIGGERED DoCastVictim randtime(7s,10s) init and re-arm,
	// arcane blast 35314 TRIGGERED DoCastVictim
	// randtime(12s,18s)/15s, dragon's breath 35250 TRIGGERED
	// DoCastVictim randtime(18s,22s)/randtime(12s,22s) with
	// 50% SAY_DRAGONS_BREATH, knockback 37317 TRIGGERED
	// DoCastVictim randtime(22s,28s)/randtime(15s,25s),
	// solarburn 35267 TRIGGERED DoCastVictim 30s/30s;
	// KilledUnit SAY_SLAY, JustDied SAY_DEATH; cast-state
	// gates have no cast-state bridge — see
	// lua_scripts/mechanar/boss_nethermancer_sepethrea.lua.
	RegisterLuaBoss("boss_nethermancer_sepethrea", 17271)
	// npc_ragin_flames (22323; DB-side ScriptName binding, no
	// NPC_ constant — entry per standard TBC creature data):
	// the bridgeable UpdateAI arms — inferno 35268
	// non-triggered DoCastVictim 10s/10s and fire tail 35278
	// non-triggered DoCast(me) self-cast 500ms/500ms. The
	// Reset() ApplySpellImmune legs are PORTED (creature:ApplySpellImmune
	// bridge — see boss_nethermancer_sepethrea.lua); the SetSpeedRate leg,
	// the Check_Timer instance-boss-state despawn check, and the
	// onlyonce MoveChase(random-target) leg have no bridges —
	// see lua_scripts/mechanar/boss_nethermancer_sepethrea.lua.
	RegisterLuaBoss("npc_ragin_flames", 22323)
	// boss_pathaleon_the_calculator (19220; DB-side ScriptName
	// binding, no NPC_ constant — entry per standard TBC
	// creature data; DATA_PATHALEON_THE_CALCULATOR = 4 is
	// instance-side): summon timer 30s/randtime(30s,45s) with
	// Talk(SAY_SUMMON) after the loop — the 3x SelectTarget-gated
	// SummonCreature(21062, boss pos, type 3, 25s) legs PORTED
	// 2026-10-24 (the AttackStart relay has no bridge; wraiths
	// engage via engine aggro), mana tap 36021
	// TRIGGERED DoCastVictim randtime(12s,20s)/
	// randtime(14s,22s), arcane torrent 36022 TRIGGERED
	// DoCastVictim randtime(16s,25s)/randtime(12s,18s),
	// domination 35280 TRIGGERED DoCastVictim
	// randtime(25s,40s)/randtime(25s,30s) with SAY_DOMINATION
	// before the cast, arcane explosion 15453 TRIGGERED
	// DoCastVictim randtime(8s,13s)/randtime(10s,14s) fired
	// always (the "Heroic only" comment has no IsHeroic gate
	// in the code — C++-exact), frenzy 36992 on the
	// HealthBelowPctDamaged(20,damage) && !HasAura latch via
	// OnDamageTaken(9) + Talk(SAY_ENRAGE); KilledUnit
	// SAY_SLAY, JustDied SAY_DEATH; cast-state gates have no
	// cast-state bridge — see lua_scripts/mechanar/
	// boss_pathaleon_the_calculator.lua.
	RegisterLuaBoss("boss_pathaleon_the_calculator", 19220)
	// npc_nether_wraith (21062, from the C++ Creatures enum):
	// arcane missiles 35034 DoCast(target) on a random alive
	// player (the C++ SelectTarget(Random,1) skips top threat
	// — Lua picks any alive player, gruul/vashj precedent)
	// else DoCastVictim, urand(1s,4s)/urand(5s,10s),
	// detonation 35058 non-triggered DoCast(me) self-cast 20s
	// one-shot, then DespawnOrUnsummon after 2200ms via
	// creature:Despawn() — see lua_scripts/mechanar/
	// boss_pathaleon_the_calculator.lua.
	RegisterLuaBoss("npc_nether_wraith", 21062)
	// boss_mechano_lord_capacitus (19219; DB-side ScriptName
	// binding, no NPC_ constant — entry per standard TBC
	// creature data; DATA_MECHANOLORD_CAPACITUS = 2 is
	// instance-side): head crack 35161 non-triggered
	// DoCastVictim 10s/10s, reflective damage shield 35159
	// Talk(2) + non-triggered DoCast(me) 15s init and
	// reflective magic shield 35158 Talk(1) + non-triggered
	// DoCast(me) as a C++-exact ping-pong — each shield arm
	// schedules the other at 30s (exactly one shield event
	// pending), polarity shift 39096 non-triggered DoCastAOE
	// self-cast randtime(45s,60s) on the C++ IsHeroic()
	// engage gate (creature:IsHeroic() is bridged),
	// summon nether charge RAND of 35153/35904/35905/35906
	// non-triggered DoCastSelf with DUNGEON_MODE(randtime
	// (9s,11s), randtime(2s,5s)) evaluated at fire time,
	// berserk 26662 non-triggered DoCast(me) 3min one-shot
	// (the C++ arm does not re-arm); KilledUnit
	// SAY_KILL, JustDied SAY_DEATH; cast-state gates have no
	// cast-state bridge; the two polarity SpellScripts have
	// no SpellScript bridge — see lua_scripts/mechanar/
	// boss_mechano_lord_capacitus.lua.
	RegisterLuaBoss("boss_mechano_lord_capacitus", 19219)
	// instance_mechanar (map 554): door data 184632->
	// DATA_GATEWATCHER_IRON_HAND(1) and 184322->
	// DATA_GATEWATCHER_GYROKILL(0) as passage doors, 184449->
	// DATA_NETHERMANCER_SEPRETHREA(3) as a room door; the
	// SetBossState override is a no-op switch — documented
	// only, no-instance-script-bridge queue; see
	// hidden_files/CHECKPOINT.md.
	// zone_blades_edge_mountains residuals (npc_nether_drake
	// already ported at lua_scripts/outland/npc_nether_drake.lua
	// per the earlier Outland zone-set work): go_legion_
	// obelisk's quest-10821 spawn arm is quest-gated (no
	// player:GetQuestStatus bridge); npc_simon_bunny's Simon game
	// is unreachable — the GO-gossip -> bunny SetGUID/SetData
	// dispatch and player:SummonCreature have no bridges (the
	// summon / teleport / sound / world-search bridges it also
	// needs all exist now — the missing legs are the GO-gossip
	// entry-point dispatch, player:SummonCreature and
	// SetObjectScale); go_simon_
	// cluster / go_apexis_relic's gossip summon arms need the
	// npc_oscillating_frequency_scanner_master_bunny's
	// Reset summon machine is bridgeable and its
	// GetClosestCreatureWithEntry leg is bridged now
	// (creature:FindNearestCreature); entry 21760 sourced from
	// Wowhead TBC Classic (no C++ entry constant — DB-side ScriptName
	// binding) and the Reset arm is ported in lua_scripts/outland/
	// npc_oscillating_frequency_scanner_master_bunny.lua; its
	// IsSummonedBy player-capture has no bridge (ev22 is
	// pet-funnel-only) and spell_oscillating_field is a SpellScript
	// — both documented only; see hidden_files/CHECKPOINT.md.
	RegisterLuaBoss("npc_oscillating_frequency_scanner_master_bunny", 21760)
	// boss_doomlord_kazzak (18728) and boss_doomwalker (17711):
	// the C++ files carry no NPC_ entry constants anywhere in
	// the sources; both entries verified via the 2010
	// TrinityCore changelog paste listing each entry ID
	// (void_reaver precedent) — registered per loader order.
	RegisterLuaBoss("boss_doomlord_kazzak", 18728)
	RegisterLuaBoss("boss_doomwalker", 17711)
	// npc_nether_drake (Blade's Edge Mountains zone script,
	// first unit after the world-boss set per
	// outland_script_loader.cpp order — AddSC_blades_edge_
	// mountains precedes AddSC_boss_doomlordkazzak): the
	// bridgeable combat arms — intangible presence /
	// mana burn / arcane blast timers — over the five
	// ENTRY_* drake entries (20021 whelp, 21821 proto,
	// 21817 adolescent, 21820 mature, 21823 nihil), all
	// verifiable from the C++ sources; the SpellHit phase-
	// modulator transform has no entry-update/flag/evade/
	// movement bridges, so the IsNihil speech machine never
	// runs — see lua_scripts/outland/npc_nether_drake.lua.
	RegisterLuaBoss("npc_nether_drake", 20021)
	RegisterLuaBoss("npc_nether_drake", 21821)
	RegisterLuaBoss("npc_nether_drake", 21817)
	RegisterLuaBoss("npc_nether_drake", 21820)
	RegisterLuaBoss("npc_nether_drake", 21823)
	// npc_nagrand_banner (Nagrand zone script, third in the
	// Outland zone set per outland_script_loader.cpp order):
	// the five combat variants of the single CreatureScript
	// (the C++ GetAI switches on creature entry) — 17146 Kil
	// Sorrow spellbinder (arcane missiles + chains of ice),
	// 17147 Kil Sorrow cultist (mind sear), 17148 Kil Sorrow
	// deathsworn (50% bloodthirst latch), 18391 Giselda the
	// crone (65% transform latch), 18064 Warmaul shaman
	// (scorching totem + frost shock + 50% healing-wave
	// latch) — all entries verifiable from the C++ sources;
	// see lua_scripts/outland/npc_nagrand_banner.lua. The
	// spellbinder's victim-cast interrupt and 15% flee latches,
	// the base AI's SpellHit bannered latch (17138 Warmaul
	// reaver — no-op model), the maghar/kurenai captive
	// EscortAIs and condition_nagrand_banner have no
	// cast-state / flee / SpellHit / escort / quest-accept /
	// summon / movement / ConditionScript bridges —
	// documented only.
	RegisterLuaBoss("npc_nagrand_banner", 17146)
	RegisterLuaBoss("npc_nagrand_banner", 17147)
	RegisterLuaBoss("npc_nagrand_banner", 17148)
	RegisterLuaBoss("npc_nagrand_banner", 18391)
	RegisterLuaBoss("npc_nagrand_banner", 18064)
	// npc_phase_hunter (Netherstorm zone script, fourth in the
	// Outland zone set per outland_script_loader.cpp order):
	// one CreatureScript AI class serving both entries — 18879
	// phase hunter (materialize one-shot + mana burn on random
	// mana-holder) and 19595 drained phase hunter (same AI —
	// the C++ GetAI has no per-entry switch); both entries
	// verifiable from the C++ sources — see lua_scripts/
	// outland/npc_phase_hunter.lua. The phase-slip aura/root
	// arm, the quest-10190 Weak emote latch and the Drained
	// entry-transform arm have no aura-state / entry-update /
	// quest bridges — documented only; npc_commander_
	// dawnforge + at_commander_dawnforge are documented-only
	// (summon / movement / stand-state / AreaTrigger /
	// quest-credit bridges missing).
	RegisterLuaBoss("npc_phase_hunter", 18879)
	RegisterLuaBoss("npc_phase_hunter", 19595)
	// npc_illidari_spawn (Shadowmoon Valley zone script): one
	// CreatureScript AI class serving three entries with a
	// per-entry cast switch in UpdateAI — 22075 Illidari Soldier
	// (spellbreaker), 22074 Illidari Mind Breaker (focused bursts
	// on a random player + psychic scream + mind blast), 19797
	// Illidari Highlord (curse of flames + flamestrike); all
	// entries verifiable from the C++ sources (UpdateAI
	// GetEntry() branches + the wave-spawn table) — see
	// lua_scripts/outland/npc_illidari_spawn.lua. The JustDied
	// DespawnOrUnsummon and the LordIllidan LiveCounter() call
	// have no despawn / cross-creature bridges — documented
	// only; the Torloth cinematic machine, the Illidan event
	// controller, the infernal summon pair, the mature/enslaved
	// drake quest arms and the wilda escort all sit behind
	// no-summon / movement / SpellHit / quest / escort bridges.
	RegisterLuaBoss("npc_illidari_spawn", 22075)
	RegisterLuaBoss("npc_illidari_spawn", 22074)
	RegisterLuaBoss("npc_illidari_spawn", 19797)
	// npc_enraged_spirit (Shadowmoon Valley zone script): one
	// CreatureScript AI class serving four entries with a
	// per-entry timer switch in JustEngagedWith/UpdateAI —
	// 21050 earth (fiery boulder), 21061 fire (fel fireball),
	// 21060 air (chain lightning / hurricane alternation),
	// 21059 water (stormbolt); all entries verifiable from the
	// C++ sources (the Enraged_Dpirits enum) — see
	// lua_scripts/outland/npc_enraged_spirit.lua. The JustDied
	// soul-spawn arm is PORTED 2026-10-24 (always-spawn, entry
	// per spirit, +1z, type 0, 5s) with the totem FindNearestCreature
	// + SetFaction(FRIENDLY) legs; the totem MovePoint /
	// KilledMonsterCredit / DoCast(SOUL_CAPTURED) legs have no
	// movement / quest-credit / dead-caster-cast bridges —
	// documented only;
	// spell_unlocking_zuluheds_chains (SpellScript) and
	// npc_shadowmoon_tuber_node (unverifiable entry, SetData /
	// SpellHit arms) are documented-only.
	RegisterLuaBoss("npc_enraged_spirit", 21050)
	RegisterLuaBoss("npc_enraged_spirit", 21061)
	RegisterLuaBoss("npc_enraged_spirit", 21060)
	RegisterLuaBoss("npc_enraged_spirit", 21059)
	// npc_depth_charge (Azshara zone script): proximity-trap
	// arms only — MoveInLineOfSight on a player within 5yd ->
	// non-triggered DoCast SPELL_DEPTH_CHARGE_TRAP 38576, then
	// the 1s WeMustDieTimer despawn (C++-exact once-latch on
	// the despawn; the event-27 veto suppresses the default
	// aggro engage, matching the empty AttackStart/JustEngagedWith
	// overrides); entry 23025 verifiable from the C++ enum —
	// see lua_scripts/kalimdor/npc_depth_charge.lua. The Reset
	// SetHover/SetSwim/NOT_SELECTABLE arms are ported in
	// npc_depth_charge.lua onReset (bridged); the summoning leg
	// (SPELL_PERIODIC_DEPTH_CHARGE) and npc_rizzle_sprysprocket
	// (escort / quest-status / MotionMaster machine) are
	// documented-only, though rizzle gossip itself is ported
	// (entry 23002, World/go_scripts.cpp:268).
	RegisterLuaBoss("npc_depth_charge", 23025)
	// npc_engineer_spark_overgrind (Azuremyst Isle zone script): Reset
	// restores UNIT_NPC_FLAGS via per-GUID first-seen capture +
	// SetNPCFlags (C++-exact, 7fe143eb); entry 17243 = NPC_SPARK
	// (zone_azuremyst_isle.cpp:463) — see
	// lua_scripts/kalimdor/npc_engineer_spark_overgrind.lua. The
	// SetFaction leg awaits the faction bridge — documented only.
	RegisterLuaBoss("npc_engineer_spark_overgrind", 17243)
	// npc_sironas (Bloodmyst Isle zone script): combat rotation
	// only — uppercut (10966, 15s init then 10-12s), immolate
	// (12742, 10s init then 15-20s), curse of blood (8282, 5s
	// init then 20-25s), all non-triggered DoCastVictim
	// (C++-exact); entry 17678 verifiable from the C++
	// EndingTheirWorldMisc enum — see
	// lua_scripts/kalimdor/npc_sironas.lua. The JustDied
	// cross-creature legoso DoAction cascade and the
	// sironas-channel DoAction arms have no cross-creature /
	// DoAction bridges; the Reset display-id arm and the
	// JustDied scale reset have no display / scale bridges —
	// documented only; npc_webbed_creature (no-summon /
	// quest-credit bridges, unverifiable entry) and
	// npc_demolitionist_legoso (no-escort / quest-accept /
	// summon / movement / gameobject bridges) are
	// documented-only.
	RegisterLuaBoss("npc_sironas", 17678)
	// npc_demolitionist_legoso (Bloodmyst Isle zone script):
	// combat rotation only — frost shock (8056, 1s init then
	// 10-15s, DelayEvents(1s)), searing totem (38116, 15s init
	// then 110-130s, DelayEvents(1s)), strength of earth totem
	// (31633, 20s init then 110-130s, DelayEvents(1s)),
	// healing surge (8004, 5s init, strict self-health <85% ->
	// 10s re-arm else 2s re-arm, no DelayEvents), all
	// non-triggered (C++-exact); entry 17982 verifiable from
	// the C++ EndingTheirWorldMisc enum — see
	// lua_scripts/kalimdor/npc_demolitionist_legoso.lua. The
	// 40-phase escort event machine (quest 9759 start,
	// waypoint kneel/plant/detonate choreography,
	// draenei-explosives gameobjects, sironas-meeting
	// phases, post-slay quest credit) and the escort-player
	// healing-surge arm await the escort / quest-accept /
	// summon / movement / gameobject bridges — documented
	// only; the Reset SetCanDualWield arm has no dual-wield
	// bridge; no SAY_LEGOSO_* line fires (no talk bridge);
	// ACTION_LEGOSO_SIRONAS_KILLED has no DoAction bridge.
	RegisterLuaBoss("npc_demolitionist_legoso", 17982)
	// npc_aged_dying_ancient_kodo (Desolace zone script):
	// SpellHit arm only — hit by SPELL_KODO_KOMBO_ITEM 18153
	// -> if entry is 4700/4701/4702 and neither the caster has
	// 18172 nor the kodo has 18377: triggered self-cast 18172
	// on the caster (player-side CastSpell bridge, brutallus
	// precedent) + triggered self-cast 18377 on the kodo
	// (C++-exact); entries 4700/4701/4702 verifiable from the
	// C++ DyingKodo enum — see
	// lua_scripts/kalimdor/npc_aged_dying_ancient_kodo.lua.
	// The UpdateEntry/CombatStop/faction/movement/follow/
	// flag mutation half of the 18153 arm has no entry-update
	// / faction / movement / flag bridges; the 18362 gossip-
	// aura arm awaits flag / movement / despawn bridges; the
	// Smeed-proximity MoveInLineOfSight latch awaits a
	// proximity-detection bridge; OnGossipHello awaits the
	// gossip bridge — documented only.
	RegisterLuaBoss("npc_aged_dying_ancient_kodo", 4700)
	RegisterLuaBoss("npc_aged_dying_ancient_kodo", 4701)
	RegisterLuaBoss("npc_aged_dying_ancient_kodo", 4702)
	// npc_omen (Moonglade zone script): combat rotation only —
	// cleave (15284, 3-5s init then 8-10s) + starfall (26540,
	// 8-10s init then 14-16s, random target degraded to victim —
	// no target-selection bridge), all non-triggered (C++-exact);
	// JustDied non-triggered self-cast of summon-spotlight
	// (26392); SpellHit by Elune's Candle (26374) reschedules the
	// starfall timer to 14-16s (the aura-strip half has no
	// creature-aura bridge); entry 15467 verifiable from the C++
	// Omen enum — see lua_scripts/kalimdor/npc_omen.lua. The
	// constructor's SetImmuneToPC/MovePoint + MovementInform
	// point-1 immunity drop await the immune / movement /
	// world-search bridges; npc_clintar_spirit (quest-10965
	// escort machine) awaits the escort / quest / talk / summon
	// bridges; npc_giant_spotlight (5-min despawn sweep) ported —
	// see lua_scripts/kalimdor/npc_giant_spotlight.lua.
	RegisterLuaBoss("npc_omen", 15467)
	RegisterLuaBoss("npc_giant_spotlight", 15902)
	// npc_thrall_warchief (Orgrimmar zone script): combat rotation
	// only — chain lightning (16033, 2s init then 9s) + shock
	// (16034, 8s init then 15s), both non-triggered DoCastVictim
	// (C++-exact); JustEngagedWith PlayDirectSound (5880); entry
	// 4949 wowhead-verified — see
	// lua_scripts/kalimdor/npc_thrall_warchief.lua. The quest-6566
	// gossip menu chain awaits the gossip bridge; npc_shenthul
	// (Shattered Salute emote machine), npc_overlord_runthak_orgrimmar
	// (citizen scene) and npc_thrall_herald_of_war's scene machine
	// (DoAction/MovementInform/MovePath legs) await the emote / quest /
	// movement / DoAction bridges — documented only; the herald's
	// Reset once-per-lifetime Kor'kron guard summons are ported in
	// lua_scripts/kalimdor/npc_thrall_herald_of_war.lua.
	RegisterLuaBoss("npc_thrall_warchief", 4949)
	// Thrall, Herald of War (Wrathgate cinematic) — Reset guard
	// summons in lua_scripts/kalimdor/npc_thrall_herald_of_war.lua.
	RegisterLuaBoss("npc_thrall_herald_of_war", 31412)
	// npc_cairne_bloodhoof (Thunder Bluff zone script): combat
	// rotation only — berserker charge (16636, 30s init then 25s,
	// random-target via the GetAITargets alive pool, omen idiom) +
	// uppercut (22916, 10s then 20s) + thunderclap (23931, 15s then
	// 15s) + mortal strike (16856, 10s then 15s) + cleave (16044,
	// 5s then 7s), all non-triggered DoCastVictim (C++-exact);
	// JustEngagedWith PlayDirectSound (5884); entry 3057
	// wowhead-verified — see
	// lua_scripts/kalimdor/npc_cairne_bloodhoof.lua. The quest-925
	// hoofprint gossip menu awaits the gossip bridge — documented
	// only.
	RegisterLuaBoss("npc_cairne_bloodhoof", 3057)
	// boss_azuregos (Azshara world boss): combat rotation only —
	// mark-of-frost aura 23184 triggered self-cast on engage; mana storm
	// 21097 (5-17s then 7.5-12.5s, random alive player within 60yd of the
	// threat list, C++ SelectTarget playerOnly verified); chill 21098
	// (10-30s then 13-25s) + frost breath 21099 (2-8s then 10-15s) +
	// cleave 8255 (7s then 7s) victim-cast; reflect 22067 self-cast
	// (15-30s then 20-35s); teleport (30s then 30s, SAY_TELEPORT 0 +
	// TeleportNear of threat-list players to boss pos +3z, ResetThreatList
	// unbridged); EVENT_MARK_OF_FROST is a C++ dead event (scheduled, no
	// switch case); KilledUnit mark of frost 23182 on player victims
	// (caster deviates — no player CastSpell bridge); enrage 23537
	// one-shot latch below 26%% via 1s pump; entry 6109 — see
	// lua_scripts/kalimdor/boss_azuregos.lua. The spell_mark_of_frost
	// SpellScript awaits the SpellScript bridge — documented only.
	RegisterLuaBoss("boss_azuregos", 6109)
	// boss_twilight_corrupter (Duskwood zone script): combat
	// rotation only — soul corruption (25805, 15s init then
	// 15-19s; DoCastAOE = DoCast(nullptr, spell) self-cast,
	// C++-exact) + creature of nightmare (25806, 30s init then
	// 45s, random target degraded to victim — no target-selection
	// bridge), all non-triggered (C++-exact); JustEngagedWith
	// Talk(YELL_TWILIGHT_CORRUPTOR_AGGRO 1); KilledUnit
	// kill-counter (victim:GetTypeId() == 4, C++-exact):
	// Talk(YELL_TWILIGHT_CORRUPTOR_KILL 2) per player kill, self-
	// cast level-up (24312) at 3 kills then reset (the triggered
	// flag has no distinct bridge); entry 15625 verifiable from
	// the C++ TwilightCorrupter enum — see
	// lua_scripts/eastern_kingdoms/boss_twilight_corrupter.lua.
	// The UNIT_STATE_CASTING pump-skip has no state bridge; the
	// kill-yell's victim targeting has no bridge; at_twilight_
	// grove (quest-8735 area-trigger summon) awaits the area-
	// trigger / quest-status / world-search / summon bridges —
	// documented only.
	RegisterLuaBoss("boss_twilight_corrupter", 15625)
	// npc_marzon_silent_blade (Stormwind City zone script): aggro
	// yell only — Talk(SAY_MARZON_2 1) on OnEnterCombat (C++-
	// exact); the summoner-AttackStart gate (IsSummon /
	// GetSummonerUnit) awaits the summon bridge; EnterEvadeMode's
	// self-DisappearAndDie pair is ported via event-2 Despawn()
	// (npc_marzon_silent_blade.lua); Reset's
	// RestoreFaction awaits the faction bridge; MovementInform's
	// cross-AI escort-phase latch awaits the movement bridge;
	// entry 1755 verifiable from the C++ LordGregorLescovar enum
	// — see lua_scripts/eastern_kingdoms/npc_marzon_silent_
	// blade.lua. npc_tyrion (quest-434 OnQuestAccept escort start
	// on the 8856 spybot) awaits the quest-accept / world-search /
	// escort bridges; npc_tyrion_spybot (escort Talk chains +
	// UpdateEntry(7779)) awaits the escort / world-search /
	// cross-creature-Talk / entry-update / despawn bridges;
	// npc_lord_gregor_lescovar (escort + Marzon summon + traitor
	// faction flip + guard despawn) awaits the escort / summon /
	// faction / world-search / despawn / quest bridges —
	// documented only.
	RegisterLuaBoss("npc_marzon_silent_blade", 1755)
	// npc_highborne_lamenter (Undercity zone script): the UpdateAI
	// EventCast one-shot arm — non-triggered self-cast of SPELL_
	// HIGHBORNE_AURA 37090 at 17.5s via the engine-fired OnSpawn
	// event (C++-exact timing); the 10s EventMove arm (disable-
	// gravity + MonsterMoveWithSpeed toward z=-55.50) awaits the
	// movement bridge — documented only. Entry 21628 verifiable
	// from the C++ Sylvanas enum — see lua_scripts/eastern_
	// kingdoms/npc_highborne_lamenter.lua. npc_lady_sylvanas_
	// windrunner (combat rotation bridgeable in principle via the
	// omen 1s-pump pattern — summon-skeleton 59711 / black-arrow
	// 59712 / shoot 59710 / multi-shot 59713 victim casts + fade
	// 20672 / fade-blink 29211 self casts — but no verifiable
	// entry in the C++ sources; the lament / ribbon / sunsorrow
	// machines await the quest-reward / sound / summon / world-
	// search / cross-creature-Talk bridges) joins the TDB-dump
	// entry-evidence queue — documented only. npc_parqual_
	// fintallas is header-named but never registered by AddSC_
	// undercity — no invented registration. Undercity closes the
	// EK zone set.
	RegisterLuaBoss("npc_highborne_lamenter", 21628)
	// npc_torturer_lecraft (Dragonblight zone script): the
	// JustEngagedWith schedule + UpdateAI combat rotation —
	// hemorrhage (DoCastVictim 30478, 5-8s engage / 12-168s
	// re-arm) + kidney shot (DoCastVictim 30621, 12-15s
	// engage / 20-26s re-arm) + Talk(SAY_AGGRO 0) on the
	// 1s-pump pattern; the SpellHit text-counter machine has no
	// SpellHit bridge (fizzule precedent) — documented only.
	// Entry 27394 verifiable from the C++ TorturerLeCraft enum
	// — see lua_scripts/northrend/npc_torturer_lecraft.lua.
	// npc_commander_eligor_dawnbringer (Naxxramas-wings talk
	// cinematic: MovePoint + MovementInform image-change chain +
	// GetCreatureListWithEntryInGrid/FindNearestCreature target
	// store + SetEntry/SetDisplayId image swap + audience
	// facing) awaits the movement / world-search / entry-update
	// / cross-creature-facing bridges — documented only.
	// spell_q12096_q12092_dummy + spell_q12096_q12092_bark
	// (Strengthen the Ancients SpellScripts) have no SpellScript
	// bridge anywhere in the model (blasted_lands / gordunni
	// precedents) — documented only. npc_wyrmrest_defender
	// (VehicleAI low-hp warn arm + SpellHit / gossip / charm
	// arms) has no verifiable NPC_ entry constant in the C++
	// sources and sits behind the VehicleAI / SpellHit / gossip
	// / flag bridges — documented only. Dragonblight is the
	// third Northrend zone (dalaran, borean_tundra closed).
	RegisterLuaBoss("npc_torturer_lecraft", 27394)
	// npc_emily (Grizzly Hills zone script): the JustEngagedWith arm —
	// Talk(SAY_RANDOMAGGRO 4, no target) — the event-1 auriaya
	// precedent (EscortAI base, but the arm is a bare self-Talk);
	// the WaypointReached escort cinematic Talk chain (SAY_WORGHAGGRO1
	// 0 / SAY_WORGRAGGRO3 2 / SAY_VICTORY2 6 / SAY_VICTORY3 7 /
	// SAY_VICTORY4 8 / SAY_QUEST_COMPLETE 12) + SummonCreature /
	// DoCast / vehicle / faction / MovePoint legs await the escort /
	// summon / cast / movement / faction bridges; OnQuestAccept
	// SAY_QUEST_ACCEPT 11 awaits the quest-accept bridge. Entry
	// 26588 verifiable from the C++ Floppy enum — see
	// lua_scripts/northrend/npc_emily.lua. npc_mrfloppy
	// (JustEngagedWith cross-creature Emily Talk only — bridged:
	// FindNearestCreature returns the luaMotionCreature object and
	// Talk is called on it, ported in npc_emily.lua), npc_outhouse_bunny
	// (SpellHit 48382 emote/cast arms — no SpellHit bridge),
	// npc_amberpine_woodsman (Talk-free emote-state machine —
	// unregistered AI, no C++ entry constant), npc_wounded_skirmisher (SpellHit
	// 48812 Talk(SAY_RANDOM) + 48813 — no SpellHit bridge),
	// npc_venture_co_straggler (EVENT_STRAGGLER_2 timer Talk(SAY_SEO)
	// + MovePoint chain — no-timer bridge; SpellHit 49075 —
	// no SpellHit bridge), npc_lake_frog (EVENT_LAKEFROG_2/5
	// timer Talk SAY_MAIDEN_0/1 — no-timer bridge; ReceiveEmote /
	// gossip arms — no bridges), npc_rocket_propelled_warhead
	// (VehicleAI PassengerBoarded/DoAction — no bridges), and the
	// six spell loaders (spell_shredder_delivery,
	// spell_infected_worgen_bite, spell_vehicle_warhead_fuse,
	// spell_warhead_detonate, spell_z_check, spell_warhead_fuse —
	// all SpellScript/AuraScript, no SpellScript bridge) are
	// documented only.
	RegisterLuaBoss("npc_emily", 26588)
	// npc_apothecary_hanes (Howling Fjord zone script): the
	// UpdateAI health arm — below 75% health the 10s PotTimer
	// ticks -> triggered self-cast SPELL_HEALING_POTION 17534
	// (armPump on OnSpawn(5), timer re-arm on OnLeaveCombat(2) /
	// OnReset(23), cancel on OnDied(4); the arm is not
	// combat-gated in C++, so it rides the always-on 1s pump —
	// underbog_mushroom precedent). The escort cinematic
	// (OnQuestAccept quest-11241 launch, waypoint 1-40 Talk /
	// emote chain, DoCastAOE 42685 burn crates, GroupEventHappens
	// credit) sits behind the quest-accept / escort / movement /
	// emote / faction / quest bridges — documented only. Entry
	// 23784 verifiable from the C++ Entries enum — see
	// lua_scripts/northrend/npc_apothecary_hanes.lua.
	// npc_daegarn (gladiator quest-11300 event + 40s idle Talk
	// loop) has no NPC_ entry constant in the C++ sources —
	// minigob precedent, no registration. npc_mindless_
	// abomination (EVENT_CHECK_CHARMED despawn arm) awaits the
	// charm-check / despawn bridges; spell_mindless_abomination_
	// explosion_fx_master has no SpellScript bridge;
	// npc_riven_widow_cocoon (JustDied player-killer summon /
	// kill-credit machine) has no killer-player / cast-on-me
	// path — all documented only. Howling Fjord is the fourth
	// Northrend zone (dalaran, borean_tundra, dragonblight,
	// grizzly_hills closed).
	RegisterLuaBoss("npc_apothecary_hanes", 23784)
	// npc_trapped_mammoth_calf (Borean Tundra zone script): the Reset()
	// trap-activation arm — scan the 22 mammoth-trap GOs in enum order,
	// first found within 11.0 yards gets SetGoState(GO_STATE_ACTIVE) —
	// ported in lua_scripts/northrend/npc_trapped_mammoth_calf.lua.
	// Entry 25850 verifiable from the C++ item script (World/
	// item_scripts.cpp:167, quest-11876); the UpdateAI MovePoint,
	// DoAction(1) and MovementInform legs await the movement /
	// DoAction / MovementInform bridges — documented only.
	RegisterLuaBoss("npc_trapped_mammoth_calf", 25850)
	// npc_blessed_banner (Icecrown zone script): the Reset() arm —
	// non-triggered self-cast SPELL_THREAT_PULSE 58113 (vaelastrasz
	// self-cast convention) + Talk(BANNER_SAY 0) textId-only
	// broadcast (lecraft aggro-Talk precedent) — armed on
	// OnSpawn(5) / OnReset(23); the banner never engages, so no
	// timers and no per-GUID state. SetRegenerateHealth(false)
	// is bridged and ported in the banner Lua; the EVENT_SPAWN 3s summon chain
	// (31003 + 3x 30919 + 3x 30900 with MovePoint machines), the
	// intro Talk/facing chain, the mason SetData(1,1) triggers, the
	// EVENT_WAVE_SPAWN 10-20s summon waves (30984/30987/30986),
	// EVENT_HALOF (30989), and the PhaseCount == 8 victory machine
	// (self-cast 58084 + DespawnEntry chain + EVENT_ENDED despawn)
	// sit behind the movement / summon / world-search /
	// cross-creature-Talk / despawn bridges — documented only.
	// npc_argent_valiant (combat rotation 63010/65147 bridgeable
	// in principle) has no NPC_ entry constant in the C++
	// sources — minigob precedent, no registration; its
	// DamageTaken duel-end machine has its damage-hook (ev9),
	// faction, despawn, home-position bridges; blockers are the
	// missing entry, player self-cast 63049, and EnterEvadeMode.
	// npc_guardian_pavilion (MoveInLineOfSight trespasser machine:
	// GetAreaId 4676/4677 gates, HasAura 63987/63986 gates,
	// GetTeamId branch, player self-cast) sits behind the
	// proximity / HasAura / team / player-self-cast bridges (the
	// area-ID bridge is live — lua_creature_events.go — but the
	// proximity / facing / team legs still block the machine). npc_tournament_training_dummy (entries
	// 33272/33229/33243 verifiable) — the SetControlled(STUNNED)
	// leg has no unit-state bridge; DamageTaken zeroing, the
	// SpellHit 62544/62626/62874 credit machines, the HasAura-
	// gated recast-defend self-casts 64100/62719 and the vulnerable latch
	// are ported in
	// lua_scripts/northrend/npc_tournament_training_dummy.lua.
	// npc_frostbrood_skytalon
	// (VehicleAI) — the UpdateAI arms run through the unmodeled
	// VehicleAI base; SpellHit 59335/59319 rides ev14 but the
	// unit has no NPC_ entry constant, so it stays unregistered. Entry 30891 verifiable from the C++ BlessedBanner
	// enum — see lua_scripts/northrend/npc_blessed_banner.lua.
	// Icecrown is the fifth Northrend zone (dalaran,
	// borean_tundra, dragonblight, grizzly_hills, howling_fjord
	// closed).
	RegisterLuaBoss("npc_blessed_banner", 30891)
	// npc_storm_cloud (Zul'Drak zone script): the Reset() /
	// JustAppeared() arm — triggered self-cast STORM_VISUAL
	// 55708 (vaelastrasz self-cast convention; the triggered
	// flag is accepted but not threaded — gruul reverberation
	// precedent) — armed on OnSpawn(5) / OnReset(23); the cloud
	// never engages, so no timers and no per-GUID state. The
	// SpellHit(55516 GYMERS_GRAB) vehicle arm (kit seat-count
	// gate -> RIDE_VEHICLE 43671 + HEALING_WINDS 55549 on the
	// caster) has no SpellHit / vehicle / passenger bridges.
	// npc_drakuru_shackles (Reset summon/facing arms + the
	// SpellHit 55083 quest-12861 machine) has no flag / summon /
	// facing / SpellHit / quest / kill-credit / despawn bridges;
	// npc_captured_rageclaw (the SpellHit 55223 release machine
	// with faction / stand-state / MoveRandom / despawn arms) —
	// the Reset self-cast 54990 arm was deferred as a stranded
	// half rather than ported; npc_released_offspring_harkoa
	// (MovePoint + MovementInform despawn), npc_crusade_recruit
	// (RECRUIT_1/2 event machine + OnGossipSelect player-cast),
	// go_scourge_enclosure (GO gossip quest-12916 credit machine),
	// npc_alchemist_finklestein (the facing/emote loop +
	// ingredient aura machine) — the OnGossipSelect arm
	// (CloseGossip + CastSpell(player, 51216) + 100ms ->
	// Talk(0, player) + CastSpell(player, 51015)) is PORTED in
	// lua_scripts/northrend/npc_storm_cloud.lua; go_finklesteins_
	// cauldron (gossip 51046 self-cast), and the five
	// SpellScript/AuraScript loaders (spell_random_ingredient_
	// aura, spell_random_ingredient, spell_pot_check,
	// spell_fetch_ingredient_aura, the four scourge_disguise
	// scripts) have no SpellScript / AuraScript / gossip /
	// quest / world-search bridges — all documented only. Entry
	// 29939 verifiable from the C++ StormCloud enum — see
	// lua_scripts/northrend/npc_storm_cloud.lua. Zul'Drak is
	// the last enabled Northrend zone (dalaran, borean_tundra,
	// dragonblight, grizzly_hills, howling_fjord, icecrown,
	// sholazar_basin, storm_peaks, wintergrasp closed); the
	// Northrend zone set is now CLOSED.
	// Late-resolved quest_arms residuals (entries wowhead-verified
	// this tick; the arms were parked entry-unverifiable): the
	// etymidian OnQuestReward Talk arm and the injured-goblin
	// OnQuestAccept Talk arm — see lua_scripts/kalimdor/
	// npc_the_etymidian.lua and lua_scripts/northrend/
	// npc_injured_goblin.lua.
	RegisterLuaBoss("npc_the_etymidian", 28092)
	RegisterLuaBoss("npc_injured_goblin", 29434)
	// npc_brann_bronzebeard_keystone (Storm Peaks zone script, quest
	// 13285 "Forging the Keystone" — the TaskScheduler-census deferred
	// 13-event chain): OnGossipSelect starts the chain and the timed
	// Talk / voice-resurrection / signal-summon / kill-credit /
	// facing / Despawn steps port (zone_storm_peaks.cpp:474-624) —
	// see lua_scripts/northrend/npc_brann_bronzebeard_keystone.lua.
	// The SetWalk/MovePoint locomotion steps and the step-11 signal
	// Delete leg stay documented-only (no movement or GO-by-GUID
	// bridge). Entry 31810 verifiable from the file's own enum.
	RegisterLuaBoss("npc_brann_bronzebeard_keystone", 31810)
	// npc_av_marshal_or_warmaster (Alterac Valley zone script) — the first
	// unit of the Eastern Kingdoms pass (eastern_kingdoms_script_loader.cpp
	// order: AddSC_alterac_valley; boss_balinda follows). The combat event
	// machine (charge 22911 victim-cast, cleave 40504 victim-cast, demoral-
	// izing shout 23511 self-cast, whirlwind 13736 self-cast, enrage 8599
	// self-cast) plus the per-entry one-shot aura self-cast from the C++
	// _auraPairs table (14762->45828 / 14763->45829 / 14765->45830 /
	// 14764->45831 / 14773->45822 / 14776->45823 / 14777->45824 /
	// 14772->45826) — maiden-of-virtue convention (CreateLuaEvent timers;
	// GetVictim for DoCastVictim; no UNIT_STATE_CASTING gate). C++ quirk:
	// the charge arm reschedules undefined EVENT_CHARGE (would not compile
	// upstream) — the port re-arms the charge chain at the stated {10,25}s.
	// EVENT_CHECK_RESET (home leash) has no HomePosition bridge —
	// documented-only. All eight entries are verifiable from the file's own
	// Creatures enum — see lua_scripts/eastern_kingdoms/
	// npc_av_marshal_or_warmaster.lua.
	RegisterLuaBoss("npc_av_marshal_or_warmaster", 14762)
	RegisterLuaBoss("npc_av_marshal_or_warmaster", 14763)
	RegisterLuaBoss("npc_av_marshal_or_warmaster", 14764)
	RegisterLuaBoss("npc_av_marshal_or_warmaster", 14765)
	RegisterLuaBoss("npc_av_marshal_or_warmaster", 14772)
	RegisterLuaBoss("npc_av_marshal_or_warmaster", 14773)
	RegisterLuaBoss("npc_av_marshal_or_warmaster", 14776)
	RegisterLuaBoss("npc_av_marshal_or_warmaster", 14777)
	// boss_balinda (Alterac Valley) — the combat event machine (arcane
	// explosion 46608 victim-cast {5,15}s, cone of cold 38384 victim-cast
	// 8s->{10,20}s, fireball 46988 victim-cast 1s->{5,9}s, frostbolt 46987
	// victim-cast 4s->{4,12}s) plus the aggro Talk(0) and the DamageTaken
	// 40% one-shot iceblock latch (46604) — maiden/moroes convention
	// (CreateLuaEvent timers; GetVictim for DoCastVictim; no
	// UNIT_STATE_CASTING gate). The water elemental summon arm, the
	// JustSummoned AttackStart/faction arms, the home-leash CHECK_RESET
	// arm (no HomePosition bridge), and the BG-driven
	// DoAction(ACTION_BUFF_YELL) arm have no Lua bridges — documented
	// only. Entry 11949 verifiable from the C++ sources: BattlegroundAV.h
	// names "Captain Balinda Stonehearth" at 11949 — see
	// lua_scripts/eastern_kingdoms/boss_balinda.lua.
	RegisterLuaBoss("boss_balinda", 11949)
	// boss_drekthar (Alterac Valley) — the 5-event combat scheduler
	// (whirlwind 15589 victim-cast {1,20}s->{8,18}s, whirlwind2 13736
	// victim-cast {1,20}s->{7,25}s, knockdown 19128 victim-cast
	// 12s->{10,15}s, frenzy 8269 victim-cast 6s->{20,30}s, random-yell
	// Talk(3) {20,30}s->{20,30}s) plus the aggro Talk(0) and the
	// JustAppeared Talk(2) respawn yell (no JustAppeared bridge —
	// carried on OnSpawn(5), storm_cloud convention) — maiden
	// convention (CreateLuaEvent timers; GetVictim for DoCastVictim;
	// no UNIT_STATE_CASTING gate). The CheckInRoom home leash arm
	// (home-position 2D distance > 50yd -> EnterEvadeMode + Talk(1))
	// stays documented-only: EnterEvadeMode has no Lua bridge and
	// Talk(1) fires only inside the evade flow (a Talk-only port would
	// yell without evading — fabrication), not for lack of position
	// reads (creature:GetHomePosition is bridged, lua_objects.go:337);
	// evade still re-arms through OnReset(23). SPELL_SWEEPING_STRIKES 18765 /
	// SPELL_CLEAVE 20677 / SPELL_WINDFURY 35886 / SPELL_STORMPIKE 51876
	// are declared but unused by this AI. Entry 11946 verifiable from
	// the C++ sources: BattlegroundAV.h names "Drek'Thar" at 11946 —
	// see lua_scripts/eastern_kingdoms/boss_drekthar.lua.
	RegisterLuaBoss("boss_drekthar", 11946)
	// boss_galvangar (Alterac Valley) — the 5-event combat scheduler
	// (cleave 15284 victim-cast {1,9}s->{10,16}s, frightening shout
	// 19134 victim-cast {2,19}s->{10,15}s, whirlwind1 15589 victim-cast
	// {1,13}s->{6,10}s, whirlwind2 13736 victim-cast {5,20}s->{10,25}s,
	// mortal strike 16856 victim-cast {5,20}s->{10,30}s) plus the aggro
	// Talk(0) — maiden convention (CreateLuaEvent timers; GetVictim
	// for DoCastVictim; no UNIT_STATE_CASTING gate). The CheckInRoom
	// home leash arm (home-position 2D distance > 50yd -> EnterEvadeMode
	// + Talk(1)) and the BG-driven DoAction(ACTION_BUFF_YELL=-30001) ->
	// Talk(2) arm have no Lua bridges — documented only; evade still
	// re-arms through OnReset(23). Entry 11947 verifiable from the C++
	// sources: BattlegroundAV.h names "Captain Galvangar" at 11947 —
	// see lua_scripts/eastern_kingdoms/boss_galvangar.lua.
	RegisterLuaBoss("boss_galvangar", 11947)
	// boss_vanndar (Alterac Valley; TaskScheduler combat pump —
	// avatar 19135 victim-cast 3s->{15,20}s, thunderclap 15588
	// victim-cast 4s->{5,15}s, stormbolt 20685 victim-cast 6s->
	// {10,25}s, random-yell Talk(2) {20,30}s->{20,30}s) plus the aggro
	// Talk(0) — maiden convention (CreateLuaEvent timers; GetVictim
	// for DoCastVictim; no UNIT_STATE_CASTING gate). The 5s home leash
	// arm (home-position 2D distance > 50yd -> EnterEvadeMode +
	// Talk(1)) has no Lua bridge — documented only; evade still re-arms
	// through OnReset(23). YELL_SPELL=3 declared but unused. Entry 11948
	// verifiable from the C++ sources: BattlegroundAV.h names "Vanndar
	// Stormpike" at 11948 — see lua_scripts/eastern_kingdoms/boss_vanndar.lua.
	// instance_blackrock_depths (AddSC_instance_blackrock_depths; BRD
	// encounter-state machine, map 230 — Ring of Law/Vault/Bar/Tomb of
	// Seven/Lyceum/Iron Hall encounter slots, GO-door GUID latching
	// (arena1-4, shadow-lock, bar keg/door, tomb enter/exit, lyceum,
	// braziers, golem/throne rooms, spectral chalice), quest-gated Moira
	// UpdateEntry transform, save/load instance data string, and the 15s
	// Tomb of Seven faction-sequencer (ghost kill count 0..7 with boss
	// respawn/faction resets)): every arm needs the instance-script
	// model — the Lua scripting surface has no InstanceMapScript bridge,
	// so this unit is documented-only.
	RegisterLuaBoss("boss_vanndar", 11948)
	// npc_phalanx (Blackrock Depths; ScriptedAI combat scheduler —
	// thunderclap 8732 victim-cast 12s->10s, fireballvolley 22425
	// victim-cast 15s gated on HealthBelowPct(51) (GetHealthPct on the
	// motion object via the generic-field path in scripting/object.go),
	// mightyblow 14099 victim-cast 15s->10s) — maiden convention
	// (CreateLuaEvent timers; GetVictim for DoCastVictim; no
	// UNIT_STATE_CASTING gate; melee engine-driven). Entry 9502
	// verifiable from the C++ sources: instance_blackrock_depths.cpp
	// names NPC_PHALANX at 9502 (DATA_PHALANX = 11 in
	// blackrock_depths.h) — see lua_scripts/eastern_kingdoms/npc_phalanx.lua.
	RegisterLuaBoss("npc_phalanx", 9502)
	// boss_emperor_dagran_thaurissan / boss_draganthaurissanAI (Blackrock
	// Depths; ScriptedAI combat scheduler via GetBlackrockDepthsAI —
	// avatarofflame 15636 victim-cast 25s->18s, aggro Talk(0), player-kill
	// Talk(1) on CREATURE_EVENT_ON_TARGET_DIED) — maiden convention
	// (CreateLuaEvent timers; GetVictim for DoCastVictim; melee
	// engine-driven). Entry 9019 verifiable from the C++ sources:
	// instance_blackrock_depths.cpp names NPC_EMPEROR at 9019 (BRD
	// instance creatures enum, line 35) — see
	// lua_scripts/eastern_kingdoms/boss_emperor_dagran_thaurissan.lua.
	// HANDOFTHAURISSAN 17492 sits on SelectTarget(Random) — no bridge;
	// JustDied moira arm needs the instance-script model.
	RegisterLuaBoss("boss_emperor_dagran_thaurissan", 9019)

	// boss_doomrel (Doomrel, entry 9039 — NPC_DOOMREL at
	// instance_blackrock_depths.cpp:43): ScriptedAI combat scheduler.
	// Ported: Shadowbolt Volley 15245 (10s->12s), Curse of Weakness
	// 12493 (5s->45s), Demon Armor 13787 (16s->5min), DamageTaken 50%
	// one-shot Summon Voidwalkers 15092 (triggered, once, resets on
	// 23). Gossip challenge menu (texts approximated, DB-side).
	// Unmodeled: Immolate 12742 (SelectTarget Random, no bridge),
	// faction/immune/flag/instance arms (no bridges). Full analysis in
	// lua_scripts/eastern_kingdoms/boss_doomrel.lua.
	RegisterLuaBoss("boss_doomrel", 9039)

	// boss_gloomrel (Gloomrel, entry 9037 — NPC_GLOOMREL at
	// instance_blackrock_depths.cpp:42): pure gossip teaching script.
	// Ported: hello gates (quest 4083 rewarded + mining >= 230 +
	// no smelt 14891 -> teach option; unrewarded + mining >= 230 ->
	// tribute option) and select arms (teach-2 menu 2606, learn
	// smelt 14894, teach-3 menu 2604, tribute close). Unmodeled:
	// select +22 chalice respawn (instance GUID-data bridge) and
	// the hello menu title id (GetGossipTextId bridge). Full analysis
	// in lua_scripts/eastern_kingdoms/boss_gloomrel.lua.
	RegisterLuaBoss("boss_gloomrel", 9037)

	// boss_general_angerforge / boss_general_angerforgeAI (Blackrock
	// Depths, Hall of Crafting; ScriptedAI — mightyblow 14099 8s->18s,
	// hamstring 9080 12s->15s, cleave 20691 16s->9s, all DoCastVictim
	// phase-agnostic; DamageTaken 20% one-shot phase latch -> EVENT_MEDIC
	// one-shot + EVENT_ADDS 0s->25s PHASE_TWO nil-ticks (no positional
	// spawn bridge). Entry 9033 DB-verified (classic NPC list; neighbors
	// 9034-9040 are the C++-verified NPC_ANGERREL..NPC_DOOMREL, 9019 the
	// C++-verified NPC_EMPEROR — flamelash 9156 precedent) — see
	// lua_scripts/eastern_kingdoms/boss_general_angerforge.lua.
	RegisterLuaBoss("boss_general_angerforge", 9033)
	// boss_high_interrogator_gerstahn / boss_high_interrogator_gerstahnAI
	// (Blackrock Depths, Detention Block; ScriptedAI combat scheduler —
	// SW:P 10894 4s->7s, Mana Burn 10876 14s->10s, Psychic Scream 8122
	// 32s->30s, Shadow Shield 22417 8s->25s self-cast; random-alive-player
	// <=100yd selectors, re-arm outside target-nil check). Entry 9018
	// DB-verified (flamelash 9156 precedent) — see
	// lua_scripts/eastern_kingdoms/boss_high_interrogator_gerstahn.lua.
	RegisterLuaBoss("boss_high_interrogator_gerstahn", 9018)

	// boss_coren_direbrew (Coren Direbrew, entry 23872 — NPC_COREN at
	// instance_blackrock_depths.cpp:47): BossAI combat scheduler.
	// Ported: Summon Mole Machine 47691 (15s->15s, triggered self-cast),
	// Direbrew's Disarm 47407 (20s->20s, triggered self-cast). Gossip
	// fight/apologize menu (texts approximated, DB-side). Unmodeled:
	// intro phase + 3x antagonist summons, DamageTaken 66%/33% sister
	// summons (26764/26822), sisters/minion/antagonist scripts, mole
	// machine GO script, six SpellScript/AuraScripts, LFG FinishDungeon
	// — no summon/GO/spell/LFG bridges. Full analysis in
	// lua_scripts/eastern_kingdoms/boss_coren_direbrew.lua.
	RegisterLuaBoss("boss_coren_direbrew", 23872)

	// boss_magmus / boss_magmusAI (Blackrock Depths, Iron Hall; ScriptedAI
	// combat scheduler via GetBlackrockDepthsAI — fieryburst 13900
	// victim-cast 5s->6s; DamageTaken 50% one-shot phase-two latch
	// (moroes convention) -> warstomp 24375 victim-cast 0s->8s; melee
	// engine-driven). Entry 9938 verifiable from the C++ sources:
	// instance_blackrock_depths.cpp names NPC_MAGMUS at 9938 (BRD
	// instance creatures enum, line 44) — see
	// lua_scripts/eastern_kingdoms/boss_magmus.lua. JustEngagedWith
	// instance->SetData(TYPE_IRON_HALL, IN_PROGRESS) and the JustDied
	// throne-door/DONE arms need the instance-script model (unmodeled);
	// the sibling npc_ironhand_guardian is instance-arm-gated plus
	// DoCastAOE — documented-only.
	RegisterLuaBoss("boss_magmus", 9938)
	// boss_ambassador_flamelash / boss_ambassador_flamelashAI (Blackrock
	// Depths, Chamber of Enchantment; ScriptedAI — fireblast 15573
	// victim-cast 2s->7s) — maiden convention (CreateLuaEvent timers;
	// GetVictim for DoCastVictim; no UNIT_STATE_CASTING gate in the C++
	// UpdateAI; melee engine-driven). Entry 9156 wowhead-verified
	// (wowhead.com/mop-classic/npc=9156/ambassador-flamelash; no NPC_
	// constant in the C++ tree — DB ScriptName binding) — see
	// lua_scripts/eastern_kingdoms/boss_ambassador_flamelash.lua.
	// EVENT_SUMMON_SPIRITS (24s->30s, Burning Spirit 9178 x4 via
	// DoSpawnCreature + AttackStart) keeps its C++-exact schedule with
	// the spawn leg unmodeled (no positional-spawn bridge; azuregos
	// nil-tick schedule convention).
	RegisterLuaBoss("boss_ambassador_flamelash", 9156)
	// boss_moira_bronzebeard / boss_moira_bronzebeardAI (Blackrock
	// Depths, Lyceum; ScriptedAI combat scheduler via GetBlackrockDepthsAI
	// — mindblast 10947 victim-cast 16s->14s, shadowwordpain 10894
	// victim-cast 2s->18s, smite 10934 victim-cast 8s->10s; melee
	// engine-driven). Entry 8929 verifiable from the C++ sources:
	// instance_blackrock_depths.cpp names NPC_MOIRA at 8929 (BRD
	// instance creatures enum, line 45) — see
	// lua_scripts/eastern_kingdoms/boss_moira_bronzebeard.lua. The C++
	// EVENT_HEAL arm is commented out ("not used atm") and left out.
	RegisterLuaBoss("boss_moira_bronzebeard", 8929)
	// boss_drakkisath / boss_drakkisathAI (Blackrock Spire, Hall of
	// Blackhand; BossAI combat scheduler via GetBlackrockSpireAI —
	// firenova 23462 victim-cast 6s->10s, cleave 20691 victim-cast
	// 8s->8s, confligration 16805 victim-cast 15s->18s, thunderclap
	// 15548 victim-cast 17s->20s; melee engine-driven). Entry 10363
	// verifiable from the C++ sources: blackrock_spire.h names
	// NPC_GENERAL_DRAKKISATH at 10363 (BRS creatures enum) — see
	// lua_scripts/eastern_kingdoms/boss_drakkisath.lua. GetBlackrockSpireAI
	// is a GetInstanceAI retrieval wrapper (blackrock_spire.h:129-132);
	// the AI has no instance arms — Reset() _Reset() and JustDied()
	// _JustDied() are covered by the cancel/re-arm on combat events.
	RegisterLuaBoss("boss_drakkisath", 10363)
	// boss_halycon / boss_halyconAI (Blackrock Spire, Hall of Blackhand;
	// BossAI combat scheduler via GetBlackrockSpireAI — rend 13738
	// victim-cast 17s->8s, thrash 3391 self-cast one-shot 10s (C++-exact:
	// no re-arm), Talk(EMOTE_DEATH = 0) on OnDied; melee engine-driven).
	// Entry 10220 verifiable from the C++ sources: blackrock_spire.h
	// names NPC_HALYCON at 10220 (BRS creatures enum) — see
	// lua_scripts/eastern_kingdoms/boss_halycon.lua. GetBlackrockSpireAI
	// is a GetInstanceAI retrieval wrapper (blackrock_spire.h:129-132);
	// the AI has no instance arms — Reset() _Reset() / Initialize() are
	// covered by the cancel/re-arm on combat events. Unmodeled: JustDied
	// SummonCreature(NPC_GIZRUL_THE_SLAVENER = 10268, timed 5min) — no
	// SummonCreature bridge (phoenix / flamelash precedent).
	RegisterLuaBoss("boss_halycon", 10220)
	// boss_highlord_omokk / boss_highlordomokkAI (Blackrock Spire, Hall
	// of Blackhand; BossAI combat scheduler via GetBlackrockSpireAI —
	// frenzy 8269 victim-cast 20s->1min, knock-away 10101 victim-cast
	// 18s->12s; melee engine-driven). Entry 9196 verifiable from the C++
	// sources: blackrock_spire.h names NPC_HIGHLORD_OMOKK at 9196 (BRS
	// creatures enum) — see
	// lua_scripts/eastern_kingdoms/boss_highlord_omokk.lua.
	// GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper
	// (blackrock_spire.h:129-132); the AI has no instance arms —
	// Reset() _Reset() / JustDied _JustDied() are internal BossAI
	// machinery covered by the cancel/re-arm on combat events.
	RegisterLuaBoss("boss_highlord_omokk", 9196)
	// boss_mother_smolderweb / boss_mothersmolderwebAI (Blackrock
	// Spire, Caverns of Abomination; BossAI combat scheduler via
	// GetBlackrockSpireAI — crystalize 16104 self-cast 20s->15s,
	// mothersmilk 16468 self-cast 10s->5s, lethal-hit self-cast
	// summon 16103 (DoCast(me, 16103, true)); melee engine-driven).
	// Entry 10596 verifiable from the C++ sources: blackrock_spire.h
	// names NPC_MOTHER_SMOLDERWEB at 10596 (BRS creatures enum) —
	// see lua_scripts/eastern_kingdoms/boss_mother_smolderweb.lua.
	RegisterLuaBoss("boss_mother_smolderweb", 10596)
	// boss_overlord_wyrmthalak / boss_overlordwyrmthalakAI (Blackrock
	// Spire, Hall of Blackhand; BossAI combat scheduler via
	// GetBlackrockSpireAI — blastwave 11130 20s->20s, shout 23511
	// 2s->10s, cleave 20691 6s->7s, knockaway 20686 12s->14s, all
	// victim-cast; melee engine-driven; 51%-HP add-summon latch
	// unmodeled — no SelectTarget/SummonCreature bridges).
	// Entry 9568 verifiable from the C++ sources: blackrock_spire.h
	// names NPC_OVERLORD_WYRMTHALAK at 9568 (BRS creatures enum) —
	// see lua_scripts/eastern_kingdoms/boss_overlord_wyrmthalak.lua.
	RegisterLuaBoss("boss_overlord_wyrmthalak", 9568)
	// boss_shadow_hunter_voshgajin / boss_shadowvoshAI (Blackrock
	// Spire, entrance encounter; BossAI combat scheduler via
	// GetBlackrockSpireAI — curseofblood 24673 2s->45s, cleave 20691
	// 14s->7s, both victim-cast; melee engine-driven; hex unmodeled —
	// no SelectTarget bridge; Reset's ice-armor self-cast is commented
	// out in C++ and omitted by design).
	// Entry 9236 verifiable from the C++ sources: blackrock_spire.h
	// names NPC_SHADOW_HUNTER_VOSHGAJIN at 9236 (BRS creatures enum) —
	// see lua_scripts/eastern_kingdoms/boss_shadow_hunter_voshgajin.lua.
	RegisterLuaBoss("boss_shadow_hunter_voshgajin", 9236)
	// boss_the_beast / boss_the_beast (Blackrock Spire, arena encounter;
	// BossAI combat scheduler via GetBlackrockSpireAI — flamebreak 16785
	// 12s->10s, terrifyingroar 14100 23s->20s, fireball 16788 8s->8s..21s,
	// fireblast 16144 5s->5s..8s, all victim-cast; melee engine-driven;
	// immolate + berserkercharge unmodeled — no SelectTarget bridge;
	// SetData beast-room/reached arms + both area triggers + SpellHit
	// skinning arm unmodeled — no area-trigger/instance/MotionMaster/
	// SpellHit bridges; Reset _Reset / JustDied _JustDied covered by the
	// cancel on 2/4/23).
	// Entry 10430 verifiable from the C++ sources: blackrock_spire.h
	// names NPC_THE_BEAST at 10430 (BRS creatures enum) — see
	// lua_scripts/eastern_kingdoms/boss_the_beast.lua.
	// Warmaster Voone: 6-event combat scheduler; see
	// lua_scripts/eastern_kingdoms/boss_warmaster_voone.lua.
	// C++-exact: EVENT_PUMMEL (15615, one-shot 32s) re-arms
	// EVENT_MORTAL_STRIKE at 16s rather than itself; mortal-strike
	// otherwise loops 10s (12s init), snap-kick 6s loop (8s init),
	// cleave 12s loop (14s init), uppercut 14s loop (20s init),
	// throw-axe 8s loop (1s init) — all DoCastVictim (maiden
	// convention). No UNIT_STATE_CASTING model in Go; Reset
	// _Reset / JustDied _JustDied covered by the cancel on 2/4/23.
	// Entry 9237 verifiable from the C++ sources: blackrock_spire.h
	// names NPC_WARMASTER_VOONE at 9237 (BRS creatures enum).
	RegisterLuaBoss("boss_warmaster_voone", 9237)
	// Quartermaster Zigris: 2-event combat scheduler; see
	// lua_scripts/eastern_kingdoms/boss_quartermaster_zigris.lua.
	// Shoot 16496 1s->500ms loop, stun-bomb 16497 16s->14s loop —
	// both DoCastVictim (maiden convention). SPELL_HEALING_POTION
	// 15504 + SPELL_HOOKEDNET 15609 are enum-only in C++ (never
	// cast), omitted by design. No UNIT_STATE_CASTING model in Go;
	// Reset _Reset / JustDied _JustDied covered by the cancel on
	// 2/4/23.
	// Entry 9736 verifiable from the C++ sources: blackrock_spire.h
	// names NPC_QUARTERMASTER_ZIGRIS at 9736 (BRS creatures enum).
	RegisterLuaBoss("quartermaster_zigris", 9736)
	RegisterLuaBoss("boss_the_beast", 10430)
	// Pyroguard Emberseer: 3-event combat scheduler; see
	// lua_scripts/eastern_kingdoms/boss_pyroguard_emberseer.lua.
	// Fireshield 13376 self-cast 3s loop, firenova 23462 self-cast
	// 6s loop, flamebuffet 23341 self-cast 3s->14s loop. The whole
	// pre-fight event (immunes, altar player-check, incarceerator
	// grid loop, freeze/growing stacks, SpellHit arms) is
	// documented-only: no instance / SpellHit / SelectTarget bridges.
	// EVENT_PYROBLAST 17274 is a random-target cast (no SelectTarget
	// bridge) — unmodeled. No UNIT_STATE_CASTING model in Go; Reset
	// _Reset / JustDied _JustDied covered by the cancel on 2/4/23.
	// Entry 9816 verifiable from the C++ sources: blackrock_spire.h
	// names NPC_PYROGAURD_EMBERSEER at 9816 (BRS creatures enum).
	RegisterLuaBoss("boss_pyroguard_emberseer", 9816)
	// Blackhand Incarcerator: 1-event combat scheduler; see
	// lua_scripts/eastern_kingdoms/npc_blackhand_incarcerator.lua.
	// Strike 15580 victim-cast 8s init -> 14s loop (urand ranges use
	// the lower bound, halycon convention). EVENT_ENCAGE 16045 is a
	// random-target cast (no SelectTarget bridge) — unmodeled;
	// JustAppeared/JustReachedHome spawn/evade arms and the
	// JustEngagedWith DoZoneInCombat grid loop have no bridges.
	// Entry 10316 verifiable from the C++ sources: blackrock_spire.h
	// names NPC_BLACKHAND_INCARCERATOR at 10316 (BRS creatures enum).
	RegisterLuaBoss("npc_blackhand_incarcerator", 10316)
	// Gyth (Rend Blackhand's mount): 4-event combat scheduler; see
	// lua_scripts/eastern_kingdoms/boss_gyth.lua.
	// Corrosive-acid 16359 / freeze 16350 / flamebreath 16390 self-cast
	// 8s init -> 10s loop; knock-away 10101 victim-cast 12s init ->
	// 14s loop (urand ranges use the lower bound, halycon convention).
	// 5%-HP latch -> self-cast summon-rend 16328 via OnDamageTaken(9)
	// (mother_smolderweb latch convention; the RemoveAura(16167)
	// arm has no bridge). Pre-fight event (portcullis, Nefarius
	// SetData, MovePath) is documented-only — no gameobject /
	// creature-list / MotionMaster bridges; Reset/JustDied instance
	// arms unmodeled. No UNIT_STATE_CASTING model in Go; Reset
	// _Reset / JustDied _JustDied covered by the cancel on 2/4/23.
	// Entry 10339 verifiable from the C++ sources: blackrock_spire.h
	// names NPC_GYTH at 10339 (BRS creatures enum).
	RegisterLuaBoss("boss_gyth", 10339)
	// lua_scripts/eastern_kingdoms/boss_rend_blackhand.lua.
	// Whirlwind 13736 self-cast 13s init -> 13s loop; cleave 15284
	// victim-cast 15s init -> 10s loop; mortal-strike 16856 victim-cast
	// 17s init -> 14s loop (urand ranges use the lower bound, halycon
	// convention). SPELL_FRENZY 8269 / SPELL_KNOCKDOWN 13360 are
	// enum-only in C++ (never cast), omitted by design. The whole
	// pre-fight gyth event chain (Nefarius FindNearestCreature,
	// portcullis gameobject, MovePath, NearTeleportTo, SummonCreature
	// of Gyth, area-trigger SetData) is documented-only — no bridges
	// on the Lua surface (the_beast / gyth precedent); wave tables are
	// commented out in C++ and the EVENT_WAVE_1..6 bodies are no-ops.
	// JustDied's SetData(1, 2) on Nefarius unmodeled (creature-list
	// precedent); IsSummonedBy's SetImmuneToPC(false) + DoZoneInCombat
	// has no spawn bridge (emberseer precedent); MovementInform
	// despawn gates on MotionMaster/DespawnOrUnsummon. Reset _Reset /
	// JustDied _JustDied covered by the cancel on 2/4/23. Entry 10429
	// verifiable from the C++ sources: blackrock_spire.h:70 names
	// NPC_WARCHIEF_REND_BLACKHAND = 10429.
	RegisterLuaBoss("boss_rend_blackhand", 10429)
	// lua_scripts/eastern_kingdoms/boss_gizrul_the_slavener.lua.
	// Fatal-bite 16495 victim-cast 17s init -> 8s loop; infected-bite
	// 16128 is a one-shot self-cast at 10s — the C++ handler never
	// re-arms it, it schedules EVENT_FATAL_BITE (8s) instead, so the
	// port overwrites the live fatal-bite timer (EventMap semantics —
	// voone pummel precedent). SPELL_FRENZY 8269 is enum-only in C++
	// (never cast), omitted by design. IsSummonedBy's MovePath
	// (GIZRUL_PATH 402450) is documented-only — no MotionMaster
	// bridge (the_beast precedent). No UNIT_STATE_CASTING model in
	// Go; Reset _Reset / JustDied _JustDied covered by the cancel on
	// 2/4/23. Entry 10268 verifiable from the C++ sources:
	// blackrock_spire.h:66 names NPC_GIZRUL_THE_SLAVENER = 10268
	// (halycon's summon bridge was unbridgeable, but the entry itself
	// is C++-verifiable so the port is registered).
	RegisterLuaBoss("boss_gizrul_the_slavener", 10268)
	// lua_scripts/eastern_kingdoms/boss_urok_doomhowl.lua.
	// Rend 16509 victim-cast 17s init -> 8s loop; strike 15580
	// victim-cast 10s init -> 8s loop. Talk(SAY_AGGRO) fires on
	// combat entry (creature:Talk bridged; maiden-of-virtue
	// precedent). The C++ file schedules the spell ids as event
	// ids directly (EVENT_* constants never scheduled) — the
	// port keys timers "rend"/"strike" with EventMap cancel-on-
	// re-schedule semantics. SPELL_INTIMIDATING_ROAR 16508 is
	// enum-only in C++ (never cast), omitted by design. No
	// UNIT_STATE_CASTING model in Go; Reset _Reset / JustDied
	// _JustDied covered by the cancel on 2/4/23. Entry 10584
	// verifiable from the C++ sources: blackrock_spire.h:64 names
	// NPC_UROK_DOOMHOWL = 10584 (gizrul / rend_blackhand precedent).
	RegisterLuaBoss("boss_urok_doomhowl", 10584)
	// lua_scripts/eastern_kingdoms/boss_lord_valthalak.lua.
	// Self-cast summon-spectral-assassin 27249 6s init -> 30s loop;
	// victim-cast shadow-wrath 27286 9s init -> 19s loop (urand lower
	// bounds; gyth/urok convention). Health latches via OnDamageTaken(9):
	// 40% one-shot -> self-cast frenzy 8269 + Talk(EMOTE_FRENZY=0) +
	// CancelEvent(summon) (C++-exact drop, not re-arm); 15% one-shot ->
	// self-cast frenzy + Talk(EMOTE_FRENZY) + ScheduleEvent shadow-bolt-
	// volley 27382 7s init -> 4s loop (volley fires only after the 15%
	// latch; gyth latch convention). Entry 16042 not in the C++ creature
	// enum (ScriptName binding is DB-side); verified from the 3.3.5 data
	// source (npc=16042/lord-valthalak, "Mea Culpa" quest objective NPC
	// #16042 slain — Lord Valthalak); AI runs under BossAI(DATA_LORD_
	// VALTHALAK = 14, blackrock_spire.h:44). Reset _Reset / JustDied
	// _JustDied covered by the cancel on 2/4/23; JustDied's instance
	// SetData(DONE) has no bridge (documented-only).
	RegisterLuaBoss("boss_lord_valthalak", 16042)
	// instance_blackrock_spire (AddSC_instance_blackrock_spire; BRS
	// encounter-state machine, map 229 — InstanceMapScript + 3
	// AreaTriggerScripts. 23 encounter slots (DATA_HIGHLORD_OMOKK=0 ..
	// DATA_GENERAL_DRAKKISATH=13, DATA_DRAGONSPIRE_ROOM=15,
	// DATA_HALL_RUNE_1..7=16..22), DoorData for GO_DOORS / GO_EMBERSEER_OUT
	// / GO_DRAKKISATH_DOOR_1+2, GUID latches for all 14 bosses plus scarshield
	// infiltrator / finkle einhorn / 6 blackhand incarcerators, done-state
	// DespawnOrUnsummon of ember-seer/rend/nefarius, GO GUID latching with
	// done-state open/close on create (emberseer_in, doors, emberseer_out,
	// 7 hall runes, 7 emberseer runes, 2 portcullis), whelp-spawner spell,
	// ProcessEvent -> emberseer SetData(1,1), SetData AREATRIGGER -> 1s
	// dragonspire-store / 3s dragonspire-check loop (grid capture of
	// dreadweaver/summoner/veteran per hall rune, alive-check -> rune close +
	// HALL_RUNE_x DONE, all-7 -> DRAGONSPIRE_ROOM DONE + door open), and the
	// at_dragonspire_hall / at_blackrock_stadium /
	// at_nearby_scarshield_infiltrator AreaTriggers): every arm needs the
	// instance-script model — the Lua scripting surface has no
	// InstanceMapScript bridge, so this unit is documented-only.
	// lua_scripts/eastern_kingdoms/boss_felblood_kaelthas.lua.
	// Phase-one machine: fireball 44189 victim-cast 1ms init ->
	// 2s500ms loop; phoenix Talk(SAY_SUMMON_PHOENIX=5) + self-cast
	// 44194 12s init -> 45s loop. 50%-HP latch via OnDamageTaken(9)
	// (gyth convention): Talk(SAY_GRAVITY_LAPSE_1=2) + phase-one
	// timers cancelled (C++ phase gating stops their execution in
	// PHASE_TWO). Lethal-damage latch: Talk(SAY_DEATH=8) + outro
	// self-cast chain 48348/48349/48350/48350/3617. Flame-strike and
	// shock-barrier/pyroblast arms unmodeled (SelectTarget / heroic
	// gates — no bridges); phase-two gravity-lapse machinery
	// unmodeled (teleport/SpellHitTarget/summon bridges absent);
	// npc_felblood_kaelthas_phoenix class unmodeled (summon/instance/
	// zone-in-combat bridges absent); flame-strike AuraScript not
	// modeled. Script name is the stringified AI type (ScriptMgr.h:
	// 1234, RegisterCreatureAIWithFactory). Entry 24664 verifiable
	// from the C++ sources: magisters_terrace.h:50 names
	// BOSS_KAELTHAS_SUNSTRIDER = 24664 (boss_vaelastrasz precedent).
	RegisterLuaBoss("boss_felblood_kaelthas", 24664)
	// boss_selin_fireheart (Magister's Terrace): combat-entry
	// Talk(SAY_AGGRO) + fel-explosion self-cast 44314 2100ms->2s loop
	// (DoCastAOE = DoCast(nullptr) self-cast convention); KilledUnit
	// Talk(SAY_KILL=3) on OnTargetDied(3) with the C++ TYPEID_PLAYER
	// gate bridged by victim:GetObjectType() == "Player"; JustDied
	// Talk(SAY_DEATH=4) on OnDied(4). Sub-10%-mana drain machine
	// unmodeled (no creature GetPower bridge, no SelectTarget/
	// FindNearestCreature/MotionMaster/MovementInform bridges);
	// npc_fel_crystal class unmodeled (JustDied needs the instance
	// bridge — no .lua written, stub-free precedent). Entry 24723
	// verifiable from the C++ sources: magisters_terrace.h names
	// BOSS_SELIN_FIREHEART = 24723 (boss_felblood_kaelthas precedent).
	RegisterLuaBoss("boss_selin_fireheart", 24723)
	// boss_vexallus: BossAI combat entry + 15%-interval energy-discharge
	// latch (85/70/55/40/25%: Talk(SAY_ENERGY), Talk(EMOTE_DISCHARGE_
	// ENERGY), self-cast SPELL_SUMMON_PURE_ENERGY 44322) + 10%-HP overload
	// enrage (cancel timers, EVENT_OVERLOAD 1200ms -> 2s DoCastVictim
	// 44353 loop) + KilledUnit Talk(SAY_KILL). Chain-lightning /
	// arcane-shock event arms unmodeled (SelectTarget — no bridge);
	// JustSummoned MoveFollow unmodeled (SelectTarget — no bridge);
	// npc_pure_energy class unmodeled (entry not C++-verifiable —
	// no .lua written, queued for TDB entry evidence; stub-free
	// precedent). Entry 24744 verifiable from the C++ sources:
	// magisters_terrace.h names BOSS_VEXALLUS = 24744
	// (boss_felblood_kaelthas precedent).
	RegisterLuaBoss("boss_vexallus", 24744)
	// boss_priestess_delrissa: ScriptedAI combat entry Talk(SAY_AGGRO) +
	// self-cast flash-heal 17843 15s loop + renew 44174 10s->5s loop +
	// shield 44291 2s->7.5s loop + dispel-magic 27609 7.5s->12s loop +
	// KilledUnit player-gated PlayerDeath[PlayersKilled] talk (ids
	// 5..9, counter capped at 4, reset per engagement) + JustDied
	// Talk(SAY_DEATH). SW_PAIN 14032 machine unmodeled (SelectTarget —
	// no bridge); lackey summon/resummon + instance boss-state arms
	// unmodeled (no summon/instance bridges); the file's eight lackey
	// CreatureScript classes (kagani_nightstrike 24557, ellris_
	// duskhallow 24558, eramas_brightblaze 24554, yazzai 24561,
	// warlord_salaris 24559, garaxxas 24555, apoko 24553, zelfan
	// 24556) queued for later runs — their victim-cast loops are
	// bridgeable but ride on the instance + lackey-GUID + SelectTarget
	// bridges. Entry 24560 verifiable from the C++ sources:
	// magisters_terrace.h names BOSS_PRIESTESS_DELRISSA = 24560
	// (boss_felblood_kaelthas precedent).
	RegisterLuaBoss("boss_priestess_delrissa", 24560)
	// boss_eramas_brightblaze: ScriptedAI (via the file's lackey-common
	// base) knockdown 11428 6s loop + snap-kick 46182 4.5s loop
	// (DoCastVictim, GetVictim nil-guarded) + sub-25%-HP healing-potion
	// 15503 self-cast latch (OnDamageTaken(9), per-guid one-shot, reset
	// per engagement). Common-AI JustEngagedWith threat ring, JustDied
	// death-count, KilledUnit forward, AcquireGUIDs, Delrissa-respawn
	// Reset, and ResetThreatList machine unmodeled (no GUID-list /
	// instance / threat bridges). Entry 24554 verifiable from the C++
	// sources: boss_priestess_delrissa.cpp's m_auiAddEntries names
	// 24554 //Eramas Brightblaze (boss_felblood_kaelthas precedent).
	RegisterLuaBoss("boss_eramas_brightblaze", 24554)
	// boss_kagani_nightstrike: ScriptedAI (via the file's lackey-common
	// base) gouge 12540 5.5s loop + kick 27613 7s loop + eviscerate
	// 27611 6s->4s loop (DoCastVictim, GetVictim nil-guarded) +
	// sub-25%-HP healing-potion 15503 self-cast latch (OnDamageTaken(9),
	// per-guid one-shot, reset per engagement). The vanish 44290
	// machine (SelectTarget + ResetThreatList/AddThreat — no bridges)
	// and its InVanish backstab 15657 / kidney-shot 27615 leg
	// unmodeled; common-AI threat ring / death-count / KilledUnit
	// forward / AcquireGUIDs / Delrissa-respawn / ResetThreatList arms
	// unmodeled (no GUID-list / instance / threat bridges). Entry
	// 24557 verifiable from the C++ sources: boss_priestess_
	// delrissa.cpp's m_auiAddEntries names 24557 //Kagani Nightstrike
	// (boss_felblood_kaelthas precedent).
	RegisterLuaBoss("boss_kagani_nightstrike", 24557)
	// boss_ellris_duskhallow: ScriptedAI (via the file's lackey-common
	// base) immolate 44267 6s loop + shadow-bolt 12471 3s->5s loop
	// (DoCastVictim, GetVictim nil-guarded) + sub-25%-HP healing-potion
	// 15503 self-cast latch (OnDamageTaken(9), per-guid one-shot, reset
	// per engagement). The imp summon 44163 (JustEngagedWith self-cast
	// — PORTED 2026-10-24: the boss self-casts the spell and the engine
	// summons the imp; the imp's own AI legs stay out — imp entry is
	// spell-data-side, no C++ AI class) and the seed-of-corruption 44141
	// / curse-of-
	// agony 14875 / fear 38595 machines (SelectTarget-gated — no
	// bridge) unmodeled; common-AI threat ring / death-count /
	// KilledUnit forward / AcquireGUIDs / Delrissa-respawn /
	// ResetThreatList arms unmodeled (no GUID-list / instance / threat
	// bridges). Entry 24558 verifiable from the C++ sources: boss_
	// priestess_delrissa.cpp's m_auiAddEntries names 24558
	// //Elris Duskhallow (boss_felblood_kaelthas precedent).
	RegisterLuaBoss("boss_ellris_duskhallow", 24558)
	// boss_yazzai: ScriptedAI (via the file's lackey-common base)
	// frostbolt 15043 3s->8s loop + cone-of-cold 38384 10s loop +
	// ice-lance 46194 12s loop (DoCastVictim, GetVictim nil-guarded) +
	// sub-35%-HP ice-block 27619 self-cast one-shot latch + sub-25%-HP
	// healing-potion 15503 self-cast latch (both OnDamageTaken(9),
	// per-guid one-shot, reset per engagement). The polymorph 13323 /
	// blizzard 44178 machines (SelectTarget-gated — no bridge) and the
	// blink 14514 machine (combat-manager melee-range loop — no
	// bridge) unmodeled; common-AI threat ring / death-count /
	// KilledUnit forward / AcquireGUIDs / Delrissa-respawn /
	// ResetThreatList arms unmodeled (no GUID-list / instance / threat
	// bridges). Entry 24561 verifiable from the C++ sources: boss_
	// priestess_delrissa.cpp's m_auiAddEntries names 24561 //Yazzaj
	// (boss_felblood_kaelthas precedent).
	RegisterLuaBoss("boss_yazzai", 24561)
	// boss_warlord_salaris: ScriptedAI (via the file's lackey-common base)
	// battle-shout 27578 one-shot self-cast on JustEngagedWith (fired in
	// OnEnterCombat) + disarm 27581 6s loop + hamstring 27584 4.5s loop +
	// mortal-strike 44268 8s->4.5s loop (C++-exact re-arm) + piercing-howl
	// 23600 10s loop + frightening-shout 19134 18s loop (DoCastVictim,
	// GetVictim nil-guarded) + sub-25%-HP healing-potion 15503 self-cast
	// latch (OnDamageTaken(9), per-guid one-shot, reset per engagement).
	// The intercept 27577 machine (combat-manager melee-range loop +
	// SelectTarget-gated — no bridges) and the common-AI threat ring /
	// death-count / KilledUnit forward / AcquireGUIDs / Delrissa-respawn /
	// ResetThreatList arms unmodeled (no GUID-list / instance / threat
	// bridges). Entry 24559 verifiable from the C++ sources: boss_
	// priestess_delrissa.cpp's m_auiAddEntries names 24559 //Warlord
	// Salaris (boss_felblood_kaelthas precedent).
	RegisterLuaBoss("boss_warlord_salaris", 24559)
	// boss_garaxxas: ScriptedAI (via the file's lackey-common base)
	// aimed-shot 44271 6s loop + shoot 15620 2.5s loop + concussive-
	// shot 27634 8s loop + multi-shot 31942 10s loop (DoCastVictim,
	// GetVictim nil-guarded; C++ fires them only outside ATTACK_
	// DISTANCE, but no distance model exists on the Lua surface, so the
	// loops fire regardless of range) + sub-25%-HP healing-potion
	// 15503 self-cast latch (OnDamageTaken(9), per-guid one-shot, reset
	// per engagement). The wing-clip 44286 machine (ATTACK_DISTANCE
	// gate), the freezing-trap 44136 machine (ATTACK_DISTANCE gate +
	// gameobject lookup — no bridges), the sliver-pet NPC 24552 summon
	// (stays out: the Reset-time pet-existence check via ObjectAccessor::
	// GetUnit has no GUID-liveness bridge — an unconditional summon would
	// duplicate slivers every reset; the summon bridge itself is live),
	// and the common-AI threat ring / death-count /
	// KilledUnit forward / AcquireGUIDs / Delrissa-respawn /
	// ResetThreatList arms unmodeled (no GUID-list / instance / threat
	// bridges). Entry 24555 verifiable from the C++ sources: boss_
	// priestess_delrissa.cpp's m_auiAddEntries names 24555 //Garaxxas
	// (boss_felblood_kaelthas precedent).
	RegisterLuaBoss("boss_garaxxas", 24555)
	// boss_apoko: ScriptedAI (via the file's lackey-common base)
	// totem machine DoCast(me, RAND(27621, 44257, 15786)) 2000ms init ->
	// Totem_Amount*2000 loop (C++-exact escalation) + war-stomp 46026
	// 10s self-cast loop + frost-shock 21401 7s DoCastVictim loop +
	// lesser-healing-wave 44256 5s self-cast loop + sub-25%-HP
	// healing-potion 15503 self-cast latch (OnDamageTaken(9), per-guid
	// one-shot, reset per engagement). The purge 27626 machine gates on
	// SelectTarget(Random, 0) (no bridge), and the common-AI threat
	// ring / death-count / KilledUnit forward / AcquireGUIDs /
	// Delrissa-respawn / ResetThreatList arms unmodeled (no GUID-list /
	// instance / threat bridges). Entry 24553 verifiable from the C++
	// sources: boss_priestess_delrissa.cpp's m_auiAddEntries names
	// 24553 //Apoko (boss_felblood_kaelthas precedent).
	RegisterLuaBoss("boss_apoko", 24553)
	// boss_zelfan: ScriptedAI (via the file's lackey-common base)
	// goblin-dragon-gun 44272 20s init -> 10s DoCastVictim loop +
	// rocket-launch 44137 7s init -> 9s DoCastVictim loop + fel-iron-
	// bomb 46024 15s init -> 15s DoCastVictim loop (victim nil-guarded,
	// incarcerator convention) + high-explosive-sheep 44276 10s init ->
	// 65s self-cast loop + sub-25%-HP healing-potion 15503 self-cast
	// latch (OnDamageTaken(9), per-guid one-shot, reset per engagement).
	// The recombobulate 44274 machine rings over m_auiLackeyGUIDs via
	// ObjectAccessor with an IsPolymorphed gate (no GUID-list bridge),
	// and the common-AI threat ring / death-count / KilledUnit forward /
	// AcquireGUIDs / Delrissa-respawn / ResetThreatList arms unmodeled
	// (no GUID-list / instance / threat bridges). Entry 24556 verifiable
	// from the C++ sources: boss_priestess_delrissa.cpp's m_auiAddEntries
	// names 24556 //Zelfan (boss_felblood_kaelthas precedent).
	RegisterLuaBoss("boss_zelfan", 24556)
	// npc_kalecgos (Magister's Terrace zone script): the five-item
	// OnGossipHello/OnGossipSelect chain ("Who are you?" menu 12498 ->
	// 12500/12502/12606/12607/12608, C++-exact item texts/senders/actions;
	// gossip bridge per the nefarian precedent). No combat arms. The
	// PrepareQuestMenu leg (IsQuestGiver-gated) awaits the quest-menu
	// bridge; the MovementInform(POINT_ID_PREPARE_LANDING) landing leg +
	// the EVENT_KALECGOS_LANDING arm (DoCastAOE 44762 + SetObjectScale
	// 0.6f + 1s re-arm) + the EVENT_KALECGOS_TRANSFORM cast triplet
	// (triggered self-cast 46307 / 24085 / 44670) + UpdateEntry(24848)
	// are documented-only (no MovementInform/MotionMaster/entry-update
	// bridges). Entry 24844 verifiable from the C++ sources:
	// magisters_terrace.h:64 names NPC_KALECGOS = 24844
	// (boss_felblood_kaelthas precedent).
	RegisterLuaBoss("npc_kalecgos", 24844)
	// Magister's Terrace instance script (instance_magisters_terrace.cpp,
	// map 585) — InstanceMapScript only: 4 boss + 2 Kalecgos + 1 GO data
	// binds (EncounterCount 4, DataHeader "MT"), Delrissa death-count
	// SetData/GetData (SPECIAL ++, IN_PROGRESS reset on Delrissa
	// engage), Kael'thas pre-trash GUID set (6 trash entries within
	// 10yd of {150, 141, -14.4} -> kaelthas SetData INTRO on empty),
	// 6 door-data rows (passage/room), escape-orb 188173 unflag on
	// Kael'thas DONE, EVENT_SPAWN_KALECGOS 1min summon + flight path
	// 248440. Zero Lua InstanceMapScript bridge (karazhan / BRD /
	// blackrock-spire / ulduar precedents) — documented-only.
	RegisterLuaBoss("npc_storm_cloud", 29939)
	// npc_unworthy_initiate (The Scarlet Enclave, Acherus DK intro):
	// the class's PHASE_ATTACKING combat machine (icy touch 52372 /
	// plague strike 52373 / blood strike 52374 / death coil 52375
	// DoCastVictim loops) is ported in lua_scripts/eastern_kingdoms/
	// npc_unworthy_initiate.lua; the whole pre-combat phase state
	// machine (anchor GUID-passing, soul-prison gossip EventStart,
	// MovePoint/MovementInform leg, faction/react-state AttackStart
	// leg) is documented-only — no MotionMaster/MovementInform/
	// gameobject-gossip/faction bridges (selin_fireheart / razorgore
	// precedents). All five entries verifiable from the C++ sources:
	// chapter1.cpp's own acherus_unworthy_initiate[5] array (lines
	// 89-96) names 29519, 29520, 29565, 29566, 29567
	// (boss_felblood_kaelthas precedent).
	RegisterLuaBoss("npc_unworthy_initiate", 29519)
	RegisterLuaBoss("npc_unworthy_initiate", 29520)
	RegisterLuaBoss("npc_unworthy_initiate", 29565)
	RegisterLuaBoss("npc_unworthy_initiate", 29566)
	RegisterLuaBoss("npc_unworthy_initiate", 29567)
	// AddSC_the_scarlet_enclave_c1 (chapter1.cpp, EK loader decl :95 /
	// call :273): full-file audit 2026-10-07. Of the 14 scripts only
	// npc_unworthy_initiate is ported/registered (above). The remaining
	// 13 have zero Lua bridges — documented-only: npc_unworthy_initiate_
	// anchor (PassiveAI SetGUID/GetGUID prisoner store), go_acherus_
	// soul_prison (GO gossip -> EventStart; no gameobject-gossip bridge),
	// spell_death_knight_initiate_visual (display-ID 25354-25373 ->
	// spell 51520-51552 equip map; no SpellScript bridge), npc_eye_of_
	// acherus (charmer lookup / RemoveAurasDueToSpell / flight spline;
	// no charmer/movement bridges), npc_death_knight_initiate (duel
	// machine: SpellHit 52996 accept, DamageTaken clamp-to-1 victory
	// arms, gossip duel menu; no damage-hook / player-self-cast / gossip
	// bridges), npc_dark_rider_of_acherus (TempSummon summoner chain,
	// MoveChase, DespawnOrUnsummon; no summon/movement bridges), npc_
	// salanar_the_horseman (gossip 9739 realm-of-shadows cast, MoveInLineOf
	// Sight vehicle machine; no gossip/vehicle bridges), spell_stable_
	// master_repo (AuraScript; no bridge), spell_deliver_stolen_horse
	// (SpellScript; no bridge), npc_ros_dark_rider (EnterVehicle /
	// deathcharger faction flip; no vehicle bridge), npc_dkc1_gothik
	// (MoveInLineOfSight ghoul-collect; no proximity bridge), npc_scarlet_
	// ghoul (owner/minion machines; no minion bridges), spell_gift_of_
	// the_harvester (SpellScript; no bridge). No new registrations,
	// no Lua files.
	// AddSC_the_scarlet_enclave_c2 (chapter2.cpp, EK loader decl :96 /
	// call :274): full-file audit 2026-10-07. npc_koltira_deathweaver
	// (entry 28912, chapter2.cpp :90 BloodyBreakout enum — quest-12727
	// Bloody Breakout intro/outro event machine) is ported in
	// lua_scripts/eastern_kingdoms/npc_koltira_deathweaver.lua:
	// OnQuestAccept(12727) -> 500ms Talk(SAY_KOLTIRA_0 0) -> 5s
	// Talk(SAY_KOLTIRA_1 1) (C++-exact EVENT_INTRO_0/1 legs);
	// OnGossipHello TEXT_ID_EVENT 13425 branch (stranded on the
	// unbridged EVENT_INTRO_6, kept faithful); onReset clears timers
	// + event-gossip state. Unmodeled (no bridges): MoveJump/
	// MovePoint/MovePath/MovementInform legs (selin_fireheart /
	// kalecgos precedents), SummonCreatureGroup waves + valroth
	// (dark-rider precedent), FakeValrothTalk (no nearest-creature
	// bridge), EVENT_CHECK_PLAYER player-GUID / FailQuest (no
	// ObjectAccessor / quest-status bridges), stand-state / flag /
	// aura arms. npc_scarlet_courier (entry 29076, chapter2.cpp :336
	// ScarletCourierEnum) is ported: JustEngagedWith -> Talk(SAY_TREE2
	// 1) on event 1 (Dismount arm has no bridge — uiStage stops with
	// the unbridged machine); the mounted inconspicuous-tree stage
	// machine + MovementInform + AttackStart legs are documented-only
	// (no MotionMaster / gameobject-contact / owner bridges).
	// npc_a_special_surprise (10 execution-scene entries 29032/29061/
	// 29065/29067/29068/29070/29074/29072/29073/29071, script "signed
	// for 29032") is documented-only: the quest-status MoveInLineOfSight
	// trigger + 12-step Talk machine (stand-state / SetImmuneToPC /
	// setDeathState / Plaguefist Talk) have no bridges. spell_death_
	// knight_devour_humanoid (SpellScript OnEffectHitTarget: GetHitUnit
	// ->CastSpell(GetCaster(), GetEffectValue()), true) is documented-
	// only — no SpellScript bridge (nightbane precedent).
	RegisterLuaBoss("npc_koltira_deathweaver", 28912)
	RegisterLuaBoss("npc_scarlet_courier", 29076)
	// AddSC_the_scarlet_enclave_c5 (chapter5.cpp, EK loader decl :97 /
	// call :275): full-file audit 2026-10-07 — 3 scripts, zero Lua
	// bridges, documented-only. npc_highlord_darion_mograine (entry
	// 29173, chapter5.cpp :138) is an EscortAI event machine (Light of
	// Dawn, quest 12801): OnGossipHello offers "I am ready." while
	// quest 12801 incomplete; OnGossipSelect sets uiStep=1 + Start()
	// escort; the WaypointReached step machine summons dawn/scourge
	// waves (29219/29186/29206/29190 defenders 29174 earthshatter
	// 29182), GUID stores via ObjectAccessor, Talks, world states
	// 3592/3605, Mount 25279, SetStandState, SetVisible, emote states,
	// gossip-flag clear, GetClosestCreatureWithEntry, KillSelf; combat
	// arms 52893/53639/53635/49723/53640 inside the same UpdateAI.
	// Every arm unbridged (EscortAI/waypoint/summon/GUID-store/world-
	// state/stand-state/movement/flag bridges all absent — npc_barnes
	// precedent). npc_the_lich_king_tirion_dawn (entry 29183, :161) is
	// a trivially empty ScriptedAI (all overrides no-op) — nothing to
	// port. spell_teleport_leaders_blessing (58418/58420 portals:
	// OnEffectHitTarget GetHitPlayer + EFFECT_0/1 CalcValue quest-
	// complete -> CastSpell) is documented-only — no SpellScript
	// bridge (nightbane precedent).
	// npc_valkyr_battle_maiden (The Scarlet Enclave zone file,
	// zone_the_scarlet_enclave.cpp — AddSC_the_scarlet_enclave, loader
	// decl :94 / call :272): PassiveAI revive-valkyr driven by an
	// internal FlyBackTimer phase machine. Phase 0: emote
	// FLYGRABCLOSED (500ms re-arm). Phase 1: GetClosePoint onto the
	// summoner player + MovePoint(0), SetTarget(player), SetVisible
	// (4500ms re-arm). Phase 2: if the player has not requested
	// resurrection yet, emote CUSTOM_SPELL_01 + triggered cast
	// SPELL_REVIVE 51918 on the player + whisper Talk(0) (5000ms
	// re-arm). Phase 3: re-hide (3000ms). Phase 4: DisappearAndDie.
	// Reset repositions/flies/hides the valkyr and clears the
	// MotionMaster. Every arm is unbridged on the Lua surface: player
	// acquisition needs the summoner chain (IsSummon / ToTempSummon /
	// GetSummonerUnit / ToPlayer — no summoner bridge, midnight
	// precedent); movement (MotionMaster Clear/MovePoint,
	// GetClosePoint, UpdatePosition), visibility, SetTarget, emote,
	// SetFarVisible/setActive have no bridges (SetCanFly IS bridged —
	// creature:SetCanFly, lua_creature_events.go — but the whole AI is
	// unported, so nothing here is wired); Talk(0) is a
	// player-targeted whisper (broadcast Talk has no whisper target,
	// hiborne-lamenter precedent); IsResurrectRequested and
	// DisappearAndDie are unbridged. Entry is not C++-verifiable
	// (DB-side ScriptName binding only) — documented-only,
	// unregistered, zero Lua file.
	// npc_koltira_deathweaver (The Scarlet Enclave, Bloody Breakout
	// quest-12727 intro): OnQuestAccept(12727) -> 500ms Talk(0) ->
	// 5s Talk(1) intro legs + OnGossipHello 13425 event-menu arm
	// (event-gossip state only settable by the stranded MovementInform
	// chain) are ported in lua_scripts/eastern_kingdoms/
	// npc_koltira_deathweaver.lua; the MoveJump/MovePoint intro legs,
	// the MovementInform-triggered wave/valroth/outro machine,
	// EVENT_CHECK_PLAYER, summon groups and all flag/stand-state/
	// aura bridges are documented-only (no MotionMaster/
	// MovementInform/summon/despawn/quest-status bridges). Entry
	// 28912 verifiable from the C++ sources: chapter2.cpp's own
	// BloodyBreakout enum (line 90) names NPC_KOLTIRA = 28912
	// (boss_felblood_kaelthas precedent).
	RegisterLuaBoss("npc_koltira_deathweaver", 28912)

	// npc_scarlet_courier (The Scarlet Enclave, chapter2.cpp lines
	// 340-428): JustEngagedWith's Talk(SAY_TREE2 = 1) arm is ported
	// via OnEnterCombat(1) in lua_scripts/eastern_kingdoms/
	// npc_scarlet_courier.lua (the Dismount leg has no bridge and is
	// a documented deviation); the out-of-combat stage machine
	// (FindNearestGameObject(191144)/SetWalk/GetContactPoint/
	// MovePoint/MovementInform/tree->GetOwner()/AttackStart), Reset's
	// Mount(14338) and the unused SPELL_SHOOT (52818) are
	// documented-only. Entry 29076 verifiable from the C++ sources:
	// chapter2.cpp's own ScarletCourierEnum (line 336) names
	// NPC_SCARLET_COURIER = 29076.
	RegisterLuaBoss("npc_scarlet_courier", 29076)
	// deadmines.cpp zone script: single ItemScript item_defias_gunpowder
	// (AddSC_deadmines). OnUse(player, item, targets): null instance ->
	// SendNotification("Instance script not initialized"), return true;
	// GetData(EVENT_STATE = 7) != CANNON_NOT_USED (0) -> return false;
	// GO target entry == GO_DEFIAS_CANNON (16398) ->
	// SetData(EVENT_STATE, CANNON_GUNPOWDER_USED (1)); DestroyItemCount 1,
	// return true (deadmines.h verified). Zero Lua ItemScript/OnUse
	// bridge — documented-only (denveous_stuff precedent).
	// instance_deadmines (AddSC_instance_deadmines, map 36 — document-only audit,
	// commit follows): InstanceMapScript with encounter doors (doorData: Rhahk'Zor,
	// Sneed, Gilnid passages) + cannon event machine driven by EVENT_STATE (7).
	// OnUnitDeath: Rhahk'Zor/Sneed/Gilnid -> SetBossState DONE. SetData(EVENT_STATE)
	// latches only when cannon + ironclad door exist. Update state machine:
	// CANNON_GUNPOWDER_USED (1) -> 3s blast timer -> SummonCreatures (2x entry 657
	// Defias Pirate, corpse-timed despawn 3s), ShootCannon (GO active + sound 1400),
	// BlastOutDoor (GO destroyed + sound 3079), lever stuck (flags=4), Mr. Smite
	// Talk(SAY_ALARM1) -> PIRATES_ATTACK (1s: pirates move to -102.7,-655.9)
	// -> SMITE_ALARMED (5s: Talk(SAY_ALARM2)) -> EVENT_DONE. GetGuidData:
	// DATA_SMITE_CHEST. Zero Lua InstanceMapScript bridge — documented-only
	// (instance_blackrock_spire / instance_blackwing_lair precedent).
	// gnomeregan.cpp zone scripts (AddSC_gnomeregan — document-only audit):
	// npc_blastmaster_emi_shortfuse (EscortAI escort event machine),
	// boss_grubbis (ScriptedAI, summoner-SetData chain arm), and
	// spell_collecting_fallout (SpellScript: 25% roll_chance_i on EFFECT_0
	// launch, fail-gate on EFFECT_1). Zero Lua/Go bearer for EscortAI,
	// summoner-SetData, or SpellScript — documented-only.
	// AddSC_instance_gnomeregan (instance_gnomeregan.cpp — document-only audit):
	// InstanceMapScript only (map 90, "GNO"), zero creature/at_ scripts.
	// Arms: OnCreatureCreate latches Blastmaster Emi Shortfuse GUID (entry
	// 7998); OnGameObjectCreate latches the two cave-in GOs (146085/146086);
	// OnUnitDeath -> SetBossState DONE for DATA_VICIOUS_FALLOUT (1),
	// DATA_ELECTROCUTIONER (2), DATA_CROWD_PUMMELER (3), DATA_THERMAPLUGG (4);
	// GetGuidData serves the three latched GUIDs. Zero Lua InstanceMapScript
	// bridge (instance_deadmines / instance_blackwing_lair precedent).
	// boss_mr_smite (Deadmines, boss_mr_smite.cpp — ScriptedAI via
	// GetDeadminesAI; the first unit of the Deadmines block): the combat
	// machine (Trash 3391 self-cast {5,9}s->{6,15.5}s, Smite Slam 6435
	// victim-cast 9s->11s, both with the bCheckChances 15%-skip gate)
	// plus the phase-transition latch (stomp 6432 + Talk(2) at the 66%
	// crossing, stomp + Talk(3) at the 33% crossing — balinda
	// DamageTaken-latch convention: the OnDamageTaken(9) handler fires
	// with a fresh live-motion creature object, so (health - damage) is
	// the genuine post-hit percent) in lua_scripts/eastern_kingdoms/
	// boss_mr_smite.lua — maiden convention (CreateLuaEvent timers;
	// GetVictim for DoCastVictim; no UNIT_STATE_CASTING gate). The
	// !uiIsMoving ability-halt leg (SetCombatMovement/AttackStop/
	// InterruptNonMeleeSpells) is bridged (no-op twin) and ported in the stage arms;
	// REACT_PASSIVE stays deliberately omitted (see the Lua header), so the
	// timers keep firing through crossings (documented deviation).
	// The phase/equip event machine (instance chest DATA_SMITE_CHEST /
	// GO 144111 via ObjectAccessor, MotionMaster MovePoint, equip-swap
	// 5191/5196/7230, stand-state, MoveChase) is documented-only;
	// Reset's equipment/stand-state/react-state/NoCallAssistance arms
	// are ported (see the Lua file's onSpawn/onReset).
	// Entry 646 verifiable from the C++ sources: deadmines.h's own
	// DMCreaturesIds enum names NPC_MR_SMITE = 646.
	RegisterLuaBoss("boss_mr_smite", 646)

	// boss_vancleef (Deadmines): JustEngagedWith's Talk(SAY_AGGRO) ->
	// creature:Talk(0) on OnEnterCombat; KilledUnit's TYPEID_PLAYER-gated
	// Talk(SAY_KILL) -> OnTargetDied(3) with victim:IsPlayer(); DamageTaken
	// health latches on OnDamageTaken(9): 50% -> Talk(SAY_SUMMON) +
	// DoCastSelf(SPELL_VANCLEEFS_ALLIES 5200) ~ CastSpell(self, 5200) (the
	// summon leg can't materialize — no summon bridge, documented
	// deviation), 66% -> Talk(SAY_ONE), 33% -> Talk(SAY_TWO), 25% ->
	// Talk(SAY_THREE) (C++-exact: 50% in its own if, 25/33/66 in an
	// else-if chain; per-guid one-shot latches). Reset's DoCastSelf(674/
	// 12787) are stranded halves (no single faithful trigger — C++ Reset
	// also runs at spawn Initialize), SummonBlackguards()' DoSummon(
	// NPC_BLACKGUARD 636 x2) and EnterEvadeMode's summons.DespawnAll() are
	// documented-only (no summon bridge — dark-rider precedent).
	// Entry 639 verifiable from the C++ sources: deadmines.h's own
	// DMCreaturesIds enum names NPC_VANCLEEF = 639.
	RegisterLuaBoss("boss_vancleef", 639)

	// boss_headless_horseman (Scarlet Monastery): JustEngagedWith's
	// EVENT_HORSEMAN_CLEAVE -> CreateLuaEvent timer armed on OnEnterCombat
	// (1), cancelled on 2/4/23 (13s init, Repeat(6s, 12s) DoCastVictim
	// SPELL_HEADLESS_HORSEMAN_CLEAVE 42587 ~ GetVictim + CastSpell);
	// KilledUnit's TYPEID_PLAYER-gated Talk(SAY_KILL_PLAYER 5) ->
	// OnTargetDied(3) with victim:IsPlayer(); JustDied's DoCastSelf(
	// SPELL_HEADLESS_HORSEMAN_BURNING_COSMETIC 42971) + Talk(SAY_DEATH 4)
	// -> OnDied(4). The DamageTaken cheat-death phase latch (damage = 0
	// + StartPhase) is documented-only: its headless-phase machine (head
	// reposition/send/return spells 42410/42399/42401, cross-creature
	// DoActions, MovementInform flight path) has no bridges, so porting
	// the negation alone would make the boss unkillable; the cleave
	// therefore runs the phase-1 cadence flat (C++ re-arms at 16s/9s in
	// phases 2/3, unmodeled). EVENT_RANDOM_LAUGH is ported via creature:PlayDirectSound
	// (DoPlaySoundToSet); EVENT_CONFLAGRATE has no random-target bridge;
	// EVENT_SUMMON_PUMPKIN is phase-pinned with no summon bridge; the
	// instance GetCreature/SetData legs (DATA_HORSEMAN_HEAD,
	// DATA_HORSEMAN_EVENT_STATE), MotionMaster/MovementInform legs,
	// Reset/JustAppeared self-casts (stranded halves), and the
	// head/pumpkin/flame-bunny/sir-thomas/GO scripts (driven by
	// SpellScript/AuraScript/SpellHit/DoAction arms with no bridges) are
	// documented-only in the .lua header. Entry 23682 verifiable from the
	// C++ sources: scarlet_monastery.h's own SMCreatureIds enum names
	// NPC_HEADLESS_HORSEMAN = 23682.
	RegisterLuaBoss("boss_headless_horseman", 23682)

	// boss_herod (Scarlet Monastery, Armory wing, EK loader decl :102 /
	// call :280): OnEnterCombat(1) talks SAY_AGGRO 0, non-triggered
	// self-casts rushing charge 8260 (DoCast(me, ...) ->
	// creature:CastSpell(creature, ...)), arms cleave 15496 12s->12s and
	// whirlwind 8989 60s->30s (both non-triggered DoCastVictim ->
	// GetVictim + CastSpell, nil-victim ticks keep the schedule —
	// jeklik convention); whirlwind casts also fire Talk(SAY_WHIRLWIND
	// 1). KilledUnit's TYPEID_PLAYER-gated Talk(SAY_KILL 3) ->
	// OnTargetDied(3) with victim:IsPlayer() (illidan convention). The
	// DamageTaken enrage latch fires once when post-damage health drops
	// strictly below 30% (HealthBelowPctDamaged(30, damage) — the
	// thalnos/azshir damaged class, not the golemagg 704fb8e bug class):
	// Talk(EMOTE_ENRAGE 4) + Talk(SAY_ENRAGE 2) + non-triggered
	// self-cast frenzy 8269, modeled on the pre-damage hook (event 9)
	// with a per-GUID once-guard cleared on 2/4/23. JustDied's 20x
	// SummonCreature(NPC_SCARLET_TRAINEE 6575, timed-or-dead 10min
	// around 1939.18/-431.58/17.09) and the npc_scarlet_trainee EscortAI
	// (urand 1-6s start -> Start(true, true)) are documented-only — no
	// summon or EscortAI bridges (nightbane/npc_barnes precedent).
	// Entry 3975 has no NPC_ constant in the C++ tree —
	// RegisterScarletMonasteryCreatureAI binds the ScriptName DB-side
	// (entry wowhead-cited: www.wowhead.com/cata/npc=3975/herod).
	RegisterLuaBoss("boss_herod", 3975)

	// boss_high_inquisitor_fairbanks (Scarlet Monastery, Cathedral wing,
	// EK loader decl :103 / call :281): OnEnterCombat(1) arms curse of
	// blood 8282 10s->25s (non-triggered DoCastVictim -> GetVictim +
	// CastSpell, nil-victim ticks keep the schedule — jeklik
	// convention), dispel magic 15090 30s->30s, fear 12096 40s->40s,
	// and sleep 8399 25s->30s. C++ has zero Talk lines. The dispel
	// arm's custom target selector (TYPEID_PLAYER within 30 yd WITH a
	// dispellable DISPEL_MAGIC aura) loses the aura leg — no aura-list
	// bridge (shazzrah curse precedent) — and casts on a random alive
	// player within 30 yd, skipping the cast when none is in range
	// (C++ DoCast is conditional; the 30s repeat is unconditional).
	// Fear follows SelectTarget(Random, 1, 20.f, true): random alive
	// player within 20 yd excluding the victim (majordomo
	// victim-excluding + opera in-range convention), nil pick casts
	// nothing, 40s re-arm unconditional. Sleep's MaxThreat pick is the
	// current victim (jeklik convention). The DamageTaken latch fires
	// when post-damage health drops strictly below 25%
	// (HealthBelowPctDamaged(25, damage) — the thalnos/azshir damaged
	// class, not the golemagg 704fb8e bug class): a per-GUID once-guard
	// self-casts power word: shield 11647, then heal 12039 self-cast
	// under a 30s per-GUID cooldown (C++ _healTimer Reset(0s) in Reset,
	// so the first dip heals immediately), modeled on the pre-damage
	// hook (event 9) with the per-GUID state cleared on 2/4/23; the
	// !IsNonMeleeSpellCast(false) heal gate has no casting-state
	// bridge. SetStandState(DEAD/STAND) (the secret-chamber corpse
	// visual) is documented-only — no stand-state bridge (npc_barnes
	// precedent); BossAI::JustEngagedWith and the
	// DATA_HIGH_INQUISITOR_FAIRBANKS bookkeeping have no
	// instance-script bridge (luaBossAI shim). Entry 4542 has no NPC_
	// constant in the C++ tree — RegisterScarletMonasteryCreatureAI
	// binds the ScriptName DB-side (entry wowhead-cited:
	// www.wowhead.com/classic/npc=4542/high-inquisitor-fairbanks;
	// the adjacent 3974 was disproven before porting).
	RegisterLuaBoss("boss_high_inquisitor_fairbanks", 4542)

	// boss_houndmaster_loksey (3974, Scarlet Monastery Library, EK
	// loader decl :104 / call :282 follows fairbanks): OnEnterCombat(1)
	// Talk(SAY_AGGRO 0) + non-triggered self-cast summon scarlet hound
	// 17164 (DoCast(spellId) self-cast) + bloodlust arm 20s->(60s on
	// cast, 1s retry until below-60%): C++ EVENT_BLOODLUST casts
	// DoCastSelf(bloodlust 6742) only while HealthBelowPct(60) holds
	// (current-health read, golemagg 704fb8e bug class) and otherwise
	// re-arms at 1s. The UpdateAI UNIT_STATE_CASTING queue gate and
	// BossAI::JustEngagedWith + DATA_HOUNDMASTER_LOKSEY bookkeeping
	// have no UNIT_STATE/instance-script bridges (luaBossAI shim).
	// Entry 3974 is not named in the C++ tree
	// (RegisterScarletMonasteryCreatureAI binds the creature_template
	// ScriptName DB-side) — db.moonwell-cited (?npc=3974
	// "Houndmaster Loksey").
	RegisterLuaBoss("boss_houndmaster_loksey", 3974)

	// boss_interrogator_vishas (3983, Scarlet Monastery Graveyard, EK
	// loader decl :105 / call :283 follows loksey): OnEnterCombat(1)
	// Talk(SAY_AGGRO 0) + shadow word: pain 2767 armed at 5s->({5s,15s})
	// (non-triggered DoCastVictim -> GetVictim + CastSpell, nil-victim
	// ticks keep the schedule — jeklik convention). KilledUnit's
	// TYPEID_PLAYER-gated Talk(SAY_KILL 3) -> OnTargetDied(3) with
	// victim:IsPlayer() (illidan convention). The DamageTaken latches
	// use the C++ _yellCount staircase on HealthBelowPctDamaged
	// semantics (post-damage, strictly below — the thalnos/azshir
	// damaged class): below 60% with count < 1 -> Talk(SAY_HEALTH1 1),
	// then below 30% with count < 2 -> Talk(SAY_HEALTH2 2); both can
	// fire on one damage packet, so the port keeps a per-GUID integer
	// counter on the pre-damage hook (event 9), cleared on 2/4/23.
	// JustDied's Vorrel Sengutz Talk(SAY_TRIGGER_VORREL 0) via
	// instance->GetCreature(DATA_VORREL 3981) has no instance-creature
	// bridge (documented-only); BossAI::JustEngagedWith/_Reset/_JustDied
	// and DATA_INTERROGATOR_VISHAS=0 bookkeeping have no
	// instance-script bridge (luaBossAI shim). Entry 3983 has no NPC_
	// constant in the C++ tree — RegisterScarletMonasteryCreatureAI
	// binds the creature_template ScriptName DB-side
	// (entry classicdb/wowhead-cited: classicdb.ch/?npc=3983,
	// www.wowhead.com/classic/npc=3983/interrogator-vishas).
	RegisterLuaBoss("boss_interrogator_vishas", 3983)

	// boss_scorn (14693, Scarlet Monastery Graveyard, Scourge Invasion,
	// EK loader decl :106 / call :284 follows loksey): OnEnterCombat(1)
	// arms lich slap 28873 at 45s->45s, frostbolt volley 8398 at
	// 30s->20s, mind flay 17313 at 30s->20s, frost nova 15531 at
	// 30s->15s (all non-triggered DoCastVictim -> GetVictim + CastSpell,
	// jeklik convention, nil-victim ticks keep the schedule, no reschedule
	// jitter). Zero Talk lines in the C++ (no SAY enum); the UpdateAI
	// UNIT_STATE_CASTING queue gate and BossAI::JustEngagedWith +
	// DATA_SCORN bookkeeping have no UNIT_STATE/instance-script bridges
	// (luaBossAI shim). Entry 14693 is not named in the C++ tree
	// (RegisterScarletMonasteryCreatureAI binds the creature_template
	// ScriptName DB-side) — db.moonwell-cited (?npc=14693 "Scorn",
	// level 34 elite undead, spawned via script).
	RegisterLuaBoss("boss_scorn", 14693)

	// boss_mograine_and_whitemane (3976 Scarlet Commander Mograine + 3977
	// High Inquisitor Whitemane, Scarlet Monastery Cathedral finale, EK
	// loader decl :108 / call :286 follows instance_scarlet_monastery;
	// entries C++-named at scarlet_monastery.h:66-67): OnEnterCombat(1)
	// Mograine Talk(SAY_MO_AGGRO 0) + crusader strike 14518 at
	// {10s,15s}->10s + hammer of justice 5589 at {10s,15s}->60s; Whitemane
	// Talk(SAY_WH_INTRO 0) + 5s scheduler gate -> heal 12039 10s->13s
	// (self-cast only while own health strictly below 75% — the Mograine
	// target leg needs instance GetCreature(DATA_MOGRAINE), no bridge) +
	// power word: shield 22187 15s->15s + holy smite 9481 6s->6s (all
	// non-triggered DoCastVictim/DoCastSelf -> GetVictim/self + CastSpell,
	// nil-victim keeps schedule, jeklik convention). Both bosses:
	// KilledUnit(3) player-gated Talk(SAY_KILL 1) with the 5s _killYellTimer
	// (herod victim:IsPlayer() convention; flag cleared only on event 23,
	// C++ _Reset() never touches it). OnReset(23) cancels timers + clears
	// the yell flag + self-casts retribution aura 8990 (Lua CastSpell has
	// no triggered leg, documented). Documented-unmodeled: Mograine's
	// fake-death DamageTaken machine (door open, GetCreature(DATA_WHITEMANE)
	// MovePoint + DoZoneInCombat, flags/stand-state/react/motion legs, the
	// killing-damage negation while !_canDie, and the SpellHit(SCARLET_
	// RESURRECTION 9232) 3s/5s resurrect scheduler legs — no instance/
	// flag/stand-state/react/MotionMaster/SpellHit bridges; without the
	// negation the boss dies on the first lethal hit, headless-horseman
	// cheat-death precedent) and Whitemane's sub-50%-damaged latch (cancel
	// events, DoCastAOE deep sleep 9256, MovePosition/MovePoint to Mograine,
	// MovementInform 3s -> DoCast(SCARLET_RESURRECTION), SpellHitTarget ->
	// Talk(SAY_WH_RESURRECT 2) + reschedule + _canDie=true + MoveChase —
	// no DoCastAOE/MotionMaster/MovementInform/SpellHitTarget bridges; she
	// stays killable below 50%, documented divergence). Dominate mind 14515
	// is declared in the C++ spell enum but never cast. UpdateAI
	// UNIT_STATE_CASTING gates + BossAI::JustEngagedWith +
	// DATA_MOGRAINE_AND_WHITE_EVENT bookkeeping have no bridges (luaBossAI
	// shim).
	RegisterLuaBoss("boss_mograine_and_whitemane", 3976)
	RegisterLuaBoss("boss_whitemane", 3977)
	// boss_darkmaster_gandling.cpp (EK loader decl :109 / call :287,
	// Scholomance block start): boss AI ported in
	// lua_scripts/eastern_kingdoms/boss_darkmaster_gandling.lua (entry
	// NPC_DARKMASTER_GANDLING=1853, scholomance.h:42): OnEnterCombat
	// schedules arcane missiles 15790 (triggered victim, 4.5s->8s),
	// shadow shield 12040 (self, 12s->{14s,28s}), curse 18702 (triggered
	// victim, 2s->{15s,27s}), shadow portal 17950 (triggered random
	// alive player within 100yd, 15s->{17s,27s}, only while current
	// health strictly above 3% — the C++ re-schedule lives inside the
	// gate, so below 3% the event dies); nil-target casts nothing,
	// schedule kept. Documented-only (no bridges): GO_GATE_GANDLING
	// (177374) Reset/JustDied/JustEngagedWith state legs,
	// IsSummonedBy Talk(0)+MoveRandom(5), spell_shadow_portal (room
	// portal picker, 6 tries among open gates) + spell_shadow_portal_rooms
	// (3x RISEN_GUARDIAN 11598 summons per room + gate close),
	// UNIT_STATE_CASTING gates, BossAI::JustEngagedWith +
	// DATA_DARKMASTERGANDLING bookkeeping (luaBossAI shim).
	RegisterLuaBoss("boss_darkmaster_gandling", 1853)
	// boss_death_knight_darkreaver.cpp (EK loader decl :110 / call :288,
	// Scholomance block continues): boss AI ported in
	// lua_scripts/eastern_kingdoms/boss_death_knight_darkreaver.lua
	// (entry 14516 — no NPC_ constant in scholomance.h, DB-side
	// ScriptName binding, wowhead TBC-cited): the only arm is DamageTaken —
	// a lethal blow (current health <= incoming damage, pre-damage hook
	// event 9, NOT the golemagg HealthBelowPct bug class) triggers a
	// self-cast of 23261 (Summon Darkreaver's Fallen Charger). C++
	// Reset() and JustEngagedWith() are both empty; no timers, no Talk
	// lines, no instance bookkeeping (luaBossAI shim).
	RegisterLuaBoss("boss_death_knight_darkreaver", 14516)
	// boss_doctor_theolen_krastinov.cpp (EK loader decl :111 / call :289,
	// Scholomance block continues; script name
	// "boss_doctor_theolen_krastinov", AddSC_boss_theolenkrastinov): boss
	// AI ported in
	// lua_scripts/eastern_kingdoms/boss_doctor_theolen_krastinov.lua
	// (entry 11261 — no NPC_ constant in scholomance.h, DB-side
	// ScriptName binding, classic wowhead-verified): OnEnterCombat
	// schedules rend 16509 (triggered victim, 8s->10s), backhand 18103
	// (triggered victim, 9s->10s), frenzy 8269 (triggered self +
	// Talk(EMOTE_FRENZY_KILL 0), 1s->120s); nil-victim ticks cast nothing
	// but keep the schedule. Documented-only (no bridges): UpdateAI
	// UNIT_STATE_CASTING gates, BossAI::JustEngagedWith +
	// DATA_DOCTORTHEOLENKRASTINOV=0 bookkeeping (scholomance.h:30)
	// (luaBossAI shim).
	RegisterLuaBoss("boss_doctor_theolen_krastinov", 11261)
	// boss_illucia_barov.cpp (EK loader AddSC_boss_illuciabarov
	// decl :112 / call :290, Scholomance block continues; script
	// name "boss_illucia_barov"): boss AI ported in
	// lua_scripts/eastern_kingdoms/boss_illucia_barov.lua
	// (entry 10502 — no NPC_ constant in scholomance.h, DB-side
	// ScriptName binding, wowhead TBC-verified): OnEnterCombat
	// schedules curse of agony 18671 (triggered victim, 18s->30s),
	// shadow shock 17234 (triggered, random alive player within
	// 100yd, 9s->12s), silence 12528 (triggered victim, 5s->14s),
	// fear 12542 (triggered victim, 30s->30s); nil picks keep the
	// schedule. SPELL_DOMINATE 7645 marked UNUSED in C++ (never
	// cast). Documented-only (no bridges): UpdateAI
	// UNIT_STATE_CASTING gates, BossAI::JustEngagedWith +
	// DATA_LADYILLUCIABAROV=2 bookkeeping (scholomance.h:32)
	// (luaBossAI shim).
	RegisterLuaBoss("boss_illucia_barov", 10502)
	// boss_instructor_malicia.cpp (EK loader AddSC_boss_instructormalicia
	// decl :113 / call :292, Scholomance block continues; script
	// name "boss_instructor_malicia"): boss AI ported in
	// lua_scripts/eastern_kingdoms/boss_instructor_malicia.lua
	// (entry 10505 — no NPC_ constant in scholomance.h, DB-side
	// ScriptName binding, classic wowhead-verified): OnEnterCombat
	// schedules call of the grave 17831 (triggered victim, 4s->65s),
	// corruption 11672 (triggered, random alive player within 100yd
	// incl. victim, 8s->24s), renew 10929 (self-cast, 32s->10s),
	// flash heal 10917 (self-cast, 38s->5s while FlashCounter<2 then
	// 30s), healing touch 9889 (self-cast, 45s->5500ms while
	// TouchCounter<2 then 30s); zero Talk lines. Documented-only (no
	// bridges): UpdateAI UNIT_STATE_CASTING gates,
	// BossAI::JustEngagedWith + DATA_INSTRUCTORMALICIA=1 bookkeeping
	// (scholomance.h:31) (luaBossAI shim).
	RegisterLuaBoss("boss_instructor_malicia", 10505)
	// boss_jandice_barov.cpp (EK loader AddSC_boss_jandicebarov decl
	// :114 / call :292, Scholomance block continues; script name
	// "boss_jandice_barov"): boss AI ported in
	// lua_scripts/eastern_kingdoms/boss_jandice_barov.lua (entry
	// 10503 — no NPC_ constant in scholomance.h, DB-side ScriptName
	// binding, wotlk.wowhead-verified): OnEnterCombat schedules
	// curse of blood 24673 (victim, 15s->30s) and illusion 17773
	// (self-cast, 30s->25s, plus a 3s one-shot EVENT_SET_VISIBILITY
	// whose legs are documented-only); OnDied triggered self-cast
	// drop journal 26096; zero Talk lines. Documented-only (no
	// bridges): JustSummoned illusion arms (AttackStart on a random
	// target, ApplySpellImmune magic-damage — the immunity bridge exists
	// (creature:ApplySpellImmune) but has no summoned bearer (see
	// boss_jandice_barov.lua), SummonList
	// tracking/DespawnAll), the NOT_SELECTABLE flag / SetDisplayId
	// (11686 invisible, 11073 Jandice model) / ModifyThreatByPercent
	// (victim, -99) legs, UpdateAI UNIT_STATE_CASTING gates;
	// EVENT_CLEAVE is declared in C++ but never scheduled.
	RegisterLuaBoss("boss_jandice_barov", 10503)
	// boss_kormok.cpp (EK loader AddSC_boss_kormok decl :115 / call
	// :293, Scholomance block continues; script name "boss_kormok"):
	// boss AI ported in lua_scripts/eastern_kingdoms/boss_kormok.lua
	// (entry 16118 — no NPC_ constant in scholomance.h, DB-side
	// ScriptName binding; level 60 elite, Scholomance; not one of the
	// 8 scholomance.h tracked encounters, so no DATA_ bookkeeping).
	// OnEnterCombat schedules shadowbolt volley 20741 (victim,
	// 10s->15s), bone shield 27688 (victim per C++ DoCastVictim,
	// 2s->45s), summon bone minions 27687 (self-cast, 15s->12s);
	// sub-25% DamageTaken once-latch (damaged class, per-GUID
	// once-guard, cleared 2/4/23) self-casts summon bone mages
	// 27695; zero Talk lines. Documented-only (no bridges):
	// UpdateAI UNIT_STATE_CASTING gates, JustSummoned
	// AttackStart(victim), both summon SpellScripts (nightbane
	// precedent).
	RegisterLuaBoss("boss_kormok", 16118)
	// boss_lord_alexei_barov.cpp (EK loader AddSC_boss_lordalexeibarov
	// decl :116 / call :294, Scholomance block continues; script name
	// "boss_lord_alexei_barov"): boss AI ported in
	// lua_scripts/eastern_kingdoms/boss_lord_alexei_barov.lua (entry
	// 10504 — no NPC_ constant in scholomance.h, DB-side ScriptName
	// binding, wotlk.wowhead-verified). OnReset self-casts unholy aura
	// 17467 when absent (no HasAura bridge — per-guid latch,
	// kirtonos convention, cleared 2/4/23); OnEnterCombat schedules
	// immolate 20294 (triggered, random alive player within 100yd
	// incl. victim, 7s->12s) and veil of shadow 17820 (triggered
	// DoCastVictim, 15s->20s), nil pick casts nothing but keeps the
	// schedule; zero Talk lines. Documented-only (no bridges):
	// UpdateAI UNIT_STATE_CASTING gates, BossAI::JustEngagedWith +
	// DATA_LORDALEXEIBAROV=3 bookkeeping (luaBossAI shim).
	RegisterLuaBoss("boss_lord_alexei_barov", 10504)
	// boss_lorekeeper_polkelt.cpp (EK loader AddSC_boss_lorekeeperpolkelt
	// decl :117 / call :295, Scholomance block continues; script name
	// "boss_lorekeeper_polkelt"): boss AI ported in
	// lua_scripts/eastern_kingdoms/boss_lorekeeper_polkelt.lua (entry
	// 10901 — no NPC_ constant in scholomance.h, DB-side ScriptName
	// binding, classic.wowhead-verified). OnEnterCombat schedules
	// volatile infection 24928 (triggered DoCastVictim, 38s->32s) /
	// dark plague 18270 (triggered DoCastVictim, 8s->8s) / corrosive
	// acid 23313 (triggered DoCastVictim, 45s->25s) / noxious catalyst
	// 18151 (triggered DoCastVictim, 35s->38s), nil-victim keeps
	// schedule; zero Talk lines. Documented-only (no bridges):
	// UpdateAI UNIT_STATE_CASTING gates, BossAI::JustEngagedWith +
	// DATA_LOREKEEPERPOLKELT=4 bookkeeping (luaBossAI shim).
	RegisterLuaBoss("boss_lorekeeper_polkelt", 10901)
	// boss_ras_frostwhisper.cpp (EK loader AddSC_boss_rasfrost decl
	// :118 / call :296, Scholomance block continues; script name
	// "boss_boss_ras_frostwhisper" — C++ double-"boss" quirk preserved
	// verbatim): boss AI ported in
	// lua_scripts/eastern_kingdoms/boss_ras_frostwhisper.lua (entry
	// 10508 — no NPC_ constant in scholomance.h, DB-side ScriptName
	// binding, wowhead-verified). OnReset non-triggered self-cast ice
	// armor 18100; OnEnterCombat schedules ice armor 2s->3min (self),
	// frostbolt 21369 8s->8s (random alive player within 40yd incl.
	// victim, nil pick casts nothing), chill nova 18099 12s->14s /
	// freeze 18763 18s->24s / fear 26070 45s->30s (all non-triggered
	// DoCastVictim, nil-victim keeps schedule); EVENT_FROSTVOLLEY is
	// never scheduled in C++ (dead code) — not modeled; zero Talk
	// lines. Documented-only (no bridges): UpdateAI UNIT_STATE_CASTING
	// gates, GetScholomanceAI template validation (luaBossAI shim).
	RegisterLuaBoss("boss_boss_ras_frostwhisper", 10508)
	// boss_the_ravenian.cpp (EK loader AddSC_boss_theravenian decl
	// :119 / call :297, Scholomance block continues; script name
	// "boss_the_ravenian"): boss AI ported in
	// lua_scripts/eastern_kingdoms/boss_the_ravenian.lua (entry
	// 10507 — no NPC_ constant in scholomance.h, DB-side ScriptName
	// binding, wowhead-verified). OnEnterCombat schedules trample
	// 15550 24s->10s / cleave 20691 15s->7s / sundering cleave
	// 25174 40s->20s / knock away 10101 32s->12s (all triggered
	// DoCastVictim -> GetVictim + CastSpell(victim, spell, true),
	// nil-victim keeps schedule); zero Talk lines. Documented-only
	// (no bridges): UpdateAI UNIT_STATE_CASTING gates,
	// GetScholomanceAI template validation (luaBossAI shim),
	// BossAI::JustEngagedWith + DATA_THERAVENIAN=5 bookkeeping.
	RegisterLuaBoss("boss_the_ravenian", 10507)
	// boss_vectus.cpp (EK loader AddSC_boss_vectus decl :120 / call
	// :298, Scholomance block continues; script name "boss_vectus"):
	// boss AI ported in lua_scripts/eastern_kingdoms/boss_vectus.lua
	// (entry 10432 — no NPC_ constant in scholomance.h, DB-side
	// ScriptName binding, wowhead-verified). OnEnterCombat schedules
	// fire shield 19626 2s->90s / blast wave 16046 14s->12s (both
	// non-triggered DoCast(me), jeklik convention); DamageTaken has
	// NO once-guard — every sub-25% post-damage hit fires the
	// EVENT_FRENZY arm: non-triggered self-cast frenzy 8269 +
	// Talk(EMOTE_FRENZY 0) + 24s re-arm; SPELL_FLAMESTRIKE 18399
	// declared but unused in C++ (dead enum entry, not modeled).
	// Documented-only (no bridges): UpdateAI UNIT_STATE_CASTING
	// gates, GetScholomanceAI template validation (luaBossAI shim),
	// BossAI::JustEngagedWith bookkeeping (Vectus is not one of the
	// 8 tracked encounters — no DATA_ constant in scholomance.h).
	RegisterLuaBoss("boss_vectus", 10432)
	// boss_kirtonos_the_herald.cpp (EK loader AddSC_boss_kirtonos_the_herald
	// decl :121 / call :299, Scholomance block; script names
	// "boss_kirtonos_the_herald" + "go_brazier_of_the_herald"): boss AI
	// ported in lua_scripts/eastern_kingdoms/boss_kirtonos_the_herald.lua
	// (entry NPC_KIRTONOS = 10506 from the file's own
	// Brazier_Of_The_Herald enum — C++-named, DB-side ScriptName binding;
	// DATA_KIRTONOS via luaBossAI shim). Oct-1 port 8a04cc9 added the lua
	// file but never registered it — this line completes the unit. Arms:
	// swoop 18144 8s->15s / wing flap 12882 15s->13s (self-cast) / pierce
	// armor 6016 18s->12s / disarm 8379 22s->11s / shadow bolt 17228
	// 42s->42s / curse of tongues 12889 53s->35s / dominate mind 14515
	// {34s,48s}->{44s,48s} (all triggered DoCastVictim) / transform 16467
	// 20s->{16s,18s} (per-guid latch toggle, no HasAura bridge).
	// Documented-only: intro choreography, gate/brazier GO arms,
	// go_brazier_of_the_herald (no bridges).
	RegisterLuaBoss("boss_kirtonos_the_herald", 10506)
	// instance_scholomance.cpp audit (map 289, "SC", EK loader decl :122 /
	// call :300 follows kirtonos_the_herald): InstanceMapScript only — no
	// creature/at_ scripts, no Talk lines, no Lua InstanceMapScript bridge,
	// so this unit is documented-only. C++ arms: ctor SetHeaders("SC") +
	// SetBossNumber(EncounterCount 8, DATA 0..7: theolen/kirtonos last in C++
	// order DATA_KIRTONOS=7). OnGameObjectCreate latches 9 GUIDs: gates
	// 175570 kirtonos / 177374 gandling / 177375 malicia / 177377 theolen /
	// 177376 polkelt / 177372 ravenian / 177373 barov / 177371 illucia +
	// brazier_of_the_herald 175564. SetBossState on the 6 pre-gandling bosses
	// (DATA_LORDALEXEIBAROV/DOCTORTHEOLENKRASTINOV/THERAVENIAN/
	// LOREKEEPERPOLKELT/INSTRUCTORMALICIA/LADYILLUCIABAROV) calls
	// CheckToSpawnGandling; CheckPreBosses(DATA_DARKMASTERGANDLING=6)
	// requires all six DONE and gandling not DONE; then SummonCreature
	// NPC_DARKMASTER_GANDLING=1853 at GandlingLoc (180.7712, -5.428603,
	// 75.57024, 1.291544). ReadSaveDataMore re-checks across map loads.
	// GetGuidData returns the 9 latched GUIDs. No SummonCreature/SetBossState/
	// GetBossState/GetGuidData bridges — joins the no-instance-script-bridge
	// queue (deadmines/blackrock_spire/gnomeregan/karazhan/molten_core/
	// naxxramas/trial_of_the_crusader/scarlet_monastery precedent).
	// instance_shadowfang_keep.cpp audit (map 33, "SK", EK loader decl :124 /
	// call :302 follows shadowfang_keep): InstanceMapScript only — no
	// creature/at_/go scripts, no Lua InstanceMapScript bridge, so this
	// unit is documented-only. C++ arms: ctor SetHeaders("SK") +
	// MAX_ENCOUNTER 4, m_auiEncounter zeroed; Load resets IN_PROGRESS to
	// NOT_STARTED. OnCreatureCreate latches uiAshGUID (NPC_ASH=3850),
	// uiAdaGUID (NPC_ADA=3849), uiArchmageArugalGUID
	// (NPC_ARCHMAGE_ARUGAL=4275), and the
	// NPC_DND_CRAZED_APOTHECARY_GENERATOR=36212 vector.
	// OnGameObjectCreate latches DoorCourtyardGUID (GO_COURTYARD_DOOR=18895,
	// encounter[0]), DoorSorcererGUID (GO_SORCERER_DOOR=18972, encounter[2]),
	// DoorArugalGUID (GO_ARUGAL_DOOR=18971, encounter[3]) — each re-opened
	// on late map load when its encounter is already DONE. SetData:
	// TYPE_FREE_NPC=1 DONE -> DoUseDoorOrButton(courtyard door);
	// TYPE_RETHILGORE=2 DONE -> DoSpeech (Ada Talk(SAY_BOSS_DIE_AD=4) +
	// Ash Talk(SAY_BOSS_DIE_AS=3), both alive); TYPE_FENRUS=3 DONE -> 1s
	// timer, uiPhase=1; data==7 -> open sorcerer door; TYPE_NANDOS=4 DONE
	// -> open arugal door; DATA_SPAWN_VALENTINE_ADDS=7 -> each generator
	// CastSpell(68610). Every DONE persists the 4 encounters to DB.
	// GetData returns encounters 1..4. Update (Fenrus DONE only): phase 1
	// -> summon duplicate Archmage (entry 4275) at SpawnLocation[4]
	// (-138.64, 2170.159, 136.577, 2.737), SetImmuneToPC, REACT_DEFENSIVE,
	// triggered self-cast SPELL_ASHCROMBE_TELEPORT=15742,
	// Talk(SAY_ARCHMAGE=0), 2s -> phase 2 -> summon 4x
	// NPC_ARUGAL_VOIDWALKER=4627 at SpawnLocation[0..3]. No
	// SummonCreature/SetData/GetData/DoUseDoorOrButton/SaveToDB bridges —
	// joins the no-instance-script-bridge queue
	// (deadmines/blackrock_spire/gnomeregan/karazhan/molten_core/
	// naxxramas/trial_of_the_crusader/scarlet_monastery/scholomance
	// precedent).
	// shadowfang_keep.cpp audit (EK loader decl :123 / call :301 follows
	// instance_scholomance): all four AddSC arms accounted for.
	// npc_arugal_voidwalker (entry 4627) and boss_archmage_arugal
	// (entry 4275) ported in lua_scripts/eastern_kingdoms/
	// npc_arugal_voidwalker.lua + boss_archmage_arugal.lua (Oct-1
	// 90de428). npc_shadowfang_prisoner
	// (NPC_ASH=3850/NPC_ADA=3849, shadowfang_keep.h:39-40) is an EscortAI
	// waypoint step machine with gossip select/hello: WaypointReached
	// Talks (wp 0 free / wp 10 open door / wp 12 post-door / wp 13 ADA
	// post2) + DoCast(me, SPELL_UNLOCK=6421) at wp 11 by Ash only,
	// instance SetData(TYPE_FREE_NPC=1, DONE) at wp 12, gossip gated on
	// TYPE_FREE_NPC != DONE && TYPE_RETHILGORE=2 == DONE — no
	// EscortAI/waypoint/gossip bridges, documented-only
	// (npc_barnes precedent). spell_shadowfang_keep_haunting_spirits
	// is an AuraScript (SPELL_AURA_DUMMY, EFFECT_0): CalcPeriodic arms
	// 30-90s, dummy tick casts aurEff->GetAmount() triggered on target,
	// UpdatePeriodic recalculates — no SpellScript/AuraScript bridge,
	// joins the no-SpellScript-bridge queue. No new Lua files;
	// arugal slot added 2026-10-25 (below).
	// boss_archmage_arugal (Shadowfang Keep): BossAI-derived
	// (shadowfang_keep.cpp:230) — the slot, not Eluna registration, gates
	// beginInstanceEncounter (worldport_instance.go:376).
	RegisterLuaBoss("boss_archmage_arugal", 4275)
	// boss_apothecary_hummel.cpp audit (EK loader decl :125 / call :303
	// follows instance_shadowfang_keep): 9 scripts. boss_apothecary_hummel
	// (entry 36296 — no NPC_ constant in C++, DB-side ScriptName binding,
	// wowhead npc=36296 / TrinityCore issue #16629 cited) ported in
	// lua_scripts/eastern_kingdoms/boss_apothecary_hummel.lua: gossip +
	// quest-reward (14488) start the intro machine (SAY_0..2 at 1ms/4s/4s),
	// START_FIGHT arms CALL_BAXTER 6s (Talk 3) / CALL_FRYE 14s (Talk 4) /
	// CALL_CRAZED 15s (Talk 6) / PERFUME_SPRAY 3640ms (DoCastVictim 68607)
	// / CHAIN_REACTION 15s->25s (triggered DoCastVictim 69218); lethal-blow
	// DamageTaken clamps to health-1 via the event-9 second return with a
	// per-GUID feign-death latch (Talk 5 + triggered self-cast 29266).
	// npc_apothecary_baxter (36565) + npc_apothecary_frye (36272) were
	// ported Oct-1 (364e946) and verified C++-exact this tick: baxter
	// EVENT_COLOGNE_SPRAY 7s->4s (DoCastVictim 68948) / EVENT_CHAIN_REACTION
	// 12s->25s (DoCastVictim 69218 + DoCastVictim 68821, both non-triggered),
	// JustDied Talk(0); frye JustDied Talk(0). DATA_APOTHECARY_HUMMEL=6 /
	// DATA_SPAWN_VALENTINE_ADDS=7 per shadowfang_keep.h:33-34.
	// Documented-only (no bridges): 6 SpellScripts + 2 AuraScripts
	// (68965/68644/69038/68966/68798/68614), npc_apothecary_genericAI
	// DoAction/MovementInform legs, summon-DoAction/DoZoneInCombat relays,
	// SetImmuneToPC/SetFaction/SetFlag/RemoveAurasDueToSpell arms,
	// SummonedCreatureDies (no event-21 fire site — the lethal clamp
	// persists and the QUIET_SUICIDE arm cannot fire), instance SetBossState
	// / SetData / LFG FinishDungeon(288) legs, DoCastAOE(68821), the
	// EVENT_CRAZED_APOTHECARY SetData-only timer, UNIT_STATE_CASTING gates.
	RegisterLuaBoss("boss_apothecary_hummel", 36296)
	RegisterLuaBoss("npc_apothecary_baxter", 36565)
	RegisterLuaBoss("npc_apothecary_frye", 36272)
	// boss_magistrate_barthilas.cpp audit (Stratholme block, EK loader decl
	// :126 / call :304 follows hummel): 1 script, pure timer-driven
	// ScriptedAI — no NPC_ constant in C++ (DB-side ScriptName binding,
	// entry 10435 wowhead-cited). Ported in
	// lua_scripts/eastern_kingdoms/boss_magistrate_barthilas.lua:
	// FURIOUS_ANGER 5s->4s self-cast 16791 (re-arm precedes the
	// AngerCount > 25 check, so the 26th cast is last) / DRAINING_BLOW
	// 20s->15s DoCastVictim 16793 / CROWD_PUMMEL 15s DoCastVictim 10887 /
	// MIGHTY_BLOW 10s->20s DoCastVictim 14099 (GetVictim + CastSpell,
	// nil-victim keeps schedule). Documented-only (no bridges):
	// SetDisplayId(10433/3637) legs in Reset/JustDied (jandice precedent).
	RegisterLuaBoss("boss_magistrate_barthilas", 10435)
	// boss_maleki_the_pallid.cpp audit (Stratholme block, EK loader decl
	// :127 / call :305 follows barthilas): 1 script, pure timer-driven
	// ScriptedAI — no NPC_ constant in C++ (DB-side ScriptName binding,
	// entry 10438 wowhead-cited). Ported in
	// lua_scripts/eastern_kingdoms/boss_maleki_the_pallid.lua:
	// FROSTBOLT 1s->3.5s DoCastVictim 17503 / ICETOMB 16s->28s DoCastVictim
	// 16869 / DRAINLIFE 31s DoCastVictim 20743 (rand32()%90/65/55 cast
	// gates preserved, re-arm unconditional; GetVictim + CastSpell,
	// nil-victim keeps schedule). Documented-only (no bridges):
	// JustDied instance->SetData(TYPE_PALLID 4, IN_PROGRESS) (no
	// instance-script bridge), UpdateAI UNIT_STATE_CASTING gate, dead
	// EVENT_DRAIN_MANA 17243 (declared, never scheduled).
	RegisterLuaBoss("boss_maleki_the_pallid", 10438)
	// boss_nerubenkan.cpp audit (Stratholme block, EK loader decl :128 /
	// call :306 follows maleki): 1 script, pure timer-driven ScriptedAI —
	// no Talk lines, no NPC_ constant in stratholme.h (DB-side ScriptName
	// binding, entry 10437 classic.wowhead-cited). Ported in
	// lua_scripts/eastern_kingdoms/boss_nerubenkan.lua: ENCASINGWEBS 7s->30s
	// DoCastVictim 4962 / PIERCEARMOR 19s->35s DoCastVictim 6016
	// (urand(0,3)<2 50% cast gate, re-arm unconditional) / CRYPTSCARABS
	// 3s->20s DoCastVictim 31602 / RAISEUNDEADSCARAB 3s->16s PORTED:
	// SummonCreature(10876, irand(-9,9) offsets, type 1, 180s) (the
	// stale "no summon bridge, herod precedent" is retracted; the
	// scarab's AttackStart(victim) has no bridge — it idles until the
	// engine aggros it, noxxion precedent) (GetVictim +
	// CastSpell, nil-victim keeps schedule; JustEngagedWith empty in C++).
	// Documented-only (no bridges): JustDied instance->SetData(TYPE_NERUB
	// 3, IN_PROGRESS) (no instance-script bridge), UpdateVictim gate,
	// melee engine-driven.
	RegisterLuaBoss("boss_nerubenkan", 10437)
	// boss_cannon_master_willey.cpp audit (Stratholme block, EK loader decl
	// :129 / call :307 follows nerubenkan): 1 script, pure timer-driven
	// ScriptedAI — no Talk lines, no NPC_ constant in stratholme.h
	// (DB-side ScriptName binding, entry 10997 classic.wowhead-cited).
	// Ported in lua_scripts/eastern_kingdoms/boss_cannon_master_willey.lua:
	// PUMMEL 7s->12s DoCastVictim 15615 (90% cast gate, re-arm
	// unconditional) / KNOCKAWAY 11s->14s DoCastVictim 10101 (80% gate,
	// re-arm unconditional) / SHOOT 1s->1s DoCastVictim 16496 /
	// SUMMONRIFLEMAN 15s->30s PORTED: rand32()%9 case selection as the
	// random leg + 3x SummonCreature(11054) at the C++-exact triples
	// (type 2, 4min; the stale "no summon bridge, herod precedent" is
	// retracted; 9 ADD positions C++-exact in the lua header) (GetVictim
	// + CastSpell, nil-victim keeps schedule; JustEngagedWith empty in
	// C++). Ported: JustDied 7x SummonCreature(11054) at ADD 1,2,3,4,5,7,9
	// (C++ skips 6/8). Documented-only (no bridges): UpdateVictim gate,
	// melee engine-driven.
	RegisterLuaBoss("boss_cannon_master_willey", 10997)
	// boss_baroness_anastari.cpp audit (Stratholme block, EK loader decl
	// :130 / call :308 follows willey): 1 script, BossAI timer AI
	// (TYPE_BARONESS = 2, stratholme.h:29) — no Talk lines, no NPC_
	// constant (DB-side ScriptName binding, entry 10436 wowhead-cited).
	// Ported in lua_scripts/eastern_kingdoms/boss_baroness_anastari.lua:
	// BANSHEEWAIL 1s->4s DoCastVictim 16565 / BANSHEECURSE 11s->18s
	// DoCastVictim 16867 / SILENCE 13s->13s DoCastVictim 18327 /
	// POSSESS 20s-30s random (SelectTarget Random,1,0,true,false ->
	// random alive non-victim player, fairbanks-fear convention):
	// triggered 17244 + 17246 on target, triggered self-cast 17250
	// (invisibility), GUID latch, immediate EVENT_CHECK_POSSESSED;
	// check leg via GetPlayerByGUID (nil = chain dies, C++-exact):
	// !HasAura(17246) or HealthBelowPct(50) -> strip auras, re-arm
	// possess 20s-30s random, else repeat 1s. Reset's instance-wide
	// DoRemoveAurasDueToSpellOnPlayers (17244/17246) is ported in onReset
	// (event 23 — GetPlayersInWorld + mapId/instanceId filter,
	// player:RemoveAura); documented-only: JustDied
	// instance->SetData(TYPE_BARONESS, IN_PROGRESS)
	// (IN_PROGRESS-not-DONE C++ quirk, nerubenkan precedent),
	// UpdateVictim/UNIT_STATE_CASTING gates, melee engine-driven.
	RegisterLuaBoss("boss_baroness_anastari", 10436)
	// boss_ramstein_the_gorger.cpp audit (EK loader decl :131 / call
	// :309 follows baroness_anastari): 1 script, ScriptedAI diff-timer AI
	// (Initialize/Reset arms Trample 3s + Knockout 12s; UpdateVictim
	// gate; empty JustEngagedWith; no Talk, no gossip/quest, no
	// Spell/AuraScripts) — C++-verified lua registered
	// (NPC_RAMSTEIN=10439 stratholme.h:54). Lua arms: OnEnterCombat
	// Trample 3s->7s (non-triggered self-cast 5568) / Knockout 12s->10s
	// (non-triggered DoCastVictim 17307, nil-victim keeps schedule);
	// timers cleared on Reset/LeaveCombat/Died. Documented-only (no
	// bridges): JustDied's 30x SummonCreature(NPC_MINDLESS_UNDEAD=11030,
	// 3969.35f±10, -3391.87f±10, 119.11f, 30min timed-or-dead) +
	// AttackStart(SelectNearestTarget(100)) legs, instance->SetData(
	// TYPE_RAMSTEIN=5, DONE), GetStratholmeAI instance leg; melee
	// engine-driven.
	RegisterLuaBoss("boss_ramstein_the_gorger", 10439)
	// boss_timmy_the_cruel.cpp audit (EK loader decl :132 / call :310
	// follows ramstein_the_gorger): 1 script, ScriptedAI diff-timer AI
	// (Initialize/Reset arms RavenousClaw 10s + clears HasYelled latch;
	// JustEngagedWith Talk(SAY_SPAWN=0) once per engagement cycle;
	// UpdateVictim gate; no gossip/quest, no Spell/AuraScripts) —
	// Oct-1 lua_scripts/eastern_kingdoms/boss_timmy_the_cruel.lua
	// name-by-name C++-verified exact (NPC_TIMMY_THE_CRUEL=10808
	// stratholme.h:66) and registered. Lua arms: OnEnterCombat
	// RavenousClaw 10s->15s (non-triggered DoCastVictim 17470,
	// nil-victim keeps schedule); yelled latch cleared on OnReset (23);
	// timers cleared on Reset/LeaveCombat/Died. Documented-only (no
	// bridges): GetStratholmeAI -> GetInstanceAI instance leg;
	// melee engine-driven.
	RegisterLuaBoss("boss_timmy_the_cruel", 10808)
	// boss_postmaster_malown.cpp port (EK loader decl :133 / call :311
	// follows timmy_the_cruel): 1 script, BossAI event arms (BossAI
	// constructor arg TYPE_MALOWN=7, stratholme.h:35; no NPC_ constant —
	// DB-side ScriptName binding, entry 11143 wowhead-cited). New
	// lua_scripts/eastern_kingdoms/boss_postmaster_malown.lua. Lua arms:
	// OnEnterCombat schedules WAILINGDEAD 19s / BACKHAND 8s /
	// CURSEOFWEAKNESS 20s / CURSEOFTONGUES 22s / CALLOFTHEGRAVE 25s;
	// WAILINGDEAD 7713 65% gate -> 19s re-arm / BACKHAND 6253 45% gate /
	// CURSEOFWEAKNESS 8552 3% gate / CURSEOFTONGUES 12889 3% gate /
	// CALLOFTHEGRAVE 17831 5% gate (all triggered DoCastVictim ->
	// GetVictim + CastSpell(victim, spell, true), nil-victim keeps
	// schedule); C++ quirk preserved verbatim: every non-wailing case
	// reschedules EVENT_WAILINGDEAD with its own timer (8s/20s/22s/25s)
	// instead of its own event, replacing any pending wailingdead timer;
	// KilledUnit Talk(SAY_KILL 0) player-gated (illidan convention);
	// timers cleared on LeaveCombat/Died/Reset. Documented-only (no
	// bridges): empty Reset + BossAI::JustEngagedWith/TYPE_MALOWN
	// bookkeeping (luaBossAI shim); UNIT_STATE_CASTING gates (barthilas
	// precedent); GetStratholmeAI factory; melee engine-driven.
	RegisterLuaBoss("boss_postmaster_malown", 11143)
	// boss_baron_rivendare.cpp audit (EK loader decl :134 / call :312
	// follows postmaster_malown): 2 scripts, BossAI(creature, TYPE_BARON=6)
	// + npc_summoned_skeleton (ScriptedAI). Existing Oct-1
	// lua_scripts/eastern_kingdoms/boss_baron_rivendare.lua (3c2c512)
	// audited name-by-name vs C++ and fixed two deviations: shadowbolt's
	// SelectTarget(Random, 0) now picks a random alive player on the
	// map+instance with no distance bound (C++ DefaultTargetSelector
	// dist=0 = ignored = unlimited; threat-list-only pick approximated,
	// no threat-list bridge) instead of an arbitrary 100yd cap; the
	// RaiseDead latch now persists across evades exactly like C++ (C++
	// Reset/JustEngagedWith never touch the bool — only the event
	// handler toggles it). Arms: OnEnterCombat schedules shadowbolt
	// 17393 5s->10s / cleave 15284 8s->{7s,17s} / mortal strike 15708
	// 12s->{10s,25s} (all DoCastVictim -> GetVictim + CastSpell,
	// nil-victim keeps schedule, moroes/maiden convention) / raise dead
	// 15s->12s (!latch: DoCastSelf 17473 + 6x triggered DoCastSelf
	// 17475-17480 + Talk(0); latch: Talk(1)). Timers cleared on
	// LeaveCombat/Died/Reset. Documented-only (no bridges): Reset's
	// TYPE_RAMSTEIN-DONE->TYPE_BARON-NOT_STARTED leg, JustEngagedWith's
	// TYPE_BARON IN_PROGRESS leg, JustDied's TYPE_BARON DONE leg, the
	// UNIT_STATE_CASTING gates, GetStratholmeAI factory; melee
	// engine-driven. npc_summoned_skeleton's SpellHit(17471) ->
	// DoCastSelf(17472, TRIGGERED) leg not registered — no C++-verifiable
	// NPC entry and no SpellHit bridge (entry-unverifiable queue).
	RegisterLuaBoss("boss_baron_rivendare", 10440)
	// boss_dathrohan_balnazzar.cpp audit (EK loader decl :135 / call :313
	// follows baron_rivendare): 1 script, ScriptedAI via GetStratholmeAI
	// (no BossAI, no instance bookkeeping). Fresh port —
	// lua_scripts/eastern_kingdoms/boss_dathrohan_balnazzar.lua (Oct-1
	// pass was document-only, zero trust). Arms: OnEnterCombat
	// Initialize() — mindblast 17287 6s->{15s,20s} (shared timer across
	// both phases, kept pending on transform) / crusadershammer 17286
	// 8s->12s / crusaderstrike 17281 12s->15s / holystrike 17284 18s->15s
	// (all DoCastVictim -> GetVictim + CastSpell, nil-victim keeps
	// schedule). Per-tick HealthBelowPct(40) check on creature event 7
	// (100ms world tick, victim-gated per UpdateVictim): transform cancels the
	// Dathrohan arms, self-casts 17288 (barthilas convention), and starts
	// the Balnazzar arms at their C++-frozen initials — shadowshock 17399
	// 4s->11s / psychicscream 13704 16s->20s / sleep 12098 20s->15s
	// (SelectTarget(Random, 0) -> random alive player on map+instance, no
	// distance bound; nil pick casts nothing, re-arm unconditional) /
	// mindcontrol 15690 10s->15s. Timers + transform latch cleared on
	// LeaveCombat/Died/Reset. Documented-only: transform's InterruptNonMeleeSpells gate is bridged
	// but the arm stays unported on UpdateEntry(10813)/Reset's
	// UpdateEntry(10812) revert, JustDied's 8x zombie (10698) summons at
	// the file's m_aSummonPoint coords (1h timed despawn), empty
	// JustEngagedWith, GetStratholmeAI factory; melee engine-driven.
	RegisterLuaBoss("boss_dathrohan_balnazzar", 10812)
	// boss_order_of_silver_hand.cpp audit (EK loader decl :136 / call
	// :314 follows dathrohan_balnazzar :135/:313; one script
	// "boss_silver_hand_bosses", ScriptedAI diff-timer AI, Oct-1 pass was
	// document-only): all five Order of the Silver Hand members run the
	// same AI — Gregor 17910 / Cathela 17911 / Nemas 17912 / Aelmar 17913 /
	// Vicar 17914 (SH_GREGOR..SH_VICAR, file's own enum). ScriptName binds
	// all five rows DB-side.
	RegisterLuaBoss("boss_silver_hand_bosses", 17910)
	RegisterLuaBoss("boss_silver_hand_bosses", 17911)
	RegisterLuaBoss("boss_silver_hand_bosses", 17912)
	RegisterLuaBoss("boss_silver_hand_bosses", 17913)
	RegisterLuaBoss("boss_silver_hand_bosses", 17914)
	// instance_stratholme.cpp audit (map 329, "STR", EK loader
	// decl :137 / call :315 follows order_of_silver_hand): InstanceMapScript
	// only — zero creature/at_/go scripts, zero Spell/AuraScripts, no Lua
	// InstanceMapScript bridge, so this unit is documented-only. C++ arms:
	// ctor SetHeaders("STR"), 6 EncounterState NOT_STARTED,
	// IsSilverHandDead[5] false, timmySpawned false, scarletsKilled 0.
	// OnUnitDeath: crimson guardsman/conjuror/initiate/gallant deaths inside
	// the beforeScarletGate ellipse increment scarletsKilled — at 15
	// (TIMMY_THE_CRUEL_CRUSADERS_REQUIRED) summons NPC_TIMMY_THE_CRUEL
	// 10808 at (3625.358, -3188.108, 130.3985, 4.834562), one-shot.
	// OnCreatureCreate/Remove latch baron/ysida/ysida-trigger GUIDs +
	// crystals and bile/venom abomination GuidSets (crystals unused in
	// logic — "change to DONE when crystals implemented" TODO).
	// OnGameObjectCreate latches 12 GO GUIDs with late-load reopen legs
	// (ziggurats 1-5, port gauntlet/slaugther/elders, service entrance,
	// ysida cage; gauntlet gate 1 gets GO_FLAG_LOCKED, "DB bug?" C++ note).
	// SetData: TYPE_BARON_RUN IN_PROGRESS arms 45min EVENT_BARON_RUN /
	// FAIL removes ultimatum 27861 auras + ysida feign-death 29266 /
	// DONE opens ysida cage, ysida self-casts 31912, AI Talk(0),
	// questgiver flag restore, MovePoint 1, per-player 31913 credit leg.
	// TYPE_BARONESS/NERUB/PALLID IN_PROGRESS open ziggurats 1/2/3 +
	// StartSlaugtherSquare (all three IN_PROGRESS opens port gauntlet +
	// slaugther GOs). TYPE_RAMSTEIN IN_PROGRESS: if all abominations
	// dead, summons ramstein 10439 (4032.84, -3390.24, 119.73, 4.71,
	// 30min despawn); DONE schedules 1min EVENT_SLAUGHTER_SQUARE;
	// NOT_STARTED reopens port gauntlet. TYPE_BARON IN_PROGRESS closes
	// ziggurats 4/5; DONE opens them + port gauntlet, removes ultimatum,
	// cascades TYPE_BARON_RUN DONE. TYPE_SH_* latch the 5 silver-hand
	// death flags consumed by GetData(TYPE_SH_QUEST)=20 (all five ->
	// boss_silver_hand_bosses quest credit leg, already ported). DONE
	// persists to DB. GetSaveData/Load: six encounter states (IN_PROGRESS
	// on 0/4/5 reset to NOT_STARTED at load). GetGuidData: DATA_BARON /
	// DATA_YSIDA_TRIGGER / NPC_YSIDA. Update: EventMap arms —
	// EVENT_BARON_RUN expires to FAIL (unless already DONE);
	// EVENT_SLAUGHTER_SQUARE summons 4x black guard 10394 at the ramstein
	// position and opens ziggurats 4/5. The single Talk line (Ysida
	// SAY_YSIDA_SAVED 0) is instance-driven, not a creature script.
	// No SummonCreature/GetCreature/GetGameObject/SetGoState/MovePoint/
	// AreaBoundary/EventMap bridges — joins the no-instance-script-bridge
	// queue (deadmines/blackrock_spire/gnomeregan/karazhan/molten_core/
	// naxxramas/trial_of_the_crusader precedent). STRATHOLME GROUP
	// COMPLETE.
	// sunken_temple.cpp audit (EK loader decl :139 / call :317 follows
	// stratholme): two scripts, zero bridgeable arms — documented-only,
	// no lua, no registration. at_malfurion_stormrage (AreaTriggerScript):
	// OnTrigger summons Malfurion Stormrage 15362 at the player (facing
	// -1.52f, 100s timed-or-dead despawn) only when the player has an
	// instance script, no Malfurion within 15 yd, quest 8555
	// (THE_CHARGE_OF_DRAGONFLIGHTS) REWARDED, and quest 8733
	// (ERANIKUS_TYRANT_OF_DREAMS) not REWARDED; returns false. No
	// AreaTrigger bridge on the Lua surface (boss_the_beast /
	// OnlyOnceAreaTriggerScript precedent) — cannot register. go_atalai_statue
	// (GameObjectScript + go_atalai_statueAI : GameObjectAI): OnGossipHello's
	// only arm is instance->SetData(EVENT_STATE, go entry) feeding the
	// instance_sunken_temple statue press-order puzzle machine (GO_ATALAI_
	// STATUE1..6 = 148830-148835, lights 148883/148937, sunken_temple.h
	// STScriptName "instance_sunken_temple"/DataHeader "ST"/EVENT_STATE=1);
	// the AI's GetAI leg is GetSunkenTempleAI (GetInstanceAI) — instance
	// model blocked. A gossip hook whose sole arm is instance SetData
	// would be a stub (go_blackfathom_fire / go_keystone_chamber
	// precedent) — DOCUMENTED, not registered. Joins the no-instance-/
	// no-AreaTrigger-bridge queues.
	// instance_sunken_temple.cpp audit (map 109, "ST", EK loader decl
	// :140 / call :318 follows sunken_temple): InstanceMapScript only —
	// zero creature/at_/go scripts, zero Talk, zero Spell/AuraScripts, no
	// Lua InstanceMapScript bridge, so this unit is documented-only. C++
	// arms: ctor SetHeaders("ST"), State=0, s1..s6 false. OnGameObjectCreate
	// latches 6 statue GUIDs (GO_ATALAI_STATUE1..6 = 148830-148835).
	// Update() runs the statue press-order machine (correct order 1-6):
	// statue-N leg requires s1..s(N-1) all true with sN..s6 false, then
	// UseStatue summons GO_ATALAI_LIGHT1 148883 at the statue + sets
	// GAMEOBJECT_FLAGS 4, sets sN=true, resets State=0; the statue-6 leg
	// also calls UseLastStatue — 6x GO_ATALAI_LIGHT2 148937 at the C++-exact
	// statuePositions + SummonCreature(NPC_ATALALARION 8580, corpse-timed
	// 10min, atalalarianPos). SetData/GetData(EVENT_STATE=1) carry the
	// pending statue entry from go_atalai_statue gossip (audited above).
	// No OnGameObjectCreate/Update bridges (SummonGameObject/SummonCreature
	// are bridged now) — joins the no-instance-script-bridge queue. SUNKEN
	// TEMPLE GROUP COMPLETE.
	// instance_sunwell_plateau.cpp audit (map 580, "SWP", EK loader decl
	// :141 / call :319 follows instance_sunken_temple): InstanceMapScript only —
	// zero creature/at_/go scripts, zero Talk, zero Spell/AuraScripts, no
	// Lua InstanceMapScript bridge, so this unit is documented-only. C++
	// arms: ctor SetHeaders("SWP") + SetBossNumber(EncounterCount 6) +
	// LoadDoorData (7 rows: GO_FIRE_BARRIER 188075->DATA_FELMYST=2 PASSAGE,
	// GO_MURUS_GATE_1 187990->DATA_MURU=4 ROOM, GO_MURUS_GATE_2 188118->
	// DATA_MURU=4 PASSAGE, GO_BOSS_COLLISION_1/2 188523/188524 + GO_FORCE_
	// FIELD 188421 ->DATA_KALECGOS=0 ROOM) + LoadObjectData (13 creature
	// pairs: 24850->DATA_KALECGOS_DRAGON, 24891->DATA_KALECGOS_HUMAN,
	// 24892->DATA_SATHROVARR, 24882->DATA_BRUTALLUS, 24895->DATA_MADRIGOSA,
	// 25038->DATA_FELMYST, 25166->DATA_ALYTHESS, 25165->DATA_SACROLASH,
	// 25741->DATA_MURU, 25315->DATA_KILJAEDEN, 25608->DATA_KILJAEDEN_
	// CONTROLLER, 26046->DATA_ANVEENA, 25319->DATA_KALECGOS_KJ) +
	// LoadBossBoundaries (DATA_KALECGOS: union of CircleBoundary(1704.9,
	// 928.4, 34.0) + RectangleBoundary(1689.2, 1713.3, 762.2, 1074.8)).
	// GetPlayerInMap: first player in map without aura 45839 (TC_LOG_DEBUG
	// if the player list is empty), else nullptr; GetGuidData(DATA_PLAYER_
	// GUID) returns that player's GUID or ObjectGuid::Empty. All six boss
	// fights already ported + registered (lua_scripts/sunwellplateau/;
	// boss_ai.go). Joins the no-instance-script-bridge queue. SUNWELL
	// PLATEAU GROUP STARTS.
	// instance_scarlet_monastery.cpp audit (map 189, "SM", EK loader
	// decl :107 / call :285 follows scorn): InstanceMapScript only — no
	// creature/at_ scripts, no Lua InstanceMapScript bridge, so this unit
	// is documented-only. C++ arms: ctor SetHeaders("SM") +
	// SetBossNumber(EncounterCount 9) + LoadObjectData(8 creature pairs:
	// 23775->DATA_HORSEMAN_HEAD, 23682->DATA_HEADLESS_HORSEMAN,
	// 23686->DATA_FLAME_BUNNY, 23758->DATA_EARTH_BUNNY, 23904->DATA_THOMAS,
	// 3976->DATA_MOGRAINE, 3981->DATA_VORREL, 3977->DATA_WHITEMANE; 3 GO
	// pairs: 186267->DATA_PUMPKIN_SHRINE, 104600->DATA_HIGH_INQUISITORS_DOOR,
	// 186314->DATA_LOOSELY_TURNED_SOIL), _horsemanState NOT_STARTED.
	// HandleStartEvent sets IN_PROGRESS, GO_FLAG_NOT_SELECTABLE on shrine +
	// soil, summons 23775/23686/23758 at the SM spawn positions, schedules
	// EVENT_ACTIVE_EARTH_EXPLOSION (1) at 1.5s, EVENT_SPAWN_HEADLESS_HORSEMAN
	// (2) at 3s, EVENT_DESPAWN_OBJECTS (3) at 10s, despawns Sir Thomas.
	// SetData: DATA_START_HORSEMAN_EVENT starts once, DATA_HORSEMAN_EVENT_STATE
	// sets state, DATA_PREPARE_RESET clears + despawns bunnies;
	// GetData: DATA_HORSEMAN_EVENT_STATE. Update: earth bunny self-casts
	// SPELL_EARTH_EXPLOSION 42373; horseman 23682 summon + AI DoAction
	// (ACTION_HORSEMAN_EVENT_START 101) — the horseman AI itself is ported in
	// lua_scripts/eastern_kingdoms/boss_headless_horseman.lua; shrine + soil
	// RemoveFromWorld. No SummonCreature/GetCreature/GetGameObject/SetFlag/
	// DespawnOrUnsummon/SetData/GetData/EventMap bridges — joins the
	// no-instance-script-bridge queue (deadmines/blackrock_spire/gnomeregan/
	// karazhan/molten_core/naxxramas/trial_of_the_crusader precedent).

	// boss_arcanist_doan (Scarlet Monastery, Library wing, EK loader
	// decl :98 / call :276): OnEnterCombat(1) arms silence 8988
	// 15s->{15s,20s}, arcane explosion 9433 3s->8s (both non-triggered
	// DoCastVictim -> GetVictim + CastSpell, nil-victim ticks keep the
	// schedule), and polymorph 13323 30s->20s on a random alive player
	// within 30 yd skipping position 0 (C++ SelectTarget(Random, 1,
	// 30.0f, true) — majordomo victim-excluding + opera in-range
	// convention); Talk(SAY_AGGRO 0) on engage. The UpdateAI
	// below-50% arm (Talk(SAY_SPECIALAE 1) + non-triggered DoCastSelf
	// arcane bubble 9438, once via _healthAbove50Pct) is modeled on
	// the pre-damage hook (event 9) with current-health strictly
	// below 50% semantics (C++ HealthBelowPct — golemagg 704fb8e bug
	// class); the latch fires on the damage hook rather than the next
	// UpdateAI tick (documented timing deviation) and the
	// DoCastAOE(detonation 9435) rides the self-cast lowering in Lua
	// (boss_arcanist_doan.lua:210, selin_fireheart convention) — the
	// old "no DoCastAOE bridge" claim was stale. The UpdateAI UNIT_STATE_CASTING gates and
	// BossAI::JustEngagedWith + DATA_ARCANIST_DOAN bookkeeping have
	// no instance/UNIT_STATE bridges (luaBossAI shim). Entry 6487 is
	// not named in the C++ tree (RegisterScarletMonasteryCreatureAI
	// binds the creature_template ScriptName DB-side) — wowhead-cited
	// (classic.wowhead.com/npc=6487/arcanist-doan).
	RegisterLuaBoss("boss_arcanist_doan", 6487)
	// Azshir the Sleepless (6490, Scarlet Monastery Graveyard rare)
	// runs call of the grave 17831 30s->30s / terrify 7399 20s->20s
	// (non-triggered DoCastVictim -> GetVictim + CastSpell; nil-victim
	// ticks cast nothing but keep the schedule — jeklik convention)
	// and the DamageTaken soul siphon 7290 once when post-damage
	// health drops strictly below 50% (HealthBelowPctDamaged —
	// damaged class, not the golemagg 704fb8e bug class) + 20s
	// re-arm; no Talk lines. The UpdateAI UNIT_STATE_CASTING gates
	// and BossAI::JustEngagedWith + DATA_AZSHIR bookkeeping have no
	// instance/UNIT_STATE bridges (luaBossAI shim). Entry 6490 is
	// not named in the C++ tree (RegisterScarletMonasteryCreatureAI
	// binds the creature_template ScriptName DB-side) — wowhead-cited
	// (classic.wowhead.com/npc=6490/azshir-the-sleepless).
	RegisterLuaBoss("boss_azshir_the_sleepless", 6490)
	// boss_bloodmage_thalnos (4543, Scarlet Monastery Graveyard, EK
	// loader decl :100 / call :278): OnEnterCombat(1) Talk(SAY_AGGRO 0)
	// + flame shock 8053 10s->{10s,15s}, shadow bolt 1106 2s->2s,
	// flame spike 8814 8s->30s, fire nova 16079 40s->40s (all
	// non-triggered DoCastVictim -> GetVictim + CastSpell; nil-victim
	// ticks cast nothing but keep the schedule — jeklik convention).
	// KilledUnit(3): Talk(SAY_KILL 2) only when the victim is a player
	// (C++ TYPEID_PLAYER gate -> victim:IsPlayer(), illidan
	// convention). DamageTaken(9): once, when post-damage health drops
	// strictly below 35% (HealthBelowPctDamaged — damaged class, not
	// the golemagg 704fb8e current-health bug class): Talk(SAY_HEALTH
	// 1), per-GUID once-guard; the C++ arm casts no spell. The
	// UpdateAI UNIT_STATE_CASTING queue gate and
	// BossAI::JustEngagedWith + DATA_BLOODMAGE_THALNOS encounter
	// bookkeeping have no instance/UNIT_STATE bridges (luaBossAI shim).
	// Entry 4543 is not named in the C++ tree
	// (RegisterScarletMonasteryCreatureAI binds the creature_template
	// ScriptName DB-side) — wowhead-cited
	// (www.wowhead.com/cata/npc=4543/bloodmage-thalnos).
	RegisterLuaBoss("boss_bloodmage_thalnos", 4543)
	// Archaedas (2748, Uldaman) runs the Ground Tremor (6524)
	// schedule (60s init, 45s re-arm, DoCastVictim -> GetVictim +
	// CastSpell). The awaken sequence (SpellHit 10347 Talk/yell +
	// 4s waking-up timer + AttackStart on the instance's GUID 0),
	// the 10s wall-minion SetData(DATA_MINIONS) pump, the <66%/
	// <33% guardian/vault-walker awaken arms, the JustDied
	// SetData(DATA_ANCIENT_DOOR, DONE)/SetData(DATA_MINIONS,
	// SPECIAL) legs and the Reset freeze-aura/flag/faction legs
	// are instance-model/SpellHit-15/flag/faction-blocked; the
	// KilledUnit Talk is queued (no CreatureEvent id). The stone
	// keepers (4857) run their JustDied triggered self-cast of
	// Self Destruct 9874 (headless_horseman/omen onDied self-cast
	// precedent); their faction/flag/root and DATA_STONE_KEEPERS
	// legs are documented-only. The archaedas minions
	// (7309/7077/7076/10120) have no bridgeable AI arms (state
	// legs + SpellHit-15 awaken + engine-driven melee) and the
	// Altar of Archaedas GO script is entry-unverifiable (no GO
	// entry named in the C++ sources) — documented in the .lua
	// header. Entries C++-verified via instance_uldaman.cpp's
	// OnCreatureCreate name-comments feeding the GUID vectors the
	// boss's GetGuidData(0/1-4/5-10) arms consume (kalecgos
	// entry-verifiability check, ramstein strength).
	RegisterLuaBoss("boss_archaedas", 2748)
	RegisterLuaBoss("npc_stonekeepers", 4857)
	// Ironaya (7228, Uldaman) runs Arcing Smash (8374, 3s init /
	// 13s re-arm, DoCast(me) -> self CastSpell) plus the one-shot
	// <50% Knockaway (10101, DoCastVictim triggered -> GetVictim +
	// CastSpell) and <25% W-Stomp (11876, self-cast) arms via a
	// 1s health check (kalecgos precedent). The Knockaway threat-
	// reset leg and the ActivateIronaya instance choreography have
	// no bridges; the latches clear on combat end/reset.
	// Entry C++-verified via instance_uldaman.cpp OnCreatureCreate
	// (case 7228: // Ironaya).
	RegisterLuaBoss("boss_ironaya", 7228)
	// npc_blackfathom_deeps_event (Blackfathom Deeps dungeon script):
	// 4825 Aku'mai Snapjaw runs Ravage (8391, DoCastVictim ->
	// GetVictim + CastSpell, 5-8s init / 9-14s repeat) and 4978
	// Aku'mai Servant runs Frost Nova (865, non-triggered DoCastAOE ->
	// self-cast per the illidan/vaelastrasz convention, 9-12s init /
	// 25-30s repeat) plus Frostbolt Volley (8398, random alive
	// player target, 2-4s init / 5-8s repeat), all non-triggered
	// (C++-exact). The JustDied DATA_EVENT increment, the
	// IsSummonedBy DoZoneInCombat leg, and the 15%-hp flee arms
	// (4977/4823, unregistered) have no bridges; the
	// go_blackfathom_fire gossip and npc_morridune escort scripts
	// are documented-only. Entries C++-verified via
	// blackfathom_deeps.h BFDCreatureIds plus instance_
	// blackfathom_deeps.cpp's SetData(DATA_FIRE) summon arms
	// (ramstein strength); see lua_scripts/kalimdor/
	// blackfathom_deeps.lua.
	RegisterLuaBoss("npc_blackfathom_deeps_event", 4825)
	RegisterLuaBoss("npc_blackfathom_deeps_event", 4978)

	// boss_gelihast (Gelihast, entry 6243 — BFDCreatureIds names no
	// Gelihast entry; registered on the DB ScriptName tie plus wowhead
	// npc=6243 corroboration, jeklik/vishas precedent): Throw Net 6533
	// on the victim (2-4s init / 4-7s repeat, jeklik GetVictim +
	// CastSpell convention, non-triggered); melee engine-driven. The
	// BossAI::JustEngagedWith instance leg and the GetInstanceAI leg
	// have no bridges; see lua_scripts/kalimdor/boss_gelihast.lua.
	RegisterLuaBoss("boss_gelihast", 6243)

	// boss_kelris (Twilight Lord Kelris, entry 4832 — C++-verified via
	// blackfathom_deeps.h BFDCreatureIds plus instance_blackfathom_deeps.
	// cpp's OnCreatureCreate twilightLordKelrisGUID arm, ramstein
	// strength): Mind Blast 15587 on the victim (2-5s init / 7-9s repeat,
	// jeklik GetVictim + CastSpell convention, non-triggered) and Sleep
	// 8399 on a random alive player (9-12s init / 15-20s repeat, janalai
	// randomPlayerInRange convention, non-triggered, Talk SAY_SLEEP);
	// aggro Talk SAY_AGGRO plus RemoveAura of the 8734 channeling aura
	// (Reset casts it on self — C++ Reset/JustReachedHome legs), death
	// Talk SAY_DEATH; melee engine-driven. The BossAI _Reset/
	// _JustReachedHome/_JustDied instance legs and the GetInstanceAI
	// leg have no bridges; see lua_scripts/kalimdor/boss_kelris.lua.
	RegisterLuaBoss("boss_kelris", 4832)

	// boss_aku_mai (Aku'mai, entry 4829 — below kalecgos: BFDCreatureIds
	// names no Aku'mai entry and the instance OnCreatureCreate cases only
	// Kelris/Lorgus; registered on the DB ScriptName tie plus wowhead
	// npc=4829 corroboration, jeklik precedent): Poison Cloud 3815 on the
	// victim (5-9s init / 25-50s repeat, jeklik GetVictim + CastSpell
	// convention, non-triggered) and the sub-30% one-shot DamageTaken
	// enrage — non-triggered self-cast Frenzied Rage 3490 on projected health
	// (C++ DoCast(me, spell) defaults to TRIGGERED_NONE; jeklik pre-damage subtraction, C++-exact); melee engine-driven.
	// The BossAI _Reset/_JustDied instance legs and the GetInstanceAI leg
	// have no bridges; see lua_scripts/kalimdor/boss_aku_mai.lua.
	RegisterLuaBoss("boss_aku_mai", 4829)

	// instance_blackfathom_deeps.cpp audit (map 48, "BFD", Kalimdor loader
	// decl :24 / call :137 follows boss_aku_mai): InstanceMapScript only —
	// OnCreatureCreate latches Kelris 4832 GUID + randomizes Lorgus Jett
	// 12902 home position; OnGameObjectCreate latches 4 fires of Aku'mai
	// 21118-21121, shrine of Gelihast 103015, altar of the deeps 103016,
	// Aku'mai door 21117 (NOT_SELECTABLE gates on Gelihast/Aku'mai boss
	// state); SetData DATA_FIRE spawns snapjaw/softshell/servant/crustacean
	// packs via shrine1 GO, DATA_EVENT opens the main door at 18 deaths;
	// SetBossState DONE clears the shrine/altar flags + summons Morridune
	// 6729. No SummonCreature/DoCast/GameObject-flag/GetGuidData bridges —
	// joins the no-instance-script-bridge queue. BLACKFATHOM GROUP CLOSED
	// (blackfathom_deeps, gelihast, kelris, aku_mai, instance).

	// npc_jaina_proudmoore (Jaina, entry 17772) and npc_thrall (Thrall,
	// entry 17852 — C++-verified via hyjal.h HYCreaturesIds plus
	// instance_hyjal.cpp's OnCreatureCreate GUID-capture arms and the
	// matching DATA_JAINAPROUDMOORE/DATA_THRALL GetGuidData legs,
	// ramstein strength): the hyjalAI constructor spell tables (Blizzard
	// 31266 + Pyroblast 31263 random-player + Summon Elementals 31264
	// self for Jaina; Chain Lightning 31330 victim + Summon Dire Wolf
	// 31331 random-player for Thrall), non-triggered casts on the
	// constructor-rolled fixed cooldowns (C++-exact — urand evaluated
	// once at construction), engage Talk ATTACKED and death Talk DEATH;
	// melee engine-driven. The hyjalAI wave machine (StartEvent/Retreat/
	// IsDummy escort choreography) and all gossip arms (instance-data
	// gates) have no bridges; see lua_scripts/kalimdor/hyjal.lua.
	RegisterLuaBoss("npc_jaina_proudmoore", 17772)
	RegisterLuaBoss("npc_thrall", 17852)

	// hyjal_trash (Battle for Mount Hyjal trash — C++-verified entries via
	// hyjal.h HYCreaturesIds "Trash Mobs summoned in waves" enum; the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat spell legs (giant infernal 17908 Flame Buffet
	// 31724 + one-shot Immolation 37059; abomination 17898 Disease Cloud
	// 31607 refresh + Knockdown 31610; ghoul 17895 Frenzy 31540;
	// necromancer 17899 Shadow Bolt 31627; banshee 17905 Curse 31651 +
	// Wail 38183 + Anti-Magic Shell 31662; crypt fiend 17897 Web 28991;
	// gargoyle 17906 Gargoyle Strike 31664; frost wyrm 17907 Frost Breath
	// 31688; fel stalker 17916 Mana Burn 31729) on C++-verbatim timers.
	// The EscortAI wave/overrun escort machine, waypoint AddThreat legs,
	// instance DATA_RAIDDAMAGE/DATA_TRASH legs, flag/model/gravity/summon
	// arms, and necromancer KilledUnit skeleton spawns have no bridges;
	// see lua_scripts/kalimdor/hyjal_trash.lua. npc_alliance_rifleman is
	// NOT registered — no C++ entry evidence (gelihast precedent).
	RegisterLuaBoss("npc_giant_infernal", 17908)
	RegisterLuaBoss("npc_abomination", 17898)
	RegisterLuaBoss("npc_ghoul", 17895)
	RegisterLuaBoss("npc_necromancer", 17899)
	RegisterLuaBoss("npc_banshee", 17905)
	RegisterLuaBoss("npc_crypt_fiend", 17897)
	RegisterLuaBoss("npc_gargoyle", 17906)
	RegisterLuaBoss("npc_frost_wyrm", 17907)
	RegisterLuaBoss("npc_fel_stalker", 17916)

	// boss_rage_winterchill (Rage Winterchill, entry 17767 — C++-verified
	// via hyjal.h HYCreaturesIds "Bosses summoned after every 8 waves"
	// enum plus instance_hyjal.cpp's OnCreatureCreate GUID-capture case,
	// ramstein strength; the creature_template ScriptName binding stays
	// DB-side): the self-contained in-combat legs (Frost Armor 31256 self
	// init 37s -> {40s,60s}; Death and Decay 31258 victim init 45s ->
	// {60s,80s} + Talk SAY_DECAY; Frost Nova 31250 victim init 15s ->
	// {30s,45s} + Talk SAY_NOVA; Icebolt 31249 on a random alive player
	// within 40 init 10s -> {11s,31s}; engage Talk SAY_ONAGGRO, KilledUnit
	// Talk SAY_ONSLAY, death Talk SAY_ONDEATH) on C++-verbatim timers;
	// melee engine-driven. The IsEvent EscortAI escort machine (8
	// waypoints, waypoint-7 AddThreat on the instance Jaina GUID) and the
	// DATA_RAGEWINTERCHILLEVENT Reset/engage/death instance legs have no
	// bridges; see lua_scripts/kalimdor/boss_rage_winterchill.lua.
	RegisterLuaBoss("boss_rage_winterchill", 17767)

	// boss_anetheron (Anetheron, entry 17808 — C++-verified via hyjal.h
	// HYCreaturesIds "Bosses summoned after every 8 waves" enum plus
	// instance_hyjal.cpp's OnCreatureCreate GUID-capture case (:119) and
	// the DATA_ANETHERON GetGuidData leg (:155), ramstein strength; the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs (Carrion Swarm 31306 on a random alive
	// player within 100 init 45s -> {45s,60s} + Talk SAY_SWARM; Sleep
	// 31298 self-cast-on-target x3 random players within 100 init 60s ->
	// 60s + Talk SAY_SLEEP; Vampiric Aura 38196 self triggered init 5s ->
	// {10s,20s}; Inferno 31299 on a random alive player within 100 init
	// 45s -> 45s + Talk SAY_INFERNO; engage Talk SAY_ONAGGRO, KilledUnit
	// (player-gated) Talk SAY_ONSLAY, death Talk SAY_ONDEATH) on
	// C++-verbatim timers; melee engine-driven. The IsEvent EscortAI
	// escort machine and DATA_ANETHERONEVENT Reset/engage/death instance
	// legs have no bridges; npc_towering_infernal is unregistered (zero
	// C++ entry evidence, gelihast precedent) and the vampiric-aura
	// AuraScript proc has no AuraScript-handler bridge; see
	// lua_scripts/kalimdor/boss_anetheron.lua.
	RegisterLuaBoss("boss_anetheron", 17808)

	// boss_kazrogal (Kazrogal, entry 17888 — C++-verified via hyjal.h
	// HYCreaturesIds "Bosses summoned after every 8 waves" enum (:80) plus
	// instance_hyjal.cpp's OnCreatureCreate GUID-capture case (:122) and
	// the DATA_KAZROGAL GetGuidData leg (:156), ramstein strength; the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs (Cleave 31436 self non-triggered init
	// 5s -> {6s,21s}; War Stomp 31480 self non-triggered init 15s -> 60s;
	// Mark 31447 non-triggered self-cast init 45s, re-armed on the
	// C++-verbatim decreasing MarkTimerBase 45000 -> 40000 -> ... floor
	// 5500, Talk SAY_MARK on each expiry; engage Talk SAY_ONAGGRO,
	// KilledUnit Talk SAY_ONSLAY (no TYPEID gate in C++), no death Talk —
	// DoPlaySoundToSet(11018) is ported via creature:PlayDirectSound) on C++-verbatim
	// timers; melee engine-driven. The IsEvent EscortAI escort machine
	// (8 C++-verbatim waypoints, WaypointReached(7) AddThreat on the
	// DATA_THRALL GUID) and DATA_KAZROGALEVENT Reset/engage/death
	// instance legs have no bridges; the spell_mark_of_kazrogal
	// SpellScript target filter and AuraScript periodic handler have no
	// script bridges; see lua_scripts/kalimdor/boss_kazrogal.lua.
	RegisterLuaBoss("boss_kazrogal", 17888)

	// boss_azgalor (Azgalor, entry 17842 — C++-verified via hyjal.h
	// HYCreaturesIds "Bosses summoned after every 8 waves" enum (:81) plus
	// instance_hyjal.cpp's OnCreatureCreate GUID-capture case (:126) and
	// the DATA_AZGALOR GetGuidData leg (:157), ramstein strength; the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs (Rain of Fire 31340 on a random alive
	// player within 30 init 20s -> {20s,35s}; Doom 31347 on a random alive
	// player within 100 excluding the victim (C++ position 1 "never on
	// tank" has no threat-list bridge) init 50s -> {45s,50s}; Howl of
	// Azgalor 31344 self non-triggered init 30s -> 30s; Cleave 31345 on
	// victim init 10s -> {10s,15s}; Berserk 26662 triggered self one-shot
	// at 600s; engage Talk SAY_ONAGGRO (3), KilledUnit Talk SAY_ONSLAY
	// (1) with no TYPEID gate, death Talk SAY_ONDEATH (0); SAY_DOOM (2)
	// is marked "Not used?" in C++ and has no Talk arm) on C++-verbatim
	// timers; melee engine-driven. The IsEvent EscortAI escort machine
	// (8 C++-verbatim waypoints, WaypointReached(7) AddThreat on the
	// DATA_THRALL GUID) and DATA_AZGALOREVENT Reset/engage/death instance
	// legs have no bridges; npc_lesser_doomguard is unregistered (zero
	// C++ entry evidence, gelihast precedent) and its bridgeable arms are
	// documented in the header; see lua_scripts/kalimdor/boss_azgalor.lua.
	RegisterLuaBoss("boss_azgalor", 17842)

	// boss_archimonde (Archimonde, entry 17968 — C++-verified via hyjal.h
	// HYCreaturesIds "Bosses summoned after every 8 waves" enum (:82) plus
	// instance_hyjal.cpp's OnCreatureCreate GUID-capture case (:128) and
	// the DATA_ARCHIMONDE GetGuidData leg (:158), ramstein strength; the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs (pre-combat triggered Drain World
	// Tree 39140 self-cast on OnSpawn(5) — ACTION_CHANNEL_WORLD_TREE;
	// Fear 31970 self non-triggered init 42s -> 42s; Air Burst 32014 on
	// a random alive player excluding the victim (C++ position 1 "not on
	// tank" has no threat-list bridge) + Talk SAY_AIR_BURST (3) init 30s
	// -> {25s,40s}; Grip of the Legion 31972 on an unbounded random
	// alive player init {5s,25s} -> {5s,25s}; Finger of Death 31984 with
	// the C++ 5-yard melee check (no player in range -> cast on a random
	// alive player, re-arm 1s; else re-arm 5s) init 15s; Hand of Death
	// 35354 self non-triggered init 10min -> 2s; soul-charge state machine
	// as Lua counters per C++ class bucket (priest/paladin/warlock -> red,
	// mage/rogue/warrior -> yellow, druid/shaman/hunter -> green), unleash
	// urand(0,2) DoCastVictim with the C++ no-aura no-re-arm rule;
	// DamageTaken 10% leg: Talk SAY_ENRAGE (5) + triggered Protection of
	// Elune 38528 self-cast; engage Talk SAY_AGGRO (1), KilledUnit Talk
	// SAY_SLAY (4), death Talk SAY_DEATH (6)) on C++-verbatim timers;
	// melee engine-driven. The Doomfire summon choreography, distance
	// check on DATA_CHANNEL_TARGET, wisp phase, instance DATA_ARCHIMONDE
	// legs, and the drain-dummy SpellScript have no bridges; the three
	// add scripts (npc_doomfire / npc_doomfire_targetting /
	// npc_ancient_wisp) are unregistered (zero C++ entry evidence,
	// gelihast precedent) with their bridgeable arms documented in the
	// header; see lua_scripts/kalimdor/boss_archimonde.lua.
	RegisterLuaBoss("boss_archimonde", 17968)

	// instance_hyjal.cpp audit (map 534, "HY", Kalimdor loader decl :28 /
	// call :141 follows boss_archimonde): InstanceMapScript only —
	// OnCreatureCreate latches the 4 boss + Jaina 17772 / Thrall 17852 /
	// Tyrande 17948 GUIDs (Archimonde hidden+passive until Azgalor DONE);
	// OnGameObjectCreate latches horde/elf gates + ancient gems; SetData
	// drives the 5-encounter array, Azgalor-DONE unhides Archimonde +
	// one-shot YELL_ARCHIMONDE_INTRO (8), retreat/TRASH/RaidDamage/world
	// states; GetData/GetGuidData/Load (IN_PROGRESS->NOT_STARTED) /save.
	// No instance-script/GUID/GO-flag bridges — joins the
	// no-instance-script-bridge queue. MOUNT HYJAL GROUP: hyjal, archimonde,
	// instance done; rage_winterchill/anetheron/kazrogal/azgalor/hyjal_trash
	// remain.

	// boss_captain_skarloc (Captain Skarloc, entry 17862 — C++-verified
	// via old_hillsbrad.cpp:151's ENTRY_SCARLOC constant (C++ spelling
	// "SCARLOC") plus its Thrall-escort SummonCreature call (:263); the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs (engage Talk SAY_TAUNT1 (1) +
	// SAY_TAUNT2 (2); KilledUnit Talk SAY_SLAY (3) no TYPEID gate; death
	// Talk SAY_DEATH (4); Holy Light 29427 self non-triggered init
	// {20s,30s} -> 30s; Cleanse 29380 self non-triggered init 10s -> 10s;
	// Hammer of Justice 13005 on the victim (jeklik GetVictim + CastSpell
	// convention) non-triggered init {20s,35s} -> 60s; Holy Shield 31904
	// self non-triggered init 240s -> 240s; Devotion Aura 8258 self
	// non-triggered init 3s -> {45s,55s}) on C++-verbatim timers; melee
	// engine-driven. The JustDied TYPE_THRALL_EVENT/TYPE_THRALL_PART1
	// instance leg has no instance-data bridge; SPELL_CONSECRATION's
	// C++ cast is commented out (no observable behavior to port);
	// SAY_ENTER 0 is declared but never Talked in C++; the SD%Complete:
	// 75 missing adds, spawn waypoints, and pre-combat Thrall speech need
	// summon and EscortAI movement bridges that do not exist; see
	// lua_scripts/kalimdor/boss_captain_skarloc.lua.
	RegisterLuaBoss("boss_captain_skarloc", 17862)

	// boss_epoch_hunter (Epoch Hunter, entry 18096 — C++-verified via
	// old_hillsbrad.cpp:174's ENTRY_EPOCH constant plus its Thrall-escort
	// SummonCreature call (:615); the creature_template ScriptName
	// binding stays DB-side; not GUID-bound in instance_old_hillsbrad.cpp
	// — gossip-summoned): the self-contained in-combat legs (engage Talk
	// SAY_AGGRO (1); KilledUnit Talk SAY_SLAY (2) no TYPEID gate; death
	// Talk SAY_DEATH (4); Sand Breath 31914 on the victim non-triggered
	// init {8s,16s} -> {10s,20s} + unconditional Talk SAY_BREATH (3);
	// Impending Death 31916 on the victim non-triggered init {25s,30s}
	// -> 25000 + rand32() % 5000; Wing Buffet 31475 on a random alive
	// player in the instance (nefarian convention) non-triggered init
	// 35s -> 25000 + rand32() % 10000; Magic Disruption Aura 33834 self
	// non-triggered init 40s -> 15s) on C++-verbatim timers; melee
	// engine-driven. The JustDied TYPE_THRALL_EVENT/TYPE_THRALL_PART4
	// instance leg has no instance-data bridge; Sand Breath's
	// InterruptNonMeleeSpells arm is bridged (no-op twin) and ported; SAY_ENTER 0 is declared but never Talked in C++; the
	// SD%Complete: 60 missing pre-event spawns and uncoordinated escort
	// speech need summon and EscortAI bridges that do not exist; see
	// lua_scripts/kalimdor/boss_epoch_hunter.lua.
	RegisterLuaBoss("boss_epoch_hunter", 18096)

	// boss_lieutenant_drake (Lieutenant Drake, entry 17848 — C++-verified
	// via instance_old_hillsbrad.cpp:39's DRAKE_ENTRY constant plus its
	// SetData-driven SummonCreature call (:149) after the fifth barrel
	// gossip; the creature_template ScriptName binding stays DB-side; not
	// GUID-bound in instance_old_hillsbrad.cpp — gossip-summoned): the
	// self-contained in-combat legs (engage Talk SAY_AGGRO (1); KilledUnit
	// Talk SAY_SLAY (2) no TYPEID gate; death Talk SAY_DEATH (5);
	// Whirlwind 31909 on the victim non-triggered init 20s -> 20000 +
	// rand32() % 5000; Frightening Shout 33789 on the victim
	// non-triggered init 30s -> 25000 + rand32() % 10000 + Talk
	// SAY_SHOUT (4); Mortal Strike 31911 on the victim non-triggered
	// init 45s -> 20000 + rand32() % 10000 + Talk SAY_MORTAL (3)) on
	// C++-verbatim timers; melee engine-driven. The CanPatrol/wpId
	// MovePoint patrol machine (@todo make this work in C++; 19 waypoints
	// C++-verbatim in the .lua header) has no movement bridge;
	// ExplodingShout_Timer's 25000 init is never expired by UpdateAI (no
	// observable behavior); SAY_ENTER 0 is declared but never Talked in
	// C++; go_barrel_old_hillsbrad's OnGossipHello SetData(IN_PROGRESS)
	// legs have no instance-data or gameobject-gossip bridge, so the
	// barrel script is documented, not registered — see
	// lua_scripts/kalimdor/boss_lieutenant_drake.lua.
	RegisterLuaBoss("boss_lieutenant_drake", 17848)

	// npc_thrall_old_hillsbrad (Thrall, entry 17876 — C++-verified via
	// old_hillsbrad.h:35's THRALL_ENTRY constant plus
	// instance_old_hillsbrad.cpp:108's OnCreatureCreate case (ThrallGUID
	// capture); the creature_template ScriptName binding stays DB-side):
	// the self-contained in-combat legs only (legoso combat-rotation-only
	// precedent) — engage Talk SAY_TH_RANDOM_AGGRO (13); 1s pump for the
	// UpdateAI LowHp latch (Talk SAY_TH_RANDOM_LOW_HP (11) at <20% hp,
	// cleared on Reset); KilledUnit Talk SAY_TH_RANDOM_KILL (14), no
	// TYPEID gate; death Talk SAY_TH_RANDOM_DIE (12) unless killer == me;
	// melee engine-driven. The entire EscortAI waypoint/summon/mount
	// machine (24 waypoint cases, C++-verbatim in the .lua header), all
	// gossip legs (Thrall's start/skarloc/tarren chains, Taretha's epoch
	// chain, Erozion's bomb-grant/teleport), the JustSummoned
	// AttackStart leg, and the instance SetData(TYPE_THRALL_EVENT/PART*)
	// legs have no movement / summon-with-position / gossip / inventory /
	// instance-data bridges — documented in
	// lua_scripts/kalimdor/old_hillsbrad.lua, not wired. npc_erozion
	// (gossip-only) and npc_taretha (EscortAI, zero bridgeable arms) are
	// documented-only, not registered — see the .lua header; both entries
	// (TARETHA_ENTRY 18887 via old_hillsbrad.h:36 +
	// instance_old_hillsbrad.cpp:111; EROZION_ENTRY 18723 via this cpp's
	// ThrallOldHillsbrad enum + the waypoint-106 summon) are
	// C++-verifiable.
	RegisterLuaBoss("npc_thrall_old_hillsbrad", 17876)

	// boss_aeonus (Aeonus, entry 17881 — C++-verified via
	// the_black_morass.h:62's NPC_AEONUS constant plus
	// instance_the_black_morass.cpp:64's RiftWaves summon and :306's
	// GetEntry() == NPC_AEONUS medivh-threat check; the creature_template
	// ScriptName binding stays DB-side): the self-contained in-combat
	// legs only — engage Talk SAY_AGGRO (1) + arm the three timers at
	// their C++ ScheduleEvent cooldowns; Sand Breath 31473 and Time Stop
	// 31422 on the victim (jeklik GetVictim + CastSpell convention);
	// Frenzy Talk EMOTE_FRENZY (5) + self-cast Enrage 37605; KilledUnit
	// Talk SAY_SLAY (3) gated on the victim being a player (C++'s
	// TYPEID_PLAYER leg — terestian_illhoof convention); death Talk
	// SAY_DEATH (4); melee engine-driven. The MoveInLineOfSight Time
	// Keeper leg (SAY_BANISH (2) + full-health one-shot of NPC_TIME_KEEPER
	// 17918 within 20yd) has no nearby-creature enumeration bridge, and
	// the JustDied instance->SetData(TYPE_RIFT/TYPE_MEDIVH, DONE) legs
	// have no instance-data bridge (standing) — documented in
	// lua_scripts/kalimdor/boss_aeonus.lua, not wired. SPELL_CLEAVE 40504
	// and H_SPELL_SAND_BREATH 39049 are declared in the C++ enum but
	// never used by UpdateAI; SAY_ENTER 0 is never Talked in C++.
	RegisterLuaBoss("boss_aeonus", 17881)

	// boss_chrono_lord_deja (Chrono Lord Deja, entry 17879 — C++-verified
	// via the_black_morass.h:60's NPC_CRONO_LORD_DEJA constant (C++
	// spells the constant "CRONO" while the script name spells it
	// "chrono") plus instance_the_black_morass.cpp:60's RiftWaves second
	// wave portal boss summon; the creature_template ScriptName binding
	// stays DB-side): the self-contained in-combat legs only — engage
	// Talk SAY_AGGRO (1) + arm the three timers at their C++
	// ScheduleEvent cooldowns; Arcane Blast 31457 on the victim (jeklik
	// GetVictim + CastSpell convention); Time Lapse Talk SAY_BANISH (2)
	// + self-cast 31467; Arcane Discharge 31472 on an unbounded random
	// alive player (nil target -> no cast, nefarian convention);
	// KilledUnit Talk SAY_SLAY (3) with no TYPEID gate (C++ talks
	// unconditionally here, unlike aeonus); death Talk SAY_DEATH (4);
	// melee engine-driven. The MoveInLineOfSight Time Keeper leg
	// (SAY_BANISH (2) + full-health one-shot of NPC_TIME_KEEPER 17918
	// within 20yd) has no nearby-creature enumeration bridge, the
	// JustDied instance->SetData(TYPE_RIFT, SPECIAL) leg has no
	// instance-data bridge (standing), and the heroic-only Attraction
	// 38540 arm has no difficulty bridge — documented in
	// lua_scripts/kalimdor/boss_chrono_lord_deja.lua, not wired.
	// H_SPELL_ARCANE_BLAST 38538 / H_SPELL_ARCANE_DISCHARGE 38539 are
	// declared but never cast by UpdateAI; SAY_ENTER 0 is never Talked
	// in C++.
	RegisterLuaBoss("boss_chrono_lord_deja", 17879)

	// boss_temporus (Temporus, entry 17880 — C++-verified via
	// the_black_morass.h:61's NPC_TEMPORUS constant plus
	// instance_the_black_morass.cpp:62's RiftWaves wave-4 portal boss
	// summon with a 140s NextPortalTime; the creature_template ScriptName
	// binding stays DB-side): the self-contained in-combat legs only —
	// engage Talk SAY_AGGRO (1) + arm the three timers at their C++
	// ScheduleEvent cooldowns; Haste 31458, Mortal Wound 31464 and Wing
	// Buffet 31475 self-cast; KilledUnit Talk SAY_SLAY (3) with no
	// TYPEID gate (C++ talks unconditionally here, like deja, unlike
	// aeonus); death Talk SAY_DEATH (4); melee engine-driven. The
	// MoveInLineOfSight Time Keeper leg (SAY_BANISH (2) + full-health
	// one-shot of NPC_TIME_KEEPER 17918 within 20yd) has no
	// nearby-creature enumeration bridge, the JustDied
	// instance->SetData(TYPE_RIFT, SPECIAL) leg has no instance-data
	// bridge (standing), and the heroic-only Spell Reflection 38592 arm
	// (C++ marks it "//Not Implemented (Heroic mod)") has no difficulty
	// bridge — documented in lua_scripts/kalimdor/boss_temporus.lua, not
	// wired. H_SPELL_WING_BUFFET 38593 is declared but never cast by
	// UpdateAI; SAY_ENTER 0 is never Talked in C++.
	RegisterLuaBoss("boss_temporus", 17880)

	// the_black_morass (zone script, NPC_MEDIVH entry 15608 —
	// C++-verified via the_black_morass.h:55's NPC_MEDIVH constant plus
	// instance_the_black_morass.cpp:137's OnCreatureCreate GUID bind
	// (DATA_MEDIVH); the creature_template ScriptName binding stays
	// DB-side): death Talk SAY_DEATH (5), skipped when the killer has
	// Medivh's own entry (C++ JustDied's entry gate, the thrall
	// old_hillsbrad self-kill-skip convention). Everything else is
	// instance-script machinery with no bridges — Reset's
	// GetData(TYPE_MEDIVH)/SPELL_CHANNEL aura management, the
	// MoveInLineOfSight player-intro leg (SAY_INTRO + SetData
	// IN_PROGRESS + SPELL_CHANNEL cast) and infinite-mob leg
	// (StopMoving + who->CastSpell SPELL_CORRUPT 31326 /
	// SPELL_CORRUPT_AEONUS 37853), SpellHit (never fires in the Lua
	// API, standing queue), and the whole UpdateAI machine
	// (TYPE_MEDIVH SPECIAL, DATA_SHIELD Life75/50/25 Talk latches
	// SAY_WEAK75/50/25, NOT_STARTED despawn-respawn, TYPE_RIFT DONE ->
	// SAY_WIN + SetData DONE) — documented in
	// lua_scripts/kalimdor/the_black_morass.lua, not wired. npc_time_rift
	// (NPC_TIME_RIFT 17838) is DOCUMENTED-not-registered: the 15s
	// PortalWaves wave pump needs summon + instance-data bridges that
	// do not exist (standing).
	RegisterLuaBoss("the_black_morass", 15608)

	// boss_kurinnaxx (Kurinnaxx, entry 15348 — C++-verified via
	// ruins_of_ahnqiraj.h:41's NPC_KURINNAXX constant; the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs only — engage arms the four timers
	// at their C++ ScheduleEvent cooldowns (no Talk in C++
	// JustEngagedWith); Mortal Wound 25646 on the victim (jeklik
	// GetVictim + CastSpell convention, 8s -> 8s); Sand Trap 25648 on a
	// bounded random alive player within 100 (nil target -> C++'s
	// victim-fallback wired: the victim casts on itself, {5s,15s} ->
	// {5s,15s}); Wide Slash 25814 self-cast (11s -> 11s); Trash 3391
	// self-cast fired once at 1s (C++ EVENT_TRASH reschedules
	// EVENT_WIDE_SLASH at 15s instead of itself — C++-verbatim);
	// DamageTaken enrage 26527 below 30% health, latched per GUID
	// (moroes event-9 convention); melee engine-driven. The
	// while(ExecuteEvent()) + UNIT_STATE_CASTING gate has no bridge
	// (aeonus precedent). JustDied's Ossirian SAY_KURINNAXX_DEATH yell
	// (ObjectAccessor::GetCreature(*me,
	// instance->GetGuidData(DATA_OSSIRIAN))) and the _JustDied() /
	// DATA_KURINNAXX boss-state legs have no instance-data bridge
	// (standing) — documented in lua_scripts/kalimdor/boss_kurinnaxx.lua,
	// not wired.
	RegisterLuaBoss("boss_kurinnaxx", 15348)

	// boss_rajaxx (General Rajaxx, entry 15341 — C++-verified via
	// ruins_of_ahnqiraj.h:42's NPC_RAJAXX constant; the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs only — engage arms the two timers
	// at their C++ ScheduleEvent cooldowns (zero Talk() calls in C++;
	// the Yells enum belongs to the Andorov event NPCs); Disarm 6713
	// on the victim (jeklik GetVictim + CastSpell convention, 10s ->
	// 22s); Thunder Crash 25599 self-cast (12s -> 21s); melee
	// engine-driven. The while(ExecuteEvent()) + UNIT_STATE_CASTING
	// gate has no bridge (aeonus precedent); spell_rajaxx_thundercrash
	// (SpellScript damage calc) has no SpellScript bridge (standing);
	// EVENT_CHANGE_AGGRO and the enraged bool are dead in C++ (never
	// used) — nothing to port. BossAI's ctor leg DATA_RAJAXX and
	// _Reset() ride the absent instance bridge — documented in
	// lua_scripts/kalimdor/boss_rajaxx.lua, not wired.
	RegisterLuaBoss("boss_rajaxx", 15341)

	// boss_moam (Moam, entry 15340 — C++-verified via
	// ruins_of_ahnqiraj.h:43's NPC_MOAM constant; the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs only — engage schedules stone
	// phase at 90s (C++ Reset); DamageTaken below 45% health enters
	// stone phase (moroes event-9 convention); stone phase casts
	// SUMMON_MANA_FIEND 1/2/3 (25681/25682/25683) + ENERGIZE 25685
	// and exits after 90s via RemoveAura(ENERGIZE) (netherspite
	// convention), rescheduling the next phase. Zero Talk() calls in
	// C++ (the Texts enum is unused). Unwired: mana-full Arcane
	// Eruption 25672 + SetPower(MANA, 0) — no SetPower bridge for
	// creatures (curator precedent); EVENT_DRAIN_MANA never scheduled
	// in C++ Reset (dead); SPELL_TRAMPLE never cast (dead);
	// EVENT_WIDE_SLASH/TRASH commented out (inactive). BossAI's ctor
	// leg DATA_MOAM and _Reset() ride the absent instance bridge —
	// documented in lua_scripts/kalimdor/boss_moam.lua, not wired.
	RegisterLuaBoss("boss_moam", 15340)

	// boss_buru (Buru the Gorger, entry 15370 — C++-verified via
	// ruins_of_ahnqiraj.h:44's NPC_BURU constant; the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs only — engage Talks EMOTE_TARGET
	// at the target, self-casts THORNS 25640, arms DISMEMBER 5s,
	// GATHERING_SPEED 9s, FULL_SPEED 60s (phase EGG); Dismember 96
	// on the victim (jeklik GetVictim + CastSpell convention, 5s);
	// Gathering Speed 1834 self-cast (9s); Full Speed 1557 self-cast
	// one-shot; the per-tick <20% health transform (checked on
	// damage, event 9) casts BURU_TRANSFORM 24721 + FULL_SPEED and
	// strips THORNS (netherspite RemoveAura convention), phase
	// TRANSFORM; KilledUnit on a player re-arms the speed timers
	// (aura strip + reschedule). Unwired: ChaseNewVictim's
	// SelectTarget/ResetThreatList/AttackStart (no Lua bridge for any
	// — zero usage in lua_scripts); npc_buru_egg's cross-AI egg legs
	// (instance GetGuidData + dynamic_cast + ManageRespawn);
	// spell_egg_explosion (SpellScript); ACTION_EXPLODE's self-damage;
	// EVENT_CREEPING_PLAGUE never scheduled in C++ (dead);
	// EVENT_RESPAWN_EGG (unbridged egg list); EnterEvadeMode egg
	// respawn. BossAI's ctor leg DATA_BURU and _Reset() ride the
	// absent instance bridge — documented in
	// lua_scripts/kalimdor/boss_buru.lua, not wired.
	RegisterLuaBoss("boss_buru", 15370)

	// boss_ayamiss (Ayamiss the Hunter, entry 15369 — C++-verified
	// via ruins_of_ahnqiraj.h:45's NPC_AYAMISS constant; the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs only — engage arms STINGER_SPRAY
	// (20-30s -> 15-20s), POISON_STINGER (5s -> 2-3s), PARALYZE
	// (15s); Stinger Spray 25749 self-cast; Poison Stinger 25748 on
	// the victim (jeklik GetVictim + CastSpell convention); Paralyze
	// 25725 on a random player via GetPlayersInWorld filtered by
	// map/instance/alive (maiden_of_virtue convention — C++
	// SelectTarget has no bridge); the <70% ground-phase transition
	// (checked on damage, event 9) cancels Poison Stinger and arms
	// LASH 25852 (5-8s -> 8-15s) + TRASH 3391 (3-6s -> 5-7s), both on
	// the victim; the <20% frenzy (event 9, latched per kurinnaxx
	// enrage convention) self-casts FRENZY 8269 + Talk(EMOTE_FRENZY
	// 0). Unwired: all motion (SetCanFly/SetDisableGravity/MovePoint/
	// SetCombatMovement/UNIT_STATE_ROOT — no bridge); SummonCreature
	// (swarmers 15546 / larvae 15555 — zero Lua usage); the swarmer
	// AttackStart cross-AI; npc_hive_zara_larva (instance boss-state
	// gated); instance SetGuidData(DATA_PARALYZED). BossAI's ctor leg
	// DATA_AYAMISS and _Reset() ride the absent instance bridge —
	// documented in lua_scripts/kalimdor/boss_ayamiss.lua, not wired.
	RegisterLuaBoss("boss_ayamiss", 15369)

	// boss_ossirian (Ossirian the Unscarred, entry 15339 —
	// C++-verified via ruins_of_ahnqiraj.h:46's NPC_OSSIRIAN
	// constant; the creature_template ScriptName binding stays
	// DB-side): the self-contained in-combat legs only — engage
	// self-casts SUPREME 25176 + Talk(SAY_AGGRO 2), arms SILENCE
	// (30s -> 20-30s), CYCLONE (20s), STOMP (30s); Curse of Tongues
	// 25195 on all live instance players (C++ DoCastAOE — no AoE
	// bridge, maiden GetPlayersInWorld convention); Cyclone 25189
	// on the victim (jeklik convention); Stomp 25188 self-cast;
	// the per-tick Supreme reapply (HasAura-gated, netherspite
	// convention) + SAY_SUPREME on a 5s timer; the 60-120yd
	// no-kiting summon (moroes GetDistance convention) on a 5s
	// timer; KilledUnit Talk(SAY_SLAY 3) via event 3; SpellHit
	// weakness -> RemoveAura(SUPREME) + crystal Delete +
	// SpawnNextCrystal via event 14 (midnight convention); engage
	// summons the crystal machine (trigger 15590 + crystal GO
	// 180619, 12-coord cycle). Unwired: weather; go_ossirian_crystal
	// (GameObjectAI); MoveInLineOfSight intro; DoAction weakness
	// trigger (cross-AI); Cleanup/JustDied/Evade instance legs.
	// BossAI's ctor leg DATA_OSSIRIAN and _Reset() ride the absent
	// instance bridge — documented in
	// lua_scripts/kalimdor/boss_ossirian.lua, not wired.
	RegisterLuaBoss("boss_ossirian", 15339)

	// boss_viscidus (Viscidus, entry 15299 — C++-verified via
	// temple_of_ahnqiraj.h:68's NPC_VISCIDUS constant; the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs only — engage (JustEngagedWith
	// -> InitSpells) self-casts TOXIN 26575 and arms POISONBOLT_VOLLEY
	// 25991 (10-15s -> 10-15s) + POISON_SHOCK 25993 (7-12s ->
	// 7-12s). Unwired: the SpellHit frost-slow/freeze machine (no
	// spell-school bridge — any-spellId matching would misfire, so
	// the SLOWED/SLOWED_MORE/FREEZE + EMOTE_SLOW/FREEZE/FROZEN
	// progression and the 15s EVENT_RESET_PHASE stay out); the
	// DamageTaken hitcounter (dead in Lua while the MELEE phase is
	// unreachable — CRACK/SHATTER/EXPLODE emotes + EXPLODE cast);
	// the glob ring (SummonCreature — zero Lua usage) and
	// npc_glob_of_viscidus 15667 JustDied/MovementInform legs
	// (cross-AI/instance/motion bridges absent — no file created,
	// no-placeholder rule); JustDied suicide cast 26003; evade
	// cleanup; BossAI ctor leg DATA_VISCIDUS. Full analysis in
	// lua_scripts/kalimdor/boss_viscidus.lua.
	RegisterLuaBoss("boss_viscidus", 15299)

	// boss_fankriss (Fankriss the Unyielding, entry 15510 — no
	// NPC_FANKRISS constant in temple_of_ahnqiraj.h; armory npc=15510;
	// the creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs only — engage arms the mortal-wound
	// timer at its C++ urand(10000,15000) init (no Talk in C++ —
	// "sound not implemented", zero Talk() calls); Mortal Wound 28467
	// on the victim (DoCastVictim, GetVictim nil-guarded,
	// urand(10000,20000)). Unwired: the spawn/hatchling machine —
	// DoSpawnCreature(15630) + me->SummonCreature(15962) have zero
	// Lua usage, DoTeleportPlayer and ModifyThreatByPercent have no
	// bridges, SelectTarget Random none either (casting Root 28858
	// alone without the teleport/summon legs would misrepresent the
	// mechanic); the GetThreat/HealthAbovePct(3) gates; evade cleanup;
	// BossAI ctor leg DATA_FRANKRIS. Full analysis in
	// lua_scripts/kalimdor/boss_fankriss.lua.
	RegisterLuaBoss("boss_fankriss", 15510)

	// boss_huhuran (Huhuran, entry 15509 — C++-verified via armory
	// npc=15509 and temple_of_ahnqiraj.h sequence adjacency; the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs only — Frenzy 26051 self-cast +
	// Talk(EMOTE_FRENZY_KILL 0) on a 25-35s timer (latched, with
	// 15s FrenzyBack re-arm); Wyvern Sting 26180 on random player
	// via GetPlayersInWorld (18-28s -> 15-32s); Acid Spit 26050 on
	// victim (8s -> 5-10s, jeklik convention); Noxious Poison 26053
	// on victim (10-20s -> 12-24s); Poison Bolt 26052 on victim
	// while Frenzy/Berserk (3s, C++-exact flag gate); Berserk
	// 26068 + Talk(EMOTE_BERSERK 1) at <31% health via event 9
	// (latched). InterruptNonMeleeSpells arms ported (frenzyback + berserk,
	// no-op twin). Unwired:
	// BossAI ctor leg DATA_HUHURAN. Full analysis in
	// lua_scripts/kalimdor/boss_huhuran.lua.
	RegisterLuaBoss("boss_huhuran", 15509)

	// boss_kri (Kri, entry 15511 — Bug Trio; the creature_template
	// ScriptName binding stays DB-side): Cleave 26350 on victim
	// (4-8s -> 5-12s); Toxic Volley 25812 on victim (6-12s ->
	// 10-15s); Poison Cloud 38718 on victim at <5% health via
	// event 9 (latched). Unwired: Vem-dead Enrage (instance
	// DATA_VEMISDEAD); JustDied loot/instance legs. Full analysis
	// in lua_scripts/kalimdor/boss_kri.lua.
	RegisterLuaBoss("boss_kri", 15511)

	// boss_vem (Vem, entry 15544 — Bug Trio): Charge 26561 on
	// random player via GetPlayersInWorld (15-27s -> 8-16s; the
	// AttackStart leg has no bridge); Knockback 26027 on victim
	// (8-20s -> 15-25s; threat modify has no bridge). Enrage 34624
	// is NOT ported: dead code in this tree — the else-branch at
	// boss_bug_trio.cpp:219 decrements Charge_Timer instead of
	// Enrage_Timer, so it never ticks down and the cast never fires.
	// Unwired: JustDied instance legs. Full analysis in
	// lua_scripts/kalimdor/boss_vem.lua.
	RegisterLuaBoss("boss_vem", 15544)

	// boss_yauj (Princess Yauj, entry 15543 — Bug Trio): Fear 19408
	// on victim (12-24s -> 20s; ResetThreatList has no bridge).
	// Unwired: Heal (instance target selection); Vem-dead Enrage;
	// JustDied summons (no SummonCreature bridge). Full analysis in
	// lua_scripts/kalimdor/boss_yauj.lua.
	RegisterLuaBoss("boss_yauj", 15543)

	// boss_sartura (Battleguard Sartura, entry 15516 —
	// C++-verified via temple_of_ahnqiraj.h:76 NPC_SARTURA; the
	// creature_template ScriptName binding stays DB-side): Whirlwind
	// 26083 self-cast (30s init -> 25-40s, 15s whirlwind mode);
	// Enrage 28747 self-cast at <20% health via event 9 (latched,
	// jeklik convention); hard Enrage 28798 self-cast at 10min
	// (latched); Talk SAY_AGGRO 0 / SAY_SLAY 1 / SAY_DEATH 2.
	// Unwired: whirlwind + AggroReset random-target switching
	// (no SelectTarget/AttackStart bridge); BossAI ctor leg
	// DATA_SARTURA (instance bridge); at_aq_battleground_sartura
	// AreaTrigger 4052; npc_sartura_royal_guard (entry not in the
	// C++ tree — left unbound). Full analysis in
	// lua_scripts/kalimdor/boss_sartura.lua.
	RegisterLuaBoss("boss_sartura", 15516)

	// boss_skeram (Prophet Skeram, entry 15263 —
	// C++-verified via temple_of_ahnqiraj.h:71 NPC_SKERAM; the
	// creature_template ScriptName binding stays DB-side): Arcane
	// Explosion 26192 self-cast (6-12s init -> 8-18s); True
	// Fulfillment 785 on a random player via GetPlayersInWorld
	// (15s -> 20-30s); Blink self-cast from {4801, 8195, 20449}
	// (30-45s init -> 10-30s); Earth Shock 26194 victim-cast
	// (2s repeat); Split Talk SAY_SPLIT at 75/50/25% via event 9
	// (latched, blink re-armed to 2s); Talk SAY_AGGRO 0 /
	// SAY_SLAY 1 / SAY_DEATH 3. Unwired: image summons
	// (SPELL_SUMMON_IMAGES 747 — no SummonCreature bridge);
	// JustSummoned positioning + health seeding; Blink
	// ResetThreatList/SetVisible; Arcane Explosion + True
	// Fulfillment SpellScripts; JustDied !IsSummon gate (no
	// IsSummon bridge — DEATH Talk ungated); BossAI ctor leg
	// DATA_SKERAM (instance bridge). Full analysis in
	// lua_scripts/kalimdor/boss_skeram.lua.
	RegisterLuaBoss("boss_skeram", 15263)

	// boss_veknilash (Emperor Vek'nilash, entry 15275 —
	// C++-verified via temple_of_ahnqiraj.h:75 NPC_VEKNILASH; the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs only — Unbalancing Strike
	// 26613 victim-cast (8-18s init -> 8-20s); Uppercut 26007
	// (C++ random melee target — no range/SelectTarget bridge,
	// ported as victim-cast, kri convention; 14-29s init ->
	// 15-30s); Berserk 26662 self-cast at 15min (60min repeat).
	// Unwired: shared twin health + SpellHit heal-brother
	// rebalance (cross-boss via instance->GetCreature(DATA_VEKLOR)
	// — no instance bridge); TryHealBrother (cross-boss); the
	// 30s teleport swap + SetAfterTeleport stun/visual 26638 +
	// post-teleport re-arm (no position/SelectTarget/
	// ResetThreatList/AttackStart bridges; InterruptNonMeleeSpells now bridged
	// but the swap stays blocked on the rest); HandleBugs + Mutate Bug
	// 802 (creature grid + SetFaction + AttackStart — no bridges);
	// Aggro/Kill/Death DoPlaySoundToSet 8661/8662/8660 (ported via
	// creature:PlayDirectSound; zero Talk in C++); TwinReset/BossAI ctor leg
	// DATA_TWIN_EMPERORS (instance bridge). Full analysis in
	// lua_scripts/kalimdor/boss_veknilash.lua.
	RegisterLuaBoss("boss_veknilash", 15275)

	// boss_veklor (Emperor Vek'lor, entry 15276 —
	// C++-verified via temple_of_ahnqiraj.h:74 NPC_VEKLOR; the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs only — Shadow Bolt 26006
	// victim-cast (2s repeat; the <45y MoveChase leg has no motion
	// bridge — fankriss convention); Blizzard 26607 on a random
	// player via GetPlayersInWorld (15-20s init -> 15-30s,
	// huhuran convention); Berserk 26662 self-cast at 15min
	// (60min repeat). VL does not melee (C++ AttackStart override
	// chases at 20y — motion bridge absent). Unwired: shared twin
	// health (no instance bridge); the 30s teleport swap +
	// SetAfterTeleport stun/visual + post-teleport Arcane Burst 5s
	// re-arm (no position/threat/interrupt bridges); Arcane Burst
	// 568 nearest-melee punishment (no range bridge); HandleBugs +
	// Explode Bug 804 (no creature-grid/SetFaction bridges);
	// Aggro/Kill/Death DoPlaySoundToSet 8657/8658/8659 (ported via
	// creature:PlayDirectSound; zero Talk in C++); TwinReset/BossAI ctor leg
	// DATA_TWIN_EMPERORS (instance bridge). Full analysis in
	// lua_scripts/kalimdor/boss_veklor.lua.
	RegisterLuaBoss("boss_veklor", 15276)

	// boss_ouro (Ouro, entry 15517 — the creature_template
	// ScriptName binding stays DB-side): the self-contained
	// in-combat legs only — engage Birth 26262 on victim; Sweep
	// 26103 on victim (5-10s -> 15-30s, !Submerged gated); Sand
	// Blast legs; Submerge cycle (60-120s) with submerged-state
	// gating suppressing Sweep/SandBlast. Zero Talk() calls in C++.
	// Unwired: instance legs. Full analysis in
	// lua_scripts/kalimdor/boss_ouro.lua.
	RegisterLuaBoss("boss_ouro", 15517)

	// npc_anubisath_sentinel (Anubisath Sentinel, entry 15264 —
	// C++-verified via the sentinel's own grid-search constant at
	// mob_anubisath_sentinel.cpp:159; the creature_template ScriptName
	// binding stays DB-side): on EVERY enter combat, picks one of the 9
	// sentinel ability buffs (2147/21737/812/13022/19595/25777/2834/9347/
	// 2148) uniformly at random and self-casts it (75dd5ef5 corrected
	// the pick to re-arm per engage, matching C++ JustEngagedWith).
	// Unwired: the whole buddy machinery (grid scan, buddy lists,
	// CallBuddiesToAttack, JustDied 50% heal + ability transfer, Reset
	// respawn — no ObjectAccessor / cross-AI / AttackStart bridges).
	// Full analysis in lua_scripts/kalimdor/npc_anubisath_sentinel.lua.
	RegisterLuaBoss("npc_anubisath_sentinel", 15264)

	// instance_temple_of_ahnqiraj.cpp audit (loader decl :93 / call :210):
	// pure InstanceMapScript (map 531, "AQ40") — creature GUID latches
	// (VEM/KRI/VEKLOR/VEKNILASH/VISCIDUS/SARTURA via creatureData),
	// 5 passage doors on 4 encounters (doorData), GetData/SetData
	// state (DATA_VEMISDEAD/VEKLORISDEAD/VEKNILASHISDEAD latches,
	// BUG_TRIO_DEATH counter, CTHUN_PHASE), IsEncounterInProgress ->
	// false. Zero Talk, zero timers, zero creature behavior — joins
	// the no-instance-script-bridge queue, documented-only, no lua.

	// boss_cthun audit (Temple of Ahn'Qiraj head, loader decl :87 / call
	// :200): eye_of_cthunAI + cthunAI are no-bridge document-only — the
	// fight runs on instance DATA_CTHUN_PHASE SetData/GetData (no
	// InstanceScript bridge), DoSpawnCreature tentacle spawns (no summon
	// bridge), SelectTarget Random (no bridge), Dark Glare positioning
	// math, and cross-AI DoAction coordination (no DoAction bridge at
	// all); the single Talk (EMOTE_WEAKENED) is trivial without the phase
	// machinery. flesh_tentacleAI's only arm (JustDied -> summoner AI
	// DoAction(ACTION_FLESH_TENTACLE_KILLED)) has no bearer — razorgore
	// precedent. The 4 tentacle NPCs ARE ported (npc_eye_tentacle 15726 /
	// npc_claw_tentacle 15725 / npc_giant_claw_tentacle 15728 /
	// npc_giant_eye_tentacle 15334, npc_*.lua convention; the four
	// luaBossAI bindings are registered below (the 2026-10-18
	// dead-binding sweep found them unbound). 2026-10-07
	// name-by-name re-verify found 3 fidelity
	// deviations, fixed: claw Hamstring repeat 10s -> 5s (C++ reset is
	// 5000 — the 10s was a giant-claw copy slip), giant claw was MISSING
	// the Hamstring arm (2s init -> 10s repeat per C++), giant eye beam
	// candidate filter lacked the Digestive Acid 26476 exclusion the C++
	// cast leg applies. No-bridge legs (portal summon/KillSelf, EvadeTimer
	// reposition, DoZoneInCombat, 35s KillSelf) stay documented in the
	// lua files, not wired.
	RegisterLuaBoss("npc_eye_tentacle", 15726)
	RegisterLuaBoss("npc_claw_tentacle", 15725)
	RegisterLuaBoss("npc_giant_claw_tentacle", 15728)
	RegisterLuaBoss("npc_giant_eye_tentacle", 15334)
	// wailing_caverns.cpp audit (loader decl :99 / call :212): single
	// script npc_disciple_of_naralex, an EscortAI — the whole Naralex
	// awakening chain (WaypointReached 4/5/11/19/24 arms,
	// TYPE_NARALEX_PART* SetData/GetData legs, Deviate/Viper/Moccasin/
	// Ectoplasm/Mutanus SummonCreature waves, cross-AI naralex Talks,
	// kneel/stand/flight-form auras, SetRun) needs escort-waypoint +
	// summon + instance-data bridges — none exist — so this unit is
	// documented-only, no lua file, joins the no-bridge queue.
	// instance_wailing_caverns.cpp audit (loader decl :100 / call :213):
	// pure InstanceMapScript (map 43, "WC") — 9 encounter-state slots
	// (lords + naralex event/parts + mutanus), Naralex GUID latch, DONE
	// persistence, 9-slot save/load with IN_PROGRESS reset. Zero Talk,
	// zero timers, zero creature behavior — joins the
	// no-instance-script-bridge queue.

	// boss_epoch (Chrono-Lord Epoch, entry 26532 — C++-verified via
	// npc_arthas.cpp:59's NPC_EPOCH constant plus :1329's
	// instance->instance->SummonCreature(NPC_EPOCH, ...) in the
	// RP3_EVENT_EPOCH_SPAWN leg of the arthas RP3 chain (summon strength);
	// BossAI's ctor leg DATA_EPOCH is culling_of_stratholme.h:118; the
	// creature_template ScriptName binding stays DB-side): the
	// self-contained in-combat legs only — engage arms the three timers
	// at their C++ ScheduleEvent cooldowns (no Talk in C++
	// JustEngagedWith); Wounding Strike 52771 on the victim (jeklik
	// GetVictim + CastSpell convention, {4s,6s} -> {12s,18s}); Curse of
	// Exertion 52772 on a random alive player within 100m (nil target ->
	// no cast, {10s,17s} -> 9.3s); Time Warp Talk SAY_TIME_WARP (2) +
	// self-cast 52766 + self-cast dummy 52736 (DoCastAOE resolves to
	// self-cast — kazrogal/illidan precedent, 25s -> 25s); KilledUnit Talk
	// SAY_SLAY (3) gated on the victim being a player (C++'s TYPEID_PLAYER
	// leg — terestian_illhoof convention); death is cancel only (C++
	// JustDied calls _JustDied(), instance bookkeeping blocked,
	// standing); melee engine-driven. The InitializeAI loot-mode leg
	// (GetBossState(DATA_EPOCH) == DONE -> RemoveLootMode) has no
	// instance-data bridge, the heroic-only Time Stop 58848 arm has no
	// difficulty bridge (standing heroic-unmodeled case), and the
	// EVENT_TIME_STEP charge machine (SpellHitTarget pushes dummy-hit
	// GUIDs, triggered 52737 charges every 500ms) has no SpellHitTarget /
	// ObjectAccessor bridge (SpellHit-15-never-fires standing queue) —
	// documented in lua_scripts/kalimdor/boss_epoch.lua, not wired.
	RegisterLuaBoss("boss_epoch", 26532)

	// npc_arthas_stratholme (Arthas, entry 26499 — C++-verified via
	// culling_of_stratholme.h:161's NPC_ARTHAS constant plus
	// instance_culling_of_stratholme.cpp:529's SummonCreature and :619's
	// OnCreatureCreate case; the creature_template ScriptName binding
	// stays DB-side): the self-contained in-combat legs only (legoso
	// combat-rotation-only precedent) — 1s engage pump for UpdateAICombat
	// (Holy Light 52444 self-cast below 40% hp, no C++ cooldown;
	// Exorcism 52445 on an unbounded random alive player every 7-14s
	// (nefarian convention), nil target retries next tick); KilledUnit
	// Talk LINE_SLAY_ZOMBIE (39) gated on the victim being NPC_RISEN_
	// ZOMBIE 27737 (event 3, nalorakk DISCOVERY); death is cancel only
	// (C++ JustDied is instance SetData(DATA_ARTHAS_DIED) + despawn,
	// blocked standing); melee engine-driven. The whole 5-RP
	// spline-chain / MovementInform / event-map escort machine, the
	// AdvanceToState snapback/react-state/gossip-flag legs, the gossip
	// AdvanceDungeon legs, the CanAIAttack 30yd leash, the JustAppeared
	// Devotion Aura self-buff, npc_stratholme_rp_dummy (MovementInform
	// forwarding, zero bridgeable arms), and spell_stratholme_crusader_
	// strike (SpellScript check handler, standing blocked queue) have no
	// movement / summon / gossip / instance-data / SpellHit bridges —
	// documented in lua_scripts/kalimdor/npc_arthas_stratholme.lua, not
	// wired.
	RegisterLuaBoss("npc_arthas_stratholme", 26499)

	// boss_infinite_corruptor (Infinite Corruptor, entry 32273 —
	// C++-verified via instance_culling_of_stratholme.cpp:62's
	// NPC_INFINITE_CORRUPTOR constant plus :791's
	// instance->SummonCreature(NPC_INFINITE_CORRUPTOR, CorruptorPos) in
	// SpawnInfiniteCorruptor (summon strength; the :787 heroic-only
	// spawn gate is a spawn-side leg, the creature_template ScriptName
	// binding stays DB-side); BossAI's ctor leg DATA_INFINITE_CORRUPTOR
	// is culling_of_stratholme.h:120): the self-contained in-combat
	// legs only — engage arms the two timers at their C++ ScheduleEvent
	// cooldowns with Talk SAY_AGGRO (0); Corrupting Blight 60588 on a
	// random alive player within 60m (nefarian convention, nil target
	// -> no cast, re-arm unconditional like C++, 7s -> 15s); Void
	// Strike 60590 on the victim (jeklik GetVictim + CastSpell
	// convention, 5s -> 5s); death is Talk SAY_DEATH (1) + cancel
	// (C++ JustDied calls _JustDied() plus guardian/rift cleanup,
	// blocked standing); melee engine-driven. The Reset
	// DoCastAOE channel leg (60422 "implicitly targets the Guardian"),
	// the SpellHitTarget(60422) guardian self-cast(60451) machine
	// (SpellHit-15-never-fires standing queue), the JustDied
	// FindNearestCreature(32281 guardian / 28409 time rift) cleanup,
	// the EnterEvadeMode REACT_PASSIVE gate, the MovementInform point
	// DespawnOrUnsummon + SetBossState FAIL, and the DoAction(-ACTION_
	// CORRUPTOR_LEAVE) SAY_FAIL(2) + MovePoint-to-rift chain
	// (culling_of_stratholme.h:150; instance caller
	// instance_culling_of_stratholme.cpp:495) have no channel /
	// SpellHit / nearby-creature / movement / DoAction / react-state /
	// instance-data bridges — documented in
	// lua_scripts/kalimdor/boss_infinite_corruptor.lua, not wired.
	RegisterLuaBoss("boss_infinite_corruptor", 32273)
	// Salramm the Fleshcrafter: combat arms ported (entry 26530, NPC_SALRAMM
	// summon-bound at instance_culling_of_stratholme.cpp:557) — timers/Talk
	// engage-death arms verified present in the port; InitializeAI spawn
	// Talk + loot-mode leg, heroic-only Curse of Twisted Flesh, BossAI
	// instance legs, JustDied SetData(DATA_NOTIFY_DEATH), and the
	// spell_salramm_steal_flesh AuraScript periodic have no spawn /
	// difficulty / instance-data / AuraScript bridges — documented in
	// lua_scripts/kalimdor/boss_salramm.lua, not wired.
	RegisterLuaBoss("boss_salramm", 26530)
	// Mal'Ganis: combat arms ported (entry 26533, NPC_MALGANIS
	// summon-bound at npc_arthas.cpp:698 / :1122 — :698 is the
	// RP5_MALGANIS_POS real-fight summon, :1122 the RP2 intro summon)
	// — the four timer arms (Carrion Swarm / Mind Blast / Vampiric
	// Touch / Sleep with the no-sleep-aura target filter), the
	// 30%/15% one-shot health yells, and the player-gated KilledUnit
	// slay Talk verified present in the port; the GetAI
	// MALGANIS_IN_PROGRESS / NullCreatureAI instance-progress gate,
	// BossAI instance legs, the DamageTaken lethal-clamp + _defeated
	// fake-death outro machine (PermBindAllPlayers), JustReachedHome
	// DespawnOrUnsummon, and the casting-state gates have no
	// instance-progress / DamageTaken / PermBind / evade / despawn /
	// casting bridges — documented in
	// lua_scripts/kalimdor/boss_mal_ganis.lua, not wired.
	RegisterLuaBoss("boss_mal_ganis", 26533)
	// Meathook: combat arms ported (entry 26529, NPC_MEATHOOK
	// summon-bound in the instance_culling_of_stratholme.cpp:553
	// WAVE_MEATHOOK wave-machine leg — not GUID-bound in
	// OnCreatureCreate, like salramm/aeonus) — the three timer arms
	// (Constricting Chains with the C++ negative-dist minimum-range
	// filter: random player >= 20yd else random player < 100m
	// excluding the victim else the victim; Disease Expulsion
	// self-cast AoE; Frenzy self-cast) and the engage / player-gated
	// KilledUnit slay Talk / death Talk verified present in the
	// port; InitializeAI's Talk(SAY_SPAWN) and the
	// GetBossState(DATA_MEATHOOK) == DONE loot-mode leg plus the
	// BossAI instance legs and JustDied SetData(DATA_NOTIFY_DEATH)
	// have no spawn / instance-data bridges — documented in
	// lua_scripts/kalimdor/boss_meathook.lua, not wired.
	RegisterLuaBoss("boss_meathook", 26529)
	// boss_celebras_the_cursed: Maraudon, entry 12225 (classicdb/tauri-cited;
	// no NPC_ constant in the C++ tree — vishas/gelihast precedent) — wrath
	// DoCast(random player) 8s/8s, entangling roots DoCastVictim 2s/20s,
	// corrupt forces self-cast 30s/20s (InterruptNonMeleeSpells leg bridged,
	// no-op twin, and ported); the JustDied summon of 13716 (Celebras the
	// Redeemed) is ported (creature:SummonCreature bridge, 2026-10-23 —
	// 'has no summon bridge' retracted) — see
	// lua_scripts/kalimdor/boss_celebras_the_cursed.lua.
	RegisterLuaBoss("boss_celebras_the_cursed", 12225)
	// boss_landslide: Maraudon, entry 12203 (classicdb-cited; no NPC_
	// constant in the C++ tree — celebras / vishas / gelihast
	// precedent) — knock away 18670 DoCastVictim 8s/15s, trample 5568
	// self-cast 2s/8s (both non-triggered), landslide 21808 self-cast
	// fired on the first below-50% tick then re-armed 60s of
	// below-50% time (event-9 crossing latch + 1s pump, C++ tick
	// semantics; InterruptNonMeleeSpells leg bridged, no-op twin,
	// and ported in both cast sites); melee is engine-driven — documented in
	// lua_scripts/kalimdor/boss_landslide.lua, not wired.
	RegisterLuaBoss("boss_landslide", 12203)
	// boss_noxxion: Maraudon, entry 13282 (wowhead-cited; no NPC_
	// constant in the C++ tree) — toxic volley 21687 DoCastVictim 7s/9s
	// + uppercut 22916 DoCastVictim 16s/12s (both non-triggered), both
	// ported. The 19s/40s invisible-adds machine is PORTED in
	// lua_scripts/kalimdor/boss_noxxion.lua: SetFaction(FRIENDLY),
	// SetFlag(59, NOT_SELECTABLE), SetDisplayId(11686), 5x
	// SummonCreature(13456, irand offsets, type 1, 90s), and the 15s
	// reappear (SetFaction(MONSTER), RemoveFlag, SetDisplayId(11172));
	// the C++ 15s timer freeze is modeled by canceling/re-arming the
	// spell timers (documented skew vs frozen remainders). Unmodeled:
	// the adds' AttackStart(victim)
	// (no bridge — the adds idle until the engine aggros them;
	// InterruptNonMeleeSpells(false) bridged and ported).
	RegisterLuaBoss("boss_noxxion", 13282)
	// boss_princess_theradras: Maraudon, entry 12201 (wowhead-cited; no
	// NPC_ constant in the C++ tree) — dust field 21909 self-cast
	// 8s/14s, boulder 21832 random-player 2s/10s, thrash 3391 self-cast
	// 5s/18s, repulsive gaze 21869 DoCastVictim 23s/20s (all
	// non-triggered); the JustDied summon of 12238 is ported
	// (creature:SummonCreature bridge, 2026-10-23 — 'has no summon
	// bridge' retracted) — see
	// lua_scripts/kalimdor/boss_princess_theradras.lua.
	RegisterLuaBoss("boss_princess_theradras", 12201)
	// instance_maraudon.cpp audit (loader decl :64 / call :177): pure
	// placeholder InstanceMapScript (map 349, "instance_maraudon") —
	// empty InstanceScript body, zero arms; the dungeon-finder credit
	// comment is the whole content. No-instance-script-bridge queue,
	// documented-only, no lua.
	// instance_dire_maul.cpp audit (loader decl :56 / call :169): pure
	// placeholder InstanceMapScript (map 429, SetBossNumber(17)) —
	// the 17-boss encounter list lives in comments (East/West/North),
	// zero runtime arms. No-instance-script-bridge queue,
	// documented-only, no lua.
	// instance_ragefire_chasm.cpp audit (loader decl :58 / call :171):
	// pure placeholder InstanceMapScript (map 389, "instance_ragefire_chasm")
	// — empty InstanceScript body, zero arms; the dungeon-finder credit
	// comment is the whole content. No-instance-script-bridge queue,
	// documented-only, no lua. (The Ragefire Chasm boss fights —
	// taragamanAI etc. — live in the Go AI track, not Lua.)
	// Onyxia: combat arms ported (entry 10184, NPC_ONYXIA GUID-bound
	// in instance_onyxias_lair.cpp OnCreatureCreate —
	// onyxias_lair.h:62; OnyxiaScriptName "instance_onyxias_lair",
	// map 249) — the four phase-1 timer arms (Flame Breath
	// DoCastVictim; Tail Sweep DoCastAOE self-cast; Cleave DoCastVictim;
	// Wing Buffet DoCastVictim) and the engage aggro Talk / ungated
	// KilledUnit Talk verified present in the port; the
	// HealthBelowPct(65)/HealthBelowPct(40) phase transitions, the
	// whole phase-2 flight machine (MoveData[8], MoveTakeoff/MovePoint,
	// SetCanFly/SetDisableGravity, SpellHit redirect, Deep Breath /
	// Movement / Fireball events), the phase-3 landing (Bellowing Roar
	// + floor-eruption GO search), the whelp/lair-guard summons, and
	// the BossAI instance/achievement/timed-achievement legs have no
	// movement / MotionMaster / summon / GO-enumeration / SpellHit /
	// instance-data / achievement bridges — documented in
	// lua_scripts/kalimdor/boss_onyxia.lua, not wired.
	RegisterLuaBoss("boss_onyxia", 10184)

	// boss_tuten_kash: Razorfen Downs, entry 7355 (razorfen_downs.h
	// RFDCreatureIds NPC_TUTEN_KASH — kalecgos passes; Tuten Kash
	// summon-event block). BossAI(creature, DATA_TUTEN_KASH)
	// (DATA_TUTEN_KASH = 0 of RFDDataTypes, "instance_razorfen_downs"
	// gate) — the instance-side BossAI legs have no bridge
	// (standing). Ported arms in lua_scripts/kalimdor/boss_tuten_kash.lua:
	// Reset self-buff refresh (Thrash 8876 / Virulent Poison 12254
	// under HasAura guards), Web Spray 12252 (random player in 100y,
	// recast-if-no-aura gate, {6s,8s}), Curse of Tuten'kash 12255
	// (self-cast, {15s,25s}); UNIT_STATE_CASTING gates unmodeled.
	RegisterLuaBoss("boss_tuten_kash", 7355)

	// npc_tomb_creature: Razorfen Downs, entries 7349 (Tomb Fiend) and
	// 7351 (Tomb Reaver) (razorfen_downs.h RFDCreatureIds — kalecgos
	// passes; used in the Tuten Kash summon event). ScriptedAI (not
	// BossAI). Ported arms in lua_scripts/kalimdor/npc_tomb_creature.lua:
	// Reset entry-conditional self-buff refresh (Poison Proc 3616 for
	// Tomb Fiend / Virulent Poison Proc 12254 for Tomb Reaver, each
	// under its C++ HasAura guard), Web 745 ({5s,8s} init, {7s,16s}
	// repeat unconditional). JustDied SetData(DATA_WAVE, entry) has no
	// instance-data bridge — documented in the lua file, not wired.
	RegisterLuaBoss("npc_tomb_creature", 7349)
	RegisterLuaBoss("npc_tomb_creature", 7351)

	// boss_mordresh_fire_eye: Razorfen Downs, entry 7357 wowhead-cited
	// (no NPC_ constant in the C++ tree — vishas precedent).
	// BossAI(creature, DATA_MORDRESH_FIRE_EYE) (razorfen_downs.h:32).
	// Ported arms in lua_scripts/kalimdor/boss_mordresh_fire_eye.lua:
	// Reset arms the out-of-combat talk loop at 10s (Talk 0, 8s ->
	// Talk 1, 3s -> emote EXCLAMATION documented-only, 6s -> Talk 2,
	// 14s -> loop; cleared on engage like C++ events.Reset());
	// JustEngagedWith Talk 3 + Fireball 12466 (victim-cast, init
	// 100ms, {2400ms,3800ms}) + Fire Nova 12470 (self-cast,
	// {8s,12s} -> {11s,16s}); UNIT_STATE_CASTING gates and BossAI
	// instance legs unmodeled.
	RegisterLuaBoss("boss_mordresh_fire_eye", 7357)

	// boss_glutton: Razorfen Downs, entry 8567 tauri-cited
	// (shoot.tauri.hu/?npc=8567 = Glutton; no NPC_ constant in the
	// C++ tree — vishas precedent). BossAI(creature, DATA_GLUTTON)
	// (razorfen_downs.h:33). Ported arms in
	// lua_scripts/kalimdor/boss_glutton.lua: JustEngagedWith Talk 0 +
	// 1s health pump; HealthBelowPct(50) -> Talk 2 latch;
	// HealthBelowPct(15) -> Talk 3 + self-cast Frenzy 12795 latch;
	// KilledUnit Talk 1 ungated; SPELL_DISEASE_CLOUD 12627 is
	// declared-but-never-cast in C++ (correctly omitted).
	RegisterLuaBoss("boss_glutton", 8567)

	// boss_amnennar_the_coldbringer: Razorfen Downs, entry 7358
	// tauri-cited (shoot.tauri.hu/?npc=7358; no NPC_ constant in the
	// C++ tree — vishas precedent). BossAI(creature,
	// DATA_AMNENNAR_THE_COLD_BRINGER). Ported arms in
	// lua_scripts/kalimdor/boss_amnennar_the_coldbringer.lua:
	// JustEngagedWith arms Wrath 13009 (8s->12s) + Frostbolt 15530
	// (1s->8s, both victim-cast) + Frost Nova 15531 (self-cast,
	// {10s,15s}->15s) + Talk 0; 1s health pump: <=60% Talk 1 +
	// DoCastVictim frost-spectres 12642 (ported — the C++ has no direct
	// summon call, the spectres are spell 12642's effect), <=50% Talk 3,
	// <=30% Talk 2 + DoCastVictim 12642 (ported; the stale "no summon
	// bridge, jeklik precedent" is retracted); KilledUnit Talk 4
	// player-gated; latches cleared on Reset. RAZORFEN DOWNS GROUP CLOSED.
	RegisterLuaBoss("boss_amnennar_the_coldbringer", 7358)

	// npc_belnistrasz: Razorfen Downs zone script, entry 8516
	// wowhead-verified (classic.wowhead.com/npc=8516/belnistrasz; no
	// NPC_ constant in the C++ tree — vishas precedent). ScriptedAI
	// (not BossAI); AddSC_razorfen_downs also registers
	// npc_idol_room_spawner (SetData-only summon machine — no summon
	// bridge, documented-only, unregistered) and go_gong (OnGossipHello
	// SendCustomAnim(0) ported in npc_belnistrasz.lua; instance
	// SetData(DATA_WAVE) leg stays out — no instance-data bridge).
	// Ported arms in lua_scripts/kalimdor/npc_belnistrasz.lua:
	// Reset refreshes Arcane Intellect 13326 under the C++ HasAura
	// guard (the gossip-questgiver flag re-set has no bridge);
	// OnQuestAccept(3525) latches eventInProgress + Talk(0) (escort
	// Start/faction/MovePath have no bridges); JustEngagedWith arms
	// Fireball 9053 (1s init, 8s repeat, victim-cast) + Frost Nova
	// 11831 (DoCastAOE resolved to self-cast, {8s,12s} init, 15s
	// repeat) + 60% Talk(6) (targeted-player Talk broadcasts);
	// MovementInform/idol-ritual machine (SummonCreature NPC 8611,
	// Talk 1-5 progress, GO summons, GroupEventHappens credit) and
	// JustDied's instance SetBossState + 5s despawn have no bridges —
	// documented in the lua file, not wired.
	RegisterLuaBoss("npc_belnistrasz", 8516)

	// npc_willix: Razorfen Kraul zone script, entry 4508
	// wowhead-verified (wowhead.com/forever/npc=4508/willix-the-importer;
	// no NPC_ constant in the C++ tree — belnistrasz precedent).
	// EscortAI (not BossAI); AddSC_razorfen_kraul's only registration.
	// Ported arms in lua_scripts/kalimdor/npc_willix.lua:
	// OnQuestAccept(1144) -> Talk(0) (EscortAI Start + faction swap
	// have no bridges); JustEngagedWith -> Talk(2) ungated
	// (targeted-player Talk broadcasts). The whole WaypointReached
	// escort path (Talk 1/3-10, emotes, boar 4514 summons,
	// GroupEventHappens credit), JustSummoned AttackStart, and
	// JustDied FailQuest have no bridges — documented in the lua
	// file, not wired.
	RegisterLuaBoss("npc_willix", 4508)
	// instance_razorfen_kraul.cpp audit (loader decl :76 / call :189):
	// InstanceMapScript (map 47, "instance_razorfen_kraul") — GO latch
	// 21099 (ward door) + 20920 faction-hack on OnGameObjectCreate, and
	// an EVENT_WARD_KEEPER SetData counter whose Update tick would open
	// the door at 2 kills. No-instance-script-bridge queue,
	// documented-only, no lua. NOTE: EVENT_WARD_KEEPER has zero
	// in-tree callers — no script reports a ward keeper death — so the
	// door-open arm is provably dead code in the C++ tree itself.

	// boss_zum_rah: Zul'Farrak, entry 7271 (zulfarrak.h ZFEntries
	// ENTRY_ZUM_RAH — kalecgos passes). BossAI(creature, DATA_ZUM_RAH)
	// (DATA_ZUM_RAH = 0 of ZFDataTypes, "instance_zulfarrak" gate) —
	// the instance-side BossAI legs have no bridge (standing).
	// Ported arms in lua_scripts/kalimdor/boss_zum_rah.lua: engage
	// Talk(0), Shadow Bolt 12739 (victim, 1s init, 4s repeat),
	// Shadowbolt Volley 15245 (random player in instance, 10s init,
	// 9s repeat), Talk(2) on KilledUnit, DamageTaken HP machine
	// (Talk(1) + one-shot Ward of Zum'rah 11086 self-cast at 80%/40%,
	// one-shot Healing Wave 12491 self-cast at 30%). Reset's
	// SetFaction(FACTION_FRIENDLY) and JustDied's SetData(DONE) have
	// no bridges — documented in the lua file, not wired.
	RegisterLuaBoss("boss_zum_rah", 7271)

	// npc_sergeant_bly: Zul'Farrak zone script, entry 7604 (zulfarrak.h
	// ZFEntries ENTRY_BLY — kalecgos passes). ScriptedAI (not BossAI).
	// Ported arms in lua_scripts/kalimdor/npc_sergeant_bly.lua: Shield
	// Bash 11972 (victim, 5s init, 15s repeat) and Revenge 12170
	// (victim, 8s init, 10s repeat), both unconditional (the C++ casts
	// Revenge unconditionally despite its dodge/parry/block code
	// comment — the port mirrors the code). Reset's
	// SetFaction(FACTION_FRIENDLY), the postGossipStep pyramid machine
	// (instance GUID-lookup + cross-creature DoAction + SetFaction +
	// AttackStart), and the EVENT_PYRAMID-gated gossip arms have no
	// bridges — documented in the lua file, not wired.
	RegisterLuaBoss("npc_sergeant_bly", 7604)

	// npc_weegli_blastfuse: Zul'Farrak zone script, entry 7607
	// (zulfarrak.h ZFEntries ENTRY_WEEGLI — kalecgos passes).
	// ScriptedAI (not BossAI). Ported arms in
	// lua_scripts/kalimdor/npc_weegli_blastfuse.lua: Bomb 8858 (victim,
	// 10s init, 10s repeat, unconditional); Shoot 6660 + SetSheath
	// RANGED/MELEE on the swing cadence (creature:SetSheath bridge,
	// 2026-10-20; melee-range gate via IsWithinDist 5.0, swing gate
	// paced at 2s). AttackStartCaster, the MovementInform pyramid legs
	// (no instance SetData/GetData bridges), DestroyDoor's MovePoint
	// leg (no point-movement bridge; SetFaction IS bridged but the arm
	// is instance-gated), and the pyramid-gated gossip arms have no
	// bridges; LandMine_Timer and SPELL_GOBLIN_LAND_MINE /
	// SPELL_WEEGLIS_BARREL are dead C++ legs — documented in the lua
	// file, not wired.
	RegisterLuaBoss("npc_weegli_blastfuse", 7607)

	// zulfarrak.cpp residual audit (loader decl :98 / call :214; Bly +
	// Weegli + Zum'rah ported above): go_troll_cage (GameObjectScript —
	// OnGossipHello sets instance EVENT_PYRAMID/CAGES_OPEN and herds
	// the 5 Bly crew members via GetGuidData + SetReactState/SetWalk/
	// SetHomePosition/MovePoint/SetFaction — no instance/movement/
	// faction bridges); go_shallow_grave (GO gossip first-use random
	// summon: 65% zombie 7286 / 10% dead hero 7276 via SummonCreature —
	// no summon bridge, so the gossip arm would be dead content);
	// at_zumrah (AreaTriggerScript — FindNearestCreature 7271 30y +
	// SetFaction(FACTION_TROLL_FROSTMANE): no area-trigger bridge).
	// All three documented-only, joining the no-bridge queues.

	// npc_qiraj_war_spawn: Silithus zone script, entries 15414
	// (Qiraji Wasp), 15422 (Qiraji Tank), 15423 (Kaldorei Infantry),
	// 15424 (Anubisath Conqueror) (zone_silithus.cpp enum
	// AnachronosTheAncient — kalecgos passes for all four).
	// ScriptedAI (not BossAI). Ported arms in
	// lua_scripts/kalimdor/npc_qiraj_war_spawn.lua: Poison Cloud
	// 28528 + Summon Poison Cloud 24319 (self, 38500ms init, 300s
	// repeat), Frost Debuff 35871 (self, 58s init, 300s repeat),
	// Fire Explosion 42075 (self, 80950ms init, 300s repeat) — all
	// qiraji-side entries only (entry-gated as in C++); Stoned
	// Channel visual 15533 (self, 100s init, 2s repeat) for all four
	// entries — the C++ RemoveAllAttackers/AttackStop preamble has
	// no bridge, documented deviation in the lua file. The
	// FindNearestCreature target-acquisition machine, the
	// Caelestrasz-gated Stoned 33652 arm, and JustDied's
	// DespawnOrUnsummon + trigger LiveCounter have no bridges —
	// documented in the lua file, not wired.
	RegisterLuaBoss("npc_qiraj_war_spawn", 15414)
	RegisterLuaBoss("npc_qiraj_war_spawn", 15422)
	RegisterLuaBoss("npc_qiraj_war_spawn", 15423)
	RegisterLuaBoss("npc_qiraj_war_spawn", 15424)

	// boss_slad_ran: Gundrak dungeon boss script, entry 29304
	// (gundrak.h NPC_SLAD_RAN — kalecgos pass), plus the summoned
	// adds 29713 (Slad'ran Constrictor) and 29680 (Slad'ran Viper)
	// (boss_slad_ran.cpp enum Creatures — kalecgos passes).
	// BossAI. Ported arms in lua_scripts/northrend/boss_slad_ran.lua:
	// JustEngagedWith Talk(SAY_AGGRO 0); Poison Nova 55081 (victim,
	// 10s init, 15s repeat) + Talk(EMOTE_NOVA 5) on the same tick;
	// Powerful Bite 48287 (victim, 3s init, 10s repeat); Venom Bolt
	// 54970 (victim, 15s init, 10s repeat); KilledUnit Talk(SAY_SLAY
	// 1) player-gated (event 3, nalorakk precedent); JustDied
	// Talk(EMOTE_ACTIVATE_ALTAR 6) + Talk(SAY_DEATH 2). The DamageTaken
	// phase machine (HealthBelowPct 30/25 — no health-pct bridge, and
	// the SummonCreature snake/constrictor waves sit behind the summon
	// STRAND), JustSummoned's MovePoint choreography, the
	// SetGUID/WasWrapped cross-AI wrap latch, and
	// achievement_snakes_whyd_it_have_to_be_snakes (no
	// achievement-criteria bridge) are documented in the lua file,
	// not wired.
	RegisterLuaBoss("boss_slad_ran", 29304)

	// boss_moorabi: Gundrak dungeon boss script, entry 29305
	// (boss_moorabi.cpp enum NPC_MOORABI — kalecgos pass; the
	// RegisterCreatureAIWithFactory(GetGundrakAI) ScriptName binding
	// is instance-shimmed, the creature_template binding DB-side).
	// BossAI. Ported arms in lua_scripts/northrend/boss_moorabi.lua:
	// JustEngagedWith Talk(SAY_AGGRO 0) + DoCastSelf(Mojo Frenzy
	// 55163, triggered); EVENT_TRANFORMATION (12s, single-fire):
	// Talk(EMOTE_BEGIN_TRANSFORM 5) + Talk(SAY_TRANSFORM 3) +
	// DoCastSelf(Transformation 55098) +
	// DoCastSelf(Summon Phantom Transform 55097, triggered) — no
	// reschedule, matching the C++ SpellHit(55098) cancel leg;
	// KilledUnit Talk(SAY_SLAY 1) player-gated (event 3 — nalorakk
	// precedent); JustDied Talk(EMOTE_ACTIVATE_ALTAR 7) +
	// Talk(SAY_DEATH 2). The three DoCastAOE combat arms (Ground
	// Tremor/Quake, Numbing Shout/Roar, Determined Stab/Gore) have no
	// DoCastAOE bridge, the PHASE_INTRO EVENT_PHANTOM has no phase
	// bridge, EnterEvadeMode has no despawn bridge, SpellHit/GetData
	// have no cross-AI bridges, achievement_less_rabi has no
	// achievement-criteria bridge, and spell_moorabi_mojo_frenzy has
	// no AuraScript bridge — all documented in the lua file, not wired.
	RegisterLuaBoss("boss_moorabi", 29305)

	// boss_drakkari_colossus: Gundrak dungeon boss script, entry 29307
	// (gundrak.h NPC_DRAKKARI_COLOSSUS — kalecgos pass; the
	// RegisterCreatureAIWithFactory(GetGundrakAI) ScriptName binding
	// is instance-shimmed, the creature_template binding DB-side).
	// BossAI. Ported arm in lua_scripts/northrend/boss_drakkari_colossus.lua:
	// EVENT_MIGHTY_BLOW — DoCastVictim(Mighty Blow 54719), Reset-scheduled
	// 10-30s init, rescheduled 5-15s (the only bridgeable arm of the slice).
	// The DamageTaken phase machine (HealthBelowPct 50/5 — no health-pct
	// bridge) sits behind the absent DoAction bridge, and the freeze/unfreeze
	// action legs behind SetImmuneToPC / SetReactState / motion-master /
	// aura-removal bridges; boss_drakkari_elemental and npc_living_mojo are
	// entry-unverifiable (no host-entry constants in the C++ sources — joins
	// the bridgeable-but-entry-blocked queue) — all documented in the lua
	// file, not wired.
	RegisterLuaBoss("boss_drakkari_colossus", 29307)

	// boss_gal_darah: Gundrak dungeon boss script, entry 29306
	// (gundrak.h NPC_GAL_DARAH — kalecgos pass; the
	// RegisterCreatureAIWithFactory(GetGundrakAI) ScriptName binding
	// is instance-shimmed, the creature_template binding DB-side).
	// BossAI. Ported arms in lua_scripts/northrend/boss_gal_darah.lua:
	// JustEngagedWith Talk(SAY_AGGRO 0); KilledUnit Talk(SAY_SLAY 1)
	// player-gated (event 3 — nalorakk precedent); JustDied
	// Talk(SAY_DEATH 2) + DoCastSelf(Clear Puncture 60022, triggered).
	// The whole combat rotation is phase-gated through
	// events.SetPhase/IsInPhase (no phase bridge — moorabi precedent)
	// and the phase flips are driven by unbridged arms, so
	// EVENT_PUNCTURE / EVENT_WHIRLING_SLASH / EVENT_ENRAGE /
	// EVENT_STOMP / EVENT_STAMPEDE / EVENT_IMPALING_CHARGE /
	// EVENT_TRANSFORM are documented in the lua file, not wired
	// (plus: no DoCastAOE bridge, no random-target SelectTarget
	// bridge, no SpellScript binding bridge for the three SpellScripts,
	// no cross-AI SetGUID/GetData bridges, no aura-removal bridge,
	// no despawn bridge; achievement_share_the_love has no
	// achievement-criteria bridge — snakes precedent).
	RegisterLuaBoss("boss_gal_darah", 29306)

	// boss_eck: Gundrak dungeon boss script, entry 29932
	// (gundrak.h NPC_ECK_THE_FEROCIOUS — kalecgos pass; the
	// RegisterCreatureAIWithFactory(GetGundrakAI) ScriptName binding
	// is instance-shimmed, the creature_template binding DB-side).
	// BossAI. Ported arms in lua_scripts/northrend/boss_eck.lua:
	// JustEngagedWith-scheduled EVENT_BITE (DoCastVictim 55813, 5s
	// init, 8-12s repeat), EVENT_SPIT (DoCastVictim 55814, 10s init,
	// 6-14s repeat), EVENT_BERSERK (DoCastSelf 55816, 60-90s init,
	// single fire). Documented in the lua file, not wired: the ctor
	// Talk(EMOTE_SPAWN 0) (no faithful Lua trigger — event 5 fires
	// respawn-path only, phase_hunter precedent), the DamageTaken
	// HealthBelowPctDamaged(20) early-berserk leg (no health-pct
	// bridge — doomwalker precedent), and EVENT_SPRING (random-target
	// SelectTarget bridge absent — cairne/kazzak precedent).
	RegisterLuaBoss("boss_eck", 29932)

	// boss_elder_nadox: Ahn'kahet dungeon boss script, entry 29309
	// (ahnkahet.h NPC_ELDER_NADOX — kalecgos pass; the
	// GetAhnKahetAI ScriptName binding is instance-shimmed, the
	// creature_template binding DB-side).
	// BossAI. Ported arms in lua_scripts/northrend/boss_elder_nadox.lua:
	// JustEngagedWith Talk(SAY_AGGRO 0); EVENT_SUMMON_SWARMER
	// (DoCastSelf 56119 + 33% Talk(SAY_EGG_SAC 3), 10s init, 10s
	// repeat); KilledUnit Talk(SAY_SLAY 1) player-gated; JustDied
	// Talk(SAY_DEATH 2). Documented in the lua file, not wired:
	// EVENT_PLAGUE (random-target SelectTarget bridge absent —
	// cairne/kazzak precedent), the IsHeroic-gated EVENT_RAGE and
	// EVENT_CHECK_ENRAGE (no difficulty bridge — kelidan breaker
	// precedent), the HealthBelowPct(50) guardian-summon leg (no
	// health-pct bridge — doomwalker precedent), the
	// SummonedCreatureDies/GetData guardian latch (cross-AI bridges
	// absent), spell_ahn_kahet_swarm (no SpellScript binding bridge
	// — razelikh precedent), achievement_respect_your_elders (no
	// achievement-criteria bridge — snakes precedent), and
	// npc_ahnkahar_nerubian (entry unverifiable — belnistrasz/willix
	// precedent; EVENT_SPRINT 56354 arm port-pattern-ready).
	RegisterLuaBoss("boss_elder_nadox", 29309)

	// boss_prince_taldaram: Ahn'kahet dungeon boss script, entry 29308
	// (ahnkahet.h NPC_PRINCE_TALDARAM — kalecgos pass; the
	// GetAhnKahetAI ScriptName binding is instance-shimmed, the
	// creature_template binding DB-side). Sole-source verified:
	// "boss_prince_taldaram" (the creature-script registration name)
	// is registered only from boss_prince_taldaram.cpp — the ICC hit
	// boss_blood_prince_council.cpp is a distinct script,
	// boss_prince_taldaram_icc, and does not collide.
	// BossAI. Ported arms in lua_scripts/northrend/boss_prince_taldaram.lua:
	// JustEngagedWith Talk(SAY_AGGRO 2); EVENT_BLOODTHIRST (DoCastSelf
	// 55968, 10s init, 10s repeat); EVENT_CONJURE_FLAME_SPHERES
	// (DoCast(victim) 55931, 5s init, 15s repeat); KilledUnit
	// Talk(SAY_SLAY 3) player-gated; JustDied Talk(SAY_DEATH 4).
	// Documented in the lua file, not wired: EVENT_VANISH plus the
	// vanish/feed machine (threat-list gate absent, random-target
	// SelectTarget bridge absent — cairne/kazzak precedent), the
	// DamageTaken embrace-damage latch (no damage hook — unkor
	// precedent; no difficulty bridge — kelidan precedent), the
	// vanish-evade check (threat + evade bridges absent), the
	// Reset/CheckSpheres/RemovePrison legs + ctor SetDisableGravity
	// (instance-script model absent — standing blocker), the flame
	// sphere NPC (30106/31686/31687 entry-verifiable but core arms
	// behind the absent SetGUID/ObjectAccessor/motion-master/despawn
	// bridges — terestian precedent), go_prince_taldaram_sphere (no
	// GameObjectAI binding bridge — go_crystal_prison precedent),
	// and both SpellScripts (no SpellScript binding bridge — razelikh
	// precedent).
	RegisterLuaBoss("boss_prince_taldaram", 29308)

	// boss_amanitar: Ahn'kahet dungeon boss script, entry 30258
	// (ahnkahet.h NPC_AMANITAR — kalecgos pass; the
	// RegisterAhnKahetCreatureAI ScriptName binding is
	// instance-shimmed, the creature_template binding DB-side).
	// BossAI. Ported arm in lua_scripts/northrend/boss_amanitar.lua:
	// EVENT_BASH — DoCastVictim(Bash 57094), JustEngagedWith-scheduled
	// 10-14s init, rescheduled 7-12s (the only bridgeable arm of the
	// slice). Documented in the lua file, not wired: EVENT_MINI
	// (random-target SelectTarget bridge absent — cairne/kazzak
	// precedent — plus no DoCastAOE bridge — terestian/shazzrah
	// precedent), EVENT_ROOT / EVENT_BOLT (random-target SelectTarget
	// bridge absent), EVENT_SPAWN and the EVENT_RESPAWN deque machine
	// (summon STRAND absent — standing), JustSummoned /
	// SummonedCreatureDies bookkeeping (summon / cross-AI bridges
	// absent), EnterEvadeMode (no despawn bridge — terestian
	// precedent; instance-script model absent), JustDied (instance
	// bookkeeping + DoCastAOE(57283) + instance remove-auras legs
	// unbridged), npc_amanitar_mushrooms (30391 / 30435 entry-verifiable
	// but Reset passive/display bridges + MoveInLineOfSight
	// aura-removal / DoCastAOE / scale / despawn bridges + JustDied
	// DoCastAOE all absent — entry-verifiable-but-bridge-blocked queue),
	// and spell_amanitar_potent_fungus (no AuraScript binding bridge —
	// razelikh precedent).
	RegisterLuaBoss("boss_amanitar", 30258)

	// boss_volazj: Ahn'kahet dungeon boss script, entry 29311
	// (ahnkahet.h NPC_HERALD_VOLAZJ — kalecgos pass; the
	// RegisterAhnKahetCreatureAI ScriptName binding is
	// instance-shimmed, the creature_template binding DB-side).
	// BossAI. Ported arms in lua_scripts/northrend/boss_herald_volazj.lua:
	// JustEngagedWith Talk(SAY_AGGRO 0), EVENT_MIND_FLAY —
	// DoCastVictim(Mind Flay 57941, 8s init, 20s repeat),
	// EVENT_SHADOW_BOLT_VOLLEY — DoCastVictim(Shadow Bolt Volley
	// 57942, 5s init, 5s repeat — task.Repeat() no-arg = same
	// duration, TaskScheduler.h:485), KilledUnit Talk(SAY_SLAY 1)
	// player-gated, JustDied Talk(SAY_DEATH 2). Documented in the
	// lua file, not wired: the JustEngagedWith / Reset
	// DoStart/StopTimedAchievement(20382) legs (instance-script
	// model absent — standing blocker), EVENT_SHIVER (random-target
	// SelectTarget bridge absent — cairne/kazzak precedent),
	// DamageTaken NOT_SELECTABLE damage-null + 66%/33%
	// health-crossing Insanity machine (no health-pct bridge —
	// doomwalker precedent; no interrupt bridge), SpellHitTarget(
	// 57496) insanity machinery — event 15 never fires plus absent
	// summon-STRAND / victim-CastSpell / phase-mask / flag bridges
	// — Reset SetPhaseMask + ResetPlayersPhaseMask legs (phase-mask
	// / aura-removal bridges absent — aeranas precedent),
	// SummonedCreatureDespawn visage phase roll-back machine
	// (summon STRAND + cross-AI bridges absent), UpdateAI insanity
	// wait-state (rides the unbridged arms), and the Twisted Visage
	// (30625 entry-verifiable from ahnkahet.h) — the C++ file notes
	// "Missing AI for Twisted Visages", so there is nothing to port.
	RegisterLuaBoss("boss_volazj", 29311)

	// boss_jedoga_shadowseeker: Ahn'kahet dungeon boss script, entry 29310
	// (ahnkahet.h NPC_JEDOGA_SHADOWSEEKER — kalecgos pass; the
	// RegisterAhnKahetCreatureAI ScriptName binding is
	// instance-shimmed, the creature_template binding DB-side).
	// BossAI. Ported arms in lua_scripts/northrend/
	// boss_jedoga_shadowseeker.lua: JustEngagedWith Talk(SAY_AGGRO 0),
	// KilledUnit Talk(SAY_SLAY 3) player-gated, JustDied Talk(SAY_DEATH
	// 4). Documented in the lua file, not wired: the entire
	// phase-gated machinery — events.SetPhase/IsInPhase (PHASE_INTRO /
	// PHASE_ONE / PHASE_TWO / PHASE_THREE; no phase bridge — gal_darah
	// / moorabi precedent) — so the combat rotation is not emulated
	// (EVENT_CYCLONE_STRIKE DoCastSelf 56855 3s->15-30s, ungated
	// timers would over-cast during the unbridged intro and phase-two
	// flying sacrifice sequence; EVENT_LIGHTNING_BOLT 56891 /
	// EVENT_THUNDERSHOCK 56926 also need the absent random-target
	// SelectTarget bridge — cairne/kazzak precedent), the DamageTaken
	// 55% PHASE_TWO flip (no health-pct bridge — doomwalker
	// precedent), the phase-two movement/volunteer/sacrifice machine
	// (summon STRAND + motion-master + flag + interrupt + cross-AI
	// DoAction bridges absent), DoAction(ACTION_SACRIFICE) /
	// DoCastAOE(Sacrifice Beam 56150) (no DoAction bridge —
	// drakkari_colossus precedent; no DoCastAOE bridge), JustSummoned
	// / SummonedCreatureDies / EnterEvadeMode / MovementInform /
	// GetData(DATA_VOLUNTEER_WORK) legs, npc_twilight_volunteer (30385
	// entry-verifiable — ahnkahet.h, zero registration: whole AI rides
	// instance + cross-AI DoAction + movement + despawn bridges —
	// entry-verifiable-but-bridge-blocked queue), spell_random_
	// lightning_visual_effect (no SpellScript binding bridge — razelikh
	// precedent), and achievement_volunteer_work (no achievement-
	// criteria bridge — snakes precedent). The Ahn'kahet dungeon boss
	// roster closes with this unit (elder_nadox 29309, taldaram 29308,
	// amanitar 30258, volazj 29311, jedoga 29310).
	RegisterLuaBoss("boss_jedoga_shadowseeker", 29310)

	// boss_krik_thir: Azjol-Nerub dungeon boss script, entry 28684
	// (azjol_nerub.h NPC_KRIKTHIR — kalecgos pass; the
	// GetAzjolNerubAI ScriptName binding is instance-shimmed, the
	// creature_template binding DB-side). BossAI. Ported arms in
	// lua_scripts/northrend/boss_krik_thir.lua: EVENT_MIND_FLAY
	// DoCastVictim(Mind Flay 52586) randtime(1s,3s) init, 9-11s repeat
	// (the JustEngagedWith schedule is port-pattern-ready; the passive
	// engage legs themselves are unbridged), KilledUnit Talk(SAY_SLAY
	// 1) player-gated, JustDied Talk(SAY_DEATH 2) (summons.clear +
	// _JustDied instance bookkeeping has no bridge). Documented in the
	// lua file, not wired: the passive pre-fight model
	// (SetReactState(REACT_PASSIVE) — drakkari_colossus precedent;
	// JustAppeared -> SummonAdds() with instance GetBossState gate +
	// SummonCreatureGroup(1..3) — summon STRAND absent), the
	// MoveInLineOfSight passive-engage leg, EVENT_SEND_GROUP
	// DoCastAOE(52343) 70s repeat, EVENT_SWARM DoCastAOE(52440) +
	// Talk(SAY_SWARM 3) 5s (no DoCastAOE bridge — terestian/shazzrah
	// precedent), EVENT_FRENZY (HealthBelowPct(10) — doomwalker
	// precedent — + DoCastSelf 28747 + DoCastAOE 52592), the whole
	// DoAction machine (no DoAction bridge — drakkari_colossus
	// precedent: gatewatcher-greet prefight Talk, watcher-died
	// bookkeeping, watcher/pet engaged, pet evade), EnterEvadeMode
	// (summons.DespawnAll — terestian precedent), SpellHit/SpellHitTarget
	// (event 15 never fires — standing), npc_watcher_gashra/narjil/
	// silthik (28730/28729/28731 entry-verifiable — azjol_nerub.h —
	// zero registration: the npc_gatewatcher_petAI passive + group-aggro
	// + cross-AI DoAction + instance model is unbridged; their
	// enrage/infected-bite/blinding-webs/poison-spray rotation arms are
	// port-pattern-ready, web-wrap behind the random-target SelectTarget
	// bridge — entry-verifiable-but-bridge-blocked queue), the anub'ar
	// warrior/skirmisher/shadowcaster, skittering swarmer/infector and
	// gatewatcher web-wrap NPCs (entries unverifiable from C++ — the
	// adds come from DB-side summon-group data, no NPC constants —
	// bridgeable-but-entry-blocked queue), the three SpellScripts (no
	// binding bridge — razelikh precedent), and
	// achievement_watch_him_die (no achievement-criteria bridge —
	// snakes precedent + instance model absent).
	RegisterLuaBoss("boss_krik_thir", 28684)

	// boss_hadronox: Azjol-Nerub dungeon boss script, entry 28921
	// (azjol_nerub.h NPC_HADRONOX — kalecgos pass; the
	// GetAzjolNerubAI ScriptName binding is instance-shimmed, the
	// creature_template binding DB-side). BossAI. Ported arm in
	// lua_scripts/northrend/boss_hadronox.lua: EVENT_PIERCE_ARMOR
	// DoCastVictim(Pierce Armor 53418) randtime(4s,7s) init, 10-15s
	// repeat (the JustEngagedWith arms are independent schedulers —
	// none of the unmodeled events gate this one). Documented in the
	// lua file, not wired: the whole pre-fight/step movement machine
	// (InitializeAI/SetStep react-state, SetHomePosition, MotionMaster
	// MovePoint — motion-master bridge absent), SummonCrusherPack
	// (summon STRAND + cross-AI SetData/DoAction absent), the
	// final-step MovementInform door-webbing (DoCastAOE 53177/53185 —
	// terestian/shazzrah precedent + motion-master), GetData/SetGUID
	// cross-AI, the CanAIAttack home-distance leash, the setActive leg,
	// EVENT_LEECH_POISON DoCastAOE(53030), EVENT_ACID_CLOUD
	// DoCast(53400) (random-target SelectTarget — cairne/kazzak
	// precedent), EVENT_WEB_GRAB DoCastAOE(57731), EVENT_PLAYER_CHECK
	// + the DoAction machine (ACTION_CRUSHER_ENGAGED instance
	// SetBossState + packs 2/3; ACTION_HADRONOX_MOVE — no DoAction
	// bridge, drakkari_colossus precedent; instance model absent),
	// EnterEvadeMode (trigger aura-scan, _DespawnAtEvade,
	// summons.DespawnAll — terestian precedent, ObjectAccessor _anubar
	// despawn), the UpdateAI casting-skip, DamageTaken NPC safeguard
	// (no DamageTaken hook — npc_unkor_the_ruthless precedent — +
	// health-pct — doomwalker precedent), JustSummoned summons.Summon,
	// npc_anub_ar_crusher (28922 entry-verifiable — zero registration:
	// the whole npc_hadronox_crusherPackAI passive pack machine is
	// unbridged, and wiring the Talk(SAY_AGGRO 1)/EVENT_SMASH(53318)
	// arms without the REACT_PASSIVE model fails the C++-exact bar —
	// amanitar mushrooms precedent; frenzy behind the DamageTaken +
	// health-pct bridges; JustDied DoAction behind the DoAction
	// bridge), the crusher-pack champion/crypt-fiend/necromancer and
	// foe champion/crypt-fiend/necromancer NPCs (entries unverifiable
	// from C++ — bridgeable-but-entry-blocked queue; rotation arms
	// port-pattern-ready), the three periodic-summon AuraScripts, the
	// leeching-poison AuraScript and the web-doors SpellScript (no
	// binding bridge — razelikh precedent), and
	// achievement_hadronox_denied (no achievement-criteria bridge —
	// snakes precedent + cross-AI GetData absent).
	RegisterLuaBoss("boss_hadronox", 28921)

	// boss_anub_arak: Azjol-Nerub dungeon boss script, entry 29120
	// (azjol_nerub.h NPC_ANUBARAK — kalecgos pass; the
	// GetAzjolNerubAI ScriptName binding is instance-shimmed, the
	// creature_template binding DB-side). BossAI. Ported arms in
	// lua_scripts/northrend/boss_anub_arak.lua: JustEngagedWith
	// Talk(SAY_AGGRO 0) (event 1; the door/timed-achievement/phase/
	// world-trigger summon legs are documented-only), KilledUnit
	// Talk(SAY_SLAY 1) player-gated (event 3 — nalorakk precedent),
	// JustDied Talk(SAY_DEATH 2) (event 4; _JustDied instance
	// bookkeeping has no bridge). Documented in the lua file, not
	// wired: the Reset flag/achievement legs, the whole
	// phase-gated combat rotation (EVENT_POUND 59433, EVENT_LEECHING_
	// SWARM 53467, EVENT_CARRION_BEETLES 53520 — no phase bridge,
	// gal_darah/moorabi precedent; swarm/beetles also need the
	// absent DoCastAOE bridge, terestian/shazzrah precedent),
	// EVENT_IMPALE (cross-AI SetGUID(GUID_TYPE_IMPALE) absent),
	// EVENT_SUBMERGE (no health-pct bridge — doomwalker precedent —
	// + no DamageTaken hook, npc_unkor_the_ruthless precedent),
	// EVENT_DARTER/ASSASSIN/GUARDIAN/VENOMANCER pet waves
	// (world-trigger grid-scan + cross-AI + summon bridges absent),
	// the SetGUID/DoAction machine (no DoAction bridge —
	// drakkari_colossus precedent), EnterEvadeMode (summons.
	// DespawnAll + _DespawnAtEvade — terestian precedent), the
	// SpellHit(SPELL_SUBMERGE 53421) payload (event 15 never fires —
	// standing), the DamageTaken submerge gate/damage-null legs, the
	// UpdateAI casting-skip, the darter/assassin/guardian/venomancer/
	// impale-target NPCs (entries unverifiable from C++ — pets come
	// from DB-side summon-spell data — bridgeable-but-entry-blocked
	// queue; guardian sunder-armor / venomancer poison-bolt / assassin
	// backstab rotation arms port-pattern-ready), and
	// spell_anubarak_pound / spell_anubarak_carrion_beetles (no
	// SpellScript/AuraScript binding bridge — razelikh precedent).
	RegisterLuaBoss("boss_anub_arak", 29120)

	// boss_trollgore: Drak'Tharon Keep dungeon boss script, entry 26630
	// (drak_tharon_keep.h NPC_TROLLGORE — kalecgos pass; the
	// GetDrakTharonKeepAI ScriptName binding is instance-shimmed, the
	// creature_template binding DB-side). BossAI. Ported arms in
	// lua_scripts/northrend/boss_trollgore.lua: JustEngagedWith
	// Talk(SAY_AGGRO 0) (event 1; the EVENT_CONSUME/EVENT_CORPSE_EXPLODE
	// schedules ride the absent DoCastAOE bridge — terestian/shazzrah
	// precedent — and EVENT_SPAWN rides the absent instance GetGuidData +
	// ObjectAccessor cross-AI bridges), EVENT_CRUSH DoCastVictim(49639)
	// randtime(1s,5s) init, 10-15s repeat, EVENT_INFECTED_WOUND
	// DoCastVictim(49637) randtime(10s,60s) init, 25-35s repeat (the
	// JustEngagedWith schedules are independent — none of the unmodeled
	// events gate these two), KilledUnit Talk(SAY_KILL 1) player-gated
	// (event 3 — nalorakk precedent), JustDied Talk(SAY_DEATH 4)
	// (event 4; _JustDied instance bookkeeping has no bridge).
	// Documented in the lua file, not wired: EVENT_CONSUME Talk(2) +
	// DoCastAOE(49380) and EVENT_CORPSE_EXPLODE Talk(3) + DoCastAOE(49555)
	// (no DoCastAOE bridge), EVENT_SPAWN invader-summoner trigger casts
	// (instance model absent), the _consumptionJunction latch (aura-stack
	// 49381/59805 — no aura-stack bridge), GetData(DATA_CONSUMPTION_
	// JUNCTION) cross-AI (its only caller is achievement_consumption_
	// junction — no achievement-criteria bridge, snakes precedent +
	// cross-AI GetData absent), JustSummoned MovePoint choreography
	// (motion-master + summon STRAND bridges absent), the UpdateAI
	// casting-skip, npc_drakkari_invader (27709/27753/27754 entry-verifiable
	// — zero registration: Dismount/SetImmuneToAll + DoCastAOE(49405)
	// behind motion + DoCastAOE bridges), spell_trollgore_consume
	// (49380/59803) + spell_trollgore_invader_taunt (49405) + spell_
	// trollgore_corpse_explode (49555/59807) (no SpellScript/AuraScript
	// binding bridge — razelikh precedent), and
	// achievement_consumption_junction.
	RegisterLuaBoss("boss_trollgore", 26630)

	// npc_slad_ran_constrictor: Gundrak script, entry 29713
	// (boss_slad_ran.cpp CREATURE_CONSTRICTORS — kalecgos pass).
	// ScriptedAI. Ported arm: Grip of Slad'ran 55093 on the victim
	// (2s init, 3-6s repeat). The 5-stack -> Snake Wrap 55126 victim
	// self-cast -> boss SetGUID latch -> DespawnOrUnsummon machine has
	// no bridges (aura-stack read, RemoveAurasDueToSpell, victim
	// CastSpell, cross-AI SetGUID, despawn) — documented in the lua
	// file, not wired.
	RegisterLuaBoss("npc_slad_ran_constrictor", 29713)

	// npc_slad_ran_viper: Gundrak script, entry 29680
	// (boss_slad_ran.cpp CREATURE_SNAKE — kalecgos pass). ScriptedAI.
	// Ported arm: Venomous Bite 54987 (victim, 2s init, 10s repeat).
	RegisterLuaBoss("npc_slad_ran_viper", 29680)

	// boss_novos: Drak'Tharon Keep dungeon boss script, entry 26631
	// (drak_tharon_keep.h NPC_NOVOS — kalecgos pass; the
	// GetDrakTharonKeepAI ScriptName binding is instance-shimmed, the
	// creature_template binding DB-side). BossAI. Ported arms in
	// lua_scripts/northrend/boss_novos.lua: JustEngagedWith
	// Talk(SAY_AGGRO 0) (event 1; the SetCrystalsStatus/
	// SetSummonerStatus/SetBubbled legs ride the absent instance
	// GetGuidData + ObjectAccessor cross-AI + flag/GO-state bridges —
	// instance model absent, standing), KilledUnit Talk(SAY_KILL 1)
	// player-gated (event 3 — nalorakk precedent), JustDied
	// Talk(SAY_DEATH 2) (event 4; _JustDied instance bookkeeping has
	// no bridge). No timers are scheduled: every C++ scheduled arm
	// rides an absent bridge — EVENT_ATTACK (random-target
	// SelectTarget — cairne/kazzak precedent) and EVENT_SUMMON_MINIONS
	// (ungated emulation would over-cast — jedoga precedent) both fire
	// only after the unbridged DoAction(ACTION_CRYSTAL_HANDLER_DIED)
	// machine (drak_tharon_keep.h:52) clears the bubble. Documented in
	// the lua file, not wired: AttackStart DoStartNoMovement (no
	// movement bridge), the _bubbled UpdateAI gate + the casting-skip
	// (no unit-state bridge), the CrystalHandlerDied machine incl.
	// Talk(SAY_ARCANE_FIELD 4) (no DoAction bridge — drakkari_colossus
	// precedent — + cross-AI SetData absent), the MoveInLineOfSight
	// _ohNovos latch (no grid/MoveInLineOfSight bridge), GetData(
	// DATA_NOVOS_ACHIEV), JustSummoned (summon STRAND absent),
	// npc_crystal_channel_target (26712 entry-verifiable — zero
	// registration: cross-AI AI()->SetData activation + MovePath
	// bridges absent), spell_novos_summon_minions (59910 — no
	// SpellScript binding bridge — razelikh precedent), and
	// achievement_oh_novos (no achievement-criteria bridge — snakes
	// precedent — + cross-AI GetData absent).
	RegisterLuaBoss("boss_novos", 26631)

	// boss_king_dred: Drak'Tharon Keep dungeon boss script, entry 27483
	// (drak_tharon_keep.h NPC_KING_DRED — kalecgos pass; the
	// GetDrakTharonKeepAI ScriptName binding is instance-shimmed, the
	// creature_template binding DB-side). BossAI. King Dred has no
	// SAY/yell enum in C++ — no Talk arms exist. Ported arms in
	// lua_scripts/northrend/boss_king_dred.lua: EVENT_GRIEVOUS_BITE
	// DoCastVictim(48920) 20s init, 20s repeat, EVENT_MANGLING_SLASH
	// DoCastVictim(48873) 18500ms init, 18500ms repeat,
	// EVENT_PIERCING_SLASH DoCastVictim(48878) 15s init, 15s repeat
	// (the JustEngagedWith schedules are independent — none of the
	// unmodeled events gate them). JustDied is only _JustDied()
	// (instance bookkeeping, no bridge) — nothing wired. Documented
	// in the lua file, not wired: EVENT_BELLOWING_ROAR DoCastAOE(22686)
	// 33s and EVENT_FEARSOME_ROAR DoCastAOE(48849) 10-20s (no DoCastAOE
	// bridge — terestian/shazzrah precedent), EVENT_RAPTOR_CALL
	// (SummonCreature RAND(NPC_DRAKKARI_GUTRIPPER 26641 /
	// NPC_DRAKKARI_SCYTHECLAW 26628) rides the absent summon STRAND —
	// the only bridgeable leg is the no-op dummy DoCastVictim(59416),
	// whose payload is the unbridged summon, so the timer is not
	// armed), the UpdateAI casting-skip (no unit-state bridge),
	// DoAction(ACTION_RAPTOR_KILLED)/GetData(DATA_RAPTORS_KILLED) (no
	// DoAction/GetData bridges), npc_drakkari_gutripper (26641) /
	// npc_drakkari_scytheclaw (26628) entry-verifiable — zero
	// registration: spawned only by the unbridged summon; GutRip 49710
	// and Rend 13738 rotation arms stay port-pattern-ready; JustDied
	// cross-AI DoAction legs behind instance GetGuidData +
	// ObjectAccessor + DoAction bridges, and
	// achievement_king_dred (no achievement-criteria bridge — snakes
	// precedent — + cross-AI GetData absent).
	RegisterLuaBoss("boss_king_dred", 27483)

	// boss_tharon_ja: Drak'Tharon Keep dungeon boss script, entry 26632
	// (drak_tharon_keep.h NPC_THARON_JA — kalecgos pass; the
	// GetDrakTharonKeepAI ScriptName binding is instance-shimmed, the
	// creature_template binding DB-side). BossAI. Ported arms in
	// lua_scripts/northrend/boss_tharon_ja.lua: JustEngagedWith
	// Talk(SAY_AGGRO 0) (event 1; BossAI::JustEngagedWith leg is
	// instance bookkeeping, no bridge), KilledUnit Talk(SAY_KILL 1)
	// player-gated (event 3 — nalorakk precedent), JustDied
	// Talk(SAY_DEATH 4) (event 4; _JustDied instance bookkeeping has
	// no bridge). No timers are scheduled: every C++ scheduled arm is
	// phase-gated by the unbridged phase machine (GOING_FLESH /
	// GOING_SKELETAL call events.Reset() — ungated emulation would
	// over-cast vs C++, jedoga precedent; anub_arak EVENT_POUND
	// precedent). Documented in the lua file, not wired: the whole
	// phase machine (EVENT_DECAY_FLESH DoCastAOE(49356) 20s ->
	// EVENT_GOING_FLESH +6s (Talk SAY_FLESH + SetDisplayId(MODEL_FLESH
	// 27073) + DoCastAOE(GIFT 52509) + DoCast-self FLESH_VISUAL 52582 /
	// DUMMY 49551 + events.Reset + phase-2 schedule) ->
	// EVENT_RETURN_FLESH DoCastAOE(53463) 20s -> EVENT_GOING_SKELETAL
	// +6s (Talk SAY_SKELETON + RestoreDisplayId + DoCastAOE(CLEAR_GIFT
	// 53242) + events.Reset + phase-1 re-arm) — the meaningful legs
	// are DoCastAOE (no bridge — terestian/shazzrah precedent) +
	// SetDisplayId/RestoreDisplayId (no display bridge); genuinely-
	// bridgeable bar (king_dred RAPTOR_CALL) not met, chain not armed;
	// phase-1 rotation (EVENT_CURSE_OF_LIFE 49527 1s->10-15s,
	// EVENT_RAIN_OF_FIRE 49518 14-18s, EVENT_SHADOW_VOLLEY 49528
	// 8-10s — bridged in isolation via moroes precedent but
	// phase-gated) and phase-2 rotation (EVENT_LIGHTNING_BREATH 49537,
	// EVENT_EYE_BEAM 49544 — both random-target SelectTarget absent,
	// cairne/kazzak precedent — EVENT_POISON_CLOUD DoCastAOE 49548);
	// Reset's _Reset() + RestoreDisplayId; JustDied's DoCastAOE(
	// CLEAR_GIFT 53242 / ACHIEVEMENT_CHECK 61863); the UpdateAI
	// casting-skip (no unit-state bridge); spell_tharon_ja_clear_gift_
	// of_tharon_ja (53242 SpellScript — no SpellScript binding bridge,
	// razelikh precedent). Drak'Tharon Keep block CLOSED.
	RegisterLuaBoss("boss_tharon_ja", 26632)

	// boss_eadric: Trial of the Champion dungeon boss script, entry
	// 35119 (trial_of_the_champion.h NPC_EADRIC — kalecgos pass; the
	// GetTrialOfTheChampionAI ScriptName binding is instance-shimmed,
	// the creature_template binding DB-side). ScriptedAI. Ported arms
	// in lua_scripts/northrend/boss_eadric.lua: uiVenganceTimer —
	// DoCastSelf(Vengeance 66865) (creature:CastSpell(creature, spell)
	// — phase_hunter / apothecary_hanes precedent), 10s init, 10s
	// repeat, scheduled on OnEnterCombat(1), cancelled on 2/4/23
	// (gargolmar precedent); melee engine-driven. No Talk arms exist
	// in C++ (the Yells enum is commented out). Documented in the lua
	// file, not wired: constructor react-state / NON_ATTACKABLE flag
	// (drakkari_colossus precedent), DamageTaken evade+FACTION_
	// FRIENDLY+bDone latch (no DamageTaken hook — npc_unkor_the_
	// ruthless precedent), MovementInform POINT_MOTION_TYPE ->
	// instance SetBossState(BOSS_ARGENT_CHALLENGE_E, DONE) +
	// DisappearAndDie (instance model absent + no despawn bridge),
	// bDone-reset MovePoint(0, 746.87, 665.87, 411.75) (no motion
	// bridge), uiHammerJusticeTimer (SelectTarget(Random,0,250,true)
	// -> Hammer of Justice 66863 + Hammer of the Righteous 66867,
	// 25s — random-target SelectTarget bridge absent, cairne/kazzak
	// precedent), uiRadianceTimer (DoCastAOE 66935, 16s — no
	// DoCastAOE bridge, terestian/shazzrah precedent). boss_paletress
	// (34928 NPC_PALETRESS — kalecgos pass): zero registration —
	// constructor / Reset (RemoveAllAuras + ObjectAccessor::GetCreature
	// (MemoryGUID) RemoveFromWorld) / SetData(1) RemoveAura(SHIELD
	// 66515) / DamageTaken / MovementInform / Holy Fire 66538 9-12s +
	// Smite 66536 5-7s (random-target SelectTarget absent) / shield-
	// gated Renew 66537 machine (aura + cross-AI ObjectAccessor
	// bridges absent) / 25% one-shot (health-pct bridge absent —
	// doomwalker precedent — + DoCastAOE triplet (Holy Nova 66546 /
	// Summon Memory 66545 / Confess 66680) — terestian/shazzrah
	// precedent; the Shield 66515 self-cast leg rides the unbridged
	// gate — jedoga precedent) / JustSummoned MemoryGUID latch
	// (summon STRAND absent) — every combat arm rides an absent
	// bridge. npc_memory: ENTRY UNVERIFIABLE (no NPC constant in C++
	// — the memory creatures materialize via DB-side summon spells
	// SPELL_MEMORY_* — belnistrasz/willix precedent): joins the
	// bridgeable-but-entry-blocked queue (Waking Nightmare 66552
	// 7s/7s self-cast port-pattern-ready; Old Wounds 66620 /
	// Shadows Past 66619 behind the random-target bridge; JustDied
	// cross-AI SetData(1,0) — no cross-AI SetData bridge).
	// npc_argent_soldier: ENTRY-VERIFIABLE BUT BRIDGE-BLOCKED (35309
	// NPC_ARGENT_LIGHWIELDER / 35305 NPC_ARGENT_MONK / 35307
	// NPC_PRIESTESS — kalecgos pass): joins the
	// entry-verifiable-but-bridge-blocked queue — whole AI is the
	// EscortAI waypoint machine (escort queue precedent) + JustDied's
	// instance SetData(DATA_ARGENT_SOLDIER_DEFEATED,+1) (instance
	// model absent). spell_eadric_radiance / spell_paletress_summon_
	// memory: no SpellScript binding bridge (razelikh precedent).
	// C++ SD%Complete 50 % note carried.
	RegisterLuaBoss("boss_eadric", 35119)

	// boss_black_knight: Trial of the Champion dungeon boss script,
	// entry 35451 (trial_of_the_champion.h NPC_BLACK_KNIGHT — kalecgos
	// pass; the GetTrialOfTheChampionAI ScriptName binding is
	// instance-shimmed, the creature_template binding DB-side).
	// ScriptedAI. Ported arms in lua_scripts/northrend/
	// boss_black_knight.lua: uiIcyTouchTimer — DoCastVictim(Icy Touch
	// 67718) (creature:CastSpell(nil, spell) — moroes precedent),
	// urand(5000,9000) init, urand(5000,7000) repeat;
	// uiPlagueStrikeTimer — DoCastVictim(SPELL_ICY_TOUCH 67718)
	// (C++ names the timer Plague Strike but casts Icy Touch —
	// replicated C++-exact), urand(10000,13000) init,
	// urand(12000,15000) repeat; uiObliterateTimer — DoCastVictim
	// (Obliterate 67725), urand(17000,19000) init, repeat; all scheduled
	// on OnEnterCombat(1), cancelled on 2/4/23 (gargolmar precedent) —
	// the three arms fire identically in PHASE_UNDEAD and PHASE_SKELETON
	// and are independent of the unbridged DamageTaken resurrect machine
	// (boss_eadric precedent). JustDied: DoCastSelf(Kill Credit 68663)
	// on event 4 (the instance->SetBossState(BOSS_BLACK_KNIGHT, DONE) leg
	// has no bridge — instance model absent, standing); melee
	// engine-driven. No Talk arms exist in C++ (no Say enum — "missing
	// yells" in the SD%Complete 80 % note). Documented in the lua file,
	// not wired: DamageTaken (damage > health && uiPhase <=
	// PHASE_SKELETON -> damage = 0 + SetHealth(0) + ROOT/STUNNED +
	// summons.DespawnAll() + SetDisplayId(29846/21300) +
	// bEventInProgress — no DamageTaken hook, npc_unkor_the_ruthless
	// precedent); the bEventInProgress resurrect machine
	// (SetFullHealth + DoCastSelf(Black Knight Res 67693, true) +
	// uiPhase++ + ClearUnitState, gated on the unbridged DamageTaken leg
	// — jedoga bar); uiDeathRespiteTimer (PHASE_UNDEAD: SelectTarget
	// (Random,0,100,true) -> Death's Respite 67745, urand(15000,16000) —
	// random-target SelectTarget absent, cairne/kazzak precedent);
	// PHASE_SKELETON legs — phase never arrives (bSummonArmy one-shot
	// ROOT/STUNNED + DoCastSelf(Army of the Dead 67761) — no unit-state
	// bridge — + DeathArmyCheckTimer ClearUnitState; Desecration 67778
	// urand(15000,16000) behind the random-target bridge; Ghoul Explode
	// 67751 8s self-cast phase-gated — jedoga bar); PHASE_GHOST legs —
	// phase never arrives (Death's Bite 67808 urand(2000,4000) — no
	// DoCastAOE bridge, terestian/shazzrah precedent; Marked for Death
	// 67882 urand(5000,7000) behind the random-target bridge); Reset's
	// summons.DespawnAll() + SetDisplayId(native) + ClearUnitState (no
	// summon / display / unit-state bridges); JustSummoned's
	// summons.Summon + cross-AI AttackStart (summon STRAND absent,
	// standing); melee's !HasUnitState(ROOT) && !HealthBelowPct(1) gate
	// (no unit-state bridge + no health-pct bridge, doomwalker
	// precedent — melee plays engine-driven unconditionally here).
	// npc_risen_ghoul: ENTRY UNVERIFIABLE (no NPC constant in C++ — the
	// ghouls materialize via the DB-side SPELL_ARMY_DEAD 67761 summon
	// effect — belnistrasz/willix precedent): zero registration; joins
	// the bridgeable-but-entry-blocked queue (port-pattern-ready:
	// uiAttackTimer SelectTarget(Random,1,100,true) -> DoCast(Leap
	// 67749), 3500 repeat — random-target bridge absent).
	// npc_black_knight_skeletal_gryphon: ENTRY UNVERIFIABLE (no NPC
	// constant ties the script name to an entry in C++ — the
	// creature_template binding is DB-side): zero registration; whole AI
	// is the EscortAI machine (constructor Start(false,true) +
	// EscortAI::UpdateAI) — escort / motion-master bridges absent
	// (escort queue precedent); joins the bridgeable-but-entry-blocked
	// queue. Trial of the Champion block stays OPEN.
	RegisterLuaBoss("boss_black_knight", 35451)
	// boss_anubarak_trial: Trial of the Crusader dungeon boss script,
	// entry 34564 (trial_of_the_crusader.h NPC_ANUBARAK — kalecgos
	// pass). lua_scripts/northrend/boss_anubarak_trial.lua:
	// JustEngagedWith Talk(SAY_AGGRO 1) (event 1; the
	// BossAI::JustEngagedWith instance-bookkeeping leg has no bridge —
	// tharon_ja precedent); KilledUnit Talk(SAY_KILL_PLAYER 7)
	// player-gated (event 3, who->GetTypeId()==TYPEID_PLAYER — nalorakk
	// precedent); JustDied Talk(SAY_DEATH 8) (event 4; _JustDied instance
	// bookkeeping has no bridge — tharon_ja precedent); EVENT_BERSERK —
	// DoCastSelf(Berserk 26662), 600s after combat start,
	// phase-independent, fires once (phase_hunter self-cast precedent);
	// scheduler cancelled on 2/4/23 (gargolmar precedent); melee
	// engine-driven. npc_swarm_scarab (34605): JustDied DoCast(killer,
	// Traitor King 68186) killer-gated (event 4; maiden_of_virtue
	// target-cast precedent) — no boss_ai.go entry needed (terestian
	// precedent: npc entries register in the lua file only). Trial of
	// the Champion block now CLOSED; Trial of the Crusader block OPEN.
	// Unmodeled (no bridges — documented in the lua header, not wired):
	// boss MoveInLineOfSight Talk(SAY_INTRO 0) (no hook); JustReachedHome
	// SetBossState(FAIL) + 10x scarab summons (instance model + summon
	// STRAND absent); JustSummoned burrow/spike legs (display / react /
	// random-target SelectTarget bridges absent); JustEngagedWith flag /
	// DoAction / summon legs; EVENT_FREEZE_SLASH (66012, 15s) and
	// EVENT_PENETRATING_COLD (66013, 20s) — bridged in isolation but
	// PHASE_MELEE-gated behind the unbridged submerge machine (tharon_ja
	// bar); EVENT_SUMMON_NERUBIAN (66332, 45s), EVENT_NERUBIAN_SHADOW_STRIKE
	// (heroic), EVENT_SUBMERGE/EMERGE/PURSUING_SPIKE (66169)/SUMMON_SCARAB/
	// SUMMON_FROST_SPHERE — phase machine + flag / summon / ObjectAccessor /
	// cross-AI bridges absent; phase-3 leg (HealthBelowPct(30) + DoCastAOE
	// Leeching Swarm 66118 — no health-pct / DoCastAOE bridges); JustDied
	// grid despawn (no despawn bridge); npc_swarm_scarab Reset / DoAction /
	// determination-timer legs (instance-gated, jedoga bar);
	// npc_nerubian_burrower (34607) — zero registration (Reset self-casts
	// + DoZoneInCombat + cross-AI ride the unbridged summon gate;
	// ACTION_SHADOW_STRIKE random-target; submerge timer behind health-pct
	// + flag / aura bridges); npc_anubarak_spike (34660) — zero registration,
	// entry-verifiable-but-bridge-blocked (DamageTaken hook / motion / threat
	// bridges absent); npc_frost_sphere (34606) — Reset legs ported via boss
	// ev19; DamageTaken / MovementInform ground-impact machine stays out
	// (no hook / motion bridge);
	// spell_pursuing_spikes, spell_impale, spell_anubarak_leeching_swarm —
	// no SpellScript / AuraScript binding bridge (razelikh precedent).
	RegisterLuaBoss("boss_anubarak_trial", 34564)
	// boss_lord_jaraxxus: Trial of the Crusader dungeon boss script,
	// entry 34780 (trial_of_the_crusader.h NPC_JARAXXUS — kalecgos
	// pass; instance_trial_of_the_crusader.cpp binds NPC_JARAXXUS ->
	// DATA_JARAXXUS with a CircleBoundary).
	// lua_scripts/northrend/boss_lord_jaraxxus.lua: JustEngagedWith
	// Talk(SAY_AGGRO 1) (event 1; the BossAI::JustEngagedWith
	// instance-bookkeeping leg has no bridge — tharon_ja precedent);
	// KilledUnit Talk(SAY_KILL_PLAYER 9) player-gated (event 3,
	// who->GetTypeId()==TYPEID_PLAYER — nalorakk precedent); JustDied
	// Talk(SAY_DEATH 10) (event 4; _JustDied instance bookkeeping has no
	// bridge — tharon_ja precedent); EVENT_FEL_FIREBALL —
	// DoCastVictim(Fel Fireball 66532), 6s init, urand(11s,13s) repeat
	// (moroes precedent; no submerge/phase gate — UpdateAI runs combat
	// events whenever UpdateVictim() holds); EVENT_NETHER_POWER —
	// DoCastSelf(Nether Power 66228), 22s init, 42s repeat
	// (phase_hunter self-cast precedent; C++'s triggered +
	// SPELLVALUE_AURA_STACK RAID_MODE(5,10) args have no bridge);
	// EVENT_ENRAGE — Talk(SAY_BERSERK 11) + DoCastSelf(Berserk 64238),
	// 10min, fires once (anubarak berserk precedent); timers cancelled
	// on 2/4/23 (gargolmar precedent); melee engine-driven.
	// npc_mistress_of_pain (34826 — Summons enum kalecgos pass):
	// JustEngagedWith EVENT_SHIVAN_SLASH — DoCastVictim(Shivan Slash
	// 66378), 4s init, urand(3s,10s) repeat (moroes precedent) — no
	// boss_ai.go entry needed (terestian precedent: npc entries register
	// in the lua file only). Trial of the Crusader block stays OPEN.
	// Unmodeled (no bridges — documented in the lua header, not wired):
	// the whole intro machine (Reset instance-state legs, EVENT_INTRO
	// MoveAlongSplineChain + DoCastSelf 66327, EVENT_TAUNT_GNOME
	// Talk(SAY_INTRO 0), EVENT_KILL_GNOME DoCastSelf 67888,
	// MovementInform SPLINE_CHAIN SetFacingToObject, EVENT_CHANGE_
	// ORIENTATION SetFacingTo, EVENT_START_COMBAT SetImmuneToPC /
	// SetReactState / DoZoneInCombat, DoAction ACTION_JARAXXUS_ENGAGE,
	// EnterEvadeMode SetBossState + DespawnOrUnsummon — instance model +
	// motion / react / immune / zone / despawn bridges absent);
	// EVENT_FEL_LIGHTNING (66528, 17s, random-target — no SelectTarget
	// bridge, cairne/kazzak precedent); EVENT_INCINERATE_FLESH (66237,
	// 14s, random-target + emote/yell); EVENT_LEGION_FLAME (66197, 20s,
	// random-target); EVENT_SUMMON_NETHER_PORTAL (66269, 20s) and
	// EVENT_SUMMON_INFERNAL_ERUPTION (66258, 1min20s) — summon STRAND
	// absent; npc_legion_flame (34784) / npc_infernal_volcano (34813) /
	// npc_fel_infernal (34815) / npc_nether_portal (34825) — zero
	// registration, entry-verifiable-but-bridge-blocked (Reset legs ride
	// the instance gate or the unbridged summon gate — jedoga / burrower
	// precedents; react / flag / random-target / difficulty bridges
	// absent); npc_mistress_of_pain Reset / JustDied instance SetData
	// legs (instance model absent), EVENT_SPINNING_SPIKE (66283,
	// random-target), EVENT_MISTRESS_KISS (66336, heroic — no difficulty
	// bridge, kelidan precedent); spell_mistress_kiss, spell_mistress_
	// kiss_area, spell_fel_streak_visual — no SpellScript / AuraScript
	// binding bridge (razelikh precedent).
	RegisterLuaBoss("boss_lord_jaraxxus", 34780)
	// npc_toc_boomkin: Faction Champions balance-druid champion script,
	// entries 34460 (Alliance, NPC_ALLIANCE_DRUID_BALANCE) / 34451 (Horde,
	// NPC_HORDE_DRUID_BALANCE) — trial_of_the_crusader.h:183/197 kalecgos
	// pass. Fight logic in lua_scripts/northrend/npc_toc_boomkin.lua:
	// Reset-scheduled 8-event rotation (cyclone 65859 random non-tank,
	// entangling roots 65857 random <=30yd, faerie fire 65863 / force of
	// nature 65861 / insect swarm 65855 / moonfire 65856 / starfire 65854
	// victim, barkskin 65860 <30% self) + equipment staff 50966; engage
	// triggered self-cast anti-AOE 68595. Base-class legs (instance
	// GetData DATA_TEAM, threat/power machines, controller cross-AI
	// DoAction, DoZoneInCombat, DoSpellAttackIfReady wrath) unbridged —
	// documented in the lua header. Controller encounter machine still
	// documented-only (06:5x audit).
	RegisterLuaBoss("npc_toc_boomkin", 34460)
	RegisterLuaBoss("npc_toc_boomkin", 34451)
	// npc_toc_druid: Faction Champions restoration-druid champion script,
	// entries 34469 (Alliance) / 34459 (Horde) — npc_toc_druidAI :
	// boss_faction_championsAI : BossAI, same chain as the registered
	// boomkin/champion siblings above; fight logic in
	// lua_scripts/northrend/npc_toc_druid.lua.
	RegisterLuaBoss("npc_toc_druid", 34469)
	RegisterLuaBoss("npc_toc_druid", 34459)
	// npc_toc_shaman: Faction Champions restoration-shaman champion script,
	// entries 34470 (Alliance) / 34444 (Horde) — npc_toc_shamanAI :
	// boss_faction_championsAI : BossAI, same chain as the druid above;
	// fight logic in lua_scripts/northrend/npc_toc_shaman.lua.
	RegisterLuaBoss("npc_toc_shaman", 34470)
	RegisterLuaBoss("npc_toc_shaman", 34444)
	// npc_toc_priest: Faction Champions discipline-priest champion script,
	// entries 34466 (Alliance) / 34447 (Horde) — npc_toc_priestAI :
	// boss_faction_championsAI : BossAI; fight logic in
	// lua_scripts/northrend/npc_toc_priest.lua.
	RegisterLuaBoss("npc_toc_priest", 34466)
	RegisterLuaBoss("npc_toc_priest", 34447)
	// npc_toc_warlock: Faction Champions warlock champion script,
	// entries 34474 (Alliance) / 34450 (Horde) — npc_toc_warlockAI :
	// boss_faction_championsAI : BossAI; fight logic in
	// lua_scripts/northrend/npc_toc_warlock.lua.
	RegisterLuaBoss("npc_toc_warlock", 34474)
	RegisterLuaBoss("npc_toc_warlock", 34450)
	// npc_toc_enh_shaman: Faction Champions enhancement-shaman champion script,
	// entries 34463 (Alliance) / 34455 (Horde) — npc_toc_enh_shamanAI :
	// boss_faction_championsAI : BossAI; fight logic in
	// lua_scripts/northrend/npc_toc_enh_shaman.lua.
	RegisterLuaBoss("npc_toc_enh_shaman", 34463)
	RegisterLuaBoss("npc_toc_enh_shaman", 34455)
	// boss_faction_champions: Trial of the Crusader Faction Champions
	// defensive self-buff EventMap timer arms, entries
	// trial_of_the_crusader.h:180-208 kalecgos pass. Each champion AI
	// struct fires a recurring timer arm that checks HealthBelowPct on
	// current health at fire time (integer strict-<, Unit.h) and
	// self-casts the defensive spell, re-arming the long cooldown, or
	// the short check interval otherwise. Fight logic in
	// lua_scripts/northrend/boss_faction_champions.lua (single-arm
	// territory; npc_toc_druid 34469/34459 and npc_toc_boomkin
	// 34460/34451 are ported separately above).
	// npc_toc_paladin (holy) 34465/34445: EVENT_HEAL_DIVINE_SHIELD
	// HealthBelowPct(30) && !HasAura(FORBEARANCE 25771) -> Divine Shield
	// 66010, 5min CD / 5s check, 20s init.
	// npc_toc_shadow_priest 34473/34441: EVENT_DISPERSION
	// HealthBelowPct(40) -> Dispersion 65544, 180s CD / 5s check,
	// 20-40s init.
	// npc_toc_mage 34468/34449: EVENT_ICE_BLOCK HealthBelowPct(30) ->
	// Ice Block 65802, 5min CD / 5s check, 10-20s init.
	// npc_toc_hunter 34467/34448: EVENT_DETERRENCE HealthBelowPct(30) ->
	// Deterrence 65871, 150s CD / 10s check, 10-20s init.
	// npc_toc_warrior 34475/34453: EVENT_RETALIATION HealthBelowPct(50) ->
	// Retaliation 65932, 5min CD / 5s check, 5-20s init.
	// npc_toc_dk 34461/34458: EVENT_ICEBOUND_FORTITUDE
	// HealthBelowPct(50) -> Icebound Fortitude 66023, 60s CD / 5s
	// check, 25-35s init.
	// npc_toc_rogue 34472/34454: EVENT_CLOAK HealthBelowPct(50) ->
	// Cloak 65961, 90s CD / 5s check, 20-30s init.
	// npc_toc_retro_paladin (retribution) 34471/34456:
	// EVENT_DPS_DIVINE_SHIELD HealthBelowPct(30) &&
	// !HasAura(FORBEARANCE 25771) -> Divine Shield 66010, 5min CD / 5s
	// check, 20-30s init. Trial of the Crusader block stays OPEN.
	RegisterLuaBoss("boss_faction_champions", 34465)
	RegisterLuaBoss("boss_faction_champions", 34445)
	RegisterLuaBoss("boss_faction_champions", 34473)
	RegisterLuaBoss("boss_faction_champions", 34441)
	RegisterLuaBoss("boss_faction_champions", 34468)
	RegisterLuaBoss("boss_faction_champions", 34449)
	RegisterLuaBoss("boss_faction_champions", 34467)
	RegisterLuaBoss("boss_faction_champions", 34448)
	RegisterLuaBoss("boss_faction_champions", 34475)
	RegisterLuaBoss("boss_faction_champions", 34453)
	RegisterLuaBoss("boss_faction_champions", 34461)
	RegisterLuaBoss("boss_faction_champions", 34458)
	RegisterLuaBoss("boss_faction_champions", 34472)
	RegisterLuaBoss("boss_faction_champions", 34454)
	RegisterLuaBoss("boss_faction_champions", 34471)
	RegisterLuaBoss("boss_faction_champions", 34456)
	// boss_twin_valkyr: Trial of the Crusader dungeon boss script,
	// entries 34497 Fjola Lightbane (trial_of_the_crusader.h
	// NPC_FJOLA_LIGHTBANE — kalecgos pass;
	// instance_trial_of_the_crusader.cpp binds NPC_FJOLA_LIGHTBANE ->
	// DATA_FJOLA_LIGHTBANE with a CircleBoundary) and 34496 Eydis
	// Darkbane (NPC_EYDIS_DARKBANE — kalecgos pass; bound to
	// DATA_EYDIS_DARKBANE the same way).
	// lua_scripts/northrend/boss_twin_valkyr.lua: JustEngagedWith
	// Talk(SAY_AGGRO 0) + DoCastSelf(Surge 65766/65768) (event 1; the
	// BossAI::JustEngagedWith instance-bookkeeping leg, DoZoneInCombat,
	// cross-AI sister AddAura/Empathy, SetCombatPulseDelay/setActive have
	// no bridge — tharon_ja precedent); KilledUnit Talk(SAY_KILL_PLAYER
	// 6) player-gated (event 3, who->GetTypeId()==TYPEID_PLAYER —
	// nalorakk precedent); JustDied Talk(SAY_DEATH 8) (event 4; _JustDied
	// instance bookkeeping has no bridge — tharon_ja precedent);
	// EVENT_TWIN_SPIKE — DoCastVictim(Light Twin Spike 66075 / Dark Twin
	// Spike 66069), 20s init, 20s repeat (moroes precedent).
	// Whole intro/positioning machine unmodeled: JustAppeared
	// PHASE_EVENT leg (no phase bridge), EVENT_START_MOVE
	// MoveAlongSplineChain (no motion bridge), MovementInform
	// SPLINE_CHAIN -> immune/react/door legs (no MovementInform bridge),
	// Reset SetReactState/passive + ModifyAuraState + summons.DespawnAll +
	// DoStopTimedAchievement (no react/aura-state/despawn/achievement
	// bridges), JustReachedHome SetBossState(FAIL) + HandleRemoveAuras +
	// DespawnOrUnsummon + DoUseDoorOrButton(DATA_MAIN_GATE) (instance
	// model + no despawn/door bridges); EVENT_SPECIAL_ABILITY stage
	// machine (no cross-AI DoAction bridge); DoAction ACTION_VORTEX
	// (DoCastAOE — terestian/shazzrah precedent) and ACTION_PACT
	// (cross-AI DoAction; self-cast shield/pact legs port-pattern-ready in
	// isolation but DoAction-gated — documented-only); EVENT_TOUCH
	// (heroic; random-target aura-filtered SelectTarget — cairne/kazzak
	// precedent); EVENT_BERSERK 64238 + Talk(SAY_BERSERK 7) (IsHeroic() ?
	// 6min : 8min timer — no difficulty bridge, kelidan precedent).
	// npc_essence_of_twin (34567/34568, entry-unverifiable, gossip),
	// npc_unleashed_dark (34628) / npc_unleashed_light (34630,
	// entry-unverifiable; SelectNearestPlayer/DoCastAOE/despawn/motion
	// legs unbridged), npc_bullet_controller (34743,
	// entry-unverifiable; DoCastAOE bridge absent) — zero registration,
	// documented-only. spell_bullet_controller, spell_powering_up,
	// spell_valkyr_essences, spell_power_of_the_twins — no SpellScript /
	// AuraScript binding bridge (razelikh precedent); join the
	// SpellScript/AuraScript queue.
	RegisterLuaBoss("boss_twin_valkyr", 34497)
	RegisterLuaBoss("boss_twin_valkyr", 34496)
	// Northrend Beasts (Trial of the Crusader) — fight logic in
	// lua_scripts/northrend/boss_northrend_beasts.lua. PORTED: boss_gormok
	// (34796) EVENT_IMPALE (DoCastVictim 66331, 10s/10s) and
	// EVENT_STAGGERING_STOMP (DoCastVictim 66330, 15s/22s); no phase args
	// on gormok's ScheduleTasks — C++ runs them whenever UpdateVictim()
	// holds. DOCUMENTED-BLOCKED: gormok's intro machine (MovementInform ->
	// EVENT_ENGAGE — no hook; DoUseDoorOrButton/immune/react/summon-STRAND
	// legs unbridged), EVENT_THROW (vehicle passengers — no vehicle
	// bridge), PassengerBoarded RISING_ANGER, JustDied/EnterEvadeMode
	// (instance model), DoCastSelf SPELL_TANKING_GORMOK (SERVERSIDE, rides
	// the intro machine); npc_snobold_vassal (34800) — whole vehicle mount
	// machine (no vehicle/SetGUID bridges) + instance DATA_SNOBOLD_COUNT +
	// EVENT_FIRE_BOMB (random-target — cairne/kazzak precedent);
	// npc_beasts_combat_stalker (36549) — Reset/DoAction legs (cross-AI
	// DoAction, no bridge), EVENT_BERSERK 26662 (instance model + no
	// difficulty bridge — kelidan precedent); boss_dreadscale (34799) /
	// boss_acidmaw (35144) — the jormungar submerge/phase machine: submerge
	// / emerge (motion/flag/react/aura/display bridges absent), EVENT_BITE
	// DoCastVictim (phase-gated behind the unbridged machine — jedoga
	// over-cast bar), EVENT_SPEW / EVENT_SWEEP (DoCastAOE —
	// terestian/shazzrah precedent), EVENT_SLIME_POOL (summon STRAND
	// absent), EVENT_SPRAY (random-target), EVENT_SUMMON_ACIDMAW (summon
	// STRAND), JustDied/DoAction ACTION_ENRAGE (instance model + cross-AI
	// DoAction); npc_jormungars_slime_pool / npc_fire_bomb
	// (entry-unverifiable, summoned by spell — entry DB-side; self-cast
	// legs port-pattern-ready but entry-blocked); boss_icehowl (34797) —
	// the whole charge machine (MoveJump/MoveCharge/MovementInform hooks,
	// random charge target, instance DATA_FURIOUS_CHARGE) +
	// PHASE_COMBAT-gated rotation (EVENT_FEROCIOUS_BUTT DoCastVictim /
	// EVENT_WHIRL DoCastSelf port-pattern-ready in isolation but
	// suspended/rescheduled around the unbridged charge — jedoga bar) +
	// EVENT_ARCTIC_BREATH (random-target) + DoAction ENRAGE/TRAMPLE_FAIL
	// (gated via spell_icehowl_trample — no SpellScript binding bridge);
	// the twelve spell scripts — no SpellScript/AuraScript binding bridge
	// (razelikh precedent); join the SpellScript/AuraScript queue.
	RegisterLuaBoss("boss_gormok", 34796)
	// Anub'Rekhan (Naxxramas) — fight logic in
	// lua_scripts/northrend/boss_anubrekhan.lua. PORTED: boss_anubrekhan
	// (15956) JustEngagedWith Talk(SAY_AGGRO 0) (BossAI bookkeeping,
	// summons.DoZoneInCombat, SetPhase have no bridge — tharon_ja
	// precedent); KilledUnit Talk(SAY_SLAY 2) C++-unconditional; the
	// player-gated corpse-scarab cast (29105) is PORTED (boss casts on the
	// victim — victim self-cast has no player:CastSpell bridge); EVENT_LOCUST
	// Talk(EMOTE_LOCUST 3) + DoCastSelf(Locust Swarm 28785), init
	// randtime(1m40s,2m), repeat 1m30s (phase_hunter self-cast precedent;
	// the PHASE_SWARM/NORMAL gating only affects the unmodeled
	// impale/scarabs arms — documented-only); EVENT_BERSERK DoCastSelf
	// (Berserk 27680, triggered), 10min init, reschedules 10min.
	// DOCUMENTED-BLOCKED: intro/summon machine (InitializeAI, Reset
	// guardCorpses.clear, JustReachedHome SummonGuards — SummonCreatureGroup
	// is DB-coord group spawn (no summon-group bridge; creature:
	// SummonCreature is live but takes explicit coords); Is25ManRaid legs
	// stay out on group spawn (Is25ManRaid itself now bridged,
	// lua_creature_events.go — IsHeroic was never a substitute for it);
	// EVENT_SPAWN_GUARD
	// SummonCreatureGroup), JustSummoned
	// EMOTE_SPAWN Talk + SummonedCreatureDies/Despawn guardCorpses
	// bookkeeping (summon/cross-AI bridges absent; NPC_CRYPT_GUARD 16573
	// joins the entry-verifiable-but-bridge-blocked queue),
	// EVENT_IMPALE (random-target SelectTarget — cairne/kazzak precedent;
	// anti-chain leg noted), EVENT_SCARABS (corpse cast — stay out: cross-AI
	// corpse cast/Talk/despawn + guardCorpses GUID bookkeeping + phase
	// gate, no bridges), JustDied DoStartTimedAchievement(9891) (no
	// achievement bridge), at_anubrekhan_entrance (no area-trigger
	// bridge).
	RegisterLuaBoss("boss_anubrekhan", 15956)

	// Razuvious (16061, Naxxramas) — lua_scripts/northrend/boss_razuvious.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0) +
	// EVENT_STRIKE DoCastVictim(Unbalancing Strike 26613) 21s/6s;
	// KilledUnit Talk(SAY_SLAY 1) gated on player or entry-16803
	// (nalorakk precedent); JustDied Talk(SAY_DEATH 3). Unmodeled:
	// SpellHit SAY_TAUNTED (SpellHit never fires in Lua), JustDied
	// DoCastAOE(Hopeless 29125) (no DoCastAOE bridge), EVENT_ATTACK
	// MoveChase (no motion bridge), EVENT_SHOUT DoCastAOE(29107) (no
	// DoCastAOE bridge), EVENT_KNIFE SelectTarget(Random) (cairne/kazzak
	// precedent), InitializeAI/JustReachedHome SummonCreatureGroup (no
	// summon bridge), npc_dk_understudy (16803, entry-verifiable —
	// LoadEquipment/emotestate/cross-AI DoZoneInCombat/DESPAWN/
	// blood-strike 61696 arms have no bridge: no equipment, instance,
	// cross-AI, despawn, or charm-possession bridge).
	RegisterLuaBoss("boss_razuvious", 16061)

	// Kel'Thuzad (15990, Naxxramas) — lua_scripts/northrend/boss_kelthuzad.lua.
	// Ported arms (C++-exact): KilledUnit Talk(SAY_SLAY 8) gated on player
	// (nalorakk precedent); JustDied Talk(SAY_DEATH 9) (tharon_ja
	// precedent). Unmodeled: SpellHit CHAINS_DUMMY Talk (SpellHit never
	// fires in Lua), EVENT_SKELETON/BANSHEE/ABOMINATION summons (stay out:
	// unported phase-one machine + cross-AI DoZoneInCombat + the
	// _skeletonCount==1 grid-search leg — summon bridge is live, coords
	// C++-literal), EVENT_DESPAWN_MINIONS despawn + SAY_AGGRO (Talk not
	// ported orphaned — vortex precedent), EVENT_PHASE_TWO (no phase
	// bridge), EVENT_FROSTBOLT_VOLLEY / EVENT_CHAINS DoCastAOE (no
	// DoCastAOE bridge), EVENT_SHADOW_FISSURE / EVENT_DETONATE_MANA /
	// EVENT_FROST_BLAST SelectTarget(Random) (cairne/kazzak precedent),
	// transition reply (lich-king cross-AI Talk via instance GetGuidData
	// + portal GO states — no instance/GO-state bridges) / transition
	// summon (SummonCreatureGroup — DB coords — + cross-AI DoAction, no
	// summon-group/cross-AI bridges),
	// phase-three 45%-health leg (no health-pct bridge), frostbolt
	// filler (phase-gated), DamageTaken zeroing (no phase bridge), Reset /
	// EnterEvadeMode / ACTION_BEGIN_ENCOUNTER (instance model),
	// GetAIForCharmedPlayer (no charmed-player AI bridge),
	// npc_kelthuzad_skeleton/banshee/abomination/guardian
	// (entry-unverifiable from C++ evidence — no registration; abomination
	// DoCastVictim(MORTAL_WOUND 28467) and guardian DoCastVictim(
	// BLOOD_TAP 28470) join the bridgeable-but-entry-blocked queue),
	// spell_kelthuzad_chains/detonate_mana/frost_blast (no AuraScript
	// binding bridge), at_kelthuzad_center (no area-trigger bridge),
	// achievement_just_cant_get_enough (no achievement bridge).
	RegisterLuaBoss("boss_kelthuzad", 15990)

	// Gluth (15932, Naxxramas) — lua_scripts/northrend/boss_gluth.lua.
	// Ported arms (C++-exact): EVENT_WOUND DoCastVictim(MORTAL_WOUND
	// 54378) 10s/10s (moroes precedent); EVENT_ENRAGE Talk(EMOTE_ENRAGE
	// 2) + DoCastSelf(ENRAGE 28371) randtime(16s,22s)/randtime(16s,22s)
	// (phase_hunter self-cast precedent); EVENT_BERSERK Talk(
	// EMOTE_BERSERKER 4) + DoCastSelf(BERSERK 26662) 8min/5min. The
	// STATE_GLUTH_EATING deferral arms (3s/5s/4s repeats) have no state
	// bridge — documented in the lua. Unmodeled: Reset react-state/
	// speed legs (no bridges), SummonedCreatureDies despawn (no summon
	// bridge), EVENT_DECIMATE DoCastAOE 28374 + 20x multi-search
	// machine (no DoCastAOE bridge), EVENT_SUMMON creature groups (no
	// summon bridge + difficulty gate), zombie single/multi search +
	// kill legs + DoAction DECIMATE_EVENT + MovementInform (no summon/
	// cross-AI / react-state / motion bridges), EMOTE_DECIMATE/SPOTS_ONE/
	// DEVOURS_ALL Talks (vortex — not ported orphaned), spell_gluth_decimate
	// + spell_gluth_zombiechow_search (no SpellScript binding bridge),
	// npc_zombie_chow (entry-unverifiable from C++ evidence — no
	// registration; ctor DoCastSelf(INFECTED_WOUND 29307) joins the
	// bridgeable-but-entry-blocked queue).
	RegisterLuaBoss("boss_gluth", 15932)

	// Sapphiron (15989, Naxxramas) — lua_scripts/northrend/boss_sapphiron.lua.
	// Ported arms (C++-exact): JustEngagedWith CastSpell(self,
	// FROST_AURA 28531, triggered) (BossAI bookkeeping + SetPhase(
	// PHASE_GROUND) have no bridge — tharon_ja precedent);
	// EVENT_CHECK_RESISTS DoCastSelf(CHECK_RESISTS 60539) 0s/30s (the
	// SpellHitTarget resist check + DATA_THE_HUNDRED_CLUB legs have no
	// bridges); EVENT_CLEAVE DoCastVictim(CLEAVE 19983) randtime(5s,
	// 15s)/randtime(5s,15s) (moroes precedent; PHASE_GROUND gating
	// documented, scheduled ungated — anubrekhan locust precedent);
	// EVENT_BERSERK Talk(EMOTE_ENRAGE 3) + DoCastSelf(BERSERK 26662)
	// 15min one-shot (anubarak one-shot precedent); JustDied CastSpell(
	// self, DIES 29357, triggered) death visual (_JustDied bookkeeping
	// has no bridge — tharon_ja precedent). EMOTE_AIR_PHASE (0),
	// EMOTE_GROUND_PHASE (1), EMOTE_BREATH (2) ride unported legs
	// (vortex — not ported orphaned). Unmodeled: InitializeAI/
	// Reset/DoAction(ACTION_BIRTH)/MovementInform + BIRTH + FLIGHT/
	// LIFTOFF/LAND hover/emote legs (no instance/flag/react/hover/
	// motion bridges), DamageTaken air-phase death prevention (no
	// health bridge — doomwalker precedent), EVENT_TAIL/DRAIN/
	// BLIZZARD/BREATH/EXPLOSION DoCastAOE legs (no DoCastAOE bridge),
	// EVENT_BLIZZARD difficulty gate (kelidan), EVENT_LIFTOFF summon
	// wing buffet 17025 + random icebolt target list, EVENT_ICEBOLT
	// targeted casts over the stored GUID vector (no summon/
	// random-target bridges), GetGUID(DATA_BLIZZARD_TARGET) selector
	// (no bridges), npc_sapphiron_blizzard (16474, ScriptedAI — no
	// RegisterLuaBoss surface, razuvious precedent — its
	// TaskScheduler DoCastSelf joins the bridgeable-but-entry-blocked
	// queue), go_sapphiron_birth (no GO bridge), the three SpellScript/
	// AuraScript loaders (no binding bridge — razelikh precedent),
	// achievement_the_hundred_club (no achievement bridge).
	RegisterLuaBoss("boss_sapphiron", 15989)

	// Grand Widow Faerlina (15953, Naxxramas) —
	// lua_scripts/northrend/boss_faerlina.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 1)
	// (event 1; BossAI bookkeeping + summons.DoZoneInCombat have no
	// bridge — tharon_ja precedent); KilledUnit Talk(SAY_SLAY 2)
	// player-gated (victim:GetObjectType()=="Player" — nalorakk
	// precedent); JustDied Talk(SAY_DEATH 3) (event 4; _JustDied
	// bookkeeping has no bridge); EVENT_POISON DoCastAOE 28796
	// widows-embrace-gated, EVENT_FIRE DoCast 28794 on random
	// target, EVENT_FRENZY DoCastSelf 28798 + Talk(EMOTE_FRENZY 5)
	// with GetDuration+1ms reschedule under Widow's Embrace
	// (GetDuration bridge, lua_creature_events.go:582); SpellHit
	// Talk(EMOTE_WIDOW_EMBRACE 4, caster).
	// Unmodeled: SummonAdds SummonCreatureGroup 1/2 + 25-man gate
	// (no summon/difficulty bridges), GetData(DATA_FRENZY_DISPELS 1)
	// (achievement bridge absent), at_faerlina_entrance (no
	// area-trigger bridge), achievement_momma_said_knock_you_out
	// (no achievement bridge), npc_faerlina_add (ScriptedAI — no
	// RegisterLuaBoss surface; DoCastVictim ADD_FIREBALL 54095 in
	// the bridgeable-but-entry-blocked queue).
	RegisterLuaBoss("boss_faerlina", 15953)

	// Maexxna (15952, Naxxramas, Arachnid Quarter) —
	// lua_scripts/northrend/boss_maexxna.lua. Entry wowhead-verified
	// (wowhead.com/?npc=15952; no NPC_ constant in the C++ tree —
	// naxxramas.h carries only BOSS_MAEXXNA, DB-side binding like
	// flamelash 9156). Ported arms (C++-exact): EVENT_SPRAY Talk(
	// EMOTE_WEB_SPRAY 2) + DoCastAOE(WEB_SPRAY 29484) 40s/40s
	// (DoCastAOE self-cast convention); EVENT_SHOCK DoCastAOE(
	// POISON_SHOCK 28741) randtime(5s,10s)/randtime(10s,20s);
	// EVENT_POISON DoCastVictim(NECROTIC_POISON 28776) randtime(10s,
	// 15s)/randtime(10s,20s) (CastSpell(nil) victim-cast);
	// EVENT_SUMMON Talk(EMOTE_SPIDERS 0) + C++-exact 30s/40s schedule
	// (EVENT_SUMMON spiderlings PORTED: Talk + urand(8,10)x SummonCreature
	// type 5, 30s/40s schedule; spiderling AI legs unported — no C++ AI
	// class); 30%-health
	// frenzy latch via 1s pump (both RAID_MODE aura IDs 54123/54124
	// checked, no difficulty bridge — faerlina precedent) -> DoCastSelf
	// FRENZY 54123. Unmodeled: EVENT_WRAP DoSummon(NPC_WEB_WRAP
	// 16486) + SetGUID/MoveJump web-wrap machine + npc_webwrap
	// (no summon / cross-AI SetGUID / MoveJump / MoveSplineInit
	// bridges); Reset aura-removal (no instance bridge).
	RegisterLuaBoss("boss_maexxna", 15952)

	// boss_heigan: Naxxramas boss script, entry 15936 (naxxramas.h
	// NPC_HEIGAN — kalecgos pass; the GetNaxxramasAI ScriptName binding
	// is instance-shimmed, the creature_template binding DB-side).
	// BossAI. Ported arms in lua_scripts/northrend/boss_heigan.lua:
	// JustEngagedWith Talk(SAY_AGGRO 0) + the four event seeds (event 1;
	// BossAI instance bookkeeping + GO eruption-tile enumeration have no
	// bridge — tharon_ja precedent; erupt's seed is GO-based, stays out),
	// KilledUnit Talk(SAY_SLAY 1) player-gated (event 3 — nalorakk/kelthuzad
	// precedent; the _safetyDance latch has no GetData bridge),
	// JustDied Talk(SAY_DEATH 3) (event 4; _JustDied instance
	// bookkeeping has no bridge), Reset SetReactState(REACT_AGGRESSIVE)
	// (event 23; _Reset() instance leg unbridged), leave-combat/death
	// timer teardown (events 2/4). EVENT_DISRUPT (15-20s->11s) +
	// EVENT_FEVER (10-20s->20-25s) ported as non-triggered self-casts
	// (29310/29998; DoCastAOE self-cast convention). EVENT_DANCE (90s,
	// PHASE_FIGHT-only) ported 2026-10-22: per-GUID dancing latch as the
	// SetPhase twin (tharon_ja idiom) + Talk(SAY_TAUNT 2) +
	// Talk(EMOTE_DANCE 4) + SetReactState(REACT_PASSIVE) +
	// creature:AttackStop() + creature:StopMoving() (bridged this tick) +
	// self-casts TELEPORT_SELF 30211 / PLAGUE_CLOUD 29350, 45s
	// DANCE_END seed; the king_dred genuinely-bridgeable-bar objection
	// is lifted by the SetReactState/AttackStop/StopMoving bridges.
	// EVENT_DANCE_END ported: Talk(EMOTE_DANCE_END 5) + disrupt/fever/dance
	// re-seeds + SetReactState(REACT_AGGRESSIVE) (CastStop vacuous —
	// instant casts only; DoZoneInCombat unbridged, documented).
	// EVENT_ERUPT stays out — whole arm rides GO bridges: TeleportCheaters,
	// GO-by-spawnId tile store firstEruptionDBGUID 84980 sections
	// 15/25/23/13, ObjectAccessor GO lookup + tile CastSpell. Documented
	// in the lua file, not wired: GetData(DATA_SAFETY_DANCE 19962139)
	// (no GetData bridge), spell_heigan_eruption (no SpellScript binding
	// bridge — razelikh precedent), achievement_safety_dance (no
	// achievement-criteria bridge — snakes precedent). The Naxxramas
	// block stays OPEN (13/17 closed; next: gothik — lua exists,
	// audit-and-close).
	RegisterLuaBoss("boss_heigan", 15936)

	// Gothik the Harvester (16060, Naxxramas) — lua_scripts/northrend/boss_gothik.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_INTRO_1 0)
	// (event 1; BossAI bookkeeping has no bridge — tharon_ja precedent);
	// EVENT_INTRO_2/3/4 Talk(SAY_INTRO_2/3/4 1/2/3) one-shot at
	// 4s/9s/14s from combat start (no phase mask, like C++);
	// KilledUnit Talk(SAY_KILL 6) player-gated (nalorakk/kelthuzad
	// variant); JustDied Talk(SAY_DEATH 5) (event 4; instance/GO legs
	// have no bridge). SAY_PHASE_TWO (4) + EMOTE_PHASE_TWO (7) +
	// EMOTE_GATE_OPENED (8) ride unported legs (vortex — not ported
	// orphaned). Unmodeled: EVENT_SUMMON waves10/waves25 + CGUID_TRIGGER
	// 127618 spawn layout (spawns resolve via DB spawn-GUID trigger
	// lookups — no spawn-ID bridge, coords not in C++ — + unported
	// wave/phase machine; difficulty gate is NOT a blocker — IsHeroic
	// is bridged), the
	// gate machine (DoAction OPEN_GATE — no instance/summon/cross-AI
	// bridges), EVENT_PHASE_TWO / EVENT_TELEPORT / EVENT_HARVEST
	// (DoCastAOE 28026/28025/28679 — no DoCastAOE bridge), EVENT_BOLT
	// DoCastVictim(SHADOW_BOLT 29317) 2s (moroes-ready in isolation but
	// its scheduling legs are the unbridged phase machine —
	// documented-only), DamageTaken phase-one zeroing (no phase bridge),
	// EnterEvadeMode NearTeleportTo (no motion bridge), the living/dead-
	// side check + FindEligibleTarget (no side/visibility/threat bridges),
	// the seven minion scripts (entries 16124-16150 local-enum-only —
	// entry-unverifiable, no registration; their DoCastAOE timers behind
	// the DoCastAOE bridge; spectralrider's Unholy Frenzy priority cast
	// + drain-life DoCastVictim 27994 moroes-ready but entry-blocked),
	// npc_gothik_trigger's SpellHit anchor machine (SpellHit never fires
	// in Lua), spell_gothik_shadow_bolt_volley (no SpellScript binding
	// bridge).
	RegisterLuaBoss("boss_gothik", 16060)

	// Thaddius (15928), Stalagg (15929), Feugen (15930), Naxxramas —
	// lua_scripts/northrend/boss_thaddius.lua, npc_stalagg.lua,
	// npc_feugen.lua.
	// Ported arms (C++-exact): boss_thaddius KilledUnit
	// Talk(SAY_SLAY 2) player-gated (nalorakk/kelthuzad/gothik
	// variant) + JustDied Talk(SAY_DEATH 4) (event 4;
	// _JustDied/setActive/cross-AI legs have no bridge); npc_stalagg
	// JustEngagedWith Talk(SAY_STALAGG_AGGRO 0) (cross-AI DoAction +
	// feugen AddThreat legs have no bridge) + UpdateAI
	// DoCastSelf(POWER_SURGE 28134) 10s/urandms(25,30) + KilledUnit
	// Talk(SAY_STALAGG_SLAY 1) player-gated; npc_feugen
	// JustEngagedWith Talk(SAY_FEUGEN_AGGRO 0) (cross-AI + stalagg
	// AddThreat legs have no bridge) + UpdateAI DoCastSelf(
	// STATIC_FIELD 28135) 6s/6s + KilledUnit Talk(SAY_FEUGEN_SLAY 1)
	// player-gated. SAY_AGGRO (1)/SAY_ELECT (3)/EMOTE_POLARITY_SHIFTED
	// (6) ride unported legs (vortex — not ported orphaned).
	// Unmodeled: the whole phase/cross-AI machine (InitializeAI phase
	// legs, DoAction pet-aggro/death/revive/reset legs, Transition(),
	// BeginResetEncounter, EVENT_REVIVE_*/EVENT_TRANSITION_*,
	// EVENT_SHIFT DoCastAOE 28089 — no DoCastAOE bridge; EVENT_CHAIN
	// DoCastVictim 28167 + EVENT_BERSERK DoCastSelf 27680 moroes/
	// phase_hunter-ready in isolation but trigger-blocked behind the
	// phase machine — gothik precedent; ball-lightning UpdateAI leg
	// 28299; CanAIAttack/react-state/flag/immunity legs), the pets'
	// feign-death DamageTaken machines + tesla-coil GO/beam machines
	// (SpellHit never fires in Lua), feugen's magnetic-pull timer
	// (the pull lives in spell_thaddius_magnetic_pull's OnCast — no
	// SpellScript bridge), npc_tesla's DamageTaken zeroing (no damage
	// bridge — entry 16218 has no registration), the three
	// polarity/magnetic-pull SpellScripts (no SpellScript binding
	// bridge), at_thaddius_entrance (no area-trigger bridge),
	// achievement_thaddius_shocking (no achievement bridge).
	RegisterLuaBoss("boss_thaddius", 15928)
	RegisterLuaBoss("npc_stalagg", 15929)
	RegisterLuaBoss("npc_feugen", 15930)

	// Patchwerk (16028, Naxxramas, Construct Quarter) —
	// lua_scripts/northrend/boss_patchwerk.lua. Entry from the
	// creature_template table (cross-checked: 15956/15953/15928/
	// 15952/15932/15936/15989/15990 all match naxxramas.h
	// NAXCreaturesIds; 25-man difficulty entry 29324 not registered,
	// base-entry-only convention). Ported arms (C++-exact):
	// JustEngagedWith Talk(0) + EVENT_HATEFUL 1s + EVENT_BERSERK
	// 6min; EVENT_HATEFUL threat-list algorithm (first non-victim
	// entry is secondThreat; next available entry is thirdThreat
	// when secondThreat is set, else secondThreat; on 25-man a
	// further available entry becomes thirdThreat with no victim
	// check; pick is victim/second/higher-HP-of-second-third) +
	// AddThreat 1000 each + DoCast(HATEFUL_STRIKE 28308,
	// triggered), 1s repeat; 5% frenzy latch folded into the 1s
	// pump (DoCastSelf FRENZY 28131, triggered + Talk 4, once);
	// EVENT_BERSERK DoCastSelf(BERSERK 26662, triggered) + Talk 3,
	// one-shot, then EVENT_SLIME DoCastAOE-lowered self-cast
	// (SLIME_BOLT 32309, triggered), 2s repeat; KilledUnit Talk(1)
	// at 1-in-5; JustDied Talk(2). DOCUMENTED-BLOCKED: instance
	// bookkeeping + timed achievement 10286 (no achievement/
	// instance bridges); empty-threat-list EnterEvadeMode leg
	// (EnterEvadeMode unbridged — pump re-arms instead).
	RegisterLuaBoss("boss_patchwerk", 16028)

	// Grobbulus (15931, Naxxramas, Construct Quarter) —
	// lua_scripts/northrend/boss_grobbulus.lua. Entry from the
	// creature_template table (same cross-check as patchwerk;
	// 25-man difficulty entry 29373 not registered). Fallout
	// slime entry 16290 is NPC_FALLOUT_SLIME in the C++ file.
	// Ported arms (C++-exact): JustEngagedWith + EVENT_CLOUD 15s
	// (DoCastAOE-lowered self-cast POISON_CLOUD 28240, 15s repeat)
	// + EVENT_INJECT 20s (SelectTarget Random/1/playerOnly/
	// -MUTATING_INJECTION: skip threat head, filter players
	// without the aura, random pick; DoCast MUTATING_INJECTION
	// 28169; re-arm 8s + round(120 * healthPct) ms) + EVENT_SPRAY
	// randtime (15s, 30s) (DoCastAOE-lowered self-cast SLIME_SPRAY
	// 28157) + EVENT_BERSERK 12min one-shot (DoCastAOE-lowered
	// self-cast BERSERK 26662, triggered); SpellHitTarget 15:
	// SLIME_SPRAY hits summon FALLOUT_SLIME 16290 at the target
	// position, type 3 TIMED_DESPAWN_OUT_OF_COMBAT, timer 0.
	// DOCUMENTED-BLOCKED: instance bookkeeping; poison-cloud AI
	// (entry not in the C++ file — spell-effect arrival, DB-side);
	// both SpellScripts (mutating-injection remove, poison-cloud
	// periodic trigger — no SpellScript bridge); BOMBARD_SLIME
	// 28280 is enum-only in the file.
	RegisterLuaBoss("boss_grobbulus", 15931)

	// Loatheb (16011, Naxxramas, Plague Quarter) —
	// lua_scripts/northrend/boss_loatheb.lua. Entry from the
	// creature_template table (same cross-check as patchwerk;
	// 25-man difficulty entry 29718 not registered). Ported arms
	// (C++-exact): JustEngagedWith + EVENT_NECROTIC_AURA 17s
	// (DoCastAOE-lowered self-cast NECROTIC_AURA 55593 + Talk 0 +
	// FADING Talk 2 at 14s + FADED Talk 1 at 17s, 20s repeat) +
	// EVENT_DEATHBLOOM 5s (DoCastAOE-lowered self-cast DEATHBLOOM
	// 29865, 30s repeat) + EVENT_SPORE 18s (DoCastSelf SUMMON_SPORE
	// 29234, 36s/15s 10-/25-man via victim-group-size proxy) +
	// EVENT_INEVITABLE_DOOM 2min (DoCastAOE-lowered self-cast
	// INEVITABLE_DOOM 29204; 30s repeat while counter <= 6, then
	// alternating 14s odd / 17s even on the incremented counter);
	// SummonedCreatureDies 21: summon casts FUNGAL_CREEP 29232
	// on itself, triggered; Reset 23: FUNGAL_CREEP removed from
	// all players on the boss map+instance. DOCUMENTED-BLOCKED:
	// instance bookkeeping; achievement_spore_loser criteria
	// script (no achievement bridge); deathbloom AuraScript
	// AfterRemove final-damage leg (no SpellScript bridge).
	RegisterLuaBoss("boss_loatheb", 16011)

	// Noth the Plaguebringer (15954, Naxxramas, Plague Quarter) —
	// lua_scripts/northrend/boss_noth.lua. Entry from the
	// creature_template table (wowhead classic npc=15954 link;
	// 25-man difficulty entry 29615 not registered).
	// Ported arms (C++-exact): JustEngagedWith Talk(0) +
	// EnterPhaseGround (DoZoneInCombat + GROUND_ATTACKABLE
	// 2s + BALCONY 90s/110s/180s + CURSE randtime(10s,25s) +
	// WARRIOR randtime(20s,30s) + 25-man BLINK
	// randtime(20s,30s)); CURSE DoCastAOE-lowered self-cast
	// 29213, randtime(50s,70s); WARRIOR Talk(1) + Talk(4) +
	// CastSummon(RAID(2,3),0,0) with the C++ HandleSummon
	// shuffle verbatim, 40s; BLINK cripple 29212 triggered
	// self-cast + random blink 29208-29211 self-cast +
	// ResetAllThreat, 40s; BALCONY phase + PASSIVE + SetFlag
	// NOT_SELECTABLE + AttackStop + StopMoving +
	// RemoveAllAuras + BALCONY_TELEPORT 3s (Talk 6 + 29216)
	// + WAVE randtime(5s,8s) + GROUND 70s/97s/120s; WAVE
	// Talk(5) + CastSummon by balconyCount, randtime(30s,45s);
	// GROUND balconyCount++ + TELEPORT_BACK 29231 + Talk(7)
	// + EnterPhaseGround; GROUND_ATTACKABLE RemoveFlag +
	// AGGRESSIVE; JustSummoned SetFarVisible + DoZoneInCombat;
	// KilledUnit Talk(2) player-gated; JustDied Talk(3);
	// DamageTaken balcony-phase lethal -> SetHealth(1) +
	// return false, 0. DOCUMENTED-BLOCKED: instance
	// bookkeeping; EnterPhaseGround IsThreatened evade leg;
	// EnterEvadeMode balcony teleport-back (modeled in Reset,
	// phase-gated); JustSummoned setActive; justBlinked
	// MoveChase workaround (no motion bridge); summons
	// tracking; 25-man via victim-group-size proxy.
	RegisterLuaBoss("boss_noth", 15954)

	// The Four Horsemen (Thane 16064, Lady 16065, Baron 30549,
	// Sir 16063, Naxxramas, Military Quarter) —
	// lua_scripts/northrend/boss_four_horsemen.lua. Entries
	// from naxxramas.h NAXCreaturesIds (25-man difficulty
	// entries 30603/30601/30600/30602 not registered).
	// Ported arms (C++-exact for all modeled arms): Reset
	// AGGRESSIVE + SetCombatMovement(false) +
	// SetCombatPulseDelay(0); JustEngagedWith Talk(0) +
	// SetCombatPulseDelay(5) + BeginFighting schedules;
	// all BERSERK 10min one-shot (26662 triggered self-cast);
	// baron/thane MARK 24s/12s (28834/28832), lady/sir MARK
	// 24s/15s (28833/28835), all triggered self-cast; baron
	// UNHOLY_SHADOW randtime(3s,7s)/randtime(10s,30s) victim
	// 28882 + SpellHitTarget Talk(2); thane METEOR
	// randtime(10s,25s)/randtime(13s,17s) random player
	// within 20yd 28884 + SpellHitTarget Talk(2) with
	// _shouldSay latch; lady VOIDZONE randtime(5s,10s)/
	// randtime(12s,18s) random player within 45yd 28863
	// triggered + Talk(2), plus 2s filler pump (nearest
	// within 45yd SHADOW_BOLT 57374, else UNYIELDING_PAIN
	// 57381 + Talk(7)); sir HOLYWRATH randtime(13s,18s)/
	// randtime(10s,18s) nearest player within 45yd 28883
	// triggered + SpellHitTarget Talk(2) with _shouldSay
	// latch, plus 2s filler pump (nearest within 45yd
	// HOLY_BOLT 57376, else CONDEMNATION 57377 + Talk(7));
	// baron/thane BeginFighting SetCombatMovement(true) +
	// AGGRESSIVE; KilledUnit Talk(3) player-gated; JustDied
	// Talk(4). DOCUMENTED-BLOCKED: the whole encounter
	// choreography (instance boss state, BeginEncounter
	// threat sharing, MovePoint corner paths, MovementInform
	// chain, BEGIN_FIGHTING gating — fight starts on engage
	// in Lua, a timing deviation); BeginFighting AttackStart
	// legs; lady/sir threat-empty evade legs; JustDied
	// instance/achievement legs; mark AuraScript +
	// consumption SpellScript (no SpellScript bridge);
	// Reset ResetLootMode + summons.DespawnAll.
	RegisterLuaBoss("boss_four_horsemen_baron", 30549)
	RegisterLuaBoss("boss_four_horsemen_thane", 16064)
	RegisterLuaBoss("boss_four_horsemen_lady", 16065)
	RegisterLuaBoss("boss_four_horsemen_sir", 16063)

	// instance_naxxramas.cpp audit (2026-10-18): pure InstanceMapScript
	// (map 533, "instance_naxxramas") — 16-boss GUID latches, door/
	// boundary/port GO data, achievement 7600/7601 + Immortal/Undying
	// criteria (AreAllEncountersDone + playerDied latch), and the
	// Kel'Thuzad dialogue chains (wing taunts, gothik->horsemen 8-line
	// chain, sapphiron/lich-king 6-line chain). All legs need the
	// unbridged instance machinery (SetBossState/GetBossState,
	// GetGuidData/SetData, GO flags). No-instance-script-bridge queue,
	// documented-only, no lua.

	// Commander Stoutbeard (26796), Commander Kolurg (26798), The
	// Nexus — lua_scripts/northrend/boss_nexus_commanders.lua.
	// Ported arms (C++-exact, name-by-name 2026-10-18): JustEngagedWith
	// Talk(SAY_AGGRO 0) + RemoveAura(FROZEN_PRISON 47543) (lua_objects.
	// go:242) + DoCastSelf(BATTLE_SHOUT 31403) + EVENT_CHARGE_COMMANDER
	// schedule ({3s,4s}) + EVENT_WHIRLWIND schedule (6s/8s) +
	// EVENT_FRIGHTENING_SHOUT schedule ({13s,15s}); EVENT_WHIRLWIND
	// DoCastSelf(WHIRLWIND 38618) 19500ms/25s repeat;
	// EVENT_FRIGHTENING_SHOUT DoCastAOE-lowered self-cast
	// (FRIGHTENING_SHOUT 19134) {13s,15s} init / {45s,55s} repeat;
	// EVENT_CHARGE_COMMANDER random-alive-player pool (SelectTarget
	// Random/0/100/playerOnly pool C++-exact) DoCast(CHARGE 60067)
	// {11s,15s} unconditional re-arm; KilledUnit Talk(SAY_KILL 1)
	// player-gated (nalorakk variant); JustDied Talk(SAY_DEATH 2)
	// (_JustDied bookkeeping has no bridge). Documented-only:
	// JustEngagedWith BossAI::JustEngagedWith instance leg (no
	// instance bridge). Single C++ script serves both faction-variant
	// commanders (twin_valkyr shared-handler precedent).
	RegisterLuaBoss("boss_nexus_commanders", 26796)
	RegisterLuaBoss("boss_nexus_commanders", 26798)

	// Grand Magus Telestra (26731), The Nexus —
	// lua_scripts/northrend/boss_magus_telestra.lua. Entry from
	// checkpoint kg-49 (wowhead-only; no NPC_ const in the C++
	// tree). Ported arms (C++-exact at 1s pump granularity):
	// JustEngagedWith Talk(0) + 1s UpdateAI pump (shared
	// uiCooldown freezes all timers; split/waiting branches
	// return early); waiting branch StopMoving + AttackStop
	// per tick, 4s, then RemoveFlag NOT_SELECTABLE; Phase 1/3
	// merge branch (all magi dead via FindNearestCreature
	// 100yd poll — Go event 20 fires only on timed expiry,
	// never on death — DoCastSelf TELESTRA_BACK 47714 +
	// SetVisible(true) + Phase 2/4 + waiting 4s + Talk 3);
	// Phase 0 HealthBelowPct(50) split + heroic Phase 2
	// HealthBelowPct(10) split (RemoveAllAuras +
	// SetVisible(false) + SetFlag NOT_SELECTABLE + summon
	// fire 26928 / frost 26930 / arcane 26929 magi with
	// visual self-casts + Talk 4); ICE_NOVA 7s/15s random
	// threat target 47772, GRAVITY_WELL 15s victim 47756,
	// FIREBOMB 0s/2s random threat target 47773, all
	// non-triggered (heroic H_ spell ids unused by the C++
	// arms, like C++). KilledUnit Talk(1) player-gated;
	// JustDied Talk(2) + SetVisible(true). DOCUMENTED-BLOCKED:
	// instance bookkeeping; heroic Winter Veil hat aura (no
	// game-event bridge); split CastStop; merge room-center
	// UpdatePosition (no position-set bridge); magi AttackStart
	// relay + magi spellcasting (no AI class in the C++ file,
	// DB-side SmartAI); DoAction/GetData split-personality
	// achievement legs (no achievement bridge).
	RegisterLuaBoss("boss_magus_telestra", 26731)

	// Anomalus (26763), The Nexus —
	// lua_scripts/northrend/boss_anomalus.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (instance->SetBossState(DATA_ANOMALUS, IN_PROGRESS) leg has no
	// instance bridge — tharon_ja precedent); JustDied Talk(SAY_DEATH
	// 1) (SetBossState(DONE) leg has no instance bridge);
	// SummonedCreatureDies chaosTheory=false leg PORTED (event 21 —
	// the stale "no summon bridge" is retracted; the latch is
	// write-only, its only reader is the unbridged achievement).
	// Documented-only: Reset Initialize + SetBossState(NOT_STARTED)
	// (no instance bridge), GetData(DATA_CHAOS_THEORY) cross-AI leg
	// (no GetData bridge), the UpdateAI home-position >60 evade hack
	// (no position bridge), the RIFT_SHIELD HasAura + GUID-lookup +
	// RemoveAurasDueToSpell machine (no aura/GUID bridges), the Phase
	// 0 + HealthBelowPct(50) shield+Talk(SAY_SHIELD 3)+SummonCreature(
	// CHAOTIC_RIFT 26918 entry-unverifiable local-enum-only)+AttackStart
	// +Talk(SAY_RIFT 2) machine stays out: the summon bridge is live
	// but 26918 has no template row (gothik-minions precedent), the
	// rift's AttackStart has no bridge, and the rift AI (burst +
	// wraith summons) has no bearer (herod precedent),
	// uiSparkTimer SelectTarget(Random,0)->DoCast(
	// SPARK 47751/57062) 5s/5s (no random-target SelectTarget bridge).
	// npc_chaotic_rift (26918 local-enum-only, gothik-minions
	// precedent — entry-unverifiable) gets no registration: its burst
	// timer (DoCast 47688/47737 via instance GetGuidData(DATA_ANOMALUS)
	// + HasAura(RIFT_SHIELD) cross-creature check) and crazed-mana-
	// wraith summon timer (NPC_CRAZED_MANA_WRAITH 26746 also
	// local-enum-only entry-unverifiable) have no instance/
	// random-target/summon bridges. achievement_chaos_theory (OnCheck
	// GetData(DATA_CHAOS_THEORY)) joins the no-achievement-bridge
	// queue (kelthuzad/thaddius precedent).
	RegisterLuaBoss("boss_anomalus", 26763)

	// Keristrasza (26723), The Nexus —
	// lua_scripts/northrend/boss_keristrasza.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0) (the
	// DoCastAOE(SPELL_INTENSE_COLD 48094) leg has no DoCastAOE bridge
	// — nexus_commanders EVENT_FRIGHTENING_SHOUT precedent; the
	// BossAI::JustEngagedWith instance-bookkeeping leg has no bridge
	// — tharon_ja precedent; the three events.ScheduleEvent legs have
	// no timer-event bridge); KilledUnit Talk(SAY_SLAY 1) player-gated
	// (nalorakk/kelthuzad variant — seventh ported variant, fifth
	// identical to nalorakk/kelthuzad/gothik/thaddius); JustDied
	// Talk(SAY_DEATH 3) (_JustDied bookkeeping has no bridge).
	// Documented-only: Reset Initialize + RemoveFlag(UNIT_FLAG_
	// STUNNED) + RemovePrison(CheckContainmentSpheres()) + _Reset()
	// (no instance/GO-state/flag bridges — containment-sphere prison
	// machine documented-only); DamageTaken HealthBelowPctDamaged(25)
	// -> Talk(SAY_ENRAGE 2)+Talk(SAY_FRENZY 5)+DoCast(SPELL_ENRAGE
	// 8599) one-shot latch (no health-pct bridge); the UpdateAI event
	// machine — EVENT_CRYSTAL_FIRE_BREATH DoCastVictim(CRYSTALFIRE_
	// BREATH 48096) 14s, EVENT_CRYSTAL_CHAINS_CRYSTALLIZE DoCast(me,
	// TAIL_SWEEP 50155) 5s (DUNGEON_MODE init 30s/11s), EVENT_TAIL_
	// SWEEP Talk(SAY_CRYSTAL_NOVA 4)+heroic DoCast(CRYSTALLIZE 48179)/
	// normal SelectTarget(Random)->DoCast(CRYSTAL_CHAINS 50997)
	// DUNGEON_MODE(30s,11s) repeat (no timer-event/random-target/
	// cast bridges); SetGUID(DATA_INTENSE_COLD) _intenseColdList leg
	// (no SetGUID/GetData bridge). containment_sphere
	// (GameObjectScript OnGossipHello — cross-AI CheckContainment
	// Spheres + RemovePrison) joins the instance-model-blocked gossip
	// queue; spell_intense_cold (AuraScript HandlePeriodicTick stack>=2
	// -> caster-AI SetGUID(DATA_INTENSE_COLD)) has no AuraScript
	// bridge; achievement_intense_cold (OnCheck _intenseColdList)
	// joins the no-achievement-bridge queue.
	RegisterLuaBoss("boss_keristrasza", 26723)

	// Drakos the Interrogator (27654), The Oculus —
	// lua_scripts/northrend/boss_drakos.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (the BossAI::JustEngagedWith instance-bookkeeping leg has no
	// bridge — tharon_ja precedent; the MAGIC_PULL/STOMP timer legs
	// are ported via CreateLuaEvent); KilledUnit Talk(SAY_KILL 1)
	// C++-UNCONDITIONAL (the anubrekhan unconditional variant —
	// second unconditional, the eighth ported variant overall);
	// JustDied Talk(SAY_DEATH 2) (_JustDied bookkeeping has no
	// bridge; the instance->DoStartTimedAchievement(ACHIEV_TIMED_
	// START_EVENT 18153) leg has no achievement bridge —
	// kelthuzad/thaddius precedent).
	// Documented-only: Reset Initialize + _Reset() (no timer-event /
	// instance bridges); the UpdateAI event machine —
	// EVENT_BOMB_SUMMON SummonCreature(NPC_UNSTABLE_SPHERE 28166)
	// 2s, EVENT_MAGIC_PULL DoCast(SPELL_MAGIC_PULL 51336) 15s,
	// EVENT_STOMP Talk(SAY_STOMP 4)+DoCast(SPELL_THUNDERING_STOMP
	// 50774) 15s (no timer-event/cast/summon bridges);
	// npc_unstable_sphere (28166, NPC_UNSTABLE_SPHERE in the file's
	// local enum, boss_drakos.cpp:31; CreatureScript
	// "npc_unstable_sphere", boss_drakos.cpp:159) — PORTED in
	// lua_scripts/northrend/npc_unstable_sphere.lua (Reset self-auras
	// + 3s pulse + 19s despawn; MoveRandom stays out, no motion
	// bridge). The boss port already summons this entry live.
	RegisterLuaBoss("boss_drakos", 27654)
	RegisterLuaBoss("npc_unstable_sphere", 28166)

	// Mage-Lord Urom (27655), The Oculus —
	// lua_scripts/northrend/boss_urom.lua.
	// Ported arms (C++-exact): KilledUnit Talk(SAY_PLAYER_KILL 7)
	// C++-GATED on TYPEID_PLAYER (the nalorakk / kelthuzad
	// player-gate variant — ninth ported variant overall, sixth
	// identical to nalorakk / kelthuzad / gothik / thaddius /
	// keristrasza); JustDied Talk(SAY_DEATH 6) (_JustDied
	// bookkeeping has no bridge — tharon_ja precedent; the
	// DoCastSelf(SPELL_DEATH_SPELL) leg stays out — summon delivery
	// leg, no SPELL_EFFECT_SUMMON bridge).
	// Documented-only: JustEngagedWith / StartAttack Talk arms —
	// _platform-state-gated (Talk(SAY_SUMMON_1..3) on the first
	// three engages, Talk(SAY_AGGRO) only on the fourth engage
	// when _platform > 2); porting either unconditionally would be
	// C++-inexact — no _platform-state / summon bridges (cast is
	// bridged; the machine is not);
	// Reset (SetControlled / SetDisableGravity / SetReactState /
	// DoCastSelf SPELL_EVOCATE 51602 / _Reset());
	// JustReachedHome (DoCastSelf SPELL_EVOCATE ported on events
	// 23/24); EnterEvadeMode
	// (center-gated, no evade / teleport / motion bridges);
	// AttackStart (z-gated DoStartNoMovement); the UpdateAI
	// teleport event machine (no timer-event / random-target /
	// cast / teleport / react-state / gravity bridges);
	// DamageTaken _isInCenter-gated NearTeleportTo; SpellHit
	// SPELL_SUMMON_MENAGERIE x3 (SetHomePosition + LeaveCombat +
	// Evocate — joins the SpellHit-15-never-fires queue);
	// spell_urom_frostbomb AuraScript joins the
	// no-AuraScript-bridge queue (keristrasza precedent); the
	// instance OnCreatureCreate SetPhaseMask leg has no instance
	// bridge.
	RegisterLuaBoss("boss_urom", 27655)
	// Varos Cloudstrider (27447), The Oculus —
	// lua_scripts/northrend/boss_varos.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (the BossAI::JustEngagedWith instance-bookkeeping leg has no
	// bridge — tharon_ja precedent); JustDied Talk(SAY_DEATH 3)
	// (_JustDied bookkeeping has no bridge — tharon_ja precedent;
	// the DoCast(me, SPELL_DEATH_SPELL, true) leg stays out —
	// summon delivery leg, no SPELL_EFFECT_SUMMON bridge).
	// Documented-only: InitializeAI DoCast(SPELL_CENTRIFUGE_SHIELD
	// 50053) gated on GetBossState(DATA_DRAKOS) != DONE; Reset
	// (events.ScheduleEvent EVENT_AMPLIFY_MAGIC / EVENT_ENERGIZE_
	// CORES_VISUAL / EVENT_CALL_AZURE); the UpdateAI energize-
	// cores / azure-captain / amplify-magic event machine — the
	// timer-event / cast / random-target legs ARE bridged and the
	// three arms are ported via CreateLuaEvent; npc_azure_ring_
	// captain entry-unverifiable (no C++-evidenced entry — joins
	// the entry-unverifiable queue; SpellHitTarget leg joins the
	// SpellHit-15-never-fires queue; DoAction / MovementInform /
	// instance legs have no bridges); spell_varos_centrifuge_
	// shield joins the no-AuraScript-bridge queue; spell_varos_
	// energize_core_area_enemy / _entry join the
	// no-SpellScript-bridge queue.
	RegisterLuaBoss("boss_varos", 27447)

	// Ley-Guardian Eregos (27656), The Oculus —
	// lua_scripts/northrend/boss_eregos.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 1)
	// (the BossAI::JustEngagedWith instance-bookkeeping leg has
	// no bridge — tharon_ja precedent; the drake-vehicle
	// FindNearestCreature achievement-void legs have no
	// creature-search bridge); KilledUnit Talk(SAY_KILL 3)
	// C++-GATED on TYPEID_PLAYER (the nalorakk / kelthuzad
	// player-gate variant — tenth ported variant overall,
	// seventh identical to nalorakk / kelthuzad / gothik /
	// thaddius / keristrasza / urom); JustDied Talk(SAY_DEATH 4)
	// (_JustDied bookkeeping has no bridge — tharon_ja
	// precedent).
	// Documented-only: Reset (Initialize _phase / void
	// booleans + _Reset() + DoAction); DoAction schedules the
	// four PHASE_NORMAL events (no timer-event / phase bridge);
	// the UpdateAI arcane event machine — EVENT_ARCANE_BARRAGE /
	// EVENT_ARCANE_VOLLEY / EVENT_ENRAGED_ASSAULT are ported via
	// CreateLuaEvent (timer-event / cast legs bridged);
	// EVENT_SUMMON_LEY_WHELP stays out (no summon delivery bridge);
	// DamageTaken heroic+health-gated phase-shift machine (Talk
	// SAY_SHIELD + DoCast SPELL_PLANAR_SHIFT 51162 + 6x SPELL_PLANAR_
	// ANOMALIES 57959 — no bridges); JustSummoned /
	// SummonedCreatureDespawn (NPC_PLANAR_ANOMALY 30879 —
	// CombatStop / MoveRandom / SPELL_PLANAR_BLAST 57976 — no
	// motion bridge; CastSpell bridged but summons never delivered); spell_eregos_planar_shift
	// joins the no-AuraScript-bridge queue (AfterEffectRemove
	// -> DoAction(ACTION_SET_NORMAL_EVENTS)); achievement_gen_
	// eregos_void x3 (achievement_ruby_void / _emerald_void /
	// _amber_void — the drake-void achievements 2044 / 2045 /
	// 2046) join the unmodeled-achievement queue (no
	// achievement bridge — the kelthuzad / thaddius precedent).
	RegisterLuaBoss("boss_eregos", 27656)

	// Malygos (28859), Eye of Eternity —
	// lua_scripts/northrend/boss_malygos.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_START_P_ONE 1)
	// (the setActive / CheckRequiredBosses / EnterEvadeMode /
	// SetBossState / DoCast SPELL_BERSERK / DoStartTimedAchievement
	// legs have no bridges — the tharon_ja BossAI-bookkeeping
	// precedent); JustDied Talk(SAY_DEATH 17) (the _JustDied
	// bookkeeping has no bridge; the gift-box-bunny GUID ->
	// SummonGameObject GO_HEART_OF_MAGIC legs, the NPC_ALEXSTRASZA
	// 32295 summon, and the 5s DespawnOrUnsummon have no instance /
	// summon / despawn bridges — tharon_ja precedent).
	// Documented-only: KilledUnit (player-gated Talk(SAY_KILLED_PLAYER_P_ONE 3 /
	// P_TWO 8 / P_THREE 15) selected by the AI's internal _phase with a 5s
	// kill-spam filter — no phase bridge, so no faithful port);
	// Reset / Initialize (gravity / immunity / flags / flight-speed legs,
	// SetPhase(PHASE_NOT_STARTED), REACT_PASSIVE, SetBossState NOT_STARTED —
	// no instance / phase / react-state bridges); the ~60-action DoAction
	// machine (land-encounter start, platform-destroy intro, vortex legs,
	// surge-of-power legs, respawn handling — gated on MotionMaster /
	// instance GUIDs / timed events — no DoAction / motion / instance
	// bridges); MovementInform (vortex takeoff-land / cyclic movement
	// points — no motion bridge); DamageTaken (surge-of-power and
	// destroy-platform health gates, immune flag flips — no health /
	// timer-event bridges); the UpdateAI arcane / vortex / power-spark /
	// phase-three disk machines (no timer-event / cast / phase bridges);
	// SpellHit (SPELL_POWER_SPARK_MALYGOS -> Talk SAY_BUFF_SPARK 14 +
	// despawn the spark; SPELL_MALYGOS_BERSERK -> Talk
	// EMOTE_HIT_BERSERKER_TIMER — no SpellHit / cast bridges, joins the
	// SpellHit-15-never-fires queue); MoveInLineOfSight (power-spark
	// proximity cast — no LOS bridge); npc_portal_eoe (NPC_PORTAL_TRIGGER
	// 30118 — SpellHit / aura-maintenance UpdateAI — no SpellHit / cast /
	// aura bridges); npc_power_spark (NPC_POWER_SPARK 30084 —
	// despawn-when-reached UpdateAI + MoveFollow legs
	// documented-only, no motion bridge; JustDied me->CastSpell(me,
	// SPELL_POWER_SPARK_DEATH 55852, true) PORTED as
	// lua_scripts/northrend/npc_power_spark.lua, event 4 —
	// triggered self-cast, no phase gate in C++);
	// npc_melee_hover_disk (30234) / npc_caster_hover_disk (30248) —
	// VehicleAI PassengerBoarded / MovementInform machines (no vehicle /
	// passenger / motion bridges); npc_nexus_lord (DoAction ->
	// EVENT_NUKE_DUMMY / EVENT_ARCANE_SHOCK / EVENT_HASTE_BUFF machine —
	// no timer-event / cast bridges); npc_scion_of_eternity (entry-
	// UNVERIFIABLE from repo sources: NPC_SURGE_OF_POWER 30334 is
	// defined at eye_of_eternity.h :60 but never referenced anywhere
	// in the C++ tree; the ScriptName->entry binding is DB-side —
	// the npc_azure_ring_captain / gothik-minions precedent) —
	// EVENT_ARCANE_BARRAGE machine, JustDied increments
	// DATA_SUMMON_DEATHS — no timer-event / instance / target-selection
	// bridges); npc_arcane_overload (NPC_ARCANE_OVERLOAD 30282 — SetGUID
	// DATA_LAST_OVERLOAD_GUID / phase-gated DespawnOrUnsummon legs — no
	// summon / despawn / instance bridges; the SpellHit
	// SPELL_ARCANE_BOMB_TRIGGER arm IS ported (event 14, see below));
	// npc_wyrmrest_skytalon (NPC_WYRMREST_SKYTALON 30161 —
	// VehicleAI phase-three disk machine — no vehicle / motion /
	// passenger bridges); npc_static_field (NPC_VORTEX_TRIGGER 30090 —
	// no bridge); the 17 SpellScript / AuraScript handlers (portal beam,
	// random portal, arcane storm, vortex dummy, vortex visual, arcane
	// overload, nexus-lord align-disk aggro, scion arcane barrage,
	// destroy-platform channel, alexstrasza bunny boom visual + event,
	// wyrmrest skytalon buddy summon + ride trigger, surge-of-power 25
	// warning selector + surge, alexstrasza gift beam + gift beam
	// visual — no SpellScript bridge / no AuraScript bridge, join the
	// standing queues); achievement_denyin_the_scion (vehicle-base entry
	// == NPC_HOVER_DISK_MELEE gate — no achievement bridge, joins the
	// unmodeled-achievement queue).
	RegisterLuaBoss("boss_malygos", 28859)

	// npc_arcane_overload (Eye of Eternity) — fight logic in
	// lua_scripts/northrend/npc_arcane_overload.lua.
	// Ported arms (C++-exact): SpellHit SPELL_ARCANE_BOMB_TRIGGER 56430 ->
	// DoCastAOE SPELL_ARCANE_BOMB_KNOCKBACK_DAMAGE 56431 (triggered) +
	// DoCastSelf SPELL_ARCANE_OVERLOAD_1 56432 (triggered), in C++ order
	// (event 14, gated on the trigger spell ID; both with the exact
	// triggered flag).
	// Documented-only: IsSummonedBy instance->SetGUID
	// DATA_LAST_OVERLOAD_GUID / phase-gated DespawnOrUnsummon legs — no
	// instance / phase / despawn bridges.
	RegisterLuaBoss("npc_arcane_overload", 30282)

	// npc_power_spark (Eye of Eternity) — fight logic in
	// lua_scripts/northrend/npc_power_spark.lua.
	// Ported arms (C++-exact): JustDied -> me->CastSpell(me,
	// SPELL_POWER_SPARK_DEATH 55852, true) (event 4 — triggered
	// self-cast, no phase gate in the C++ arm).
	// Documented-only: constructor Talk(EMOTE_POWER_SPARK_SUMMONED 0) /
	// MoveToMalygos MoveFollow (no spawn / motion bridges); UpdateAI
	// phase-one/vortex despawn gates + CHASE_MOTION_TYPE MoveFollow
	// maintenance (no per-tick UpdateAI / motion / instance bridges).
	RegisterLuaBoss("npc_power_spark", 30084)

	// Sartharion (28860), Obsidian Sanctum —
	// lua_scripts/northrend/boss_sartharion.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_SARTHARION_AGGRO 0)
	// (the BossAI::JustEngagedWith bookkeeping (setActive /
	// CheckRequiredBosses / SetBossState IN_PROGRESS), DoZoneInCombat, the
	// FetchDragons machine (drake power-aura casts 61248 / 58105 / 61251,
	// SPELL_WILL_OF_SARTHARION 61254, loot-mode ladder, drake init
	// MovePoints, UNIT_FLAG_NON_ATTACKABLE legs) and the nine-event
	// ScheduleEvent calls have no bridges — tharon_ja / malygos
	// BossAI-bookkeeping precedent); KilledUnit player-gated
	// Talk(SAY_SARTHARION_SLAY 8) (event 3 — razuvious player-gated
	// variant precedent); JustDied Talk(SAY_SARTHARION_DEATH 6) (the
	// _JustDied bookkeeping has no bridge; the three drake DisappearAndDie
	// legs via instance->GetGuidData(DATA_TENEBRON / DATA_SHADRON /
	// DATA_VESPERON) have no instance bridge).
	// Documented-only: Reset / Initialize (DrakeRespawn — instance
	// boss-state-gated drake respawn + MoveTargetedHome + flag legs — no
	// instance / motion bridges; SetBossState(DATA_PORTAL_OPEN,
	// NOT_STARTED) — no instance bridge); JustReachedHome (_Reset()
	// bookkeeping — no bridge); AddDrakeLootMode (loot-mode ladder — no
	// loot-mode bridge); FetchDragons / CallDragon (entry-switched
	// Talk(SAY_SARTHARION_CALL_TENEBRON 3 / CALL_SHADRON 4 / CALL_VESPERON 5)
	// + AddAura power-of legs + MovePoint legs — no timer-event / cast /
	// motion bridges); GetData TWILIGHT_ACHIEVEMENTS (drakeCount — no
	// achievement bridge, joins the unmodeled-achievement queue);
	// CastLavaStrikeOnTarget (fire-cyclone grid search + random-cyclone
	// CastSpell(SPELL_LAVA_STRIKE 57571) — no creature-search / cast
	// bridges); the UpdateAI nine-event machine (hard enrage SPELL_PYROBUFFET
	// 56916 — dead arm, never scheduled; flame tsunami WHISPER_LAVA_CHURN 9
	// Talk leg PORTED (30s/30s; the NPC_FLAME_TSUNAMI 30616 summon waves
	// stay out — no summon / motion bridges); flame breath Talk(SAY_BREATH 2)
	// + SPELL_FLAME_BREATH 56908; tail lash SPELL_TAIL_LASH 56910; cleave
	// SPELL_CLEAVE 56909; lava strike + urand SAY_SARTHARION_SPECIAL 7;
	// call-dragon legs) + the 35%-health SPELL_BERSERK 61632 leg (Talk
	// SAY_SARTHARION_BERSERK 1, gated on the three drake boss states) +
	// the 10%-health soft-enrage lava-strike cadence shift — no
	// health / instance bridges.
	RegisterLuaBoss("boss_sartharion", 28860)

	// Tenebron (30452), Shadron (30451), Vesperon (30449), Obsidian
	// Sanctum drake mini-bosses — lua_scripts/northrend/obsidian_sanctum.lua
	// (dummy_dragonAI base shared across the three drakes).
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0) +
	// the ScheduleEvent(EVENT_SHADOW_FISSURE / EVENT_SHADOW_BREATH)
	// timer seeds (timer legs ported via CreateLuaEvent — the
	// tharon_ja precedent's "no bridges" claim is stale; the
	// DoZoneInCombat leg alone stays out); KilledUnit player-gated
	// Talk(SAY_SLAY 1) (event 3 — razuvious player-gated variant
	// precedent); JustDied Talk(SAY_DEATH 2) (the _canLoot /
	// SetLootRecipient leg, the entry-switched power-aura removals
	// (61248 / 58105 / 61251) + DoRemoveAurasDueToSpellOnPlayers
	// legs, the acolyte KillSelf legs (FindNearestCreature 31218 /
	// 31219), the SetBossState (DATA_TENEBRON / SHADRON / VESPERON,
	// DONE) legs, and the Twilight Revenge DoCast (60639) on
	// Sartharion via instance GUID fetch have no instance /
	// creature-search bridges — the aura / cast legs are bridged
	// but instance-gated).
	// Documented-only: dummy_dragonAI Reset / SetData(DATA_CAN_LOOT) /
	// MovementInform (the waypoint machine, POINT_ID_INIT 100 /
	// POINT_ID_LAND 200 — no motion / instance bridges);
	// OpenPortal (portal grid search GO_TWILIGHT_PORTAL 193988 50.0f;
	// Tenebron egg summons 30882 / 31204; Shadron / Vesperon acolyte
	// summons 31218 / 31219; Talk(WHISPER_OPEN_PORTAL 6 /
	// WHISPER_OPENED_PORTAL 7); portal SetRespawnTime 30000 — no
	// summon / creature-search / GO-respawn bridges);
	// ExecuteEvent EVENT_SHADOW_FISSURE (random-target
	// SPELL_SHADOW_FISSURE 57579 — fissureTick ported) /
	// EVENT_SHADOW_BREATH (Talk(SAY_BREATH 3) + DoCastVictim
	// SPELL_SHADOW_BREATH 57570 — breathTick ported) — the
	// target-selection / cast legs are bridged via
	// CreateLuaEvent; the UpdateAI EVENT_FREE_MOVEMENT machine
	// stays out (no motion bridge); npc_tenebron Reset /
	// JustEngagedWith (+EVENT_HATCH_EGGS 30s ported) / UpdateAI
	// (instance legs unbridged); npc_shadron Reset (aura strips
	// 57948 / 57835 ported via HasAura/RemoveAura +
	// SetBossState(DATA_PORTAL_OPEN, NOT_STARTED) unbridged) /
	// JustEngagedWith (+EVENT_ACOLYTE_SHADRON 1min ported) /
	// UpdateAI (instance-gated OpenPortal +
	// SetBossState(IN_PROGRESS) — no instance bridges);
	// npc_vesperon Reset / JustEngagedWith (+EVENT_ACOLYTE_VESPERON
	// 1min ported) / UpdateAI (instance-gated OpenPortal — no
	// instance bridges; DoCastVictim(SPELL_TWILIGHT_TORMENT_VESP
	// 57948) bridgeable-in-isolation but rides the unbridged
	// machine);
	// npc_acolyte_of_shadron (31218): Reset AddAura(SPELL_TWILIGHT_
	// SHIFT_ENTER 57620) PORTED; DespawnOrUnsummon(28s) inexpressible
	// (Go Despawn takes no delay arg); the instance gift-aura legs
	// and JustDied's instance/player legs have no instance bridges;
	// no registration (walkdir-attached via RegisterCreatureEvent);
	// npc_acolyte_of_vesperon (31219): Reset AddAura(57620) +
	// DoCast(me, SPELL_TWILIGHT_TORMENT_VESP_ACO 58853) non-triggered
	// self-cast PORTED; JustDied self RemoveAurasDueToSpell(58853)
	// PORTED; the instance / vesperon / player legs have no
	// instance bridges; no registration (walkdir-attached);
	// npc_twilight_eggs (30882 / 31204): Reset AddAura(57620)
	// PORTED; EVENT_TWILIGHT_EGGS 20s payload / SpawnWhelps (whelp
	// summons 30890 / 31214) / JustSummoned — no summon / instance
	// bridges; no registration (walkdir-attached);
	// npc_flame_tsunami (30616): EVENT_TSUNAMI_TIMER DoCast(me,
	// SPELL_FLAME_TSUNAMI_DMG_AURA 57491) non-triggered self-cast
	// 100ms init / 500ms re-arm PORTED via CreateLuaEvent; the
	// ctor / EVENT_TSUNAMI_BUFF (lava blaze 30643 creature-search)
	// legs have no bridges; no registration (walkdir-attached);
	// npc_twilight_fissure: entry UNVERIFIABLE from C++ (no local
	// NPC enum — joins the entry-unverifiable queue) — no
	// registration; npc_twilight_whelp: EVENT_FADE_ARMOR timer leg
	// PORTED 2026-10-16;
	// achievement_twilight_assist / duo / zone
	// (GetData(TWILIGHT_ACHIEVEMENTS) >= 1 / >= 2 / == 3 — no
	// achievement bridge, join the unmodeled-achievement queue).
	RegisterLuaBoss("npc_tenebron", 30452)
	RegisterLuaBoss("npc_shadron", 30451)
	RegisterLuaBoss("npc_vesperon", 30449)

	// General Bjarngrim (28586), Halls of Lightning —
	// lua_scripts/northrend/boss_bjarngrim.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (event 1; the CallForHelp(30.0f) lieutenant-fetch leg and
	// instance->SetBossState(DATA_BJARNGRIM, IN_PROGRESS) have no
	// bridges); KilledUnit unconditional Talk(SAY_SLAY 4)
	// (event 3 — anubrekhan unconditional variant precedent);
	// JustDied Talk(SAY_DEATH 5) (event 4; the
	// SetBossState(DATA_BJARNGRIM, DONE) leg has no instance
	// bridge).
	// Documented-only: npc_stormforged_lieutenant (29240 — entry
	// verifiable from the local enum, the acolyte precedent — but no
	// Talk arms anywhere: JustEngagedWith is instance-gated
	// AttackStart, UpdateAI is cast-gated (ARC_WELD 59085 /
	// RENEW_STEEL_N 52774) — no instance / cast / timer bridges —
	// no registration); boss_bjarngrim Reset / Initialize
	// (canBuff AddAura TEMPORARY_ELECTRICAL_CHARGE 52092; the
	// unassigned-GUID lieutenant respawn loop; stance-reset +
	// DoCast(DEFENSIVE_STANCE 53790); SetEquipmentSlots 37871 /
	// 35642; SetBossState NOT_STARTED — no aura / cast /
	// equipment / instance bridges); EnterEvadeMode (canBuff flag —
	// no bridge); DoRemoveStanceAura (no aura-removal bridge);
	// UpdateAI (the stance-change machine — Talk(SAY_DEFENSIVE_STANCE
	// 1 / SAY_BATTLE_STANCE 2 / SAY_BERSEKER_STANCE 3) +
	// Talk(EMOTE 6 / 7 / 8) riding a no-timer-event machine, and
	// the stance-switched DoCastSelf / DoCastVictim timer events
	// (SPELL_REFLECTION 36096 / KNOCK_AWAY 52029 / PUMMEL 12555 /
	// IRONFORM 52022; INTERCEPT 58769 / WHIRLWIND 52027 /
	// CLEAVE 15284; MORTAL_STRIKE 16856 / SLAM 52026) — no
	// timer-event / cast bridges).
	RegisterLuaBoss("boss_bjarngrim", 28586)

	// Loken (28923), Halls of Lightning —
	// lua_scripts/northrend/boss_loken.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 2)
	// (event 1; BossAI bookkeeping passthrough, SetPhase,
	// ScheduleEvent legs, and DoStartTimedAchievement(20384)
	// have no bridges); KilledUnit player-gated Talk(SAY_SLAY 4)
	// (event 3 — razuvious player-gated variant precedent);
	// JustDied Talk(SAY_DEATH 8) (event 4; _JustDied passthrough
	// and DoRemoveAurasDueToSpellOnPlayers(59414) have no
	// bridges).
	// Documented-only: Reset / Initialize (timed-achievement
	// stop, no bridge); MoveInLineOfSight intro Talk(0) (no
	// MoveInLineOfSight bridge); EVENT_INTRO_DIALOGUE Talk(1)
	// (no timer bridge); the UpdateAI timer machine
	// (ARC_LIGHTNING 52921; LIGHTNING_NOVA 52960 with Talk(3) +
	// Talk(EMOTE 9); PULSING_SHOCKWAVE 52961 + aura 59414 —
	// no timer-event / cast / target-selection bridges);
	// DamageTaken health-pct Talk arms (5 / 6 / 7) (no
	// DamageTaken bridge); spell_loken_pulsing_shockwave
	// (distance-scaled CalculateDamage SpellScript — no
	// SpellScript bridge, joins the no-SpellScript-bridge
	// queue).
	RegisterLuaBoss("boss_loken", 28923)

	// Ionar (28546), Halls of Lightning —
	// lua_scripts/northrend/boss_ionar.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (event 1; instance->SetBossState(DATA_IONAR, IN_PROGRESS)
	// has no instance bridge); KilledUnit player-gated
	// Talk(SAY_SLAY 2) (event 3 — razuvious player-gated variant
	// precedent, seventeenth player-gated variant ported);
	// JustDied Talk(SAY_DEATH 3) (event 4;
	// lSparkList.DespawnAll() and SetBossState(DATA_IONAR, DONE)
	// have no bridges).
	// Documented-only: Reset / Initialize (flag/timer/visibility
	// legs — no bridges); SpellHit SPELL_DISPERSE 52770 split
	// machine (spark summons, visibility, flags, motion — no
	// bridges); CallBackSparks (no ObjectAccessor / motion
	// bridges); DamageTaken invisible-damage-0 (no DamageTaken
	// bridge); JustSummoned / SummonedCreatureDespawn (no
	// summon-list / cast / target-selection / motion bridges);
	// UpdateAI split machine (no timer / visibility / flag /
	// cast / motion bridges); timer casts (STATIC_OVERLOAD 52658
	// random-target; BALL_LIGHTNING 52780 victim — no timer-event
	// / cast / target-selection bridges); disperse health check
	// with Talk(SAY_SPLIT 1) riding the unmodeled machine; and
	// npc_spark_of_ionar (NPC_SPARK_OF_IONAR 28926, local enum —
	// no Talk arms anywhere, entry-verifiable but
	// bridge-blocked: REACT_PASSIVE, MovementInform point
	// machine, DamageTaken damage-0, UpdateAI boss-state/distance
	// despawn machine — no react / motion / DamageTaken / timer
	// / instance bridges; the stormforged_lieutenant
	// no-bridgeable-arms precedent, no registration).
	RegisterLuaBoss("boss_ionar", 28546)

	// Volkhan (28587), Halls of Lightning —
	// lua_scripts/northrend/boss_volkhan.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (event 1; SetPhase / ScheduleEvent legs and the
	// BossAI::JustEngagedWith passthrough have no bridges);
	// KilledUnit player-gated Talk(SAY_SLAY 3) (event 3 —
	// razuvious player-gated variant precedent, eighteenth
	// player-gated variant ported); JustDied Talk(SAY_DEATH 4)
	// (event 4; DespawnGolem ObjectAccessor / summon-list leg
	// and _JustDied passthrough have no bridges).
	// Documented-only: Reset / Initialize (flag/timer/phase
	// clears + _Reset + DespawnGolem + forge schedule — no
	// bridges); AttackStart (threat / motion — no bridges);
	// DespawnGolem / ShatterGolem (no ObjectAccessor /
	// summon-list / cast bridges); JustSummoned (no
	// summon-list / cast / motion / target-selection bridges);
	// MovementInform (no motion bridge); GetData
	// DATA_SHATTER_RESISTANT (no GetData bridge — the
	// achievement consumer); the UpdateAI timer machine
	// (Talk(2) SAY_STOMP / Talk(6) EMOTE_SHATTER arms ride the
	// SHATTERING_STOMP machine; Talk(5) EMOTE_TO_ANVIL rides
	// the forge summon-phase machine; Talk(1) SAY_FORGE rides
	// the health-check machine — no timer-event / cast /
	// motion / target-selection bridges); and
	// npc_molten_golem (NPC_MOLTEN_GOLEM 28695, local enum —
	// no Talk arms anywhere, entry-verifiable but
	// bridge-blocked: Reset timers, AttackStart, DamageTaken
	// brittle-golem transform (UpdateEntry 28681), SpellHit
	// SPELL_SHATTER despawn, UpdateAI BLAST_WAVE /
	// IMMOLATION_STRIKE machine — no DamageTaken / SpellHit /
	// timer-event / cast / motion bridges; the
	// stormforged_lieutenant no-bridgeable-arms precedent, no
	// registration); and achievement_shatter_resistant
	// (GetData(DATA_SHATTER_RESISTANT 2042) < 5 — no GetData
	// / achievement bridges; joins the unmodeled-achievement
	// queue).
	RegisterLuaBoss("boss_volkhan", 28587)

	// Maiden of Grief (27975), Halls of Stone —
	// lua_scripts/northrend/boss_maiden_of_grief.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (event 1; the BossAI::JustEngagedWith passthrough and the
	// DoStartTimedAchievement(ACHIEV_GOOD_GRIEF_START_EVENT 20383)
	// leg have no bridges); KilledUnit player-gated Talk(SAY_SLAY 1)
	// (event 3 — razuvious player-gated variant precedent,
	// nineteenth player-gated variant ported); JustDied
	// Talk(SAY_DEATH 2) (event 4; _JustDied passthrough has no
	// bridge).
	// Documented-only: Reset / Initialize (the four ScheduleEvent
	// legs + _Reset() + DoStopTimedAchievement — no timer /
	// achievement bridges); the UpdateAI event machine
	// (PARTING_SORROW random-target 59723; STORM_OF_GRIEF
	// DoCastVictim 50752; SHOCK_OF_SORROW threat reset + Talk(3)
	// SAY_STUN + DoCastAOE 50760; PILLAR_OF_WOE random-target /
	// victim 50761 — no timer-event / cast / target-selection /
	// threat bridges; Talk(3) rides the SHOCK_OF_SORROW machine).
	RegisterLuaBoss("boss_maiden_of_grief", 27975)

	// Krystallus (27977), Halls of Stone —
	// lua_scripts/northrend/boss_krystallus.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (event 1; the BossAI::JustEngagedWith passthrough and the
	// five ScheduleEvent legs have no bridges); KilledUnit
	// player-gated Talk(SAY_KILL 1) (event 3 — razuvious
	// player-gated variant precedent, twentieth player-gated
	// variant ported); JustDied Talk(SAY_DEATH 2) (event 4;
	// _JustDied passthrough has no bridge).
	// Documented-only: Reset (_Reset()); the UpdateAI event
	// machine (BOULDER_TOSS random-target 50843; GROUND_SPIKE
	// heroic-only random-target 59750; GROUND_SLAM self 50827 +
	// SHATTER 10s chain; STOMP self 48131; SHATTER self 50810 —
	// no timer-event / cast / target-selection bridges; enum
	// Yells SAY_SHATTER = 3 is declared but never Talk()ed in
	// C++); spell_krystallus_shatter (RemoveAurasDueToSpell
	// STONED 50812 + triggered SHATTER_EFFECT 50811) and
	// spell_krystallus_shatter_effect (radius-scaled hit damage)
	// — no SpellScript bridge; both join the
	// no-SpellScript-bridge queue.
	RegisterLuaBoss("boss_krystallus", 27977)

	// Sjonnir the Ironshaper (27978), Halls of Stone —
	// lua_scripts/northrend/boss_sjonnir.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (event 1; the CheckRequiredBosses / EnterEvadeMode gate and
	// the BossAI::JustEngagedWith passthrough have no bridges;
	// the six ScheduleEvent legs are ported via CreateLuaEvent);
	// timer-driven casts (maiden convention): CHAIN_LIGHTNING
	// random-target 50830; LIGHTNING_SHIELD self 50831;
	// STATIC_CHARGE DoCastVictim 50834; LIGHTNING_RING self 51849;
	// SUMMON health-tier pipe summons 27982/27979/27981/27980
	// (type 6, 30s, 20s re-arm; JustSummoned MovePoint has no
	// bridge; add AIs unported so summons are inert); FRENZY
	// triggered self 28747 (one-shot 5min); KilledUnit
	// player-gated Talk(SAY_SLAY 1) (event 3 — razuvious
	// player-gated variant precedent, twenty-first player-gated
	// variant ported); JustDied Talk(SAY_DEATH 2) (event 4;
	// _JustDied passthrough has no bridge).
	// Documented-only: Reset / Initialize (_Reset() +
	// abuseTheOoze = 0); DoAction(ACTION_OOZE_DEAD 1) /
	// GetData(DATA_ABUSE_THE_OOZE 2) (no DoAction / GetData
	// bridges; consumer: achievement_abuse_the_ooze below); the
	// npc_malformed_ooze (27981 local enum; no Talk arms — the
	// 10s/3s merge machine has no proximity / summon / despawn /
	// timer bridges; npc_spark_of_ionar no-bridgeable-arms
	// precedent — no registration); npc_iron_sludge (28165 local
	// enum; no Talk arms — JustDied DoAction leg has no
	// ObjectAccessor / GetGuidData / DoAction bridges; no
	// registration); achievement_abuse_the_ooze (GetData >= 5 —
	// no GetData / achievement bridges; joins the
	// unmodeled-achievement queue).
	RegisterLuaBoss("boss_sjonnir", 27978)

	// Auriaya (33515), Ulduar —
	// lua_scripts/northrend/boss_auriaya.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (event 1; the BossAI::JustEngagedWith passthrough, the
	// SendEncounterUnit engage leg and the five ScheduleEvent
	// legs have no bridges); KilledUnit player-gated +
	// roll_chance_i(50) Talk(SAY_SLAY 1) (event 3 — razuvious
	// player-gated variant precedent plus the chance gate:
	// math.random(1, 100) <= 50; first chance-gated slay variant
	// ported, twenty-second player-gated variant ported);
	// timer-driven casts via CreateLuaEvent (maiden/baltharus
	// convention, GUID-keyed timers, cancel on events 2/4/23):
	// EVENT_SONIC_SCREECH DoCastVictim 64422 48s/{22s,30s};
	// EVENT_TERRIFYING_SCREECH Talk(EMOTE_FEAR 3) + DoCastSelf
	// 64386 + EVENT_BLAST 1ms, 38s/{36s,45s}; EVENT_BLAST
	// DoCastAOE-as-self-cast 64389 one-shot; EVENT_SUMMON_DEFENDER
	// Talk(EMOTE_DEFENDER 4) + DoCastSelf 64448 +
	// EVENT_ACTIVATE_DEFENDER 2s, 1min one-shot;
	// EVENT_ACTIVATE_DEFENDER DoCastSelf 64449 one-shot;
	// EVENT_BERSERK DoCastSelf 47008 triggered + Talk(SAY_BERSERK 2)
	// 10min one-shot (cast-before-Talk order kept).
	// npc_sanctum_sentry (34014 local enum — newly registered):
	// EVENT_RIP DoCastVictim 64375 6s/{10s,12s}.
	// Documented-only: Reset / Initialize (_Reset() +
	// _crazyCatLady / _nineLives flags + HandleCats(true) —
	// no bridges); DoAction(ACTION_CRAZY_CAT_LADY 0 /
	// ACTION_DEFENDER_DIED 1) / GetData(DATA_NINE_LIVES 30763077
	// / DATA_CRAZY_CAT_LADY 30063007) — no DoAction / GetData
	// bridges; JustDied (DoPlaySoundToSet 15476 +
	// SendEncounterUnit disengage + HandleCats(false) grid
	// despawn — no Talk arms, no bridges; event 4 not
	// registered); the UpdateAI event machine's remaining legs
	// (EVENT_SWARNING_GUARDIAN random-target DoCast 64396 25-45s
	// + summon leg — no target-search / summon bridges);
	// npc_sanctum_sentry remaining legs (Reset self-buff 64369
	// one-shot; EVENT_SAVAGE_POUNCE random-target CatsTargetSelector
	// leg; SetWalk legs; JustDied DoAction(ACTION_CRAZY_CAT_LADY)
	// — no target-search / movement / ObjectAccessor / DoAction
	// bridges);
	// npc_feral_defender (no creature entry in C++ evidence, no
	// Talk arms — the respawn/feign-death machinery has no
	// bridges; no registration); npc_swarming_guardian (no entry
	// in C++ evidence, no Talk arms; no registration);
	// npc_seeping_essence_stalker (no entry in C++ evidence, no
	// Talk arms; no registration);
	// spell_auriaya_strenght_of_the_pack (64381), spell_auriaya_
	// sentinel_blast (64392 / 64679), spell_auriaya_agro_creator
	// (63709), spell_auriaya_feral_essence_removal (64456),
	// spell_auriaya_feral_rush (64496 / 64674) — no SpellScript
	// bridge; all five join the no-SpellScript-bridge queue;
	// spell_auriaya_random_agro_periodic (61906) — no AuraScript
	// bridge; joins the no-AuraScript-bridge queue;
	// achievement_nine_lives / achievement_crazy_cat_lady
	// (GetData-based OnCheck — no GetData / achievement bridges;
	// both join the unmodeled-achievement queue).
	RegisterLuaBoss("boss_auriaya", 33515)

	// Flame Leviathan (33113), Ulduar —
	// lua_scripts/northrend/boss_flame_leviathan.lua.
	// Ported arms (C++-exact): JustDied Talk(SAY_DEATH 2)
	// (event 4; the _JustDied() passthrough has no bridge and the
	// flag/dynflag/npcflag comment legs are DB cosmetics — the
	// sjonnir JustDied-Talk precedent; no KilledUnit talk in
	// C++ — no event 3 arms exist — and the aggro talk is
	// conditional, see below);
	// timer-driven casts via CreateLuaEvent (maiden/baltharus
	// convention, GUID-keyed timers, cancel on events 2/4/23):
	// EVENT_PURSUE Talk(SAY_TARGET 3) + DoCastSelf 62374
	// (single-arg DoCast resolves SELF via AISpellInfo default;
	// spell script selects target) 1ms/35s; EVENT_MISSILE
	// DoCastSelf 62400 triggered {1.5s,4s}/2s; EVENT_VENT
	// DoCastAOE-as-self-cast 62396 20s/20s; EVENT_SPEED
	// DoCastAOE-as-self-cast 62375 15s/15s.
	// Documented-only: Reset / Initialize / InitializeAI (no
	// bridges); JustEngagedWith — ActiveTower() picks the aggro
	// talk conditionally on tower state (ActiveTowers false ->
	// SAY_AGGRO 0; true + towers up -> SAY_HARDMODE 4; true +
	// all down -> SAY_TOWER_NONE 5) — the tower-destruction /
	// DoAction / vehicle state has no bridge, so event 1 is not
	// registered (the malygos phase-select conditional-variant
	// precedent); SpellHit (SPELL_START_THE_ENGINE /
	// SPELL_ELECTROSHOCK / SPELL_OVERLOAD_CIRCUIT ++Shutdown —
	// no bridges); GetData / SetData / DoAction (tower
	// destruction loot-mode stripping — no bridges); the
	// UpdateAI event machine's remaining legs (EVENT_SHUTDOWN
	// Talk(SAY_OVERLOAD 11) + Talk(EMOTE_OVERLOAD 13) overload /
	// vehicle leg; EVENT_REPAIR Talk(EMOTE_REPAIR 14) unit-state
	// leg; EVENT_SUMMON DoSummonFlyer; EVENT_THORIM_S_HAMMER /
	// EVENT_MIMIRON_S_INFERNO / EVENT_HODIR_S_FURY /
	// EVENT_FREYA_S_WARD tower-state-gated + summon legs — no
	// DoAction / summon / vehicle / unit-state bridges);
	// SpellHitTarget Talk(EMOTE_PURSUE 12, passenger) — no
	// vehicle bridge; boss_flame_leviathan_seat (33114 local
	// enum — PassengerBoarded -> leviathan AI Talk
	// (SAY_PLAYER_RIDING 10) — no vehicle bridge; no
	// registration); boss_flame_leviathan_defense_turret
	// (33142 ulduar.h — no Talk arms; no registration);
	// boss_flame_leviathan_defense_cannon (no Talk arms; no
	// registration); boss_flame_leviathan_overload_device
	// (33143 ulduar.h — no Talk arms; no registration);
	// boss_flame_leviathan_safety_container (33218 local enum —
	// JustDied has no Talk arms; no registration);
	// npc_mechanolift (33214 local enum — no Talk arms; no
	// registration); npc_pool_of_tar (33189 local enum — no
	// Talk arms; no registration); npc_colossus (33237 ulduar.h
	// — JustDied has no Talk arms; no registration);
	// npc_thorims_hammer / npc_mimirons_inferno /
	// npc_hodirs_fury / npc_freyas_ward / npc_freya_ward_summon
	// (beacon local enums — no Talk arms; no registrations);
	// npc_brann_bronzebeard_ulduar_intro (gossip leg — no gossip
	// model; no registration); npc_lorekeeper (line-1286 Talk is
	// commented out in C++ — dead code; no registration);
	// go_ulduar_tower (GameObjectScript — no GO bridge; no
	// registration); nine achievements (achievement_three_car_
	// garage_demolisher / chopper / siege, achievement_shutout,
	// achievement_unbroken, achievement_orbital_bombardment,
	// achievement_orbital_devastation, achievement_nuked_from_
	// orbit, achievement_orbit_uary — GetData / vehicle /
	// orbit-state OnCheck legs — no achievement bridge; all
	// nine join the unmodeled-achievement queue);
	// spell_overload_circuit / spell_tar_blaze — no AuraScript
	// bridge; both join the no-AuraScript-bridge queue;
	// spell_load_into_catapult / spell_systems_shutdown — both are
	// SpellScriptLoader classes with GetAuraScript() innards — no
	// AuraScript bridge; both join the no-AuraScript-bridge queue;
	// spell_auto_repair / spell_pursue /
	// spell_vehicle_throw_passenger — no SpellScript bridge;
	// all three join the no-SpellScript-bridge queue.
	RegisterLuaBoss("boss_flame_leviathan", 33113)

	// Ignis the Furnace Master (33118), Ulduar —
	// lua_scripts/northrend/boss_ignis.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (event 1; the BossAI::JustEngagedWith passthrough, the six
	// ScheduleEvent legs and the DoStartTimedAchievement leg have
	// no bridges — the auriaya engage-port precedent);
	// KilledUnit player-gated Talk(SAY_SLAY 4) (event 3 — the
	// razuvious player-gated variant precedent); JustDied
	// Talk(SAY_DEATH 6) (event 4; the _JustDied() passthrough has
	// no bridge — the sjonnir JustDied-Talk precedent);
	// timer-driven casts via CreateLuaEvent (maiden/baltharus
	// convention, GUID-keyed timers, cancel on events 2/4/23):
	// EVENT_JET Talk(EMOTE_JETS 7) + DoCastSelf 62680
	// 30s/{35s,40s} (Talk-before-cast order kept); EVENT_BERSERK
	// DoCastSelf 47008 triggered + Talk(SAY_BERSERK 5) 480s
	// one-shot (cast-before-Talk order kept; fixed a stale header
	// that documented the berserk as non-triggered).
	// Documented-only: Reset / Initialize (vehicle passenger
	// removal + DoStopTimedAchievement — no bridges);
	// GetData(DATA_SHATTERED 29252926) — no GetData bridge
	// (consumer: achievement_ignis_shattered below); JustSummoned
	// (NPC_IRON_CONSTRUCT faction / react-state / flag / immune
	// / root legs — no bridges); DoAction(ACTION_REMOVE_BUFF
	// 20 — RemoveAuraFromStack + GameTime <5s shattered window
	// — no bridges); the UpdateAI event machine's remaining legs
	// (EVENT_SLAG_POT Talk(SAY_SLAG_POT 2) + GRAB_POT / CHANGE_POT
	// / END_POT vehicle chain; EVENT_SCORCH Talk(SAY_SCORCH 3) +
	// SummonCreature(NPC_GROUND_SCORCH 33221); EVENT_CONSTRUCT
	// Talk(SAY_SUMMON 1) + DoSummon (self-casts left out — would
	// diverge without the summon leg) — no timer-event / summon /
	// target-selection / vehicle bridges);
	// npc_iron_construct (33121 local enum — entry-verifiable
	// but bridge-blocked: DamageTaken shatter leg + HEAT /
	// MOLTEN / BRITTLE / IsInWater machine; no Talk arms — no
	// registration); npc_scorch_ground (33221 local enum —
	// entry-verifiable but bridge-blocked: MoveInLineOfSight /
	// AddAura(SPELL_HEAT 65667) machine; no Talk arms — no
	// registration); spell_ignis_slag_pot (62717) — no
	// AuraScript bridge; joins the no-AuraScript-bridge queue;
	// achievement_ignis_shattered (GetData-based OnCheck — no
	// GetData / achievement bridges; joins the
	// unmodeled-achievement queue).
	RegisterLuaBoss("boss_ignis", 33118)

	// Razorscale (33186), Ulduar —
	// lua_scripts/northrend/boss_razorscale.lua.
	// Ported arms (C++-exact): EVENT_BERSERK DoCastSelf 47008
	// TRIGGERED + Talk(EMOTE_BERSERK 2, me) 900s one-shot,
	// no phase gate, timer-driven via CreateLuaEvent
	// (maiden/baltharus convention, GUID-keyed timers, cancel on
	// events 2/4/23; cast-before-Talk order kept); event 1 models
	// only the berserk timer leg — JustEngagedWith carries no
	// Talk arms on the boss itself (BossAI passthrough +
	// SendEncounterUnit + phase-machine ScheduleAirPhaseEvents +
	// summons DoAction + HandleMusic legs have no bridges);
	// JustDied carries no Talk arms (event 4 models only the
	// timer wipe — the auriaya JustDied-wipe precedent).
	// Documented-only: the whole air/ground/perma-ground phase
	// machine (DoAction ACTION_START_FIGHT / ACTION_GROUND_PHASE
	// / ACTION_START_PERMA_GROUND / ACTION_RETURN_TO_BASE,
	// MovementInform POINT_RAZORSCALE_*, SetPhase(PHASE_AIR /
	// PHASE_GROUND / PHASE_PERMA_GROUND), MoveSplineInit /
	// MovePoint / MoveLand / MoveTakeoff — no DoAction /
	// MovementInform / MotionMaster / spline / phase bridges);
	// EVENT_FIREBALL (SelectTarget Random + DoCast 63815,
	// {2s,3s}, PHASE_AIR) / EVENT_DEVOURING_FLAME
	// (SelectTarget Random + DoCast 63236, {6s,12s}/{10s,12s}) —
	// random-target + phase legs; EVENT_SUMMON_MINIONS/_2
	// (NPC_RAZORSCALE_SPAWNER summon leg); EVENT_FLAME_BREATH /
	// EVENT_FLAME_BREATH_GROUND (Talk EMOTE_BREATH + DoCastVictim
	// 63317/64021 — phase-gated); EVENT_WING_BUFFET +
	// EVENT_FIREBOLT + EVENT_RESUME_AIR_PHASE chain (phase /
	// motion legs); EVENT_FUSE_ARMOR (victim + HasAura 64774 gate,
	// phase-gated); EVENT_RESUME_MOVE_CHASE (motion leg);
	// DamageTaken perma-ground latch; SpellHit harpoon-hit
	// counter -> DoAction(ACTION_GROUND_PHASE); CanAIAttack /
	// JustSummoned / SummonedCreatureDies / EnterEvadeMode /
	// HandleMusic / SummonMinions — no bridges;
	// npc_expedition_commander (33210 — gossip -> DoAction +
	// DoAction-gated Talks 0/1/2 + GO harpoon summon legs);
	// npc_expedition_defender (33816 — Reset one-shot 65146,
	// DoAction/MovementInform machine); npc_expedition_trapper
	// (33259 — DoAction machine + MovementInform -> SPELL_SHACKLE
	// 62646); npc_expedition_engineer (33287 — DoAction machine +
	// spline legs; DoAction-gated Talks 0/1); npc_razorscale_
	// spawner (33245 — summon chain); npc_darkrune_watcher (33453:
	// EVENT_LIGHTNING_BOLT DoCastVictim 63809 5s/3s,
	// EVENT_CHAIN_LIGHTNING DoCastVictim 64758 34s/{9s,15s}),
	// npc_darkrune_guardian (33388: EVENT_STORMSTRIKE DoCastVictim
	// 64757 23s/{13s,25s}), npc_darkrune_sentinel (33846:
	// EVENT_HEROIC_STRIKE DoCastVictim 45026 9s/{5s,9s},
	// EVENT_BATTLE_SHOUT DoCastSelf 46763 15s/25s, EVENT_WHIRLWIND
	// DoCastSelf 63808 17s/{10s,13s}) — all three timer-driven but
	// spawner-machine adds (boss EVENT_SUMMON_MINIONS ->
	// NPC_RAZORSCALE_SPAWNER -> mole-machine GO ->
	// SPELL_TRIGGER_SUMMON_IRON_DWARVES -> SPELL_SUMMON_IRON_
	// DWARF_* — task exclusion: summon/add legs out; no
	// registrations); npc_razorscale_harpoon_fire_state (33282 —
	// SpellHit machine); npc_razorscale_devouring_flame (Reset
	// one-shot 64709/64734 triggered); go_razorscale_harpoon /
	// go_razorscale_mole_machine — no GO bridge; four spell
	// scripts (63317/64021 flame_breath, 63968/63970/63969
	// summon_iron_dwarves, 64821 fuse_armor, 62669 firebolt) — no
	// SpellScript / AuraScript bridges; two achievements
	// (iron_dwarf_medium_rare / quick_shave — GetData-based —
	// no bridges; both join the unmodeled-achievement queue).
	RegisterLuaBoss("boss_razorscale", 33186)

	// XT-002 Deconstructor (33293), Ulduar —
	// lua_scripts/northrend/boss_xt002.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (event 1; the BossAI::JustEngagedWith passthrough, the
	// phase-gated ScheduleEvent legs and the
	// DoStartTimedAchievement leg have no bridges — the auriaya
	// engage-port precedent); EVENT_ENRAGE Talk(SAY_BERSERK 5) +
	// DoCastSelf(SPELL_ENRAGE 26662) 2-arg non-triggered 600s
	// one-shot, no phase gate (scheduled bare in JustEngagedWith,
	// ExposeHeart never cancels it), timer-driven via
	// CreateLuaEvent (maiden/baltharus convention, GUID-keyed
	// timers, cancel on events 2/4/23 — Talk-before-cast order
	// kept); KilledUnit player-gated Talk(SAY_SLAY 4) (event 3 —
	// the razuvious player-gated variant precedent); JustDied
	// Talk(SAY_DEATH 6) (event 4; the _JustDied() passthrough +
	// RemoveFlag NOT_SELECTABLE leg has no bridge — the sjonnir
	// JustDied-Talk precedent; also wipes the enrage timer).
	// Documented-only: Reset / Initialize / EnterEvadeMode
	// (DoStopTimedAchievement 21027 — no bridges);
	// DoAction(ACTION_ENTER_HARD_MODE — no DoAction bridge);
	// GetData(DATA_HARD_MODE / DATA_HEALTH_RECOVERED /
	// DATA_GRAVITY_BOMB_CASUALTY — no GetData bridge);
	// SetData(DATA_TRANSFERED_HEALTH health-transfer +
	// DATA_GRAVITY_BOMB_CASUALTY — no SetData bridge); the
	// heart-exposure phase machine (ExposeHeart: SetPhase
	// PHASE_HEART + react-state / AttackStop + Talk
	// SAY_HEART_OPENED 1 + EVENT_SUBMERGE; DisposeHeart:
	// Talk SAY_HEART_CLOSED 2 + Talk EMOTE_HEART_CLOSED 9 +
	// hard-mode react-state + RescheduleEvents; PHASE_1/
	// PHASE_HEART SetPhase gates — no phase / DoAction /
	// instance-GUID bridges; the gothik phase-gating precedent);
	// EVENT_SEARING_LIGHT (DoCastSelf 63018, 9s/11s init, re-arm
	// {16s,20s}, PHASE_1-gated + GROUP_SEARING_GRAVITY) /
	// EVENT_GRAVITY_BOMB (DoCastSelf 63024, 18s/21s init, re-arm
	// {16s,20s}, PHASE_1-gated) / EVENT_TYMPANIC_TANTRUM
	// (Talk SAY_TYMPANIC_TANTRUM 3 + Talk EMOTE_TYMPANIC_TANTRUM
	// 10 + DoCastSelf 62776, 60s init, re-arm 1min, PHASE_1-gated,
	// canceled by ExposeHeart) — porting on continuous timers
	// would fabricate casts during the 30s PHASE_HEART exposures;
	// EVENT_PHASE_CHECK / EVENT_SUBMERGE (DoCastSelf 37751 +
	// Talk EMOTE_HEART_OPENED 8 + heart DoAction) /
	// EVENT_DISPOSE_HEART / EVENT_ENTER_HARD_MODE (SetFullHealth
	// + DoCastSelf 65737 TRIGGERED + LOOT_MODE_HARD_MODE_1) /
	// EVENT_RESUME_ATTACK — phase / hard-mode / DoAction machine;
	// PassengerBoarded Talk(EMOTE_SCRAPBOT 11) — no vehicle /
	// PassengerBoarded bridge; npc_xt002_heart (NullCreatureAI —
	// DoAction vehicle/cast legs; JustDied -> xt002 DoAction —
	// no bridges; no registration); npc_scrapbot (scheduler /
	// vehicle / SPELL_SCRAP_REPAIR machinery); npc_pummeller
	// (scheduler trample / arcing smash / uppercut legs);
	// npc_boombot (scheduler + DamageTaken boom leg + SPELL_BOOM
	// 62834); npc_life_spark (scheduler SPELL_SHOCK 64230 leg);
	// npc_xt_void_zone (scheduler SPELL_CONSUMPTION 64208 leg +
	// SpellInfo BP0 machine) — no scheduler / movement / vehicle
	// / damage-taken bridges; no registrations; the nine spell
	// scripts (63018/65121 searing_light_spawn_life_spark,
	// 63024/64234 gravity_bomb_aura, 63025/64233
	// gravity_bomb_damage, 62791 heart_overload_periodic, 62826
	// energy_orb (Talk SAY_SUMMON 7 rides the spell script),
	// 62775 tympanic_tantrum, 37751 submerged, 65032
	// 321_boombot_aura, 63849 exposed_heart — no SpellScript /
	// AuraScript bridges; all nine join the no-SpellScript /
	// no-AuraScript-bridge queues); the three achievements
	// (nerf_engineering / heartbreaker / nerf_gravity_bombs —
	// GetData-based OnCheck — no GetData / achievement bridges;
	// all three join the unmodeled-achievement queue).
	RegisterLuaBoss("boss_xt002", 33293)

	// General Vezax (33271), Ulduar —
	// lua_scripts/northrend/boss_general_vezax.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// + DoCast(me, SPELL_AURA_OF_DESPAIR 62692) 2-arg
	// non-triggered self-cast at engage (event 1; the
	// BossAI::JustEngagedWith passthrough, the
	// CheckShamanisticRage map-player-scan leg and the four
	// unbridgeable ScheduleEvent legs have no bridges — the
	// malygos engage-aura precedent); timer-driven casts via
	// CreateLuaEvent (maiden/baltharus convention, GUID-keyed
	// timers, cancel on events 2/4/23): EVENT_SURGE_OF_DARKNESS
	// Talk(EMOTE_SURGE_OF_DARKNESS 8) + Talk(SAY_SURGE_OF_DARKNESS
	// 2) + DoCast(me, SPELL_SURGE_OF_DARKNESS 62662) 2-arg
	// non-triggered, 60s init, re-arm {50s,70s} (no phase/machine
	// leg touches it — the vapor hard-mode leg only cancels
	// EVENT_SARONITE_VAPORS and EVENT_SEARING_FLAMES; Talk order
	// kept); EVENT_BERSERK Talk(SAY_BERSERK 4) + DoCast(me,
	// SPELL_BERSERK 26662) 2-arg non-triggered 600s one-shot
	// (Talk-before-cast order kept); KilledUnit player-gated
	// Talk(SAY_SLAY 1) (event 3 — the razuvious player-gated
	// variant precedent); JustDied Talk(SAY_DEATH 3) (event 4;
	// the _JustDied() passthrough + DoRemoveAurasDueToSpellOn
	// Players(62692) leg has no bridge — the sjonnir JustDied-Talk
	// precedent; also wipes the surge/berserk timers).
	// Documented-only: Reset / Initialize (shadowDodger /
	// smellSaronite / animusDead / vaporCount init — no bridges);
	// SpellHitTarget (SPELL_SHADOW_CRASH_HIT 62659 player leg ->
	// shadowDodger = false — no SpellHit bridge);
	// GetData(DATA_SHADOWDODGER 29962997 / DATA_SMELL_SARONITE
	// 31813188 — no GetData bridge); DoAction(ACTION_VAPORS_DIE
	// / ACTION_ANIMUS_DIE — no DoAction bridge);
	// CheckShamanisticRage map-player scan — no bridge;
	// EVENT_SHADOW_CRASH (CheckPlayersInRange / SelectTarget
	// Random + DoCast 62660, init {8s,10s}, re-arm {8s,12s}) /
	// EVENT_MARK_OF_THE_FACELESS (CheckPlayersInRange /
	// SelectTarget Random + DoCast 63276, init {35s,40s}, re-arm
	// {35s,45s}) — random-target selection legs; no target-search
	// bridge; EVENT_SEARING_FLAMES (DoCastAOE 62661, re-arm
	// {14s,17500ms} — canceled by the unbridgeable vapor
	// hard-mode leg and rescheduled by the unbridgeable
	// ACTION_ANIMUS_DIE DoAction leg — animus phases stay out);
	// EVENT_SARONITE_VAPORS (DoCast 63081 self-cast, 30s init,
	// re-arm {30s,35s} + hard-mode machine: Talk SAY_HARDMODE 5 +
	// Talk EMOTE_BARRIER 7 + summons.DespawnAll + DoCast 63364 +
	// DoCast 63145 + LOOT_MODE_HARD_MODE_1 + CancelEvents) —
	// summon leg + hard-mode phase machine; boss_saronite_animus
	// (no creature-entry enum in C++ — spell-summoned via 63145;
	// entry-unverifiable — no registration; Reset one-shot 63319
	// + EVENT_PROFOUND_OF_DARKNESS DoCastAOE(63420, TRIGGERED)
	// 3s/3s timer leg rides the unbridged summon machine —
	// task exclusion: summon/add legs out; JustDied -> vezax
	// DoAction — no DoAction bridge); npc_saronite_vapors (no
	// entry enum — spell-summoned via 63081; entry-unverifiable
	// — no registration; constructor Talk(EMOTE_VAPORS 0) —
	// no spawn bridge; MoveRandom / DamageTaken machine — no
	// movement / damage-taken / DoAction bridges); the three
	// spell scripts (63276 mark_of_the_faceless /
	// mark_of_the_faceless_leech, 63323 saronite_vapors — no
	// SpellScript / AuraScript bridges; all three join the
	// no-SpellScript / no-AuraScript-bridge queues); the two
	// achievements (shadowdodger / smell_saronite — GetData-based
	// OnCheck — no GetData / achievement bridges; both join the
	// unmodeled-achievement queue).
	RegisterLuaBoss("boss_general_vezax", 33271)

	// Assembly of Iron (Steelbreaker 32867 / Runemaster Molgeim
	// 32927 / Stormcaller Brundir 32857), Ulduar —
	// lua_scripts/northrend/boss_assembly_of_iron.lua.
	// Ported arms (C++-exact for all modeled arms): engage
	// Talk(SAY_AGGRO 0) x3 (event 1) + steelbreaker DoCast(me,
	// SPELL_HIGH_VOLTAGE 61890); EVENT_BERSERK x3 (Talk
	// SAY_BERSERK 5/6/6 + DoCast(SPELL_BERSERK 47008) self-cast,
	// 15min init, one-shot); EVENT_FUSION_PUNCH (melee-range
	// gated DoCastVictim(SPELL_FUSION_PUNCH 61903), 15s init,
	// {13s,22s} repeat); EVENT_SHIELD_OF_RUNES (DoCast(me,
	// SPELL_SHIELD_OF_RUNES 62274), 30s init, {27s,34s} repeat);
	// EVENT_OVERLOAD (Talk EMOTE 7 + SAY_SPECIAL 2 + DoCast
	// 61869 self-cast, {60s,120s}); EVENT_CHAIN_LIGHTNING
	// (random-target DoCast(SPELL_CHAIN_LIGHTNING 61879) via the
	// GetAITargets twin — the razuvious idiom — 4s init,
	// {7s,10s} repeat); KilledUnit player-gated Talk(SAY_SLAY 1)
	// x3 (event 3); JustDied Talk(SAY_DEATH 3/4/4) x3 (event 4);
	// RemoveAllAuras() on the event-23 Reset handlers (the
	// _Reset()/phase legs have no bridges).
	// Documented-only (no bridges): DoAction
	// ACTION_SUPERCHARGE 1 / ACTION_ADD_CHARGE 2 — the phase
	// machine and the death-cascade never advance, so static
	// disruption / overwhelming power / rune of death / rune of
	// summoning / lightning whirl / tendrils flight machine stay
	// unfired (C++-exact for the reachable phase-1 state — the
	// gothik precedent); GetData(DATA_PHASE_3) + the
	// achievement_assembly_i_choose_you OnCheck; EVENT_RUNE_OF_
	// POWER (random alive ally via instance GUIDs — note the
	// C++ case-2 DATA_STEELBREAKER copy-paste quirk);
	// EVENT_MOVE_POSITION + the tendrils flight/land/ground
	// machine (no motion/hover bridges); the DONE-branch kill
	// credit (no instance GetBossState bridge); the three spell
	// scripts (no AuraScript/SpellScript bridges); the
	// UNIT_STATE_CASTING gates (IsCasting is bridged-but-false
	// in Go — vacuous).
	RegisterLuaBoss("boss_steelbreaker", 32867)
	RegisterLuaBoss("boss_runemaster_molgeim", 32927)
	RegisterLuaBoss("boss_stormcaller_brundir", 32857)

	// Kologarn (32930), Ulduar —
	// lua_scripts/northrend/boss_kologarn.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO
	// 0) (event 1; the six ScheduleEvent legs, the vehicle-kit
	// DoZoneInCombat arms leg and the BossAI::JustEngagedWith
	// passthrough have no bridges — the auriaya engage-port
	// precedent); KilledUnit player-gated Talk(SAY_SLAY 1)
	// (event 3 — the razuvious player-gated variant precedent
	// — the twenty-ninth player-gated variant ported);
	// JustDied Talk(SAY_DEATH 6) (event 4; the DoCast
	// SPELL_KOLOGARN_PACIFY 63726 leg, the MoveTargetedHome
	// leg, the SetFlag(NOT_SELECTABLE) leg, the
	// SetCorpseDelay(604800) leg and the _JustDied()
	// passthrough have no bridges — the sjonnir JustDied-Talk
	// precedent).
	// Documented-only: Reset / PassengerBoarded
	// (Talk(SAY_LEFT_ARM_GONE 2) / Talk(SAY_RIGHT_ARM_GONE 3)
	// + SPELL_ARM_DEAD_DAMAGE 63629 / rubble-stalker casts /
	// EVENT_STONE_SHOUT / CRITERIA_DISARMED legs — no
	// PassengerBoarded bridge); JustSummoned (focused-eyebeam
	// visuals, REACT_PASSIVE, MoveChase(eyebeamTarget) — no
	// bridges); the UpdateAI event machine (melee check /
	// sweep / smash / stone shout / berserk Talk(SAY_BERSERK
	// 7) / arm respawns / stone grip Talk(SAY_GRAB_PLAYER 5)
	// + Talk(EMOTE_STONE_GRIP 8) / focused eyebeam — no
	// timer-event / cast / vehicle / summon bridges); the
	// eight spell scripts (spell_ulduar_rubble_summon /
	// spell_ulduar_stone_grip_cast_target /
	// spell_ulduar_cancel_stone_grip /
	// spell_ulduar_squeezed_lifeless — no SpellScript
	// bridges; spell_ulduar_stone_grip_absorb /
	// spell_ulduar_stone_grip — no AuraScript bridges;
	// spell_kologarn_stone_shout /
	// spell_kologarn_summon_focused_eyebeam — no SpellScript
	// bridges; all eight join the no-SpellScript /
	// no-AuraScript-bridge queues).
	RegisterLuaBoss("boss_kologarn", 32930)
	// Hodir (Ulduar) — fight logic in
	// lua_scripts/northrend/boss_hodir.lua. Ported arms:
	// JustEngagedWith Talk(SAY_AGGRO 0) (event 1), KilledUnit
	// player-gated Talk(SAY_SLAY 1) (event 3), DamageTaken
	// lethal -> Talk(SAY_DEATH 4) + damage=0 rewrite (event 9).
	// Timer-event Talks (flash freeze / stalactite /
	// hard-mode-failed / berserk), the DamageTaken companion
	// legs (faction flip, despawn, kill credit), helper-NPC
	// DoAction legs and both spell scripts are bridge-blocked
	// — documented in the lua header. Snowpacked-icicle
	// (33174) Reset/despawn + toasty-fire (33342) Reset/SpellHit
	// add arms are ported in the same file (ev23/ev4/ev14;
	// summoned adds need no separate registration).
	RegisterLuaBoss("boss_hodir", 32845)

	// boss_freya.lua (lua_scripts/northrend/boss_freya.lua):
	// 32906 Freya + elders 32915 / 32914 / 32913 (boss_freya,
	// boss_elder_brightleaf / boss_elder_ironbranch /
	// boss_elder_stonebark — ulduar.h NPC_FREYA line 80,
	// NPC_BRIGHTLEAF line 137, NPC_STONEBARK line 138,
	// NPC_IRONBRANCH line 136; AddSC_boss_freya(), loader
	// decl 120 / call 315 — the ELEVENTH group of the
	// "// Ulduar" block in AddNorthrendScripts()).
	// Freya: KilledUnit player-gated Talk(SAY_SLAY 2)
	// (event 3); DamageTaken lethal -> Talk(SAY_DEATH 3) +
	// damage=0 rewrite (event 9; the manual JustDied(who)
	// call's only bridgeable arm is the death Talk, inlined
	// C++-exact — the hodir precedent applies verbatim).
	// Elders (identical): JustEngagedWith Talk(SAY_ELDER_AGGRO
	// 0) gated by !HasAura(62467 DRAINED_OF_POWER) (event 1);
	// KilledUnit player-gated Talk(SAY_ELDER_SLAY 1) (event 3);
	// JustDied Talk(SAY_ELDER_DEATH 2) (event 4).
	// Freya's JustEngagedWith aggro Talk choice (SAY_AGGRO 0
	// vs SAY_AGGRO_WITH_ELDER 1), timer event machine yells,
	// wave/summon NPC scripts, two spell scripts and the four
	// achievement scripts are bridge-blocked — documented in
	// the lua header.
	RegisterLuaBoss("boss_freya", 32906)
	RegisterLuaBoss("boss_freya", 32915)
	RegisterLuaBoss("boss_freya", 32914)
	RegisterLuaBoss("boss_freya", 32913)

	// boss_thorim.lua (lua_scripts/northrend/boss_thorim.lua):
	// 32865 Thorim (boss_thorim — ulduar.h NPC_THORIM line 79;
	// AddSC_boss_thorim(), loader decl 121 / call 316 — the
	// TWELFTH group of the "// Ulduar" block in
	// AddNorthrendScripts()).
	// JustEngagedWith Talk(SAY_AGGRO_1 0) (event 1; the
	// auriaya engage-port precedent); KilledUnit player-gated
	// Talk(SAY_SLAY 4) (event 3 — the razuvious precedent —
	// at least the forty-first player-gated variant ported;
	// file-birth order puts it at sixty-second).
	// Talk(SAY_DEATH 7) lives in FinishEncounter() (called
	// from the event machine, not from JustDied) — event 4
	// deliberately not registered. Timer Talks, the pre-phase
	// DamageTaken SAY_JUMPDOWN arm (phase + instance-state
	// condition), cross-creature Talk(SAY_SPECIAL),
	// DoAction/SpellHit Talks (incl. cross-creature
	// Talk(SAY_SIF_DESPAWN 1) in the pre-phase SpellHit leg),
	// the ten spell scripts, the condition script and the
	// three achievements are bridge-blocked — documented in
	// the lua header.
	RegisterLuaBoss("boss_thorim", 32865)

	// boss_yogg_saron.lua (lua_scripts/northrend/boss_yogg_saron.lua):
	// 33134 Sara + 33288 Yogg-Saron (boss_sara / boss_yogg_saron —
	// ulduar.h NPC_SARA line 197 / NPC_YOGG_SARON line 82;
	// AddSC_boss_yogg_saron(), loader decl 122 / call 317 — the
	// THIRTEENTH group of the "// Ulduar" block in
	// AddNorthrendScripts()).
	// JustEngagedWith Talk(SAY_SARA_AGGRO 2) (event 1; the
	// auriaya engage-port precedent); JustDied
	// Talk(SAY_YOGG_SARON_DEATH 6) (event 4 — the sjonnir
	// JustDied-Talk precedent). Sara's player-gated
	// Talk(SAY_SARA_KILL 5) carries an additional
	// !IsInEvadeMode() gate with no Lua bridge — event 3
	// deliberately not registered. Voice/brain arms, the
	// keeper/illusion/gossip legs, Sara's phase-gated timer
	// arms and all thirty-three spell scripts are bridge-
	// blocked — documented in the lua header. Timer-driven
	// cast ports (CreateLuaEvent): guardian DARK_VOLLEY,
	// corruptor random-spell, crusher DIMINISH_POWER, immortal
	// DRAIN_LIFE — all ungated, non-triggered.
	RegisterLuaBoss("boss_yogg_saron", 33134)
	RegisterLuaBoss("boss_yogg_saron", 33288)
	// Algalon the Observer (Ulduar) — boss_algalon_the_observer.cpp,
	// AddSC_boss_algalon_the_observer() (loader decl 123 / call 318) — the
	// FOURTEENTH group of the "// Ulduar" block, the last Ulduar boss group
	// (boss_yogg_saron -> boss_algalon_the_observer; next call is
	// AddSC_instance_ulduar()). Bridgeable arms: KilledUnit player-gated
	// Talk(SAY_ALGALON_KILL 20) (event 3 — the razuvious player-gated
	// variant precedent — at least the fortieth player-gated variant ported
	// (39 'player-gated' lua files git-born before it)),
	// with the C++ _hasYelled 1s rate limit modeled as an os.time()
	// per-guid deadline (1s resolution); DamageTaken phase-two
	// Talk(SAY_ALGALON_PHASE_TWO 11) gated on HealthBelowPctDamaged(20) +
	// the _phaseTwo latch (event 9 — the moroes threshold+latch precedent).
	// JustEngagedWith is _firstPull-gated (START_TIMER on first pull vs
	// AGGRO later — no flag bridge, no single C++-exact yell), the 2.5%
	// _fightWon damage=0 rides the unbridgeable outro machine, the
	// collapsing-star summon Talk-only pair and the big-bang phase-machine
	// choreography are bridge-blocked — documented in the lua header.
	// Timer-driven cast ports (CreateLuaEvent): QUANTUM_STRIKE,
	// PHASE_PUNCH, COSMIC_SMASH (Talk 10 + cast), ASCEND_TO_THE_HEAVENS
	// (Talk 9 + cast, one-shot — the 2.5s-later EVENT_EVADE leg has no
	// bridge), constellation ARCANE_BARRAGE (SPELLVALUE_MAX_TARGETS=1
	// unbridgeable) — all ungated, non-triggered; the intro timer base is
	// the 8s re-pull path (the 26.5s first-pull intro is unbridged).
	RegisterLuaBoss("boss_algalon_the_observer", 32871)
	// Prince Keleseth (Utgarde Keep) — boss_keleseth.cpp,
	// AddSC_boss_keleseth() (loader decl 126 / call 321) — the FIRST group
	// of the "// Utgarde Keep - Utgarde Keep" block (instance_ulduar ->
	// boss_keleseth; next call is AddSC_boss_skarvald_dalronn()).
	// Bridgeable arms: JustEngagedWith unconditional Talk(SAY_START_COMBAT
	// 1) (event 1 — the auriaya engage-port precedent; the runemage /
	// strategist guard AttackStart leg has no bridge), JustDied
	// Talk(SAY_DEATH 5) (event 4 — the sjonnir JustDied-Talk precedent),
	// EVENT_SHADOWBOLT DoCastVictim(43667) non-triggered {2s,3s}/{2s,3s},
	// EVENT_FROST_TOMB random player 100y without the 48400 aura:
	// Talk(3)+Talk(4, target)+DoCast(target, 42672, true)+target
	// self-cast 42714, true, {14s,19s}/{14s,19s} (re-arm outside the
	// target check, C++-exact) — all on the CreateLuaEvent timer
	// convention (GUID-keyed timers, cancel on events 2/4/23). The
	// EVENT_SUMMON_SKELETONS Talk is fused with the summon (zarithrian
	// precedent), npc_frost_tomb / npc_vrykul_skeleton ride summon /
	// DoAction / instance legs, the spell_frost_tomb AuraScript and the
	// achievement_on_the_rocks GetData check are bridge-blocked —
	// documented in the lua header.
	RegisterLuaBoss("boss_keleseth", 23953)
	// Skarvald the Constructor and Dalronn the Controller (Utgarde Keep)
	// — boss_skarvald_dalronn.cpp, AddSC_boss_skarvald_dalronn() (loader
	// decl 127 / call 322) — the SECOND group of the "// Utgarde Keep -
	// Utgarde Keep" block (boss_keleseth -> boss_skarvald_dalronn).
	// Bridgeable arms: Skarvald JustEngagedWith Talk(SAY_AGGRO 0)
	// (event 1), EVENT_SKARVALD_CHARGE one-shot 5s DoCast(target, 43651)
	// non-triggered on a random 5-30yd unit (C++-exact: the :208 re-arm
	// schedules the global EVENT_CHARGE = 1003, which never matches the
	// ExecuteEvent switch — the arm fires once), EVENT_STONE_STRIKE
	// DoCastVictim(48583) non-triggered 10s/{5s,10s}, DamageTaken enrage
	// latch DoCast(me, 48193) non-triggered at 15% (event 9); Dalronn
	// EVENT_SHADOW_BOLT random player 45y DoCast(target, 43649)
	// non-triggered 1s/2.1s, EVENT_DEBILITATE random player 50y
	// DoCast(target, 43650) non-triggered 5s/{5s,10s}, 5s one-shot
	// Talk(SAY_AGGRO 0) (EVENT_DELAYED_AGGRO_SAY); both bosses'
	// KilledUnit player-gated Talk(SAY_KILL 3) (event 3); the
	// !IsInGhostForm gates are satisfied structurally (ghost entries
	// 27390 / 27389 not registered). The JustDied died-first/death
	// split is instance-GuidData / cross-creature-gated, the
	// EVENT_DEATH_RESPONSE Talk is DoAction-gated, and Dalronn's
	// EVENT_SUMMON_SKELETONS is a heroic summon leg — all
	// bridge-blocked, documented in the lua header.
	RegisterLuaBoss("boss_skarvald_the_constructor", 24200)
	RegisterLuaBoss("boss_dalronn_the_controller", 24201)
	// Ingvar the Plunderer (Utgarde Keep) — boss_ingvar_the_plunderer.cpp,
	// AddSC_boss_ingvar_the_plunderer() (loader decl 128 / call 323) — the
	// THIRD group of the "// Utgarde Keep - Utgarde Keep" block
	// (boss_skarvald_dalronn -> boss_ingvar_the_plunderer).
	// Bridgeable arms (human entry only): JustEngagedWith Talk(SAY_AGGRO
	// 0) (event 1 — the PHASE_EVENT/PHASE_UNDEAD early return is modeled
	// with the engaged latch), EVENT_CLEAVE DoCastVictim(42724)
	// {6s,12s}/{6s,12s}, EVENT_SMASH DoCastAOE-as-self-cast(42669)
	// {12s,17s}/{12s,16s}, EVENT_STAGGERING_ROAR DoCast(me, 42708)
	// {18s,21s}/{18s,22s}, EVENT_ENRAGE DoCast(me, 42705)
	// {7s,14s}/{7s,14s} (all non-triggered, all on the CreateLuaEvent
	// timer convention); DamageTaken feign-death Talk(SAY_DEATH 2)
	// (event 9 — the feignDead latch mirrors the PHASE_HUMAN ->
	// PHASE_EVENT transition and cancels the human timers, which is the
	// EventMap phase gate C++-exact); JustDied Talk(SAY_DEATH 2)
	// unconditional (event 4, both entries); KilledUnit player-gated
	// Talk(SAY_SLAY 1) (event 3, both entries). The undead entry 23980 is
	// registered because phase-2 kills and the real death yell fire on
	// it. All PHASE_UNDEAD arms (dark smash, dreadful roar, woe strike,
	// shadow axe), EVENT_SUMMON_BANSHEE, EVENT_JUST_TRANSFORMED, the
	// DoAction(ACTION_START_PHASE_2) machine, npc_annhylde_the_caller
	// (MovementInform / instance-GuidData / cross-creature resurrect
	// machine) and the two spell scripts are bridge-blocked — documented
	// in the lua header.
	RegisterLuaBoss("boss_ingvar_the_plunderer", 23954)
	RegisterLuaBoss("boss_ingvar_the_plunderer", 23980)
	// Svala Sorrowgrave (Utgarde Pinnacle) — boss_svala.cpp,
	// AddSC_boss_svala() (loader decl 132 / call 327) — the FIRST group of
	// the "// Utgarde Keep - Utgarde Pinnacle" block (utgarde_keep ->
	// boss_svala). Ported arms (lua_scripts/northrend/boss_svala.lua):
	// JustEngagedWith unconditional Talk(SAY_AGGRO 2) (event 1 — the
	// auriaya engage-port precedent), KilledUnit player-gated Talk
	// (SAY_SLAY 3) (event 3 — the razuvious player-gated variant
	// precedent), JustDied unconditional Talk(SAY_DEATH 4) (event 4 —
	// the sjonnir JustDied-Talk precedent). Timer-driven cast ports
	// (CreateLuaEvent): npc_scourge_hulk (26555) mightyBlow 48697 and
	// volatileInfection 56785 — both ungated in its UpdateAI, non-
	// triggered (the yogg_saron adds precedent). The NORMAL-phase-gated
	// EVENT_SINISTER_STRIKE / EVENT_CALL_FLAMES arms stay out (the
	// NORMAL/SACRIFICING phase machine has no bridges — the gothik
	// precedent); the whole intro Talk machine (EVENT_INTRO_* timers,
	// cross-creature Arthas talks), the EVENT_RITUAL_PREPARATION Talk
	// (SAY_SACRIFICE_PLAYER) fused with the ritual summon machine, the
	// spectator / spell_paralyze_pinnacle / achievement_incredible_hulk
	// legs are bridge-blocked — documented in the lua header; the
	// npc_ritual_channeler (27281) heroic Reset AddAura (SHADOWS_IN_THE_
	// DARK 59407, IsHeroic-gated, event 23) is ported — its paralyze timer
	// rides unbridged instance GetGuidData legs (documented in the header).
	RegisterLuaBoss("boss_svala", 26668)
	// AddSC_boss_palehoof: Gortok Palehoof (Utgarde Pinnacle) — ported arms
	// (lua_scripts/northrend/boss_palehoof.lua): JustEngagedWith
	// unconditional Talk(SAY_AGGRO 0) (event 1 — the auriaya engage-port
	// precedent), KilledUnit player-gated Talk(SAY_SLAY 1) (event 3 — the
	// razuvious player-gated variant precedent). Timer-driven cast ports
	// (CreateLuaEvent): boss EVENT_ARCING_SMASH 48260, EVENT_IMPALE 48261
	// (random player), EVENT_WITHERING_ROAR 48256 — all ungated,
	// non-triggered; the four miniboss timer kits (furbolg CRAZED /
	// CHAIN_LIGHTNING / TERRIFYING_ROAR, worgen MORTAL_WOUND / ENRAGE /
	// ENRAGE_2, rhino GORE / GRIEVOUS_WOUND / STOMP, jormungar ACID_SPIT
	// / ACID_SPLATTER / POISON_BREATH) — all ungated in their
	// ExecuteEvent switches, non-triggered (the yogg_saron adds
	// precedent). The JustDied DoPlaySoundToSet(13467) has no Talk; the
	// orb / awaken-spell / sphere GameObject / ACTION_* DoAction machine
	// (timers schedule on combat start, documented deviation) and the
	// SpellScriptLoader hooks are bridge-blocked — documented in the lua
	// header.
	RegisterLuaBoss("boss_palehoof", 26687)
	// AddSC_boss_skadi: Skadi the Ruthless (Utgarde Pinnacle) — ported
	// arms (lua_scripts/northrend/boss_skadi.lua): KilledUnit
	// player-gated Talk(SAY_KILL 1) (event 3 — the razuvious
	// player-gated variant precedent), JustDied unconditional
	// Talk(SAY_DEATH 3) (event 4 — the sjonnir JustDied-Talk precedent).
	// Timer-driven cast ports (CreateLuaEvent): the ymirjar trash kits —
	// warrior HAMSTRING 48639 / STRIKE 48640, witch doctor SHADOW_BOLT
	// 49084 / SHRINK 49089, harpooner NET 49092 (random player, 30y) /
	// THROW 49091 — all ungated in their ScheduleTasks, non-triggered
	// (the yogg_saron adds precedent). Talk(SAY_AGGRO 0) rides DoAction
	// (ACTION_START_ENCOUNTER) (event 1 deliberately not registered),
	// the SAY_DRAKE_BREATH / SAY_DRAKE_DEATH Talks, the ground-phase
	// timer kits (ACTION_GAUNTLET_END-gated — the gothik precedent), the
	// whole flying-phase / harpoon / vehicle DoAction machine, the grauf
	// spline-chain emotes (no MovementInform bridge), the SpellScript /
	// AuraScript hooks, the achievement GetData check and the area
	// trigger are bridge-blocked — documented in the lua header.
	RegisterLuaBoss("boss_skadi", 26693)
	// AddSC_boss_ymiron: King Ymiron (Utgarde Pinnacle) — ported arms
	// (lua_scripts/northrend/boss_ymiron.lua): JustEngagedWith
	// unconditional Talk(SAY_AGGRO 0) (event 1 — the auriaya engage-port
	// precedent), KilledUnit player-gated Talk(SAY_SLAY 1) (event 3 —
	// the razuvious player-gated variant precedent), JustDied
	// unconditional Talk(SAY_DEATH 2) (event 4 — the sjonnir
	// JustDied-Talk precedent). Timer-driven cast ports (CreateLuaEvent):
	// EVENT_BANE 48294, EVENT_FETID_ROT 48291, EVENT_DARK_SLASH 48292,
	// EVENT_ANCESTORS_VENGEANCE 16939 — all ungated in ExecuteEvent,
	// non-triggered (normal-mode DUNGEON_MODE values; the assembly_of_iron
	// deviation precedent for the unbridged boat-phase DelayEvents leg).
	// The MovementInform POINT_BOAT summon Talks
	// (SAY_SUMMON_BJORN/HALDOR/RANULF/TORGYN 3-6), the ancestor summon /
	// channel / REACT_PASSIVE / EVENT_RESUME_COMBAT machine and the four
	// ancestor events (gothik-gated), the spell_dark_slash SpellScript
	// OnHit hook and the achievement_kings_bane GetData check are
	// bridge-blocked — documented in the lua header.
	RegisterLuaBoss("boss_ymiron", 26861)
	// AddSC_boss_archavon: Archavon the Stone Watcher (Vault of
	// Archavon) — ported arms (lua_scripts/northrend/boss_archavon.lua):
	// timer-driven cast ports (CreateLuaEvent, GUID-keyed timers, wipe on
	// events 2/4/23): EVENT_ROCK_SHARDS 58678 (random player, 15s/15s),
	// EVENT_CHOKING_CLOUD 58960 (random player, TRIGGERED) + Talk
	// (EMOTE_LEAP 1, target) (30s/30s — the 10y min-range SelectTarget
	// gate is unbridged), EVENT_STOMP 58663 + 3s EVENT_IMPALE 58666
	// chain (45s re-arm; DoCastVictim), EVENT_BERSERK Talk(EMOTE_BERSERK
	// 0) + DoCastSelf 47008 (5min one-shot) — all ungated in
	// ExecuteEvent, non-triggered except Crushing Leap (the 2026-10-06
	// "zero bridgeable arms" closure was stale "no-timer-bridge"
	// doctrine). JustEngagedWith carries no Talk arms; no JustDied /
	// KilledUnit / DamageTaken / DoAction arms exist. The
	// spell_archavon_rock_shards SpellScript hook and the
	// UNIT_STATE_CASTING gate are bridge-blocked — documented in the lua
	// header.
	RegisterLuaBoss("boss_archavon", 31125)
	// AddSC_boss_emalon: Emalon the Storm Watcher (Vault of Archavon)
	// — ported arms (lua_scripts/northrend/boss_emalon.lua):
	// timer-driven cast ports (CreateLuaEvent, GUID-keyed timers, wipe
	// on events 2/4/23): EVENT_CHAIN_LIGHTNING 64213 (random player,
	// 5s/25s), EVENT_LIGHTNING_NOVA 64216 (DoCastAOE = self-cast,
	// 40s/40s), EVENT_BERSERK 26662 + Talk(EMOTE_BERSERK 2) (6min
	// one-shot); Tempest Minion 33998 EVENT_SHOCK 64363 (DoCastVictim,
	// 20s/20s). The EVENT_OVERCHARGE summons-list leg (+ Talk
	// EMOTE_OVERCHARGE), the Reset-time SummonCreature x4, the
	// overcharge self-stack loop (no GetAura stack-count bridge), the
	// instance->GetGuidData(DATA_EMALON) legs, SetFullHealth,
	// DespawnOrUnsummon, DoZoneInCombat and the UNIT_STATE_CASTING
	// gates are bridge-blocked — documented in the lua header.
	RegisterLuaBoss("boss_emalon", 33993)
	// AddSC_boss_koralon: Koralon the Flame Watcher (Vault of Archavon)
	// — ported arms (lua_scripts/northrend/boss_koralon.lua):
	// immediate DoCast(me, SPELL_BURNING_FURY 66721) on event 1 before
	// scheduling, then EVENT_BURNING_FURY 66721 (20s/20s),
	// EVENT_BURNING_BREATH 66665 (15s/45s), EVENT_METEOR_FISTS 66725
	// (75s/45s), EVENT_FLAME_CINDER 66684 (30s/30s, SPELL_FLAME_CINDER_B
	// never cast) — all 2-arg non-triggered, timer-driven
	// (CreateLuaEvent, GUID-keyed timers, wipe on events 2/4/23).
	// Zero Talk lines in the C++. The three SpellScript/AuraScript
	// loaders (spell_koralon_meteor_fists,
	// spell_koralon_meteor_fists_damage, spell_flame_warder_meteor_fists)
	// and the UNIT_STATE_CASTING gates are bridge-blocked —
	// documented in the lua header.
	RegisterLuaBoss("boss_koralon", 35013)
	// AddSC_boss_toravon: Toravon the Ice Watcher (Vault of Archavon)
	// — ported arms (lua_scripts/northrend/boss_toravon.lua):
	// immediate DoCastSelf(SPELL_FROZEN_MALLET 71993) on event 1 before
	// scheduling, then EVENT_FROZEN_ORB 72091 (12s/Repeat 32s — the
	// SPELLVALUE_MAX_TARGETS RAID_MODE(1, 3) leg is unbridged),
	// EVENT_WHITEOUT 72034 (25s/Repeat 38s), EVENT_FREEZING_GROUND
	// 72090 (random player — the SelectTarget position-1 tank-exclusion
	// is unbridged, 7s/Repeat 38s). Timer-driven (CreateLuaEvent,
	// GUID-keyed timers, wipe on events 2/4/23). Zero Talk lines in the
	// C++. The npc_frozen_orb_stalker Reset leg, the npc_frozen_orb
	// IsSummonedBy instance leg, the spell_toravon_random_aggro
	// SpellScript and the UNIT_STATE_CASTING gates are bridge-blocked
	// — documented in the lua header.
	RegisterLuaBoss("boss_toravon", 38433)
	// AddSC_boss_cyanigosa: Cyanigosa (Violet Hold) — ported arms
	// (lua_scripts/northrend/boss_cyanigosa.lua): JustEngagedWith
	// unconditional Talk(SAY_AGGRO 0) (event 1 — the auriaya
	// engage-port precedent), KilledUnit player-gated Talk(SAY_SLAY 1)
	// (event 3 — the razuvious player-gated variant precedent),
	// JustDied unconditional Talk(SAY_DEATH 2) (event 4 — the sjonnir
	// JustDied-Talk precedent). The SAY_SPAWN / SAY_DISRUPTION /
	// SAY_BREATH_ATTACK / SAY_SPECIAL_ATTACK enum entries are never
	// Talk()ed (dead enum text); the ScheduleTasks scheduler machine,
	// the spell_cyanigosa_arcane_vacuum SpellScript hook and the
	// achievement_defenseless GetData check are bridge-blocked —
	// documented in the lua header.
	RegisterLuaBoss("boss_cyanigosa", 31134)
	// AddSC_boss_erekem: Erekem (Violet Hold) — ported arms
	// (lua_scripts/northrend/boss_erekem.lua): JustEngagedWith
	// unconditional Talk(SAY_AGGRO 0) (event 1 — the auriaya
	// engage-port precedent), KilledUnit player-gated Talk(SAY_SLAY 1)
	// (event 3 — the razuvious player-gated variant precedent),
	// JustDied unconditional Talk(SAY_DEATH 2) (event 4 — the sjonnir
	// JustDied-Talk precedent). The SAY_SPAWN / SAY_ADD_KILLED /
	// SAY_BOTH_ADDS_KILLED enum entries are never Talk()ed anywhere
	// in the VioletHold tree (dead enum text); npc_erekem_guard
	// (29395) carries zero Talk arms and is not registered; the
	// ScheduleTasks scheduler machine, the _phase/CheckGuardAlive
	// machine, the instance GetGuidData cross-creature legs, the
	// JustReachedHome SetData leg and the guard spell-timer machine
	// are bridge-blocked — documented in the lua header.
	RegisterLuaBoss("boss_erekem", 29315)
	// AddSC_boss_ichoron: Ichoron (Violet Hold) — timer-driven cast
	// ports (lua_scripts/northrend/boss_ichoron.lua): water-bolt-volley
	// DoCastAOE 54241 {10s,15s}, water-blast random-target 54237
	// {6s,9s}, Reset triggered threat-proc 61732 on event 23; Talk
	// 0/1/2 retained. The bubble-phase machine (shrink/bubble Async,
	// shatter/DRAINED/enrage DoAction/health legs), globule machine,
	// four SpellScript/AuraScript hooks and achievement_dehydration
	// stay out — documented in the lua header.
	RegisterLuaBoss("boss_ichoron", 29313)
	// AddSC_boss_lavanthor: Lavanthor (Violet Hold) — timer-driven cast
	// ports (lua_scripts/northrend/boss_lavanthor.lua): firebolt
	// random-player 54235 1s/{5s,13s}, flame-breath DoCastVictim 54282
	// 5s/{10s,15s}, lava-burn random-target 54249 10s/{15s,23s}; zero
	// Talk lines in C++. The heroic cauterizing-flames leg stays out
	// (no difficulty bridge) — documented in the lua header.
	RegisterLuaBoss("boss_lavanthor", 29312)
	// AddSC_boss_moragg: Moragg (Violet Hold) — timer-driven cast ports
	// (lua_scripts/northrend/boss_moragg.lua): engage-immediate ray of
	// pain 54438 + ray of suffering 54442 self-casts, optic-link
	// random-player 54396 15s/25s, corrosive-saliva DoCastVictim 54527
	// 5s/10s; zero Talk lines in C++. The spell_moragg_ray and
	// spell_moragg_optic_link AuraScripts stay out (no AuraScript
	// bridge — the optic-link stage/damage ramp and the ray periodic
	// triggers are documented gaps) — documented in the lua header.
	RegisterLuaBoss("boss_moragg", 29316)
	// AddSC_boss_xevozz: Xevozz (Violet Hold) — timer-driven cast
	// ports (lua_scripts/northrend/boss_xevozz.lua): arcane-barrage-
	// volley DoCastAOE 54202 {8s,10s}/{8s,10s}, arcane-buffet random-
	// player 45y 54226 {10s,11s}/{15s,20s}; Talk 0/1/2 retained. The
	// ethereal-sphere summon leg (summon + DoAction + heroic gates),
	// SpellHit SAY_SUMMON_ENERGY, MoveFollow/JustReachedHome instance
	// legs and the SpellScript hook stay out — documented in the lua
	// header.
	RegisterLuaBoss("boss_xevozz", 29266)

	// Zuramat the Obliterator (Violet Hold) — timer-driven cast ports
	// (lua_scripts/northrend/boss_zuramat.lua): void-shift random-
	// player 60y 54361 9s/15s, shroud-of-darkness DoCastSelf 54524
	// {18s,20s}/{18s,20s}; Talk 0/1/2 retained. The summon-void-sentry
	// leg (unbridged sentry AI), the void_dance achievement and the
	// JustReachedHome instance leg stay out — documented in the lua
	// header.
	RegisterLuaBoss("boss_zuramat", 29314)

	// Bronjahm (Forge of Souls) — full timer-arm port from
	// boss_bronjahm.cpp; see lua_scripts/northrend/boss_bronjahm.lua.
	// Timer-port doctrine: MAGIC_BANE / SHADOW_BOLT / CORRUPT_SOUL /
	// FEAR / the EVENT_SOULSTORM one-shot and the DamageTaken 30%-hp
	// PHASE_1->PHASE_2 latch all ride CreateLuaEvent; Talk legs are
	// C++-exact. The JustSummoned soul-fragment machine, GetData,
	// npc_corrupted_soul_fragment (36535), the SpellScript/AuraScript
	// hooks and the achievement remain bridge-blocked — documented in
	// the lua header.
	RegisterLuaBoss("boss_bronjahm", 36497)

	// Devourer of Souls (Forge of Souls) — bridgeable Talk arms ported
	// from boss_devourer_of_souls.cpp; see
	// lua_scripts/northrend/boss_devourer_of_souls.lua.
	// Talk(MIRRORED_SOUL / UNLEASHED_SOULS / WAILING_SOULS emotes) rides
	// scheduler event legs (no-timer-bridge); KilledUnit Talk is
	// display-ID gated (no display-ID bridge); the JustDied outro
	// summon loop and the summon->AI()->Talk(SAY_JAINA_OUTRO /
	// SAY_SYLVANAS_OUTRO) legs are instance-driven Talk on other
	// creatures (no bridge); the crucible SummonCreature guard, the
	// scheduler machine (PHANTOM_BLAST / MIRRORED_SOUL / WELL_OF_SOULS
	// / UNLEASHED_SOULS face-flip / WAILING_SOULS beam sweep), the
	// SpellHitTarget / GetData(DATA_THREE_FACED) legs, the three
	// mirrored-soul SpellScript / AuraScript hooks and
	// achievement_three_faced are bridge-blocked — documented in the
	// lua header.
	RegisterLuaBoss("boss_devourer_of_souls", 36502)

	// Forgemaster Garfrost (Pit of Saron) — bridgeable Talk + timer-driven
	// cast arms ported from boss_forgemaster_garfrost.cpp; see
	// lua_scripts/northrend/boss_forgemaster_garfrost.lua.
	// Timer arms port via CreateLuaEvent: engage permafrost 70326
	// self-cast, throw-saronite 68788 random-player 7s/{12.5s,20s},
	// event-9 DamageTaken phase latches (<=66% / <=33%, floored) with
	// Talk(1)/Talk(2) + thundering-stomp 68771, forge-jump timer skeleton
	// (1500ms -> 5000ms resume -> chilling-wave 68778 5s/4s phase-2-gated
	// / deep-freeze 70381 10s/35s phase-3-gated). The Oct-2 stale
	// no-timer-bridge / no-health-pct-bridge calls are retracted. The
	// forge-jump MovementInform landing legs (forge-blade/forge-mace +
	// equipment), SpellHitTarget/GetData permafrost-stack leg, the
	// instance-driven tyrannus Talk, the permafrost SpellScript and the
	// achievement are bridge-blocked — documented in the lua header.
	RegisterLuaBoss("boss_forgemaster_garfrost", 36494)

	// Krick and Ick (Pit of Saron) — bridgeable Talk + timer-driven cast
	// arms ported from boss_krickandick.cpp; see
	// lua_scripts/northrend/boss_krickandick.lua.
	// boss_ick (36476 Ick) UpdateAI timer arms port via CreateLuaEvent:
	// MIGHTY_KICK 69021 victim-cast 20s/25s, EVENT_SPECIAL
	// {30s,35s}/{23s,28s} RAND selector (explosive-barrage 69263 AOE /
	// poison-nova Talk(0)+AOE 68989 / pursuit 68987 self-cast). The Oct-2
	// "no bridgeable own arms" call is retracted (stale no-timer-bridge
	// doctrine) — 36476 IS registered, before 36477 in loader order
	// (AddSC_boss_ick registers boss_ick first). The krick-cast toxic-waste
	// / shadow-bolt arms, the krick instance-driven Talks, the outro
	// machine, DoAction threat legs and the six SpellScript / AuraScript
	// hooks are bridge-blocked — documented in the lua header.
	RegisterLuaBoss("boss_ick", 36476)
	RegisterLuaBoss("boss_krick", 36477)
	// Scourgelord Tyrannus + Rimefang (Pit of Saron) — bridgeable Talk +
	// timer-driven cast arms ported from boss_scourgelord_tyrannus.cpp;
	// see lua_scripts/northrend/boss_scourgelord_tyrannus.lua.
	// Tyrannus combat arms port via CreateLuaEvent (overlord-brand 69172,
	// forceful-smash 69155 -> unholy-power 69167 chain, mark-of-rimefang
	// 69275 — all vehicle-ungated, intro exits before combat start);
	// Rimefang EVENT_ICY_BLAST 69232 (15s/15s, ungated by the waypoint
	// patrol) ports too, so 36661 IS registered — the Oct-2
	// non-registration rested on the stale no-timer-bridge doctrine,
	// retracted 2026-10-13. The intro machine (DoAction ACTION_START_INTRO
	// + area trigger), the overlord-brand / mark-of-rimefang aura chains,
	// the rimefang waypoint patrol and hoarfrost handoff are
	// bridge-blocked — documented in the lua header.
	RegisterLuaBoss("boss_tyrannus", 36658)
	RegisterLuaBoss("boss_rimefang", 36661)
	// Frostsworn General (Halls of Reflection) — bridgeable Talk arms
	// ported from halls_of_reflection.cpp; see
	// lua_scripts/northrend/npc_frostsworn_general.lua.
	// All other zone-script creatures in the file carry zero bridgeable
	// arms (gauntlet/escape trash expose only scheduler-schedule legs;
	// the intro/escape/uther/lich-king Talk machines ride instance
	// SetData/DoAction/timer legs with no instance-script, timer, or
	// sound bridges), so only 36723 is registered (the bronjahm
	// npc_corrupted_soul_fragment precedent); the five area triggers and
	// four spell scripts are bridge-blocked — documented in the lua
	// header.
	RegisterLuaBoss("npc_frostsworn_general", 36723)
	// Falric (Halls of Reflection) — bridgeable Talk arms ported from
	// boss_falric.cpp; see lua_scripts/northrend/boss_falric.lua.
	// JustEngagedWith Talk(SAY_AGGRO 0) -> event 1 (the auriaya
	// engage-port precedent); KilledUnit player-gated Talk(SAY_SLAY 1)
	// -> event 3 (the razuvious player-gated variant precedent, tenth
	// ported variant, eighth identical); JustDied Talk(SAY_DEATH 2) ->
	// event 4 (the sjonnir JustDied-Talk precedent). The UpdateAI
	// EventMap machine (incl. timer-driven
	// Talk(SAY_IMPENDING_DESPAIR 3)) has no-timer /
	// no-random-target-SelectTarget bridges (the boss_toravon
	// precedent), the DamageTaken hopelessness machine has no
	// health-pct bridge (the garfrost precedent), and the
	// DoZoneInCombat / instance SetBossState legs have no instance
	// bridge — documented in the lua header.
	RegisterLuaBoss("boss_falric", 38112)
	// Marwyn (Halls of Reflection) — bridgeable Talk arms ported from
	// boss_marwyn.cpp; see lua_scripts/northrend/boss_marwyn.lua.
	// JustEngagedWith Talk(SAY_AGGRO 0) -> event 1 (the auriaya
	// engage-port precedent); KilledUnit player-gated Talk(SAY_SLAY 1)
	// -> event 3 (the razuvious player-gated variant precedent,
	// eleventh ported variant, ninth identical); JustDied
	// Talk(SAY_DEATH 2) -> event 4 (the sjonnir JustDied-Talk
	// precedent). The UpdateAI EventMap machine (incl. timer-driven
	// Talk(SAY_CORRUPTED_FLESH 3)) has no-timer /
	// no-random-target-SelectTarget bridges (the boss_toravon
	// precedent), the scheduler-schedule legs and the DoZoneInCombat /
	// instance SetBossState legs have no bridge, and
	// spell_marwyn_shared_suffering has no AuraScript bridge (the
	// boss_moragg optic-link precedent) — documented in the lua
	// header. Fourth and last group of the "// Halls of Reflection"
	// block — the block CLOSES here.
	RegisterLuaBoss("boss_marwyn", 38113)
	// Lord Marrowgar (Icecrown Citadel) — bridgeable Talk arms ported
	// from boss_lord_marrowgar.cpp; see
	// lua_scripts/northrend/boss_lord_marrowgar.lua.
	// JustEngagedWith Talk(SAY_AGGRO 1) -> event 1 (the auriaya
	// engage-port precedent); KilledUnit player-gated Talk(SAY_KILL 4)
	// -> event 3 (the razuvious player-gated variant precedent,
	// twelfth ported variant, tenth identical); JustDied
	// Talk(SAY_DEATH 5) -> event 4 (the sjonnir JustDied-Talk
	// precedent). npc_coldflame (36672) and npc_bone_spike (36619)
	// expose zero own Talk lines — not registered (the bronjahm
	// npc_corrupted_soul_fragment precedent). The UpdateAI EventMap
	// machine (incl. timer-driven Talk(EMOTE_BONE_STORM 7),
	// Talk(SAY_BONE_STORM 2) and Talk(SAY_BERSERK 6)) has no-timer /
	// no-random-target-SelectTarget / no-cast / no-motion /
	// no-health-pct bridges (the boss_toravon precedent); the
	// Talk(SAY_ENTER_ZONE 0) leg rides DoAction with no bridge, its
	// at_lord_marrowgar_entrance trigger joins the unbridged
	// area-trigger queue, Talk(SAY_BONESPIKE 3) rides the
	// no-SpellScript-bridge queue with the other five spell scripts,
	// and spell_marrowgar_coldflame_damage rides the
	// no-AuraScript-bridge queue — documented in the lua header.
	// First group of the "// Icecrown Citadel" block.
	RegisterLuaBoss("boss_lord_marrowgar", 36612)

	// Lady Deathwhisper (Icecrown Citadel) — bridgeable Talk arms
	// ported from boss_lady_deathwhisper.cpp; see
	// lua_scripts/northrend/boss_lady_deathwhisper.lua.
	// boss_lady_deathwhisper: JustEngagedWith Talk(SAY_AGGRO 7)
	// (event 1, the auriaya engage-port precedent);
	// KilledUnit Talk(SAY_KILL 14) victim->GetTypeId() ==
	// TYPEID_PLAYER gated (event 3, the razuvious player-gated
	// variant precedent — THIRTEENTH ported variant, ELEVENTH
	// identical to
	// nalorakk/kelthuzad/gothik/thaddius/garfrost/krick/tyrannus/falric/marwyn/marrowgar);
	// JustDied Talk(SAY_DEATH 16) (event 4, the sjonnir
	// JustDied-Talk precedent). npc_darnavan: JustEngagedWith
	// Talk(SAY_DARNAVAN_AGGRO 0) (event 1) registered for both
	// raid-mode entries 38472 / 38485. No other bridgeable arm in
	// the file: the intro DoAction/scheduler machine
	// (Talk(SAY_INTRO_1 0 .. SAY_INTRO_7 6) — no-DoAction bridge /
	// no-timer-bridge), the berserk/dominate-mind/dark-empowerment
	// timer legs (no-timer-bridge), the phase-transition
	// Talk(SAY_PHASE_2 8) + Talk(EMOTE_PHASE_2 9) mana-pct gate (no
	// health-pct bridge), the cross-AI Talk(SAY_ANIMATE_DEAD 13)
	// and darnavan->AI()->Talk(SAY_DARNAVAN_RESCUED 1) legs (no
	// bridge), the four spell scripts (no-AuraScript-bridge /
	// no-SpellScript-bridge queues), the cult adds / vengeful
	// shades (zero own Talk, not registered) and the
	// at_lady_deathwhisper_entrance trigger (unbridged-area-trigger
	// queue) are bridge-blocked — documented in the lua header.
	// Second group of the "// Icecrown Citadel" block.
	RegisterLuaBoss("boss_lady_deathwhisper", 36855)
	RegisterLuaBoss("npc_darnavan", 38472)
	RegisterLuaBoss("npc_darnavan", 38485)

	// Deathbringer Saurfang (Icecrown Citadel) — bridgeable Talk
	// arms ported from boss_deathbringer_saurfang.cpp; see
	// lua_scripts/northrend/boss_deathbringer_saurfang.lua.
	// boss_deathbringer_saurfang: JustEngagedWith Talk(SAY_AGGRO 7)
	// (event 1, the auriaya engage-port precedent);
	// KilledUnit Talk(SAY_KILL 10) victim->GetTypeId() ==
	// TYPEID_PLAYER gated (event 3, the razuvious player-gated
	// variant precedent — FOURTEENTH ported variant, TWELFTH
	// identical to
	// nalorakk/kelthuzad/gothik/thaddius/garfrost/krick/tyrannus/falric/marwyn/marrowgar/deathwhisper).
	// No other bridgeable arm in the file: Talk(SAY_FRENZY 11)
	// rides a DamageTaken HealthBelowPct(31) gate (no health-pct
	// bridge), Talk(SAY_DEATH 13) rides the DamageTaken FightWonValue
	// health gate (JustDied is empty — the sjonnir JustDied-Talk
	// precedent does not apply), Talk(SAY_MARK_OF_THE_FALLEN_CHAMPION
	// 8) rides a SpellHitTarget switch (no-SpellHit-bridge queue),
	// the intro Talk (SAY_INTRO_ALLIANCE_2/3/6/7,
	// SAY_INTRO_HORDE_2/4/9) rides the scheduler machine
	// (no-timer-bridge; DoAction legs have no-DoAction bridge), and
	// Talk(SAY_BLOOD_BEASTS 9) / Talk(SAY_BERSERK 12) /
	// Talk(EMOTE_SCENT_OF_BLOOD 14) ride scheduler legs
	// (no-timer-bridge). npc_high_overlord_saurfang_icc and
	// npc_muradin_bronzebeard_icc expose Talk only on
	// DoAction/scheduler/movement legs (no-DoAction /
	// no-timer-bridge / no-motion bridges; gossip entry is
	// instance-model-blocked) and npc_saurfang_event has zero Talk
	// lines — not registered (the bronjahm
	// npc_corrupted_soul_fragment precedent). The 9 spell scripts
	// join the no-SpellScript-bridge queue, the 3 aura scripts the
	// no-AuraScript-bridge queue, and achievement_ive_gone_and_made_a_mess
	// the no-achievement bridge queue — documented in the lua
	// header. Fourth group of the "// Icecrown Citadel" block.
	RegisterLuaBoss("boss_deathbringer_saurfang", 37813)

	// Festergut (Icecrown Citadel) — bridgeable Talk arms ported
	// from boss_festergut.cpp; see
	// lua_scripts/northrend/boss_festergut.lua.
	// boss_festergut: JustEngagedWith Talk(SAY_AGGRO 1) (event 1,
	// the auriaya engage-port precedent); KilledUnit Talk(SAY_KILL
	// 7) victim->GetTypeId() == TYPEID_PLAYER gated (event 3, the
	// razuvious player-gated variant precedent — FIFTEENTH ported
	// variant, THIRTEENTH identical to
	// nalorakk/kelthuzad/gothik/thaddius/garfrost/krick/tyrannus/falric/marwyn/marrowgar/deathwhisper/saurfang);
	// JustDied Talk(SAY_DEATH 9) (event 4, the sjonnir JustDied-Talk
	// precedent). No other bridgeable arm in the file:
	// Talk(SAY_PUNGENT_BLIGHT 4) / Talk(EMOTE_WARN_PUNGENT_BLIGHT
	// 5) / Talk(EMOTE_GAS_SPORE 2) / Talk(EMOTE_WARN_GAS_SPORE 3)
	// / Talk(SAY_BERSERK 8) ride scheduler legs (no-timer-bridge);
	// Talk(EMOTE_PUNGENT_BLIGHT 6) rides the pungent-blight
	// SpellScript and Talk(SAY_STINKY_DEAD 0) rides the stinky
	// cross-AI JustDied leg (no-SpellScript / no cross-AI-Talk
	// bridge). npc_stinky_icc has zero own Talk lines — not
	// registered (the bronjahm npc_corrupted_soul_fragment
	// precedent). The 2 spell scripts join the
	// no-SpellScript-bridge queue, the 1 aura script the
	// no-AuraScript-bridge queue, and achievement_flu_shot_shortage
	// the no-achievement bridge queue — documented in the lua
	// header. Fifth group of the "// Icecrown Citadel" block.
	RegisterLuaBoss("boss_festergut", 36626)

	// Rotface (Icecrown Citadel) — bridgeable Talk arms ported
	// from boss_rotface.cpp; see
	// lua_scripts/northrend/boss_rotface.lua.
	// boss_rotface: JustEngagedWith Talk(SAY_AGGRO 1) (event 1,
	// the auriaya engage-port precedent); KilledUnit Talk(SAY_KILL
	// 6) victim->GetTypeId() == TYPEID_PLAYER gated (event 3, the
	// razuvious player-gated variant precedent — SIXTEENTH ported
	// variant, FOURTEENTH identical to
	// nalorakk/kelthuzad/gothik/thaddius/garfrost/krick/tyrannus/falric/marwyn/marrowgar/deathwhisper/saurfang/festergut);
	// JustDied Talk(SAY_DEATH 8) (event 4, the sjonnir JustDied-Talk
	// precedent). No other bridgeable arm in the file:
	// Talk(SAY_SLIME_SPRAY 3) rides the SPELL_SLIME_SPRAY case of
	// the SpellHitTarget switch (no-SpellHit-bridge);
	// Talk(EMOTE_SLIME_SPRAY 2) rides the EVENT_SLIME_SPRAY
	// scheduler leg (no-timer-bridge); Talk(SAY_PRECIOUS_DIES 0)
	// rides npc_precious_icc JustDied as cross-AI Talk (no bridge);
	// Talk(EMOTE_PRECIOUS_ZOMBIES 0) rides its EVENT_SUMMON_ZOMBIES
	// scheduler leg (no-timer-bridge); npc_precious_icc has zero
	// bridgeable arms — not registered (the bronjahm
	// npc_corrupted_soul_fragment precedent); npc_little_ooze /
	// npc_big_ooze have zero Talk lines — not registered. The 10
	// spell scripts (Talk(EMOTE_MUTATED_INFECTION 9),
	// Talk(EMOTE_UNSTABLE_2..4) / Talk(EMOTE_UNSTABLE_EXPLOSION),
	// rotface->AI()->Talk(SAY_UNSTABLE_EXPLOSION 5)) join the
	// no-SpellScript-bridge queue and
	// spell_rotface_mutated_infection_aura the no-AuraScript-bridge
	// queue — documented in the lua header. Sixth group of the
	// "// Icecrown Citadel" block.
	RegisterLuaBoss("boss_rotface", 36627)

	// Professor Putricide (Icecrown Citadel) — bridgeable Talk arms
	// ported from boss_professor_putricide.cpp; see
	// lua_scripts/northrend/boss_professor_putricide.lua.
	// boss_professor_putricide: JustEngagedWith Talk(SAY_AGGRO 4)
	// (event 1, the auriaya engage-port precedent); KilledUnit
	// Talk(SAY_KILL 11) victim->GetTypeId() == TYPEID_PLAYER gated
	// (event 3, the razuvious player-gated variant precedent —
	// SEVENTEENTH ported variant, FIFTEENTH identical to
	// nalorakk/kelthuzad/gothik/thaddius/garfrost/krick/tyrannus/falric/marwyn/marrowgar/deathwhisper/saurfang/festergut/rotface);
	// JustDied Talk(SAY_DEATH 13) (event 4, the sjonnir
	// JustDied-Talk precedent). No other bridgeable arm in the
	// file: Talk(SAY_BERSERK 12) / Talk(EMOTE_UNSTABLE_EXPERIMENT 5)
	// / Talk(EMOTE_MALLEABLE_GOO 9) ×2 / Talk(EMOTE_CHOKING_GAS_BOMB
	// 10) / Talk(SAY_FESTERGUT_DEATH 1) / Talk(SAY_ROTFACE_DEATH 3)
	// / Talk(SAY_TRANSFORM_1 7) / Talk(SAY_TRANSFORM_2 8) ride
	// scheduler legs (no-timer-bridge); Talk(SAY_FESTERGUT_GASEOUS_BLIGHT
	// 0) / Talk(SAY_ROTFACE_OOZE_FLOOD 2) /
	// Talk(SAY_PHASE_TRANSITION_HEROIC 6) ride DoAction legs
	// (no-DoAction bridge). npc_volatile_ooze / npc_gas_cloud have
	// zero Talk lines — not registered (the bronjahm
	// npc_corrupted_soul_fragment precedent). The 20 spell scripts
	// join the no-SpellScript-bridge queue and the 5 aura scripts
	// the no-AuraScript-bridge queue — documented in the lua
	// header. Seventh group of the "// Icecrown Citadel" block.
	RegisterLuaBoss("boss_professor_putricide", 36678)

	// Blood Prince Council (Icecrown Citadel) — bridgeable Talk arms
	// ported from boss_blood_prince_council.cpp; see
	// lua_scripts/northrend/boss_blood_prince_council.lua.
	// boss_prince_keleseth_icc (37972, NPC_PRINCE_KELESETH),
	// boss_prince_taldaram_icc (37973, NPC_PRINCE_TALDARAM),
	// boss_prince_valanar_icc (37970, NPC_PRINCE_VALANAR) — the three
	// share the BloodPrincesBossAI base: KilledUnit Talk(kill text)
	// victim->GetTypeId() == TYPEID_PLAYER gated, entry-switched
	// (event 3, the razuvious player-gated variant precedent);
	// JustDied Talk(death text) entry-switched (event 4, the sjonnir
	// JustDied-Talk precedent); timer-driven cast arms port under the
	// retracted no-timer-bridge doctrine (see the lua header): keleseth
	// EVENT_BERSERK (DoCastSelf 50334 + Talk(EMOTE_KELESETH_BERSERK 4),
	// 600000ms one-shot), EVENT_SHADOW_RESONANCE (Talk(
	// SAY_KELESETH_SPECIAL 2) + DoCastSelf 71943, {10000,15000}ms),
	// EVENT_SHADOW_LANCE (DoCastVictim 71405 base variant, 2000ms);
	// taldaram EVENT_BERSERK (DoCastSelf 50334 +
	// Talk(EMOTE_TALDARAM_BERSERK 5), 600000ms one-shot),
	// EVENT_GLITTERING_SPARKS (DoCastVictim 71806, {15000,50000}ms),
	// EVENT_CONJURE_FLAME (DoCastSelf 71718 base variant +
	// Talk(SAY_TALDARAM_SPECIAL 2), {20000,30000}ms); valanar
	// EVENT_BERSERK (DoCastSelf 50334 + Talk(SAY_VALANAR_BERSERK 5),
	// 600000ms one-shot) — timers GUID-keyed via CreateLuaEvent,
	// cancelled on events 2/4/23 (the falric idiom). Out legs:
	// Talk(SelectInvocationSay()) rides ACTION_CAST_INVOCATION
	// (no-DoAction bridge); the controller's EVENT_INVOCATION_OF_BLOOD
	// Talk(textId) is cross-AI Talk on a scheduler leg (no bridge);
	// the _isEmpowered variants (keleseth 71815, taldaram 72040,
	// valanar 72039) ride the unbridged invocation machine; valanar
	// EVENT_KINETIC_BOMB and the base EVENT_SHOCK_VORTEX leg are
	// random-target SelectTarget casts (out); the heroic-gated
	// shadow-prison engage casts are PORTED in boss_blood_prince_council.lua
	// (creature:IsHeroic() bridged; the old "no IsHeroic bridge" claim retracted).
	// boss_blood_council_controller
	// has zero own Talk lines — not registered (the bronjahm
	// npc_corrupted_soul_fragment precedent); npc_blood_queen_lana_thel
	// Talk(SAY_INTRO_1) rides the ACTION_START_INTRO DoAction leg
	// (no-DoAction bridge) and Talk(SAY_INTRO_2) the EVENT_INTRO_1
	// scheduler leg (no-timer-bridge) — not registered;
	// npc_ball_of_flame / npc_kinetic_bomb / npc_dark_nucleus have
	// zero Talk lines — not registered. The 6 spell scripts join the
	// no-SpellScript-bridge queue, the 5 aura scripts the
	// no-AuraScript-bridge queue, and
	// at_blood_prince_council_start_intro the no-AreaTrigger bridge
	// queue (the boss_anubrekhan at_anubrekhan_entrance precedent) —
	// documented in the lua header. Eighth group of the
	// "// Icecrown Citadel" block.
	RegisterLuaBoss("boss_prince_keleseth_icc", 37972)
	RegisterLuaBoss("boss_prince_taldaram_icc", 37973)
	RegisterLuaBoss("boss_prince_valanar_icc", 37970)

	// boss_blood_queen_lana_thel: Icecrown Citadel boss script,
	// entry 37955 (icecrown_citadel.h NPC_BLOOD_QUEEN_LANA_THEL —
	// kalecgos pass; the RegisterIcecrownCitadelCreatureAI
	// ScriptName binding is instance-shimmed, the
	// creature_template binding DB-side). BossAI. Ported arms in
	// lua_scripts/northrend/boss_blood_queen_lana_thel.lua:
	// JustEngagedWith Talk(SAY_AGGRO 0) (event 1, the auriaya
	// engage-port precedent), KilledUnit Talk(SAY_KILL 8)
	// victim->GetTypeId() == TYPEID_PLAYER gated (event 3, the
	// razuvious player-gated variant precedent; identical to
	// nalorakk / kelthuzad / gothik / thaddius / garfrost / krick /
	// tyrannus / falric / marwyn / marrowgar / deathwhisper /
	// saurfang / festergut / rotface / putricide), JustDied Talk(SAY_DEATH 11) (event 4, the sjonnir
	// JustDied-Talk precedent). No other bridgeable arm in the
	// file: Talk(SAY_WIPE 9) rides JustReachedHome with the
	// unbridged instance->SetBossState(FAIL) leg (no event-24 port
	// precedent); Talk(SAY_AIR_PHASE 7) rides the POINT_AIR
	// MovementInform leg (no MovementInform bridge);
	// Talk(EMOTE_BERSERK_RAID 12 / SAY_BERSERK 10) rides
	// EVENT_BERSERK, Talk(SAY_VAMPIRIC_BITE 1) rides
	// EVENT_VAMPIRIC_BITE, Talk(SAY_PACT_OF_THE_DARKFALLEN 6) rides
	// EVENT_PACT_OF_THE_DARKFALLEN,
	// Talk(EMOTE_SWARMING_SHADOWS 5 / SAY_SWARMING_SHADOWS 4)
	// rides EVENT_SWARMING_SHADOWS (no-timer-bridge). The
	// frenzied-bloodthirst AuraScript's cross-AI
	// bloodQueen->AI()->Talk(EMOTE_BLOODTHIRST 3) /
	// Talk(SAY_MIND_CONTROL 2) have no bridge. The 5 spell scripts,
	// the remaining 2 aura scripts, and the 2
	// AchievementCriteriaScripts join the no-SpellScript-bridge /
	// no-AuraScript-bridge / no-achievement-criteria-bridge queues —
	// documented in the lua header. Ninth group of the
	// "// Icecrown Citadel" block.
	RegisterLuaBoss("boss_blood_queen_lana_thel", 37955)

	// boss_sister_svalna: Icecrown Citadel boss script, entry 37126
	// (icecrown_citadel.h NPC_SISTER_SVALNA — kalecgos pass; the
	// RegisterIcecrownCitadelCreatureAI ScriptName binding is
	// instance-shimmed, the creature_template binding DB-side).
	// BossAI. Ported arms in
	// lua_scripts/northrend/boss_sister_svalna.lua: KilledUnit
	// Talk(SAY_SVALNA_KILL 4) victim->GetTypeId() == TYPEID_PLAYER
	// gated plus Talk(SAY_SVALNA_KILL_CAPTAIN 1) when the victim is
	// a creature with a captain entry (37122–37125) (event 3, the
	// razuvious player-gated variant precedent plus
	// victim:GetEntry()), JustDied Talk(SAY_SVALNA_DEATH 6)
	// (event 4, the sjonnir JustDied-Talk precedent). No other
	// bridgeable arm in the file: the JustEngagedWith
	// crok->AI()->Talk(SAY_CROK_COMBAT_SVALNA 5) is cross-AI Talk
	// (no bridge); Talk(SAY_SVALNA_CAPTAIN_DEATH 5) rides the
	// ACTION_CAPTAIN_DIES DoAction leg (no-DoAction bridge);
	// Talk(EMOTE_SVALNA_BROKEN_SHIELD 8) rides the SPELL_HURL_SPEAR
	// SpellHit arm (event 15 never fires); Talk(EMOTE_SVALNA_IMPALE
	// 7) rides the SpellHitTarget leg (no bridge); JustEngagedWith
	// DoCastSelf(SPELL_DIVINE_SURGE 71465, true) + the
	// EVENT_SVALNA_COMBAT Talk(SAY_SVALNA_AGGRO 3) one-shot (9s) port
	// under the CreateLuaEvent doctrine (event 1, the auriaya
	// engage-port precedent); the impaling-spear
	// scheduler leg (random-target, no bridge) and the gauntlet-action
	// scheduler legs stay out (no-DoAction bridge; the gothik
	// phase-machine precedent) — documented in the lua header.
	// npc_impaling_spear (38248) has zero bridgeable arms (NOT
	// registered); the 3 spell scripts join the no-SpellScript-bridge
	// queue and at_icc_start_frostwing_gauntlet the no-AreaTrigger
	// queue — documented in the lua header. Tenth group of the
	// "// Icecrown Citadel" block.
	RegisterLuaBoss("boss_sister_svalna", 37126)

	// npc_captain_arnath / npc_captain_brandon / npc_captain_grondel /
	// npc_captain_rupert: Icecrown Citadel Frostwing Halls gauntlet
	// scripts, entries 37122–37125 (icecrown_citadel.h
	// NPC_CAPTAIN_ARNATH..RUPERT — kalecgos pass; bindings
	// instance-shimmed / DB-side). Shared npc_argent_captainAI
	// (ScriptedAI) base; one lua file for the 4 entries, per the
	// boss_blood_prince_council 3-entries-1-file precedent. Ported
	// arm in lua_scripts/northrend/npc_argent_captains.lua:
	// KilledUnit Talk(SAY_CAPTAIN_KILL 2) victim->GetTypeId() ==
	// TYPEID_PLAYER gated (event 3, the razuvious player-gated
	// variant precedent). The stateful JustDied arms
	// (Talk(SAY_CAPTAIN_DEATH 0) / Talk(SAY_CAPTAIN_SECOND_DEATH 3)
	// on the per-AI _firstDeath flag, reset by the unbridged
	// ACTION_RESET_EVENT DoAction leg) have no per-creature-state
	// port precedent and are documented; Talk(SAY_CAPTAIN_RESURRECTED
	// 1) rides the SPELL_REVIVE_CHAMPION SpellHit arm (event 15
	// never fires). Tenth group of the "// Icecrown Citadel" block.
	RegisterLuaBoss("npc_captain_arnath", 37122)
	RegisterLuaBoss("npc_captain_brandon", 37123)
	RegisterLuaBoss("npc_captain_grondel", 37124)
	RegisterLuaBoss("npc_captain_rupert", 37125)

	// npc_crok_scourgebane: Icecrown Citadel Frostwing Halls gauntlet
	// escort, entry 37129 (icecrown_citadel.h NPC_CROK_SCOURGEBANE —
	// kalecgos pass; binding instance-shimmed / DB-side). Ported arms
	// in lua_scripts/northrend/boss_sister_svalna.lua: the
	// Reset-seeded EVENT_SCOURGE_STRIKE (DoCastVictim 71488,
	// {7500,12500}ms, re-armed {10000,14000}ms) and
	// EVENT_DEATH_STRIKE (DoCastVictim 71489 gated on
	// HealthBelowPct(20), {25000,30000}ms, re-armed {5000,10000}ms)
	// re-keyed to event 1 (the falric idiom). The gauntlet intro /
	// waypoint / DoAction / DamageTaken arms have no bridge —
	// documented in the lua header. Tenth group of the
	// "// Icecrown Citadel" block.
	RegisterLuaBoss("npc_crok_scourgebane", 37129)

	// npc_frostwing_ymirjar_vrykul (one entry per ymirjar type):
	// 37132 battle maiden, 38125 deathbringer, 37127 frostbinder,
	// 37134 huntress, 37133 warlord (icecrown_citadel.h
	// NPC_YMIRJAR_* — kalecgos pass; bindings instance-shimmed /
	// DB-side). Ported arms in
	// lua_scripts/northrend/boss_sister_svalna.lua: the
	// JustEngagedWith-seeded timer casts re-keyed to event 1 —
	// frostbinder DoCastSelf 71270 + frozen orb (DoCastSelf 71274,
	// {5000,10000}ms, re-armed {10000,20000}ms); battle maiden
	// barbaric strike (DoCastVictim 71257, {5000,8000}ms, re-armed
	// {2000,4000}ms) + adrenaline rush (AddAura 71258,
	// {10000,20000}ms); warlord whirlwind (DoCastAOE 41056,
	// {5000,8000}ms, re-armed {6000,12000}ms) + JustDied RemoveAura
	// 71244 (event 4); huntress rapid shot (DoCastVictim 71251,
	// {10000,20000}ms, re-armed {20000,30000}ms) + shoot
	// (DoCastVictim 71253, 1000ms, re-armed 2000ms); deathbringer
	// shadow bolt (DoCastVictim 71296, {6000,12000}ms, re-armed
	// {5000,8000}ms) + death embrace (DoCastVictim 71299,
	// {5000,10000}ms, re-armed {10000,20000}ms) — timers GUID-keyed
	// via CreateLuaEvent, cancelled on events 2/4/23. The
	// random-target / summon / OOC / 25man arms have no bridge —
	// documented in the lua header. Tenth group of the
	// "// Icecrown Citadel" block.
	RegisterLuaBoss("npc_ymirjar_battle_maiden", 37132)
	RegisterLuaBoss("npc_ymirjar_deathbringer", 38125)
	RegisterLuaBoss("npc_ymirjar_frostbinder", 37127)
	RegisterLuaBoss("npc_ymirjar_huntress", 37134)
	RegisterLuaBoss("npc_ymirjar_warlord", 37133)

	// npc_captain_* undead variants: 37491 Arnath, 37493 Brandon,
	// 37494 Grondel, 37495 Rupert (icecrown_citadel.h
	// NPC_CAPTAIN_*_UNDEAD — kalecgos pass; bindings DB-side). C++
	// switches entry via UpdateEntry() on SPELL_REVIVE_CHAMPION hit
	// without replacing the AI; Eluna dispatches by current entry,
	// so the undead entries share the npc_argent_captains.lua
	// handlers (event 1 idempotent; per-cast spell variants resolve
	// via GetEntry()). Tenth group of the "// Icecrown Citadel"
	// block.
	RegisterLuaBoss("npc_captain_arnath_undead", 37491)
	RegisterLuaBoss("npc_captain_brandon_undead", 37493)
	RegisterLuaBoss("npc_captain_grondel_undead", 37494)
	RegisterLuaBoss("npc_captain_rupert_undead", 37495)

	// npc_the_lich_king_controller: Icecrown Citadel script, entry 16980
	// (icecrown_citadel.h NPC_THE_LICH_KING_VALITHRIA — kalecgos pass; the
	// RegisterIcecrownCitadelCreatureAI ScriptName binding is
	// instance-shimmed, the creature_template binding DB-side).
	// ScriptedAI. Ported arms in
	// lua_scripts/northrend/boss_valithria_dreamwalker.lua:
	// JustEngagedWith Talk(SAY_LICH_KING_INTRO 0) (event 1, the auriaya
	// engage-port precedent). No other bridgeable arm in the file:
	// boss_valithria_dreamwalker (36789) exposes zero bridgeable arms
	// (NOT registered) — Talk(SAY_VALITHRIA_SUCCESS 7 /
	// SAY_VALITHRIA_75_PERCENT 2) ride HealReceived legs (no
	// HealReceived bridge); Talk(SAY_VALITHRIA_25_PERCENT 3 /
	// SAY_VALITHRIA_DEATH 4) ride DamageTaken legs (no DamageTaken
	// bridge — she never actually dies, damage is negated); Talk
	// (SAY_VALITHRIA_ENTER_COMBAT 0 / SAY_VALITHRIA_BERSERK 6 /
	// SAY_VALITHRIA_DREAM_PORTAL 1) ride scheduler legs (no-timer-bridge;
	// the EVENT_INTRO_TALK leg is scheduled from DoAction
	// ACTION_ENTER_COMBAT — no-DoAction bridge);
	// SAY_VALITHRIA_PLAYER_DEATH 5 is declared but never referenced by a
	// Talk() call (dead text evidence); InitializeAI's _spawnHealth arm
	// (curhealth via GetCreatureData(spawnId)) rides the unbridged
	// InitializeAI + DB legs; the lich king controller's Reset
	// scheduler / JustSummoned / UpdateAI summoner machine legs have no
	// bridges; npc_green_dragon_combat_trigger / npc_risen_archmage /
	// npc_blazing_skeleton / npc_suppresser / npc_blistering_zombie /
	// npc_gluttonous_abomination / npc_dream_portal / npc_dream_cloud
	// have zero Talk lines (NOT registered); the 9 spell scripts join
	// the no-SpellScript-bridge queue and achievement_portal_jockey the
	// no-achievement-criteria queue — documented in the lua header.
	// Eleventh group of the "// Icecrown Citadel" block.
	RegisterLuaBoss("npc_the_lich_king_controller", 16980)

	// boss_sindragosa: Icecrown Citadel script, entry 36853
	// (icecrown_citadel.h NPC_SINDRAGOSA — the RegisterIcecrownCitadelCreatureAI
	// ScriptName binding is instance-shimmed, the creature_template binding
	// DB-side). BossAI. Ported arms in
	// lua_scripts/northrend/boss_sindragosa.lua:
	// JustEngagedWith Talk(SAY_AGGRO 0) (event 1, the auriaya
	// engage-port precedent; the instance CheckRequiredBosses gate /
	// evade / DoCastSpellOnPlayers LIGHT_S_HAMMER_TELEPORT / DoCastSelf
	// aura / SetBossState / setActive / SetFarVisible / DoZoneInCombat
	// legs have no bridges); KilledUnit Talk(SAY_KILL 8) gated on
	// victim->GetTypeId() == TYPEID_PLAYER (event 3, the razuvious
	// player-gated variant precedent); JustDied Talk(SAY_DEATH 10)
	// (event 4, the sjonnir JustDied-Talk precedent; the heroic
	// FROST_INFUSION_CREDIT DoCastAOE leg has no-cast bridge). No other
	// bridgeable arm in the file: the scheduler Talk legs
	// (EMOTE_BERSERK_RAID 11 / SAY_BERSERK 9 / SAY_UNCHAINED_MAGIC 1 /
	// EMOTE_WARN_BLISTERING_COLD 2 / SAY_BLISTERING_COLD 3 /
	// SAY_AIR_PHASE 5 / SAY_PHASE_2 6) have no-timer-bridge (the
	// boss_toravon precedent); SAY_RESPITE_FOR_A_TORMENTED_SOUL 4 is
	// declared but never referenced by a Talk() call (dead text
	// evidence); the DoAction ACTION_START_FROSTWYRM intro machine has
	// no-DoAction bridge; JustReachedHome SetBossState FAIL has no
	// instance bridge (no event-24 port precedent);
	// spell_sindragosa_ice_tomb_target HandleSindragosaTalk is cross-AI
	// Talk(EMOTE_WARN_FROZEN_ORB 7) (no cross-AI-Talk bridge);
	// npc_ice_tomb (36980) / npc_spinestalker (37534) /
	// npc_rimefang_icc (37533) / npc_sindragosa_trash (37531/37532)
	// have zero Talk lines (NOT registered); at_sindragosa_lair joins
	// the no-AreaTrigger bridge queue, achievement_all_you_can_eat the
	// no-achievement-criteria queue, and the spell/aura scripts the
	// no-SpellScript-bridge / no-AuraScript-bridge queues — documented
	// in the lua header. Twelfth group of the "// Icecrown Citadel"
	// block.
	RegisterLuaBoss("boss_sindragosa", 36853)
}

func init() {
	// The Lich King (36597) — Lua port of
	// src/server/scripts/Northrend/IcecrownCitadel/boss_the_lich_king.cpp
	// (2899 lines; 10 CreatureScripts (boss_the_lich_king (BossAI,
	// DATA_THE_LICH_KING = 12) + npc_tirion_fordring_tft +
	// npc_shambling_horror_icc + npc_raging_spirit +
	// npc_valkyr_shadowguard + npc_strangulate_vehicle +
	// npc_terenas_menethil + npc_spirit_warden + npc_spirit_bomb +
	// npc_broken_frostmourne) + 31 SpellScript/AuraScript legs + 1
	// generic spell_trigger_spell_from_caster with-args leg +
	// achievement_been_waiting_long_time / achievement_neck_deep_in_vile;
	// all registered from inside AddSC_boss_the_lich_king(); loader decl
	// 183 / call 378 per northrend_script_loader.cpp — the THIRTEENTH
	// group of the "// Icecrown Citadel" block. Entry-verifiable:
	// icecrown_citadel.h NPC_THE_LICH_KING = 36597 (line 314),
	// instance_icecrown_citadel.cpp OnCreatureCreate binds case
	// NPC_THE_LICH_KING (line 314); RegisterIcecrownCitadelCreatureAI
	// ScriptName bindings are DB-side as usual.
	// Ported arms in
	// lua_scripts/northrend/boss_the_lich_king.lua:
	// boss_the_lich_king KilledUnit Talk(SAY_LK_KILL 10) gated on
	// victim->GetTypeId() == TYPEID_PLAYER (event 3, the razuvious
	// player-gated variant precedent; the !me->IsInEvadeMode() and
	// !events.IsInPhase(PHASE_OUTRO) legs have no bridges). No other
	// bridgeable arm in the file: DoAction Talk(SAY_LK_INTRO_1 0 /
	// SAY_LK_FROSTMOURNE_ESCAPE 8) has no-DoAction bridge; SpellHit
	// Talk(SAY_LK_FROSTMOURNE_KILL 9) is event 15 (never fires);
	// MovementInform Talk(SAY_LK_REMORSELESS_WINTER 4 ×2) and the
	// cross-AI tirion->AI()->Talk(SAY_TIRION_OUTRO_2) have no
	// MovementInform / cross-AI-Talk bridges; all scheduler Talk legs
	// (SAY_LK_INTRO_2 1 / SAY_LK_INTRO_3 2 / EMOTE_NECROTIC_PLAGUE_WARNING
	// 13 / EMOTE_DEFILE_WARNING 12 / SAY_LK_HARVEST_SOUL 7 ×2 /
	// SAY_LK_QUAKE 5 ×2 / SAY_LK_SUMMON_VALKYR 6 / SAY_LK_OUTRO_1..8
	// 14..21 / SAY_LK_BERSERK 11) have no-timer-bridge (the boss_toravon
	// precedent); npc_tirion_fordring_tft (38995) Talk legs ride DoAction
	// / scheduler (no bridges); npc_terenas_menethil (36823/39217) Talk
	// legs are scheduler-driven (no bridges); the 7 remaining NPC AIs
	// have zero Talk lines (NOT registered, the bronjahm
	// npc_corrupted_soul_fragment precedent); the 31 spell/aura legs +
	// the with-args trigger leg join the no-SpellScript-bridge /
	// no-AuraScript-bridge queues and the 2 achievements the
	// no-achievement-criteria-bridge queue — documented in the lua
	// header. Thirteenth group of the "// Icecrown Citadel" block.
	RegisterLuaBoss("boss_the_lich_king", 36597)

	// npc_rotting_frost_giant (Icecrown Citadel instance script):
	// Reset() three-timer seed + UpdateAI combat rotation — death
	// plague (selector: player, <= 100.0f, non-victim, no 72865 /
	// 72884 aura; Talk EMOTE 0 + cast 72865; 15s re-arm), stomp
	// 64652 DoCastVictim (5-8s engage / 15-18s re-arm), arctic
	// breath 72848 DoCastVictim (10-15s engage / 26-33s re-arm);
	// JustDied instance-player aura removal 72865 via
	// map/instance-filtered RemoveAura. Entries 38490/38494
	// verifiable from icecrown_citadel.h:158-159 — see
	// lua_scripts/northrend/npc_rotting_frost_giant.lua.
	// Fourteenth group of the "// Icecrown Citadel" block.
	RegisterLuaBoss("npc_rotting_frost_giant", 38490)
	RegisterLuaBoss("npc_rotting_frost_giant", 38494)

	// npc_darkfallen_icc (Icecrown Citadel instance script):
	// DarkFallenAI base + 5 subclass ScheduleSpells — blood knight
	// (37595): vampiric aura 71736 one-shot, unholy strike 70437
	// victim / blood mirror 70450 self-cast; noble (37663): chains of
	// shadow 72960 random non-aura target one-shot (fiend summon PORTED
	// via the SummonCreature bridge, npc_darkfallen_icc.lua noble arm);
	// archmage (37664): amplify magic 70408 / blast
	// wave 70407 self / polymorph 70410 (noble+archmage
	// DoSpellAttackIfReady filler unmodeled, melee disabled
	// C++-exact); advisor (37571): lich slap 72057 victim / shroud
	// 72066 lowest-HP darkfallen ally in 40y; tactician (37666):
	// unholy strike victim / shadowstep 70431 + blood sap 70432
	// random target; vampiric fiend (37901): disease cloud 41290
	// self, leeching root 70671 victim one-shot, 38s self-despawn;
	// out-of-combat emote machine (polymorph-ally 72106 + random
	// DarkFallensEmotes) on spawn/evade.
	// Entries verifiable from icecrown_citadel.h:257-262 — see
	// lua_scripts/northrend/npc_darkfallen_icc.lua.
	// Fifteenth group of the "// Icecrown Citadel" block.
	RegisterLuaBoss("npc_darkfallen_icc", 37595)
	RegisterLuaBoss("npc_darkfallen_icc", 37663)
	RegisterLuaBoss("npc_darkfallen_icc", 37664)
	RegisterLuaBoss("npc_darkfallen_icc", 37571)
	RegisterLuaBoss("npc_darkfallen_icc", 37666)
	RegisterLuaBoss("npc_darkfallen_icc", 37901)
}

func init() {
	// Baltharus the Warborn (39751) — Lua port of
	// src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/boss_baltharus_the_warborn.cpp
	// (368 lines; 2 CreatureScripts (boss_baltharus_the_warborn (BossAI,
	// DATA_BALTHARUS_THE_WARBORN = 1) + npc_baltharus_the_warborn_clone
	// (BossAI, DATA_BALTHARUS_CLONE = 3)); all registered from inside
	// AddSC_boss_baltharus_the_warborn(); loader decl 190 / call 385 per
	// northrend_script_loader.cpp — the THIRD group of the
	// "// Ruby Sanctum" block. Entry-verifiable: ruby_sanctum.h
	// NPC_BALTHARUS_THE_WARBORN = 39751 (line 67),
	// instance_ruby_sanctum.cpp creatureData binds NPC_BALTHARUS_THE_WARBORN
	// (line 49); ScriptName bindings are DB-side as usual.
	// Ported arms in
	// lua_scripts/northrend/boss_baltharus_the_warborn.lua:
	// boss_baltharus_the_warborn JustEngagedWith Talk(SAY_AGGRO 1)
	// (event 1, the auriaya precedent); JustDied Talk(SAY_DEATH 4)
	// (event 4, the sjonnir precedent); KilledUnit Talk(SAY_KILL 2) gated
	// on victim->GetTypeId() == TYPEID_PLAYER (event 3, the razuvious
	// player-gated variant precedent). No other bridgeable arm in the
	// file: DoAction Talk(SAY_CLONE 3) has no-DoAction bridge; the
	// EVENT_INTRO_TALK Talk(SAY_BALTHARUS_INTRO 0) has no-timer-bridge;
	// the JustDied _JustDied() instance leg and the cross-AI
	// xerestrasza->AI()->DoAction(ACTION_BALTHARUS_DEATH) leg have no
	// bridges; npc_baltharus_the_warborn_clone (39899) has zero Talk
	// lines — registered via RegisterCreatureEvent in the lua file (entry-based dispatch). Third group
	// of the "// Ruby Sanctum" block.
	RegisterLuaBoss("boss_baltharus_the_warborn", 39751)
}

func init() {
	// Saviana Ragefire (39747) — Lua port of
	// src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/boss_saviana_ragefire.cpp
	// (1 CreatureScript (boss_saviana_ragefire (BossAI, DATA_SAVIANA_RAGEFIRE = 2))
	// + 2 SpellScriptLoaders (spell_saviana_conflagration_init 74452 /
	// spell_saviana_conflagration_throwback 74455)); all registered from inside
	// AddSC_boss_saviana_ragefire(); loader decl 191 / call 386 per
	// northrend_script_loader.cpp — the FOURTH group of the
	// "// Ruby Sanctum" block. Entry-verifiable: ruby_sanctum.h
	// NPC_SAVIANA_RAGEFIRE = 39747 (line 77), DATA_SAVIANA_RAGEFIRE = 2
	// (line 33); instance_ruby_sanctum.cpp creatureData binds
	// NPC_SAVIANA_RAGEFIRE -> DATA_SAVIANA_RAGEFIRE (line 53); ScriptName
	// bindings are DB-side as usual.
	// Ported arms in lua_scripts/northrend/boss_saviana_ragefire.lua:
	// boss_saviana_ragefire JustEngagedWith Talk(SAY_AGGRO 0) (event 1,
	// the auriaya precedent); KilledUnit Talk(SAY_KILL 3) gated on
	// victim->GetTypeId() == TYPEID_PLAYER (event 3, the razuvious
	// player-gated variant precedent). No other bridgeable arm in the
	// file: JustDied _JustDied() + DoPlaySoundToSet(SOUND_ID_DEATH 17531)
	// (no instance-script bridge; sound arm ported via creature:PlayDirectSound, zero Talk); MovementInform
	// Talk(SAY_CONFLAGRATION 1) rides POINT_FLIGHT (no-movement-inform
	// bridge); UpdateAI Talk(EMOTE_ENRAGED 2) rides EVENT_ENRAGE
	// (no-timer-bridge); the two SpellScripts (no-SpellScript bridge);
	// the EnterEvadeMode _DespawnAtEvade() leg — all documented in the
	// lua header. Fourth group of the "// Ruby Sanctum" block.
	RegisterLuaBoss("boss_saviana_ragefire", 39747)
}

func init() {
	// General Zarithrian (39746) — Lua port of
	// src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/boss_general_zarithrian.cpp
	// (2 CreatureScripts: boss_general_zarithrian (BossAI, DATA_GENERAL_ZARITHRIAN = 1)
	// + npc_onyx_flamecaller (ScriptedAI, NPC_ONYX_FLAMECALLER = 39814, zero Talk —
	// not registered, the bronjahm precedent)); both registered from inside
	// AddSC_boss_general_zarithrian(); loader decl 192 / call 387 per
	// northrend_script_loader.cpp — the FIFTH group of the
	// "// Ruby Sanctum" block. Entry-verifiable: ruby_sanctum.h
	// NPC_GENERAL_ZARITHRIAN = 39746 (line 72), DATA_GENERAL_ZARITHRIAN = 1
	// (line 32); instance_ruby_sanctum.cpp creatureData binds
	// NPC_GENERAL_ZARITHRIAN -> DATA_GENERAL_ZARITHRIAN (line 52); ScriptName
	// bindings are DB-side as usual.
	// Ported arms in lua_scripts/northrend/boss_general_zarithrian.lua:
	// boss_general_zarithrian JustEngagedWith Talk(SAY_AGGRO 0) (event 1,
	// the auriaya precedent); KilledUnit Talk(SAY_KILL 1) gated on
	// victim->GetTypeId() == TYPEID_PLAYER (event 3, the razuvious
	// player-gated variant precedent); JustDied Talk(SAY_DEATH 3)
	// (event 4, the sjonnir precedent). No other bridgeable arm in the
	// file: UpdateAI Talk(SAY_ADDS 2) rides EVENT_SUMMON_ADDS
	// (no-timer-bridge); the Reset/CanAIAttack instance-state legs (no
	// instance-script bridge); the JustSummoned add-registration and
	// EnterEvadeMode summons.DespawnAll()/_DespawnAtEvade() legs (no
	// summon-registry / evade-model bridges); the DoCastSelf/DoCastVictim
	// legs (no-cast bridge); npc_onyx_flamecaller zero Talk (not
	// registered) — all documented in the lua header. Fifth group of
	// the "// Ruby Sanctum" block.
	RegisterLuaBoss("boss_general_zarithrian", 39746)
}

func init() {
	// Halion (39863) + Twilight Halion (40142) — Lua port of
	// src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/boss_halion.cpp
	// (10 CreatureScripts: boss_halion (BossAI, DATA_HALION) +
	// boss_twilight_halion (BossAI, DATA_TWILIGHT_HALION) +
	// npc_halion_controller (ScriptedAI, NPC_HALION_CONTROLLER = 40146, zero
	// Talk on its own unit — not registered, the bronjahm precedent) +
	// npc_meteor_strike_flame / npc_meteor_strike_initial / npc_meteor_strike /
	// npc_combustion_consumption / npc_orb_carrier / npc_living_inferno /
	// npc_living_ember (ScriptedAI, zero Talk — not registered) +
	// go_twilight_portal (GameObjectAI) + 17 SpellScriptLoaders
	// (meteor_strike_marker, combustion_consumption x2, marks x2,
	// combustion_consumption_periodic, damage_aoe_summon,
	// twilight_realm_handlers x2, summon_exit_portals, twilight_phasing,
	// twilight_cutter, clear_debuffs, spawn_living_embers, blazing_aura —
	// no-SpellScript bridge)); all registered from inside AddSC_boss_halion()
	// (line 1906); loader decl 193 / call 388 per northrend_script_loader.cpp —
	// the SIXTH and FINAL group of the "// Ruby Sanctum" block, closing it.
	// Entry-verifiable: ruby_sanctum.h NPC_HALION = 39863 (line 80),
	// NPC_TWILIGHT_HALION = 40142 (line 81); instance_ruby_sanctum.cpp
	// creatureData binds NPC_HALION -> DATA_HALION (line 54) and
	// NPC_TWILIGHT_HALION -> DATA_TWILIGHT_HALION (line 55); ScriptName
	// bindings are DB-side as usual.
	// Ported arms in lua_scripts/northrend/boss_halion.lua:
	// boss_halion JustEngagedWith Talk(SAY_AGGRO 2) (event 1, the auriaya
	// precedent); boss_halion JustDied Talk(SAY_DEATH 5) (event 4, the
	// sjonnir precedent); boss_twilight_halion KilledUnit Talk(SAY_KILL 6)
	// gated on victim->GetTypeId() == TYPEID_PLAYER (event 3, the razuvious
	// player-gated variant precedent — material halion has no KilledUnit arm).
	// No other bridgeable arm in the file: DamageTaken Talk(SAY_PHASE_TWO 4 /
	// SAY_PHASE_THREE 2) has no-DamageTaken bridge; SpellHit Talk(SAY_REGENERATE 0)
	// rides the SpellHit-15-never-fires queue; UpdateAI Talk(SAY_METEOR_STRIKE 3)
	// rides EVENT_METEOR_STRIKE (no-timer-bridge); the controller's cross-AI
	// Talk(SAY_INTRO 1 / EMOTE_CORPOREALITY_TOT 4 / TIT 3 / POT 8 / PIP 9)
	// and the orb machine's Talk(EMOTE_WARN_LASER 0 / SAY_SPHERE_PULSE 1)
	// have no cross-AI-Talk bridge; SAY_BERSERK 7 is dead text. Sixth and
	// final group of the "// Ruby Sanctum" block — Ruby Sanctum CLOSED (6/6).
	RegisterLuaBoss("boss_halion", 39863)
	RegisterLuaBoss("boss_twilight_halion", 40142)
}

func init() {
	// Emerald Dragons (14887 Ysondre / 14888 Lethon / 14889 Emeriss / 14890
	// Taerar) — Lua port of
	// src/server/scripts/World/boss_emerald_dragons.cpp
	// (4 CreatureScripts: boss_ysondre / boss_lethon / boss_emeriss /
	// boss_taerar (WorldBossAI over the shared emerald_dragonAI base) +
	// npc_dream_fog (ScriptedAI, zero Talk — not registered) +
	// npc_spirit_shade (PassiveAI, zero Talk — not registered) +
	// 2 SpellScriptLoaders (spell_dream_fog_sleep, spell_mark_of_nature —
	// no-SpellScript bridge — not registered)); all registered from inside
	// AddSC_emerald_dragons() (line 808); loader decl 23 / call 44 per
	// world_script_loader.cpp — the group immediately after
	// AddSC_areatrigger_scripts() (call 43) in AddWorldScripts().
	// Entry-verifiable: the file's own EmeraldDragonNPC enum
	// (DRAGON_YSONDRE 14887 / DRAGON_LETHON 14888 / DRAGON_EMERISS 14889 /
	// DRAGON_TAERAR 14890); ScriptName bindings are DB-side as usual.
	// Ported arms in lua_scripts/world/boss_emerald_dragons.lua (new world/
	// dir mirroring the C++ src/server/scripts/World layout): the four
	// JustEngagedWith Talk(SAY_*_AGGRO 0) arms (event 1, the auriaya
	// precedent; the WorldBossAI legs have no bridges) + their ExecuteEvent
	// timer arms via CreateLuaEvent {min,max} tables (cancel on 2/4/23) —
	// lightning wave 24819 {10s,20s} / shadow bolt whirl 24834 {15s,30s} /
	// volatile infection 24928 120s / arcane blast 24857 {7s,12s} /
	// bellowing roar 22686 {20s,30s}; the ysondre / lethon / emeriss
	// DamageTaken arms (event 9, pre-damage health, floor-form 75/50/25%
	// thresholds) — Talk(SAY_*_SUMMON 1) + 10x 24795 triggered / 24811 /
	// 24910 triggered + ++_stage.
	// Documented-only: taerar DamageTaken (event 9 bridges; the banish
	// machine rides no-flag / no-react-state / no-summon-death bridges);
	// emerald_dragonAI / emeriss KilledUnit player-gated victim-self-casts
	// (25040 / 24904, no Go spell effect); the Reset self-cast 25041 +
	// RemoveFlag / SetReactState legs; the UpdateAI SPELL_SUMMON_PLAYER
	// 24776 max-threat pull; npc_dream_fog + npc_spirit_shade (no-movement /
	// no-despawn bridges — not registered); the SpellHitTarget shade-summon
	// machine; the two SpellScriptLoaders (no-SpellScript bridge).
	RegisterLuaBoss("boss_ysondre", 14887)
	RegisterLuaBoss("boss_lethon", 14888)
	RegisterLuaBoss("boss_emeriss", 14889)
	RegisterLuaBoss("boss_taerar", 14890)
	// npc_guard_generic (World guard script): the JustEngagedWith arm —
	// Talk(SAY_GUARD_SIL_AGGRO 0, who) gated in C++ on
	// me->GetEntry() == NPC_CENARION_HOLD_INFANTRY (15184) — the
	// event-1 auriaya precedent; the player target flattens through the
	// moroes precedent (Lua Talk takes no target); the GuardAI
	// _combatScheduler melee/spell legs are SelectSpell machines (spell IDs
	// unnamed in C++ — no bridge). Entry-verifiable: 15184 from the file's
	// own GuardMisc enum; ScriptName bindings are DB-side as usual — see
	// lua_scripts/world/npc_guard.lua. Ported too: the
	// npc_guard_shattrath_faction ScheduleVanish banish machine (entry
	// 18549, zero Talk — 5s victim-player gate -> 36671/36642
	// non-triggered + 9s 39533/36643 triggered chain, unconditional
	// re-arm; the exile/teleport legs deviate: player:CastSpell has no
	// bridge, so the creature casts them on the player instead; cancel on
	// 2/4/23) and the Stormwind/Orgrimmar ReceiveEmote reply legs
	// (entries 68/1976/3296, ev8 handler in npc_guard.lua, IsFriendlyTo
	// bridge). Documented-only: the Reset 10-minute SelectSpell buff
	// machine.
	RegisterLuaBoss("npc_guard_generic", 15184)
	// boss_nexusprince_shaffar (Mana Tombs, Outland dungeon pass): the
	// bridgeable Talk arms — JustEngagedWith Talk(SAY_AGGRO 1) (event 1,
	// the auriaya precedent), KilledUnit Talk(SAY_SLAY 2) player-gated
	// (event 3, the razuvious precedent), JustDied Talk(SAY_DEAD 4)
	// (event 4, the sjonnir precedent). Entry 18344 NPC_SHAFFAR from the
	// file's own Creatures enum; ScriptName bindings are DB-side as usual
	// — see lua_scripts/outland/boss_nexusprince_shaffar.lua. The
	// one-shot 100-unit MoveInLineOfSight SAY_INTRO Talk, the Reset
	// three-beacon summon legs, the JustSummoned beacon-visual /
	// random-target AttackStart legs, and the ExecuteEvent machine
	// (urand-gated SAY_SUMMON, frostbolt/frostnova casts; blink stays
	// out on no motion/interrupt bridge) are ported there. The beacon
	// AI (18431 arcane-bolt 1s/2-4.5s + apprentice timer, beacon
	// JustSummoned victim engage) and the apprentice AI (18430 3s
	// fire/frost ping-pong) are ported in the same file. npc_yor is
	// zero-Talk and NOT registered: its entry is DB-side only (no
	// Creatures entry constant in the C++ file) and is not guessed.
	RegisterLuaBoss("boss_nexusprince_shaffar", 18344)
	RegisterLuaBoss("npc_ethereal_beacon", 18431)
	RegisterLuaBoss("npc_ethereal_apprentice", 18430)
	// boss_ambassador_hellmaw (Shadow Labyrinth, Outland dungeon
	// pass): the bridgeable Talk arms — JustEngagedWith Talk(SAY_AGGRO
	// 1) (event 1, the auriaya precedent; the SetBossState IN_PROGRESS
	// leg rides the no-instance bridge), KilledUnit Talk(SAY_SLAY 3)
	// player-gated (event 3, the razuvious precedent), JustDied
	// Talk(SAY_DEATH 4) (event 4, the sjonnir precedent; the
	// SetBossState DONE leg rides the no-instance bridge). Entry
	// 18731 NPC_AMBASSADOR_HELLMAW from shadow_labyrinth.h (the
	// .cpp file itself carries zero NPC_ constants — the NPC_ANZU-in-
	// sethekk_halls.h precedent); ScriptName bindings are DB-side as
	// usual — see
	// lua_scripts/outland/boss_ambassador_hellmaw.lua. The
	// DoAction-driven DoIntro Talk(SAY_INTRO 0) is called from
	// instance_shadow_labyrinth.cpp:137 (no InstanceScript dispatch /
	// no-escort bridges — documented-only); the Reset ScheduleEvent
	// legs + DoAction BANISH leg ride no-timer / no-instance /
	// no-cast bridges; the UpdateEscortAI machine (corrosive acid /
	// fear / heroic-only berserk casts, banish evade) rides no-timer
	// / no-cast / no-random-target / no-heroic-mode bridges —
	// documented-only. SAY_HELP = 2 is unused in the file.
	RegisterLuaBoss("boss_ambassador_hellmaw", 18731)
	// boss_blackheart_the_inciter (Shadow Labyrinth, Outland dungeon
	// pass): the bridgeable Talk arms — JustEngagedWith Talk(SAY_AGGRO
	// 1) (event 1, the auriaya precedent; the ScheduleEvent legs ride
	// the no-timer bridge), KilledUnit Talk(SAY_SLAY 2) player-gated
	// (event 3, the razuvious precedent), JustDied Talk(SAY_DEATH 4)
	// (event 4, the sjonnir precedent; the _JustDied() instance leg
	// rides the no-instance bridge). Entry 18667 NPC_BLACKHEART from
	// shadow_labyrinth.h (line 44; the .cpp file itself carries zero
	// NPC_ constants — the ambassador_hellmaw precedent); ScriptName
	// bindings are DB-side as usual — see
	// lua_scripts/outland/boss_blackheart_the_inciter.lua. The
	// charm-count SetData machine, the whole UpdateAI timer machine
	// (incite-chaos threat reset / charge / war-stomp casts), the
	// mc_dummy NullCreatureAI (NPC_BLACKHEART_DUMMY1..5 = 19300-19304
	// — zero Talk arms), BlackheartCharmedPlayerAI, and the
	// spell_blackheart_incite_chaos SpellScript ride no-timer /
	// no-cast / no-random-target / no-summon / no-threat /
	// no-charmed-player / no-instance / no-SpellScript bridges —
	// documented-only. SAY_INTRO 0 / SAY_HELP 3 / SAY2_* 5-9 are
	// never called in the file.
	RegisterLuaBoss("boss_blackheart_the_inciter", 18667)
	// boss_grandmaster_vorpil.lua — ported bridgeable arms: the
	// SayHelp one-shot riding the EVENT_SUMMON_TRAVELER timer
	// machine, the void-portal + void-traveler SummonCreature legs,
	// the MoveInLineOfSight SAY_INTRO one-shot (event-27 fire-site
	// mismatch), the whole UpdateAI timer machine (shadowbolt
	// volley / heroic-only banish / draw-shadows player teleports /
	// traveler summon + 20%-health enrage), and the zero-Talk
	// npc_voidtraveler follow/sacrifice machine (NPC_VOID_TRAVELER
	// 19226 NOT registered — shaffar-beacon precedent) ride
	// no-timer / no-cast / no-random-target / no-summon /
	// no-motion / no-instance bridges — documented-only.
	RegisterLuaBoss("boss_grandmaster_vorpil", 18732)
	// boss_darkweaver_syth (Shadow Labyrinth, Outland dungeon pass) —
	// fight logic in lua_scripts/outland/boss_darkweaver_syth.lua: the
	// event-1 flame-shock / chain-lightning / frost-shock / shadow-bolt
	// volley timers, the 50%/10% health one-shot latches (event 9), the
	// Talk arms (SAY_AGGRO 1 event 1, SAY_SLAY 2 event 3 player-gated —
	// the razuvious precedent, SAY_DEATH event 4). The summon-elemental
	// / summon-clone machine legs ride no-summon / no-cast bridges —
	// documented-only. Entry 18968 is DB-side (no NPC_ constant in the
	// C++ tree — the syth enum in boss_darkweaver_syth.cpp carries
	// spell/talk IDs only); ScriptName bindings are DB-side as usual.
	RegisterLuaBoss("boss_darkweaver_syth", 18968)
	// boss_murmur (Shadow Labyrinth, Outland dungeon pass) — fight
	// logic in lua_scripts/outland/boss_murmur.lua: the 40%-health
	// touch-of-murmur one-shot latch (event 23), the sonic-boom /
	// shockwave timer arms, the Talk arms (event 1/3/4). The
	// resonance / magnetic-pull legs ride no-cast / no-motion bridges
	// — documented-only. Entry 18708 is DB-side (no NPC_ constant in
	// the C++ tree); ScriptName bindings are DB-side as usual.
	RegisterLuaBoss("boss_murmur", 18708)
	// boss_pandemonius (Mana-Tombs, Outland dungeon pass) — fight
	// logic in lua_scripts/outland/boss_pandemonius.lua: the event-1
	// void-blast / dark-shell timer arms, the KilledUnit Talk(SAY_KILL
	// 1) (event 3, the razuvious player-gated precedent), the engage
	// Talk arm. The InterruptNonMeleeSpells(true) dark-shell leg is bridged (no-op twin)
	// and ported. Entry 18341 is DB-side (no NPC_ constant in
	// the C++ tree); ScriptName bindings are DB-side as usual.
	RegisterLuaBoss("boss_pandemonius", 18341)
	// boss_anzu (Sethekk Halls, Outland dungeon pass) — fight logic in
	// lua_scripts/outland/boss_anzu.lua: event-1 paralyzing-screech /
	// cyclone-of-feathers timers, the event-9 health-latch arms, the
	// Talk arms (events 1/3/4). The BossAI::JustEngagedWith instance
	// leg (SetBossState IN_PROGRESS) rides the no-instance bridge —
	// documented-only. Entry 23035 is DB-side (no NPC_ constant in the
	// C++ tree); ScriptName bindings are DB-side as usual.
	RegisterLuaBoss("boss_anzu", 23035)
	// boss_talon_king_ikiss (Sethekk Halls, Outland dungeon pass) —
	// fight logic in lua_scripts/outland/boss_talon_king_ikiss.lua:
	// the event-1 blink / arcane-volley / slow timer arms, the 20%-
	// health one-shot latch (event 9), the intro latch + Talk(SAY_INTRO
	// 0) (event 27), the Talk arms (events 1/3/4). The heroic-only
	// slow leg rides the no-heroic bridge — documented-only. Entry
	// 18473 is DB-side (no NPC_ constant in the C++ tree); ScriptName
	// bindings are DB-side as usual.
	RegisterLuaBoss("boss_talon_king_ikiss", 18473)
	// boss_exarch_maladaar + boss_shirrak_the_dead_watcher (Auchenai
	// Crypts, Outland dungeon pass): both ScriptedAI-derived
	// (maladaar.cpp:169, shirrak.cpp:63), registered per the outland
	// dungeon-boss convention (vorpil/syth/murmur precedent); Auchenai
	// Crypts is not a raid map, so the slot's admission effect stays inert
	// (beginInstanceEncounter IsRaid gate) and all luaBossAI hooks are no-ops.
	RegisterLuaBoss("boss_exarch_maladaar", 18373)
	RegisterLuaBoss("boss_shirrak_the_dead_watcher", 18371)
	// boss_illidan.lua — ported bridgeable arm: boss_illidan_stormrageAI::KilledUnit
	// Talk(SAY_ILLIDAN_KILL 1) gated on victim->GetTypeId() == TYPEID_PLAYER —
	// the razuvious player-gated precedent (event 3). The rest — intro/combat/
	// air-phase/demon-form/phase-4/outro timer Talks (ENRAGE/DUPLICITY/UNCONVINCED/
	// PREPARED/EYE_BLAST/MORPH/SHADOW_PRISON/CONFRONT_MAIEV/FRENZY/TAUNT/DEFEATED),
	// the DoAction SAY_ILLIDAN_MINION Talk (no-action bridge), the no-Talk
	// JustEngagedWith/JustDied (no-instance bridge), the phase-transition
	// DamageTaken legs, the npc_akama_illidan gossip/MovementInform/timer Talks
	// (no-gossip / no-motion / no-timer bridges — NOT registered), the zero-Talk
	// companion AIs (parasitic_shadowfiend / blade_of_azzinoth / flame_of_azzinoth /
	// illidan_db_target / shadow_demon / cage_trap_trigger / illidari_elite /
	// illidan_generic_fire — NOT registered), the npc_maiev DamageTaken/DoAction/
	// timer Talks (damage-gated / no-action / no-timer — NOT registered), and the
	// 21 SpellScriptLoaders (no-SpellScript / no-AuraScript bridges) — documented-only.
	RegisterLuaBoss("boss_illidan", 22917)
	// npc_imprisoned_beryl_sorcerer (Borean Tundra, quest 11648) —
	// ported arms in lua_scripts/northrend/npc_imprisoned_beryl_sorcerer.lua:
	// UpdateAI rebuff timer (immediate + 180s, !HasAura(45631) gate,
	// non-triggered self-cast 45631) and the SpellHit impact arm
	// (event 14: 45634->45702 / 48252->48254 non-triggered self-casts,
	// player-caster gated) plus the GotStinged remainder (quest-11648
	// gate, per-player sting-count step twin of the caster GetAuraCount
	// read, neural step-4 kill credit, Talk(step, caster)). Entry 25478 verifiable from the C++
	// ImprisionedBerylSorcerer enum.
	RegisterLuaBoss("npc_imprisoned_beryl_sorcerer", 25478)
	// npc_hidden_cultist (Borean Tundra, quest 11794) — ported arm in
	// lua_scripts/northrend/npc_hidden_cultist.lua: Reset()
	// DoCast(SPELL_SHROUD_OF_THE_DEATH_CULTIST 46077) non-triggered
	// self-cast (events 5/23). Entries 25827/25248/25828 verifiable
	// from the C++ HiddenCultist enum; all three share the AI.
	RegisterLuaBoss("npc_hidden_cultist", 25827)
	RegisterLuaBoss("npc_hidden_cultist", 25248)
	RegisterLuaBoss("npc_hidden_cultist", 25828)
	// npc_counselor_talbot (Borean Tundra, Last Rites quest chain) —
	// ported arms in lua_scripts/northrend/npc_counselor_talbot.lua:
	// Initialize()/Reset() timer seeds -> OnEnterCombat CreateLuaEvent
	// arms (ShadowBolt 15537 urand(5s,12s), Deflection 51009
	// urand(20s,25s), SoulBlast 50992 urand(12s,18s), all non-triggered
	// DoCastVictim; UpdateVictim-gated in C++ so combat-start seeding
	// is faithful). The GetAreaId()==4125 gate is bridged
	// (lua_creature_events.go) and ported; behaviorally nil — Talbot
	// is a fixed-coords quest summon.
	// bCheck/FindNearestCreature GUID latch, MovementInform thassarian
	// latch, and JustDied ObjectAccessor/motion/reward legs are
	// documented-only (no bridges). Entry 25301 verifiable from the
	// C++ LastRites enum (zone_borean_tundra.cpp:413).
	RegisterLuaBoss("npc_counselor_talbot", 25301)
	// npc_mageguard_dalaran (Dalaran, trespasser guards) — ported arm
	// in lua_scripts/northrend/npc_mageguard_dalaran.lua: constructor
	// SetFlag NON_ATTACKABLE + ApplySpellImmune normal/magic damage
	// (event 5 spawn). Entries 29254/29255 verifiable from the C++
	// Dalaran enum (zone_dalaran.cpp:55-56). MoveInLineOfSight
	// trespasser teleport stays documented-only (ev27 engage-only,
	// IsInBack unbridged for handler creatures).
	RegisterLuaBoss("npc_mageguard_dalaran", 29254)
	RegisterLuaBoss("npc_mageguard_dalaran", 29255)
	// npc_wg_queue (Wintergrasp battle-magi) — ported arm
	// in lua_scripts/northrend/npc_wg_queue.lua: ungated Frost
	// Armor self-cast (0s, 180s re-arm). Entries 32169/32170
	// (Dalaran pair) + 35600/35601/35602 (Ironforge/Exodar/
	// Orgrimmar) wowhead-only + 35596/35597 (Shattrath pair)
	// + 35611 (Undercity) changelog-sourced (no C++ NPC_
	// constant for these NPCs).
	// Battlefield gossip arms stay documented-only (no
	// battlefield bridge).
	RegisterLuaBoss("npc_wg_queue", 32169)
	RegisterLuaBoss("npc_wg_queue", 32170)
	RegisterLuaBoss("npc_wg_queue", 35600)
	RegisterLuaBoss("npc_wg_queue", 35601)
	RegisterLuaBoss("npc_wg_queue", 35602)
	RegisterLuaBoss("npc_wg_queue", 35596)
	RegisterLuaBoss("npc_wg_queue", 35597)
	RegisterLuaBoss("npc_wg_queue", 35611)
	RegisterLuaBoss("npc_wg_queue", 35598)
	RegisterLuaBoss("npc_wg_queue", 35599)
	RegisterLuaBoss("npc_wg_queue", 35603)
	RegisterLuaBoss("npc_wg_queue", 35612)
	// npc_mimiron_assault_bot (Ulduar, Mimiron ACU phase) — ported arm
	// in lua_scripts/northrend/npc_mimiron_assault_bot.lua: engage-seeded
	// magnetic-field timer (14s, 30s re-arm, victim-cast 64668).
	// Entry 34057 wowhead-only (no C++ NPC_ constant for this add).
	// ROOT retarget arm stays documented-only (no HasUnitState /
	// AttackStart bridge).
	RegisterLuaBoss("npc_mimiron_assault_bot", 34057)
	// npc_mimiron_frost_bomb (Ulduar, Mimiron VX-001 phase) — ported arms
	// in lua_scripts/northrend/npc_mimiron_frost_bomb.lua: Reset-seeded
	// explosion chain (10s AOE 64626, +3s AOE 65354, despawn +3s).
	// Entry 34149 wowhead-only (spell 64627 summon link; no C++ NPC_
	// constant for this add).
	RegisterLuaBoss("npc_mimiron_frost_bomb", 34149)
	// npc_mimiron_proximity_mine (Ulduar, Mimiron Leviathan phase) —
	// ported arms in lua_scripts/northrend/npc_mimiron_proximity_mine.lua:
	// Reset-seeded arm chain (1.5s self 65345, +33.5s HasAura-gated
	// AOE 66351, despawn +1s). Entry 34362 wowhead-only (Ulduar page;
	// 27679 rejected as MoP-era).
	RegisterLuaBoss("npc_mimiron_proximity_mine", 34362)
	// npc_mimiron_flames (Ulduar, Mimiron hard-mode fire) — ported arm
	// in lua_scripts/northrend/npc_mimiron_flames.lua: Reset-seeded
	// one-shot spread cast (4s, AOE 64562). Entries 34363
	// "Flames (Initial)" (spell 64563 summon link) + 34121
	// "Flames (Spread)" (spell 64564 summon link), wowhead-only;
	// both share the AI (the spread chain needs every patch to
	// cast 64562). IN_PROGRESS despawn gate stays documented-only
	// (no instance boss-state bridge).
	RegisterLuaBoss("npc_mimiron_flames", 34363)
	RegisterLuaBoss("npc_mimiron_flames", 34121)
	// npc_aeranas (Hellfire Peninsula, Avruu's Orb summon) — ported
	// arms in lua_scripts/outland/npc_aeranas.lua: Reset faction/
	// questgiver/Talk + 8s monster-faction timer, combat shock/
	// winds pumps, 30% free arm. Entry 17085 wowhead-only.
	RegisterLuaBoss("npc_aeranas", 17085)
	// npc_magister_aledis (Hellfire Peninsula, Arelion's Secret duel)
	// — ported arms in lua_scripts/outland/npc_magister_aledis.lua:
	// Reset faction/gossip/immune, gossip-select duel chain,
	// fireball/frostnova pumps, 20% defeat arm. Entry 20159
	// wowhead-only. EngageWithTarget + 60s evade stay out (no
	// bridges); DoZoneInCombat is dungeon-gated.
	RegisterLuaBoss("npc_magister_aledis", 20159)
	// npc_taskmaster_fizzule (Northern Barrens, Deep Cover) —
	// ported arms in lua_scripts/kalimdor/npc_taskmaster_fizzule.lua:
	// flare/folly SpellHit latch, salute ReceiveEmote friend arm.
	// Entry 7233 wowhead-only. 120s MoveIdle/EnterEvadeMode revert
	// stays out (no bridges).
	RegisterLuaBoss("npc_taskmaster_fizzule", 7233)
	// npc_tournament_training_dummy (Icecrown, Argent Tournament) —
	// ported arms in lua_scripts/northrend/npc_tournament_training_dummy.lua:
	// EVENT_DUMMY_RECAST_DEFEND (Reset-seeded 5s repeat, 33272 leg:
	// !HasAura(64100) gate, triggered self-cast 64100), the SpellHit
	// charge/melee/ranged credit machine (event 14: 33272 gated,
	// 33229 ungated, 33243 gated), and DamageTaken damage=0
	// (event 9). Entries 33272 (NPC_CHARGE_TARGET), 33229
	// (NPC_MELEE_TARGET) and 33243 (NPC_RANGED_TARGET) verifiable
	// from the C++ TournamentDummy enum — all three share the AI
	// struct, so all three register.
	RegisterLuaBoss("npc_tournament_training_dummy", 33272)
	RegisterLuaBoss("npc_tournament_training_dummy", 33229)
	RegisterLuaBoss("npc_tournament_training_dummy", 33243)
}
