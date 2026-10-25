-- The Black Knight (Trial of the Champion) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheChampion/boss_black_knight.cpp
-- (boss_black_knight (ScriptedAI), npc_risen_ghoul (ScriptedAI),
-- npc_black_knight_skeletal_gryphon (EscortAI); AddSC_boss_black_knight
-- at end registers all three via GetTrialOfTheChampionAI).
-- The second Trial of the Champion group in northrend_script_loader.cpp
-- order (decl 46 / call 241, immediately after
-- AddSC_boss_argent_challenge(); Trial of the Champion block stays
-- OPEN).
-- Entry: 35451 The Black Knight (trial_of_the_champion.h
-- NPC_BLACK_KNIGHT — kalecgos pass; the GetTrialOfTheChampionAI
-- ScriptName binding is instance-shimmed, the creature_template binding
-- DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "boss_black_knight",
-- "npc_risen_ghoul" and "npc_black_knight_skeletal_gryphon" hits
-- boss_black_knight.cpp only (loader carries only the AddSC decl/call
-- lines); zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim (moroes precedent);
-- creature:CastSpell(creature, spell) = DoCastSelf (phase_hunter /
-- apothecary_hanes precedent — the trailing true is the triggered
-- port).
-- C++ has no Say enum at all ("missing yells" in its SD%Complete
-- 80 % note) — no Talk arms to port.
-- C++ note: the uiPlagueStrikeTimer arm calls
-- DoCastVictim(SPELL_ICY_TOUCH), not Plague Strike — replicated
-- C++-exact here (apparent C++ bug; the timers' init/repeat windows
-- are verbatim from C++ Initialize()).
-- Ported arms (C++-exact for all modeled arms):
-- boss_black_knight: uiIcyTouchTimer — DoCastVictim(Icy Touch 67718),
-- urand(5000,9000) init, urand(5000,7000) repeat; uiPlagueStrikeTimer —
-- DoCastVictim(Icy Touch 67718) (see note above), urand(10000,13000)
-- init, urand(12000,15000) repeat; uiObliterateTimer —
-- DoCastVictim(Obliterate 67725), urand(17000,19000) init, repeat —
-- all scheduled from combat start (the three arms fire identically in
-- PHASE_UNDEAD and PHASE_SKELETON; the phase machine advances via the
-- ported DamageTaken/resurrect machine, and the timers keep running
-- through the fake-death window gated by the resurrecting latch, but
-- STOP (no re-arm) once PHASE_GHOST arrives — C++ PHASE_GHOST case
-- carries only death bite + marked death).
-- JustDied: DoCastSelf(Kill Credit 68663) on event
-- 4 (its instance SetBossState(BOSS_BLACK_KNIGHT, DONE) leg has no
-- bridge — instance model absent, standing). OnLeaveCombat(2) /
-- OnDied(4) / OnReset(23) cancel the scheduler (gargolmar precedent).
-- Ported arms, continued:
-- boss_black_knight DamageTaken + resurrect machine — PORTED:
-- damage > health && uiPhase <= PHASE_SKELETON -> damage = 0 +
-- SetHealth(0) + rooted (SetRooted covers the ROOT half of
-- AddUnitState(ROOT | STUNNED); the STUNNED half has no bridge) +
-- SetDisplayId(MODEL_SKELETON 29846 on the UNDEAD->SKELETON fall /
-- MODEL_GHOST 21300 on SKELETON->GHOST) + the UpdateAI resurrect
-- machine (4s one-shot: SetFullHealth + triggered DoCastSelf(Black
-- Knight Res 67693) + phase advance + root clear). The stale "no
-- DamageTaken hook / no display bridge" note is retracted (event 9
-- fires pre-damage; SetDisplayId/SetRooted bridged). The UpdateAI
-- !HasUnitState(ROOT) && !HealthBelowPct(1) melee gate
-- (boss_black_knight.cpp:264) suppresses the swing for the whole
-- fake-death window; Go's engine tick swings while rooted
-- (creaturemotion.go:3016-3017: DoMeleeAttackIfReady gates CASTING
-- only), so SetMeleeAttack(false) rides the fake-death arm and
-- SetMeleeAttack(true) rides the resurrect + evade/reset restores.
-- The bEventInProgress
-- latch is modeled as the Lua-side resurrecting flag so repeat
-- lethal hits in the fake-death window re-run the idempotent legs
-- without re-arming the timer (C++ never re-arms uiResurrectTimer
-- mid-window either). summons.DespawnAll() has no summon bridge.
-- Unmodeled (no bridges — documented, not wired):
-- boss_black_knight uiDeathRespiteTimer (PHASE_UNDEAD) — PORTED below
-- (threat-list pick, urand(15000,16000), phase + fake-death gating
-- per the icy-touch idiom).
-- boss_black_knight PHASE_SKELETON legs: phase arrives via the ported
-- DamageTaken/resurrect machine; the bSummonArmy one-shot ROOT leg, the
-- uiDeathArmyCheckTimer clear, and the melee-gate leg are PORTED
-- 2026-10-24 (SetRooted(true) + SetMeleeAttack(false) at skeleton entry,
-- 7s one-shot clear; the STUNNED half has no bridge, so rotation casts
-- gate on the armyWindow latch while re-arming, as in the fake-death
-- window). DoCastSelf(Army of the Dead 67761) stays out (DB-side summon
-- effect, ghoul entry unverifiable, no Go spell-effect producer);
-- uiDesecration PORTED below (skeleton-entry + urand(15000,16000));
-- uiGhoulExplode
-- PORTED 2026-10-24 — DoCastSelf (67751), 8000 repeat, scheduled at
-- skeleton entry (its C++ timer is frozen at 8000 until PHASE_SKELETON),
-- gated on the resurrecting / armyWindow latches while re-arming,
-- stops at PHASE_GHOST.
-- boss_black_knight PHASE_GHOST legs — phase now arrives via the
-- ported resurrect machine: uiDeathBiteTimer PORTED 2026-10-24 —
-- DoCastAOE(Death's Bite 67808) lowered to self-cast (selin/mr_smite
-- precedent), urand(2000,4000) init/repeat, scheduled at ghost entry
-- (its C++ timer is frozen until PHASE_GHOST); the UNDEAD/SKELETON
-- rotation timers STOP at PHASE_GHOST (C++ PHASE_GHOST case carries
-- only death bite + marked death — no re-arm, phase never leaves
-- ghost). uiMarkedDeathTimer — PORTED below (ghost-entry +
-- urand(5000,7000), threat-list pick).
-- boss_black_knight Reset — summons.DespawnAll() + SetDisplayId(native)
-- + ClearUnitState(ROOT | STUNNED) — the despawn leg stays out: the
-- ghoul entry is unverifiable (DB-side SPELL_ARMY_DEAD summon) so no
-- grid twin exists; display/root legs are bridged (see DamageTaken). JustSummoned — summons.Summon + cross-AI AttackStart(me->
-- GetVictim()) (summon STRAND absent, standing). Melee gated on
-- !HasUnitState(ROOT) && !HealthBelowPct(1): the READ has no unit-state
-- bridge, but both legs are modeled Lua-side (health via GetHealthPct,
-- root via the DamageTaken fake-death arm and the skeleton army-window
-- arm, which drive SetMeleeAttack(false/true)); no unmodeled melee gap.
-- npc_risen_ghoul — zero registration: ENTRY UNVERIFIABLE (no NPC
-- constant for the ghoul in trial_of_the_champion.h or anywhere in the
-- C++ sources; the ghouls materialize via the DB-side SPELL_ARMY_DEAD
-- 67761 summon effect — belnistrasz/willix precedent). Joins the
-- bridgeable-but-entry-blocked queue with a port-pattern-ready rotation
-- arm: uiAttackTimer — SelectTarget(Random, 1, 100, true) ->
-- DoCast(Leap 67749), 3500 repeat — entry unverifiable, so
-- unregistrable despite the live GetAITargets bridge.
-- npc_black_knight_skeletal_gryphon — zero registration: ENTRY
-- UNVERIFIABLE (no NPC constant ties the script name to an entry in
-- C++ — the creature_template binding is DB-side; the header's
-- VEHICLE_BLACK_KNIGHT 35491 names the mount, not the script binding).
-- Whole AI is the EscortAI machine (constructor Start(false, true) +
-- EscortAI::UpdateAI) — escort / motion-master bridges absent (escort
-- queue precedent). Joins the bridgeable-but-entry-blocked queue.

