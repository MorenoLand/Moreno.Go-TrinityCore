-- Ragnaros (Molten Core) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/MoltenCore/
-- boss_ragnaros.cpp (boss_ragnarosAI only; npc_son_of_flame documented
-- below, not registered); molten_core.h:39 (BOSS_RAGNAROS = 9, tenth
-- and final Molten Core boss), :63 (NPC_RAGNAROS = 11502).
-- Creature entry: 11502 Ragnaros (C++ ScriptName "boss_ragnaros" per
-- AddSC_boss_ragnaros). Talk lines used: SAY_KILL=9 (25% on kill),
-- SAY_WRATH=8 (50% on wrath of ragnaros), SAY_HAND=7 (50% on hand of
-- ragnaros), SAY_MAGMABURST=10 (once, first magma blast), SAY_
-- REINFORCEMENTS1=5 (first submerge), SAY_REINFORCEMENTS2=6 (later
-- submerges); SAY_SUMMON_MAJ=0, SAY_ARRIVAL2_MAJ=2, SAY_ARRIVAL1_RAG=
-- 1, SAY_ARRIVAL3_RAG=3 and SAY_ARRIVAL5_RAG=4 belong to the intro /
-- majordomo-summon sequence and are unmodeled here (see deviations).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat: arm
-- eruption 17731 15s then {20s,45s} non-triggered DoCastVictim /
-- wrath of ragnaros 20566 30s then 25s non-triggered DoCastVictim
-- (50% Talk(SAY_WRATH)) / hand of ragnaros 19780 25s then 20s
-- non-triggered self-cast (50% Talk(SAY_HAND)) / lava burst 21158
-- 10s then 10s non-triggered DoCastVictim / elemental fire 20564 3s
-- then {10s,14s} non-triggered DoCastVictim / magma blast 20565 2s
-- then 2500ms non-triggered DoCastVictim (the melee-range gate has
-- no bridge — the cast fires unconditionally and the once-only
-- Talk(SAY_MAGMABURST) fires on the first tick; see deviations) /
-- submerge 3min then 3min (see below). Nil-victim ticks cast nothing
-- but keep the schedule (jeklik convention). OnTargetDied(3): 25%
-- Talk(SAY_KILL). OnDied/OnLeaveCombat/OnReset: cancel timers and
-- clear per-GUID state (C++ Reset re-initializes the AI state).
-- The submerge arm (C++-exact for the modeled arms): Talk(SAY_
-- REINFORCEMENTS1) the first time and summon-mark state, Talk(SAY_
-- REINFORCEMENTS2) on later submerges; _isBanished is set and the
-- 90s _emergeTimer armed — while banished the per-tick handlers
-- cast nothing but keep the schedule (C++ returns early from
-- UpdateAI while banished, freezing the event queue; the port keeps
-- the 90s emerge arm). Emerge: flip _isBanished back (the C++ Talk/
-- faction/emote/flag/AttackStart arms have no bridges — see
-- deviations). The 8-son-of-flame summons and the DATA_RAGNAROS_ADDS
-- counter arms have no bridges — they are never spawned, so the
-- C++ early-emerge condition (adds > 8) can never fire and emerge
-- happens on the 90s timer only.
-- The npc_son_of_flame AI in the same C++ file (ScriptName
-- "npc_SonOfFlame", entry 12143) is not registered: no
-- NPC_SON_OF_FLAME constant exists in the C++ tree (the summon uses
-- a hardcoded 12143) and the creature_template ScriptName binding
-- is DB-side (garr firesworn convention). Its JustDied
-- DATA_RAGNAROS_ADDS increment is instance-side and blocked on the
-- instance-script model.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate
-- and the post-event casting check have no UNIT_STATE bridge —
-- timers fire unconditionally (jeklik convention); no instance-
-- script model — boss admission via the luaBossAI shim
-- (BossAI::JustEngagedWith and the BOSS_RAGNAROS bookkeeping arms
-- skipped), and the DATA_RAGNAROS_ADDS SetData/GetData arms and the
-- majordomo-executus kill arm are unmodeled; the intro sequence
-- (emerge emote, intro Talks SAY_ARRIVAL1_RAG/SAY_ARRIVAL3_RAG/
-- SAY_ARRIVAL5_RAG, REACT_PASSIVE/AGGRESSIVE, UNIT_FLAG_NON_
-- ATTACKABLE, SetImmuneToPC, the ObjectAccessor majordomo kill) has
-- no react/emote/flag/immune/creature-lookup bridges — the fight
-- starts on pull instead of after the 53s intro; the submerge
-- AttackStop, ResetThreatList, InterruptNonMeleeSpells, faction
-- change, NOT_SELECTABLE flag, SUBMERGED emotestate and submerge
-- emote arms have no bridges; the emerge react/faction/flag/emote/
-- AttackStart arms have no bridges; no summon model — the 8 sons of
-- flame never spawn; the magma blast IsWithinMeleeRange gate has no
-- bridge; SPELL_RAGSUBMERGE 21107 / SPELL_RAGEMERGE 20568 /
-- SPELL_SONS_OF_FLAME_DUMMY 21108 are commented out / server-side in
-- C++ and unused; SPELL_MELT_WEAPON 21388 is declared but never
-- scheduled by the AI.

