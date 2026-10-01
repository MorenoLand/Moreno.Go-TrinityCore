-- Hungarfen (The Underbog) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/TheUnderbog/
-- boss_hungarfen.cpp (boss_hungarfen — the only boss AI class
-- AddSC_boss_hungarfen registers; npc_underbog_mushroom, the
-- summoned mushroom, is ported separately in
-- npc_underbog_mushroom.lua; no other creature/GO scripts in
-- this file). First boss in The Underbog set per
-- outland_script_loader.cpp order (the next untouched zone
-- after The Slave Pens 4/4 COMPLETE).
-- Entry 17770 verified externally (wowhead npc=17770/hungarfen,
-- tauri npc=17770, mennu/quagmirran precedent); the_underbog.h
-- defines NO NPC_ entries and instance_the_underbog.cpp is a
-- placeholder with NO OnCreatureCreate mappings at all — the
-- entry is not verifiable from the C++ sources, documented
-- here. The creature_template ScriptName bindings are DB-side
-- (no TDB in this workspace).
-- The file has NO Talk calls (no Say enum at all) and its
-- KilledUnit override is empty — no event 3 registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pump); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention); C++
-- DoCastVictim takes nil as the victim arm (felmyst
-- convention). Fight shape (C++-exact for all modeled arms):
-- Hungarfen (17770): OnEnterCombat(1): per-GUID reset to the
-- C++ Initialize() values (root false / mushroomTimer 5000 /
-- acidGeyserTimer 10000 — the C++ JustEngagedWith override is
-- empty) + 1s scheduler pump (a port of UpdateAI — 1s
-- granularity exact for all C++ timers; the !UpdateVictim
-- early return collapses into the pump). Pump: foul spores
-- 31673 once-guarded when GetHealthPct() <= 20 (C++
-- !HealthAbovePct(20)) — DoCastSelf non-triggered (C++-
-- exact), root=true after the cast. Underbog mushroom 17990:
-- 5s init then 10s — timer bookkeeping only: the whole fire
-- arm is me->SummonCreature(17990, target pos + rand32()%8
-- offsets, TEMPSUMMON_TIMED_DESPAWN 22s) and no summon/
-- position bridges exist (steamrigger mechanic precedent);
-- re-arm 10000 regardless (C++-exact). Acid geyser 38739:
-- 10s init then 10000+rand32()%7500 — target = random alive
-- player in the instance (C++ SelectTarget(Random, 0), thespia
-- precedent), nil pick casts nothing, DoCast(target, 38739)
-- non-triggered (C++-exact), re-arm regardless. OnDied(4):
-- cleanup (no JustDied override in C++). OnLeaveCombat(2)/
-- OnReset(23): cancel the pump, drop per-GUID state (the
-- _Reset arm is instance-blocked).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (instance_the_underbog.cpp is a placeholder and stays
-- blocked on the instance-script model); no summon/position
-- bridges — the mushroom summon machine unmodeled (timer
-- bookkeeping exact; the ScriptData heroic "faster rate" note
-- stays unmodeled — no difficulty bridge, thespia precedent;
-- the ScriptData "spell data same in both modes" uncertainty
-- is documented, not bridged); no SpellScript/AuraScript
-- scripts in this file.

local SPELL_FOUL_SPORES = 31673
local SPELL_ACID_GEYSER = 38739

local ENTRY_HUNGARFEN = 17770

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

-- C++ SelectTarget(Random, 0): random alive player in the
-- instance, no distance gate (thespia precedent). Nil when the
-- instance is empty — the caller skips the cast (C++-exact).
local function pickRandomPlayer(creature)
    local found = playersInInstance(creature)
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ Initialize() values: Root=false, Mushroom_Timer=5000,
-- AcidGeyser_Timer=10000 (the C++ JustEngagedWith is empty).
local function freshState()
    return {
        root = false,
        mushroom = 5000,
        acidGeyser = 10000,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Foul spores 31673: fires once when the boss is at or
    -- below 20% health (C++ !HealthAbovePct(20)) — non-
    -- triggered self-cast, root=true afterwards (C++-exact,
    -- rokmar frenzy once-guard precedent).
    if not st.root and creature:GetHealthPct() <= 20 then
        creature:CastSpell(nil, SPELL_FOUL_SPORES)
        st.root = true
    end

    -- Mushroom 17990: 5s init then 10s — timer bookkeeping
    -- only: the SummonCreature fire arm (target position +
    -- rand32()%8 offsets, TEMPSUMMON_TIMED_DESPAWN 22s) has no
    -- summon/position bridges (steamrigger precedent);
    -- re-arm 10000 regardless (C++-exact).
    if st.mushroom <= 1000 then
        st.mushroom = 10000
    else
        st.mushroom = st.mushroom - 1000
    end

    -- Acid geyser 38739: 10s init then 10000+rand32()%7500 —
    -- random alive player in the instance (thespia precedent),
    -- nil pick casts nothing, non-triggered cast on the target
    -- (C++-exact), re-arm regardless.
    if st.acidGeyser <= 1000 then
        local target = pickRandomPlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_ACID_GEYSER)
        end
        st.acidGeyser = 10000 + math.random(0, 7499)
    else
        st.acidGeyser = st.acidGeyser - 1000
    end
end

RegisterCreatureEvent(ENTRY_HUNGARFEN, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_HUNGARFEN, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_HUNGARFEN, 4, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_HUNGARFEN, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
