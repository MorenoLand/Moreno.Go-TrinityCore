-- Crystalcore Devastator (The Eye, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/Eye/
-- the_eye.cpp (npc_crystalcore_devastator — the only
-- CreatureScript AI class AddSC_the_eye registers).
-- Entry: 20040 (wowhead-verified — the C++ sources carry no
-- NPC_ constant for the Devastator; the creature_template
-- ScriptName bindings are DB-side, no TDB in this
-- workspace). instance_the_eye.cpp stays blocked on the
-- instance-script model (DOCUMENTED-ONLY).
-- No Talk lines in the C++ (no SAY enum at all).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 23 OnReset (no DamageTaken override — no
-- event 9).
-- Timers via CreateLuaEvent (per-GUID scheduler pump); melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell,
-- triggered) = DoCastVictim, creature:CastSpell(creature,
-- spell) = DoCast(me).
-- Fight shape (C++-exact for all modeled arms):
-- 1. Knock Away 22893: 25s init then 23s — TRIGGERED
--    DoCastVictim (C++ DoCastVictim(22893, true), C++-exact),
--    guarded on the C++ GetVictim() nil check; re-arm 23s
--    regardless (C++-exact — the re-arm sits outside the
--    victim check). The C++ ResetThreat(victim) has no
--    bridge — there is no single-target threat bridge
--    (void_reaver no-ModifyThreatByPercent precedent) —
--    unmodeled.
-- 2. Countercharge 35035: 9s init then 45s — non-triggered
--    DoCast(me) self-cast (C++-exact); re-arm 45s
--    regardless (C++-exact). The C++ UNIT_STATE_CASTING
--    early-return gates have no cast-state bridge — the arms
--    fire unconditionally (zereketh precedent).
-- OnReset(23) / OnLeaveCombat(2): per-GUID state cleared (the
-- C++ Reset() re-latches the ctor / Initialize() values;
-- the GetTheEyeAI instance hook is unbridgeable).

local SPELL_COUNTERCHARGE = 35035
local SPELL_KNOCKAWAY = 22893

local ENTRY_CRYSTALCORE_DEVASTATOR = 20040

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetDevastator(guid)
    cancelPump(guid)
    states[guid] = nil
end

-- C++ ctor / Reset() Initialize(): Knockaway_Timer = 25000,
-- Countercharge_Timer = 9000. Timers engage-start (alar
-- engagement-schedule precedent).
local function freshState()
    return {
        knockaway = 25000,
        countercharge = 9000,
    }
end

local function mobTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Knock Away 22893: 25s init then 23s — TRIGGERED
    -- DoCastVictim with the C++ GetVictim() nil guard
    -- (C++-exact); re-arm 23s regardless (C++-exact).
    -- ResetThreat(victim): no single-target threat bridge —
    -- unmodeled.
    if st.knockaway <= 1000 then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(nil, SPELL_KNOCKAWAY, true)
        end
        st.knockaway = 23000
    else
        st.knockaway = st.knockaway - 1000
    end

    -- Countercharge 35035: 9s init then 45s — non-triggered
    -- DoCast(me) self-cast (C++-exact); re-arm 45s
    -- regardless (C++-exact).
    if st.countercharge <= 1000 then
        creature:CastSpell(creature, SPELL_COUNTERCHARGE)
        st.countercharge = 45000
    else
        st.countercharge = st.countercharge - 1000
    end
end

RegisterCreatureEvent(ENTRY_CRYSTALCORE_DEVASTATOR, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetDevastator(guid)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        mobTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_CRYSTALCORE_DEVASTATOR, 2, function(_, creature)
    resetDevastator(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_CRYSTALCORE_DEVASTATOR, 4, function(_, creature)
    resetDevastator(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_CRYSTALCORE_DEVASTATOR, 23, function(_, creature)
    resetDevastator(creature:GetGUID())
end)
