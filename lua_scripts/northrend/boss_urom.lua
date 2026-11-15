-- Mage-Lord Urom (The Oculus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Oculus/boss_urom.cpp
-- (boss_urom (CreatureScript) via GetOculusAI<boss_uromAI>
-- (BossAI, DATA_UROM); spell_urom_frostbomb (AuraScript, registered
-- from inside AddSC_boss_urom via RegisterSpellScript, no separate
-- loader lines); AddSC_boss_urom() at end registers both; loader
-- decl 85 / call 280 per northrend_script_loader.cpp — the SECOND
-- group of the "// The Nexus: The Oculus" block in
-- AddNorthrendScripts(), immediately after AddSC_boss_drakos() (decl
-- 84 / call 279); the call after it is AddSC_boss_varos() — loader
-- order confirmed this run; the checkpoint sequence (drakos ->
-- urom) is followed).
-- Entry: 27655 Mage-Lord Urom (oculus.h NPC_UROM, line 43;
-- instance_oculus.cpp OnCreatureCreate binds case NPC_UROM (line
-- 67) — entry-verifiable, registration proceeds (the
-- nexus_commanders kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual. The instance leg also
-- gates SetPhaseMask(1, true) on GetBossState(DATA_VAROS) == DONE —
-- PORTED 2026-11-05 in lua_scripts/northrend/instance_oculus.lua
-- (ON_CREATURE_CREATE DONE-gated leg + ON_UPDATE DONE poll).
-- Sole-source verified: whole-server-tree grep for "boss_urom"
-- hits boss_urom.cpp + the loader decl/call lines only; grep for
-- "spell_urom_frostbomb" hits boss_urom.cpp only (registered from
-- inside AddSC_boss_urom, no separate loader lines); zero sql/
-- hits. No urom lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 9 OnDamageTaken, 14 OnHitBySpell,
-- 23 OnReset, 24 OnReachHome. Driven by CreateLuaEvent timers
-- (thekal delayTimers idiom); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- KilledUnit — Talk(SAY_PLAYER_KILL 7) (event 3 — C++-GATED
-- who->GetTypeId() == TYPEID_PLAYER; the nalorakk / kelthuzad
-- player-gate variant — ninth ported variant overall, sixth
-- identical to nalorakk / kelthuzad / gothik / thaddius /
-- keristrasza).
-- JustDied — Talk(SAY_DEATH 6) (event 4; the _JustDied
-- SetBossState(DONE) leg is PORTED in uromDied (base-first,
-- 2026-11-05) + the DoCastSelf(SPELL_DEATH_SPELL 50415, true) leg
-- PORTED 2026-11-13 (round-45 re-triage: the "no effect-28" ground
-- was false — creatureEffectSummonType dispatches from the
-- creature-cast path, boss_ai.go:3574-3581, gated on spellKnown;
-- 50415 seeded from wago.tools 3.4.0.43659: single effect 28 SUMMON,
-- MiscValue 28012 Amber Drake, MiscValueB 64; the summon needs a
-- creature_template 28012 row in the deployment world DB).
-- JustEngagedWith / StartAttack — the platform machine, PORTED
-- 2026-11-15 in uromEnterCombat/startAttack: Lua-local per-GUID
-- _platform (0..3) + shuffled _group (constructor shuffle twin,
-- Fisher-Yates over {1,2,3}) + _isInCenter. On engages 0..2:
-- SummonCreature x4 (Group[_group[_platform]] phantasmal entries
-- at SetPosition offsets ±4 x/y, z/o = boss's; SummonCreature
-- bridged, lua_creature_events.go:4482, default type 7
-- MANUAL_DESPAWN = the C++ SummonCreature default) + Talk(
-- SAY_SUMMON_1..3 0..2 = _platform) + DoCast(TeleportSpells[
-- _platform]) as self-cast (the spell's DBC target is the caster —
-- C++ SpellHit fires on Urom himself) + _platform++. On engage 3
-- (_platform > 2): CastStop twin creature:InterruptSpell(1)
-- (flamecaller precedent — no direct CastStop bridge) + Talk(
-- SAY_AGGRO 3) + center timers (teleport 30-35s, frostbomb 5-8s,
-- timebomb 20-25s, randtime ranges resolved with math.random at
-- schedule time so the thekal due-timestamp math stays numeric).
-- The BossAI::JustEngagedWith base arms (SetBossState(DATA_UROM,
-- IN_PROGRESS) + pulse + me-form DoZone) are PORTED in
-- uromEnterCombat (base call first, boss_urom.cpp:147-150;
-- SetBossState PORTED 2026-11-05, creature:SetBossState bridged,
-- lua_creature_events.go:7099).
-- ev1 RE-FIRE verified 2026-11-15: triggerCreatureAggro sets
-- enteredCombat on !motion.InCombat (creaturemotion.go:1644) and
-- fires OnEnterCombat (:1707); CombatStop clears InCombat
-- (lua_creature_events.go:3569) and fires no Lua hooks, so the
-- ev14 LeaveCombat teardown re-arms ev1 for the next engage —
-- the platform cycle re-fires exactly like C++ JustEngagedWith.
-- The summoned adds get NO explicit AttackStart: C++ has no such
-- leg (their own AI / proximity aggro engages them) — C++-exact.
-- Reset — SetControlled(false, UNIT_STATE_ROOT) ports via
-- creature:SetRooted(false) (uromReset); SetDisableGravity(false) is
-- bridged AND ported in uromReset (the old "no bridge" line was
-- stale) and SetReactState(REACT_AGGRESSIVE) is vacuous here (Lua
-- never sets REACT_PASSIVE); the ungated DoCastSelf(
-- SPELL_EVOCATE 51602) leg is ported on OnReset(23) (the Reset bullet's
-- earlier "no cast bridge" line was stale — creature:CastSpell is
-- established in-tree); _Reset() SetBossState(NOT_STARTED,
-- != DONE-gated) is PORTED in uromReset (2026-11-05; _Reset() is
-- LAST in C++ Reset, so the state leg follows the Evocate cast).
-- uromReset also clears _isInCenter (C++ Reset :133) and cancels
-- the center timers (EnterEvadeMode events.Reset() twin) — but
-- _platform/_group are NOT reset (C++ Reset never touches them;
-- only death/respawn reconstructs the AI, mirrored by the
-- encounters[guid] clear in uromDied).
-- JustReachedHome — DoCastSelf(SPELL_EVOCATE 51602) — ported below
-- on event 24 (this tick; the old "no cast bridge" note was stale —
-- the Reset leg's port above already uses the established CastSpell
-- bridge).
-- UpdateAI — the center teleport event machine, PORTED 2026-11-15
-- as CreateLuaEvent timers (only armed on the 4th engage, so the
-- C++ _platform < 3 early return needs no Lua gate):
-- EVENT_TELEPORT — DelayEvents(10s) via thekal delayTimers
-- (before the explosion is scheduled, so it is not delayed) +
-- REACT_PASSIVE + AttackStop + StopMoving + SetDisableGravity(true)
-- + SetCanFly(true) + SetRooted(true) (SetControlled ROOT twin) +
-- DoCast(SPELL_TELEPORT 51112) self-cast + _isInCenter = true +
-- EVENT_CAST_EXPLOSION 2s; EVENT_CAST_EXPLOSION — Talk(EMOTE_
-- ARCANE_EXPLOSION 4) + Talk(SAY_ARCANE_EXPLOSION 5) + DoCastAOE(
-- SPELL_EMPOWERED_ARCANE_EXPLOSION 51110) as self-cast
-- (DoCastAOE ≡ self-cast convention, boss_ai.go DoCastAOE
-- self-cast lowering) + EVENT_TELEPORT_BACK DUNGEON_MODE(10s/8s)
-- via creature:IsHeroic(); EVENT_TELEPORT_BACK — REACT_AGGRESSIVE
-- + gravity/fly/root off + NearTeleportTo(victim position) +
-- AttackStart(victim) + _isInCenter = false + EVENT_TELEPORT
-- 30-35s; EVENT_FROST_BOMB — DoCastVictim(SPELL_FROSTBOMB 51103)
-- 5-8s repeat; EVENT_TIME_BOMB — SelectTarget(Random) from the
-- threat list (maiden randomTargetInRange idiom, no range filter —
-- C++ passes no range) -> DoCast(SPELL_TIME_BOMB 51121) 20-25s
-- repeat. Deviation: no UNIT_STATE_CASTING gate (maiden deviation
-- — creature casts are packet-visual in Go, timers fire
-- unconditionally). Timers cancel on events 2/4/23.
-- DamageTaken — PORTED 2026-11-15 on event 9: lethal gate
-- (damage >= creature:GetHealth(), GetHealth via the object.go
-- generic fallback) + _isInCenter -> _isInCenter = false +
-- NearTeleportTo(1124.0432, 1078.2109, 508.3597, 5.4623);
-- ev9 fires on killing blows (combat.go:1499/2326) and DamageTaken
-- arms must NOT take the nefarian postDamage > 0 guard (that guard
-- is for UpdateAI HealthBelowPct arms only) — returns (true,
-- damage), the sindragosa idiom, damage unmodified.
-- SpellHit — PORTED 2026-11-15 on event 14: SPELL_SUMMON_
-- MENAGERIE(3) x3 -> SetHomePosition (three platform coords) +
-- LeaveCombat (RemoveAllAuras + CombatStop(false) + EngagementOver,
-- whose Go analog is the CombatStop InCombat=false clear) +
-- DoCastSelf(SPELL_EVOCATE 51602, true). ev14 SELF-CAST verified
-- 2026-11-15: castCreatureSpellInternal seeds hitTargets with the
-- explicit targetGUID (boss_ai.go:1801) and the ev14 loop fires per
-- hit target when spellKnown (:4248-4292), so a self-cast menagerie
-- spell fires (creature=Urom, caster=Urom, spellId) — the old
-- "trigger alone" stay-out is retired. The spellKnown gate is why
-- the menagerie spells must be seeded (see SEED NOTE below).
-- EnterEvadeMode — gated on _platform > 2 (center only): _EnterEvade-
-- Mode + NearTeleportTo(1118.3101, 1080.3800, 508.3610, 4.25) or
-- MoveTargetedHome + Reset + events.Reset — creature:EnterEvadeMode
-- IS bridged (lua_creature_events.go:5624) and the MoveTargetedHome
-- twin creature:MoveHome IS bridged (:5599); the stay-out holds on
-- different grounds: no ev25 hook exists (runtime.go CreatureEvents:
-- 25 unused, no Go constant — engine-initiated evade has no Lua
-- hook), and the _platform > 2 / _isInCenter gates ride the Lua-local
-- platform machine (boss_urom.cpp:114-130, :189-219). The
-- events.Reset() leg IS mirrored by the ev2/ev23 timer cancels.
-- AttackStart — z-position-gated DoStartNoMovement vs BossAI::
-- AttackStart — creature:AttackStart IS bridged (lua_creature_events.go:7325,
-- EngageWithTarget alias); DoStartNoMovement itself has no bridge, and
-- that, not movement, is the blocker here.
-- spell_urom_frostbomb — AuraScript OnPeriodic (EFFECT_1 SPELL_AURA_
-- PERIODIC_DAMAGE): heroic + IsInCombat gate -> caster CastSpell(
-- target, SPELL_FROST_BUFFET 58025) — PORTED below via
-- RegisterAuraEvent (tick 194); the "no AuraScript bridge" block is
-- stale (tick 188).
-- SEED NOTE (2026-11-15): the machine casts 9 spells that are NOT
-- yet in tools/spelldbc_seed/main.go — 50476/50495/50496 (menagerie),
-- 51112 (teleport), 51110 (empowered arcane explosion), 51121 (time
-- bomb), 51602 (evocate), 51103 (frostbomb), 58025 (frost buffet).
-- castCreatureSpellInternal gates EVERYTHING on spellKnown
-- (boss_ai.go:1825-1831) and the ev14 loop on spellKnown too, so
-- until they are seeded the casts fizzle silently and the cycle
-- never advances past the first engage (dormant, like 49519 before
-- its seed). Wago verdicts pulled this tick; seed + DBC regen land
-- with this port in the same push.

local ENTRY_UROM = 27655

local SAY_SUMMON_1 = 0
local SAY_SUMMON_2 = 1
local SAY_SUMMON_3 = 2
local SAY_AGGRO = 3
local EMOTE_ARCANE_EXPLOSION = 4
local SAY_ARCANE_EXPLOSION = 5
local SAY_PLAYER_KILL = 7
local SAY_DEATH = 6

local SPELL_EVOCATE = 51602
local SPELL_DEATH_SPELL = 50415 -- oculus.h:83
local SPELL_FROSTBOMB = 51103
local SPELL_FROST_BUFFET = 58025
local SPELL_TELEPORT = 51112
local SPELL_EMPOWERED_ARCANE_EXPLOSION = 51110
local SPELL_TIME_BOMB = 51121

local TELEPORT_SPELLS = {50476, 50495, 50496}

-- boss_urom.cpp:63-69 Group[] (C++ 0-based; Lua 1-based).
local MENAGERIE_GROUPS = {
    {27645, 27645, 27642, 27644},
    {27650, 27650, 27653, 27651},
    {27647, 27647, 27648, 27649},
}

-- boss_urom.cpp:296-312 SpellHit SetHomePosition coords, keyed by
-- menagerie spell id (ev14 dispatch table).
local MENAGERIE_HOMES = {
    [50476] = {968.66, 1042.53, 527.32, 0.077},
    [50495] = {1164.02, 1170.85, 527.321, 3.66},
    [50496] = {1118.31, 1080.377, 508.361, 4.25},
}

local DATA_UROM = 2 -- oculus.h:33 (OCDataTypes)
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)

local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2

-- Per-GUID encounter state (boss_uromAI members, boss_urom.cpp:328-331):
-- _platform (constructor 0, StartAttack only), _group (constructor
-- shuffle), _isInCenter (Reset/teleport-back/ev9 clear).
local encounters = {}

local function shuffledGroups()
    local g = {1, 2, 3}
    for i = 3, 2, -1 do
        local j = math.random(i)
        g[i], g[j] = g[j], g[i]
    end
    return g
end

-- thekal delayTimers idiom (boss_thekal.lua): due-timestamp entries
-- so DelayEvents preserves each timer's remaining time; a fired
-- entry removes itself, so a delay never resurrects a spent timer.
local timers = {}

local function nowMs()
    return GetCurrTime()
end

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, t in pairs(per) do
            RemoveEventById(t.id)
        end
        timers[guid] = nil
    end
end

local function armEntry(guid, key, delayMs, fn)
    local entry = {due = nowMs() + delayMs, fn = fn}
    entry.id = CreateLuaEvent(function()
        local per = timers[guid]
        if per and per[key] == entry then
            per[key] = nil
        end
        fn()
    end, delayMs)
    return entry
end

local function schedule(guid, key, delayMs, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key].id)
    end
    per[key] = armEntry(guid, key, delayMs, fn)
end

-- C++ EventMap::DelayEvents: push every armed timer out by extraMs,
-- preserving each timer's remaining time.
local function delayTimers(guid, extraMs)
    local per = timers[guid]
    if not per then
        return
    end
    local now = nowMs()
    for key, t in pairs(per) do
        RemoveEventById(t.id)
        local remaining = t.due - now
        if remaining < 0 then
            remaining = 0
        end
        per[key] = armEntry(guid, key, remaining + extraMs, t.fn)
    end
end

-- Cyclic timer chain (teleport -> explosion -> teleport-back ->
-- teleport): forward-declare like the tharon_ja / gal_darah idiom —
-- a closure referencing a later `local function` would bind nil.
local onTeleport, onExplosion, onTeleportBack, onFrostBomb, onTimeBomb

-- C++ UpdateAI EVENT_TELEPORT (boss_urom.cpp:243-258).
onTeleport = function(creature, guid)
    local st = encounters[guid]
    if not st then
        return
    end
    delayTimers(guid, 10000)
    creature:SetReactState(REACT_PASSIVE)
    creature:AttackStop()
    creature:StopMoving()
    creature:SetDisableGravity(true)
    creature:SetCanFly(true)
    creature:SetRooted(true)
    creature:CastSpell(creature, SPELL_TELEPORT)
    st.isInCenter = true
    schedule(guid, "explosion", 2000, function() onExplosion(creature, guid) end)
end

-- C++ UpdateAI EVENT_CAST_EXPLOSION (boss_urom.cpp:259-265).
onExplosion = function(creature, guid)
    creature:Talk(EMOTE_ARCANE_EXPLOSION)
    creature:Talk(SAY_ARCANE_EXPLOSION)
    creature:CastSpell(creature, SPELL_EMPOWERED_ARCANE_EXPLOSION)
    local backDelay = 10000
    if creature:IsHeroic() then
        backDelay = 8000
    end
    schedule(guid, "teleport_back", backDelay, function() onTeleportBack(creature, guid) end)
end

-- C++ UpdateAI EVENT_TELEPORT_BACK (boss_urom.cpp:266-276).
onTeleportBack = function(creature, guid)
    local st = encounters[guid]
    if not st then
        return
    end
    creature:SetReactState(REACT_AGGRESSIVE)
    creature:SetDisableGravity(false)
    creature:SetCanFly(false)
    creature:SetRooted(false)
    local victim = creature:SelectVictim()
    if victim then
        creature:NearTeleport(victim:GetX(), victim:GetY(), victim:GetZ(), victim:GetO())
        creature:AttackStart(victim)
    end
    st.isInCenter = false
    schedule(guid, "teleport", math.random(30000, 35000), function() onTeleport(creature, guid) end)
end

-- C++ UpdateAI EVENT_FROST_BOMB (boss_urom.cpp:277-280).
onFrostBomb = function(creature, guid)
    local victim = creature:SelectVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FROSTBOMB)
    end
    schedule(guid, "frostbomb", math.random(5000, 8000), function() onFrostBomb(creature, guid) end)
end

-- C++ UpdateAI EVENT_TIME_BOMB (boss_urom.cpp:281-286): SelectTarget
-- (Random) with no range — every alive player on the threat list.
local function randomTarget(creature)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

onTimeBomb = function(creature, guid)
    local target = randomTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_TIME_BOMB)
    end
    schedule(guid, "timebomb", math.random(20000, 25000), function() onTimeBomb(creature, guid) end)
