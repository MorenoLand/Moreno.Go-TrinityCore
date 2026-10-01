-- Millhouse Manastorm (The Arcatraz, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/arcatraz/
-- arcatraz.cpp (npc_millhouse_manastorm — the AI class AddSC_
-- arcatraz registers alongside npc_warden_mellichar; no
-- SpellScript/AuraScript scripts in this file).
-- First script in the Arcatraz set per outland_script_loader.cpp
-- order (AddSC_arcatraz precedes the four boss AddSC_* calls;
-- instance_arcatraz.cpp stays blocked on the instance-script
-- model).
-- Entry: NPC_MILLHOUSE = 20977 (arcatraz.h AZCreatureIds —
-- verifiable from the C++ sources); the creature_template
-- ScriptName bindings are DB-side (no TDB in this workspace).
-- Talk: SAY_INTRO_1 = 0 (intro — phase-1 arm, C++-exact),
-- SAY_INTRO_2 = 1 (intro — phase-2 arm, C++-exact), SAY_WATER
-- = 2 (conjure water — phase-3 arm, C++-exact), SAY_BUFFS = 3
-- (buffs — phase-4 arm, C++-exact), SAY_DRINK = 4 (drink —
-- phase-5 arm, C++-exact), SAY_READY = 5 (ready — phase-6 arm,
-- C++-exact), SAY_KILL = 6 (kill — the C++ KilledUnit override
-- gates on victim->GetTypeId() == TYPEID_PLAYER, kargath
-- GetObjectType convention, C++-exact), SAY_PYRO = 7
-- (pyroblast — fired from the pyroblast arm, C++-exact), SAY_
-- LOWHP = 9 (health below 20% — fired once from the LowHp
-- latch, C++-exact), SAY_DEATH = 10 (death — fired from the
-- C++ JustDied, C++-exact); SAY_ICEBLOCK = 8 is never Talk()ed
-- by the C++ AI (unused enum); SAY_COMPLETE = 11 fires only
-- from the C++ Reset() when the instance boss-state for DATA_
-- HARBINGER_SKYRISS is DONE — instance blocked, unreachable in
-- this model.
-- Eluna creature events: 5 OnSpawn, 1 OnEnterCombat, 2
-- OnLeaveCombat, 3 OnTargetDied, 4 OnDied, 23 OnReset.
-- Timers via CreateLuaEvent (per-GUID scheduler pumps); melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention); DoCastVictim takes nil as the
-- target arm (felmyst convention); DoCastSelf takes the
-- creature as the target arm (omor precedent).
-- Fight shape (C++-exact for all modeled arms): Millhouse
-- (20977): OnSpawn(5): fresh C++ ctor/Initialize() state
-- (EventProgress_Timer = 2000 / Phase = 1 / Init = false /
-- LowHp = false / Pyroblast_Timer = 1000 / Fireball_Timer =
-- 2500) + a 1s intro pump. The intro pump is the pre-combat
-- event machine: it ticks even in combat (the C++ arms run
-- inside the same UpdateAI), in C++ arm order — 1s
-- granularity exact for the C++ timers.
-- Intro pump: phase 1 — Talk(SAY_INTRO_1), re-arm 18000;
-- phase 2 — Talk(SAY_INTRO_2), re-arm 18000; phase 3 — Talk
-- (SAY_WATER) + DoCastSelf(SPELL_CONJURE_WATER 36879), re-arm
-- 7000; phase 4 — Talk(SAY_BUFFS) + DoCastSelf(SPELL_ICE_
-- ARMOR 36881), re-arm 7000; phase 5 — Talk(SAY_DRINK) +
-- DoCastSelf(SPELL_ARCANE_INTELLECT 36880), re-arm 7000;
-- phase 6 — Talk(SAY_READY), re-arm 6000; phase 7 — the
-- instance->SetData(DATA_WARDEN_2, DONE) arm is instance-
-- blocked, the me->SetImmuneToNPC(false) arm has no immune
-- bridge (executioner precedent), so the observable remainder
-- is Init = true — the pump self-cancels (the C++ ++Phase
-- takes it to 8 and the `Phase < 8` gate ends the machine).
-- OnEnterCombat(1): per-GUID reset to the C++ Initialize()
-- combat values (LowHp = false / Pyroblast_Timer = 1000 /
-- Fireball_Timer = 2500 — the C++ has no JustEngagedWith
-- override, so there is no engage yell) + a 1s combat pump (a
-- port of the combat half of UpdateAI in C++ arm order — the
-- !UpdateVictim early return collapses into the pump, which
-- only exists while in combat). Combat pump: LowHp latch —
-- Talk(SAY_LOWHP) once when health drops strictly below 20%
-- (C++ HealthBelowPct(20), halazzi strict-fraction
-- precedent); pyroblast 33975 — 1s init then 40s, Talk(SAY_
-- PYRO), DoCastVictim (felmyst convention), re-arm 40000
-- (C++-exact; the C++ !IsNonMeleeSpellCast(false) early-
-- return gate has no cast-state bridge — netherspite
-- precedent — so the arm fires unconditionally); fireball
-- 14034 — 2.5s init then 4s, DoCastVictim (felmyst
-- convention), re-arm 4000 (C++-exact). Melee is
-- engine-driven.
-- OnTargetDied(3): Talk(SAY_KILL) gated on the killed unit
-- being a player (C++-exact, kargath 3-arg-handler
-- convention). OnDied(4): Talk(SAY_DEATH) + cancel both pumps
-- + drop per-GUID state (the _JustDied arm is instance-
-- blocked; the commented-out FailQuest arm has no quest
-- bridge). OnLeaveCombat(2): cancel the combat pump + drop
-- combat state, and restart the intro machine when it had not
-- completed (the C++ Reset() re-Initialize()s — Phase = 1 /
-- Init = false — and the instance DATA_WARDEN_2 gate is
-- blocked, so the replay models the pre-phase-7 evade case;
-- once phase 7 completed, DATA_WARDEN_2 would be DONE in the
-- real instance and the intro stays done, C++-exact for the
-- reachable timeline). OnReset(23): cancel the combat pump +
-- drop combat state only — Eluna's On_Reset also fires ahead
-- of OnDied and OnSpawn (verified in engine/world/lua_
-- creature_events.go), so the intro restart stays on the
-- evade hook to avoid double pumps.
-- npc_warden_mellichar is documented only, not registered
-- (omor-heads precedent): entry NPC_MELLICHAR = 20904
-- (arcatraz.h). Its Reset() (SetFlag(UNIT_FLAG_NON_ATTACKABLE)
-- — no flag bridge; DoCastSelf(SPELL_TARGET_OMEGA 36852);
-- instance->SetBossState(DATA_HARBINGER_SKYRISS, NOT_STARTED)
-- — instance blocked), its empty AttackStart override, its
-- MoveInLineOfSight proximity arm (no LoS-aggro bridge —
-- gargolmar precedent — so the engage trigger itself is
-- unreachable), its JustEngagedWith (Talk(YELL_INTRO1) +
-- DoCastSelf(SPELL_BUBBLE_VISUAL 36849) + instance
-- SetBossState(IN_PROGRESS) + HandleGameObject(DATA_WARDENS_
-- SHIELD) + IsRunning = true), its whole UpdateAI phase
-- machine (CanProgress gated on instance GetData(DATA_WARDEN_
-- 1..4)/GetBossState(DATA_HARBINGER_SKYRISS); DoPrepareFor
-- Phase: InterruptNonMeleeSpells + RemoveAurasByType + DoCast
-- (me, SPELL_TARGET_ALPHA 36856 / BETA 36854 / DELTA 36857 /
-- GAMMA 36858) + instance SetData + HandleGameObject arms;
-- the SummonCreature arms — 20905/20906 at (472.231,
-- -150.86, 42.6573), 20977 at (417.242, -149.795, 42.6548),
-- 20908/20909 at (420.851, -174.337, 42.6655), 20910/20911 at
-- (470.364, -174.656, 42.6753), 20912 at (446.086, -182.506,
-- 44.0852) — no summon bridge, steamrigger precedent) and its
-- JustSummoned DoZoneInCombat/AttackStart relay (no cross-
-- creature bridge) all await engine bridges: no instance-
-- script/summon/cross-creature/gameobject/flag/LoS-aggro
-- bridges. Its yells (YELL_INTRO1 = 0 / YELL_INTRO2 = 1 /
-- YELL_RELEASE1 = 2 / YELL_RELEASE2A = 3 / YELL_RELEASE2B = 4
-- / YELL_RELEASE3 = 5 / YELL_RELEASE4 = 6 / YELL_WELCOME = 7)
-- are carried here for the record.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — the DATA_WARDEN_2/DATA_HARBINGER_
-- SKYRISS reads in Reset() and the phase-7 SetData arm are
-- skipped (instance_arcatraz.cpp stays blocked on the
-- instance-script model); no immune bridge — the phase-7
-- SetImmuneToNPC(false) arm unmodeled; no movement bridge —
-- the AttackStart MoveChase(who, 25.0f) arm unmodeled (omor
-- SetCombatMovement precedent); no cast-state bridge — the
-- pyroblast IsNonMeleeSpellCast(false) early-return gate
-- unmodeled (netherspite precedent); the unused C++ spells
-- (ARCANE_MISSILES 33833 / CONE_OF_COLD 12611 / FIRE_BLAST
-- 13341 / FROSTBOLT 15497) never fire in the C++ AI, so they
-- are not added (the ScriptData "@todo make better combatAI
-- for Millhouse" caveat is upstream, documented, not
-- bridged); no SpellScript/AuraScript scripts in this file.