local ENTRY_BLACK_KNIGHT = 35451

local SPELL_ICY_TOUCH = 67718
local SPELL_OBLITERATE = 67725
local SPELL_KILL_CREDIT = 68663
local SPELL_BLACK_KNIGHT_RES = 67693 -- boss_black_knight.cpp:56
local SPELL_GHOUL_EXPLODE = 67751 -- boss_black_knight.cpp:51
local SPELL_DEATH_BITE = 67808 -- boss_black_knight.cpp:55
local SPELL_DEATH_RESPITE = 67745 -- boss_black_knight.cpp:37
local SPELL_DESECRATION = 67778 -- boss_black_knight.cpp:46
local SPELL_MARKED_DEATH = 67882 -- boss_black_knight.cpp:53

local MODEL_SKELETON = 29846 -- boss_black_knight.cpp:66
local MODEL_GHOST = 21300 -- boss_black_knight.cpp:67

local PHASE_UNDEAD = 1 -- boss_black_knight.cpp:72
local PHASE_SKELETON = 2 -- boss_black_knight.cpp:73
local PHASE_GHOST = 3 -- boss_black_knight.cpp:74

local timers = {}

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

local bkState = {}
local nativeDisplay = {}

local function getBlackKnightState(guid)
    local st = bkState[guid]
    if not st then
        st = { phase = PHASE_UNDEAD, resurrecting = false, armyWindow = false }
        bkState[guid] = st
    end
    return st
