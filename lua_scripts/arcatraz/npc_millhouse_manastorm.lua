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
-- HARBINGER_SKYRISS is DONE — PORTED in ev23 below via the
-- bridged GetBossState (lua_creature_events.go:6624), with
-- the IsAlive guard (Go fires ev23 ahead of OnDied).
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
-- instance->SetData(DATA_WARDEN_2, DONE) arm is PORTED via
-- the redirect onto Mellichar's per-creature store
-- (npc_mellichar.lua) and the me->SetImmuneToNPC(false) arm
-- is PORTED via the bridged unit-flags surface (executioner
-- precedent), so the observable remainder is Init = true —
-- the pump self-cancels (the C++ ++Phase takes it to 8 and
-- the `Phase < 8` gate ends the machine).
-- OnEnterCombat(1): per-GUID reset to the C++ Initialize()
-- combat values (LowHp = false / Pyroblast_Timer = 1000 /
-- Fireball_Timer = 2500 — the C++ has no JustEngagedWith
-- override, so there is no engage yell) + a 500ms combat pump (a
-- port of the combat half of UpdateAI in C++ arm order — the
-- !UpdateVictim early return is PORTED as the GetVictim guard
-- in combatTick; the 2500ms fireball init is not a multiple of
-- 1000, so the pump runs at 500ms and all combat timers land
-- exact; the pump only exists while in combat). Combat pump: LowHp latch —
-- Talk(SAY_LOWHP) once when health drops strictly below 20%
-- (C++ HealthBelowPct(20), halazzi strict-fraction
-- precedent); pyroblast 33975 — 1s init then 40s, Talk(SAY_
-- PYRO), DoCastVictim (felmyst convention), re-arm 40000
-- (C++-exact; the C++ !IsNonMeleeSpellCast(false) early-
-- return gate is PORTED via creature:IsCasting() — while
-- casting, the tick returns before the arm, so pyroblast
-- stays due and fireball does not advance); fireball
-- 14034 — 2.5s init then 4s, DoCastVictim (felmyst
-- convention), re-arm 4000 (C++-exact). Melee is
-- engine-driven.
-- OnTargetDied(3): Talk(SAY_KILL) gated on the killed unit
-- being a player (C++-exact, kargath 3-arg-handler
-- convention). OnDied(4): Talk(SAY_DEATH) + cancel both pumps
-- + drop per-GUID state (the _JustDied arm is instance-
-- blocked; the FailQuest arm is commented out in C++
-- (arcatraz.cpp:64-66); FailQuest bridged, lua_player.go:4511). OnLeaveCombat(2): cancel the combat pump + drop
-- combat state, and restart the intro machine when it had not
-- completed (the C++ Reset() re-Initialize()s — Phase = 1 /
-- Init = false — and the DATA_WARDEN_2 gate now rides the
-- ev5 respawn check below, so the replay models the
-- pre-phase-7 evade case; once phase 7 completed,
-- DATA_WARDEN_2 is DONE in Mellichar's store and the intro
-- stays done, C++-exact for the reachable timeline). OnReset(23): cancel the combat pump +
-- drop combat state only — Eluna's On_Reset also fires ahead
-- of OnDied and OnSpawn (verified in engine/world/lua_
-- creature_events.go), so the intro restart stays on the
-- evade hook to avoid double pumps.
-- npc_warden_mellichar is PORTED whole in npc_mellichar.lua
-- (entry NPC_MELLICHAR = 20904, arcatraz.h): ev27 MoveInLOS
-- trigger + JustEngagedWith, the 7-phase UpdateAI poller,
-- DoPrepareForPhase, the summon arms, JustSummoned, the
-- FAIL -> Reset leg, and the Reset arms. Its yells
-- (YELL_INTRO1 = 0 / YELL_INTRO2 = 1 / YELL_RELEASE1 = 2 /
-- YELL_RELEASE2A = 3 / YELL_RELEASE2B = 4 / YELL_RELEASE3
-- = 5 / YELL_RELEASE4 = 6 / YELL_WELCOME = 7) live there.
-- The 05:33 head-entry unblock stands: InterruptNonMeleeSpells
-- + SummonCreature + RemoveAurasByType are bridged, the
-- HandleGameObject arms ride FindNearestGameObject +
-- SetGoState, and the instance DATA_WARDEN_* data rides
-- Mellichar's per-creature store with all writers redirected
-- (this file's phase-7 write, the wave-mob death relays).
-- CORRECTION to the earlier note here: ev27 is NOT
-- unreachable for Mellichar — the Go sight scan does not
-- gate on UNIT_FLAG_NON_ATTACKABLE (creaturemotion.go:9438)
-- and his faction (16, TDB creature_template) is hostile,
-- so the scan fires ev27 and the handler vetoes the engage.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — the DATA_WARDEN_2 read in Reset()
-- and the phase-7 SetData arm are PORTED via the redirect
-- onto Mellichar's per-creature store (npc_mellichar.lua:
-- the ev5 respawn check reads it, the introTick phase-7 arm
-- writes it; the DATA_HARBINGER_SKYRISS read is PORTED as
-- the SAY_COMPLETE arm in ev23 below; instance_arcatraz.cpp
-- stays blocked on the instance-script model); the Reset()
-- instance-gated SetImmuneToNPC(false) arm stays fused to
-- its unported instance gate (the phase-7 arm is PORTED via
-- the bridged unit-flags surface — see introTick); the AttackStart
-- MoveChase(who, 25.0f) arm is PORTED in ev1 below (MoveChase
-- bridged, lua_creature_events.go:5252); the pyroblast
-- IsNonMeleeSpellCast(false) early-return gate is PORTED
-- via creature:IsCasting() (live CastingUntil window,
-- lua_creature_events.go:2275): while casting, combatTick
-- returns before the pyroblast arm, so the pyroblast timer
-- stays due and the fireball timer does not advance that
-- tick (C++ return semantics); the intro machine and LowHp
-- latch sit before the gate in C++ arm order and stay
-- ungated; the unused C++ spells
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
local SAY_COMPLETE = 11

