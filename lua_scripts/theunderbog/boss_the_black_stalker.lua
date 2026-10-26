-- The Black Stalker (The Underbog) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/TheUnderbog/
-- boss_the_black_stalker.cpp (boss_the_black_stalker — the
-- only AI class AddSC_boss_the_black_stalker registers; no
-- other creature/GO scripts in this file). Second and final
-- boss in The Underbog set per outland_script_loader.cpp
-- order (hungarfen 1/2 closed; instance_the_underbog.cpp is a
-- placeholder and stays blocked on the instance-script
-- model).
-- Entry 17882 verified externally (wotlk.ezhead npc=17882,
-- wow-petopia npc=17882, db.moonwell npc=17882, tauri
-- npc=17882); the_underbog.h defines NO NPC_ entries and
-- instance_the_underbog.cpp has NO OnCreatureCreate mappings
-- at all — the entry is not verifiable from the C++ sources,
-- documented here. The creature_template ScriptName bindings
-- are DB-side (no TDB in this workspace).
-- The file has NO Talk calls (no Say enum at all) and its
-- KilledUnit override does not exist — no event 3 registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pump); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): The Black
-- Stalker (17882): OnEnterCombat(1): per-GUID reset to the
-- C++ Initialize() values (levitate 12000 / chainLightning
-- 6000 / staticCharge 10000 / sporeStriders 10000+rand32()%
-- 5000 / check 5000 / InAir false — the C++ JustEngagedWith
-- override is empty) + 1s scheduler pump (a port of UpdateAI
-- — 1s granularity exact for all C++ timers; the
-- !UpdateVictim early return collapses into the pump). Pump:
-- levitate 31704: 12s init then 12000+rand32()%3000 — target
-- = random alive threat-list unit with the threat head skipped
-- (C++ SelectTarget(Random, 1), offset-before-filter,
-- steamrigger convention), nil pick casts nothing,
-- non-triggered DoCast on the target, re-arm
-- regardless (C++-exact — the re-arm is outside the if
-- (target) block). Chain lightning 31717: 6s init then 7000
-- — random alive threat-list unit (C++ SelectTarget
-- (Random, 0) defaults, kalithresh precedent), nil pick casts
-- nothing, re-arm 7000 regardless (C++-exact). Static charge
-- 31715: 10s init then 10000 — random alive threat-list
-- player within 30 yd (C++ SelectTarget(Random, 0, 30, true),
-- thespia distance-gate convention), nil pick casts nothing,
-- re-arm 10000 regardless (C++-exact). Spore striders 38755:
-- 10000+rand32()%5000 init then 10000+rand32()%5000 — timer
-- bookkeeping exact; the IsHeroic()-gated fire arm IS ported
-- (creature:IsHeroic(), see deliberate deviations below);
-- re-arm regardless (C++-exact). JustSummoned(19): spore
-- strider AttackStart relay PORTED (entry-gated 22299, random
-- threat-list unit with the threat head skipped or victim
-- fallback, EngageWithTarget
-- — skyriss ev19 precedent; Go spell-summon funnel fires ev19
-- on the caster). OnDied(4): cleanup (the
-- Striders.DespawnAll arm is summon-bridgeless, steamrigger
-- precedent). OnLeaveCombat(2)/OnReset(23): cancel the pump,
-- drop per-GUID state (the _Reset arm is instance-blocked;
-- the Reset() Striders.DespawnAll arm is summon-bridgeless).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (instance_the_underbog.cpp stays blocked on the
-- instance-script model); difficulty is bridged —
-- creature:IsHeroic() (per-instance difficulty, lua_creature_events.go)
-- drives the heroic-only spore-strider summon fire arm; no ObjectAccessor/cross-creature
-- bridge — the LevitatedTarget followup machine is PORTED
-- below (GUID->player scan + HasAura gate + SUSPENSION leg;
-- only the MAGNETIC_PULL 31705 player self-cast leg stays out
-- — no player self-cast bridge); no
-- home-position bridge exists (lua_creature_events.go) but
-- EnterEvadeMode has no bridge — the check_Timer 60-yd evade
-- arm unmodeled (timer bookkeeping exact; the re-arm from
-- 5000 to 1000 is C++-exact); the JustSummoned AttackStart
-- relay IS ported (above) — only the SummonList Striders
-- tracking/DespawnAll arms stay out (no creature-by-GUID
-- bridge; Go spell-duration temp summons cover the death/
-- reset cleanup partially); SPELL_LEVITATION_PULSE 31701 is an unused enum
-- member — never referenced in the file (gruul unused-enum
-- precedent); the ScriptData "Timers may be incorrect"
-- (SD%Complete: 95) is an upstream caveat, documented, not
-- bridged; no SpellScript/AuraScript scripts in this file.