local ENTRY_RAGNAROS = 11502

local SAY_REINFORCEMENTS1 = 5
local SAY_REINFORCEMENTS2 = 6
local SAY_HAND = 7
local SAY_WRATH = 8
local SAY_KILL = 9
local SAY_MAGMABURST = 10

local SPELL_ERRUPTION = 17731
local SPELL_HAND_OF_RAGNAROS = 19780
local SPELL_MAGMA_BLAST = 20565
local SPELL_WRATH_OF_RAGNAROS = 20566
local SPELL_ELEMENTAL_FIRE = 20564
local SPELL_LAVA_BURST = 21158

local timers = {}
local state = {}

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

local function getState(guid)
    local s = state[guid]
    if not s then
        s = { hasYelledMagmaBurst = false, hasSubmergedOnce = false,
              isBanished = false }
        state[guid] = s
    end
    return s
end

local function isBanished(guid)
    local s = state[guid]
    return s and s.isBanished
end

-- C++ EVENT_ERUPTION: non-triggered DoCastVictim(17731); re-arm
-- {20s,45s}.
local function onEruption(creature, guid)
    if not isBanished(guid) then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_ERRUPTION)
        end
    end
    schedule(guid, "eruption", 20000 + math.random(0, 25000), function()
        onEruption(creature, guid)
    end)
end

-- C++ EVENT_WRATH_OF_RAGNAROS: non-triggered DoCastVictim(20566);
-- 50% Talk(SAY_WRATH); re-arm 25s.
local function onWrath(creature, guid)
    if not isBanished(guid) then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_WRATH_OF_RAGNAROS)
            if math.random(0, 1) == 1 then
                creature:Talk(SAY_WRATH)
            end
        end
    end
    schedule(guid, "wrath", 25000, function()
        onWrath(creature, guid)
    end)
end

-- C++ EVENT_HAND_OF_RAGNAROS: non-triggered DoCast(me, 19780);
-- 50% Talk(SAY_HAND); re-arm 20s.
local function onHand(creature, guid)
    if not isBanished(guid) then
        creature:CastSpell(creature, SPELL_HAND_OF_RAGNAROS)
        if math.random(0, 1) == 1 then
            creature:Talk(SAY_HAND)
        end
    end
    schedule(guid, "hand", 20000, function()
        onHand(creature, guid)
    end)
end

-- C++ EVENT_LAVA_BURST: non-triggered DoCastVictim(21158); re-arm
-- 10s.
local function onLavaBurst(creature, guid)
    if not isBanished(guid) then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_LAVA_BURST)
        end
    end
    schedule(guid, "lavaburst", 10000, function()
        onLavaBurst(creature, guid)
    end)
end

-- C++ EVENT_ELEMENTAL_FIRE: non-triggered DoCastVictim(20564);
-- re-arm {10s,14s}.
local function onElementalFire(creature, guid)
    if not isBanished(guid) then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_ELEMENTAL_FIRE)
        end
    end
    schedule(guid, "elementalfire", 10000 + math.random(0, 4000), function()
        onElementalFire(creature, guid)
    end)
