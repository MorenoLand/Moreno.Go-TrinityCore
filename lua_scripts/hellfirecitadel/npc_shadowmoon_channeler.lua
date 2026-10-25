-- Shadowmoon Channeler (Blood Furnace, Hellfire Citadel) — Lua port
-- of the npc_shadowmoon_channeler AI class in src/server/scripts/
-- Outland/HellfireCitadel/BloodFurnace/boss_kelidan_the_breaker.cpp
-- (the add AI class AddSC_boss_kelidan_the_breaker registers
-- alongside the boss; no other creature/GO scripts in this file).
-- Entry: ENTRY_CHANNELER = 17653 from the file's own enum
-- (verified externally — wowhead npc=17653/shadowmoon-channeler,
-- mmo4ever creature 17653, db.pandawow controlled abilities match
-- Mark of Shadow + Shadow Bolt); the creature_template ScriptName
-- bindings are DB-side (no TDB in this workspace).
-- Talk: NONE — the file defines no Say enum and neither struct
-- overrides KilledUnit (no event 3 registered).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pump); melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady. C++ DoCast default is
-- triggered=false (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Channeler
-- (17653): OnEnterCombat(1): per-GUID reset to the C++
-- Initialize() values (shadowBolt 1000+rand32()%1000 /
-- markOfShadow 5000+rand32()%2000 — the C++ JustEngagedWith's
-- ChannelerEngaged relay is cross-creature blocked, the
-- InterruptNonMeleeSpells arm has no live unit-state read bridge (IsCasting is constant-false — Go casts are fire-and-forget) and the
-- DoStartMovement arm (= MoveChase(victim)) is subsumed by the engine combat chase) + 1s scheduler
-- pump (a port of UpdateAI — 1s granularity exact for all C++
-- timers; the !UpdateVictim early return collapses into the
-- pump). Pump: mark of shadow 30937: 5000+rand32()%2000 init
-- then 15000+rand32()%5000 (C++ {15s,20s}) — target = random
-- threat-list unit alive, no distance gate (C++ SelectTarget(Random, 0)
-- defaults playerOnly=false), nil pick casts nothing, non-triggered
-- DoCast on the target (C++ DoCast(target), C++-exact), re-arm
-- {15s,20s} regardless (C++-exact); shadow bolt 12739
-- (H_SPELL_SHADOW_BOLT 15472 is an unused C++ enum member —
-- the cast is unconditional, C++-exact): 1000+
-- rand32()%1000 init then 5000+rand32()%1000 (C++ {5s,6s}) —
-- non-triggered DoCastVictim (felmyst convention), re-arm
-- {5s,6s} regardless (C++-exact).
-- OnDied(4): cleanup (the ChannelerDied cross-creature relay to
-- Kelidan is unbridgeable — documented below; the !killer early
-- return is C++-exact). OnLeaveCombat(2)/OnReset(23): cleanup.
-- Deliberate deviations (all await engine bridges): no
-- ObjectAccessor/cross-creature bridge — the whole pre-combat
-- channeling machine unmodeled: the !UpdateVictim check_Timer
-- arm (GetChanneled(me) cross-creature GUID relay from Kelidan +
-- DoCast(channeled, SPELL_CHANNELING 39123)) and the
-- JustEngagedWith ChannelerEngaged(who) relay (which drives
-- Kelidan's addYell-gated SAY_ADD_AGGRO talk and the AttackStart
-- (who) relay) and the JustDied ChannelerDied(killer) relay
-- (which drives Kelidan's all-channelers-dead activation —
-- REACT_AGGRESSIVE + RemoveFlag(NON_ATTACKABLE) +
-- SetImmuneToAll(false) + AttackStart(killer)); no unit-state
-- bridge — the Reset/JustEngagedWith InterruptNonMeleeSpells
-- arms unmodeled (mennu/thespia precedent); H_SPELL_SHADOW_BOLT
-- 15472 is an unused C++ enum member (the cast is
-- unconditional); no movement bridge — the JustEngagedWith
-- DoStartMovement arm is engine-chase-subsumed (see above).

local SPELL_MARK_OF_SHADOW = 30937
local SPELL_SHADOW_BOLT = 12739

local ENTRY_CHANNELER = 17653

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetAdd(guid)
    cancelPump(guid)
    states[guid] = nil
end

-- C++ SelectTarget(Random, 0): random threat-list unit alive, no
-- distance gate (defaults playerOnly=false, so pets/totems are
-- eligible, council precedent). Nil when the threat list holds no
-- living unit — the caller skips the cast (C++-exact).
local function pickRandomPlayer(creature)
    local found = {}
    for _, u in ipairs(creature:GetAITargets()) do
        if not u:IsDead() then
            found[#found + 1] = u
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ Initialize() values: ShadowBolt_Timer=1000+rand32()%1000,
-- MarkOfShadow_Timer=5000+rand32()%2000, check_Timer=0 (the
-- channeling check arm is cross-creature gated — documented
-- above).
local function freshState()
    return {
        shadowBolt = 1000 + math.random(0, 999),
        markOfShadow = 5000 + math.random(0, 1999),
    }
end

local function addTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Mark of shadow 30937: 5000+rand32()%2000 init then
    -- 15000+rand32()%5000 (C++ {15s,20s}) — random threat-list
    -- unit alive, no distance gate, nil pick casts
    -- nothing, non-triggered DoCast on the target (C++
    -- DoCast(target), C++-exact), re-arm {15s,20s} regardless
    -- (C++-exact).
    if st.markOfShadow <= 1000 then
        local target = pickRandomPlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_MARK_OF_SHADOW)
        end
        st.markOfShadow = 15000 + math.random(0, 4999)
    else
        st.markOfShadow = st.markOfShadow - 1000
    end

    -- Shadow bolt 12739 (H_SPELL 15472 unused in the C++):
    -- 1000+rand32()%1000 init then 5000+rand32()%1000 (C++
    -- {5s,6s}) — non-triggered DoCastVictim (felmyst
    -- convention), re-arm {5s,6s} regardless (C++-exact).
    if st.shadowBolt <= 1000 then
        creature:CastSpell(nil, SPELL_SHADOW_BOLT)
        st.shadowBolt = 5000 + math.random(0, 999)
    else
        st.shadowBolt = st.shadowBolt - 1000
    end
end

RegisterCreatureEvent(ENTRY_CHANNELER, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetAdd(guid)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        addTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_CHANNELER, 2, function(_, creature)
    resetAdd(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_CHANNELER, 4, function(_, creature)
    resetAdd(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_CHANNELER, 23, function(_, creature)
    resetAdd(creature:GetGUID())
end)
