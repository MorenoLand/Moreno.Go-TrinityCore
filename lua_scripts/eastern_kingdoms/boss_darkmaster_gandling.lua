-- Darkmaster Gandling (1853), Scholomance final boss — Lua port of
-- src/server/scripts/EasternKingdoms/Scholomance/
-- boss_darkmaster_gandling.cpp (creature AI only); entry C++-named in
-- scholomance.h (NPC_DARKMASTER_GANDLING=1853 :42, DATA_DARKMASTERGANDLING=6
-- :36; RegisterScholomanceCreatureAI binds the creature_template ScriptName
-- DB-side). Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
--
-- Modeled arms (C++-exact where bridges exist):
-- OnEnterCombat: arcane missiles 15790 at 4.5s->8s / shadow shield 12040
-- (non-triggered self) at 12s->{14s,28s} / curse 18702 at 2s->{15s,27s} /
-- shadow portal 17950 at 15s->{17s,27s}. Triggered DoCastVictim ->
-- GetVictim + CastSpell(victim, spell, true), nil-victim keeps schedule
-- (moroes/maiden triggered convention; the C++ DoCast's triggered flag
-- -> CastSpell's third argument). urand re-arms -> math.random
-- (sulfuron convention). Shadow portal: only while own current health is
-- strictly above 3% (HealthAbovePct(3) current-health read); target =
-- random alive player within 100yd (SelectTarget(Random, 0, 100, true),
-- janalai/kelris randomPlayerInRange convention), nil pick casts nothing
-- but the re-arm still happens — the {17s,27s} schedule lives INSIDE the
-- health gate in C++, so below 3% the event is never re-armed and dies
-- (modeled by skipping the reschedule entirely). IsSummonedBy's
-- Talk(YELL_SUMMONED 0) is document-only with its MoveRandom(5) (no
-- MotionMaster bridge, gizrul precedent) — Gandling is map-spawned, not
-- summoned, so the arm never fires in normal play.
--
-- Documented-unmodeled (no bridges):
-- Reset/JustDied/JustEngagedWith GO_GATE_GANDLING (177374) state legs
-- (SetGoState ACTIVE/ACTIVE/READY) PORTED below via
-- creature:FindNearestGameObject — C++ used instance GetGuidData;
-- the entry is unique in the instance, so the target is identical.
-- spell_shadow_portal (17950) + spell_shadow_portal_rooms (17863..)
-- PORTED below via RegisterSpellEvent (17950 ev5 room picker + the six
-- room spells ev4): up to 6 random picks among rooms whose gates are
-- still open, the hit unit casts that room's portal on itself, and the
-- room arm summons 3x NPC_RISEN_GUARDIAN 11598 at the room's SummonPos,
-- MoveRandom(5), and closes that room's gate. UpdateAI UNIT_STATE_CASTING queue
-- gate PORTED (creature:IsCasting - due arms defer 500ms, broodkeeper
-- precedent) + DATA_DARKMASTERGANDLING bookkeeping has no bridge
-- (luaBossAI shim; base pulse/zone arms PORTED in handler).

local ENTRY_GANDLING = 1853
local GO_GATE_GANDLING = 177374 -- scholomance.h:49

local SPELL_ARCANE_MISSILES = 15790
local SPELL_SHADOW_SHIELD = 12040
local SPELL_CURSE = 18702
local SPELL_SHADOW_PORTAL = 17950

local GO_GATE_RAVENIAN = 177372 -- scholomance.h:50
local GO_GATE_THEOLEN = 177377 -- scholomance.h:51
local GO_GATE_ILLUCIA = 177371 -- scholomance.h:52
local GO_GATE_MALICIA = 177375 -- scholomance.h:53
local GO_GATE_BAROV = 177373 -- scholomance.h:54
local GO_GATE_POLKELT = 177376 -- scholomance.h:55

local NPC_RISEN_GUARDIAN = 11598

-- C++ SPSpells (boss_darkmaster_gandling.cpp:178-185) + ScriptEventId
-- (:285-292): each room portal spell pairs 1:1 with its event id in room
-- order; Lua has no MiscValue read, so the room arm recovers the room
-- from the spell id (spell:GetEntry()). pos is the C++ 0-based SummonPos
-- start index (Lua 1-based at use, +1); Hall of Secrets / Vault of the
-- Ravenian reuse pos 0 in C++ ("Not yet spawned" — they summon at the
-- Hall of the Damned positions), kept C++-exact here. phase is the
-- C++ AI SetData(0, phase) value; the SetData leg itself is a no-op
-- (entry 11598 has no CreatureScript in-tree), so it is not called.
local SHADOW_PORTAL_ROOMS = {
    { spell = 17863, pos = 0, phase = 1, gate = GO_GATE_RAVENIAN }, -- Hall of Secrets
    { spell = 17939, pos = 0, phase = 2, gate = GO_GATE_THEOLEN }, -- Hall of the Damned
    { spell = 17943, pos = 3, phase = 3, gate = GO_GATE_MALICIA }, -- The Coven
    { spell = 17944, pos = 6, phase = 4, gate = GO_GATE_ILLUCIA }, -- The Shadow Vault
    { spell = 17946, pos = 9, phase = 5, gate = GO_GATE_BAROV }, -- Barov Family Vault
    { spell = 17948, pos = 0, phase = 6, gate = GO_GATE_POLKELT }, -- Vault of the Ravenian
}

-- C++ SummonPos (boss_darkmaster_gandling.cpp:257-282), file order: the
-- first three are the Hall of the Damned positions; the array holds 12
-- of 18 slots, the trailing six defaulting to 0,0,0,0 and never indexed
-- (the highest pos used is 9+2=11).
local SHADOW_PORTAL_SUMMON_POS = {
    { 177.9624, -68.23893, 84.95197, 3.228859 },
    { 183.7705, -61.43489, 84.92424, 5.148721 },
    { 184.7035, -77.74805, 84.92424, 4.660029 },
    { 111.7203, -1.105035, 85.45985, 3.961897 },
    { 118.0079, 6.430664, 85.31169, 2.408554 },
    { 120.0276, -7.496636, 85.31169, 2.984513 },
    { 245.3716, 0.628038, 72.73877, 0.01745329 },
    { 240.9920, 3.405653, 72.73877, 6.143559 },
    { 240.9543, -3.182943, 72.73877, 0.2268928 },
    { 181.8245, -42.58117, 75.4812, 4.660029 },
    { 177.7456, -42.74745, 75.4812, 4.886922 },
    { 185.6157, -42.91200, 75.4812, 4.45059 },
}

-- Rooms whose gates this script has closed, per instance id: Go stamps
-- every gameobject GO_STATE_READY (1) at load (lua_objects.go:3872),
-- so a never-touched room gate reads closed even though C++ reads its
-- DB spawn state (ACTIVE/open) — the latch, not GetGoState, is the
-- open/closed record (a live ACTIVE read still counts as open). Keyed
-- by instance id (moroes precedent): a fresh instance gets a fresh
-- bucket, so rooms reopen on instance reset; a wipe in the same
-- instance keeps them closed, as in C++.
local closedShadowPortalRooms = {}

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

local function castOnVictim(creature, spellId, triggered)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spellId, triggered)
    end
