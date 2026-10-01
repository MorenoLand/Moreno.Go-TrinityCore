-- The Lurker Below (Serpentshrine Cavern) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/SerpentShrine/
-- boss_lurker_below.cpp (boss_the_lurker_below,
-- npc_coilfang_ambusher — the two creature AI classes
-- AddSC_boss_the_lurker_below registers; go_strange_pool is a
-- GameObjectScript — no GO bridge, documented only, not
-- registered; NPC_COILFANG_GUARDIAN = 21865 ... see below).
-- Entry 21217 verified from the C++ sources
-- (instance_serpent_shrine.cpp: case 21217 -> LurkerBelow,
-- DATA_THELURKERBELOW); 21865 comes from the file's own
-- Creatures enum (NPC_COILFANG_AMBUSHER). NPC_COILFANG_
-- GUARDIAN (21873) has no AI class in this file — documented
-- only, not registered. The creature_template ScriptName
-- bindings are DB-side (no TDB in this workspace).
-- Talk lines used: EMOTE_SPOUT=0 ("The Lurker Below takes a
-- deep breath." — spout start). No SAY_ lines exist in this
-- file. SPELL_KNOCKBACK=19813 is defined in the C++ enum but
-- never called anywhere in the file — documented only (gruul
-- precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset (neither AI class overrides KilledUnit —
-- no event 3). Timers via CreateLuaEvent (per-GUID scheduler
-- pump); melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady. C++ DoCast default is
-- triggered=false (vaelastrasz convention); DoCastVictim takes
-- nil as the victim arm (felmyst convention).
-- Fight shape (C++-exact for the modeled arms):
-- boss_the_lurker_below (21217): OnEnterCombat(1): per-GUID
-- scheduler reset to the C++ Initialize() values (spoutAnim
-- 1000 / rot 0 / waterbolt 15000 / spout 45000 / whirl 18000 /
-- phase 120000 / geyser {15s,20s} / check 15000 / wait 60000 /
-- wait2 60000 / spawned false / inRange false) — submerged
-- starts FALSE at combat: the whole fishing pre-phase (the
-- CheckCanStart/DATA_STRANGE_POOL poll, the emerge anim, the
-- WaitTimer/WaitTimer2 chain, SetVisible, the NOT_SELECTABLE
-- flag and ImmuneToPC arms) has no instance/GO/display/flag
-- bridges — the modeled fight starts on pull per the
-- magtheridon/ragnaros-intro convention. + 1s scheduler pump
-- (a port of UpdateAI — 1s granularity is exact for the 45s/
-- 18s/15s/2s/60s/120s timers). Pump, !Submerged arm: phase 120s
-- -> DoCast self 37550 non-triggered (C++ DoCast default),
-- phase=60000, submerged=true (the InterruptNonMeleeSpells arm
-- has no interrupt bridge — krosh convention). Spout 45s ->
-- Talk(EMOTE_SPOUT), re-arm 45000, whirl=20000 (whirl directly
-- after spout, C++-exact), rot=20000, return (C++-exact — the
-- MoveRotate 20s + react-passive arms have no
-- movement/react bridges; the 20s rotating spout machine —
-- the per-tick HasInArc + SPOUT_DIST(100 yd) + IsInWater
-- player pick and the triggered 37433 per-player casts, the
-- SPOUT_ANIM 42835 upkeep, and the MovementInform react-
-- aggressive flip — has no arc/movement/react/water bridges,
-- so the RotTimer arm only does C++-exact timer bookkeeping
-- and the spout delivers no damage in this model, header).
-- Whirl 18s (20s right after a spout) -> DoCast self 37660,
-- re-arm 18000 (C++-exact). CheckTimer 15s then 2s -> inRange =
-- any alive player in the instance within 5 yd (the C++ threat-
-- list IsWithinMeleeRange scan — 5-yd player stand-in, gruul
-- convention; no threat bridge), re-arm 2000 (C++-exact).
-- Geyser: timer fires -> target = random alive player
-- excluding the victim (C++ SelectTarget(Random, 1),
-- curator/teron convention; nil -> victim fallback, C++-exact
-- "if (!target && me->GetVictim())"), if a target exists ->
-- DoCast(target, 37478, triggered=true) (C++-exact); re-arm
-- {15s,20s} regardless (C++-exact). Waterbolt: when !inRange
-- and timer fires -> target = random alive player in the
-- instance (C++ SelectTarget(Random, 0), teron convention; nil
-- -> victim fallback), if a target exists -> DoCast(target,
-- 37138, triggered=true) (C++-exact); re-arm 3000 (15s init,
-- C++-exact). Submerged arm: phase 60s -> submerged=false,
-- RemoveAllAuras, DoCast self SPELL_EMERGE 20568 triggered
-- (C++-exact true), spawned=false (the 9x summon arm — 6x
-- NPC_COILFANG_AMBUSHER + 3x NPC_COILFANG_GUARDIAN at the
-- AddPos coords — has no summon bridge, unmodeled; the
-- SetUInt32Value IMMUNE_TO_PC flag arm has no flag bridge,
-- unmodeled), spout=3000 (C++ "directly cast Spout after
-- emerging!"), phase=120000, return (C++-exact). The
-- IsThreatened/EnterEvadeMode/DoZoneInCombat arms have no
-- evade bridge — timers fire unconditionally (jeklik
-- convention). The MoveInLineOfSight aggro machine has no LOS
-- hook (hydross convention) — the fight starts on pull.
-- OnDied(4): cleanup (the SetData DONE/IN_PROGRESS and
-- Summons.DespawnAll arms are instance/summon blocked).
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop per-GUID
-- state (the Reset arms — SetSwim/SetDisableGravity, the
-- SPELL_SUBMERGE self-cast, SetVisible(false), the NOT_
-- SELECTABLE flag + ImmuneToPC arms, Summons.DespawnAll and
-- the SetData NOT_STARTED arms — are movement/display/flag/
-- instance/summon blocked).
-- npc_coilfang_ambusher (21865): OnEnterCombat(1): per-GUID
-- reset (multishot 10000 / shootbow 4000, C++ Initialize()) +
-- 1s pump (a port of UpdateAI — 1s granularity exact; the C++
-- has no UpdateVictim call in this UpdateAI; SetCombatMovement
-- (false) has no movement bridge — engine melee applies).
-- Pump: multishot fires -> if the victim exists -> DoCastVictim
-- 37790 triggered (C++-exact), re-arm 10000 + rand32()%10000,
-- shootbow += 1500 (the GCD arm, C++-exact). Shootbow fires ->
-- target = random alive player in the instance (C++
-- SelectTarget(Random, 0)) -> CastSpell(target, 37770,
-- triggered) (the C++ AddSpellBP0(1100) arg has no bridge —
-- quake/blaze SPELLVALUE precedent, documented deviation);
-- re-arm 4000 + rand32()%5000; multishot += 1500 (C++-exact).
-- The MoveInLineOfSight 45-yd aggro machine has no LOS hook —
-- unmodeled (hydross precedent). OnDied(4)/OnLeaveCombat(2)/
-- OnReset(23): cancel the pump, drop per-GUID state.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (DATA_THELURKERBELOWEVENT 17 NOT_STARTED/IN_PROGRESS/DONE
-- and DATA_STRANGE_POOL 23 NOT_STARTED/IN_PROGRESS bookkeeping
-- skipped); no GO bridge — go_strange_pool's OnGossipHello
-- (the 25% fishing roll, the 54587 cast, the DATA_STRANGE_POOL
-- transitions) unmodeled; no summon bridge — the 9x submerged
-- ambusher/guardian summons unmodeled; no movement bridge —
-- the spout MoveRotate 20s machine, the rot-arc spout picks
-- and the SPOUT_ANIM upkeep unmodeled; no LOS/arc/water/react/
-- flag/display/interrupt/threat/evade/yell bridges — the
-- arc+distance+water spout target pick, the react passive/
-- aggressive flips, the visibility/flag/immune reset arms, the
-- InterruptNonMeleeSpells arm and the evade/zone-combat arms
-- unmodeled; the shoot-bow 1100 BP0 arg has no bridge (cast
-- without it); SPELL_KNOCKBACK 19813 defined but unused in
-- C++ — documented only; NPC_COILFANG_GUARDIAN (21873) has no
-- AI class in this file — documented only, not registered; no
-- SpellScript/AuraScript scripts in this file.

local SPELL_SPOUT = 37433
local SPELL_SPOUT_ANIM = 42835
local SPELL_GEYSER = 37478
local SPELL_WHIRL = 37660
local SPELL_WATERBOLT = 37138
local SPELL_SUBMERGE = 37550
local SPELL_EMERGE = 20568

local SPELL_SPREAD_SHOT = 37790
local SPELL_SHOOT = 37770

local EMOTE_SPOUT = 0

local ENTRY_LURKER = 21217
local ENTRY_AMBUSHER = 21865

local MELEE_RANGE_STANDIN = 5

local lurkerTimers = {}
local lurkerState = {}
local ambusherTimers = {}
local ambusherState = {}

local function cancelPump(timers, guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetLurker(guid)
    cancelPump(lurkerTimers, guid)
    lurkerState[guid] = nil
end

local function resetAmbusher(guid)
    cancelPump(ambusherTimers, guid)
    ambusherState[guid] = nil
end

-- Alive players sharing the creature's map+instance (teron
-- convention).
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
-- instance.
local function pickRandomAlivePlayer(creature)
    local found = playersInInstance(creature)
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ SelectTarget(Random, 1): random alive player other than
-- the victim. Returns nil when no such pick exists.
local function pickRandomNonVictim(creature)
    local victim = creature:GetVictim()
    local found = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if p ~= victim then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ Initialize(): spoutAnim 1000, rot 0, waterbolt 15000,
-- spout 45000, whirl 18000, phase 120000, geyser {15s,20s},
-- check 15000, wait 60000, wait2 60000, spawned false,
-- inRange false. Submerged starts false at combat (header —
-- the fishing pre-phase is instance/GO blocked).
local function freshLurkerState()
    return {
        spoutAnim = 1000,
        rot = 0,
        waterbolt = 15000,
        spout = 45000,
        whirl = 18000,
        phase = 120000,
        geyser = 15000 + math.random(0, 4999),
        check = 15000,
        spawned = false,
        inRange = false,
        submerged = false,
    }
end

local function lurkerTick(creature, guid)
    local st = lurkerState[guid]
    if not st then
        return
    end

    if not st.submerged then
        -- Phase 120s -> submerge: self-cast 37550
        -- non-triggered (C++ DoCast default), phase=60s,
        -- submerged (the InterruptNonMeleeSpells arm has no
        -- interrupt bridge — krosh convention).
        if st.phase <= 1000 then
            creature:CastSpell(nil, SPELL_SUBMERGE)
            st.phase = 60000
            st.submerged = true
        else
            st.phase = st.phase - 1000
        end

        -- Spout 45s: talk + re-arm; whirl re-arms to 20s and
        -- the rotation lasts 20s (C++-exact order, return —
        -- header).
        if st.spout <= 1000 then
            creature:Talk(EMOTE_SPOUT)
            st.spout = 45000
            st.whirl = 20000
            st.rot = 20000
            return
        else
            st.spout = st.spout - 1000
        end

        -- Whirl 18s (20s right after a spout): self-cast
        -- 37660, re-arm 18s.
        if st.whirl <= 1000 then
            creature:CastSpell(nil, SPELL_WHIRL)
            st.whirl = 18000
        else
            st.whirl = st.whirl - 1000
        end

        -- CheckTimer 15s then 2s: melee-range scan via the
        -- 5-yd player stand-in (header).
        if st.check <= 1000 then
            st.inRange = false
            for _, p in ipairs(playersInInstance(creature)) do
                if creature:GetDistance(p) <= MELEE_RANGE_STANDIN then
                    st.inRange = true
                    break
                end
            end
            st.check = 2000
        else
            st.check = st.check - 1000
        end

        -- RotTimer: the 20s rotating spout machine has no
        -- arc/movement/water bridges — timer bookkeeping only
        -- (header); the spout arm returns above, so during the
        -- rotation only these timers count down.
        if st.rot > 0 then
            if st.rot <= 1000 then
                st.rot = 0
            else
                st.rot = st.rot - 1000
            end
            return
        end

        -- Geyser {15s,20s}: random non-victim player pick with
        -- victim fallback (C++-exact), triggered cast 37478;
        -- the timer re-arms regardless (C++-exact).
        if st.geyser <= 1000 then
            local target = pickRandomNonVictim(creature)
            if not target then
                target = creature:GetVictim()
            end
            if target then
                creature:CastSpell(target, SPELL_GEYSER, true)
            end
            st.geyser = 15000 + math.random(0, 4999)
        else
            st.geyser = st.geyser - 1000
        end

        -- Waterbolt when no player is in melee range: random
        -- alive player pick with victim fallback (C++-exact),
        -- triggered cast 37138; 15s init, 3s re-arm.
        if not st.inRange then
            if st.waterbolt <= 1000 then
                local target = pickRandomAlivePlayer(creature)
                if not target then
                    target = creature:GetVictim()
                end
                if target then
                    creature:CastSpell(target, SPELL_WATERBOLT, true)
                end
                st.waterbolt = 3000
            else
                st.waterbolt = st.waterbolt - 1000
            end
        end
    else
        -- Submerged arm: phase 60s -> emerge: remove auras,
        -- triggered self-cast 20568 (C++-exact), spout=3s
        -- (C++ "directly cast Spout after emerging!"), phase
        -- back to 120s (the 9x summon arm and the flag arm are
        -- summon/flag blocked — header). The evade/zone-combat
        -- arms have no evade bridge — timers fire
        -- unconditionally (jeklik convention).
        if st.phase <= 1000 then
            st.submerged = false
            creature:RemoveAllAuras()
            creature:CastSpell(nil, SPELL_EMERGE, true)
            st.spawned = false
            st.spout = 3000
            st.phase = 120000
            return
        else
            st.phase = st.phase - 1000
        end
    end
end

local function lurkerEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetLurker(guid)
    lurkerState[guid] = freshLurkerState()
    lurkerTimers[guid] = CreateLuaEvent(function()
        lurkerTick(creature, guid)
    end, 1000)
end

local function lurkerLeaveCombat(event, creature)
    resetLurker(creature:GetGUID())
end

local function lurkerDied(event, creature, killer)
    resetLurker(creature:GetGUID())
end

local function lurkerReset(event, creature)
    resetLurker(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_LURKER, 1, lurkerEnterCombat)
RegisterCreatureEvent(ENTRY_LURKER, 2, lurkerLeaveCombat)
RegisterCreatureEvent(ENTRY_LURKER, 4, lurkerDied)
RegisterCreatureEvent(ENTRY_LURKER, 23, lurkerReset)

-- npc_coilfang_ambusher (21865) — see header.

-- C++ Initialize(): multishot 10000, shootbow 4000.
local function freshAmbusherState()
    return {
        multishot = 10000,
        shootbow = 4000,
    }
end

local function ambusherTick(creature, guid)
    local st = ambusherState[guid]
    if not st then
        return
    end

    -- MultiShot: DoCastVictim 37790 triggered, re-arm
    -- 10000+rand()%10000; the shootbow timer gains the 1500ms
    -- GCD arm (C++-exact).
    if st.multishot <= 1000 then
        if creature:GetVictim() then
            creature:CastSpell(creature:GetVictim(), SPELL_SPREAD_SHOT, true)
        end
        st.multishot = 10000 + math.random(0, 9999)
        st.shootbow = st.shootbow + 1500
    else
        st.multishot = st.multishot - 1000
    end

    -- Shoot bow: random alive player pick -> triggered cast
    -- 37770 (the C++ AddSpellBP0(1100) arg has no bridge —
    -- quake/blaze precedent, header); re-arm 4000+rand()%5000;
    -- the multishot timer gains the 1500ms GCD arm (C++-exact).
    if st.shootbow <= 1000 then
        local target = pickRandomAlivePlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_SHOOT, true)
        end
        st.shootbow = 4000 + math.random(0, 4999)
        st.multishot = st.multishot + 1500
    else
        st.shootbow = st.shootbow - 1000
    end
end

local function ambusherEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetAmbusher(guid)
    ambusherState[guid] = freshAmbusherState()
    ambusherTimers[guid] = CreateLuaEvent(function()
        ambusherTick(creature, guid)
    end, 1000)
end

local function ambusherCleanup(event, creature)
    resetAmbusher(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_AMBUSHER, 1, ambusherEnterCombat)
RegisterCreatureEvent(ENTRY_AMBUSHER, 2, ambusherCleanup)
RegisterCreatureEvent(ENTRY_AMBUSHER, 4, ambusherCleanup)
RegisterCreatureEvent(ENTRY_AMBUSHER, 23, ambusherCleanup)
