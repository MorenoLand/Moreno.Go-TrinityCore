-- Laj (The Botanica, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- botanica/boss_laj.cpp (boss_laj — the only CreatureScript
-- AI class AddSC_boss_laj registers; no SpellScript/
-- AuraScript scripts in this file).
-- Second boss in the Botanica set per outland_script_loader.
-- cpp order (freywinn, laj, warp_splinter, thorngrin_the_
-- tender, commander_sarannis).
-- Entry: 17980 (NPC_LAJ — the_botanica.h, verifiable from
-- the C++ sources; DATA_LAJ = 3 — instance-side constant,
-- unbridgeable). instance_the_botanica.cpp OnCreatureCreate
-- maps NPC_LAJ to LajGUID (verified). The creature_template
-- ScriptName bindings are DB-side (no TDB in this
-- workspace).
-- Talk: EMOTE_SUMMON = 0 (fired from the summon arm,
-- C++-exact). The file defines no SAY_ enums: the C++
-- JustEngagedWith override is empty (no pull yell,
-- C++-exact) and KilledUnit is not overridden (no kill
-- talk, C++-exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pumps); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Laj
-- (17980): OnEnterCombat(1): per-GUID reset to the C++ ctor
-- / Initialize() values (CanSummon = false / Teleport 20000
-- / Summon 2500 / Transform 30000 / Allergic 5000 — the
-- BossAI ctor DATA_LAJ = 3 arm is instance-blocked,
-- unbridgeable) + 1s scheduler pump (a port of UpdateAI in
-- C++ arm order — 1s granularity exact for all C++ timers;
-- the !UpdateVictim early return collapses into the pump).
-- Pump:
-- 1. Summon arm (only while canSummon — set true only by
--    the teleport arm, C++-exact): 2.5s — Talk(EMOTE_SUMMON)
--    + DoSummons + CanSummon = false, re-arm 2500
--    (C++-exact). The eight TRIGGERED summon casts (SPELL_
--    SUMMON_LASHER_1..4 34681/34684/34686/34688 / SPELL_
--    SUMMON_FLAYER_1..4 34682/34685/34690/34687 — the four
--    rand32() % 4 lasher/flayer pairs) have no summon bridge
--    (steamrigger precedent), so the casts are skipped and
--    only the talk and the latch bookkeeping are modeled.
--    The JustSummoned summon->AI()->AttackStart(SelectTarget
--    (Random, 0)) arm is unreachable (no summons exist) —
--    unmodeled.
-- 2. Allergic arm: 5s init then 25000 + rand32() % 15000 —
--    DoCastVictim(SPELL_ALLERGIC_REACTION 34697) non-
--    triggered (C++-exact), re-arm 25000 + rand32() % 15000
--    (C++-exact).
-- 3. Teleport arm: 20s init then 30000 + rand32() % 10000 —
--    DoCast(me, SPELL_TELEPORT_SELF 34673) non-triggered
--    (self-cast convention, C++-exact), CanSummon = true,
--    re-arm 30000 + rand32() % 10000 (C++-exact). Melee is
--    engine-driven.
-- The Transform_Timer / DoTransform() arm is a no-op in
-- this model: rand32() % 5 only SetDisplayId(MODEL_DEFAULT /
-- MODEL_ARCANE / MODEL_FIRE / MODEL_FROST / MODEL_NATURE)
-- and ApplySpellImmune school arms, which have no display /
-- immune bridges — unmodeled (documented below).
-- OnDied(4): cleanup (the C++ has no JustDied override
-- beyond the instance-blocked _JustDied). OnLeaveCombat(2)/
-- OnReset(23): cancel the pump, drop per-GUID state (the
-- C++ Reset() observable remainder — the SetDisplayId /
-- ApplySpellImmune arms have no bridges — lands nowhere;
-- the Initialize() re-latch lands on OnEnterCombat, gargolmar
-- precedent; the _Reset arm is instance-blocked; Eluna's
-- On_Reset fires ahead of OnDied and OnSpawn — millhouse
-- note — so both hooks land the same reset).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_the_botanica.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_LAJ = 3
-- bookkeeping in Reset/JustEngagedWith/JustDied skipped);
-- no summon bridge — the DoSummons TRIGGERED lasher/flayer
-- summon casts and the JustSummoned AttackStart relay
-- unmodeled (steamrigger precedent); no display / immune
-- bridge — the Reset() SetDisplayId(MODEL_DEFAULT) +
-- ApplySpellImmune school arms and the whole DoTransform()
-- (five-model / five-immunity-school rotation) unmodeled;
-- the ScriptData "SD%Complete: 90 / Immunities are wrong,
-- must be adjusted to use resistance from creature_templates.
-- Most spells require database support." caveat is upstream,
-- documented, not bridged; no SpellScript/AuraScript
-- scripts in this file.

