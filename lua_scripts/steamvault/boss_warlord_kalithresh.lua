-- Warlord Kalithresh (The Steamvault) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/SteamVault/
-- boss_warlord_kalithresh.cpp (boss_warlord_kalithresh,
-- npc_naga_distiller — the two AI classes AddSC_boss_warlord_
-- kalithresh registers; the distiller Reset + DamageTaken arms
-- are PORTED below and its StartRageGen rides the boss rage
-- arm). Third and final boss in the Steam Vault set, after
-- Mekgineer Steamrigger per outland_script_loader.cpp order;
-- instance_steam_vault.lua PORTED (6ca5a6e0). Entry 17798 verified from the C++
-- sources (steam_vault.h: NPC_WARLORD_KALITHRESH = 17798;
-- instance_steam_vault.cpp maps it to DATA_WARLORD_KALITHRESH
-- = 2); entry 17954 for the Naga Distiller is the C++ file's
-- own FindNearestCreature(17954, 100.0f) constant. The
-- creature_template ScriptName bindings are DB-side (no TDB
-- in this workspace).
-- Talk lines used: SAY_REGEN=1 (rage start — fires only with
-- the distiller arm, below), SAY_AGGRO=2 (pull), SAY_SLAY=3
-- (kill — NO TYPEID gate in C++, C++-exact, morogrim/vashj
-- precedent), SAY_DEATH=4 (death); SAY_INTRO=0 is defined but
-- never called in the file (unused enum member, documented
-- only, gruul precedent); the bool CanRage member is
-- written once in Initialize() and never read (unused member,
-- documented only).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent
-- (per-GUID scheduler pump); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelastrasz convention);
-- DoCastVictim takes nil as the victim arm (felmyst convention).
-- Fight shape (C++-exact for the modeled arms):
-- boss_warlord_kalithresh (17798): OnEnterCombat(1):
-- per-GUID scheduler reset to the C++ Initialize() values
-- (reflection 10000 / impale 7000+rand32()%7000 / rage 45000)
-- + Talk(SAY_AGGRO) + 1s scheduler pump (a port of UpdateAI —
-- 1s granularity is exact for all C++ timers here; the
-- !UpdateVictim early return collapses into the pump, which
-- only runs in combat). Pump: spell reflection 31534 10s then
-- 15000+rand32()%10000 -> DoCast(me), re-arm (C++-exact).
-- Impale 39061 7000+rand32()%7000 then 7500+rand32()%5000 ->
-- target = threat-list any-unit (C++ SelectTarget(Random, 0),
-- salramm idiom), nil pick casts nothing, re-arm regardless
-- (C++-exact). The rage arm is PORTED below
-- (FindNearestCreature(17954, 100.0f) gate + Talk(SAY_REGEN) +
-- DoCast(me, 37081) + distiller StartRageGen relay; first fire
-- 45000, re-arm 3000+rand32()%15000, C++-exact). The SpellHit arm
-- (SPELL_WARLORDS_RAGE_PROC 36453 -> RemoveAurasDueToSpell when
-- instance DATA_DISTILLER == DONE) is PORTED below (ev14).
-- OnTargetDied(3):
-- Talk(SAY_SLAY) (no TYPEID gate, C++-exact). OnDied(4):
-- Talk(SAY_DEATH) + cleanup (the SetBossState DONE arm is
-- PORTED below). OnLeaveCombat(2)/OnReset(23): cancel the
-- pump, drop per-GUID state (the Initialize() reset and the
-- SetBossState NOT_STARTED arm are skipped — no consumer in
-- Lua; ev23 fires on the death path too, midnight precedent).
-- npc_naga_distiller (17954): Reset + DamageTaken PORTED below
-- (RegisterCreatureEvent, no boss_ai.go entry needed — the
-- water-elemental precedent in boss_hydromancer_thespia.lua);
-- its StartRageGen (RemoveFlag pair + triggered DoCast(me, SPELL_
-- WARLORDS_RAGE_NAGA 31543) + instance SetData(DATA_DISTILLER,
-- IN_PROGRESS)) rides the boss rage arm (the distiller object
-- found by FindNearestCreature is the bearer); its
-- JustEngagedWith is empty in C++ (no arm).
-- Deliberate deviations: the instance-script model is PORTED
-- (instance_steam_vault.lua — the DATA_DISTILLER state machine
-- in StartRageGen/DamageTaken/SpellHit rides it, and the boss
-- SetBossState DONE arm is ported in ev4); boss admission via
-- the luaBossAI shim for the rest; the DATA_WARLORD_KALITHRESH
-- NOT_STARTED/IN_PROGRESS bookkeeping in Reset/JustEngagedWith
-- stays out (no consumer in Lua; ev23 fires on the death path
-- too, midnight precedent); FindNearestCreature IS bridged
-- (lua_creature_events.go:6280) and the distiller object it
-- returns is the StartRageGen bearer (the old "no cross-
-- creature bridge" claim is retracted — the kill-the-distiller
-- counterplay is ported with the rage, not fabricated without
-- it); no SpellScript/AuraScript scripts in this file.

