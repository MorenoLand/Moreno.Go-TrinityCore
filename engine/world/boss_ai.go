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
}
