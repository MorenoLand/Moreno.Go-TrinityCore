-- Gal'darah (Gundrak) — Lua port of
-- src/server/scripts/Northrend/Gundrak/boss_gal_darah.cpp
-- (boss_gal_darah + spell_gal_darah_impaling_charge +
-- spell_gal_darah_stampede_charge + spell_gal_darah_clear_puncture +
-- achievement_share_the_love).
-- Gundrak dungeon-script unit per northrend_script_loader.cpp order
-- (decl 23 / call 218, immediately after AddSC_boss_drakkari_colossus();
-- next: boss_eck).
-- Entry: 29306 Gal'darah (gundrak.h NPC_GAL_DARAH — the
-- RegisterCreatureAIWithFactory(GetGundrakAI) ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(creature, spell[, triggered]) = DoCastSelf
-- (phase_hunter / apothecary_hanes precedent — the trailing true is
-- the triggered port).
-- Ported arms (C++-exact for all modeled arms):
-- boss_gal_darah: JustEngagedWith Talk(SAY_AGGRO 0) (event 1);
-- KilledUnit Talk(SAY_SLAY 1) player-gated (event 3,
-- victim:GetObjectType() == "Player" — nalorakk precedent);
-- JustDied Talk(SAY_DEATH 2) + DoCastSelf(Clear Puncture 60022,
-- triggered) (event 4).
-- Unmodeled (no bridges — documented, not wired):
-- The whole combat rotation is phase-gated through events.SetPhase /
-- IsInPhase (PHASE_TROLL / PHASE_RHINO) and the phase flips are
-- driven by unbridged arms, so the rotation is not emulable without
-- a phase bridge (SetPhase/IsInPhase zero hits on the Lua surface —
-- moorabi precedent): EVENT_PUNCTURE (DoCastVictim Puncture 55276,
-- 10s init, 8s repeat), EVENT_WHIRLING_SLASH (DoCastVictim Whirling
-- Slash 55250, 21s init, 21s repeat), EVENT_ENRAGE (DoCastSelf
-- Enrage 55285, 15s init, 20s repeat), EVENT_STOMP (DoCastAOE Stomp
-- 55292, 25s init, 20s repeat — no DoCastAOE bridge anyway),
-- EVENT_STAMPEDE (Talk(SAY_SUMMON_RHINO 3) + DoCastAOE Stampede
-- 55218, 10s init, 15s repeat — no DoCastAOE bridge anyway),
-- EVENT_IMPALING_CHARGE (SelectTarget Random 60y -> DoCast Impaling
-- Charge 54956, 21s init, 31s repeat — no random-target SelectTarget
-- bridge, cairne/kazzak precedent), and EVENT_TRANSFORM (phase flip
-- via IsInPhase + Talk(SAY_TRANSFORM_1/2) + DoCastSelf Transform
-- Rhino 55297 / Transform Back 55299 — its 5s trigger follows the
-- second unbridged Impaling Charge / second Whirling Slash, so the
-- flip-back timeline cannot be faithfully rebuilt).
-- Reset DoCastAOE(Hearth Beam Visual 54988, triggered) — no
-- DoCastAOE bridge. JustSummoned rhino-spirit leg (NPC_RHINO_SPIRIT
-- 29791 -> Stampede Spirit 55221/55219 self-casts + Stampede Spirit
-- Charge 59823 on a random target) — the summon itself comes from
-- the unbridged DoCastAOE(STAMPEDE), and random-target select has
-- no bridge. EnterEvadeMode summons.DespawnAll + _DespawnAtEvade —
-- no despawn bridge (terestian precedent). SpellHit(SPELL_TRANSFORM
-- _BACK 55299) -> RemoveAurasDueToSpell(55297) — event 15 never
-- fires (standing) + no aura-removal bridge (aeranas precedent).
-- SetGUID/GetData(DATA_SHARE_THE_LOVE) impaled-players latch —
-- ObjectAccessor::GetUnit + cross-AI bridges absent (zero hits).
-- spell_gal_darah_impaling_charge (SpellScript 54956/59827: charge
-- hit -> target casts Impaling Charge Control Vehicle 54958 on the
-- boss + caster AI()->SetGUID(target, DATA_SHARE_THE_LOVE)) — no
-- SpellScript binding bridge (razelikh precedent) + cross-AI SetGUID
-- bridge absent — documented-only.
-- spell_gal_darah_stampede_charge (SpellScript 55220/59823: rhino
-- spirit despawns 1s after its charge hits) — no SpellScript binding
-- bridge + no despawn bridge — documented-only.
-- spell_gal_darah_clear_puncture (SpellScript 60022: remove Puncture
-- 55276 / heroic 59826) — no SpellScript binding bridge —
-- documented-only.
-- achievement_share_the_love (AchievementCriteriaScript: GetData
-- (DATA_SHARE_THE_LOVE) >= 5) — no achievement-criteria bridge on
-- the Lua surface (snakes precedent) + cross-AI GetData bridge
-- absent — documented-only.
-- JustEngagedWith InterruptNonMeleeSpells — no bridge; BossAI
-- _Reset/_JustDied instance bookkeeping has no bridge.

local ENTRY_GAL_DARAH = 29306

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local SPELL_CLEAR_PUNCTURE = 60022

local function galDarahEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

local function galDarahTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function galDarahDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    creature:CastSpell(creature, SPELL_CLEAR_PUNCTURE, true)
end

RegisterCreatureEvent(ENTRY_GAL_DARAH, 1, galDarahEnterCombat)
RegisterCreatureEvent(ENTRY_GAL_DARAH, 3, galDarahTargetDied)
RegisterCreatureEvent(ENTRY_GAL_DARAH, 4, galDarahDied)
