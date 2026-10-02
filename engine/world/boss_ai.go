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
}
