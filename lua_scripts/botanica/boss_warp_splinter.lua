-- Warp Splinter (The Botanica, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- botanica/boss_warp_splinter.cpp (boss_warp_splinter and
-- npc_warp_splinter_treant — the two CreatureScript AI
-- classes AddSC_boss_warp_splinter registers; no SpellScript/
-- AuraScript scripts in this file).
-- Third boss in the Botanica set per outland_script_loader.
-- cpp order (freywinn, laj, warp_splinter, thorngrin_the_
-- tender, commander_sarannis).
-- Entry: 17977 (NPC_WARP_SPLINTER — the_botanica.h,
-- verifiable from the C++ sources; DATA_WARP_SPLINTER = 4
-- — instance-side constant, unbridgeable). instance_the_
-- botanica.cpp OnCreatureCreate maps NPC_WARP_SPLINTER to
-- WarpSplinterGUID (verified). The creature_template
-- ScriptName bindings are DB-side (no TDB in this
-- workspace).
-- Talk: SAY_AGGRO = 0 (pull — fired from the C++
-- JustEngagedWith, C++-exact), SAY_SLAY = 1 (kill — the C++
-- KilledUnit talks unconditionally, no player gate,
-- gargolmar precedent, C++-exact), SAY_SUMMON = 2 (summon
-- treants — fired at the end of the C++ SummonTreants(),
-- C++-exact), SAY_DEATH = 3 (death — fired from the C++
-- JustDied, C++-exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via
-- CreateLuaEvent (per-GUID scheduler pumps); melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=
-- false (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Warp
-- Splinter (17977): OnEnterCombat(1): per-GUID reset to the
-- C++ ctor / Initialize() values (War Stomp {25s,40s} /
-- Arcane Volley {8s,20s} / Summon Treants 45s — the BossAI
-- ctor DATA_WARP_SPLINTER = 4 arm is instance-blocked,
-- unbridgeable) + Talk(SAY_AGGRO) + 1s scheduler pump (a
-- port of UpdateAI in C++ arm order — 1s granularity exact
-- for all C++ timers; the !UpdateVictim early return
-- collapses into the pump). Pump:
-- 1. War stomp 34716 — {25s,40s} init then {25s,40s}:
--    non-triggered DoCastVictim (C++-exact), re-arm
--    {25s,40s} regardless (C++-exact).
-- 2. Arcane volley 36705 — {8s,20s} init then {20s,35s}:
--    non-triggered DoCastVictim (C++-exact), re-arm
--    {20s,35s} regardless (C++-exact).
-- 3. Summon treants 34727 — 45s init then 45s: the six
--    me->SummonCreature(CREATURE_TREANT 19949, treant_pos
--    [i]..., TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN, C++ enum 2 ->
--    bridge type 1, 25s) casts
--    are ported (ring O = -GetAbsoluteAngle from the spawn-pos
--    ring point, atan2 twin) + the ENSURE_AI WarpGuid write
--    rides boss ev19 summoned:SetData (soccothrates
--    cross-creature precedent) + Talk(SAY_SUMMON) at the end
--    — re-arm 45s (C++-exact).
--    Melee is engine-driven.
-- OnTargetDied(3): Talk(SAY_SLAY) unconditionally (no
-- player gate, C++-exact). OnDied(4): Talk(SAY_DEATH) +
-- cleanup (the _JustDied arm is instance-blocked). OnLeave
-- Combat(2)/OnReset(23): cancel the pump, drop per-GUID
-- state (the C++ Reset() observable remainder —
-- Initialize() re-latch + me->SetSpeedRate(MOVE_RUN, 0.7f), now ported
-- via creature:SetSpeedRate; the Initialize()
-- re-latch lands on OnEnterCombat, gargolmar precedent;
-- the _Reset arm is instance-blocked; Eluna's On_Reset
-- fires ahead of OnDied and OnSpawn — millhouse note — so
-- both hooks land the same reset).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_the_botanica.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_WARP_SPLINTER
-- = 4 bookkeeping in Reset/JustEngagedWith/JustDied
-- skipped); the treant port + six SummonTreants casts are
-- PORTED (see below) — the old 'no movement / cross-creature /
-- cast-bp0 bridge' claims are stale (MoveFollow :5291,
-- GetCreatureByGUID :4050, CastCustomSpell bp0 :971, Kill-self
-- :3228, SummonCreature :3828 all live); the boss-side
-- SetSpeedRate(MOVE_RUN, 0.7f) IS ported (ev23, bridge live
-- :5641); the Treant_Spawn_Pos_X/Y bookkeeping feeds only the
-- summon orientation O (captured on ev5 spawn, C++ ctor twin);
-- the ScriptData "SD%Complete: 80 / Includes Sapling (need
-- some better control with these)" caveat is upstream,
-- documented, not bridged; no SpellScript/AuraScript
-- scripts in this file.
-- Treant (19949, npc_warp_splinter_treant): C++ UpdateAI —
-- no victim: every 1s (check_Timer 1000, init 0) resolve
-- WarpGuid via GetCreatureByGUID; in melee range of Warp:
-- Warp casts SPELL_HEAL_FATHER 6262 on itself with BP0 =
-- treant current HP (CastCustomSpell bp0; the C++
-- original-caster treant GUID arg is accepted-and-ignored
-- by the bridge, cast still fires) then treant KillSelf;
-- else MoveFollow(Warp, 0, 0). Victim leg: engine melee
-- only when the victim is not Warp (Warp never attacks a
-- treant, so that arm is unreachable in practice).
-- JustEngagedWith is an empty C++ override (no ev1);
-- MoveInLineOfSight is an empty C++ override (ev27
-- return-true veto, soccothrates precedent); Reset
-- re-latches check_Timer = 0 (pump restart on ev23).

