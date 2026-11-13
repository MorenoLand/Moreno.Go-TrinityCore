-- Razuvious (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_razuvious.cpp
-- (boss_razuvious (BossAI, BOSS_RAZUVIOUS); npc_dk_understudy
-- (ScriptedAI); AddSC_boss_razuvious registers both; loader decl 63 /
-- call 258 per northrend_script_loader.cpp — the FIFTH Naxxramas group
-- in AddNorthrendScripts(), immediately after AddSC_boss_grobbulus(),
-- under the "// Naxxramas" marker; the call after it is
-- AddSC_boss_kelthuzad()).
-- Entry: 16061 Razuvious (naxxramas.h NPC_RAZUVIOUS — kalecgos pass;
-- instance_naxxramas.cpp binds NPC_RAZUVIOUS -> RazuviousGUID and
-- DATA_RAZUVIOUS -> RazuviousGUID; the CreatureScript ScriptName
-- binding is instance-shimmed, the creature_template binding DB-side
-- as usual). npc_dk_understudy: NPC_DK_UNDERSTUDY = 16803 (same pass).
-- Sole-source verified: whole-server-tree grep for "boss_razuvious"
-- hits boss_razuvious.cpp only (loader carries only the decl/call
-- lines); grep for "npc_dk_understudy" hits the .cpp only; zero sql/
-- hits. No razuvious lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 5 OnSpawn, 23 OnReset, 24 OnReachedHome
-- (understudy: 1, 7, 24). Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim, creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell) = DoCastSelf (phase_hunter /
-- apothecary_hanes precedent). C++ randtime -> math.random
-- (amanitar / boss_black_knight precedent). Victim gating: C++
-- KilledUnit checks TYPEID_PLAYER or (TYPEID_UNIT and entry
-- NPC_DK_UNDERSTUDY); Lua expresses it via victim:GetObjectType()
-- (nalorakk precedent) plus victim:GetEntry() for creatures.
-- Ported arms (C++-exact for all modeled arms):
-- boss_razuvious: JustEngagedWith Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith me-form DoZoneInCombat PORTED
-- (ScriptedCreature.cpp:542; heigan precedent — the 2026-10-23
-- header claim lumped this into "instance-bookkeeping"); the
-- instance CheckRequiredBosses leg and the summons-container
-- DoZoneInCombat call stay out (SetBossState IN_PROGRESS is set in
-- enterCombat, below — bridge live 2026-11-05); me->StopMoving;
-- KilledUnit Talk(SAY_SLAY 1) (event 3 — C++-GATED: Talk fires only
-- when the victim is a player or a TYPEID_UNIT with entry
-- NPC_DK_UNDERSTUDY 16803 — the nalorakk player-gate variant, extended
-- with the understudy-entry arm); EVENT_STRIKE —
-- DoCastVictim(Unbalancing Strike 26613), 21s init, 6s repeat
-- (moroes precedent); EVENT_SHOUT — DoCastAOE(Disrupting Shout 29107)
-- 1-arg DoCastAOE (triggered defaults false), 16s init, 16s repeat — lowered to self-cast
-- creature:CastSpell(creature, spell) (boss_ai.go DoCastAOE self-cast
-- convention — selin_fireheart / mr_smite precedent; the Oct-12 "no
-- DoCastAOE bridge" claim was wrong, retracted this run); JustDied
-- Talk(SAY_DEATH 3) + DoCastAOE(Hopeless 29125, triggered) as
-- creature:CastSpell(creature, spell, true) (event 4; the amanitar
-- OnDied-cast precedent; the RemoveCharmAuras leg has no bridge
-- and the events.Reset leg rides cancelTimers (SetBossState DONE is
-- set in died, below — bridge live 2026-11-05).
-- SummonAdds (InitializeAI + JustReachedHome) + the three boss-state
-- legs (IN_PROGRESS / DONE / NOT_STARTED) + the understudy
-- JustEngagedWith cross-AI DoZoneInCombat + the understudy
-- JustReachedHome DONE-despawn — PORTED 2026-11-05, see below.
-- All timers cancelled on 2/4/23 (gargolmar precedent).
-- No summon-arm Talk orphaning: EVENT_KNIFE's target leg rides the
-- GetAITargets threat-list pool + 45yd filter (knifeTick — the old
-- "unbridged SelectTarget" claim was stale).
-- Unmodeled (no bridges — documented, not wired):
-- boss_razuvious SpellHit — SPELL_UNDERSTUDY_TAUNT 29060 ->
-- Talk(SAY_TAUNTED 2, caster) PORTED 2026-11-13 (strip 8,
-- razyviousSpellHit, event 14): the 2026-10-15 stay-out is retired —
-- ev14 DOES fire on the pet cast path (pet_combat.go:8073,
-- executePetSpellZeroDamageTail, the zero-damage leg a taunt takes;
-- handlePetCastSpell accepts charmer motions, spells.go:2417), so
-- the MC'd understudy's taunt reaches Razuvious; Talk(textId[,
-- target]) is bridged (lua_creature_events.go:1252) and the
-- broadcastCreatureTalk twin drops a non-player whisper target
-- exactly like C++ SendChat (creature_text.go:140-150);
-- JustEngagedWith EVENT_ATTACK (7s, one-shot, boss_razuvious.cpp:140-146)
-- — SetCombatMovement(true) is a mid-combat flag no-op per the
-- ScriptedAI NOTE, but the paired MoveChase(victim) IS the real
-- effect: the chase generator persists for the rest of the fight.
-- PORTED 2026-10-22 as a 7s one-shot Lua timer flipping the per-tick
-- pursuit gate (lua_creature_events.go) — the same net behavior
-- (05453b0f dropped this arm, misreading the flag no-op as the
-- whole arm).
-- Reset — SetCombatMovement(false) + _Reset() — PORTED 2026-10-22
-- (creature:SetCombatMovement, lua_creature_events.go — C++
-- boss_razuvious.cpp:169);
-- InitializeAI / JustReachedHome — SummonAdds: SummonCreatureGroup(
-- SUMMON_GROUP_10MAN 1) + 25-man SUMMON_GROUP_25MAN 2 — PORTED
-- 2026-11-05 (creature:SummonCreatureGroup bridged, tick 238; the old
-- "no summon bridge" claim was stale): event 5 (InitializeAI leg,
-- DONE-gated via creature:GetBossState, hakkar precedent) + event 24
-- (JustReachedHome leg); the tracked rows despawn on the evade path
-- (event 2, the _Reset summons.DespawnAll leg — NOT on death, where
-- C++ keeps them) and re-summon on reaching home. The instance
-- SetBossState legs are PORTED too (bridge live 2026-11-05,
-- lua_creature_events.go): IN_PROGRESS on enter combat, DONE on died
-- (moroes precedent), NOT_STARTED on reset; CheckRequiredBosses
-- stays out.
-- npc_dk_understudy (ScriptedAI, 16803): LoadEquipment(1) — no
-- equipment bridge; JustEngagedWith — PORTED 2026-11-05: the
-- SetUInt32Value(UNIT_NPC_EMOTESTATE, EMOTE_ONESHOT_NONE) leg has no
-- bridge, but the cross-AI DoZoneInCombat on Razuvious via
-- _instance->GetGuidData(DATA_RAZUVIOUS) ports through the ev5
-- SetGuidData(DATA_RAZUVIOUS 2) latch on Razuvious (selin/crystal
-- precedent) + GetInstanceCreature(DATA_RAZUVIOUS) in the new event-1
-- understudy handler (the 2026-10-23 "no PORT site" claim is retired
-- — the site is the event-1 registration itself); JustReachedHome —
-- DespawnOrUnsummon when BOSS_RAZUVIOUS == DONE — PORTED 2026-11-05
-- via creature:GetBossState(BOSS_RAZUVIOUS 10) in the new event-24
-- understudy handler (the base ScriptedAI::JustReachedHome leg is a
-- no-op); UpdateAI — bloodStrikeTimer DoCastVictim(
-- SPELL_UNDERSTUDY_BLOOD_STRIKE 61696) — PORTED 2026-10-23 as the
-- event-7 understudyAIUpdate below: the timer is 0-initialized and
-- never re-armed, so C++ casts on every AI tick; the
-- !isPossessedByPlayer() gates ride the new creature:IsCharmed()
-- bridge (lua_creature_events.go). OnCharmed — stays out: the arm's
-- only effects are SetUInt32Value emotestate + cross-AI DoZoneInCombat
-- on Razuvious (different creature, instance-leg-gated — same stay-out
-- as the understudy JustEngagedWith leg above, 2026-10-23 re-eval);
-- the ScriptedAI::JustEngagedWith base is
-- a no-op hook.

local ENTRY_RAZUVIOUS = 16061
local ENTRY_DK_UNDERSTUDY = 16803

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_TAUNTED = 2
local SAY_DEATH = 3

local SPELL_UNBALANCING_STRIKE = 26613
local SPELL_DISRUPTING_SHOUT = 29107 -- boss_razuvious.cpp:37
local SPELL_JAGGED_KNIFE = 55550 -- boss_razuvious.cpp:38
local SPELL_HOPELESS = 29125 -- boss_razuvious.cpp:39
local SPELL_UNDERSTUDY_TAUNT = 29060 -- boss_razuvious.cpp:40

local BOSS_RAZUVIOUS = 10 -- naxxramas.h NAXEncounter (ANUBREKHAN 0 ... RAZUVIOUS 10)
local DATA_RAZUVIOUS = 2 -- naxxramas.h NAXData64 (ANUBREKHAN 0, FAERLINA 1, RAZUVIOUS 2)
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

local SUMMON_GROUP_10MAN = 1
local SUMMON_GROUP_25MAN = 2

local timers = {}
local understudies = {} -- per-boss-GUID SummonAdds rows (Reset DespawnAll leg)

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

-- C++ SummonAdds (boss_razuvious.cpp:74-79): SummonCreatureGroup(1)
-- + group 2 on 25-man; the returned rows are tracked per boss GUID
-- for the Reset summons.DespawnAll leg (see leaveCombat below).
local function summonAdds(creature, guid)
    local summoned = creature:SummonCreatureGroup(SUMMON_GROUP_10MAN)
    if creature:Is25ManRaid() then
        for _, add in ipairs(creature:SummonCreatureGroup(SUMMON_GROUP_25MAN)) do
            summoned[#summoned + 1] = add
        end
    end
    understudies[guid] = summoned
end

local function despawnAdds(guid)
    local adds = understudies[guid]
    if adds then
        for _, add in ipairs(adds) do
            if add ~= nil then
                add:Despawn()
            end
        end
        understudies[guid] = nil
    end
end

-- C++ SelectTarget(Random, 0, 45.0f) leg: alive units on the threat
-- list (C++ playerOnly default = false), within IsWithinCombatRange
-- (Unit.cpp:583-598: 3D dist < nominal + both combat reaches; player
-- reach is DEFAULT_PLAYER_COMBAT_REACH 1.5, ObjectDefines.h:40) —
-- the GetAITargets pool (the omen precedent's jaraxxus idiom sans
-- player filter, plus the reach-adjusted 45yd filter; the old "no
-- random-target SelectTarget bridge (cairne/kazzak precedent)" claim
-- was stale).
local PLAYER_COMBAT_REACH = 1.5

local function randomThreatTargetInRange(creature, maxDist)
    local candidates = {}
    local casterReach = creature:GetCombatReach()
    for _, unit in ipairs(creature:GetAITargets()) do
        local targetReach = PLAYER_COMBAT_REACH
        if unit:GetObjectType() ~= "Player" then
            targetReach = unit:GetCombatReach()
        end
        if not unit:IsDead()
                and creature:GetDistance(unit) < maxDist + casterReach + targetReach then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_KNIFE (:156-159): SelectTarget(Random, 0, 45.0f) ->
-- randomThreatTargetInRange(creature, 45), nil-guarded pick (C++
-- `if` — skip cast on nil); DoCast 2-arg = non-triggered ->
-- CastSpell(target, SPELL_JAGGED_KNIFE 55550); re-arm
-- randtime(10s,15s) unconditional (outside the target-if,
-- C++-exact); 10s init (:129).
local function knifeTick(creature, guid)
    local target = randomThreatTargetInRange(creature, 45)
    if target then
        creature:CastSpell(target, SPELL_JAGGED_KNIFE)
    end
    schedule(guid, "knife", {10000, 15000}, function()
        knifeTick(creature, guid)
    end)
end

-- C++ EVENT_STRIKE: DoCastVictim(Unbalancing Strike 26613), 21s init,
-- 6s repeat.
local function strikeTick(creature, guid)
    creature:CastSpell(nil, SPELL_UNBALANCING_STRIKE)
    schedule(guid, "strike", 6000, function()
        strikeTick(creature, guid)
    end)
end

-- C++ EVENT_KNIFE (:156-159) — SelectTarget(Random, 0, 45.0f)
-- (playerOnly default false, C++-exact) over the threat-list pool +
-- 45yd filter (randomThreatTargetInRange — the omen idiom), nil-guarded
-- pick, DoCast 2-arg non-triggered (Jagged Knife 55550), re-arm
-- randtime(10s,15s) unconditional, 10s init (:129); the old
-- "no random-target SelectTarget bridge" claim was stale.
-- C++ EVENT_SHOUT (:152-154): DoCastAOE(Disrupting Shout 29107) 1-arg (triggered defaults false)
-- (non-triggered); 16s init, 16s repeat. DoCastAOE lowers to self-cast
-- creature:CastSpell(creature, spell) (boss_ai.go convention —
-- selin_fireheart / mr_smite precedent; the old "no DoCastAOE bridge"
-- claim was wrong).
local function shoutTick(creature, guid)
    creature:CastSpell(creature, SPELL_DISRUPTING_SHOUT)
    schedule(guid, "shout", 16000, function()
        shoutTick(creature, guid)
    end)
end

local function razyviousEnterCombat(event, creature, target)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542; heigan precedent)
    creature:SetBossState(BOSS_RAZUVIOUS, BOSS_STATE_IN_PROGRESS) -- BossAI::JustEngagedWith (moroes precedent)
    creature:SetCombatMovement(false) -- Reset arm, boss_razuvious.cpp:169 (ranged AI stands and casts)
    creature:StopMoving() -- boss_razuvious.cpp:123 (the summons.DoZoneInCombat
    -- sibling at :124 stays out — summons-container call, 2026-10-23 re-eval)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "attack", 7000, function()
        -- C++ EVENT_ATTACK (7s, one-shot): SetCombatMovement(true) is a
        -- mid-combat flag no-op, but the paired MoveChase(victim) is the
        -- real effect — chase persists for the rest of the fight. The
        -- Lua twin flips the per-tick pursuit gate
        -- (lua_creature_events.go), the same net behavior.
        creature:SetCombatMovement(true)
    end)
    schedule(guid, "strike", 21000, function()
        strikeTick(creature, guid)
    end)
    schedule(guid, "shout", 16000, function()
        shoutTick(creature, guid)
    end)
    schedule(guid, "knife", 10000, function()
        knifeTick(creature, guid)
    end)
end

local function razyviousLeaveCombat(event, creature)
    -- C++ EnterEvadeMode -> Reset -> _Reset summons.DespawnAll; the
    -- re-summon rides event 24 below. Death never fires event 2, so
    -- this stays evade-only, C++-exact.
    despawnAdds(creature:GetGUIDLow())
    cancelTimers(creature:GetGUIDLow())
end

-- C++ KilledUnit: Talk(SAY_SLAY) when victim->GetTypeId() ==
-- TYPEID_PLAYER or (TYPEID_UNIT and GetEntry() == NPC_DK_UNDERSTUDY
-- 16803).
local function razyviousTargetDied(event, creature, victim)
    if victim then
        local otype = victim:GetObjectType()
        if otype == "Player" or (otype == "Creature" and victim:GetEntry() == ENTRY_DK_UNDERSTUDY) then
            creature:Talk(SAY_SLAY)
        end
    end
end

-- C++ boss_razuviousAI::SpellHit (boss_razuvious.cpp:102-106):
-- SPELL_UNDERSTUDY_TAUNT 29060 -> Talk(SAY_TAUNTED 2, caster).
-- No gate: every taunt hit yells. The caster is the MC'd understudy
-- creature (pet cast path, pet_combat.go:8073); Talk's optional
-- target arg carries it verbatim (arlockk precedent).
local function razyviousSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_UNDERSTUDY_TAUNT then
        creature:Talk(SAY_TAUNTED, caster)
    end
end

local function razyviousDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    -- C++ JustDied (:108-118): Talk(SAY_DEATH) then DoCastAOE(Hopeless
    -- 29125, triggered) — the amanitar precedent (Go carries no
    -- alive-gate, so the OnDied cast is faithful). The summons
    -- RemoveCharmAuras + instance->SetBossState bookkeeping has no
    -- bridge (tharon_ja precedent).
    creature:CastSpell(creature, SPELL_HOPELESS, true)
    creature:SetBossState(BOSS_RAZUVIOUS, BOSS_STATE_DONE) -- C++ JustDied (:115); moroes precedent
    cancelTimers(creature:GetGUIDLow())
end

local function razyviousReset(event, creature)
    -- C++ Reset (boss_razuvious.cpp:168-171): SetCombatMovement(false)
    -- + _Reset bookkeeping (SetBossState NOT_STARTED; moroes precedent).
    creature:SetCombatMovement(false)
    creature:SetBossState(BOSS_RAZUVIOUS, BOSS_STATE_NOT_STARTED)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ InitializeAI (boss_razuvious.cpp:82-89): if not dead and boss
-- state != DONE, Reset() + SummonAdds(). The SetGuidData latch is the
-- C++ instance OnCreatureCreate bind (NPC_RAZUVIOUS -> DATA_RAZUVIOUS,
-- instance_naxxramas.cpp) in its Lua form (selin/crystal precedent);
-- the understudy's JustEngagedWith reads it back.
local function razuviousSpawn(event, creature)
    creature:SetGuidData(DATA_RAZUVIOUS)
    if not creature:IsDead() and creature:GetBossState(BOSS_RAZUVIOUS) ~= BOSS_STATE_DONE then
        creature:SetCombatMovement(false) -- Reset leg (:168-171)
        creature:SetBossState(BOSS_RAZUVIOUS, BOSS_STATE_NOT_STARTED) -- _Reset leg
        summonAdds(creature, creature:GetGUIDLow())
    end
end

-- C++ JustReachedHome (boss_razuvious.cpp:91-96): _JustReachedHome
-- (no model) + SummonAdds() + MotionMaster Initialize (engine-side
-- on home arrival, no bridge needed).
local function razuviousReachHome(event, creature)
    summonAdds(creature, creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_RAZUVIOUS, 1, razyviousEnterCombat)
RegisterCreatureEvent(ENTRY_RAZUVIOUS, 2, razyviousLeaveCombat)
RegisterCreatureEvent(ENTRY_RAZUVIOUS, 3, razyviousTargetDied)
RegisterCreatureEvent(ENTRY_RAZUVIOUS, 4, razyviousDied)
RegisterCreatureEvent(ENTRY_RAZUVIOUS, 5, razuviousSpawn)
RegisterCreatureEvent(ENTRY_RAZUVIOUS, 14, razyviousSpellHit)
RegisterCreatureEvent(ENTRY_RAZUVIOUS, 23, razyviousReset)
RegisterCreatureEvent(ENTRY_RAZUVIOUS, 24, razuviousReachHome)

-- npc_dk_understudy (16803) UpdateAI, boss_razuvious.cpp:205-216. The
-- bloodStrikeTimer is 0-initialized and never re-armed, so C++ casts
-- Blood Strike (61696) on every AI tick. !isPossessedByPlayer() gates
-- both the UpdateVictim early-out and the cast; Lua twins it with
-- IsCharmed() — the only charm source in this fight is priest Mind
-- Control, so the engine's Charmed flag is exact here. GetVictim() nil
-- twins the !UpdateVictim() return; melee stays engine-driven, and the
-- charmed (MC'd) understudy is pet-driven by the engine anyway.
local SPELL_UNDERSTUDY_BLOOD_STRIKE = 61696 -- boss_razuvious.cpp:41

local function understudyAIUpdate(event, creature, diff)
    if not creature:IsCharmed() then
        if creature:GetVictim() == nil then
            return
        end
        creature:CastSpell(nil, SPELL_UNDERSTUDY_BLOOD_STRIKE)
    end
end

-- C++ npc_dk_understudyAI::JustEngagedWith (boss_razuvious.cpp:177-183):
-- the emotestate SetUInt32Value leg has no bridge; the cross-AI
-- DoZoneInCombat on Razuvious (via GetGuidData(DATA_RAZUVIOUS),
-- latched by the boss's event-5 handler above) ports here.
local function understudyEnterCombat(event, creature)
    local razuvious = creature:GetInstanceCreature(DATA_RAZUVIOUS)
    if razuvious ~= nil then
        razuvious:DoZoneInCombat()
    end
end

-- C++ npc_dk_understudyAI::JustReachedHome (boss_razuvious.cpp:185-191):
-- DespawnOrUnsummon when BOSS_RAZUVIOUS == DONE (the base
-- ScriptedAI::JustReachedHome leg is a no-op).
local function understudyReachHome(event, creature)
    if creature:GetBossState(BOSS_RAZUVIOUS) == BOSS_STATE_DONE then
        creature:Despawn()
    end
end

RegisterCreatureEvent(ENTRY_DK_UNDERSTUDY, 1, understudyEnterCombat)
RegisterCreatureEvent(ENTRY_DK_UNDERSTUDY, 7, understudyAIUpdate)
RegisterCreatureEvent(ENTRY_DK_UNDERSTUDY, 24, understudyReachHome)
