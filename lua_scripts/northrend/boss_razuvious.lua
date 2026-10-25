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
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
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
-- BossAI::JustEngagedWith instance-bookkeeping leg, me->StopMoving,
-- summons.DoZoneInCombat (stays out — summons-container call, not a
-- plain me->DoZoneInCombat() JustEngagedWith arm; tharon_ja precedent
-- — 2026-10-23 re-evaluated against the live bridge);
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
-- OnDied-cast precedent; the RemoveCharmAuras / events.Reset /
-- instance->SetBossState bookkeeping has no bridge — tharon_ja
-- precedent). All timers cancelled on 2/4/23 (gargolmar precedent).
-- No summon-arm Talk orphaning: EVENT_KNIFE's target leg rides the
-- GetAITargets threat-list pool + 45yd filter (knifeTick — the old
-- "unbridged SelectTarget" claim was stale).
-- Unmodeled (no bridges — documented, not wired):
-- boss_razuvious SpellHit — SPELL_UNDERSTUDY_TAUNT 29060 ->
-- Talk(SAY_TAUNTED 2, caster) stays out: the MC'd understudy's taunt
-- routes through the pet cast path (handlePetCastSpell /
-- executePetSpellWithOptions, pets.go), which has no event-14 fire
-- site — only the creature-cast path (boss_ai.go:1885) and the
-- player-cast path (spells.go:9427) fire it (re-audit 2026-10-15:
-- the 22:34 "trigger live in Go" claim was wrong);
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
-- SUMMON_GROUP_10MAN 1) + 25-man SUMMON_GROUP_25MAN 2 — no summon
-- bridge (standing; the old "no difficulty bridge" parenthetical was
-- stale — IsHeroic() is bridged, lua_creature_events.go:260).
-- npc_dk_understudy (ScriptedAI, 16803 — joins the
-- entry-verifiable-but-bridge-blocked queue; ScriptedAI has no
-- RegisterLuaBoss-style surface): LoadEquipment(1) — no equipment
-- bridge; JustEngagedWith — SetUInt32Value(UNIT_NPC_EMOTESTATE,
-- EMOTE_ONESHOT_NONE) + cross-AI DoZoneInCombat on razuvious via
-- _instance->GetGuidData(DATA_RAZUVIOUS) — stays out: 2026-10-23
-- re-evaluation against the live bridge — a DIFFERENT creature's arm
-- (razuvious, not the understudy) riding the unbridged instance
-- GetGuidData leg; the understudy has no event-1 registration in Lua
-- to host it, so there is no PORT site either; JustReachedHome — DespawnOrUnsummon when BOSS_RAZUVIOUS ==
-- DONE — instance model; UpdateAI — bloodStrikeTimer DoCastVictim(
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
local SAY_DEATH = 3

local SPELL_UNBALANCING_STRIKE = 26613
local SPELL_DISRUPTING_SHOUT = 29107 -- boss_razuvious.cpp:37
local SPELL_JAGGED_KNIFE = 55550 -- boss_razuvious.cpp:38
local SPELL_HOPELESS = 29125 -- boss_razuvious.cpp:39

local timers = {}

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
    creature:SetCombatMovement(false) -- Reset arm, boss_razuvious.cpp:169 (ranged AI stands and casts)
    creature:StopMoving() -- boss_razuvious.cpp:123 (the summons.DoZoneInCombat
    -- sibling at :124 stays out — summons-container call, 2026-10-23 re-eval)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
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
    cancelTimers(creature:GetGUID())
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

local function razyviousDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    -- C++ JustDied (:108-118): Talk(SAY_DEATH) then DoCastAOE(Hopeless
    -- 29125, triggered) — the amanitar precedent (Go carries no
    -- alive-gate, so the OnDied cast is faithful). The summons
    -- RemoveCharmAuras + instance->SetBossState bookkeeping has no
    -- bridge (tharon_ja precedent).
    creature:CastSpell(creature, SPELL_HOPELESS, true)
    cancelTimers(creature:GetGUID())
end

local function razyviousReset(event, creature)
    -- C++ Reset (boss_razuvious.cpp:168-171): SetCombatMovement(false)
    -- + _Reset bookkeeping (tharon_ja precedent for the latter).
    creature:SetCombatMovement(false)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_RAZUVIOUS, 1, razyviousEnterCombat)
RegisterCreatureEvent(ENTRY_RAZUVIOUS, 2, razyviousLeaveCombat)
RegisterCreatureEvent(ENTRY_RAZUVIOUS, 3, razyviousTargetDied)
RegisterCreatureEvent(ENTRY_RAZUVIOUS, 4, razyviousDied)
RegisterCreatureEvent(ENTRY_RAZUVIOUS, 23, razyviousReset)

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

RegisterCreatureEvent(ENTRY_DK_UNDERSTUDY, 7, understudyAIUpdate)
