package world

import (
	"context"
	"math/rand"
	"time"

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
	ai.despawnSummons(s, m.Map, m.InstanceID)
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

		// Spawn 2 Blackguards (Entry 636) near VanCleef
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
			s.motionMu.Lock()
			s.motionMapLocked(m.Map, m.InstanceID)[bgGUID] = bgMotion
			s.motionMu.Unlock()
			ai.summons = append(ai.summons, bgGUID)
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

func (ai *vancleefAI) despawnSummons(s *Server, mapID, instanceID uint32) {
	if s == nil {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	for _, guid := range ai.summons {
		delete(s.motionMapLocked(mapID, instanceID), guid)
	}
	ai.summons = nil
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

func init() {
	RegisterBossAI("boss_vancleef", 639, newVanCleefAI)
	RegisterBossAI("boss_mr_smite", 646, newMrSmiteAI)
	RegisterBossAI("boss_rhahkzor", 644, newRhahkZorAI)
	RegisterBossAI("boss_taragaman", 11520, newTaragamanAI)
	RegisterBossAI("boss_kresh", 3653, newKreshAI)
}