end

local function healthAbovePct(creature, pct)
    local maxHealth = creature:GetMaxHealth()
    return maxHealth > 0 and creature:GetHealth() > math.floor(maxHealth * pct / 100)
end

-- C++ SelectTarget(Random, 0, 100, true): random alive
-- threat-list player within maxDist yards.
local function randomThreatPlayerInRange(creature, maxDist)
    local targets = creature:GetAITargets()
    local out = {}
    for _, unit in ipairs(targets) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) - creature:GetCombatReach() - 1.5 < maxDist then
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

-- C++ EVENT_ARCANEMISSILES: triggered DoCastVictim(15790); Repeat(8s).
local function onArcaneMissiles(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "arcaneMissiles", 500, function()
            onArcaneMissiles(creature, guid)
        end)
        return
    end
    castOnVictim(creature, SPELL_ARCANE_MISSILES, true)
    schedule(guid, "arcaneMissiles", 8000, function()
        onArcaneMissiles(creature, guid)
    end)
end

-- C++ EVENT_SHADOWSHIELD: DoCast(me, 12040) non-triggered; Repeat(14s, 28s).
local function onShadowShield(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shadowShield", 500, function()
            onShadowShield(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SHADOW_SHIELD)
    schedule(guid, "shadowShield", math.random(14000, 28000), function()
        onShadowShield(creature, guid)
    end)
end

-- C++ EVENT_CURSE: triggered DoCastVictim(18702); Repeat(15s, 27s).
local function onCurse(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "curse", 500, function()
            onCurse(creature, guid)
        end)
        return
    end
    castOnVictim(creature, SPELL_CURSE, true)
    schedule(guid, "curse", math.random(15000, 27000), function()
        onCurse(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_PORTAL: only if HealthAbovePct(3); triggered DoCast on
-- the random player target; Repeat(17s, 27s) lives inside the gate.
local function onShadowPortal(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shadowPortal", 500, function()
            onShadowPortal(creature, guid)
        end)
        return
    end
    if not healthAbovePct(creature, 3) then
        return
    end
    local target = randomThreatPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_PORTAL, true)
    end
    schedule(guid, "shadowPortal", math.random(17000, 27000), function()
        onShadowPortal(creature, guid)
    end)
end

-- C++ JustEngagedWith: BossAI::JustEngagedWith pulse/zone arms PORTED + the four events +
-- GO_GATE_GANDLING SetGoState(READY) PORTED below (FindNearestGameObject).
local function gandlingEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540; boss_darkmaster_gandling.cpp:81 base call first)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542; boss_darkmaster_gandling.cpp:81 base call first)
    schedule(guid, "arcaneMissiles", 4500, function()
        onArcaneMissiles(creature, guid)
    end)
    schedule(guid, "shadowShield", 12000, function()
        onShadowShield(creature, guid)
    end)
    schedule(guid, "curse", 2000, function()
        onCurse(creature, guid)
    end)
    schedule(guid, "shadowPortal", 15000, function()
        onShadowPortal(creature, guid)
    end)
    local gate = creature:FindNearestGameObject(GO_GATE_GANDLING, 100.0)
    if gate then gate:SetGoState(1) end -- GO_STATE_READY