end

-- C++ boss_uromAI::StartAttack (boss_urom.cpp:189-219), called from
-- JustEngagedWith on every fresh engage (ev1 re-fire verified, see
-- header). Platform leg (_platform 0..2): summon x4 + Talk + menagerie
-- self-cast + _platform++. Center leg (_platform > 2): CastStop twin
-- + Talk(SAY_AGGRO) + the three center timers.
local function startAttack(creature, guid)
    local st = encounters[guid]
    if not st then
        st = {platform = 0, group = shuffledGroups(), isInCenter = false}
        encounters[guid] = st
    end
    if st.platform > 2 then
        creature:InterruptSpell(1)
        creature:Talk(SAY_AGGRO)
        schedule(guid, "teleport", math.random(30000, 35000), function() onTeleport(creature, guid) end)
        schedule(guid, "frostbomb", math.random(5000, 8000), function() onFrostBomb(creature, guid) end)
        schedule(guid, "timebomb", math.random(20000, 25000), function() onTimeBomb(creature, guid) end)
        return
    end
    -- C++ SetPosition(i) (boss_urom.cpp:167-187): case 0 (+4,-4),
    -- case 1 (+4,+4), case 2 (-4,+4), case 3 (-4,-4).
    local group = MENAGERIE_GROUPS[st.group[st.platform + 1]]
    local cx, cy, cz, co = creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO()
    local offsets = {{4, -4}, {4, 4}, {-4, 4}, {-4, -4}}
    for i = 1, 4 do
        creature:SummonCreature(group[i], cx + offsets[i][1], cy + offsets[i][2], cz, co)
    end
    creature:Talk(st.platform)
    creature:CastSpell(creature, TELEPORT_SPELLS[st.platform + 1])
    st.platform = st.platform + 1
