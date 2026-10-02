package world

import (
	"context"
	"math/rand"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// BossAI defines the interface for scripted boss encounters.
// Reference: TrinityCore BossAI.h / ScriptedCreature.h.
type BossAI interface {
	OnReset(ctx context.Context, s *Server, motion *creatureMotion)
	OnAggro(ctx context.Context, s *Server, motion *creatureMotion, victim uint64)
	OnDamageTaken(ctx context.Context, s *Server, motion *creatureMotion, attacker uint64, damage uint32)
	OnKillPlayer(ctx context.Context, s *Server, motion *creatureMotion, victim uint64)
	OnEvade(ctx context.Context, s *Server, motion *creatureMotion)
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

// -------------------------------------------------------------
// Edwin VanCleef (Entry 639, Script: boss_vancleef)
// Reference: src/server/scripts/EasternKingdoms/Deadmines/boss_vancleef.cpp
// -------------------------------------------------------------
type vancleefAI struct {
	motion       *creatureMotion
	guardsCalled bool
	health66     bool
	health50     bool
	health33     bool
	health25     bool
	summons      []uint64
	// pendingSummonHooks accumulates summons spawned by the 50% arm while the
	// damage path holds motionMu; their Eluna hook fires after the caller's
	// unlock via firePendingSummonHooks (Lua handler methods lock motionMu
	// on demand, so the fire must never run under it).
	pendingSummonHooks []*creatureMotion
}

func newVanCleefAI(m *creatureMotion) BossAI {
	return &vancleefAI{motion: m}
}

func (ai *vancleefAI) OnReset(ctx context.Context, s *Server, m *creatureMotion) {
	ai.guardsCalled = false
	ai.health66 = false
	ai.health50 = false
	ai.health33 = false
	ai.health25 = false
	ai.despawnSummons(ctx, s, m)
}

// firePendingSummonHooks dispatches Eluna CREATURE_EVENT_ON_JUST_SUMMONED_CREATURE
// (19) for the summons queued by the 50% arm. Call only after the damage path
// releases motionMu. The boolean veto gates only the empty ScriptedAI::
// JustSummoned base (see fireCreatureSummoned), so it is discarded.
func (ai *vancleefAI) firePendingSummonHooks(ctx context.Context, s *Server, m *creatureMotion) {
	if len(ai.pendingSummonHooks) == 0 {
		return
	}
	pending := ai.pendingSummonHooks
	ai.pendingSummonHooks = nil
	for _, summon := range pending {
		s.fireCreatureSummoned(ctx, m, scripting.CreatureEventOnJustSummonedCreature, summon)
	}
}

// drainBossSummonHooks fires deferred Eluna summon hooks queued by a boss AI's
// damage handling. Only vancleefAI queues today; the rest are no-ops. Call
// after the damage path releases motionMu.
func (s *Server) drainBossSummonHooks(ctx context.Context, owner *creatureMotion, ai BossAI) {
	if vc, ok := ai.(*vancleefAI); ok {
		vc.firePendingSummonHooks(ctx, s, owner)
	}
}

func (ai *vancleefAI) OnAggro(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
	// SAY_AGGRO = 0: "None may challenge the Brotherhood!"
	s.broadcastCreatureTalk(ctx, m.Map, m.GUID, m.Entry, "Edwin VanCleef", 0, 0)
}

func (ai *vancleefAI) OnDamageTaken(ctx context.Context, s *Server, m *creatureMotion, attacker uint64, damage uint32) {
	if m.MaxHealth == 0 {
		return
	}
	hpPct := float32(m.Health) / float32(m.MaxHealth) * 100.0

	// 66% HP Quote
	if !ai.health66 && hpPct <= 66.0 {
		ai.health66 = true
		// SAY_ONE = 1: "Lapdogs, all of you!"
		s.broadcastCreatureTalk(ctx, m.Map, m.GUID, m.Entry, "Edwin VanCleef", 1, 0)
	}

	// 50% HP: Summons 2 Defias Blackguards
	if !ai.guardsCalled && hpPct <= 50.0 {
		ai.guardsCalled = true
		ai.health50 = true
		// SAY_SUMMON = 2: "%s calls more of his allies out of the shadows."
		s.broadcastCreatureTalk(ctx, m.Map, m.GUID, m.Entry, "Edwin VanCleef", 2, 0)
		s.castCreatureSpell(ctx, m, 5200, m.GUID) // SPELL_VANCLEEFS_ALLIES

		// Spawn 2 Blackguards (Entry 636) near VanCleef. The damage paths
		// (combat.go, spells.go) hold s.motionMu across OnDamageTaken, so
		// this arm uses the caller-held lock and must not relock it. The
		// Eluna CREATURE_EVENT_ON_JUST_SUMMONED_CREATURE (19) fire is
		// deferred to firePendingSummonHooks after the caller's unlock —
		// TempSummon::InitSummon (TemporarySummon.cpp:229-244) fires
		// Eluna::JustSummoned (CreatureHooks.cpp:165-171, args
		// (event, creature, summon)) once the summon exists.
		for i := 0; i < 2; i++ {
			bgGUID := uint64(0xF130000000000000) | uint64(636)<<24 | uint64(rand.Intn(90000)+10000)
			offset := float32((i+1)*2) - 3.0
			bgMotion := &creatureMotion{
				GUID:       bgGUID,
				Entry:      636,
				Map:        m.Map,
				InstanceID: m.InstanceID,
				HomeX:      m.X + offset,
				HomeY:      m.Y + offset,
				HomeZ:      m.Z,
				X:          m.X + offset,
				Y:          m.Y + offset,
				Z:          m.Z,
				Speed:      2.5,
				RunSpeed:   7.0,
				Health:     150,
				MaxHealth:  150,
				AttackTime: 2000,
				Faction:    m.Faction,
				Level:      18,
				InCombat:   true,
				TargetGUID: m.TargetGUID,
			}
			bgMotion.ThreatMgr = NewThreatManager(bgGUID)
			bgMotion.ThreatMgr.AddThreat(m.TargetGUID, 100, true)
			s.motionMapLocked(m.Map, m.InstanceID)[bgGUID] = bgMotion
			ai.summons = append(ai.summons, bgGUID)
			ai.pendingSummonHooks = append(ai.pendingSummonHooks, bgMotion)
			s.broadcastMonsterMoveInInstance(m.Map, m.InstanceID, bgGUID, bgMotion.X, bgMotion.Y, bgMotion.Z, bgMotion.X, bgMotion.Y, bgMotion.Z, 0, false)
		}
	}

	// 33% HP Quote
	if !ai.health33 && hpPct <= 33.0 {
		ai.health33 = true
		// SAY_TWO = 3: "Fools! Our cause is righteous!"
		s.broadcastCreatureTalk(ctx, m.Map, m.GUID, m.Entry, "Edwin VanCleef", 3, 0)
	}

	// 25% HP Quote
	if !ai.health25 && hpPct <= 25.0 {
		ai.health25 = true
		// SAY_THREE = 5: "The Brotherhood shall prevail!"
		s.broadcastCreatureTalk(ctx, m.Map, m.GUID, m.Entry, "Edwin VanCleef", 5, 0)
	}
}

func (ai *vancleefAI) OnKillPlayer(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
	// SAY_KILL = 4: "And stay down!"
	s.broadcastCreatureTalk(ctx, m.Map, m.GUID, m.Entry, "Edwin VanCleef", 4, 0)
}

func (ai *vancleefAI) OnEvade(ctx context.Context, s *Server, m *creatureMotion) {
	ai.OnReset(ctx, s, m)
}

func (ai *vancleefAI) OnUpdate(ctx context.Context, s *Server, m *creatureMotion, diff time.Duration, players []playerPos, now time.Time) {
}

func (ai *vancleefAI) despawnSummons(ctx context.Context, s *Server, owner *creatureMotion) {
	if s == nil || owner == nil {
		return
	}
	s.motionMu.Lock()
	motions := s.motionMapLocked(owner.Map, owner.InstanceID)
	var despawned []*creatureMotion
	for _, guid := range ai.summons {
		if m, ok := motions[guid]; ok {
			despawned = append(despawned, m)
		}
		delete(motions, guid)
	}
	ai.summons = nil
	s.motionMu.Unlock()
	// Eluna::SummonedCreatureDespawn (CreatureHooks.cpp:174-181),
	// CREATURE_EVENT_ON_SUMMONED_CREATURE_DESPAWN (20), args
	// (event, creature, summon), is fired by TempSummon::UnSummon
	// (TemporarySummon.cpp:274-284) before the summon leaves the world;
	// Go fires per summon after the unlock since the Lua handlers' object
	// methods lock motionMu on demand. The boolean veto gates only the
	// empty ScriptedAI::SummonedCreatureDespawn base, so it is discarded
	// (see fireCreatureSummoned).
	for _, m := range despawned {
		s.fireCreatureSummoned(ctx, owner, scripting.CreatureEventOnSummonedCreatureDespawn, m)
	}
}

// -------------------------------------------------------------
// Mr. Smite (Entry 646, Script: boss_mr_smite)
// Reference: src/server/scripts/EasternKingdoms/Deadmines/boss_mr_smite.cpp
// -------------------------------------------------------------
type mrSmiteAI struct {
	motion     *creatureMotion
	phase      uint8
	trashTimer time.Duration
	slamTimer  time.Duration
}

func newMrSmiteAI(m *creatureMotion) BossAI {
	return &mrSmiteAI{
		motion:     m,
		trashTimer: 6 * time.Second,
		slamTimer:  11 * time.Second,
	}
}

func (ai *mrSmiteAI) OnReset(ctx context.Context, s *Server, m *creatureMotion) {
	ai.phase = 0
	ai.trashTimer = 6 * time.Second
	ai.slamTimer = 11 * time.Second
}

func (ai *mrSmiteAI) OnAggro(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
	// SAY_AGGRO = 0: "You there, check out that noise!"
	s.broadcastCreatureTalk(ctx, m.Map, m.GUID, m.Entry, "Mr. Smite", 0, 0)
}

func (ai *mrSmiteAI) OnDamageTaken(ctx context.Context, s *Server, m *creatureMotion, attacker uint64, damage uint32) {
	if m.MaxHealth == 0 {
		return
	}
	hpPct := float32(m.Health) / float32(m.MaxHealth) * 100.0

	// Phase 1 -> Phase 2 at 66% HP
	if ai.phase == 0 && hpPct <= 66.0 {
		ai.phase = 1
		// Stun players with Smite Stomp (Spell 6432)
		s.castCreatureSpell(ctx, m, 6432, m.TargetGUID)
		// SAY_PHASE_1 = 2: "You landlubbers are tougher than I thought, I'll have to Improvise!"
		s.broadcastCreatureTalk(ctx, m.Map, m.GUID, m.Entry, "Mr. Smite", 2, 0)
	} else if ai.phase == 1 && hpPct <= 33.0 {
		ai.phase = 2
		// Stun players with Smite Stomp (Spell 6432)
		s.castCreatureSpell(ctx, m, 6432, m.TargetGUID)
		// SAY_PHASE_2 = 3: "D'ah! Now you're making me angry!"
		s.broadcastCreatureTalk(ctx, m.Map, m.GUID, m.Entry, "Mr. Smite", 3, 0)
	}
}

func (ai *mrSmiteAI) OnKillPlayer(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
}

func (ai *mrSmiteAI) OnEvade(ctx context.Context, s *Server, m *creatureMotion) {
	ai.OnReset(ctx, s, m)
}

func (ai *mrSmiteAI) OnUpdate(ctx context.Context, s *Server, m *creatureMotion, diff time.Duration, players []playerPos, now time.Time) {
	if !m.InCombat || m.TargetGUID == 0 {
		return
	}
	ai.trashTimer -= diff
	if ai.trashTimer <= 0 {
		ai.trashTimer = time.Duration(6000+rand.Intn(4000)) * time.Millisecond
		s.castCreatureSpell(ctx, m, 3391, m.TargetGUID) // SPELL_TRASH
	}

	ai.slamTimer -= diff
	if ai.slamTimer <= 0 {
		ai.slamTimer = 11 * time.Second
		s.castCreatureSpell(ctx, m, 6435, m.TargetGUID) // SPELL_SMITE_SLAM
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

func (ai *rhahkZorAI) OnDamageTaken(ctx context.Context, s *Server, m *creatureMotion, attacker uint64, damage uint32) {
}

func (ai *rhahkZorAI) OnKillPlayer(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
}

func (ai *rhahkZorAI) OnEvade(ctx context.Context, s *Server, m *creatureMotion) {
	ai.OnReset(ctx, s, m)
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

func (ai *taragamanAI) OnDamageTaken(ctx context.Context, s *Server, m *creatureMotion, attacker uint64, damage uint32) {
}

func (ai *taragamanAI) OnKillPlayer(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
}

func (ai *taragamanAI) OnEvade(ctx context.Context, s *Server, m *creatureMotion) {
	ai.OnReset(ctx, s, m)
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

func (ai *kreshAI) OnDamageTaken(ctx context.Context, s *Server, m *creatureMotion, attacker uint64, damage uint32) {
	if m.MaxHealth == 0 {
		return
	}
	hpPct := float32(m.Health) / float32(m.MaxHealth) * 100.0
	if !ai.shieldUsed && hpPct <= 20.0 {
		ai.shieldUsed = true
		s.castCreatureSpell(ctx, m, 8269, m.GUID) // Shell Shield
	}
}

func (ai *kreshAI) OnKillPlayer(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
}

func (ai *kreshAI) OnEvade(ctx context.Context, s *Server, m *creatureMotion) {
	ai.OnReset(ctx, s, m)
}

func (ai *kreshAI) OnUpdate(ctx context.Context, s *Server, m *creatureMotion, diff time.Duration, players []playerPos, now time.Time) {
}

// castCreatureSpell casts a spell from creature to target or self
func (s *Server) castCreatureSpell(ctx context.Context, m *creatureMotion, spellID uint32, targetGUID uint64) {
	if s == nil || m == nil || spellID == 0 {
		return
	}
	now := time.Now()
	castID := uint8(1)
	castTimeStamp := uint32(now.UnixMilli())
	hitTargets := []uint64{targetGUID}
	spellTarget := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnitWireMask, UnitGUID: targetGUID}
	goPkt := protocol.BuildSpellGo(m.GUID, m.GUID, castID, spellID, spellCastFlagGo, castTimeStamp, hitTargets, nil, spellTarget)

	if targetSess := s.findSessionByGUID(targetGUID); targetSess != nil {
		_ = targetSess.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, true)
		s.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, targetSess)
	} else {
		s.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, nil)
	}
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
func (ai *luaBossAI) OnDamageTaken(ctx context.Context, s *Server, m *creatureMotion, attacker uint64, damage uint32) {
}
func (ai *luaBossAI) OnKillPlayer(ctx context.Context, s *Server, m *creatureMotion, victim uint64) {
}
func (ai *luaBossAI) OnEvade(ctx context.Context, s *Server, m *creatureMotion) {}
func (ai *luaBossAI) OnUpdate(ctx context.Context, s *Server, m *creatureMotion, diff time.Duration, players []playerPos, now time.Time) {
}

// RegisterLuaBoss marks a creature entry as a Lua-scripted boss: its BossAI
// slot carries the no-op luaBossAI so engine boss handling (instance
// encounter admission) applies, while the Lua script drives the fight.
func RegisterLuaBoss(scriptName string, entry uint32) {
	RegisterBossAI(scriptName, entry, newLuaBossAI)
}

func init() {
	RegisterBossAI("boss_vancleef", 639, newVanCleefAI)
	RegisterBossAI("boss_mr_smite", 646, newMrSmiteAI)
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
	// armageddon InterruptNonMeleeSpells arm has no bridge; the
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
	// have no bridges; no summon model — the 8 sons of flame never spawn,
	// so the >8-adds early-emerge arm is unmodeled (emerge on the 90s
	// timer only); the magma blast IsWithinMeleeRange gate has no bridge;
	// the npc_son_of_flame add AI is not registered (no entry constant in
	// the C++ tree, DB-side ScriptName binding).
	RegisterLuaBoss("boss_ragnaros", 11502)
	// RegisterLuaBoss wires lua_scripts/blacktemple/boss_warlord_najentus.lua.
	// The UpdateAI casting gates have no bridge (no UNIT_STATE model); the
	// encounter-state bookkeeping is blocked on the instance-script model; the
	// impaling-spine GO summon (185584) and the spine-removal gossip arm have
	// no bridges (the GO handler needs the instance-creature lookup); the
	// needle-spine targeting SpellScript filter has no SpellScript bridge.
	RegisterLuaBoss("boss_najentus", 22887)
	// RegisterLuaBoss wires lua_scripts/blacktemple/boss_supremus.lua.
	// The UpdateAI casting gates have no bridge (no UNIT_STATE model); the
	// encounter-state bookkeeping is blocked on the instance-script model; the
	// ResetThreatList/AddThreat arms have no threat bridge (the hateful strike
	// target pick is approximated by the highest-health melee-range player);
	// the ApplySpellImmune taunt arms have no bridge; the volcano and molten
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
	// ResetThreatList/AddThreat and ApplySpellImmune arms have no bridges;
	// the shadow of death / spiritual vengeance AuraScripts have no
	// AuraScript bridge.
	RegisterLuaBoss("boss_teron_gorefiend", 22871)
	// RegisterLuaBoss wires lua_scripts/blacktemple/boss_gurtogg_bloodboil.lua.
	// The UpdateAI casting gates have no bridge (no UNIT_STATE model); the
	// encounter-state bookkeeping is blocked on the instance-script model
	// (DATA_GURTOGG_BLOODBOIL); the GetThreat/ModifyThreatByPercent/AddThreat
	// and AttackStart(oldTarget) victim-restore arms have no threat bridge
	// (the fel rage GUID is followed via per-GUID Lua state); the
	// ApplySpellImmune taunt/attack-me arms and the bewildering-strike
	// CanAIAttack gate have no bridges; the fel acid breath combat-reach
	// pick is approximated at 10 yd (no combat-reach bridge); the player
	// self-casts 40617/40603 have no player-side CastSpell bridge and are
	// applied via AddAura (their whole effect is the aura); the berserk and
	// death DoPlaySoundToSet arms have no sound bridge; the npc_fel_geyser
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
	// the minions weave (90%, fires empty — no summon bridge), the
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
	// The virtual-item-slot zeroing arms have no item-display
	// bridge; the charging AttackStart victim-switch arm has no
	// bridge; the instance/SetBossState/DoZoneInCombat arms are
	// blocked on the instance-script model.
	RegisterLuaBoss("boss_high_king_maulgar", 18831)
	// Krosh Firehand (18832) runs the fireball 2s floor (the
	// within-30-yd gate has no distance bridge), the spell-shield
	// arm and blast wave 60s (random alive player — the
	// threat-list/15-yd picks have no bridges; the
	// InterruptNonMeleeSpells arms have no interrupt bridge).
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
	// the SpellHitTarget knockback arms are unmodeled (event 15
	// never fires in the Go engine).
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
	// once-guarded <70% phase-2 switch, forked lightning in phase 2.
	// Teleport, shield-channel/enchanted/tainted/Coilfang summons,
	// the DATA_CANSTARTPHASE3 poll and the intro machine are
	// unmodeled. Tainted Elemental (22009) runs poison bolt
	// {5s,10s} + 30s despawn; the JustDied death relay is
	// unmodeled. Fight logic in
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
	// summon timer kept as bookkeeping only (no summon bridge).
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
	// illusion summons, the heroic mana-burn arm and the
	// cast-state gates have no instance / cross-creature /
	// summon / difficulty / cast-state bridges — see
	// lua_scripts/arcatraz/boss_harbinger_skyriss.lua. The
	// illusion AI (entries 21466/21467) is documented there
	// but not registered: its C++ AI is a no-op. Arcatraz
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
	// TRIGGERED summon lasher/flayer casts have no summon bridge
	// and the DoTransform display/immune arms have no display /
	// immune bridges — see lua_scripts/botanica/boss_laj.lua.
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
	// boss_doomlordkazzak and boss_doomwalker: the C++ files
	// carry no NPC_ entry constants anywhere in the sources —
	// the entries are DB-side only — so both are documented-
	// only, unregistered (void_reaver precedent); see
	// hidden_files/CHECKPOINT.md.
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
	// soul-spawn arm has no summon bridge and the totem
	// credit arm sits behind no world-search / faction /
	// movement / quest-credit bridges — documented only;
	// spell_unlocking_zuluheds_chains (SpellScript) and
	// npc_shadowmoon_tuber_node (unverifiable entry, SetData /
	// SpellHit arms) are documented-only.
	RegisterLuaBoss("npc_enraged_spirit", 21050)
	RegisterLuaBoss("npc_enraged_spirit", 21061)
	RegisterLuaBoss("npc_enraged_spirit", 21060)
	RegisterLuaBoss("npc_enraged_spirit", 21059)
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
	// bridges; npc_giant_spotlight (5-min despawn sweep) awaits
	// the world-search / despawn bridges — documented only.
	RegisterLuaBoss("npc_omen", 15467)
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
	// DisappearAndDie pair awaits the despawn bridge; Reset's
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
	// npc_blessed_banner (Icecrown zone script): the Reset() arm —
	// non-triggered self-cast SPELL_THREAT_PULSE 58113 (vaelastrasz
	// self-cast convention) + Talk(BANNER_SAY 0) textId-only
	// broadcast (lecraft aggro-Talk precedent) — armed on
	// OnSpawn(5) / OnReset(23); the banner never engages, so no
	// timers and no per-GUID state. SetRegenerateHealth(false)
	// has no regen bridge; the EVENT_SPAWN 3s summon chain
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
	// DamageTaken duel-end machine has no damage-hook /
	// player-self-cast / faction / despawn bridges.
	// npc_guardian_pavilion (MoveInLineOfSight trespasser machine:
	// GetAreaId 4676/4677 gates, HasAura 63987/63986 gates,
	// GetTeamId branch, player self-cast) sits behind the
	// proximity / area-ID / HasAura / team / player-self-cast
	// bridges. npc_tournament_training_dummy (entries
	// 33272/33229/33243 verifiable) — DamageTaken zeroing,
	// SpellHit 62544/62626/62874 credit machines, the HasAura-
	// gated EVENT_DUMMY_RECAST_DEFEND self-casts 64100/62719, and
	// the stunned control have no damage-hook / SpellHit /
	// HasAura / unit-state bridges. npc_frostbrood_skytalon
	// (VehicleAI) — the UpdateAI arms run through the unmodeled
	// VehicleAI base; SpellHit 59335/59319 has no SpellHit
	// bridge. Entry 30891 verifiable from the C++ BlessedBanner
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
	// ingredient Talk/cast machine + gossip), go_finklesteins_
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
	// has no HomePosition bridge — documented only; evade still re-arms
	// through OnReset(23). SPELL_SWEEPING_STRIKES 18765 /
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
	// lua_scripts/eastern_kingdoms/boss_vaelastrasz.lua.
	// Combat entry self-cast essence-of-the-red 23513 + SetHealth(30%
	// of max); cleave 19983 victim-cast 10s init -> 15s loop;
	// flamebreath 23461 victim-cast 15s init -> 8s loop (urand lower
	// bound); firenova 23462 victim-cast 20s init -> 15s loop;
	// burning-adrenaline tank 18173 victim-cast 45s init -> 45s loop.
	// 15%-HP Talk(SAY_HALFLIFE=3) one-shot latch via OnDamageTaken(9)
	// (gyth convention); KilledUnit 20% Talk(SAY_KILLTARGET=4) via
	// OnTargetDied(3) (Eluna::KilledUnit mapping). The gossip
	// pre-fight (EVENT_SPEECH_1..4 chain, SAY_LINE1..3, faction
	// change, AttackStart) has no creature-gossip bridge; tailswipe
	// is commented out in C++; the burning-adrenaline caster arm
	// gates on SelectTarget (no bridge); the burning-adrenaline
	// AuraScript is not modeled. Entry 13020 verifiable from the
	// C++ sources: blackwing_lair.h:54 names NPC_VAELASTRAZ = 13020
	// (boss_urok_doomhowl precedent).
	RegisterLuaBoss("boss_vaelastrasz", 13020)
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
	// — no summon bridge) and the seed-of-corruption 44141 / curse-of-
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
	// (no summon bridge), and the common-AI threat ring / death-count /
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
	// InterruptNonMeleeSpells/REACT_PASSIVE) has no bridges, so the
	// timers keep firing through crossings (documented deviation).
	// The phase/equip event machine (instance chest DATA_SMITE_CHEST /
	// GO 144111 via ObjectAccessor, MotionMaster MovePoint, equip-swap
	// 5191/5196/7230, stand-state, MoveChase) and Reset's equipment/
	// stand-state/react-state/NoCallAssistance arms are documented-only.
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
	// phases 2/3, unmodeled). EVENT_RANDOM_LAUGH has no sound bridge
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
	// Aku'mai Servant runs Frostbolt Volley (8398, random alive
	// player target, 2-4s init / 5-8s repeat), all non-triggered
	// (C++-exact). The Servant's Frost Nova DoCastAOE arm, the
	// JustDied DATA_EVENT increment, the IsSummonedBy DoZoneInCombat
	// leg, and the 15%-hp flee arms (4977/4823, unregistered) have
	// no bridges; the go_blackfathom_fire gossip and npc_morridune
	// escort scripts are documented-only. Entries C++-verified via
	// blackfathom_deeps.h BFDCreatureIds plus instance_
	// blackfathom_deeps.cpp's SetData(DATA_FIRE) summon arms
	// (ramstein strength); see lua_scripts/kalimdor/
	// blackfathom_deeps.lua.
	RegisterLuaBoss("npc_blackfathom_deeps_event", 4825)
	RegisterLuaBoss("npc_blackfathom_deeps_event", 4978)

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
	// enrage — triggered self-cast Frenzied Rage 3490 on projected health
	// (jeklik pre-damage subtraction, C++-exact); melee engine-driven.
	// The BossAI _Reset/_JustDied instance legs and the GetInstanceAI leg
	// have no bridges; see lua_scripts/kalimdor/boss_aku_mai.lua.
	RegisterLuaBoss("boss_aku_mai", 4829)

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
	// DoPlaySoundToSet(11018) has no sound bridge) on C++-verbatim
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
	// InterruptNonMeleeSpells arm has no spell-interrupt bridge (maiden
	// precedent); SAY_ENTER 0 is declared but never Talked in C++; the
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
	// 10s init, 10s repeat, unconditional). AttackStartCaster, the
	// Shoot 6660 / SetSheath arms gated on isAttackReady +
	// IsWithinMeleeRange (no bridge — magtheridon precedent), the
	// MovementInform pyramid legs (no instance SetData/GetData
	// bridges), DestroyDoor's SetFaction + MovePoint legs, and the
	// pyramid-gated gossip arms have no bridges; LandMine_Timer and
	// SPELL_GOBLIN_LAND_MINE / SPELL_WEEGLIS_BARREL are dead C++
	// legs — documented in the lua file, not wired.
	RegisterLuaBoss("npc_weegli_blastfuse", 7607)

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
	// + flag / aura bridges); npc_anubarak_spike (34660) and npc_frost_sphere
	// (34606) — zero registration, entry-verifiable-but-bridge-blocked
	// (DamageTaken hook / motion / threat / react / display bridges absent);
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
	// precedent); KilledUnit Talk(SAY_SLAY 2) C++-unconditional (the
	// player-gated corpse-scarab cast has no summon bridge); EVENT_LOCUST
	// Talk(EMOTE_LOCUST 3) + DoCastSelf(Locust Swarm 28785), init
	// randtime(1m40s,2m), repeat 1m30s (phase_hunter self-cast precedent;
	// the PHASE_SWARM/NORMAL gating only affects the unmodeled
	// impale/scarabs arms — documented-only); EVENT_BERSERK DoCastSelf
	// (Berserk 27680, triggered), 10min init, reschedules 10min.
	// DOCUMENTED-BLOCKED: intro/summon machine (InitializeAI, Reset
	// guardCorpses.clear, JustReachedHome SummonGuards — summon STRAND
	// absent; Is25ManRaid legs — no difficulty bridge, kelidan
	// precedent; EVENT_SPAWN_GUARD SummonCreatureGroup), JustSummoned
	// EMOTE_SPAWN Talk + SummonedCreatureDies/Despawn guardCorpses
	// bookkeeping (summon/cross-AI bridges absent; NPC_CRYPT_GUARD 16573
	// joins the entry-verifiable-but-bridge-blocked queue),
	// EVENT_IMPALE (random-target SelectTarget — cairne/kazzak precedent;
	// anti-chain leg noted), EVENT_SCARABS (corpse summon — summon
	// bridges absent), JustDied DoStartTimedAchievement(9891) (no
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
	// fires in Lua), EVENT_SKELETON/BANSHEE/ABOMINATION summons (no
	// summon bridge), EVENT_DESPAWN_MINIONS despawn + SAY_AGGRO (Talk not
	// ported orphaned — vortex precedent), EVENT_PHASE_TWO (no phase
	// bridge), EVENT_FROSTBOLT_VOLLEY / EVENT_CHAINS DoCastAOE (no
	// DoCastAOE bridge), EVENT_SHADOW_FISSURE / EVENT_DETONATE_MANA /
	// EVENT_FROST_BLAST SelectTarget(Random) (cairne/kazzak precedent),
	// transition reply/summon legs (instance model / no summon bridge),
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
	// 127618 spawn layout (no summon bridge + difficulty gate), the
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

	// Commander Stoutbeard (26796), Commander Kolurg (26798), The
	// Nexus — lua_scripts/northrend/boss_nexus_commanders.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0) +
	// DoCastSelf(BATTLE_SHOUT 31403) + EVENT_WHIRLWIND schedule
	// (6s/8s init); EVENT_WHIRLWIND DoCastSelf(WHIRLWIND 38618)
	// 19500ms/25s repeat; KilledUnit Talk(SAY_KILL 1) player-gated
	// (nalorakk variant); JustDied Talk(SAY_DEATH 2) (_JustDied
	// bookkeeping has no bridge). Documented-only: JustEngagedWith
	// RemoveAurasDueToSpell(FROZEN_PRISON 47543) (no aura-removal
	// bridge), EVENT_CHARGE_COMMANDER SelectTarget(Random)->DoCast(
	// CHARGE 60067) (no random-target SelectTarget bridge),
	// EVENT_FRIGHTENING_SHOUT DoCastAOE(19134) (no DoCastAOE bridge).
	// Single C++ script serves both faction-variant commanders
	// (twin_valkyr shared-handler precedent).
	RegisterLuaBoss("boss_nexus_commanders", 26796)
	RegisterLuaBoss("boss_nexus_commanders", 26798)

	// Anomalus (26763), The Nexus —
	// lua_scripts/northrend/boss_anomalus.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (instance->SetBossState(DATA_ANOMALUS, IN_PROGRESS) leg has no
	// instance bridge — tharon_ja precedent); JustDied Talk(SAY_DEATH
	// 1) (SetBossState(DONE) leg has no instance bridge).
	// Documented-only: Reset Initialize + SetBossState(NOT_STARTED)
	// (no instance bridge), GetData(DATA_CHAOS_THEORY) cross-AI leg
	// (no GetData bridge), SummonedCreatureDies chaosTheory=false leg
	// (no summon bridge), the UpdateAI home-position >60 evade hack
	// (no position bridge), the RIFT_SHIELD HasAura + GUID-lookup +
	// RemoveAurasDueToSpell machine (no aura/GUID bridges), the Phase
	// 0 + HealthBelowPct(50) shield+Talk(SAY_SHIELD 3)+SummonCreature(
	// CHAOTIC_RIFT 26918 entry-unverifiable local-enum-only)+AttackStart
	// +Talk(SAY_RIFT 2) machine (no health-pct/summon/random-target/
	// cross-AI bridges), uiSparkTimer SelectTarget(Random,0)->DoCast(
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
	// bridge — tharon_ja precedent; the three events.ScheduleEvent
	// legs have no timer-event bridge); KilledUnit Talk(SAY_KILL 1)
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
	// npc_unstable_sphere entry-unverifiable (28166, local-enum-
	// only) — no registration (Reset self-auras + MoveRandom +
	// DespawnOrUnsummon(19s), UpdateAI pulse timer 3s DoCast(
	// SPELL_UNSTABLE_SPHERE_PULSE 50757) — no bridges).
	RegisterLuaBoss("boss_drakos", 27654)

	// Mage-Lord Urom (27655), The Oculus —
	// lua_scripts/northrend/boss_urom.lua.
	// Ported arms (C++-exact): KilledUnit Talk(SAY_PLAYER_KILL 7)
	// C++-GATED on TYPEID_PLAYER (the nalorakk / kelthuzad
	// player-gate variant — ninth ported variant overall, sixth
	// identical to nalorakk / kelthuzad / gothik / thaddius /
	// keristrasza); JustDied Talk(SAY_DEATH 6) (_JustDied
	// bookkeeping has no bridge — tharon_ja precedent; the
	// DoCastSelf(SPELL_DEATH_SPELL) leg has no cast bridge).
	// Documented-only: JustEngagedWith / StartAttack Talk arms —
	// _platform-state-gated (Talk(SAY_SUMMON_1..3) on the first
	// three engages, Talk(SAY_AGGRO) only on the fourth engage
	// when _platform > 2); porting either unconditionally would be
	// C++-inexact — no _platform-state / summon / cast bridges;
	// Reset (SetControlled / SetDisableGravity / SetReactState /
	// DoCastSelf SPELL_EVOCATE 51602 / _Reset()); EnterEvadeMode
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
	// the DoCast(me, SPELL_DEATH_SPELL, true) leg has no cast
	// bridge — the urom precedent).
	// Documented-only: InitializeAI DoCast(SPELL_CENTRIFUGE_SHIELD
	// 50053) gated on GetBossState(DATA_DRAKOS) != DONE; Reset
	// (events.ScheduleEvent EVENT_AMPLIFY_MAGIC / EVENT_ENERGIZE_
	// CORES_VISUAL / EVENT_CALL_AZURE); the UpdateAI energize-
	// cores / azure-captain / amplify-magic event machine (no
	// timer-event / cast / random-target bridges); npc_azure_ring_
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
	// the UpdateAI arcane event machine (EVENT_ARCANE_BARRAGE /
	// EVENT_ARCANE_VOLLEY / EVENT_ENRAGED_ASSAULT / EVENT_SUMMON_
	// LEY_WHELP — no timer-event / cast bridges); DamageTaken
	// heroic+health-gated phase-shift machine (Talk SAY_SHIELD +
	// DoCast SPELL_PLANAR_SHIFT 51162 + 6x SPELL_PLANAR_
	// ANOMALIES 57959 — no bridges); JustSummoned /
	// SummonedCreatureDespawn (NPC_PLANAR_ANOMALY 30879 —
	// CombatStop / MoveRandom / SPELL_PLANAR_BLAST 57976 —
	// no motion / cast bridges); spell_eregos_planar_shift
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
	// aura bridges); npc_power_spark (NPC_POWER_SPARK 30084 — despawn-when-
	// reached UpdateAI, JustDied cast — no motion / cast bridges);
	// npc_melee_hover_disk (30234) / npc_caster_hover_disk (30248) —
	// VehicleAI PassengerBoarded / MovementInform machines (no vehicle /
	// passenger / motion bridges); npc_nexus_lord (DoAction ->
	// EVENT_NUKE_DUMMY / EVENT_ARCANE_SHOCK / EVENT_HASTE_BUFF machine —
	// no timer-event / cast bridges); npc_scion_of_eternity (NPC_SURGE_OF_
	// POWER 30334 — EVENT_ARCANE_BARRAGE machine, JustDied increments
	// DATA_SUMMON_DEATHS — no timer-event / instance / target-selection
	// bridges); npc_arcane_overload (NPC_ARCANE_OVERLOAD 30282 — SetGUID
	// DATA_LAST_OVERLOAD_GUID / phase-gated DespawnOrUnsummon / SpellHit
	// SPELL_ARCANE_BOMB_TRIGGER legs — no summon / despawn / SpellHit
	// bridges); npc_wyrmrest_skytalon (NPC_WYRMREST_SKYTALON 30161 —
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
	// 56916; flame tsunami WHISPER_LAVA_CHURN 9 + NPC_FLAME_TSUNAMI 30616
	// summon waves; flame breath Talk(SAY_BREATH 2) + SPELL_FLAME_BREATH
	// 56908; tail lash SPELL_TAIL_LASH 56910; cleave SPELL_CLEAVE 56909;
	// lava strike + urand SAY_SARTHARION_SPECIAL 7; call-dragon legs) +
	// the 35%-health SPELL_BERSERK 61632 leg (Talk SAY_SARTHARION_BERSERK 1,
	// gated on the three drake boss states) + the 10%-health soft-enrage
	// lava-strike cadence shift — no timer-event / cast / health / instance
	// bridges.
	RegisterLuaBoss("boss_sartharion", 28860)

	// Tenebron (30452), Shadron (30451), Vesperon (30449), Obsidian
	// Sanctum drake mini-bosses — lua_scripts/northrend/obsidian_sanctum.lua
	// (dummy_dragonAI base shared across the three drakes).
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (the DoZoneInCombat and the ScheduleEvent(EVENT_SHADOW_FISSURE
	// / EVENT_SHADOW_BREATH) legs have no bridges — the tharon_ja /
	// malygos precedents); KilledUnit player-gated Talk(SAY_SLAY 1)
	// (event 3 — razuvious player-gated variant precedent);
	// JustDied Talk(SAY_DEATH 2) (the _canLoot / SetLootRecipient
	// leg, the entry-switched power-aura removals (61248 / 58105 /
	// 61251) via RemoveAurasDueToSpell +
	// DoRemoveAurasDueToSpellOnPlayers, the acolyte KillSelf legs
	// (FindNearestCreature 31218 / 31219), the SetBossState
	// (DATA_TENEBRON / SHADRON / VESPERON, DONE) legs, and the
	// Twilight Revenge DoCast (60639) on Sartharion via instance
	// GUID fetch have no bridges — no aura / instance / cast /
	// creature-search bridges).
	// Documented-only: dummy_dragonAI Reset / SetData(DATA_CAN_LOOT) /
	// MovementInform (the waypoint machine, POINT_ID_INIT 100 /
	// POINT_ID_LAND 200 — no motion / instance / target-selection
	// bridges); OpenPortal (portal grid search GO_TWILIGHT_PORTAL
	// 193988 50.0f; Tenebron egg summons 30882 / 31204; Shadron /
	// Vesperon acolyte summons 31218 / 31219; Talk(WHISPER_OPEN_PORTAL
	// 6 / WHISPER_OPENED_PORTAL 7); portal SetRespawnTime 30000 — no
	// summon / cast / creature-search / GO-respawn bridges);
	// ExecuteEvent EVENT_SHADOW_FISSURE (random-target
	// SPELL_SHADOW_FISSURE 57579 — no target-selection / cast
	// bridges) / EVENT_SHADOW_BREATH (Talk(SAY_BREATH 3) +
	// DoCastVictim(SPELL_SHADOW_BREATH 57570) — no cast bridge); the
	// UpdateAI EVENT_FREE_MOVEMENT machine (no motion bridge);
	// npc_tenebron Reset / JustEngagedWith (+EVENT_HATCH_EGGS 30s) /
	// UpdateAI (no timer bridges); npc_shadron Reset (aura strips
	// 57948 / 57835 + SetBossState(DATA_PORTAL_OPEN, NOT_STARTED))
	// / JustEngagedWith (+EVENT_ACOLYTE_SHADRON 1min) / UpdateAI
	// (instance-gated OpenPortal + SetBossState(IN_PROGRESS) — no
	// timer / instance bridges); npc_vesperon Reset / JustEngagedWith
	// (+EVENT_ACOLYTE_VESPERON 1min) / UpdateAI (instance-gated
	// OpenPortal + DoCastVictim(SPELL_TWILIGHT_TORMENT_VESP 57948)
	// — no timer / instance / cast bridges);
	// npc_acolyte_of_shadron (31218) / npc_acolyte_of_vesperon
	// (31219): Reset / JustDied / UpdateAI melee — all aura / cast /
	// instance-gated, no Talk arms — no registration;
	// npc_twilight_eggs (30882 / 31204): SpawnWhelps (whelp summons
	// 30890 / 31214) / JustSummoned / UpdateAI — no summon /
	// timer / aura bridges — no registration;
	// npc_flame_tsunami (30616): aura / cast / creature-search legs
	// (57494 / 57491 / 60430, lava blaze 30643) — no bridges — no
	// registration; npc_twilight_fissure: entry UNVERIFIABLE from C++
	// (no local NPC enum — joins the entry-unverifiable queue) and
	// aura / cast-gated arms regardless — no registration;
	// npc_twilight_whelp: aura / timer / cast-gated arms
	// (60708) — no bridges — no registration;
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
	// (event 1; the CheckRequiredBosses / EnterEvadeMode gate,
	// the BossAI::JustEngagedWith passthrough and the six
	// ScheduleEvent legs have no bridges); KilledUnit
	// player-gated Talk(SAY_SLAY 1) (event 3 — razuvious
	// player-gated variant precedent, twenty-first player-gated
	// variant ported); JustDied Talk(SAY_DEATH 2) (event 4;
	// _JustDied passthrough has no bridge).
	// Documented-only: Reset / Initialize (_Reset() +
	// abuseTheOoze = 0); DoAction(ACTION_OOZE_DEAD 1) /
	// GetData(DATA_ABUSE_THE_OOZE 2) (no DoAction / GetData
	// bridges; consumer: achievement_abuse_the_ooze below); the
	// UpdateAI event machine (CHAIN_LIGHTNING random-target
	// 50830; LIGHTNING_SHIELD self 50831; STATIC_CHARGE
	// DoCastVictim 50834; LIGHTNING_RING self 51849; SUMMON
	// health-tier pipe summons 27982/27979/27981/27980 30s;
	// FRENZY triggered 28747 — no timer-event / cast /
	// target-selection / health-pct / summon bridges);
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
	// ported, twenty-second player-gated variant ported).
	// Documented-only: Reset / Initialize (_Reset() +
	// _crazyCatLady / _nineLives flags + HandleCats(true) —
	// no bridges); DoAction(ACTION_CRAZY_CAT_LADY 0 /
	// ACTION_DEFENDER_DIED 1) / GetData(DATA_NINE_LIVES 30763077
	// / DATA_CRAZY_CAT_LADY 30063007) — no DoAction / GetData
	// bridges; JustDied (DoPlaySoundToSet 15476 +
	// SendEncounterUnit disengage + HandleCats(false) grid
	// despawn — no Talk arms, no bridges; event 4 not
	// registered); the UpdateAI event machine (EVENT_SONIC_SCREECH
	// 64422 DoCastVictim 22-30s; EVENT_TERRIFYING_SCREECH
	// Talk(EMOTE_FEAR 3) + DoCastSelf 64386 + EVENT_BLAST 36-45s;
	// EVENT_BLAST DoCastAOE 64389; EVENT_SUMMON_DEFENDER
	// Talk(EMOTE_DEFENDER 4) + DoCastSelf 64448 +
	// EVENT_ACTIVATE_DEFENDER 2s; EVENT_ACTIVATE_DEFENDER
	// DoCastSelf 64449; EVENT_SWARNING_GUARDIAN random-target
	// DoCast 64396 25-45s; EVENT_BERSERK DoCastSelf 47008 +
	// Talk(SAY_BERSERK 2) 10min — no timer-event / cast /
	// target-selection bridges; EMOTE_FEAR / EMOTE_DEFENDER /
	// SAY_BERSERK ride the unbridgeable event machine);
	// npc_sanctum_sentry (34014 local enum — entry-verifiable but
	// bridge-blocked, no Talk arms; Reset self-cast 64369 and the
	// EVENT_RIP / EVENT_SAVAGE_POUNCE machine have no bridges;
	// JustDied DoAction(ACTION_CRAZY_CAT_LADY) has no
	// ObjectAccessor / DoAction bridges; npc_spark_of_ionar
	// no-bridgeable-arms precedent — no registration);
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
	// conditional, see below).
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
	// UpdateAI event machine (EVENT_PURSUE Talk(SAY_TARGET 3);
	// EVENT_SHUTDOWN Talk(SAY_OVERLOAD 11) + Talk(EMOTE_OVERLOAD
	// 13); EVENT_REPAIR Talk(EMOTE_REPAIR 14);
	// EVENT_THORIM_S_HAMMER Talk(SAY_TOWER_STORM 9);
	// EVENT_MIMIRON_S_INFERNO Talk(SAY_TOWER_FLAME 7);
	// EVENT_HODIR_S_FURY Talk(SAY_TOWER_FROST 6);
	// EVENT_FREYA_S_WARD Talk(SAY_TOWER_NATURE 8) — no
	// timer-event / cast / summon / target-selection bridges;
	// all timer-leg yells ride the unbridgeable event machine);
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
	// spell_load_into_catapult / spell_auto_repair /
	// spell_systems_shutdown / spell_pursue /
	// spell_vehicle_throw_passenger — no SpellScript bridge;
	// all five join the no-SpellScript-bridge queue.
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
	// no bridge — the sjonnir JustDied-Talk precedent).
	// Documented-only: Reset / Initialize (vehicle passenger
	// removal + DoStopTimedAchievement — no bridges);
	// GetData(DATA_SHATTERED 29252926) — no GetData bridge
	// (consumer: achievement_ignis_shattered below); JustSummoned
	// (NPC_IRON_CONSTRUCT faction / react-state / flag / immune
	// / root legs — no bridges); DoAction(ACTION_REMOVE_BUFF
	// 20 — RemoveAuraFromStack + GameTime <5s shattered window
	// — no bridges); the UpdateAI event machine (EVENT_JET
	// Talk(EMOTE_JETS 7); EVENT_SLAG_POT Talk(SAY_SLAG_POT 2) +
	// GRAB_POT / CHANGE_POT / END_POT vehicle chain; EVENT_SCORCH
	// Talk(SAY_SCORCH 3) + SummonCreature(NPC_GROUND_SCORCH
	// 33221); EVENT_CONSTRUCT Talk(SAY_SUMMON 1) + DoSummon;
	// EVENT_BERSERK Talk(SAY_BERSERK 5) — no timer-event / cast
	// / summon / target-selection / vehicle bridges; all
	// timer-leg yells ride the unbridgeable event machine);
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

	// XT-002 Deconstructor (33293), Ulduar —
	// lua_scripts/northrend/boss_xt002.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (event 1; the BossAI::JustEngagedWith passthrough, the
	// five ScheduleEvent legs and the DoStartTimedAchievement
	// leg have no bridges — the auriaya engage-port precedent);
	// KilledUnit player-gated Talk(SAY_SLAY 4) (event 3 — the
	// razuvious player-gated variant precedent — the
	// twenty-fourth player-gated variant ported); JustDied
	// Talk(SAY_DEATH 6) (event 4; the _JustDied() passthrough +
	// RemoveFlag NOT_SELECTABLE leg has no bridge — the sjonnir
	// JustDied-Talk precedent).
	// Documented-only: Reset / Initialize / EnterEvadeMode
	// (DoStopTimedAchievement 21027 — no bridges);
	// DoAction(ACTION_ENTER_HARD_MODE — no DoAction bridge);
	// GetData(DATA_HARD_MODE / DATA_HEALTH_RECOVERED /
	// DATA_GRAVITY_BOMB_CASUALTY — no GetData bridge);
	// SetData(DATA_TRANSFERED_HEALTH health-transfer +
	// DATA_GRAVITY_BOMB_CASUALTY — no SetData bridge);
	// ExposeHeart / DisposeHeart (Talk(SAY_HEART_OPENED 1) /
	// Talk(SAY_HEART_CLOSED 2) + Talk(EMOTE_HEART_CLOSED 9) —
	// phase / react-state legs unbridgeable);
	// PassengerBoarded Talk(EMOTE_SCRAPBOT 11) — no vehicle /
	// PassengerBoarded bridge; the UpdateAI event machine
	// (EVENT_SEARING_LIGHT / EVENT_GRAVITY_BOMB /
	// EVENT_TYMPANIC_TANTRUM Talk(SAY_TYMPANIC_TANTRUM 3) +
	// Talk(EMOTE_TYMPANIC_TANTRUM 10) / EVENT_PHASE_CHECK /
	// EVENT_SUBMERGE Talk(EMOTE_HEART_OPENED 8) /
	// EVENT_DISPOSE_HEART / EVENT_ENRAGE Talk(SAY_BERSERK 5) /
	// EVENT_ENTER_HARD_MODE / EVENT_RESUME_ATTACK — no
	// timer-event / cast / phase / react-state bridges; all
	// timer-leg yells ride the unbridgeable event machine);
	// npc_xt002_heart (NullCreatureAI — no Talk arms; no
	// registration); npc_scrapbot / npc_pummeller / npc_boombot
	// / npc_life_spark / npc_xt_void_zone (no Talk arms
	// anywhere — scheduler / movement / vehicle / damage-taken
	// legs — no registrations); the nine spell scripts
	// (spell_xt002_searing_light_spawn_life_spark /
	// spell_xt002_gravity_bomb_aura /
	// spell_xt002_gravity_bomb_damage /
	// spell_xt002_heart_overload_periodic /
	// spell_xt002_energy_orb (Talk(SAY_SUMMON 7) rides the
	// spell script) / spell_xt002_tympanic_tantrum /
	// spell_xt002_submerged / spell_xt002_321_boombot_aura /
	// spell_xt002_exposed_heart — no SpellScript / AuraScript
	// bridges; all nine join the no-SpellScript /
	// no-AuraScript-bridge queues); the three achievements
	// (achievement_nerf_engineering / achievement_heartbreaker
	// / achievement_nerf_gravity_bombs — GetData-based OnCheck
	// — no GetData / achievement bridges; all three join the
	// unmodeled-achievement queue).
	RegisterLuaBoss("boss_xt002", 33293)

	// General Vezax (33271), Ulduar —
	// lua_scripts/northrend/boss_general_vezax.lua.
	// Ported arms (C++-exact): JustEngagedWith Talk(SAY_AGGRO 0)
	// (event 1; the BossAI::JustEngagedWith passthrough, the
	// DoCast SPELL_AURA_OF_DESPAIR 62692 leg, the
	// CheckShamanisticRage (shaman HasSpell 30823 ->
	// SPELL_CORRUPTED_RAGE 68415) leg and the six ScheduleEvent
	// legs have no bridges — the auriaya engage-port precedent);
	// KilledUnit player-gated Talk(SAY_SLAY 1) (event 3 — the
	// razuvious player-gated variant precedent — the
	// twenty-fifth player-gated variant ported); JustDied
	// Talk(SAY_DEATH 3) (event 4; the _JustDied() passthrough +
	// DoRemoveAurasDueToSpellOnPlayers 62692 leg has no bridge —
	// the sjonnir JustDied-Talk precedent).
	// Documented-only: Reset / Initialize (_Reset() + flag
	// init — no bridges); SpellHitTarget (SPELL_SHADOW_CRASH_
	// HIT 62659 player leg -> shadowDodger = false — no
	// SpellHit bridge); GetData(DATA_SHADOWDODGER 29962997 /
	// DATA_SMELL_SARONITE 31813188 — no GetData bridge);
	// DoAction(ACTION_VAPORS_DIE / ACTION_ANIMUS_DIE — no
	// DoAction bridge); CheckShamanisticRage map-player scan —
	// no bridge; the UpdateAI event machine
	// (EVENT_SURGE_OF_DARKNESS Talk(EMOTE_SURGE_OF_DARKNESS 8)
	// + Talk(SAY_SURGE_OF_DARKNESS 2) / EVENT_SARONITE_VAPORS
	// hard-mode leg Talk(SAY_HARDMODE 5) + Talk(EMOTE_BARRIER
	// 7) + SPELL_SARONITE_BARRIER 63364 / 63145 + hard-mode
	// loot mode / EVENT_BERSERK Talk(SAY_BERSERK 4) — no
	// timer-event / cast / summon / loot-mode bridges; all
	// timer-leg yells ride the unbridgeable event machine);
	// boss_saronite_animus (no entry enum in C++ — spell-
	// summoned; entry-unverifiable — no registration; no Talk
	// arms; DoAction(ACTION_ANIMUS_DIE) leg unbridgeable);
	// npc_saronite_vapors (no entry enum in C++ — spell-
	// summoned; entry-unverifiable — no registration;
	// Talk(EMOTE_VAPORS 0) rides the unbridgeable constructor;
	// DamageTaken / DoAction(ACTION_VAPORS_DIE) legs — no
	// bridges); the three spell scripts
	// (spell_general_vezax_mark_of_the_faceless /
	// spell_general_vezax_mark_of_the_faceless_leech /
	// spell_general_vezax_saronite_vapors — no SpellScript /
	// AuraScript bridges; all three join the no-SpellScript /
	// no-AuraScript-bridge queues); the two achievements
	// (achievement_shadowdodger / achievement_smell_saronite —
	// GetData-based OnCheck — no GetData / achievement
	// bridges; both join the unmodeled-achievement queue).
	RegisterLuaBoss("boss_general_vezax", 33271)

	// Assembly of Iron (Steelbreaker 32867 / Runemaster Molgeim
	// 32927 / Stormcaller Brundir 32857), Ulduar —
	// lua_scripts/northrend/boss_assembly_of_iron.lua.
	// Ported arms (C++-exact): JustEngagedWith
	// Talk(SAY_AGGRO 0) (event 1; the BossAI::JustEngagedWith
	// passthrough, SetPhase / ScheduleEvent / DoCast
	// SPELL_HIGH_VOLTAGE 61890 legs have no bridges — the
	// auriaya engage-port precedent); KilledUnit player-gated
	// Talk(SAY_SLAY 1) (event 3 — the razuvious player-gated
	// variant precedent — the twenty-sixth / twenty-seventh /
	// twenty-eighth player-gated variants ported); JustDied
	// Talk(SAY_DEATH 3/4) (event 4; the _JustDied() passthrough,
	// the instance GetBossState DONE check, the DONE branch
	// (Talk SAY_ENCOUNTER_DEFEATED + DoCastAOE
	// SPELL_KILL_CREDIT 65195), the SetLootRecipient leg and
	// the supercharge DoAction cascade have no bridges — the
	// sjonnir JustDied-Talk precedent).
	// Documented-only: Reset / Initialize (all three — no
	// bridges); DoAction(ACTION_SUPERCHARGE / ACTION_ADD_CHARGE
	// — no DoAction bridge); GetData(DATA_PHASE_3 — no GetData
	// bridge); the three UpdateAI event machines (fusion punch
	// / static disruption / overwhelming power / rune of power
	// / shield of runes / rune of death / rune of summoning /
	// chain lightning / overload / lightning whirl / lightning
	// tendrils flight machine / berserk — no timer-event /
	// cast / summon / phase / movement / hover bridges; all
	// timer-leg yells ride the unbridgeable event machine);
	// the three spell scripts (spell_shield_of_runes /
	// spell_assembly_meltdown / spell_assembly_rune_of_
	// summoning — no AuraScript / SpellScript bridges; all
	// three join the no-AuraScript / no-SpellScript-bridge
	// queues); the achievement
	// (achievement_assembly_i_choose_you — GetData-based
	// OnCheck — no GetData / achievement bridges; joins the
	// unmodeled-achievement queue).
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
	// — documented in the lua header.
	RegisterLuaBoss("boss_hodir", 32845)

	// boss_freya.lua (lua_scripts/northrend/boss_freya.lua):
	// 32906 Freya + elders 32915 / 32914 / 32913 (boss_freya,
	// boss_elder_brightleaf / boss_elder_ironbranch /
	// boss_elder_stonebark — ulduar.h NPC_FREYA line 80,
	// NPC_BRIGHTLEAF line 137, NPC_STONEBARK line 138,
	// NPC_IRONBRANCH line 136; AddSC_boss_freya(), loader
	// decl 120 / call 316 — the ELEVENTH group of the
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
	// the thirty-fourth player-gated variant ported).
	// Talk(SAY_DEATH 7) lives in FinishEncounter() (called
	// from the event machine, not from JustDied) — event 4
	// deliberately not registered. Timer Talks, the pre-phase
	// DamageTaken SAY_JUMPDOWN arm (phase + instance-state
	// condition), cross-creature Talk(SAY_SPECIAL),
	// DoAction/SpellHit Talks, the ten spell scripts, the
	// condition script and the three achievements are
	// bridge-blocked — documented in the lua header.
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
	// deliberately not registered. Voice/brain/tentacle/
	// keeper/illusion creatures, the observation-ring gossip
	// Talks, timer Talks and all thirty-three spell scripts
	// are bridge-blocked — documented in the lua header.
	RegisterLuaBoss("boss_yogg_saron", 33134)
	RegisterLuaBoss("boss_yogg_saron", 33288)
	// Algalon the Observer (Ulduar) — boss_algalon_the_observer.cpp,
	// AddSC_boss_algalon_the_observer() (loader decl 123 / call 318) — the
	// FOURTEENTH group of the "// Ulduar" block, the last Ulduar boss group
	// (boss_yogg_saron -> boss_algalon_the_observer; next call is
	// AddSC_instance_ulduar()). Bridgeable arms: KilledUnit player-gated
	// Talk(SAY_ALGALON_KILL 20) (event 3 — the razuvious player-gated
	// variant precedent — the thirty-fifth player-gated variant ported),
	// with the C++ _hasYelled 1s rate limit modeled as an os.time()
	// per-guid deadline (1s resolution); DamageTaken phase-two
	// Talk(SAY_ALGALON_PHASE_TWO 11) gated on HealthBelowPctDamaged(20) +
	// the _phaseTwo latch (event 9 — the moroes threshold+latch precedent).
	// JustEngagedWith is _firstPull-gated (START_TIMER on first pull vs
	// AGGRO later — no flag bridge, no single C++-exact yell), the 2.5%
	// _fightWon damage=0 rides the unbridgeable outro machine, the intro /
	// big-bang / cosmic-smash / collapsing-star / ascend / outro / despawn
	// timer Talks, Brann's DoAction/MovementInform/timer Talks
	// (NPC_BRANN_BRONZBEARD_ALG 34064), the collapsing-star _dying
	// damage=0 (no Talk arm), the ten spell scripts and the GO script are
	// bridge-blocked — documented in the lua header.
	RegisterLuaBoss("boss_algalon_the_observer", 32871)
	// Prince Keleseth (Utgarde Keep) — boss_keleseth.cpp,
	// AddSC_boss_keleseth() (loader decl 126 / call 321) — the FIRST group
	// of the "// Utgarde Keep - Utgarde Keep" block (instance_ulduar ->
	// boss_keleseth; next call is AddSC_boss_skarvald_dalronn()).
	// Bridgeable arms: JustEngagedWith unconditional Talk(SAY_START_COMBAT
	// 1) (event 1 — the auriaya engage-port precedent; the runemage /
	// strategist guard AttackStart leg has no bridge), JustDied
	// Talk(SAY_DEATH 5) (event 4 — the sjonnir JustDied-Talk precedent).
	// The EVENT_SUMMON_SKELETONS / EVENT_FROST_TOMB timer Talks, the
	// targeted frost-tomb emote, npc_frost_tomb / npc_vrykul_skeleton (no
	// Talk arms), the spell_frost_tomb AuraScript and the
	// achievement_on_the_rocks GetData check are bridge-blocked —
	// documented in the lua header.
	RegisterLuaBoss("boss_keleseth", 23953)
	// Skarvald the Constructor and Dalronn the Controller (Utgarde Keep)
	// — boss_skarvald_dalronn.cpp, AddSC_boss_skarvald_dalronn() (loader
	// decl 127 / call 322) — the SECOND group of the "// Utgarde Keep -
	// Utgarde Keep" block (boss_keleseth -> boss_skarvald_dalronn).
	// Bridgeable arms: Skarvald JustEngagedWith Talk(SAY_AGGRO 0)
	// (event 1 — the auriaya engage-port precedent), both bosses'
	// KilledUnit player-gated Talk(SAY_KILL 3) (event 3 — the razuvious
	// player-gated variant precedent); the !IsInGhostForm gates are
	// satisfied structurally (ghost entries 27390 / 27389 not registered,
	// no bridgeable arms). Dalronn's aggro Talk rides the 5s
	// EVENT_DELAYED_AGGRO_SAY timer (event 1 deliberately NOT registered
	// — an immediate yell would deviate from C++), the
	// JustDied died-first/death split is instance-GuidData /
	// cross-creature-gated, the EVENT_DEATH_RESPONSE response Talk is
	// DoAction/timer-gated, and the enrage/spell-cast legs carry no Talk
	// arms — all bridge-blocked, documented in the lua header.
	RegisterLuaBoss("boss_skarvald_the_constructor", 24200)
	RegisterLuaBoss("boss_dalronn_the_controller", 24201)
	// Ingvar the Plunderer (Utgarde Keep) — boss_ingvar_the_plunderer.cpp,
	// AddSC_boss_ingvar_the_plunderer() (loader decl 128 / call 323) — the
	// THIRD group of the "// Utgarde Keep - Utgarde Keep" block
	// (boss_skarvald_dalronn -> boss_ingvar_the_plunderer).
	// Bridgeable arms: JustEngagedWith Talk(SAY_AGGRO 0) (event 1, human
	// entry only — the PHASE_EVENT/PHASE_UNDEAD early return is modeled
	// with the engaged latch, the auriaya engage-port precedent),
	// DamageTaken feign-death Talk(SAY_DEATH 2) (event 9, human entry only
	// — the feignDead latch mirrors the PHASE_HUMAN -> PHASE_EVENT
	// transition, the moroes threshold+latch precedent), JustDied
	// Talk(SAY_DEATH 2) unconditional (event 4, both entries — the sjonnir
	// JustDied-Talk precedent), KilledUnit player-gated Talk(SAY_SLAY 1)
	// (event 3, both entries — the razuvious player-gated variant
	// precedent). The undead entry 23980 is registered because phase-2
	// kills and the real death yell fire on it. The
	// EVENT_JUST_TRANSFORMED aggro yell + all spell-cast timer arms, the
	// feign-death damage=0 rewrite, DoAction(ACTION_START_PHASE_2),
	// npc_annhylde_the_caller (MovementInform YELL_RESURRECT /
	// instance-GuidData / cross-creature resurrect machine),
	// npc_ingvar_throw_dummy (no Talk arms) and the two spell scripts
	// (SpellScript / AuraScript) are bridge-blocked — documented in the
	// lua header.
	RegisterLuaBoss("boss_ingvar_the_plunderer", 23954)
	RegisterLuaBoss("boss_ingvar_the_plunderer", 23980)
	// Svala Sorrowgrave (Utgarde Pinnacle) — boss_svala.cpp,
	// AddSC_boss_svala() (loader decl 132 / call 327) — the FIRST group of
	// the "// Utgarde Keep - Utgarde Pinnacle" block (utgarde_keep ->
	// boss_svala).
	// Bridgeable arms: JustEngagedWith unconditional Talk(SAY_AGGRO 2)
	// (event 1 — the auriaya engage-port precedent), KilledUnit
	// player-gated Talk(SAY_SLAY 3) (event 3 — the razuvious player-gated
	// variant precedent), JustDied unconditional Talk(SAY_DEATH 4)
	// (event 4 — the sjonnir JustDied-Talk precedent). The whole intro
	// Talk machine (EVENT_INTRO_* timers, cross-creature Arthas talks),
	// the EVENT_RITUAL_PREPARATION Talk(SAY_SACRIFICE_PLAYER), the phase
	// transitions, spectator / ritual-channeler / scourge-hulk machines
	// (no Talk arms), the spell_paralyze_pinnacle SpellScript and the
	// achievement_incredible_hulk GetData check are bridge-blocked —
	// documented in the lua header.
	RegisterLuaBoss("boss_svala", 26668)
	// AddSC_boss_palehoof: Gortok Palehoof (Utgarde Pinnacle) — ported arms
	// (lua_scripts/northrend/boss_palehoof.lua): JustEngagedWith
	// unconditional Talk(SAY_AGGRO 0) (event 1 — the auriaya engage-port
	// precedent), KilledUnit player-gated Talk(SAY_SLAY 1) (event 3 — the
	// razuvious player-gated variant precedent). The
	// JustDied DoPlaySoundToSet(13467) has no Talk; the orb / awaken-spell /
	// miniboss timer machines, the ACTION_* DoAction legs and the
	// SpellScriptLoader hooks are bridge-blocked — documented in the lua
	// header.
	RegisterLuaBoss("boss_palehoof", 26687)
	// AddSC_boss_skadi: Skadi the Ruthless (Utgarde Pinnacle) — ported
	// arms (lua_scripts/northrend/boss_skadi.lua): KilledUnit
	// player-gated Talk(SAY_KILL 1) (event 3 — the razuvious
	// player-gated variant precedent), JustDied unconditional
	// Talk(SAY_DEATH 3) (event 4 — the sjonnir JustDied-Talk precedent).
	// Talk(SAY_AGGRO 0) rides DoAction(ACTION_START_ENCOUNTER) (event 1
	// deliberately not registered), the SAY_DRAKE_BREATH /
	// SAY_DRAKE_DEATH Talks, the whole flying-phase / harpoon / vehicle
	// DoAction machine, the grauf spline-chain emotes (no MovementInform
	// bridge), the ymirjar trash spell timers, the SpellScript /
	// AuraScript hooks, the achievement GetData check and the area
	// trigger are bridge-blocked — documented in the lua header.
	RegisterLuaBoss("boss_skadi", 26693)
	// AddSC_boss_ymiron: King Ymiron (Utgarde Pinnacle) — ported arms
	// (lua_scripts/northrend/boss_ymiron.lua): JustEngagedWith
	// unconditional Talk(SAY_AGGRO 0) (event 1 — the auriaya engage-port
	// precedent), KilledUnit player-gated Talk(SAY_SLAY 1) (event 3 —
	// the razuvious player-gated variant precedent), JustDied
	// unconditional Talk(SAY_DEATH 2) (event 4 — the sjonnir
	// JustDied-Talk precedent). The MovementInform POINT_BOAT summon
	// Talks (SAY_SUMMON_BJORN/HALDOR/RANULF/TORGYN 3-6), the ancestor
	// summon / channel / REACT_PASSIVE / EVENT_RESUME_COMBAT machine,
	// the EVENT_* spell timer legs, the spell_dark_slash SpellScript
	// OnHit hook and the achievement_kings_bane GetData check are
	// bridge-blocked — documented in the lua header.
	RegisterLuaBoss("boss_ymiron", 26861)
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
	// AddSC_boss_ichoron: Ichoron (Violet Hold) — ported arms
	// (lua_scripts/northrend/boss_ichoron.lua): JustEngagedWith
	// unconditional Talk(SAY_AGGRO 0) (event 1 — the auriaya
	// engage-port precedent), KilledUnit player-gated Talk(SAY_SLAY 1)
	// (event 3 — the razuvious player-gated variant precedent),
	// JustDied unconditional Talk(SAY_DEATH 2) (event 4 — the sjonnir
	// JustDied-Talk precedent). The SAY_SPAWN enum entry is never
	// Talk()ed (dead enum text); the Talk(SAY_SHATTER/EMOTE_SHATTER)
	// and Talk(SAY_BUBBLE) legs ride DoAction ACTION_PROTECTIVE_BUBBLE_
	// SHATTERED / ACTION_DRAINED (no DoAction bridge); Talk(SAY_ENRAGE)
	// rides the UpdateAI HealthBelowPct(25) leg (no bridge); npc_ichor_
	// globule carries zero Talk arms and is not registered; the Reset /
	// ScheduleTasks / JustReachedHome-SetData / ACTION_WATER_GLOBULE_HIT
	// / GetData(DATA_DEHYDRATION) machines, the four
	// spell_ichoron_* SpellScript/AuraScript hooks and the
	// achievement_dehydration GetData check are bridge-blocked —
	// documented in the lua header.
	RegisterLuaBoss("boss_ichoron", 29313)
}