end

-- C++ EVENT_MAGMA_BLAST: if the victim is not within melee range,
-- non-triggered DoCastVictim(20565); once-only Talk(SAY_MAGMABURST);
-- re-arm 2500ms. The IsWithinMeleeRange gate has no bridge — the
-- cast fires unconditionally and the Talk fires on the first tick.
local function onMagmaBlast(creature, guid)
    local s = getState(guid)
    if not s.isBanished then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_MAGMA_BLAST)
            if not s.hasYelledMagmaBurst then
                creature:Talk(SAY_MAGMABURST)
                s.hasYelledMagmaBurst = true
            end
        end
    end
    schedule(guid, "magmablast", 2500, function()
        onMagmaBlast(creature, guid)
    end)
end

-- C++ UpdateAI banished arm: _emergeTimer 90s -> flip _isBanished
-- back. The C++ faction/emote/flag/AttackStart arms have no bridges;
-- the sons-of-flame counter never advances (no summon bridge), so
-- the >8-adds early-emerge arm is unmodeled.
local function onEmerge(creature, guid)
    local s = getState(guid)
    s.isBanished = false
end

-- C++ EVENT_SUBMERGE (only when not already banished): Talk(SAY_
-- REINFORCEMENTS1) the first time / SAY_REINFORCEMENTS2 later;
-- _isBanished set, _emergeTimer 90s armed; re-arm 3min. The summon
-- (8 sons of flame on random targets), threat reset, faction/flag/
-- emote/react/immobilize arms have no bridges.
local function onSubmerge(creature, guid)
    local s = getState(guid)
    if not s.isBanished then
        if not s.hasSubmergedOnce then
            creature:Talk(SAY_REINFORCEMENTS1)
            s.hasSubmergedOnce = true
        else
            creature:Talk(SAY_REINFORCEMENTS2)
        end
        s.isBanished = true
        schedule(guid, "emerge", 90000, function()
            onEmerge(creature, guid)
        end)
    end
    schedule(guid, "submerge", 180000, function()
        onSubmerge(creature, guid)
    end)
end

local function ragnarosEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    state[guid] = { hasYelledMagmaBurst = false, hasSubmergedOnce = false,
                    isBanished = false }
    schedule(guid, "eruption", 15000, function()
        onEruption(creature, guid)
    end)
    schedule(guid, "wrath", 30000, function()
        onWrath(creature, guid)
    end)
    schedule(guid, "hand", 25000, function()
        onHand(creature, guid)
    end)
    schedule(guid, "lavaburst", 10000, function()
        onLavaBurst(creature, guid)
    end)
    schedule(guid, "elementalfire", 3000, function()
        onElementalFire(creature, guid)
    end)
    schedule(guid, "magmablast", 2000, function()
        onMagmaBlast(creature, guid)
    end)
    schedule(guid, "submerge", 180000, function()
        onSubmerge(creature, guid)
    end)
end

-- C++ KilledUnit: 25% Talk(SAY_KILL) (urand(0, 99) < 25, C++-exact).
local function ragnarosTargetDied(event, creature, victim)
    if math.random(0, 99) < 25 then
        creature:Talk(SAY_KILL)
    end
end

local function ragnarosResetState(guid)
    cancelTimers(guid)
    state[guid] = nil
end

local function ragnarosLeaveCombat(event, creature)
    ragnarosResetState(creature:GetGUID())
end

local function ragnarosDied(event, creature, killer)
    ragnarosResetState(creature:GetGUID())
end

local function ragnarosReset(event, creature)
    ragnarosResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_RAGNAROS, 1, ragnarosEnterCombat)
RegisterCreatureEvent(ENTRY_RAGNAROS, 2, ragnarosLeaveCombat)
RegisterCreatureEvent(ENTRY_RAGNAROS, 3, ragnarosTargetDied)
RegisterCreatureEvent(ENTRY_RAGNAROS, 4, ragnarosDied)
RegisterCreatureEvent(ENTRY_RAGNAROS, 23, ragnarosReset)
