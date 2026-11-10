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
-- PORTED 2026-11-10 via the ev7 proximity poll + prefight pump
-- (see onAIUpdate / prefightArm below). SAY_AGGRO_
-- SOCCOTHRATES_FIRST = 0 and SAY_SOCCOTHRATES_25_PERCENT = 6
-- and SAY_DALLIAH_CONVO_1..3 = 8..10 are Dalliah's
-- creature-text lines, fired via dalliah->AI()->Talk(...) —
-- PORTED 2026-10-24 in bossTick via the FindNearestCreature /
-- grid twin (see below).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 7 OnAIUpdate, 23 OnReset, 27
-- OnMoveInLineOfSight. Timers via
-- CreateLuaEvent (per-GUID scheduler pump); melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee
-- AttackIfReady.
-- Fight shape (C++-exact for all modeled arms): Soccothrates
-- (20886): OnEnterCombat(1): per-GUID reset to the C++ ctor
-- / JustEngagedWith values (felfireShock {12s,14s} / knockAway
-- {11s,12s} / dalliahTaunt = false — the BossAI::JustEngagedWith
-- setActive/ScheduleTasks legs stay blocked; the SetBossState(2,1)
-- IN_PROGRESS write is PORTED in ev1 below; the base pulse +
-- me-zone arms are PORTED in ev1 below; the EVENT_ME_FIRST 6s
-- one-shot arm is PORTED 2026-10-24 in bossTick) + Talk
-- (SAY_AGGRO) + 1s scheduler pump (a port of UpdateAI in C++
-- arm order — 1s granularity exact for all C++ timers; the
-- !UpdateVictim early return collapses into the pump: the
-- preFight branch is PORTED 2026-11-10 via the ev7 proximity
-- poll + prefight pump (see onAIUpdate / prefightArm below); the dalliahDeath branch is PORTED
-- 2026-10-24 via event 7 + creature:SetData (see onAIUpdate). felfire shock 35759 —
-- {12s,14s} init then {12s,14s}, TRIGGERED DoCastVictim (C++
-- `DoCastVictim(SPELL_FELFIRE_SHOCK, true)`, C++-exact),
-- re-arm {12s,14s} regardless (C++-exact); knock away 36512
-- — {11s,12s} init then {11s,12s}, non-triggered DoCast on
-- self (C++ DoCast(me), C++-exact), Talk(SAY_KNOCK_AWAY),
-- re-arm {11s,12s} regardless (C++-exact). The HealthBelowPct
-- (25) latch runs every tick (halazzi strict-fraction
-- precedent): the first sub-25% tick sets the dalliahTaunt
-- latch and talks SAY_SOCCOTHRATES_25_PERCENT on Dalliah via
-- GetCreaturesInGrid (dead-inclusive, C++-exact — no alive/
-- in-combat gate in the C++ arm; see bossTick). The C++ UNIT_STATE_CASTING early-
-- return gates (before and after the event switch, boss_
-- wrath_scryer_soccothrates.cpp:238/:263) are PORTED via
-- the shared-pump gate idiom: each due arm carries `and
-- not creature:IsCasting()` (live CastingUntil window,
-- lua_creature_events.go:1950) — a gated arm fires
-- nothing, consumes no re-arm, and stays due via the
-- else-branch countdown, since C++ events.Update precedes
-- the gate; the post-loop 25% latch is gated the same way
-- (the C++ pre-loop return skips it while casting). Melee
-- is engine-driven.
-- OnTargetDied(3): Talk(SAY_SLAY) (C++ talks unconditionally —
-- no player gate, C++-exact). OnDied(4): Talk(SAY_DEATH) +
-- cleanup (the _JustDied setActive/persistence legs stay
-- blocked; the SetBossState(2,3) DONE write is PORTED; the
-- dalliah->AI()->SetData(1, 1) death relay is PORTED
-- below via FindNearestCreature + SetData). OnLeaveCombat(2)/OnReset(23): cancel the
-- pump, drop per-GUID state (the C++ Reset() observable
-- remainder — the schedule / dalliahTaunt / preFight re-latch
-- — lands on OnEnterCombat per the gargolmar precedent; the
-- SetBossState(2,0) NOT_STARTED write is PORTED in ev23 below).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_arcatraz.cpp stays blocked on the instance-
-- script model; the BossAI ctor DATA_SOCCOTHRATES = 2
-- SetBossState bookkeeping is PORTED as the (2,1)/(2,0)/(2,3)
-- writes below); the MoveInLineOfSight prefight machine is
-- PORTED 2026-11-10 (see onAIUpdate / prefightArm): the 70yd
-- proximity arm rides the ev7 poll (runthak precedent: ev27
-- fires only from the aggro-engage branch, and this trigger is
-- out-of-combat proximity) + creature:GetPlayersInGrid(70)
-- (3D + reach, live players same map+instance), and the
-- conversation flag rides the per-creature ScriptData store
-- via creature:SetData(DATA_CONVERSATION, DONE) /
-- GetData (lua_creature_events.go:809) — the instance file
-- needs no switch: the only C++ reader is Soccothrates's own
-- MoveInLineOfSight gate (instance_arcatraz.cpp:133-134), so
-- the flag never leaves this creature;
-- the EVENT_PREFIGHT_1..9 chain, Talk(SAY_SOCCOTHRATES_
-- CONVO_1..4), the dalliah->AI()->Talk(SAY_DALLIAH_CONVO_1..3)
-- arms, the dalliah + self MovePoint arms (MoveTo twin,
-- lua_creature_events.go:4986) and the SetFacingToObject /
-- SetHomePosition arms (SetFacingToObject via the atan2 twin +
-- bridged creature:SetFacingTo, nightbane precedent;
-- SetHomePosition bridged, lua_creature_events.go:6554) all
-- ride the chain; the Dalliah legs resolve via the grid twin
-- (GetCreaturesInGrid, dead-inclusive — the C++ arms null-check
-- the instance GUID only). The chain dies on engage (the C++
-- combat branch consumes remaining PREFIGHT events via
-- default) and the conversation flag is instance-lifetime
-- (never cleared by Reset, C++-exact). The ev27 veto below
-- ports the other half of the override: C++ MoveInLineOfSight
-- never calls the base, so Soccothrates never proximity-aggros
-- — every aggro-engage-branch fire is vetoed and combat starts
-- only via damage (ev1); no
-- cross-creature bridge — PORTED 2026-10-24: the EVENT_ME_FIRST
-- 6s arm (dalliah->AI()->Talk(SAY_AGGRO_SOCCOTHRATES_FIRST)
-- when Dalliah is alive and out of combat), the HealthBelowPct(25)
-- taunt (dalliah->AI()->Talk(SAY_SOCCOTHRATES_25_PERCENT)),
-- the JustDied dalliah->AI()->SetData(1, 1) relay, and the
-- SetData(1) / EVENT_DALLIAH_DEATH machine (6s later
-- Talk(SAY_DALLIAH_DEATH)) — all via event 7 + creature:SetData
-- (ScriptData store, lua_creature_events.go) + the
-- FindNearestCreature / grid twin idiom (see bossTick, onAIUpdate
-- and the OnDied relay below); the UNIT_STATE_CASTING
-- early-return gates are PORTED (see bossTick); the unused spell enums (SPELL_
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
local SAY_SOCCOTHRATES_CONVO_1 = 7
local SAY_SOCCOTHRATES_CONVO_2 = 8
local SAY_SOCCOTHRATES_CONVO_3 = 9
local SAY_SOCCOTHRATES_CONVO_4 = 10
local SAY_DALLIAH_CONVO_1 = 8
local SAY_DALLIAH_CONVO_2 = 9
local SAY_DALLIAH_CONVO_3 = 10

-- arcatraz.h AZDataTypes: DATA_CONVERSATION = 4; the flag value
-- is EncounterState DONE = 3 (InstanceScript.h).
local DATA_CONVERSATION = 4
local CONVERSATION_DONE = 3

local ENTRY_SOCCOTHRATES = 20886
local ENTRY_DALLIAH = 20885

local timers = {}
local states = {}
local deathTalk = {}
local prefight = {}

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
    prefight[guid] = nil -- C++ Reset()/JustEngagedWith: preFight = false
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
    -- regardless (C++-exact). The C++ pre-loop + in-loop
    -- CASTING gates are PORTED: a gated arm fires nothing
    -- and stays due (else-branch countdown).
    if st.felfireShock <= 1000 and not creature:IsCasting() then
        creature:CastSpell(nil, SPELL_FELFIRE_SHOCK, true)
        st.felfireShock = 12000 + math.random(0, 1999)
    else
        st.felfireShock = st.felfireShock - 1000
    end

    -- Knock away 36512: {11s,12s} init then {11s,12s} — non-
    -- triggered DoCast on self (C++ DoCast(me), C++-exact),
    -- Talk(SAY_KNOCK_AWAY), re-arm {11s,12s} regardless
    -- (C++-exact).
    if st.knockAway <= 1000 and not creature:IsCasting() then
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
        if st.meFirst <= 1000 and not creature:IsCasting() then
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
    if not st.dalliahTaunt and not creature:IsCasting() then
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

-- C++ MoveInLineOfSight prefight machine (boss_wrath_scryer_
-- soccothrates.cpp:127-136 trigger, :156-211 EVENT_PREFIGHT_
-- 1..9 chain): trigger = instance GetData(DATA_CONVERSATION)
-- == NOT_STARTED + player within 70y -> Talk CONVO_1, flag
-- DONE, chain at 2s/3s/3s/6s/2s/3s/2s/4s/4s. The Dalliah legs
-- resolve via GetCreaturesInGrid (dead-inclusive — the C++
-- arms null-check the instance GUID only, 25%-latch twin).
-- Returns the delay to the next step, or nil when the chain
-- ends (step 9: facing + home positions, preFight = false).
local function dalliahInGrid(creature)
    local found = creature:GetCreaturesInGrid(ENTRY_DALLIAH, 200)
    if #found > 0 then
        return found[1]
    end
    return nil
end

local function prefightArm(creature, step)
    if step == 1 then
        local dalliah = dalliahInGrid(creature)
        if dalliah ~= nil then
            dalliah:Talk(SAY_DALLIAH_CONVO_1)
        end
        return 3000
    elseif step == 2 then
        creature:Talk(SAY_SOCCOTHRATES_CONVO_2)
        return 3000
    elseif step == 3 then
        local dalliah = dalliahInGrid(creature)
        if dalliah ~= nil then
            dalliah:Talk(SAY_DALLIAH_CONVO_2)
        end
        return 6000
    elseif step == 4 then
        creature:Talk(SAY_SOCCOTHRATES_CONVO_3)
        return 2000
    elseif step == 5 then
        local dalliah = dalliahInGrid(creature)
        if dalliah ~= nil then
            dalliah:Talk(SAY_DALLIAH_CONVO_3)
        end
        return 3000
    elseif step == 6 then
        creature:Talk(SAY_SOCCOTHRATES_CONVO_4)
        return 2000
    elseif step == 7 then
        local dalliah = dalliahInGrid(creature)
        if dalliah ~= nil then
            dalliah:MoveTo(0, 118.6048, 96.84852, 22.44115)
        end
        return 4000
    elseif step == 8 then
        creature:MoveTo(0, 122.1035, 192.7203, 22.44115)
        return 4000
    elseif step == 9 then
        local dalliah = dalliahInGrid(creature)
        if dalliah ~= nil then
            -- SetFacingToObject twin (nightbane atan2 idiom):
            -- Dalliah faces self, self faces Dalliah.
            local faceDalliah = math.atan2(creature:GetY() - dalliah:GetY(), creature:GetX() - dalliah:GetX())
            if faceDalliah < 0 then
                faceDalliah = faceDalliah + 2 * math.pi
            end
            dalliah:SetFacingTo(faceDalliah)
            local faceSelf = math.atan2(dalliah:GetY() - creature:GetY(), dalliah:GetX() - creature:GetX())
            if faceSelf < 0 then
                faceSelf = faceSelf + 2 * math.pi
            end
            creature:SetFacingTo(faceSelf)
            dalliah:SetHomePosition(dalliah:GetX(), dalliah:GetY(), dalliah:GetZ(), 1.51737)
            creature:SetHomePosition(creature:GetX(), creature:GetY(), creature:GetZ(), 4.725722)
        end
        return nil
    end
    return nil
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
    local guid = creature:GetGUIDLow()
    -- Prefight machine (C++ UpdateAI !UpdateVictim order: the
    -- preFight block runs before the dalliahDeath block below).
    -- Engage kills the chain (JustEngagedWith preFight = false;
    -- the combat branch consumes remaining PREFIGHT events via
    -- default); the conversation flag survives (instance-
    -- lifetime, never cleared by Reset).
    if creature:GetVictim() ~= nil then
        prefight[guid] = nil
    else
        local pf = prefight[guid]
        if pf == nil then
            if creature:GetData(DATA_CONVERSATION) ~= CONVERSATION_DONE then
                local players = creature:GetPlayersInGrid(70)
                if #players > 0 then
                    creature:Talk(SAY_SOCCOTHRATES_CONVO_1)
                    creature:SetData(DATA_CONVERSATION, CONVERSATION_DONE)
                    prefight[guid] = { step = 1, remaining = 2000 }
                end
            end
        else
            if diff == nil then
                diff = 100
            end
            pf.remaining = pf.remaining - diff
            if pf.remaining <= 0 then
                local nextDelay = prefightArm(creature, pf.step)
                if nextDelay == nil then
                    prefight[guid] = nil
                else
                    pf.step = pf.step + 1
                    pf.remaining = nextDelay
                end
            end
        end
    end
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
    local guid = creature:GetGUIDLow()
    resetBoss(guid)
    states[guid] = freshState()
    creature:SetBossState(2, 1) -- DATA_SOCCOTHRATES, IN_PROGRESS (BossAI::_JustEngagedWith, ScriptedCreature.cpp:537; gruul/omor precedent)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 2, function(_, creature)
    resetBoss(creature:GetGUIDLow())
    if creature:IsAlive() then
        creature:SetData(1, 0) -- C++ Reset() clears dalliahDeath (boss_wrath_scryer_soccothrates.cpp:94-99)
    end
end)

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 3, function(_, creature)
    -- C++ KilledUnit talks unconditionally — no player gate
    -- (gargolmar precedent, C++-exact).
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 4, function(_, creature)
    creature:SetBossState(2, 3) -- DATA_SOCCOTHRATES, DONE (boss_wrath_scryer_soccothrates.cpp:102-106: _JustDied then Talk)
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
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 23, function(_, creature)
    if not creature:IsDead() and creature:GetBossState(2) ~= 3 then
        creature:SetBossState(2, 0) -- DATA_SOCCOTHRATES, NOT_STARTED (BossAI::_Reset, ScriptedCreature.cpp:499-511)
    end
    resetBoss(creature:GetGUIDLow())
    if creature:IsAlive() then
        creature:SetData(1, 0) -- C++ Reset() clears dalliahDeath (boss_wrath_scryer_soccothrates.cpp:94-99)
    end
end)

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 7, onAIUpdate)

-- C++ MoveInLineOfSight (boss_wrath_scryer_soccothrates.cpp:
-- 127-136) is an override that never calls the base, so
-- Soccothrates never proximity-aggros — combat starts only
-- via damage (ev1). Go's fireCreatureMoveInLOS fires ev27 on
-- the aggro-engage branch and a boolean true vetoes exactly
-- that engage, so return true models the override C++-exact
-- (cyanigosa precedent).
RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 27, function(_, creature, unit)
    return true
end)