local SPELL_SPELL_REFLECTION = 31534
local SPELL_IMPALE = 39061
local SPELL_WARLORDS_RAGE = 37081
local SPELL_WARLORDS_RAGE_NAGA = 31543
local SPELL_WARLORDS_RAGE_PROC = 36453

local SAY_REGEN = 1
local SAY_AGGRO = 2
local SAY_SLAY = 3
local SAY_DEATH = 4

local ENTRY_KALITHRESH = 17798
local ENTRY_NAGA_DISTILLER = 17954

local DATA_DISTILLER = 3 -- steam_vault.h DATA_DISTILLER
local ENCOUNTER_IN_PROGRESS = 1
local ENCOUNTER_DONE = 3 -- EncounterState (InstanceScript.h:68-76)

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local timers = {}
local states = {}

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
end

-- instance_steam_vault.lua owns the SetData switch; the boss
-- file reaches it through the shared instanceData table
-- (slad_ran.lua:261-263 precedent: creature:GetMap():GetInstanceData()).
local function instanceDataOf(creature)
    local mapObj = creature:GetMap()
    return mapObj and mapObj:GetInstanceData() or nil
end

-- C++ SelectTarget draws the threat list (UnitAI.h:217-218): position 0
-- takes any entry; playerOnly defaults false, so any threat-list unit
-- qualifies (not just players), with no distance filter (salramm
-- idiom). Dead members are offline refs in C++ (skipped at list
-- build) so they are filtered here. The old instance-player pool is
-- retired (threat-list precedent, Oct-25 strips). Nil when none
-- present — the caller casts nothing (C++-exact).
local function pickRandomThreatTarget(creature)
    local found = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            found[#found + 1] = t
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ Initialize() values: Reflection_Timer 10000,
-- Impale_Timer 7000 + rand32() % 7000, Rage_Timer 45000.
local function freshState()
    return {
        reflection = 10000,
        impale = 7000 + math.random(0, 6999),
        rage = 45000,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Warlord's rage 37081: 45s then 3000+rand32()%15000.
    -- C++ boss_warlord_kalithresh.cpp:172-181: the fire arm is
    -- FindNearestCreature(17954, 100.0f)-gated; Talk(SAY_REGEN)
    -- + DoCast(me, 37081) + distiller StartRageGen (:81-89:
    -- RemoveFlag pair + triggered DoCast(me, 31543) + instance
    -- SetData(DATA_DISTILLER, IN_PROGRESS)) — the distiller
    -- object found by the gate IS the bearer (FindNearestCreature
    -- is bridged, lua_creature_events.go:6280); nil pick casts
    -- nothing, re-arm regardless (C++-exact).
    if st.rage <= 1000 then
        local distiller = creature:FindNearestCreature(ENTRY_NAGA_DISTILLER, 100)
        if distiller then
            creature:Talk(SAY_REGEN)
            creature:CastSpell(creature, SPELL_WARLORDS_RAGE)
            distiller:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            distiller:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
            distiller:CastSpell(distiller, SPELL_WARLORDS_RAGE_NAGA, true)
            local data = instanceDataOf(creature)
            if data then
                data:SetData(DATA_DISTILLER, ENCOUNTER_IN_PROGRESS)
            end
        end
        st.rage = 3000 + math.random(0, 14999)
    else
        st.rage = st.rage - 1000
    end

    -- Spell reflection 31534: 10s then 15000+rand32()%10000,
    -- DoCast(me) (C++-exact).
    if st.reflection <= 1000 then
        creature:CastSpell(creature, SPELL_SPELL_REFLECTION)
        st.reflection = 15000 + math.random(0, 9999)
    else
        st.reflection = st.reflection - 1000
    end

    -- Impale 39061: 7000+rand32()%7000 then 7500+rand32()%5000;
    -- threat-list any-unit pick, nil pick casts nothing, re-arm
    -- regardless (C++-exact).
    if st.impale <= 1000 then
        local target = pickRandomThreatTarget(creature)
        if target then
            creature:CastSpell(target, SPELL_IMPALE)
        end
        st.impale = 7500 + math.random(0, 4999)
    else
        st.impale = st.impale - 1000
    end
end

RegisterCreatureEvent(ENTRY_KALITHRESH, 1, function(_, creature)
    local guid = creature:GetGUIDLow()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_KALITHRESH, 2, function(_, creature)
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_KALITHRESH, 3, function(_, creature)
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_KALITHRESH, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    creature:SetBossState(2, 3) -- DATA_WARLORD_KALITHRESH, DONE (boss_warlord_kalithresh.cpp: Talk then SetBossState)
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_KALITHRESH, 23, function(_, creature)
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_KALITHRESH, 14, function(_, creature, _, spellId)
    -- C++ boss_warlord_kalithresh.cpp:152-157: SPELL_WARLORDS_RAGE_PROC
    -- (36453) hit while instance DATA_DISTILLER == DONE -> the proc
    -- aura comes off (RemoveAura twin of RemoveAurasDueToSpell).
    if spellId == SPELL_WARLORDS_RAGE_PROC then
        local data = instanceDataOf(creature)
        if data and data.distillerState == ENCOUNTER_DONE then
            creature:RemoveAura(SPELL_WARLORDS_RAGE_PROC)
        end
    end
end)

-- npc_naga_distiller (17954): Reset + DamageTaken PORTED —
-- the instance file stores the distiller state and the boss
-- rage arm above is the StartRageGen bearer, so the distiller
-- has live Lua sites now (the "empty skeleton" objection in
-- the old header is retired).
local function resetDistiller(creature)
    -- C++ npc_naga_distillerAI::Reset (:66-77): flag pair set,
    -- then cleared while the distiller state is IN_PROGRESS.
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    local data = instanceDataOf(creature)
    if data and (data.distillerState or 0) == ENCOUNTER_IN_PROGRESS then
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    end
end

RegisterCreatureEvent(ENTRY_NAGA_DISTILLER, 5, function(_, creature)
    resetDistiller(creature)
end)

RegisterCreatureEvent(ENTRY_NAGA_DISTILLER, 2, function(_, creature)
    resetDistiller(creature)
end)

RegisterCreatureEvent(ENTRY_NAGA_DISTILLER, 23, function(_, creature)
    resetDistiller(creature)
end)

RegisterCreatureEvent(ENTRY_NAGA_DISTILLER, 9, function(_, creature, _, damage)
    -- C++ npc_naga_distillerAI::DamageTaken (:91-95): lethal hit
    -- -> instance SetData(DATA_DISTILLER, DONE). DamageTaken fires
    -- on lethal hits (nefarian-form rule), so no postDamage guard;
    -- the pre-damage health check is the C++ check verbatim.
    if creature:GetHealth() <= damage then
        local data = instanceDataOf(creature)
        if data then
            data:SetData(DATA_DISTILLER, ENCOUNTER_DONE)
        end
    end
end)