end

local function gandlingLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function gandlingDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    local gate = creature:FindNearestGameObject(GO_GATE_GANDLING, 100.0)
    if gate then gate:SetGoState(0) end -- GO_STATE_ACTIVE
end

-- C++ Reset: _Reset + GO_GATE_GANDLING SetGoState(ACTIVE) PORTED below.
local function gandlingReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    local gate = creature:FindNearestGameObject(GO_GATE_GANDLING, 100.0)
    if gate then gate:SetGoState(0) end -- GO_STATE_ACTIVE
end

RegisterCreatureEvent(ENTRY_GANDLING, 1, gandlingEnterCombat)
RegisterCreatureEvent(ENTRY_GANDLING, 2, gandlingLeaveCombat)
RegisterCreatureEvent(ENTRY_GANDLING, 4, gandlingDied)
RegisterCreatureEvent(ENTRY_GANDLING, 23, gandlingReset)

-- C++ spell_shadow_portal::HandleCast (boss_darkmaster_gandling.cpp:182-231,
-- registered OnEffectHitTarget EFFECT_0 SPELL_EFFECT_DUMMY at :234): up to
-- 6 tries, each a urand(0,5) room pick; a room qualifies only while its
-- gate is open; the hit unit then casts that room's portal spell on
-- itself (non-triggered). Fires on the creature cast path
-- (RegisterSpellEvent ev5, boss_ai.go OnEffectHitTarget leg). The
-- player-side room cast rides the triggered-only player funnel (the
-- player CastSpell bridge takes no triggered arg), a documented delta
-- from C++'s non-triggered cast.
local function shadowPortalPick(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local closed = closedShadowPortalRooms[caster:GetInstanceId()]
    local spellId = nil
    local attempts = 0
    while spellId == nil do
        if attempts >= 6 then
            break
        end
        attempts = attempts + 1
        local room = SHADOW_PORTAL_ROOMS[math.random(#SHADOW_PORTAL_ROOMS)]
        local gate = caster:FindNearestGameObject(room.gate, 100.0)
        if gate ~= nil and (gate:GetGoState() == 0 or closed == nil or closed[room.spell] == nil) then
            spellId = room.spell
        end
    end
    if spellId ~= nil then
        target:CastSpell(target, spellId)
    end
end

RegisterSpellEvent(SPELL_SHADOW_PORTAL, 5, shadowPortalPick)

-- C++ spell_shadow_portal_rooms::HandleSendEvent (boss_darkmaster_gandling.cpp:306-367,
-- registered OnEffectHit EFFECT_1 SPELL_EFFECT_SEND_EVENT at :370): the
-- room spell's effect-1 MiscValue (5618-5623) selects the room; summons
-- 3x NPC_RISEN_GUARDIAN at the room's SummonPos with
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT (2min), MoveRandom(5), AI
-- SetData(0, phase), and closes that room's gate (GO_STATE_READY).
-- The SetData leg is a no-op in C++ too (see SHADOW_PORTAL_ROOMS).
-- Fires on the player cast path (RegisterSpellEvent ev4, spells.go
-- OnEffectHit leg); the SEND_EVENT default is an explicit no-op there,
-- so the hook is the only effect. One handler for all six room spells,
-- the room recovered from spell:GetEntry() (no MiscValue bridge). The
-- gate search runs 200yd: the hook fires in the HIT phase, before the
-- room spell's teleport effect, so the caster may still be up to 100yd
-- from Gandling on the far side of a ~70yd-distant gate.
local function shadowPortalRoom(event, spell, effectIndex)
    if effectIndex ~= 1 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local room = nil
    for _, r in ipairs(SHADOW_PORTAL_ROOMS) do
        if r.spell == spell:GetEntry() then
            room = r
            break
        end
    end
    if room == nil then
        return
    end
    for i = 0, 2 do
        local pos = SHADOW_PORTAL_SUMMON_POS[room.pos + 1 + i]
        local summoned = caster:SummonCreature(NPC_RISEN_GUARDIAN, pos[1], pos[2], pos[3], pos[4], 3, 120000)
        if summoned ~= nil then
            summoned:MoveRandom(5)
        end
    end
    local gate = caster:FindNearestGameObject(room.gate, 200.0)
    if gate ~= nil then
        gate:SetGoState(1) -- GO_STATE_READY
        local instanceId = caster:GetInstanceId()
        local closed = closedShadowPortalRooms[instanceId]
        if closed == nil then
            closed = {}
            closedShadowPortalRooms[instanceId] = closed
        end
        closed[room.spell] = true
    end
end

for _, room in ipairs(SHADOW_PORTAL_ROOMS) do
    RegisterSpellEvent(room.spell, 4, shadowPortalRoom)
end