end

-- C++ uiIcyTouchTimer: DoCastVictim(Icy Touch), urand(5000,9000) init,
-- urand(5000,7000) repeat; independent of the DamageTaken / phase
-- machine (fires identically in PHASE_UNDEAD and PHASE_SKELETON).
-- C++-exact: the phase timers keep running through the 4s fake-death
-- window (UpdateAI never early-returns on bEventInProgress), but each cast
-- attempt fails on UNIT_STATE_STUNNED (Spell::CheckCast); the STUNNED half
-- has no bridge, so the resurrecting latch gates the cast while the re-arm
-- continues.
local function icyTouchTick(creature, guid)
    if getBlackKnightState(guid).phase == PHASE_GHOST then
        return
    end
    if not getBlackKnightState(guid).resurrecting and not getBlackKnightState(guid).armyWindow then
        creature:CastSpell(nil, SPELL_ICY_TOUCH)
    end
    local next = math.random(5000, 7000)
    schedule(guid, "icy_touch", next, function()
        icyTouchTick(creature, guid)
    end)
end

-- C++ uiPlagueStrikeTimer: DoCastVictim(SPELL_ICY_TOUCH) — the C++
-- names the timer Plague Strike but casts Icy Touch (apparent C++
-- bug); replicated C++-exact. urand(10000,13000) init,
-- urand(12000,15000) repeat.
-- C++-exact: the phase timers keep running through the 4s fake-death
-- window (UpdateAI never early-returns on bEventInProgress), but each cast
-- attempt fails on UNIT_STATE_STUNNED (Spell::CheckCast); the STUNNED half
-- has no bridge, so the resurrecting latch gates the cast while the re-arm
-- continues.
local function plagueStrikeTick(creature, guid)
    if getBlackKnightState(guid).phase == PHASE_GHOST then
        return
    end
    if not getBlackKnightState(guid).resurrecting and not getBlackKnightState(guid).armyWindow then
        creature:CastSpell(nil, SPELL_ICY_TOUCH)
    end
    local next = math.random(12000, 15000)
    schedule(guid, "plague_strike", next, function()
        plagueStrikeTick(creature, guid)
    end)
end

-- C++ uiObliterateTimer: DoCastVictim(Obliterate), urand(17000,19000)
-- init, urand(17000,19000) repeat.
-- C++-exact: the phase timers keep running through the 4s fake-death
-- window (UpdateAI never early-returns on bEventInProgress), but each cast
-- attempt fails on UNIT_STATE_STUNNED (Spell::CheckCast); the STUNNED half
-- has no bridge, so the resurrecting latch gates the cast while the re-arm
-- continues.
local function obliterateTick(creature, guid)
    if getBlackKnightState(guid).phase == PHASE_GHOST then
        return
    end
    if not getBlackKnightState(guid).resurrecting and not getBlackKnightState(guid).armyWindow then
        creature:CastSpell(nil, SPELL_OBLITERATE)
    end
    local next = math.random(17000, 19000)
    schedule(guid, "obliterate", next, function()
        obliterateTick(creature, guid)
    end)
