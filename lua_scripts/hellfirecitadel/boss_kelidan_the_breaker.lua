-- Kelidan the Breaker (Blood Furnace, Hellfire Citadel) — Lua port of
-- src/server/scripts/Outland/HellfireCitadel/BloodFurnace/
-- boss_kelidan_the_breaker.cpp (boss_kelidan_the_breaker — the only
-- CreatureScript AI class AddSC_boss_kelidan_the_breaker registers;
-- npc_shadowmoon_channeler is the add AI class registered alongside in
-- this file, ported separately). Second boss in the Blood Furnace set
-- per outland_script_loader.cpp order (instance_blood_furnace.cpp stays
-- blocked on the instance-script model).
-- Entry: NPC_KELIDAN_THE_BREAKER = 17377 in blood_furnace.h (also
-- mapped in instance_blood_furnace.cpp OnCreatureCreate { NPC_KELIDAN_
-- THE_BREAKER, DATA_KELIDAN_THE_BREAKER }); the creature_template
-- ScriptName bindings are DB-side (no TDB in this workspace).
-- Talk: SAY_WAKE = 0 (pull), SAY_KILL = 2 (kill — the C++ gates with
-- rand32()%2, C++-exact), SAY_NOVA = 3 (burning nova), SAY_DIE = 4
-- (death). SAY_ADD_AGGRO = 1 fires only from the unbridgeable
-- ChannelerEngaged cross-creature relay — unreachable, no event 1
-- talk for it.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent
-- (per-GUID scheduler pump); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast default is
-- triggered=false (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Kelidan (17377):
-- OnEnterCombat(1): per-GUID reset to the C++ Initialize() values
-- (shadowVolley 1000 / burningNova 15000 / corruption 5000 / firenova
-- false — the C++ JustEngagedWith does not schedule anything; the
-- InterruptNonMeleeSpells arm has no unit-state bridge and the
-- DoStartMovement arm has no movement bridge) + Talk(SAY_WAKE) + 1s
-- scheduler pump (a port of UpdateAI — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses into the
-- pump). Pump: the Firenova machine first — while firenova is set
-- nothing else fires (the C++ early return is C++-exact): fire nova
-- 33132 (H_SPELL_FIRE_NOVA 37371 — no difficulty bridge) 5s after
-- burning nova — triggered DoCastSelf, firenova=false afterwards,
-- ShadowVolley_Timer=2000 (C++-exact); shadow bolt volley 28599
-- (H_SPELL_SHADOW_BOLT_VOLLEY 40070 — no difficulty bridge): 1s init
-- then 5000+rand32()%8000 (C++ {5s,13s}) — non-triggered DoCastSelf
-- (C++ DoCast(me), C++-exact), re-arm {5s,13s} regardless
-- (C++-exact); corruption 30938: 5s init then 30000+rand32()%20000
-- (C++ {30s,50s}) — non-triggered DoCastSelf (C++-exact), re-arm
-- {30s,50s} regardless (C++-exact); burning nova: 15s init then
-- 20000+rand32()%8000 (C++ {20s,28s}) — the InterruptNonMeleeSpells
-- arm has no unit-state bridge; Talk(SAY_NOVA); the AddAura(SPELL_
-- BURNING_NOVA 30940, me) arm has no aura bridge (the 5s firenova
-- machine drives the explosion — timer bookkeeping exact); the
-- IsHeroic() DoTeleportAll arm has no teleport/difficulty bridge;
-- firenova=true, firenovaTimer=5000, re-arm {20s,28s} regardless
-- (C++-exact).
-- OnTargetDied(3): 50% Talk(SAY_KILL) (C++ `if (rand32() % 2)
-- return;` — C++-exact). OnDied(4): Talk(SAY_DIE) + cleanup (the
-- _JustDied arm is instance-blocked). OnLeaveCombat(2)/OnReset(23):
-- cancel the pump, drop per-GUID state (the _Reset arm is
-- instance-blocked; the Reset() REACT_PASSIVE + UNIT_FLAG_NON_
-- ATTACKABLE + SetImmuneToAll(true) arms have no flag/react/immune
-- bridges — thespia/kalithresh precedent — and the
-- SummonChannelers() arm has no summon bridge, steamrigger
-- precedent).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (instance_blood_furnace.cpp stays blocked on the instance-script
-- model; the BossAI ctor DATA_KELIDAN_THE_BREAKER = 2 bookkeeping
-- in Reset/JustEngagedWith/JustDied skipped); no summon bridge —
-- the whole SummonChannelers machine (5 channelers at the
-- ShadowmoonChannelers spawn points, TEMPSUMMON_CORPSE_TIMED_
-- DESPAWN) unmodeled; no ObjectAccessor/cross-creature bridge —
-- ChannelerEngaged (the addYell-gated SAY_ADD_AGGRO talk and the
-- channeler AttackStart(who) relay), ChannelerDied (the all-
-- channelers-dead activation: REACT_AGGRESSIVE +
-- RemoveFlag(NON_ATTACKABLE) + SetImmuneToAll(false) + AttackStart
-- (killer)) and GetChanneled unmodeled, so SAY_ADD_AGGRO is
-- unreachable in this model; no flag/react/immune bridges — the
-- pre-fight passive/non-attackable/immune channeling state
-- unmodeled, and with it the !UpdateVictim evocation (SPELL_
-- EVOCATION 30935) check_Timer arm, which only fires in that
-- state; no unit-state bridge — the JustEngagedWith and burning-
-- nova InterruptNonMeleeSpells arms unmodeled (mennu/thespia
-- precedent); no aura bridge — the burning-nova AddAura(30940)
-- arm unmodeled (timer bookkeeping exact); no teleport bridge —
-- the heroic DoTeleportAll arm unmodeled; no difficulty bridge —
-- the IsHeroic() arms and H_SPELL_FIRE_NOVA 37371 / H_SPELL_
-- SHADOW_BOLT_VOLLEY 40070 unmodeled (thespia precedent); no
-- movement bridge — the JustEngagedWith DoStartMovement arm
-- unmodeled; SPELL_VORTEX 37370 is an unused enum member —
-- never referenced in the file (gruul unused-enum precedent).

local SPELL_SHADOW_BOLT_VOLLEY = 28599
local SPELL_CORRUPTION = 30938
local SPELL_FIRE_NOVA = 33132

local SAY_WAKE = 0
local SAY_KILL = 2
local SAY_NOVA = 3
local SAY_DIE = 4

local ENTRY_KELIDAN = 17377

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

-- C++ Initialize() values: ShadowVolley_Timer=1000,
-- BurningNova_Timer=15000, Corruption_Timer=5000, check_Timer=0
-- (the evocation check arm only fires in the unbridgeable
-- passive/immune channeling state — documented above), Firenova=
-- false, addYell=false (the ChannelerEngaged relay is
-- cross-creature blocked — documented above).
local function freshState()
    return {
        shadowVolley = 1000,
        burningNova = 15000,
        corruption = 5000,
        firenova = false,
        firenovaTimer = 0,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Firenova machine: while firenova is set, nothing else fires
    -- (the C++ early return, C++-exact). Fire nova 33132
    -- (heroic 37371 — no difficulty bridge): triggered DoCastSelf
    -- (C++ DoCast(me, SPELL_FIRE_NOVA, true), C++-exact),
    -- firenova=false afterwards, ShadowVolley_Timer=2000.
    if st.firenova then
        if st.firenovaTimer <= 1000 then
            creature:CastSpell(creature, SPELL_FIRE_NOVA, true)
            st.firenova = false
            st.shadowVolley = 2000
        else
            st.firenovaTimer = st.firenovaTimer - 1000
        end
        return
    end

    -- Shadow bolt volley 28599 (heroic 40070 — no difficulty
    -- bridge): 1s init then 5000+rand32()%8000 (C++ {5s,13s}) —
    -- non-triggered DoCastSelf (C++ DoCast(me), C++-exact),
    -- re-arm {5s,13s} regardless (C++-exact).
    if st.shadowVolley <= 1000 then
        creature:CastSpell(creature, SPELL_SHADOW_BOLT_VOLLEY)
        st.shadowVolley = 5000 + math.random(0, 7999)
    else
        st.shadowVolley = st.shadowVolley - 1000
    end

    -- Corruption 30938: 5s init then 30000+rand32()%20000 (C++
    -- {30s,50s}) — non-triggered DoCastSelf (C++-exact), re-arm
    -- {30s,50s} regardless (C++-exact).
    if st.corruption <= 1000 then
        creature:CastSpell(creature, SPELL_CORRUPTION)
        st.corruption = 30000 + math.random(0, 19999)
    else
        st.corruption = st.corruption - 1000
    end

    -- Burning nova: 15s init then 20000+rand32()%8000 (C++
    -- {20s,28s}) — the InterruptNonMeleeSpells arm has no
    -- unit-state bridge; Talk(SAY_NOVA); the AddAura(30940) arm
    -- has no aura bridge and the heroic DoTeleportAll arm has no
    -- teleport/difficulty bridge (documented above); firenova=
    -- true with a 5s firenovaTimer, re-arm {20s,28s} regardless
    -- (C++-exact).
    if st.burningNova <= 1000 then
        creature:Talk(SAY_NOVA)
        st.burningNova = 20000 + math.random(0, 7999)
        st.firenovaTimer = 5000
        st.firenova = true
    else
        st.burningNova = st.burningNova - 1000
    end
end

RegisterCreatureEvent(ENTRY_KELIDAN, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_WAKE)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_KELIDAN, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_KELIDAN, 3, function(_, creature)
    -- C++ `if (rand32() % 2) return;` — 50% Talk(SAY_KILL)
    -- (C++-exact).
    if math.random(0, 1) ~= 0 then
        return
    end
    creature:Talk(SAY_KILL)
end)

RegisterCreatureEvent(ENTRY_KELIDAN, 4, function(_, creature)
    creature:Talk(SAY_DIE)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_KELIDAN, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
