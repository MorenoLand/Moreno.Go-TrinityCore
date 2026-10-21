-- Temple of Ahn'Qiraj: Ouro — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_ouro.cpp
-- (boss_ouroAI : public BossAI(creature, DATA_OURO);
-- GetAI via GetAQ40AI<boss_ouroAI>(creature) (AQ40ScriptName
-- "instance_temple_of_ahnqiraj" gate); kalimdor loader decl 95 /
-- call per kalimdor_script_loader.cpp — ninth "// Temple of
-- ahn'qiraj" loader group, after twinemperors, before
-- npc_anubisath_sentinel).
-- Entry: Ouro = 15517 (TrinityCore 3.3.5 creature_template; no
-- NPC_OURO constant in temple_of_ahnqiraj.h — the
-- creature_template ScriptName binding stays DB-side, loader
-- convention). DATA_OURO = 8 (temple_of_ahnqiraj.h:38).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Zero Talk() calls in C++.
-- The Submerged state is latched in Lua: while submerged the
-- Sweep/SandBlast legs are suppressed (C++ !Submerged gates)
-- and the Back leg re-arms the Submerge cycle
-- (urand(60000,120000), C++-verbatim).
-- Ported arms (the self-contained in-combat legs):
-- - Birth 26262 (JustEngagedWith DoCastVictim): victim cast on
--   engage (fankriss GetVictim convention).
-- - Sweep 26103 (DoCastVictim, !Submerged): init
--   urand(5000,10000) -> urand(15000,30000).
-- - Sand Blast 26102 (DoCastVictim, !Submerged): init
--   urand(20000,35000) -> urand(20000,35000).
-- - Submerge (urand(90000,150000) init -> urand(60000,120000)
--   repeat): self-cast Dirt Mound Passive 26092 as the submerge
--   visual. The EMOTE_ONESHOT_SUBMERGE / SetFlag(UNIT_FLAG_NOT_SELECTABLE)
--   / SetFaction(FACTION_FRIENDLY) legs port via creature:PerformEmote(374),
--   creature:SetFlag(59, ...) and creature:SetFaction(...) — the stale
--   "no SetFlag/SetFaction bridge" claim was retracted 2026-10-21
--   (both bridged on the handler surface; stalagg/feugen precedent).
-- - Back from submerge (urand(30000,45000) after submerge):
--   RemoveFlag/SetFaction(FACTION_MONSTER) port via
--   creature:RemoveFlag(59, ...) and creature:SetFaction(...) before
--   the victim Ground Rupture 26100 (DoCastVictim, C++-verbatim) +
--   submerged flag cleared + Submerge re-armed.
-- Unmodeled (documented-only, no bridges):
-- - ChangeTarget while submerged (urand(5000,8000) init ->
--   urand(10000,20000)): SelectTarget Random + DoTeleportTo —
--   SelectTarget/DoTeleportTo have zero Lua usage, documented-only.
-- - Spawn_Timer (urand(10000,20000) init): initialized in
--   Initialize() but NEVER referenced in UpdateAI — dead in C++.
-- - Enrage bool: set false in Initialize(), never read anywhere
--   in the file — dead in C++ (the SD%Complete:85 note's missing
--   enrage; no Ouro enrage spell exists in this file).
-- - BossAI ctor leg DATA_OURO and _Reset() — instance bridge,
--   standing.
-- - UpdateVictim gating + DoMeleeAttackIfReady — engine-driven
--   in Go, standing.
local ENTRY = 15517

local SPELL_SWEEP = 26103
local SPELL_SANDBLAST = 26102
local SPELL_GROUND_RUPTURE = 26100
local SPELL_BIRTH = 26262
local SPELL_DIRTMOUND_PASSIVE = 26092
-- UnitDefines.h:149; SharedDefines.h:226/231.
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local FACTION_MONSTER = 14
local FACTION_FRIENDLY = 35

local timers = {}
local flags = {}

local onSubmerge

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    flags[guid] = nil
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

local function st(guid)
    local s = flags[guid]
    if not s then
        s = { submerged = false }
        flags[guid] = s
    end
    return s
end

