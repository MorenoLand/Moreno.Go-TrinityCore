-- Reliquary of Souls (Black Temple) — Lua port of
-- src/server/scripts/Outland/BlackTemple/boss_reliquary_of_souls.cpp
-- (boss_reliquary_of_souls, npc_enslaved_soul and
-- npc_reliquary_combat_trigger only; the three essence boss AIs
-- and the four aura/spell scripts documented below, not
-- registered); black_temple.h:36 (DATA_RELIQUARY_OF_SOULS = 5,
-- sixth boss), :53-:54 (DATA_ESSENCE_OF_SUFFERING = 20,
-- DATA_ESSENCE_OF_DESIRE = 21), :59 (DATA_RELIQUARY_COMBAT_
-- TRIGGER = 26), :76 (NPC_RELIQUARY_OF_SOULS = 22856), :91
-- (NPC_RELIQUARY_WORLD_TRIGGER = 23472), :92 (NPC_ENSLAVED_SOUL =
-- 23469), :113 (NPC_RELIQUARY_COMBAT_TRIGGER = 23417).
-- Creature entries: 22856 Reliquary of Souls (C++ ScriptName
-- "boss_reliquary_of_souls" per RegisterBlackTempleCreatureAI in
-- AddSC_boss_reliquary_of_souls; the creature_template ScriptName
-- binding is DB-side — no TDB in this workspace, so only the
-- C++-side naming is verified); 23469 Enslaved Soul (ScriptName
-- "npc_enslaved_soul"); 23417 Reliquary combat trigger (ScriptName
-- "npc_reliquary_combat_trigger"). The three essence bosses bind
-- the ScriptNames "boss_essence_of_suffering" /
-- "boss_essence_of_desire" / "boss_essence_of_anger" DB-side, but
-- no essence entry constant or literal exists anywhere in the C++
-- tree (only the summon spells 41488/41493/41496, whose summon
-- targets are DB-side), so the entries cannot be verified from
-- the C++ sources alone and the essence AIs are not registered
-- (garr firesworn convention).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 9 OnDamageTaken (returns false, newDamage; the boolean
-- is consumed by the engine, the number rewrites damage), 23
-- OnReset. Timers via CreateLuaEvent; melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms):
-- Reliquary of Souls: OnEnterCombat models the C++ ACTION_START_
-- COMBAT arm — the combat trigger's MoveInLineOfSight sighting
-- (70 yd, player-only, boundary-gated) that delivers it has no
-- MoveInLineOfSight bridge, so the fight starts on pull instead
-- (ragnaros-intro convention); the UNIT_STAND_STATE_STAND
-- SetByteValue arm has no bridge. Arms the 10s EVENT_SUBMERGE
-- (one-shot, C++-exact): triggered self-cast submerge visual
-- 28819, non-triggered self-cast of the phase's summon spell
-- (41488 suffering / 41493 desire / 41496 anger — the picker
-- mirrors C++ GetSummonSpell, but the phase is pinned at
-- SUFFERING: the ACTION_ESSENCE_OF_SUFFERING_DEAD /
-- ACTION_ESSENCE_OF_DESIRE_DEAD phase advances have no
-- DoAction/instance-creature bridge); the enslaved-souls
-- DoAction(ACTION_KILL_SELF) arm has no cross-creature DoAction
-- bridge. OnLeaveCombat/OnReset: cancel timers, drop per-GUID
-- state (C++ Reset: _Reset, REACT_PASSIVE, _inCombat=false, phase
-- reset — the react/summons arms have no bridge). OnDied: cancel
-- timers (the instance SetBossState(DONE) arm is blocked on the
-- instance-script model). The UpdateAI !_inCombat early return is
-- implicit — timers only exist once pulled; the UNIT_STATE_
-- CASTING gate has no bridge (jeklik convention). The ACTION_
-- KILL_SELF arm (KillSelf + combat-trigger DoAction) and the
-- EnterEvadeMode summons.DespawnAll/KillAssyncEvents/
-- _DespawnAtEvade arms have no bridges (evade-side cleanup is
-- engine-side).
-- Enslaved Soul: OnReset: triggered self-cast enslaved soul
-- passive 41535 (C++-exact; the REACT_PASSIVE arm, the
-- reliquary->JustSummoned link and the DoZoneInCombat arms have
-- no react/instance bridges, and the 3s REACT_AGGRESSIVE +
-- DoZoneInCombat scheduler arm has no bridge). OnDamageTaken(9):
-- lethal damage is rewritten to 0 (C++ damage = 0, C++-exact)
-- with the once-guard in per-GUID state, plus the triggered
-- self-cast soul release 41542 (the REACT_PASSIVE/AttackStop/
-- MotionMaster-clear arms have no bridge, and the 500ms-delayed
-- KillSelf arm has no despawn/kill bridge — the soul survives at
-- its pre-lethal health after the release cast). The reliquary-
-- driven DoAction(ACTION_KILL_SELF) soul-release path has no
-- bearer (no cross-creature DoAction bridge).
-- Reliquary combat trigger: OnDamageTaken(9) always rewrites
-- damage to 0 (C++ damage = 0 invulnerable arm, C++-exact). Its
-- MoveInLineOfSight -> reliquary ACTION_START_COMBAT arm is
-- modeled on the reliquary side as the pull start (no
-- MoveInLineOfSight bridge); the Reset DONE-state despawn arm is
-- blocked on the instance-script model; the EnterEvadeMode
-- reliquary relay and the DoAction(ACTION_KILL_SELF) -> KillSelf
-- arms have no bridges.
-- Not registered (entries unverifiable from the C++ sources):
-- boss_essence_of_suffering — Reset triggered-AoE 41292 (aura of
-- suffering); pull: soul drain 41303 20s then {30s,35s} self-cast
-- (the SPELLVALUE_MAX_TARGETS=5 arm has no bridge) / frenzy 41305
-- 45s then {45s,50s} self-cast + Talk(SUFF_SAY_ENRAGE=2); lethal
-- damage -> never-die (damage=0), Talk(SUFF_SAY_RECAP=3),
-- AttackStop, REACT_PASSIVE, InterruptNonMeleeSpells, MovePoint
-- to the despawn point, then MovementInform -> reliquary
-- ACTION_ESSENCE_OF_SUFFERING_DEAD + triggered submerge self-cast
-- + DespawnOrUnsummon(2s) — the whole transition is blocked on
-- the instance/DoAction/movement bridges; Talk lines SUFF_SAY_
-- AGRO=0 / SUFF_SAY_SLAY=1 / SUFF_SAY_ENRAGE=2 / SUFF_SAY_RECAP=3
-- (SUFF_EMOTE_ENRAGE=5 is the frenzy SpellScript arm, bridgeless).
-- boss_essence_of_desire — Reset triggered self-cast 41350 (aura
-- of desire); pull: spirit shock 41426 11s then {10s,15s} on the
-- victim / rune shield 41431 16s self-cast / deaden 41410 31s on
-- the victim + Talk(DESI_SAY_SPEC=2), Talk(DESI_SAY_FREED=0); same
-- never-die/MovePoint transition via ACTION_ESSENCE_OF_DESIRE_
-- DEAD (blocked); Talk lines DESI_SAY_FREED=0 / DESI_SAY_SLAY=1 /
-- DESI_SAY_SPEC=2 / DESI_SAY_RECAP=3.
-- boss_essence_of_anger — Reset self-cast 41337 (aura of anger);
-- pull: Talk(ANGER_SAY_FREED=0), soul scream 41545 11s self-cast,
-- spite 41376 20s self-cast (the SPELLVALUE_MAX_TARGETS=3 arm has
-- no bridge) + Talk(ANGER_SAY_SPITE=5), the 1s tanker-check loop
-- (on tank switch: Talk(ANGER_SAY_SEETHE=2) + Talk(ANGER_EMOTE_
-- SEETHE=3) + triggered seethe 41364 self-cast — the
-- GetThreat/victim-GUID arms have no threat bridge), FREED_2
-- Talk(ANGER_SAY_FREED_2=1) on the 1s..3min one-shot; JustDied:
-- DoPlaySoundToSet(11401) (no sound bridge) + reliquary ACTION_
-- KILL_SELF (blocked). Talk lines ANGER_SAY_FREED=0 / ANGER_SAY_
-- FREED_2=1 / ANGER_SAY_SEETHE=2 / ANGER_EMOTE_SEETHE=3 /
-- ANGER_SAY_SPITE=5.
-- The four aura/spell scripts have no bridges (standing gaps):
-- spell_reliquary_of_souls_aura_of_desire (41350 — damage proc
-- -> 41352 at half damage, -5 periodic decay), spell_reliquary_
-- of_souls_submerge (28819 — stand-state bytes on apply/remove),
-- spell_reliquary_of_souls_spite (41376 — on-remove casts 41377
-- on the target), spell_reliquary_of_souls_frenzy (41305 —
-- after-cast Talk(SUFF_EMOTE_ENRAGE)).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate
-- has no UNIT_STATE bridge — timers fire unconditionally (jeklik
-- convention); no instance-script model — boss admission via the
-- luaBossAI shim (BossAI::JustEngagedWith/JustDied and the
-- DATA_RELIQUARY_OF_SOULS / DATA_ESSENCE_OF_*/DATA_RELIQUARY_
-- COMBAT_TRIGGER bookkeeping arms skipped).