local SAY_INTRO_1 = 0
local SAY_INTRO_2 = 1
local SAY_WATER = 2
local SAY_BUFFS = 3
local SAY_DRINK = 4
local SAY_READY = 5
local SAY_KILL = 6
local SAY_PYRO = 7
local SAY_LOWHP = 9
local SAY_DEATH = 10

local SPELL_CONJURE_WATER = 36879
local SPELL_ARCANE_INTELLECT = 36880
local SPELL_ICE_ARMOR = 36881
local SPELL_PYROBLAST = 33975
local SPELL_FIREBALL = 14034

local ENTRY_MILLHOUSE = 20977

local introTimers = {}
local introState = {}
local introDone = {}
local combatTimers = {}
local combatState = {}

local function healthBelowPct(creature, pct)
    local maxHealth = creature:GetMaxHealth()
    return maxHealth > 0
        and creature:GetHealth() * 100 / maxHealth < pct
end

local function cancelIntro(guid)
    local id = introTimers[guid]
    if id then
        RemoveEventById(id)
        introTimers[guid] = nil
    end
end

local function cancelCombat(guid)
    local id = combatTimers[guid]
    if id then
        RemoveEventById(id)
        combatTimers[guid] = nil
    end
end

local function introTick(creature, guid)
    local st = introState[guid]
    if not st then
        return
    end
    if st.timer > 1000 then
        st.timer = st.timer - 1000
        return
    end
    if st.phase < 8 then
        if st.phase == 1 then
            creature:Talk(SAY_INTRO_1)
            st.timer = 18000
        elseif st.phase == 2 then
            creature:Talk(SAY_INTRO_2)
            st.timer = 18000
        elseif st.phase == 3 then
            creature:Talk(SAY_WATER)
            creature:CastSpell(creature, SPELL_CONJURE_WATER)
            st.timer = 7000
        elseif st.phase == 4 then
            creature:Talk(SAY_BUFFS)
            creature:CastSpell(creature, SPELL_ICE_ARMOR)
            st.timer = 7000
        elseif st.phase == 5 then
            creature:Talk(SAY_DRINK)
            creature:CastSpell(creature, SPELL_ARCANE_INTELLECT)
            st.timer = 7000
        elseif st.phase == 6 then
            creature:Talk(SAY_READY)
            st.timer = 6000
        else
            -- Phase 7: the instance->SetData(DATA_WARDEN_2,
            -- DONE) arm is instance-blocked and the
            -- SetImmuneToNPC(false) arm has no immune bridge;
            -- the observable remainder is Init = true.
            introDone[guid] = true
            cancelIntro(guid)
            introState[guid] = nil
            return
        end
        st.phase = st.phase + 1
    end