-- C++ Sweep_Timer (!Submerged): DoCastVictim(SPELL_SWEEP),
-- init urand(5000,10000) -> urand(15000,30000).
local function onSweep(creature, guid)
    local s = st(guid)
    if not s.submerged then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_SWEEP)
        end
        schedule(guid, "sweep", math.random(15000, 30000), function()
            onSweep(creature, guid)
        end)
    else
        -- C++ keeps decrementing Sweep_Timer while submerged; an
        -- expiry wraps the uint32 and kills the arm for the rest of
        -- combat (boss_ouro.cpp UpdateAI). Mirror by dropping the key.
        local per = timers[guid]
        if per then
            per["sweep"] = nil
        end
    end
end

-- C++ SandBlast_Timer (!Submerged): DoCastVictim(SPELL_SANDBLAST),
-- init urand(20000,35000) -> urand(20000,35000).
local function onSandBlast(creature, guid)
    local s = st(guid)
    if not s.submerged then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_SANDBLAST)
        end
        schedule(guid, "sandblast", math.random(20000, 35000), function()
            onSandBlast(creature, guid)
        end)
    else
        -- Same uint32-underflow kill as Sweep_Timer (boss_ouro.cpp
        -- UpdateAI): drop the key, no re-arm.
        local per = timers[guid]
        if per then
            per["sandblast"] = nil
        end
    end
end

-- C++ Back_Timer (Submerged): RemoveFlag/SetFaction legs port via
-- creature:RemoveFlag(59, ...) and creature:SetFaction(...) ahead of
-- the DoCastVictim Ground Rupture 26100 + submerged cleared +
-- Submerge re-armed to urand(60000,120000), C++-verbatim.
local function onBack(creature, guid)
    local s = st(guid)
    creature:RemoveFlag(59, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetFaction(FACTION_MONSTER)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_GROUND_RUPTURE)
    end
    -- C++ boss_ouro.cpp:137-143 order: RemoveFlag, SetFaction, cast,
    -- then Submerged=false; the Lua latch follows the same order.
    s.submerged = false
    schedule(guid, "submerge", math.random(60000, 120000), function()
        onSubmerge(creature, guid)
    end)
end

-- C++ Submerge_Timer (!Submerged): emote + SetFlag(UNIT_FLAG_NOT_SELECTABLE)
-- + SetFaction(FACTION_FRIENDLY) port via creature:PerformEmote(374),
-- creature:SetFlag(59, ...) and creature:SetFaction(...); the Dirt Mound
-- Passive 26092 self-cast is the ported submerge visual. Latched in
-- st(guid); Back_Timer = urand(30000,45000), C++-verbatim.
onSubmerge = function(creature, guid)
    local s = st(guid)
    -- C++ boss_ouro.cpp:114:
    -- me->HandleEmoteCommand(EMOTE_ONESHOT_SUBMERGE) (SharedDefines.h:2012,
    -- value 374), then the flag/faction legs (C++-verbatim order),
    -- before the Dirt Mound Passive cast; Submerged=true follows.
    creature:PerformEmote(374)
    creature:SetFlag(59, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetFaction(FACTION_FRIENDLY)
    creature:CastSpell(creature, SPELL_DIRTMOUND_PASSIVE)
    s.submerged = true
    schedule(guid, "back", math.random(30000, 45000), function()
        onBack(creature, guid)
    end)
end

-- C++ JustEngagedWith: DoCastVictim(SPELL_BIRTH) then
-- BossAI::JustEngagedWith (instance leg, standing).
local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    st(guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BIRTH)
    end
    schedule(guid, "sweep", math.random(5000, 10000), function()
        onSweep(creature, guid)
    end)
    schedule(guid, "sandblast", math.random(20000, 35000), function()
        onSandBlast(creature, guid)
    end)
    schedule(guid, "submerge", math.random(90000, 150000), function()
        onSubmerge(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ has no JustDied arm, but a dead creature stops UpdateAI, so the
-- re-arming sweep / sandblast / submerge / back chain must be cancelled
-- on death or it keeps casting from the corpse forever (the submerged
-- latch clears in cancelTimers too).
local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