local ENTRY_RELIQUARY_OF_SOULS = 22856
local ENTRY_ENSLAVED_SOUL = 23469
local ENTRY_RELIQUARY_COMBAT_TRIGGER = 23417

local SPELL_SUBMERGE_VISUAL = 28819
local SPELL_SUMMON_ESSENCE_OF_SUFFERING = 41488
local SPELL_SUMMON_ESSENCE_OF_DESIRE = 41493
local SPELL_SUMMON_ESSENCE_OF_ANGER = 41496

local SPELL_ENSLAVED_SOUL_PASSIVE = 41535
local SPELL_SOUL_RELEASE = 41542

local PHASE_SUFFERING, PHASE_DESIRE, PHASE_ANGER = 1, 2, 3

local timers = {}
local reliquaryState = {}
local soulState = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ boss_reliquary_of_souls::GetSummonSpell, phase-picked.
-- Phase stays at SUFFERING: the ACTION_ESSENCE_OF_SUFFERING_DEAD
-- / ACTION_ESSENCE_OF_DESIRE_DEAD advances have no DoAction /
-- instance-creature bridge.
local function summonSpellForPhase(phase)
    if phase == PHASE_ANGER then
        return SPELL_SUMMON_ESSENCE_OF_ANGER
    elseif phase == PHASE_DESIRE then
        return SPELL_SUMMON_ESSENCE_OF_DESIRE
    end
    return SPELL_SUMMON_ESSENCE_OF_SUFFERING
