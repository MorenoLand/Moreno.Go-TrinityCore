-- Warlord Kalithresh (The Steamvault) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/SteamVault/
-- boss_warlord_kalithresh.cpp (boss_warlord_kalithresh,
-- npc_naga_distiller — the two AI classes AddSC_boss_warlord_
-- kalithresh registers; the distiller is documented only —
-- its whole AI is flag/cross-creature/instance gated, below —
-- not registered). Third and final boss in the Steam Vault
-- set, after Mekgineer Steamrigger per outland_script_loader.
-- cpp order; instance_steam_vault.cpp stays blocked on the
-- instance-script model. Entry 17798 verified from the C++
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
-- only, gruul precedent); the bool CanRage member is never
-- read or written anywhere (unused member, documented only).
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
-- target = random alive player in the instance (C++
-- SelectTarget(Random, 0), thespia precedent), nil pick casts
-- nothing, re-arm regardless (C++-exact). The rage arm has no
-- bridge — the whole fire arm is FindNearestCreature(17954,
-- 100.0f)-gated and the fire arm is cross-creature (ENSURE_AI
-- -> StartRageGen on the distiller); only timer bookkeeping is
-- kept (first fire 45000, re-arm 3000+rand32()%15000,
-- C++-exact). The SpellHit arm (SPELL_WARLORDS_RAGE_PROC
-- 36453 -> RemoveAurasDueToSpell when instance DATA_
-- DISTILLER == DONE) is instance-blocked. OnTargetDied(3):
-- Talk(SAY_SLAY) (no TYPEID gate, C++-exact). OnDied(4):
-- Talk(SAY_DEATH) + cleanup (the SetBossState DONE arm is
-- instance-blocked). OnLeaveCombat(2)/OnReset(23): cancel the
-- pump, drop per-GUID state (the Initialize() reset and the
-- SetBossState NOT_STARTED arm are instance-blocked).
-- npc_naga_distiller (17954): documented only, NOT registered
-- — its Reset SetFlag(UNIT_FLAG_NOT_SELECTABLE) /
-- UNIT_FLAG_NON_ATTACKABLE arms have no flag bridge, its
-- StartRageGen (RemoveFlag pair + triggered DoCast(me, SPELL_
-- WARLORDS_RAGE_NAGA 31543) + instance SetData(DATA_DISTILLER,
-- IN_PROGRESS)) fires only cross-creature from the boss's
-- rage arm (no cross-creature bridge), and its DamageTaken ->
-- instance SetData(DATA_DISTILLER, DONE) relay is instance-
-- blocked — so a registration would be an empty skeleton
-- (vashj shield-generator-channel precedent).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (the DATA_WARLORD_KALITHRESH NOT_STARTED/IN_PROGRESS/DONE
-- bookkeeping in Reset/JustEngagedWith/JustDied and the DATA_
-- DISTILLER state machine in StartRageGen/DamageTaken/SpellHit
-- skipped); no FindNearestCreature/cross-creature bridge — the
-- rage Talk(SAY_REGEN) + self-cast of SPELL_WARLORDS_RAGE
-- 37081 + the distiller StartRageGen relay unmodeled (timer
-- bookkeeping only); no flag bridge — the distiller's
-- NOT_SELECTABLE/NON_ATTACKABLE setup unmodeled; no
-- SpellScript/AuraScript scripts in this file.

local SPELL_SPELL_REFLECTION = 31534
local SPELL_IMPALE = 39061

local SAY_REGEN = 1
local SAY_AGGRO = 2
local SAY_SLAY = 3
local SAY_DEATH = 4

local ENTRY_KALITHRESH = 17798

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

local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ SelectTarget(Random, 0): random alive player in the
-- instance (thespia precedent); nil when none present — the
-- caller casts nothing (C++-exact).
local function pickRandomPlayer(creature)
    local found = playersInInstance(creature)
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

    -- Warlord's rage 37081: 45s then 3000+rand32()%15000. The
    -- whole fire arm is FindNearestCreature(17954, 100.0f) +
    -- cross-creature gated (Talk(SAY_REGEN) + DoCast(me, 37081)
    -- + distiller StartRageGen) — no bridges, so timer
    -- bookkeeping only (C++-exact).
    if st.rage <= 1000 then
        st.rage = 3000 + math.random(0, 14999)
    else
        st.rage = st.rage - 1000
    end

    -- Spell reflection 31534: 10s then 15000+rand32()%10000,
    -- DoCast(me) (C++-exact).
    if st.reflection <= 1000 then
        creature:CastSpell(nil, SPELL_SPELL_REFLECTION)
        st.reflection = 15000 + math.random(0, 9999)
    else
        st.reflection = st.reflection - 1000
    end

    -- Impale 39061: 7000+rand32()%7000 then 7500+rand32()%5000;
    -- random alive player pick, nil pick casts nothing, re-arm
    -- regardless (C++-exact).
    if st.impale <= 1000 then
        local target = pickRandomPlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_IMPALE)
        end
        st.impale = 7500 + math.random(0, 4999)
    else
        st.impale = st.impale - 1000
    end
end

RegisterCreatureEvent(ENTRY_KALITHRESH, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_KALITHRESH, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_KALITHRESH, 3, function(_, creature)
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_KALITHRESH, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_KALITHRESH, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
