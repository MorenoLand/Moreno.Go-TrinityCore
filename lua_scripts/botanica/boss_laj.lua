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
--    rand32() % 4 lasher/flayer pairs) summon via spell data (the
--    summoned entry IDs are not in the C++ sources, unverifiable —
--    sarannis "can't see inside a spell script" precedent), so the
--    casts are skipped and only the talk and the latch bookkeeping
--    are modeled. The JustSummoned summon->AI()->AttackStart(
--    SelectTarget(Random, 0)) arm is unreachable (no summons exist)
--    and has no AttackStart bridge — unmodeled.
-- 2. Allergic arm: 5s init then 25000 + rand32() % 15000 —
--    DoCastVictim(SPELL_ALLERGIC_REACTION 34697) non-
--    triggered (C++-exact), re-arm 25000 + rand32() % 15000
--    (C++-exact).
-- 3. Teleport arm: 20s init then 30000 + rand32() % 10000 —
--    DoCast(me, SPELL_TELEPORT_SELF 34673) non-triggered
--    (self-cast convention, C++-exact), CanSummon = true,
--    re-arm 30000 + rand32() % 10000 (C++-exact). Melee is
--    engine-driven.
-- The Transform_Timer / DoTransform() arm is PORTED 2026-10-20
-- (display bridge live): rand32() % 5 -> SetDisplayId(MODEL_DEFAULT
-- 13109 / MODEL_ARCANE 14213 / MODEL_FIRE 13110 / MODEL_FROST 14112 /
-- MODEL_NATURE 14214) + the per-case ApplySpellImmune school matrix
-- (C++ case order), 30s init then 25s + rand32() % 15000 re-arm
-- (C++-exact).
-- OnDied(4): cleanup (the C++ has no JustDied override
-- beyond the instance-blocked _JustDied). OnLeaveCombat(2)/
-- OnReset(23): cancel the pump, drop per-GUID state (the
-- C++ Reset() observable remainder — the SetDisplayId(MODEL_DEFAULT)
-- arm is PORTED 2026-10-20 and the ApplySpellImmune school matrix
-- IS ported in the event-23 hook below;
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
-- summon casts ride spell data (summoned entries unverifiable)
-- and the JustSummoned AttackStart relay has no bridge —
-- unmodeled (see above); the display / immune arms WERE ported
-- 2026-10-20 (SetDisplayId + ApplySpellImmune school matrix in
-- the event-23 hook and the DoTransform() rotation in the pump)
-- — the old "no display / immune bridge" claim is stale;
-- the ScriptData "SD%Complete: 90 / Immunities are wrong,
-- must be adjusted to use resistance from creature_templates.
-- Most spells require database support." caveat is upstream,
-- documented, not bridged; no SpellScript/AuraScript
-- scripts in this file.

local SPELL_ALLERGIC_REACTION = 34697
local SPELL_TELEPORT_SELF = 34673

-- C++ boss_laj.cpp :46-51 model enum.
local MODEL_DEFAULT = 13109
local MODEL_ARCANE = 14213
local MODEL_FIRE = 13110
local MODEL_FROST = 14112
local MODEL_NATURE = 14214

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

-- C++ Reset() (called on evade): the SetDisplayId(MODEL_DEFAULT) arm
-- is PORTED 2026-10-20 in the event-23 hook; _Reset() is
-- instance-blocked; the ApplySpellImmune school
-- matrix is PORTED in the event-23 hook below; the observable
-- remainder — the Initialize() re-latch — lands on OnEnterCombat
-- (gargolmar precedent).
local function fullReset(guid)
    cancelCombatPump(guid)
    combatStates[guid] = nil
end

-- C++ UpdateAI in arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses
-- into the pump. Only bridgeable arms are modeled (the
-- summon-cast arms stay bridge-blocked (see deviations); the
-- display/immune arms are PORTED 2026-10-20).
local function combatTick(creature, guid)
    local st = combatStates[guid]
    if not st then
        return
    end

    -- 1. Summon arm (only while canSummon, C++ gate order —
    -- always first): Talk(EMOTE_SUMMON) + CanSummon = false,
    -- the eight TRIGGERED summon casts skipped (spell-data summons,
    -- entries unverifiable — see header), re-arm 2500
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

    -- 4. Transform arm: DoTransform() — rand32() % 5 model pick +
    -- per-case SetDisplayId + ApplySpellImmune school matrix
    -- (C++ DoTransform case order: display first, then the
    -- shadow/arcane/fire/frost/nature immunity flips); re-arm
    -- 25000 + rand32() % 15000 (C++-exact).
    if st.transform <= 1000 then
        local roll = math.random(0, 4)
        local model = MODEL_DEFAULT
        local immuneMask = 32 -- SPELL_SCHOOL_MASK_SHADOW
        if roll == 1 then
            model = MODEL_ARCANE
            immuneMask = 64 -- SPELL_SCHOOL_MASK_ARCANE
        elseif roll == 2 then
            model = MODEL_FIRE
            immuneMask = 4 -- SPELL_SCHOOL_MASK_FIRE
        elseif roll == 3 then
            model = MODEL_FROST
            immuneMask = 16 -- SPELL_SCHOOL_MASK_FROST
        elseif roll == 4 then
            model = MODEL_NATURE
            immuneMask = 8 -- SPELL_SCHOOL_MASK_NATURE
        end
        creature:SetDisplayId(model)
        creature:ApplySpellImmune(0, 2, 32, immuneMask == 32)
        creature:ApplySpellImmune(0, 2, 64, immuneMask == 64)
        creature:ApplySpellImmune(0, 2, 4, immuneMask == 4)
        creature:ApplySpellImmune(0, 2, 16, immuneMask == 16)
        creature:ApplySpellImmune(0, 2, 8, immuneMask == 8)
        st.transform = 25000 + math.random(0, 14999)
    else
        st.transform = st.transform - 1000
    end
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
        transform = 30000,
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
    -- C++ boss_lajAI::Reset() :86-91: SetDisplayId(MODEL_DEFAULT)
    -- PORTED 2026-10-20 (display bridge live — C++ arm order:
    -- display first, then the school immunity matrix); shadow
    -- school immune by default, arcane/fire/frost/nature explicitly
    -- cleared (the DoTransform flips are ported in the combat tick).
    creature:SetDisplayId(MODEL_DEFAULT)
    creature:ApplySpellImmune(0, 2, 32, true)
    creature:ApplySpellImmune(0, 2, 64, false)
    creature:ApplySpellImmune(0, 2, 4, false)
    creature:ApplySpellImmune(0, 2, 16, false)
    creature:ApplySpellImmune(0, 2, 8, false)
end)