end

-- C++ KilledUnit: Talk(SAY_PLAYER_KILL) only when who->GetTypeId()
-- == TYPEID_PLAYER (nalorakk / kelthuzad player-gate variant).
local function uromTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_PLAYER_KILL)
    end
end

-- C++ JustDied: _JustDied() first (boss_urom.cpp:310-314) —
-- SetBossState(DONE) (ScriptedCreature.cpp:519), then Talk(SAY_DEATH);
-- + DoCastSelf(SPELL_DEATH_SPELL 50415, true) (triggered self-cast;
-- the summon rides creatureEffectSummonType, boss_ai.go:3574-3581).
local function uromDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    encounters[guid] = nil
    creature:SetBossState(DATA_UROM, BOSS_STATE_DONE)
    creature:Talk(SAY_DEATH)
    creature:CastSpell(creature, SPELL_DEATH_SPELL, true)
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.uromDone = true
        end
    end
end

-- C++ Reset (boss_urom.cpp:132-140): the SetDisableGravity(false) leg is
-- PORTED in uromReset below (bridge live, lua_creature_events.go:3840),
-- and SetReactState(REACT_AGGRESSIVE) is vacuous here (Lua never sets
-- REACT_PASSIVE), but the DoCastSelf(SPELL_EVOCATE) leg is ungated and
-- port-pattern-ready — self-cast on the reset path (halion
-- twilightHalionReset precedent). The root leg ports via
-- creature:SetRooted(false) (lua_creature_events.go).
local function uromReset(event, creature)
    -- C++ boss_uromAI::Reset (boss_urom.cpp:132-139): _isInCenter=false,
    -- me->SetControlled(false, UNIT_STATE_ROOT) (ports via
    -- creature:SetRooted(false) below), me->SetDisableGravity(false)
    -- (:136), SetReactState(REACT_AGGRESSIVE) (vacuous — Lua never sets
    -- REACT_PASSIVE, documented above), DoCastSelf(SPELL_EVOCATE)
    -- (Reset-only, alive-gated; the death-leg ev23 fire is phantom),
    -- _Reset() bookkeeping.
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    local st = encounters[guid]
    if st then
        st.isInCenter = false
    end
    creature:SetRooted(false)
    creature:SetDisableGravity(false)
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_EVOCATE)
        -- C++ Reset ends with _Reset() (boss_urom.cpp:140) —
        -- SetBossState(NOT_STARTED), != DONE-gated
        -- (ScriptedCreature.cpp:510); _Reset() itself returns early
        -- when dead, so the IsAlive guard above is C++-exact here.
        if creature:GetBossState(DATA_UROM) ~= BOSS_STATE_DONE then
            creature:SetBossState(DATA_UROM, BOSS_STATE_NOT_STARTED)
        end
    end
