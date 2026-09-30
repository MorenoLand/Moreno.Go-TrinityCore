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
}
