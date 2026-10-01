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
}