local SPELL_WAR_STOMP = 34716
local SPELL_ARCANE_VOLLEY = 36705

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_SUMMON = 2
local SAY_DEATH = 3

local ENTRY_WARP_SPLINTER = 17977
local ENTRY_TREANT = 19949

local TREANT_SPAWN_DIST = 50

-- C++ treant_pos[6][3] (boss_warp_splinter.cpp): the six fixed
-- summon sites. The ring X/Y computed from the spawn pos feed
-- only the orientation O (see the summon arm).
local treantPos = {
    {24.301233, 427.221100, -27.060635},
    {16.795492, 359.678802, -27.355425},
    {53.493484, 345.381470, -26.196192},
    {61.867096, 439.362732, -25.921030},
    {109.861877, 423.201630, -27.356019},
    {106.780159, 355.582581, -27.593357},
}

-- Per-GUID spawn position (C++ Treant_Spawn_Pos_X/Y, captured
-- in the ctor): latched once on ev23 (fires at spawn), never
-- updated — the summon orientation O reads it, nothing else.
local spawnPos = {}
local ENTRY_TREANT = 19949
local SPELL_HEAL_FATHER = 6262
local DATA_WARP_GUID = 1
local TREANT_SPAWN_DIST = 50

-- C++ treant_pos[6][3] (boss_warp_splinter.cpp) — the six
-- summon sites; the ring X/Y (spawn pos + 50y at (pi/3)*i)
-- feed only the orientation O, never the summon position.
local TREANT_POS = {
    {24.301233, 427.221100, -27.060635},
    {16.795492, 359.678802, -27.355425},
    {53.493484, 345.381470, -26.196192},
    {61.867096, 439.362732, -25.921030},
    {109.861877, 423.201630, -27.356019},
    {106.780159, 355.582581, -27.593357},
}

local combatTimers = {}
local combatStates = {}
-- C++ ctor Treant_Spawn_Pos_X/Y twin, captured on spawn.
local spawnPos = {}
local treantTimers = {}

local function cancelCombatPump(guid)
    local id = combatTimers[guid]
    if id then
        RemoveEventById(id)
        combatTimers[guid] = nil
    end
end