end

-- C++ JustReachedHome (boss_urom.cpp:142-145): DoCastSelf(SPELL_EVOCATE)
-- — the same ungated self-cast as the Reset leg, fired at the home-move
-- completion (C++ HomeMovementGenerator::DoFinalize). Non-triggered,
-- matching the Reset port.
local function uromReachHome(event, creature)
    creature:CastSpell(creature, SPELL_EVOCATE)
end

-- C++ boss_uromAI::JustEngagedWith (boss_urom.cpp:147-150):
-- BossAI::JustEngagedWith first, then StartAttack(). The base arms
-- port (ScriptedCreature.cpp:537-542) -- SetBossState(IN_PROGRESS)
-- + SetCombatPulseDelay(5) + me-form DoZoneInCombat — then the
-- StartAttack platform/center machine (startAttack, ported above).
local function uromEnterCombat(event, creature, target)
    creature:SetBossState(DATA_UROM, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    startAttack(creature, creature:GetGUIDLow())
end

-- C++ EnterEvadeMode events.Reset() twin (boss_urom.cpp:121-130):
-- the center timers die when combat is left; _platform/_group persist
-- (Reset never touches them) and _isInCenter clears on ev23.
local function uromLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ boss_uromAI::DamageTaken (boss_urom.cpp:322-330): lethal damage
-- while _isInCenter teleports the boss to a valid ground position
-- before dying. Returns (true, damage) — damage unmodified.
local function uromDamageTaken(event, creature, attacker, damage)
    local st = encounters[creature:GetGUIDLow()]
    if st and st.isInCenter and damage >= creature:GetHealth() then
        st.isInCenter = false
        creature:NearTeleport(1124.0432, 1078.2109, 508.3597, 5.4623)
    end
    return true, damage
end

-- C++ boss_uromAI::SpellHit (boss_urom.cpp:293-314): the menagerie
-- teleport spell landing on Urom sets the platform home, tears
-- combat down (LeaveCombat: RemoveAllAuras + CombatStop(false) +
-- EngagementOver) and re-casts Evocate (triggered). Ev14 self-cast
-- verified (see header): the cast from startAttack lands here.
local function uromSpellHit(event, creature, caster, spellId)
    local home = MENAGERIE_HOMES[spellId]
    if not home then
        return
    end
    creature:SetHomePosition(home[1], home[2], home[3], home[4])
    creature:RemoveAllAuras()
    creature:CombatStop(false)
    creature:CastSpell(creature, SPELL_EVOCATE, true)
end

RegisterCreatureEvent(ENTRY_UROM, 1, uromEnterCombat)
RegisterCreatureEvent(ENTRY_UROM, 2, uromLeaveCombat)
RegisterCreatureEvent(ENTRY_UROM, 3, uromTargetDied)
RegisterCreatureEvent(ENTRY_UROM, 4, uromDied)
RegisterCreatureEvent(ENTRY_UROM, 9, uromDamageTaken)
RegisterCreatureEvent(ENTRY_UROM, 14, uromSpellHit)
RegisterCreatureEvent(ENTRY_UROM, 23, uromReset)
RegisterCreatureEvent(ENTRY_UROM, 24, uromReachHome)

-- C++ spell_urom_frostbomb::OnPeriodic (boss_urom.cpp:358-364):
-- EFFECT_1 SPELL_AURA_PERIODIC_DAMAGE. No PreventDefaultAction, so
-- the Go default damage tick still fires. On each tick, when the
-- caster is in heroic mode and in combat, the caster casts SPELL_
-- FROST_BUFFET 58025 on the aura target, triggered. Aura spell is
-- SPELL_FROSTBOMB 51103 (boss_urom.cpp:34).

local function uromFrostbombTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster and caster:IsHeroic() and caster:IsInCombat() then
        caster:CastSpell(target, SPELL_FROST_BUFFET, true)
    end
end

RegisterAuraEvent(SPELL_FROSTBOMB, 1, uromFrostbombTick)
