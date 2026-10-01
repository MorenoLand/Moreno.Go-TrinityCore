-- Mekgineer Steamrigger (The Steamvault) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/SteamVault/
-- boss_mekgineer_steamrigger.cpp (boss_mekgineer_steamrigger,
-- npc_steamrigger_mechanic — the two AI classes AddSC_boss_
-- mekgineer_steamrigger registers; the mechanic is documented
-- only — its whole AI is instance-gated, below — not
-- registered). Second boss in the Steam Vault set, after
-- Hydromancer Thespia per outland_script_loader.cpp order.
-- Entry 17796 verified from the C++ sources (steam_vault.h:
-- NPC_MEKGINEER_STEAMRIGGER = 17796; instance_steam_vault.cpp
-- maps it to DATA_MEKGINEER_STEAMRIGGER = 1); entry 17951 for
-- the Steamrigger Mechanic is the file's own Creatures enum
-- constant. The creature_template ScriptName bindings are
-- DB-side (no TDB in this workspace).
-- Talk lines used: SAY_MECHANICS=0 (each summon threshold —
-- the Talk is unconditional in C++ even though the summons
-- have no bridge, morogrim precedent), SAY_AGGRO=1 (pull),
-- SAY_SLAY=2 (kill — NO TYPEID gate in C++, C++-exact,
-- morogrim/vashj precedent), SAY_DEATH=3 (death).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent
-- (per-GUID scheduler pump); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelastrasz convention);
-- DoCastVictim takes nil as the victim arm (felmyst convention).
-- Fight shape (C++-exact for the modeled arms):
-- boss_mekgineer_steamrigger (17796): OnEnterCombat(1):
-- per-GUID scheduler reset (shrink 20000 / sawBlade 15000 /
-- electrifiedNet 10000 / summon flags false) + Talk(SAY_AGGRO)
-- + 1s scheduler pump (a port of UpdateAI — 1s granularity is
-- exact for all C++ timers here; the !UpdateVictim early
-- return collapses into the pump, which only runs in combat).
-- Pump: super shrink ray 31485 20s -> DoCastVictim, re-arm
-- 20000. Saw blade 31486 15s -> target = random alive player
-- excluding the victim (C++ SelectTarget(Random, 1), lurker/
-- curator convention), cast on the pick or on the victim when
-- the pick is nil (C++-exact), re-arm 15000. Electrified net
-- 35107 10s -> DoCastVictim, re-arm 10000. Health thresholds
-- (once-guarded GetHealthPct() < N, C++ HealthBelowPct): at
-- 75/50/25 -> Talk(SAY_MECHANICS); the mechanic summons
-- (3x at +/-5 plus two rand32()%2 extra at (5,-7)/(7,-5),
-- TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN 240s) have no summon
-- bridge — unmodeled (below). OnTargetDied(3): Talk(SAY_SLAY)
-- (no TYPEID gate, C++-exact). OnDied(4): Talk(SAY_DEATH) +
-- cleanup (the SetBossState DONE arm is instance-blocked).
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop per-GUID
-- state (the Initialize() reset and the SetBossState NOT_
-- STARTED arm are instance-blocked). Melee is engine-driven.
-- npc_steamrigger_mechanic (17951): documented only, NOT
-- registered — its whole UpdateAI is instance-gated: the
-- repair tick fires only when instance->GetBossState(DATA_
-- MEKGINEER_STEAMRIGGER) == IN_PROGRESS AND the mekgineer is
-- within MAX_REPAIR_RANGE 13.0 yd via instance->GetCreature,
-- then DoCast(me, SPELL_REPAIR 31532, true) when not already
-- channeling, re-arm 5000 (H_SPELL_REPAIR 37936 is defined
-- but never called, gruul precedent); its MoveInLineOfSight
-- is empty ("react only if attacked"), the follow-out-of-
-- range arms are movement-blocked, and melee is engine-driven
-- anyway — so a registration would be an empty skeleton
-- (vashj shield-generator-channel precedent).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (the DATA_MEKGINEER_STEAMRIGGER NOT_STARTED/IN_PROGRESS/DONE
-- bookkeeping in Reset/JustEngagedWith/JustDied skipped; the
-- DATA_ACCESS_PANEL_MEK / main-door sequencing arms live in
-- instance_steam_vault.cpp, which stays blocked on the
-- instance-script model); no summon bridge — the 3-5x
-- mechanic summons at the fixed offsets unmodeled (the C++
-- ScriptData even notes "no known summon spells exist"); no
-- SpellScript/AuraScript scripts in this file.

local SPELL_SUPER_SHRINK_RAY = 31485
local SPELL_SAW_BLADE = 31486
local SPELL_ELECTRIFIED_NET = 35107

local SAY_MECHANICS = 0
local SAY_AGGRO = 1
local SAY_SLAY = 2
local SAY_DEATH = 3

local ENTRY_STEAMRIGGER = 17796

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetBoss(guid)
    cancelPump(guid)
    states[guid] = nil
end

local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ SelectTarget(Random, 1): random alive player in the
-- instance excluding the victim (lurker/curator convention).
-- Nil when no other player is present — the caller falls back
-- to the victim (C++-exact).
local function pickRandomPlayerExcludingVictim(creature)
    local victim = creature:GetVictim()
    local victimGuid = victim and victim:GetGUID() or nil
    local found = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if not victimGuid or p:GetGUID() ~= victimGuid then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ Initialize() values: Shrink_Timer 20000, Saw_Blade_Timer
-- 15000, Electrified_Net_Timer 10000, all summon flags false.
local function freshState()
    return {
        shrink = 20000,
        sawBlade = 15000,
        net = 10000,
        summon75 = false,
        summon50 = false,
        summon25 = false,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Super shrink ray 31485: 20s then 20s, DoCastVictim
    -- (C++-exact).
    if st.shrink <= 1000 then
        creature:CastSpell(nil, SPELL_SUPER_SHRINK_RAY)
        st.shrink = 20000
    else
        st.shrink = st.shrink - 1000
    end

    -- Saw blade 31486: 15s then 15s; random non-victim player
    -- pick, victim fallback when nil (C++-exact).
    if st.sawBlade <= 1000 then
        local target = pickRandomPlayerExcludingVictim(creature)
        if not target then
            target = creature:GetVictim()
        end
        if target then
            creature:CastSpell(target, SPELL_SAW_BLADE)
        end
        st.sawBlade = 15000
    else
        st.sawBlade = st.sawBlade - 1000
    end

    -- Electrified net 35107: 10s then 10s, DoCastVictim
    -- (C++-exact).
    if st.net <= 1000 then
        creature:CastSpell(nil, SPELL_ELECTRIFIED_NET)
        st.net = 10000
    else
        st.net = st.net - 1000
    end

    -- Summon thresholds: once-guarded HealthBelowPct checks in
    -- the C++ arm order; the Talk is unconditional in C++ and
    -- kept even though the mechanic summons have no summon
    -- bridge (morogrim precedent).
    if not st.summon75 and creature:GetHealthPct() < 75 then
        creature:Talk(SAY_MECHANICS)
        st.summon75 = true
    end
    if not st.summon50 and creature:GetHealthPct() < 50 then
        creature:Talk(SAY_MECHANICS)
        st.summon50 = true
    end
    if not st.summon25 and creature:GetHealthPct() < 25 then
        creature:Talk(SAY_MECHANICS)
        st.summon25 = true
    end
end

RegisterCreatureEvent(ENTRY_STEAMRIGGER, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_STEAMRIGGER, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_STEAMRIGGER, 3, function(_, creature)
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_STEAMRIGGER, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_STEAMRIGGER, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