-- C++ Reset() (called on evade): the observable remainder
-- — the Initialize() re-latch and the SetSpeedRate(MOVE_
-- RUN, 0.7f) arm, ported via creature:SetSpeedRate — lands on
-- OnEnterCombat (re-latch, gargolmar precedent) and OnReset(23).
local function fullReset(guid)
    cancelCombatPump(guid)
    combatStates[guid] = nil
end

-- C++ SummonTreants() (boss_warp_splinter.cpp): six
-- SummonCreature(CREATURE_TREANT, treant_pos[i], O =
-- -GetAbsoluteAngle(ring point), TEMPSUMMON_TIMED_OR_CORPSE_
-- DESPAWN, C++ enum 2 -> bridge type 1, 25s) + Talk(SAY_SUMMON) at the end. The
-- WarpGuid write lands in the boss ev19 handler below.
local function summonTreants(creature)
    local guid = creature:GetGUIDLow()
    local sp = spawnPos[guid]
    local bx, by = creature:GetX(), creature:GetY()
    local ox, oy = bx, by
    if sp then
        ox, oy = sp.x, sp.y
    end
    for i = 1, 6 do
        local angle = (math.pi / 3) * (i - 1)
        local x = ox + TREANT_SPAWN_DIST * math.cos(angle)
        local y = oy + TREANT_SPAWN_DIST * math.sin(angle)
        local face = math.atan2(y - by, x - bx)
        if face < 0 then
            face = face + 2 * math.pi
        end
        local pos = TREANT_POS[i]
        -- TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN: C++ enum 2 ->
        -- bridge type 1 (the bridge runs the 0-based Eluna
        -- scheme; sarannis/malchezaar/moroes precedent).
        creature:SummonCreature(ENTRY_TREANT, pos[1], pos[2], pos[3], -face, 1, 25000)
    end
    creature:Talk(SAY_SUMMON)
end

-- C++ UpdateAI in arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses
-- into the pump.
local function combatTick(creature, guid)
    local st = combatStates[guid]
    if not st then
        return
    end

    -- 1. War stomp arm: non-triggered DoCastVictim
    -- (C++-exact), re-arm {25s,40s} regardless.
    if st.warStomp <= 1000 then
        creature:CastSpell(nil, SPELL_WAR_STOMP)
        st.warStomp = math.random(25000, 40000)
    else
        st.warStomp = st.warStomp - 1000
    end

    -- 2. Arcane volley arm: non-triggered DoCastVictim
    -- (C++-exact), re-arm {20s,35s} regardless.
    if st.arcaneVolley <= 1000 then
        creature:CastSpell(nil, SPELL_ARCANE_VOLLEY)
        st.arcaneVolley = math.random(20000, 35000)
    else
        st.arcaneVolley = st.arcaneVolley - 1000
    end

    -- 3. Summon treants arm: the six SummonCreature casts +
    -- Talk(SAY_SUMMON) (C++-exact), re-arm 45s.
    if st.summonTreants <= 1000 then
        summonTreants(creature)
        st.summonTreants = 45000
    else
        st.summonTreants = st.summonTreants - 1000
    end
end

-- C++ ctor twin: capture the spawn position (feeds only
-- the SummonTreants ring orientation O).
RegisterCreatureEvent(ENTRY_WARP_SPLINTER, 5, function(_, creature)
    local guid = creature:GetGUIDLow()
    spawnPos[guid] = { x = creature:GetX(), y = creature:GetY() }
end)