end

local function startIntro(creature, guid)
    cancelIntro(guid)
    introDone[guid] = false
    introState[guid] = { phase = 1, timer = 2000 }
    introTimers[guid] = CreateLuaEvent(function()
        introTick(creature, guid)
    end, 1000, 0)
end

local function combatTick(creature, guid)
    local st = combatState[guid]
    if not st then
        return
    end
    if not st.lowhp and healthBelowPct(creature, 20) then
        creature:Talk(SAY_LOWHP)
        st.lowhp = true
    end
    if st.pyro <= 1000 then
        -- C++ waits for !IsNonMeleeSpellCast(false); no
        -- cast-state bridge (netherspite precedent), so the
        -- arm fires unconditionally.
        creature:Talk(SAY_PYRO)
        creature:CastSpell(nil, SPELL_PYROBLAST)
        st.pyro = 40000
    else
        st.pyro = st.pyro - 1000
    end
    if st.fire <= 1000 then
        creature:CastSpell(nil, SPELL_FIREBALL)
        st.fire = 4000
    else
        st.fire = st.fire - 1000
    end
end

RegisterCreatureEvent(ENTRY_MILLHOUSE, 5, function(_, creature)
    startIntro(creature, creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_MILLHOUSE, 1, function(_, creature)
    local guid = creature:GetGUID()
    cancelCombat(guid)
    combatState[guid] = { pyro = 1000, fire = 2500, lowhp = false }
    combatTimers[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_MILLHOUSE, 2, function(_, creature)
    local guid = creature:GetGUID()
    cancelCombat(guid)
    combatState[guid] = nil
    if not introDone[guid] then
        startIntro(creature, guid)
    end
end)

RegisterCreatureEvent(ENTRY_MILLHOUSE, 3, function(_, creature, victim)
    -- C++ `if (victim->GetTypeId() == TYPEID_PLAYER)
    -- Talk(SAY_KILL)` — kargath 3-arg-handler convention
    -- (C++-exact).
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end)

RegisterCreatureEvent(ENTRY_MILLHOUSE, 4, function(_, creature)
    local guid = creature:GetGUID()
    creature:Talk(SAY_DEATH)
    cancelIntro(guid)
    cancelCombat(guid)
    introState[guid] = nil
    introDone[guid] = nil
    combatState[guid] = nil
end)

RegisterCreatureEvent(ENTRY_MILLHOUSE, 23, function(_, creature)
    local guid = creature:GetGUID()
    cancelCombat(guid)
    combatState[guid] = nil
end)