end

-- C++ uiGhoulExplodeTimer: DoCastSelf(Ghoul Explode), 8000 repeat,
-- PHASE_SKELETON case only. The timer is frozen at 8000 until the
-- phase arrives (Initialize value, never decremented in PHASE_UNDEAD),
-- so skeleton-entry + 8000ms is C++-exact; casts fail on STUNNED in
-- C++ during the fake-death / army windows, so the latches gate the
-- cast while the re-arm continues; no re-arm once PHASE_GHOST arrives.
local function ghoulExplodeTick(creature, guid)
    local st = getBlackKnightState(guid)
    if st.phase ~= PHASE_SKELETON then
        return
    end
    if not st.resurrecting and not st.armyWindow then
        creature:CastSpell(creature, SPELL_GHOUL_EXPLODE)
    end
    schedule(guid, "ghoul_explode", 8000, function()
        ghoulExplodeTick(creature, guid)
    end)
end

-- C++ uiDeathBiteTimer: DoCastAOE(Death's Bite), urand(2000,4000)
-- init/repeat, PHASE_GHOST case only — lowered to self-cast per the
-- established DoCastAOE convention. Frozen until ghost entry, so
-- ghost-entry + urand(2000,4000) is C++-exact.
local function deathBiteTick(creature, guid)
    if getBlackKnightState(guid).phase ~= PHASE_GHOST then
        return
    end
    creature:CastSpell(creature, SPELL_DEATH_BITE)
    schedule(guid, "death_bite", math.random(2000, 4000), function()
        deathBiteTick(creature, guid)
    end)
end

-- C++ SelectTarget(Random, 0, 100, true) — threat-list players alive
-- within 100y (UnitAI.h; same stale-pool class as the eadric fix).
local function randomThreatPlayer(creature, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive()
                and creature:IsWithinDist(t, maxDist) then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ uiDeathRespiteTimer (PHASE_UNDEAD only): SelectTarget -> Death's
-- Respite, urand(15000,16000) init/repeat, unconditional re-arm. The
-- timer freezes once the phase leaves UNDEAD (never returns), so a
-- non-UNDEAD tick stops without re-arming. Casts fail on STUNNED in
-- C++ during the fake-death window, so the resurrecting latch gates
-- the cast while the re-arm continues (icy-touch idiom).
local function deathRespiteTick(creature, guid)
    local st = getBlackKnightState(guid)
    if st.phase ~= PHASE_UNDEAD then
        return
    end
    if not st.resurrecting then
        local target = randomThreatPlayer(creature, 100)
        if target then
            creature:CastSpell(target, SPELL_DEATH_RESPITE)
        end
    end
    schedule(guid, "death_respite", math.random(15000, 16000), function()
        deathRespiteTick(creature, guid)
    end)
end

-- C++ uiDesecration (PHASE_SKELETON only): same pick -> Desecration,
-- urand(15000,16000) init/repeat; frozen until skeleton entry, so
-- skeleton-entry + window is C++-exact (ghoul-explode precedent).
local function desecrationTick(creature, guid)
    local st = getBlackKnightState(guid)
    if st.phase ~= PHASE_SKELETON then
        return
    end
    if not st.resurrecting and not st.armyWindow then
        local target = randomThreatPlayer(creature, 100)
        if target then
            creature:CastSpell(target, SPELL_DESECRATION)
        end
    end
    schedule(guid, "desecration", math.random(15000, 16000), function()
        desecrationTick(creature, guid)
    end)
end

-- C++ uiMarkedDeathTimer (PHASE_GHOST only): same pick -> Marked for
-- Death, urand(5000,7000) init/repeat; frozen until ghost entry.
local function markedDeathTick(creature, guid)
    local st = getBlackKnightState(guid)
    if st.phase ~= PHASE_GHOST then
        return
    end
    local target = randomThreatPlayer(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_MARKED_DEATH)
    end
    schedule(guid, "marked_death", math.random(5000, 7000), function()
        markedDeathTick(creature, guid)
    end)
end

local function blackKnightEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    if nativeDisplay[guid] == nil then
        nativeDisplay[guid] = creature:GetDisplayId()
    end
    schedule(guid, "icy_touch", math.random(5000, 9000), function()
        icyTouchTick(creature, guid)
    end)
    schedule(guid, "plague_strike", math.random(10000, 13000), function()
        plagueStrikeTick(creature, guid)
    end)
    schedule(guid, "obliterate", math.random(17000, 19000), function()
        obliterateTick(creature, guid)
    end)
    schedule(guid, "death_respite", math.random(15000, 16000), function()
        deathRespiteTick(creature, guid)
    end)
end

-- C++ boss_black_knight::DamageTaken (boss_black_knight.cpp:268-290)
-- + the UpdateAI resurrect machine (:159-170): while the phase is
-- UNDEAD or SKELETON a killing blow is absorbed (damage = 0), the
-- knight plays dead at 0 health rooted in the next form's display,
-- and 4s later he resurrects at full health (triggered self-cast
-- Black Knight Res 67693, phase advance, root clear). Strict
-- incoming-damage check (uiDamage > health), so no postDamage guard.
local function blackKnightDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = getBlackKnightState(guid)
    if st.phase <= PHASE_SKELETON and damage > creature:GetHealth() then
        creature:SetHealth(0)
        creature:SetRooted(true)
        creature:SetMeleeAttack(false)
        if st.phase == PHASE_UNDEAD then
            creature:SetDisplayId(MODEL_SKELETON)
        else
            creature:SetDisplayId(MODEL_GHOST)
        end
        if not st.resurrecting then
            st.resurrecting = true
            schedule(guid, "resurrect", 4000, function()
                creature:SetHealth(creature:GetMaxHealth())
                creature:CastSpell(creature, SPELL_BLACK_KNIGHT_RES, true)
                st.phase = st.phase + 1
                st.resurrecting = false
                creature:SetRooted(false)
                creature:SetMeleeAttack(true)
                if st.phase == PHASE_SKELETON and not st.armyWindow then
                    st.armyWindow = true
                    creature:SetRooted(true)
                    creature:SetMeleeAttack(false)
                    schedule(guid, "army_window", 7000, function()
                        creature:SetRooted(false)
                        creature:SetMeleeAttack(true)
                        st.armyWindow = false
                    end)
                end
                if st.phase == PHASE_SKELETON then
                    schedule(guid, "ghoul_explode", 8000, function()
                        ghoulExplodeTick(creature, guid)
                    end)
                    schedule(guid, "desecration", math.random(15000, 16000), function()
                        desecrationTick(creature, guid)
                    end)
                elseif st.phase == PHASE_GHOST then
                    schedule(guid, "death_bite", math.random(2000, 4000), function()
                        deathBiteTick(creature, guid)
                    end)
                    schedule(guid, "marked_death", math.random(5000, 7000), function()
                        markedDeathTick(creature, guid)
                    end)
                end
            end)
        end
        return false, 0
    end
end

local function blackKnightLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    bkState[guid] = nil
    creature:SetMeleeAttack(true)
    creature:SetRooted(false)
end

-- C++ JustDied: DoCastSelf(Kill Credit 68663) (the
-- instance->SetBossState leg has no bridge).
local function blackKnightDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_KILL_CREDIT)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    bkState[guid] = nil
    nativeDisplay[guid] = nil
end

local function blackKnightReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    bkState[guid] = nil
    creature:SetMeleeAttack(true)
    creature:SetRooted(false)
    -- C++ Reset: SetDisplayId(GetNativeDisplayId()) — no native-display
    -- bridge, so the tharon_ja idiom: the display captured at first
    -- combat is restored (an evade mid-fake-death would otherwise leave
    -- the skeleton/ghost morph).
    if nativeDisplay[guid] ~= nil then
        creature:SetDisplayId(nativeDisplay[guid])
        nativeDisplay[guid] = nil
    end
end

RegisterCreatureEvent(ENTRY_BLACK_KNIGHT, 1, blackKnightEnterCombat)
RegisterCreatureEvent(ENTRY_BLACK_KNIGHT, 2, blackKnightLeaveCombat)
RegisterCreatureEvent(ENTRY_BLACK_KNIGHT, 4, blackKnightDied)
RegisterCreatureEvent(ENTRY_BLACK_KNIGHT, 9, blackKnightDamageTaken)
RegisterCreatureEvent(ENTRY_BLACK_KNIGHT, 23, blackKnightReset)