-- C++ ctor / Initialize() values (the whole schedule lands
-- on OnEnterCombat): War Stomp {25s,40s} / Arcane Volley
-- {8s,20s} / Summon Treants 45s. The C++ JustEngagedWith
-- only talks (the BossAI arm is instance-blocked) —
-- Talk(SAY_AGGRO) (C++-exact).
RegisterCreatureEvent(ENTRY_WARP_SPLINTER, 1, function(_, creature)
    local guid = creature:GetGUIDLow()
    cancelCombatPump(guid)
    combatStates[guid] = {
        warStomp = math.random(25000, 40000),
        arcaneVolley = math.random(8000, 20000),
        summonTreants = 45000,
    }
    creature:Talk(SAY_AGGRO)
    combatTimers[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end)

-- C++ Reset() (called on evade): fullReset — see above.
RegisterCreatureEvent(ENTRY_WARP_SPLINTER, 2, function(_, creature)
    fullReset(creature:GetGUIDLow())
end)

-- C++ KilledUnit: Talk(SAY_SLAY) unconditionally (no player
-- gate, C++-exact).
RegisterCreatureEvent(ENTRY_WARP_SPLINTER, 3, function(_, creature)
    creature:Talk(SAY_SLAY)
end)

-- C++ JustDied: Talk(SAY_DEATH) + cleanup (the _JustDied
-- arm is instance-blocked).
RegisterCreatureEvent(ENTRY_WARP_SPLINTER, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    fullReset(creature:GetGUIDLow())
end)

-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() reset as the evade hook.
RegisterCreatureEvent(ENTRY_WARP_SPLINTER, 23, function(_, creature)
    fullReset(creature:GetGUIDLow())
    -- C++ Reset() :171: SetSpeedRate(MOVE_RUN, 0.7f) (boss_warp_splinter.
    -- cpp).
    creature:SetSpeedRate(1, 0.7)
end)

-- C++ SummonTreants() ENSURE_AI(treantAI)->WarpGuid =
-- me->GetGUID() twin: boss ev19 summoned:SetData (the
-- soccothrates cross-creature precedent); the treant reads
-- it back with GetData(DATA_WARP_GUID) + GetCreatureByGUID.
RegisterCreatureEvent(ENTRY_WARP_SPLINTER, 19, function(_, creature, summoned)
    if summoned:GetEntry() == ENTRY_TREANT then
        summoned:SetData(DATA_WARP_GUID, creature:GetGUIDLow())
    end
end)

-------------------------------------------------------------------
--- Treant (19949, npc_warp_splinter_treant).
-------------------------------------------------------------------

local function cancelTreantPump(guid)
    local id = treantTimers[guid]
    if id then
        RemoveEventById(id)
        treantTimers[guid] = nil
    end
end

-- C++ npc_warp_splinter_treantAI::UpdateAI, 1s granularity
-- (check_Timer 1000, init 0): no victim -> resolve WarpGuid,
-- heal-and-KillSelf in melee range, else MoveFollow(Warp).
-- Victim leg is engine melee (C++ DoMeleeAttackIfReady when
-- the victim is not Warp).
local function treantTick(creature, guid)
    if creature:GetVictim() ~= nil then
        return
    end
    local warpGuid = creature:GetData(DATA_WARP_GUID)
    if warpGuid == 0 then
        return
    end
    local warp = creature:GetCreatureByGUID(warpGuid)
    if warp == nil then
        return
    end
    if creature:IsWithinMeleeRange(warp) then
        warp:CastCustomSpell(warp, SPELL_HEAL_FATHER, false, creature:GetHealth())
        creature:Kill()
        return
    end
    creature:MoveFollow(warp, 0, 0)
end

local function startTreantPump(creature)
    local guid = creature:GetGUIDLow()
    cancelTreantPump(guid)
    treantTimers[guid] = CreateLuaEvent(function()
        treantTick(creature, guid)
    end, 1000, 0)
end

-- C++ Initialize(): check_Timer = 0 (pump starts on spawn).
RegisterCreatureEvent(ENTRY_TREANT, 5, function(_, creature)
    startTreantPump(creature)
end)

-- C++ Reset(): Initialize() re-latch (pump restart).
RegisterCreatureEvent(ENTRY_TREANT, 23, function(_, creature)
    startTreantPump(creature)
end)

-- Pump cleanup on leave-combat / death.
RegisterCreatureEvent(ENTRY_TREANT, 2, function(_, creature)
    cancelTreantPump(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_TREANT, 4, function(_, creature)
    cancelTreantPump(creature:GetGUIDLow())
end)

-- C++ MoveInLineOfSight is an empty override (never calls
-- base): ev27 return-true veto, soccothrates precedent.
RegisterCreatureEvent(ENTRY_TREANT, 27, function(_, creature, unit)
    return true
end)