end

-- C++ EVENT_SUBMERGE: triggered self-cast 28819, then the phase's
-- summon spell, non-triggered (C++ DoCastSelf default is
-- triggered=false — vaelastrasz convention). One-shot
-- (C++-exact). The enslaved-souls DoAction(ACTION_KILL_SELF) arm
-- has no cross-creature DoAction bridge.
local function onSubmerge(creature, guid)
    creature:CastSpell(creature, SPELL_SUBMERGE_VISUAL, true)
    local state = reliquaryState[guid]
    local phase = PHASE_SUFFERING
    if state ~= nil then
        phase = state.phase
    end
    creature:CastSpell(creature, summonSpellForPhase(phase), false)
end

-- C++ DoAction(ACTION_START_COMBAT): _inCombat = true, arm
-- EVENT_SUBMERGE at 10s. The trigger's MoveInLineOfSight sighting
-- that delivers this action has no bridge — the fight starts on
-- pull (ragnaros-intro convention). The STAND stand-state arm has
-- no bridge.
local function reliquaryEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    reliquaryState[guid] = { phase = PHASE_SUFFERING }
    schedule(guid, "submerge", 10000, function()
        onSubmerge(creature, guid)
    end)
end

local function reliquaryClear(guid)
    cancelTimers(guid)
    reliquaryState[guid] = nil
