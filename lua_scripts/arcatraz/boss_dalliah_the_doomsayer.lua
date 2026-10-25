-- Dalliah the Doomsayer (The Arcatraz, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- arcatraz/boss_dalliah_the_doomsayer.cpp (boss_dalliah_the_
-- doomsayer — the only CreatureScript AI class AddSC_boss_
-- dalliah_the_doomsayer registers; no SpellScript/AuraScript
-- scripts in this file).
-- Second boss in the Arcatraz set per outland_script_
-- loader.cpp order (instance_arcatraz.cpp stays blocked on
-- the instance-script model).
-- Entry: 20885 (NPC_DALLIAH in arcatraz.h — creature entry,
-- verifiable from the C++ sources); instance_arcatraz.cpp
-- OnCreatureCreate maps NPC_DALLIAH to DalliahGUID. The
-- creature_template ScriptName bindings are DB-side (no TDB
-- in this workspace).
-- Talk: SAY_AGGRO = 1 (pull — fired from the C++
-- JustEngagedWith, C++-exact), SAY_SLAY = 2 (kill — the C++
-- KilledUnit talks unconditionally, no player gate, gargolmar
-- precedent, C++-exact), SAY_WHIRLWIND = 3 (whirlwind — fired
-- from the EVENT_WHIRLWIND arm, C++-exact), SAY_HEAL = 4
-- (heal — fired from the EVENT_HEAL arm, C++-exact), SAY_
-- DEATH = 5 (death — fired from the C++ JustDied, C++-exact);
-- SAY_SOCCOTHRATES_DEATH = 7 fires from the
-- EVENT_SOCCOTHRATES_DEATH arm, PORTED 2026-10-24 via
-- event 7 + creature:SetData (see onAIUpdate). SAY_AGGRO_
-- DALLIAH_FIRST = 0 and SAY_DALLIAH_25_PERCENT = 5 are
-- Soccothrates' creature-text lines, fired via soccothrates->
-- AI()->Talk(...) — PORTED 2026-10-24 in bossTick via the
-- FindNearestCreature / grid twin (see below).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via
-- CreateLuaEvent (per-GUID scheduler pump); melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee
-- AttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Dalliah
-- (20885): OnEnterCombat(1): per-GUID reset to the C++
-- Initialize-equivalent schedule (gift {1s,4s} / whirlwind
-- {7s,9s}; the BossAI::JustEngagedWith arm is instance-
-- blocked, DATA_DALLIAH = 1 is an instance-side constant,
-- unbridgeable) + Talk(SAY_AGGRO) + 1s scheduler pump (a port
-- of UpdateAI in C++ arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses into
-- the pump; the soccothratesDeath out-of-combat branch is
-- PORTED 2026-10-24 via event 7 + creature:SetData (see
-- onAIUpdate below). Pump:
-- gift of the doomsayer 36173 — {1s,4s} init then {16s,21s},
-- TRIGGERED DoCastVictim (C++ `DoCastVictim(SPELL_GIFT_OF_
-- THE_DOOMSAYER, true)`, C++-exact), re-arm {16s,21s}
-- regardless (C++-exact); whirlwind 36142 — {7s,9s} init then
-- {19s,21s}, non-triggered DoCast on self (C++ DoCast(me),
-- C++-exact), Talk(SAY_WHIRLWIND), re-arm {19s,21s} (C++-
-- exact), schedule the heal arm in 6s (C++-exact); heal
-- 36144 — one-shot, non-triggered DoCast on self (C++ DoCast
-- (me), C++-exact), Talk(SAY_HEAL). The HealthBelowPct(25)
-- latch runs every tick (halazzi strict-fraction precedent):
-- the first sub-25% tick sets the soccothratesTaunt latch —
-- the C++ Talk(SAY_DALLIAH_25_PERCENT) on Soccothrates' AI
-- has no cross-creature bridge, so the yell is unreachable
-- and the latch is bookkeeping only (omor summonedCount
-- precedent). The C++ UNIT_STATE_CASTING early-return gates
-- (before and after the event switch) have no cast-state
-- bridge — the arms fire unconditionally (netherspite
-- precedent). Melee is engine-driven.
-- OnTargetDied(3): Talk(SAY_SLAY) (C++ talks unconditionally —
-- no player gate, C++-exact). OnDied(4): Talk(SAY_DEATH) +
-- cleanup (the _JustDied arm is instance-blocked; the
-- soccothrates->AI()->SetData(1, 1) death relay is PORTED
-- below via FindNearestCreature + SetData). OnLeaveCombat(2)/OnReset(23): cancel the
-- pump, drop per-GUID state (the C++ Reset() observable
-- remainder — the schedule re-latch — lands on OnEnterCombat
-- per the gargolmar precedent; the _Reset arm is instance-
-- blocked).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (instance_arcatraz.cpp stays blocked on the instance-script
-- model; the BossAI ctor DATA_DALLIAH = 1 bookkeeping in
-- Reset/JustEngagedWith/JustDied skipped); no cross-creature
-- bridge — the EVENT_ME_FIRST 6s arm (Talk(SAY_AGGRO_DALLIAH_
-- FIRST) on Soccothrates' AI via instance GetGuidData(DATA_
-- SOCCOTHRATES)) PORTED 2026-10-24 in bossTick; the
-- HealthBelowPct(25) taunt (Talk(SAY_DALLIAH_25_PERCENT) on
-- Soccothrates' AI) PORTED 2026-10-24 in bossTick; the JustDied
-- soccothrates->AI()->SetData(1, 1) relay PORTED 2026-10-24
-- in OnDied below; and the EVENT_SOCCOTHRATES_DEATH machine
-- (Soccothrates' JustDied calls dalliah->AI()->SetData(1, 1)
-- when Dalliah is alive and out of combat; 6s later Dalliah
-- says SAY_SOCCOTHRATES_DEATH = 7 via the !UpdateVictim arm)
-- PORTED 2026-10-24 via event 7 + creature:SetData
-- (ScriptData store, lua_creature_events.go) + the
-- FindNearestCreature / grid twin idiom (see bossTick,
-- onAIUpdate and the OnDied relay below); the heroic-only
-- EVENT_SHADOW_WAVE 39016 arm (TRIGGERED DoCastVictim
-- {11s,16s}) is modeled via creature:IsHeroic(); no
-- cast-state bridge — the UNIT_STATE_CASTING early-return
-- gates unmodeled (netherspite precedent); no SpellScript/
-- AuraScript scripts in this file.

local SPELL_GIFT_OF_THE_DOOMSAYER = 36173
local SPELL_WHIRLWIND = 36142
local SPELL_HEAL = 36144
local SPELL_SHADOW_WAVE = 39016

local SAY_AGGRO = 1
local SAY_SLAY = 2
local SAY_WHIRLWIND = 3
local SAY_HEAL = 4
local SAY_DEATH = 5
local SAY_SOCCOTHRATES_DEATH = 7
local SAY_AGGRO_DALLIAH_FIRST = 0
local SAY_DALLIAH_25_PERCENT = 5

local ENTRY_DALLIAH = 20885
local ENTRY_SOCCOTHRATES = 20886

local timers = {}
local states = {}
local deathTalk = {}

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
    deathTalk[guid] = nil
end

-- C++ JustEngagedWith schedule (the BossAI arm is instance
-- blocked, so the whole engagement schedule lands on
-- OnEnterCombat — omor precedent): gift {1s,4s} / whirlwind
-- {7s,9s}; soccothratesTaunt is the C++ ctor latch, false at
-- engage (the C++ Reset() re-latch).
local function freshState()
    return {
        gift = 1000 + math.random(0, 2999),
        whirlwind = 7000 + math.random(0, 1999),
        heal = nil,
        shadowWave = nil,
        soccothratesTaunt = false,
        meFirst = 6000,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Gift of the doomsayer 36173: {1s,4s} init then {16s,21s}
    -- — TRIGGERED DoCastVictim (C++-exact), re-arm {16s,21s}
    -- regardless (C++-exact). The C++ UNIT_STATE_CASTING
    -- early-return gates have no live cast-state read bridge (IsCasting is constant-false — Go casts are fire-and-forget) — the arm
    -- fires unconditionally (netherspite precedent).
    if st.gift <= 1000 then
        creature:CastSpell(nil, SPELL_GIFT_OF_THE_DOOMSAYER, true)
        st.gift = 16000 + math.random(0, 4999)
    else
        st.gift = st.gift - 1000
    end

    -- Whirlwind 36142: {7s,9s} init then {19s,21s} — non-
    -- triggered DoCast on self (C++ DoCast(me), C++-exact),
    -- Talk(SAY_WHIRLWIND), re-arm {19s,21s} (C++-exact) and
    -- schedule the one-shot heal arm in 6s (C++-exact).
    if st.whirlwind <= 1000 then
        creature:CastSpell(creature, SPELL_WHIRLWIND)
        creature:Talk(SAY_WHIRLWIND)
        st.whirlwind = 19000 + math.random(0, 1999)
        st.heal = 6000
    else
        st.whirlwind = st.whirlwind - 1000
    end

    -- Heal 36144: one-shot — non-triggered DoCast on self (C++
    -- DoCast(me), C++-exact), Talk(SAY_HEAL).
    if st.heal then
        if st.heal <= 1000 then
            creature:CastSpell(creature, SPELL_HEAL)
            creature:Talk(SAY_HEAL)
            st.heal = nil
        else
            st.heal = st.heal - 1000
        end
    end

    -- Shadow wave 39016 (heroic only): TRIGGERED DoCastVictim
    -- {11s,16s} init then {11s,16s} (C++-exact); scheduled at
    -- engage only under IsHeroic() (C++ :92-94).
    if st.shadowWave then
        if st.shadowWave <= 1000 then
            creature:CastSpell(nil, SPELL_SHADOW_WAVE, true)
            st.shadowWave = 11000 + math.random(0, 4999)
        else
            st.shadowWave = st.shadowWave - 1000
        end
    end

    -- EVENT_ME_FIRST 6s one-shot (boss_dalliah_the_doomsayer.cpp:94, 167-171):
    -- PORTED 2026-10-24 — at engage + 6s, if Soccothrates is alive
    -- and out of combat, Soccothrates' AI talks SAY_AGGRO_DALLIAH_FIRST.
    -- FindNearestCreature is live-only (lua_creature_events.go), so the
    -- alive gate rides it; the !IsInCombat() check is explicit, and
    -- 200yd stands in for the instance GUID. Fires once: a guarded
    -- target costs the arm, as the C++ event is consumed on fire.
    if st.meFirst then
        if st.meFirst <= 1000 then
            st.meFirst = nil
            local soccothrates = creature:FindNearestCreature(ENTRY_SOCCOTHRATES, 200)
            if soccothrates ~= nil and not soccothrates:IsInCombat() then
                soccothrates:Talk(SAY_AGGRO_DALLIAH_FIRST)
            end
        else
            st.meFirst = st.meFirst - 1000
        end
    end

    -- HealthBelowPct(25) latch (halazzi strict-fraction
    -- precedent): PORTED 2026-10-24 — the C++ arm fires
    -- Talk(SAY_DALLIAH_25_PERCENT) on Soccothrates' AI
    -- (boss_dalliah_the_doomsayer.cpp:180-184); the C++ arm
    -- null-checks the instance GUID only, so the grid twin
    -- (dead-inclusive, twin-emperor precedent) talks on the
    -- first grid match with no alive/in-combat gate, and
    -- 200yd stands in for the instance GUID.
    if not st.soccothratesTaunt then
        local maxHealth = creature:GetMaxHealth()
        if maxHealth > 0
                and creature:GetHealth() < math.floor(maxHealth * 25 / 100) then
            st.soccothratesTaunt = true
            local soccothrates = creature:GetCreaturesInGrid(ENTRY_SOCCOTHRATES, 200)
            if #soccothrates > 0 then
                soccothrates[1]:Talk(SAY_DALLIAH_25_PERCENT)
            end
        end
    end
end

-- C++ SetData(1, 1) / EVENT_SOCCOTHRATES_DEATH machine
-- (boss_dalliah_the_doomsayer.cpp:103-109 SetData schedules the
-- 6s event; :116-136 the !UpdateVictim branch executes it):
-- Soccothrates' JustDied relays SetData(1, 1) when Dalliah is
-- alive and out of combat. Creature event 7 fires on the 100ms
-- world tick incl. pre-pull (hydross precedent); GetVictim() ==
-- nil is the !UpdateVictim twin (dathrohan precedent). The
-- countdown starts on the first event-7 tick that sees
-- GetData(1) == 1; entering combat drops it (the C++ combat
-- branch consumes the event via default), as do the
-- leave-combat/reset/enter-combat state clears. Returns
-- nothing, so it never vetoes the engine AI step.
local function onAIUpdate(event, creature, diff)
    local guid = creature:GetGUID()
    if creature:GetData(1) ~= 1 then
        deathTalk[guid] = nil
        return
    end
    if creature:GetVictim() ~= nil then
        deathTalk[guid] = nil
        creature:SetData(1, 0)
        return
    end
    if diff == nil then
        diff = 100
    end
    local remaining = deathTalk[guid]
    if remaining == nil then
        remaining = 6000
    end
    if remaining <= diff then
        creature:Talk(SAY_SOCCOTHRATES_DEATH)
        deathTalk[guid] = nil
        creature:SetData(1, 0)
    else
        deathTalk[guid] = remaining - diff
    end
end

RegisterCreatureEvent(ENTRY_DALLIAH, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    if creature:IsHeroic() then
        states[guid].shadowWave = 11000 + math.random(0, 4999)
    end
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_DALLIAH, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_DALLIAH, 3, function(_, creature)
    -- C++ KilledUnit talks unconditionally — no player gate
    -- (gargolmar precedent, C++-exact).
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_DALLIAH, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    -- C++ JustDied relay (boss_dalliah_the_doomsayer.cpp:77-84):
    -- soccothrates alive and out of combat -> soccothrates->
    -- AI()->SetData(1, 1). FindNearestCreature is live-only
    -- (lua_creature_events.go), so the alive gate rides it;
    -- 200yd (midnight precedent) stands in for the instance
    -- GUID — a single soccothrates per instance.
    local soccothrates = creature:FindNearestCreature(ENTRY_SOCCOTHRATES, 200)
    if soccothrates ~= nil and not soccothrates:IsInCombat() then
        soccothrates:SetData(1, 1)
    end
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_DALLIAH, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_DALLIAH, 7, onAIUpdate)
