-- Wrath-Scryer Soccothrates (The Arcatraz, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- arcatraz/boss_wrath_scryer_soccothrates.cpp (boss_wrath_
-- scryer_soccothrates — the only CreatureScript AI class
-- AddSC_boss_wrath_scryer_soccothrates registers; no
-- SpellScript/AuraScript scripts in this file).
-- Third boss in the Arcatraz set per outland_script_
-- loader.cpp order (instance_arcatraz.cpp stays blocked on
-- the instance-script model).
-- Entry: 20886 (NPC_SOCCOTHRATES in arcatraz.h — creature
-- entry, verifiable from the C++ sources); instance_
-- arcatraz.cpp OnCreatureCreate maps NPC_SOCCOTHRATES to
-- SoccothratesGUID. The creature_template ScriptName
-- bindings are DB-side (no TDB in this workspace).
-- Talk: SAY_AGGRO = 1 (pull — fired from the C++
-- JustEngagedWith, C++-exact), SAY_SLAY = 2 (kill — the C++
-- KilledUnit talks unconditionally, no player gate, gargolmar
-- precedent, C++-exact), SAY_KNOCK_AWAY = 3 (knock away —
-- fired from the EVENT_KNOCK_AWAY arm, C++-exact), SAY_DEATH
-- = 4 (death — fired from the C++ JustDied, C++-exact);
-- SAY_DALLIAH_DEATH = 6 fires only from the EVENT_DALLIAH_
-- DEATH arm — PORTED 2026-10-24 via event 7 +
-- creature:SetData (see onAIUpdate); SAY_SOCCOTHRATES_
-- CONVO_1..4 = 7..10 fire only from the prefight machine —
-- unreachable (MoveInLineOfSight arm instance-blocked, see
-- deviations). SAY_AGGRO_SOCCOTHRATES_FIRST = 0 and SAY_
-- SOCCOTHRATES_25_PERCENT = 6 and SAY_DALLIAH_CONVO_1..3 =
-- 8..10 are Dalliah's creature-text lines, fired via dalliah->
-- AI()->Talk(...) — PORTED 2026-10-24 in bossTick via the
-- FindNearestCreature / grid twin (see below).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via
-- CreateLuaEvent (per-GUID scheduler pump); melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee
-- AttackIfReady.
-- Fight shape (C++-exact for all modeled arms): Soccothrates
-- (20886): OnEnterCombat(1): per-GUID reset to the C++ ctor
-- / JustEngagedWith values (felfireShock {12s,14s} / knockAway
-- {11s,12s} / dalliahTaunt = false — the BossAI::JustEngagedWith
-- arm is instance-blocked, DATA_SOCCOTHRATES = 2 is an
-- instance-side constant, unbridgeable; the EVENT_ME_FIRST 6s
-- one-shot arm is PORTED 2026-10-24 in bossTick) + Talk
-- (SAY_AGGRO) + 1s scheduler pump (a port of UpdateAI in C++
-- arm order — 1s granularity exact for all C++ timers; the
-- !UpdateVictim early return collapses into the pump: the
-- preFight branch stays out (no MoveInLineOfSight /
-- instance-data bridge); the dalliahDeath branch is PORTED
-- 2026-10-24 via event 7 + creature:SetData (see onAIUpdate). felfire shock 35759 —
-- {12s,14s} init then {12s,14s}, TRIGGERED DoCastVictim (C++
-- `DoCastVictim(SPELL_FELFIRE_SHOCK, true)`, C++-exact),
-- re-arm {12s,14s} regardless (C++-exact); knock away 36512
-- — {11s,12s} init then {11s,12s}, non-triggered DoCast on
-- self (C++ DoCast(me), C++-exact), Talk(SAY_KNOCK_AWAY),
-- re-arm {11s,12s} regardless (C++-exact). The HealthBelowPct
-- (25) latch runs every tick (halazzi strict-fraction
-- precedent): the first sub-25% tick sets the dalliahTaunt
-- latch — the C++ Talk(SAY_SOCCOTHRATES_25_PERCENT) on
-- Dalliah's AI has no cross-creature bridge, so the yell is
-- unreachable and the latch is bookkeeping only (omor
-- summonedCount precedent). The C++ UNIT_STATE_CASTING early-
-- return gates (before and after the event switch) have no
-- cast-state bridge — the arms fire unconditionally
-- (netherspite precedent). Melee is engine-driven.
-- OnTargetDied(3): Talk(SAY_SLAY) (C++ talks unconditionally —
-- no player gate, C++-exact). OnDied(4): Talk(SAY_DEATH) +
-- cleanup (the _JustDied arm is instance-blocked; the
-- dalliah->AI()->SetData(1, 1) death relay is PORTED
-- below via FindNearestCreature + SetData). OnLeaveCombat(2)/OnReset(23): cancel the
-- pump, drop per-GUID state (the C++ Reset() observable
-- remainder — the schedule / dalliahTaunt / preFight re-latch
-- — lands on OnEnterCombat per the gargolmar precedent; the
-- _Reset arm is instance-blocked).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_arcatraz.cpp stays blocked on the instance-
-- script model; the BossAI ctor DATA_SOCCOTHRATES = 2
-- bookkeeping in Reset/JustEngagedWith/JustDied skipped); no
-- MoveInLineOfSight / instance-data bridge — the 70yd
-- proximity arm (instance->GetData(DATA_CONVERSATION) ==
-- NOT_STARTED + TYPEID_PLAYER gate) is unreachable, so the
-- whole preFight machine is unmodeled: the EVENT_PREFIGHT_
-- 1..9 chain, Talk(SAY_SOCCOTHRATES_CONVO_1..4), the dalliah->
-- AI()->Talk(SAY_DALLIAH_CONVO_1..3) arms, the dalliah + self
-- MovePoint arms and the SetFacingToObject / SetHomePosition
-- arms (SetFacingToObject itself is bridgeable via the atan2 twin —
-- math.atan2 + bridged creature:SetFacingTo, devourer_of_souls
-- precedent — but the arms ride the unmodeled cross-creature
-- preFight chain (EVENT_PREFIGHT_9, C++ :203-206 via instance->
-- GetGuidData(DATA_DALLIAH), me<->dalliah face targets) — no
-- cross-creature / movement bridges); no
-- cross-creature bridge — PORTED 2026-10-24: the EVENT_ME_FIRST
-- 6s arm (dalliah->AI()->Talk(SAY_AGGRO_SOCCOTHRATES_FIRST)
-- when Dalliah is alive and out of combat), the HealthBelowPct(25)
-- taunt (dalliah->AI()->Talk(SAY_SOCCOTHRATES_25_PERCENT)),
-- the JustDied dalliah->AI()->SetData(1, 1) relay, and the
-- SetData(1) / EVENT_DALLIAH_DEATH machine (6s later
-- Talk(SAY_DALLIAH_DEATH)) — all via event 7 + creature:SetData
-- (ScriptData store, lua_creature_events.go) + the
-- FindNearestCreature / grid twin idiom (see bossTick, onAIUpdate
-- and the OnDied relay below); no live cast-state read bridge (IsCasting is constant-false — Go casts are fire-and-forget)
-- — the UNIT_STATE_CASTING early-return gates unmodeled
-- (netherspite precedent); the unused spell enums (SPELL_
-- FELFIRE_LINE_UP 35770 / SPELL_CHARGE_TARGETING 36038 /
-- SPELL_CHARGE 35754) never fire in the C++ AI — the
-- ScriptData "charge left to script" caveat is upstream,
-- documented, not bridged (millhouse unused-spells
-- precedent); no SpellScript/AuraScript scripts in this file.

local SPELL_FELFIRE_SHOCK = 35759
local SPELL_KNOCK_AWAY = 36512

local SAY_AGGRO = 1
local SAY_SLAY = 2
local SAY_KNOCK_AWAY = 3
local SAY_DEATH = 4
local SAY_DALLIAH_DEATH = 6
local SAY_AGGRO_SOCCOTHRATES_FIRST = 0
local SAY_SOCCOTHRATES_25_PERCENT = 6

local ENTRY_SOCCOTHRATES = 20886
local ENTRY_DALLIAH = 20885

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

-- C++ ctor / JustEngagedWith schedule (the BossAI arms are
-- instance-blocked, so the whole engagement schedule lands
-- on OnEnterCombat — omor precedent): felfireShock {12s,14s}
-- / knockAway {11s,12s}; dalliahTaunt is the C++ ctor latch,
-- false at engage (the C++ Reset() re-latch). The EVENT_ME_
-- FIRST 6s arm is PORTED 2026-10-24 in bossTick.
local function freshState()
    return {
        felfireShock = 12000 + math.random(0, 1999),
        knockAway = 11000 + math.random(0, 999),
        dalliahTaunt = false,
        meFirst = 6000,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Felfire shock 35759: {12s,14s} init then {12s,14s} —
    -- TRIGGERED DoCastVictim (C++-exact), re-arm {12s,14s}
    -- regardless (C++-exact). The C++ UNIT_STATE_CASTING
    -- early-return gates have no live cast-state read bridge (IsCasting is constant-false — Go casts are fire-and-forget) — the arm
    -- fires unconditionally (netherspite precedent).
    if st.felfireShock <= 1000 then
        creature:CastSpell(nil, SPELL_FELFIRE_SHOCK, true)
        st.felfireShock = 12000 + math.random(0, 1999)
    else
        st.felfireShock = st.felfireShock - 1000
    end

    -- Knock away 36512: {11s,12s} init then {11s,12s} — non-
    -- triggered DoCast on self (C++ DoCast(me), C++-exact),
    -- Talk(SAY_KNOCK_AWAY), re-arm {11s,12s} regardless
    -- (C++-exact).
    if st.knockAway <= 1000 then
        creature:CastSpell(creature, SPELL_KNOCK_AWAY)
        creature:Talk(SAY_KNOCK_AWAY)
        st.knockAway = 11000 + math.random(0, 999)
    else
        st.knockAway = st.knockAway - 1000
    end

    -- EVENT_ME_FIRST 6s one-shot (boss_wrath_scryer_soccothrates.cpp:117, 254-257):
    -- PORTED 2026-10-24 — at engage + 6s, if Dalliah is alive
    -- and out of combat, Dalliah's AI talks SAY_AGGRO_SOCCOTHRATES_FIRST.
    -- FindNearestCreature is live-only (lua_creature_events.go), so the
    -- alive gate rides it; the !IsInCombat() check is explicit, and
    -- 200yd stands in for the instance GUID. Fires once: a guarded
    -- target costs the arm, as the C++ event is consumed on fire.
    if st.meFirst then
        if st.meFirst <= 1000 then
            st.meFirst = nil
            local dalliah = creature:FindNearestCreature(ENTRY_DALLIAH, 200)
            if dalliah ~= nil and not dalliah:IsInCombat() then
                dalliah:Talk(SAY_AGGRO_SOCCOTHRATES_FIRST)
            end
        else
            st.meFirst = st.meFirst - 1000
        end
    end

    -- HealthBelowPct(25) latch (halazzi strict-fraction
    -- precedent): PORTED 2026-10-24 — the C++ arm fires
    -- Talk(SAY_SOCCOTHRATES_25_PERCENT) on Dalliah's AI
    -- (boss_wrath_scryer_soccothrates.cpp:267-270); the C++ arm
    -- null-checks the instance GUID only, so the grid twin
    -- (dead-inclusive, twin-emperor precedent) talks on the
    -- first grid match with no alive/in-combat gate, and
    -- 200yd stands in for the instance GUID.
    if not st.dalliahTaunt then
        local maxHealth = creature:GetMaxHealth()
        if maxHealth > 0
                and creature:GetHealth() < math.floor(maxHealth * 25 / 100) then
            st.dalliahTaunt = true
            local dalliah = creature:GetCreaturesInGrid(ENTRY_DALLIAH, 200)
            if #dalliah > 0 then
                dalliah[1]:Talk(SAY_SOCCOTHRATES_25_PERCENT)
            end
        end
    end
end

-- C++ SetData(1, 1) / EVENT_DALLIAH_DEATH machine
-- (boss_wrath_scryer_soccothrates.cpp:139-146 SetData schedules the
-- 6s event; :216-231 the !UpdateVictim branch executes it):
-- Dalliah's JustDied relays SetData(1, 1) when Soccothrates is
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
        creature:Talk(SAY_DALLIAH_DEATH)
        deathTalk[guid] = nil
        creature:SetData(1, 0)
    else
        deathTalk[guid] = remaining - diff
    end
end

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 3, function(_, creature)
    -- C++ KilledUnit talks unconditionally — no player gate
    -- (gargolmar precedent, C++-exact).
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    -- C++ JustDied relay (boss_wrath_scryer_soccothrates.cpp:102-110):
    -- dalliah alive and out of combat -> dalliah->AI()->SetData(1, 1).
    -- FindNearestCreature is live-only (lua_creature_events.go), so the
    -- alive gate rides it; 200yd (midnight precedent) stands in for the
    -- instance GUID — a single dalliah per instance.
    local dalliah = creature:FindNearestCreature(ENTRY_DALLIAH, 200)
    if dalliah ~= nil and not dalliah:IsInCombat() then
        dalliah:SetData(1, 1)
    end
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 7, onAIUpdate)