local SPELL_ALLERGIC_REACTION = 34697
local SPELL_TELEPORT_SELF = 34673

local EMOTE_SUMMON = 0

local ENTRY_LAJ = 17980

local combatTimers = {}
local combatStates = {}

local function cancelCombatPump(guid)
    local id = combatTimers[guid]
    if id then
        RemoveEventById(id)
        combatTimers[guid] = nil
    end
end

-- C++ Reset() (called on evade): the SetDisplayId /
-- ApplySpellImmune arms have no bridges and _Reset() is
-- instance-blocked; the observable remainder — the
-- Initialize() re-latch — lands on OnEnterCombat
-- (gargolmar precedent).
local function fullReset(guid)
    cancelCombatPump(guid)
    combatStates[guid] = nil
end

-- C++ UpdateAI in arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses
-- into the pump. Only bridgeable arms are modeled (the
-- summon-cast and display/immune arms are bridge-blocked —
-- see deviations).
local function combatTick(creature, guid)
    local st = combatStates[guid]
    if not st then
        return
    end

    -- 1. Summon arm (only while canSummon, C++ gate order —
    -- always first): Talk(EMOTE_SUMMON) + CanSummon = false,
    -- the eight TRIGGERED summon casts skipped (no summon
    -- bridge — steamrigger precedent), re-arm 2500
    -- (C++-exact).
    if st.canSummon then
        if st.summon <= 1000 then
            creature:Talk(EMOTE_SUMMON)
            st.canSummon = false
            st.summon = 2500
        else
            st.summon = st.summon - 1000
        end
    end

    -- 2. Allergic arm: non-triggered DoCastVictim
    -- (C++-exact), re-arm 25000 + rand32() % 15000.
    if st.allergic <= 1000 then
        creature:CastSpell(nil, SPELL_ALLERGIC_REACTION)
        st.allergic = 25000 + math.random(0, 14999)
    else
        st.allergic = st.allergic - 1000
    end

    -- 3. Teleport arm: non-triggered DoCast on self
    -- (C++-exact), CanSummon = true, re-arm 30000 +
    -- rand32() % 10000 (C++-exact).
    if st.teleport <= 1000 then
        creature:CastSpell(creature, SPELL_TELEPORT_SELF)
        st.teleport = 30000 + math.random(0, 9999)
        st.canSummon = true
    else
        st.teleport = st.teleport - 1000
    end

    -- 4. Transform arm: DoTransform() is SetDisplayId +
    -- ApplySpellImmune only (no display / immune bridges) —
    -- a documented no-op, not scheduled.
end

-- C++ ctor / Initialize() values (the whole schedule lands
-- on OnEnterCombat): CanSummon false / Teleport 20000 /
-- Summon 2500 / Transform 30000 / Allergic 5000. The C++
-- JustEngagedWith override is empty — no pull yell
-- (C++-exact).
RegisterCreatureEvent(ENTRY_LAJ, 1, function(_, creature)
    local guid = creature:GetGUID()
    cancelCombatPump(guid)
    combatStates[guid] = {
        canSummon = false,
        teleport = 20000,
        summon = 2500,
        allergic = 5000,
    }
    combatTimers[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end)

-- C++ Reset() (called on evade): fullReset — see above.
RegisterCreatureEvent(ENTRY_LAJ, 2, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- No OnTargetDied(3): the C++ does not override KilledUnit
-- (C++-exact).

RegisterCreatureEvent(ENTRY_LAJ, 4, function(_, creature)
    fullReset(creature:GetGUID())
end)

-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() reset as the evade hook.
RegisterCreatureEvent(ENTRY_LAJ, 23, function(_, creature)
    fullReset(creature:GetGUID())
end)