local SPELL_LEVITATE = 31704
local SPELL_SUSPENSION = 31719
local SPELL_CHAIN_LIGHTNING = 31717
local SPELL_STATIC_CHARGE = 31715
local SPELL_SUMMON_SPORE_STRIDER = 38755

local ENTRY_BLACK_STALKER = 17882
local ENTRY_SPORE_STRIDER = 22299

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

-- C++ SelectTarget(Random, 0) (UnitAI.h:170 defaults
-- playerOnly=false, withTank=true, dist=0, aura=0): random alive
-- unit from the boss's threat list, tank included, no distance
-- gate (kalithresh precedent). Nil when the threat list is
-- empty — the caller skips the cast (C++-exact).
local function pickThreatTarget(creature)
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

-- C++ SelectTarget(Random, 1) (boss_the_black_stalker.cpp:98/:166):
-- UnitAI.h builds the list victim-first, so offset 1 removes the
-- victim, not the threat head. Nil when the pool is empty — the
-- levitate caller skips the cast, the summon caller falls back
-- to the victim (both C++-exact).
local function pickThreatTargetSkipFirst(creature)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end

    local targets = creature:GetAITargets()
    local found = {}
    for i, t in ipairs(targets) do
        if ((victimGuid ~= nil and t:GetGUID() ~= victimGuid)
                or (victimGuid == nil and i > 1))
                and not t:IsDead() then
            found[#found + 1] = t
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ SelectTarget(Random, 0, 30, true): random alive
-- threat-list player within 30 yd (thespia distance-gate
-- convention; IsWithinCombatRange approximated by GetDistance).
-- Nil when none is in range — the caller skips the cast
-- (C++-exact).
local function pickThreatPlayerIn30Yd(creature)
    local found = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:GetDistance(t) <= 30.0 then
            found[#found + 1] = t
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ ObjectAccessor::GetUnit(*me, guid) twin for the levitated
-- player: GUIDs are globally unique, so a GetPlayersInWorld()
-- scan matching GetGUID() is exact (anastari precedent).
local function findPlayerByGuid(guid)
    for _, pl in ipairs(GetPlayersInWorld()) do
        if pl:GetGUID() == guid then
            return pl
        end
    end
    return nil
end

-- C++ Initialize() values: Levitate_Timer=12000,
-- ChainLightning_Timer=6000, StaticCharge_Timer=10000,
-- SporeStriders_Timer=10000+rand32()%5000, check_Timer=5000,
-- InAir=false (the C++ JustEngagedWith is empty).
local function freshState()
    return {
        levitatedGuid = nil,
        levitatedTimer = 0,
        inAir = false,
        levitate = 12000,
        chainLightning = 6000,
        staticCharge = 10000,
        sporeStriders = 10000 + math.random(0, 4999),
        check = 5000,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Evade check: 5s init then 1s — timer bookkeeping only:
    -- the fire arm (60-yd home-position distance check ->
    -- EnterEvadeMode) has no EnterEvadeMode bridge; re-arm
    -- 1000 regardless (C++-exact). GetHomePosition IS bridged
    -- (lua_creature_events.go) — the evade call is the true blocker.
    if st.check <= 1000 then
        st.check = 1000
    else
        st.check = st.check - 1000
    end

    -- Spore striders 38755 (ENTRY_SPORE_STRIDER 22299):
    -- 10000+rand32()%5000 init then 10000+rand32()%5000 — the
    -- fire arm is IsHeroic()-gated (C++ :130-134, self-cast,
    -- non-triggered); the decrement arm runs on all paths
    -- (C++-exact).
    if creature:IsHeroic() and st.sporeStriders <= 1000 then
        creature:CastSpell(creature, SPELL_SUMMON_SPORE_STRIDER)
        st.sporeStriders = 10000 + math.random(0, 4999)
    else
        st.sporeStriders = st.sporeStriders - 1000
    end

    -- LevitatedTarget followup (C++ UpdateAI order: before the
    -- Levitate_Timer arm): on the 2000ms timer expiry, GetUnit
    -- the stored GUID (nil -> clear and fall through to the
    -- levitate arm, C++-exact); player found but without the
    -- LEVITATE aura -> clear and return (C++ UpdateAI return
    -- skips chain/static that tick). Not yet in air: the
    -- MAGNETIC_PULL 31705 self-cast leg stays out (no player
    -- self-cast bridge) but the InAir latch + 1500ms re-arm
    -- are kept, so the second expiry still fires SUSPENSION
    -- via player:AddAura (C++-exact timing modulo the 1s pump
    -- granularity: the 1500ms re-arm fires on the 2nd tick).
    -- In air: AddAura(SUSPENSION) + clear.
    if st.levitatedGuid then
        if st.levitatedTimer <= 1000 then
            local target = findPlayerByGuid(st.levitatedGuid)
            if not target then
                st.levitatedGuid = nil
            elseif not target:HasAura(SPELL_LEVITATE) then
                st.levitatedGuid = nil
                return
            elseif st.inAir then
                target:AddAura(SPELL_SUSPENSION)
                st.levitatedGuid = nil
            else
                st.inAir = true
                st.levitatedTimer = 1500
            end
        else
            st.levitatedTimer = st.levitatedTimer - 1000
        end
    end

    -- Levitate 31704: 12s init then 12000+rand32()%3000 —
    -- random alive threat-list unit with the threat head
    -- skipped (steamrigger convention), nil pick casts nothing,
    -- non-triggered cast on the target (C++-exact), re-arm
    -- regardless (the re-arm is outside the if (target) block,
    -- C++-exact). A cast stores the target GUID + 2000ms
    -- followup timer + InAir=false (C++ Levitate_Timer arm,
    -- overwriting any pending followup); a nil pick leaves the
    -- latch clear.
    if st.levitate <= 1000 then
        local target = pickThreatTargetSkipFirst(creature)
        if target then
            creature:CastSpell(target, SPELL_LEVITATE)
            st.levitatedGuid = target:GetGUID()
            st.levitatedTimer = 2000
            st.inAir = false
        end
        st.levitate = 12000 + math.random(0, 2999)
    else
        st.levitate = st.levitate - 1000
    end

    -- Chain lightning 31717: 6s init then 7000 — random alive
    -- threat-list unit (SelectTarget(Random, 0) defaults),
    -- nil pick casts nothing, re-arm 7000 regardless
    -- (C++-exact).
    if st.chainLightning <= 1000 then
        local target = pickThreatTarget(creature)
        if target then
            creature:CastSpell(target, SPELL_CHAIN_LIGHTNING)
        end
        st.chainLightning = 7000
    else
        st.chainLightning = st.chainLightning - 1000
    end

    -- Static charge 31715: 10s init then 10000 — random alive
    -- threat-list player within 30 yd (thespia distance-gate
    -- convention), nil pick casts nothing, re-arm 10000
    -- regardless (C++-exact).
    if st.staticCharge <= 1000 then
        local target = pickThreatPlayerIn30Yd(creature)
        if target then
            creature:CastSpell(target, SPELL_STATIC_CHARGE)
        end
        st.staticCharge = 10000
    else
        st.staticCharge = st.staticCharge - 1000
    end
end

RegisterCreatureEvent(ENTRY_BLACK_STALKER, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_BLACK_STALKER, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_BLACK_STALKER, 4, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_BLACK_STALKER, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)

-- C++ JustSummoned (boss_the_black_stalker.cpp:109-119):
-- entry-gated spore strider AttackStart relay — random alive
-- threat-list unit with the threat head skipped (steamrigger
-- convention), victim fallback when the pick is nil
-- (C++-exact); EngageWithTarget
-- is the AttackStart funnel (skyriss ev19 precedent). The Go
-- spell-summon funnel fires ev19 on the caster
-- (boss_ai.go:2083-2085, lua_creature_events.go:166), so the
-- relay fires exactly when a strider materializes; the
-- SummonList tracking/DespawnAll arms stay out (below).
RegisterCreatureEvent(ENTRY_BLACK_STALKER, 19, function(_, creature, summoned)
    if not summoned or summoned:GetEntry() ~= ENTRY_SPORE_STRIDER then
        return
    end
    local target = pickThreatTargetSkipFirst(creature)
    if not target then
        target = creature:GetVictim()
    end
    if target then
        summoned:EngageWithTarget(target)
    end
end)