end

local function reliquaryLeaveCombat(event, creature)
    reliquaryClear(creature:GetGUID())
end

-- C++ JustDied: events.Reset + instance SetBossState(DONE) — the
-- instance arm is blocked on the instance-script model.
local function reliquaryDied(event, creature, killer)
    reliquaryClear(creature:GetGUID())
end

local function reliquaryReset(event, creature)
    reliquaryClear(creature:GetGUID())
end

-- C++ npc_enslaved_soul::Reset: triggered self-cast 41535
-- (C++-exact). The REACT_PASSIVE arm, the reliquary JustSummoned
-- link and the 3s REACT_AGGRESSIVE + DoZoneInCombat scheduler arm
-- have no react/instance bridges.
local function soulReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    soulState[guid] = nil
    creature:CastSpell(creature, SPELL_ENSLAVED_SOUL_PASSIVE, true)
end

local function soulClear(guid)
    cancelTimers(guid)
    soulState[guid] = nil
end

local function soulLeaveCombat(event, creature)
    soulClear(creature:GetGUID())
end

local function soulDied(event, creature, killer)
    soulClear(creature:GetGUID())
end

-- C++ npc_enslaved_soul::DamageTaken: lethal damage is zeroed
-- (damage = 0, C++-exact), and once per soul the soul-release
-- cast fires. The REACT_PASSIVE/AttackStop/MotionMaster-clear arms
-- and the 500ms-delayed KillSelf arm have no bridges (no
-- despawn/kill bridge) — the soul survives at its pre-lethal
-- health after the release cast.
local function soulDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if health > 0 and damage >= health then
        local guid = creature:GetGUID()
        local state = soulState[guid]
        if state == nil then
            state = {}
            soulState[guid] = state
        end
        if not state.dead then
            state.dead = true
            creature:CastSpell(creature, SPELL_SOUL_RELEASE, true)
        end
        return false, 0
    end
end

-- C++ npc_reliquary_combat_trigger::DamageTaken: damage = 0 —
-- the trigger is unkillable (C++-exact). Its MoveInLineOfSight ->
-- reliquary ACTION_START_COMBAT arm has no bridge (modeled on the
-- reliquary side as the pull start); the Reset DONE-despawn arm is
-- blocked on the instance-script model; the EnterEvadeMode
-- reliquary relay and the DoAction(ACTION_KILL_SELF) -> KillSelf
-- arms have no bridges.
local function triggerDamageTaken(event, creature, attacker, damage)
    return false, 0
end

RegisterCreatureEvent(ENTRY_RELIQUARY_OF_SOULS, 1, reliquaryEnterCombat)
RegisterCreatureEvent(ENTRY_RELIQUARY_OF_SOULS, 2, reliquaryLeaveCombat)
RegisterCreatureEvent(ENTRY_RELIQUARY_OF_SOULS, 4, reliquaryDied)
RegisterCreatureEvent(ENTRY_RELIQUARY_OF_SOULS, 23, reliquaryReset)

RegisterCreatureEvent(ENTRY_ENSLAVED_SOUL, 2, soulLeaveCombat)
RegisterCreatureEvent(ENTRY_ENSLAVED_SOUL, 4, soulDied)
RegisterCreatureEvent(ENTRY_ENSLAVED_SOUL, 9, soulDamageTaken)
RegisterCreatureEvent(ENTRY_ENSLAVED_SOUL, 23, soulReset)

RegisterCreatureEvent(ENTRY_RELIQUARY_COMBAT_TRIGGER, 9, triggerDamageTaken)