local SPELL_CONJURE_WATER = 36879
local SPELL_ARCANE_INTELLECT = 36880
local SPELL_ICE_ARMOR = 36881
local SPELL_PYROBLAST = 33975
local SPELL_FIREBALL = 14034

local ENTRY_MILLHOUSE = 20977
local ENTRY_MELLICHAR = 20904 -- arcatraz.h AZCreatureIds
local DATA_WARDEN_2 = 6 -- arcatraz.h:43

local introTimers = {}
local introState = {}
local introDone = {}
local combatTimers = {}
local combatState = {}

local function healthBelowPct(creature, pct)
    local maxHealth = creature:GetMaxHealth()
    return maxHealth > 0
        and creature:GetHealth() < math.floor(maxHealth * pct / 100)
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
            -- DONE) arm (C++ arcatraz.cpp:180) is PORTED via the
            -- redirect onto Mellichar's per-creature store
            -- (npc_mellichar.lua — the sole C++ reader is
            -- Mellichar's CanProgress phase-5 gate); C++ arcatraz.
            -- cpp:182 me->SetImmuneToNPC(false) = RemoveFlag(
            -- UNIT_FIELD_FLAGS = 59, UNIT_FLAG_IMMUNE_TO_NPC
            -- 0x200; Unit.cpp:8476-8495) is PORTED via the bridged
            -- unit-flags surface; the observable remainder is
            -- Init = true.
            introDone[guid] = true
            creature:RemoveFlag(59, 0x200)
            local mellichar = creature:FindNearestCreature(ENTRY_MELLICHAR, 200)
            if mellichar ~= nil then
                mellichar:SetData(DATA_WARDEN_2, 3)
            end
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
    -- C++ UpdateAI !UpdateVictim early return (arcatraz.cpp:190-191):
    -- no victim, no LowHp talk and no casts this tick (death/evade
    -- race — the pump is cancelled by ev2/ev4, but a tick can land
    -- between the victim clearing and the cancel).
    if creature:GetVictim() == nil then
        return
    end
    if not st.lowhp and healthBelowPct(creature, 20) then
        creature:Talk(SAY_LOWHP)
        st.lowhp = true
    end
    if st.pyro <= 500 then
        -- C++ IsNonMeleeSpellCast(false) early return
        -- (arcatraz.cpp:202): while casting, the tick ends
        -- here — pyroblast stays due and fireball does not
        -- advance this tick.
        if creature:IsCasting() then
            return
        end
        creature:Talk(SAY_PYRO)
        creature:CastSpell(nil, SPELL_PYROBLAST)
        st.pyro = 40000
    else
        st.pyro = st.pyro - 500
    end
    if st.fire <= 500 then
        creature:CastSpell(nil, SPELL_FIREBALL)
        st.fire = 4000
    else
        st.fire = st.fire - 500
    end
end

RegisterCreatureEvent(ENTRY_MILLHOUSE, 5, function(_, creature)
    local guid = creature:GetGUIDLow()
    -- C++ Reset() (arcatraz.cpp:104-110): DATA_WARDEN_2 DONE
    -- -> Init = true, SetImmuneToNPC(false). The read rides
    -- Mellichar's store (npc_mellichar.lua redirect); on a
    -- fresh instance the key is unset (0) and the intro runs.
    local mellichar = creature:FindNearestCreature(ENTRY_MELLICHAR, 200)
    if mellichar ~= nil and mellichar:GetData(DATA_WARDEN_2) == 3 then
        introDone[guid] = true
        creature:RemoveFlag(59, 0x200)
        return
    end
    startIntro(creature, guid)
end)

RegisterCreatureEvent(ENTRY_MILLHOUSE, 1, function(_, creature, target)
    local guid = creature:GetGUIDLow()
    cancelCombat(guid)
    combatState[guid] = { pyro = 1000, fire = 2500, lowhp = false }
    -- C++ AttackStart (arcatraz.cpp:117-124): MoveChase(who, 25.0f)
    -- (lua_creature_events.go:5252; the chase target is the combat
    -- victim, so the pursuit leg honors the 25yd stop).
    if target then
        creature:MoveChase(target, 25.0)
    end
    combatTimers[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 500, 0)
end)

RegisterCreatureEvent(ENTRY_MILLHOUSE, 2, function(_, creature)
    local guid = creature:GetGUIDLow()
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
    local guid = creature:GetGUIDLow()
    creature:Talk(SAY_DEATH)
    cancelIntro(guid)
    cancelCombat(guid)
    introState[guid] = nil
    introDone[guid] = nil
    combatState[guid] = nil
end)

RegisterCreatureEvent(ENTRY_MILLHOUSE, 23, function(_, creature)
    local guid = creature:GetGUIDLow()
    cancelCombat(guid)
    combatState[guid] = nil
    -- C++ Reset() (arcatraz.cpp:112-113): Talk(SAY_COMPLETE) when
    -- Skyriss is DONE (GetBossState bridged, lua_creature_events.go:
    -- 6624). The IsAlive guard is required: Go fires ev23 ahead of
    -- OnDied (lua_creature_events.go:7349), while C++ Reset() never
    -- runs on death.
    if creature:IsAlive() and creature:GetBossState(3) == 3 then
        creature:Talk(SAY_COMPLETE)
    end
end)
