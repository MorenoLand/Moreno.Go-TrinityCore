-- Razorgore the Untamed (Blackwing Lair) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackwingLair/
-- boss_razorgore.cpp (boss_razorgoreAI); blackwing_lair.h:31
-- (DATA_RAZORGORE_THE_UNTAMED = 0, first boss), :88 (ACTION_PHASE_TWO =
-- 1), :89 (DATA_EGG_EVENT), :49 (NPC_RAZORGORE = 12435). Creature entry:
-- 12435 Razorgore the Untamed (C++ ScriptName "boss_razorgore" per
-- AddSC_boss_razorgore). The go_orb_of_domination GameObjectScript and
-- the spell_egg_event SpellScript from the same C++ file are not
-- registered: the orb arm blocks on instance GetData (no bridge) and
-- the orb GO entry (DB-only, not in the C++ tree); the egg-event
-- SetData bearer now exists (instanceDataOf below, gruul precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset, 38 OnDoAction. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. OnDied: Talk(SAY_DEATH=3). The SAY_EGGS_BROKEN
-- talk lines (0-2) are never used by the boss AI and are not modeled.
-- Fight shape (C++-exact for the modeled arms): phase one has no
-- events scheduled — the boss takes no part in combat until
-- DoAction(ACTION_PHASE_TWO). DamageTaken zeroes all damage while
-- secondPhase is false (the C++ @todo admits this is wrong — real
-- behavior nukes the room and respawns him — but the damage-zeroing is
-- the shipped logic). The phase-two DoAction trigger comes from the
-- instance egg machine once the egg event completes; ev38 is registered
-- below (dispatch bridged, lua_creature_events.go:802) and the sender
-- is ported (instance_blackwing_lair.lua); phase-two timers fire once
-- the egg machine sends DoAction — in-game that still waits on the orb
-- gossip (mind-control, unported), the only source of the 19873
-- egg-destroy casts, so Razorgore stays unkillable doing melee only
-- until the orb lands — which is exactly what C++ phase one does in
-- combat. Reset cancels timers and clears the phase flag.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gates
-- (boss_razorgore.cpp pre-loop :132 + in-loop :157) are ported as a
-- 500ms IsCasting defer on every ported phase-two timer tick
-- (mal_ganis idiom; IsCasting live, lua_creature_events.go:1950;
-- defer precedes target picks, casts and re-arms, no re-arm
-- consumed); the SetBossState-override arms still have no hook, but
-- the instance SetData arms ARE ported (instanceDataOf bearer; the
-- map-469 instance binding is instance_blackwing_lair.lua); the boss's
-- own _Reset/_JustDied/BossAI::JustEngagedWith
-- bookkeeping writes ARE ported (creature:SetBossState, bridge live
-- lua_creature_events.go:5917):
-- ev1 IN_PROGRESS (no JustEngagedWith override — base runs,
-- ScriptedCreature.h:329), ev23 NOT_STARTED (Reset :84-89 calls _Reset();
-- alive + != DONE gate, ScriptedCreature.cpp:499-511), ev4 DONE (JustDied
-- :92-97 calls _JustDied()). The Reset/JustDied
-- instance->SetData(DATA_EGG_EVENT, NOT_STARTED) arms ride the instance
-- bearer (instance_blackwing_lair.lua, map 469); RemoveAllAuras/SetFullHealth in DoChangePhase are PORTED
-- (RemoveAllAuras live, lua_creature_events.go:1523; SetFullHealth rides
-- SetHealth(GetMaxHealth())) — see doChangePhase below; the
-- go_orb_of_domination OnGossipHello (instance
-- GetCreature(DATA_RAZORGORE_THE_UNTAMED) -> Attack(player, true) +
-- CastSpell SPELL_MINDCONTROL 42013) rides instance legs with no bridge
-- (GO gossip hello itself is live) and the spell_egg_event
-- SpellScript (OnHit -> instance->SetData(DATA_EGG_EVENT, SPECIAL))
-- is ported in instance_blackwing_lair.lua as
-- RegisterSpellEvent(19873, 2). The mind-control/orb gameplay has no
-- bearer, so the fight cannot reach phase two until the orb gossip
-- lands (the egg machine itself, instance_blackwing_lair.cpp:262-265,
-- is ported).

local ENTRY_RAZORGORE = 12435

-- blackwing_lair.h BWLMisc: ACTION_PHASE_TWO = 1, DATA_EGG_EVENT = 2.
local DATA_EGG_EVENT = 2

-- The boss files reach the instance script through the shared
-- instanceData table (gruul.lua precedent:
-- creature:GetMap():GetInstanceData()); the map-469 binding is
-- instance_blackwing_lair.lua, so every write below is nil-guarded
-- only for the unloaded / non-instance case.
local function instanceDataOf(creature)
    local mapObj = creature:GetMap()
    return mapObj and mapObj:GetInstanceData() or nil
end

-- C++ BossAI::_Reset / _JustDied (ScriptedCreature.cpp:499-516):
-- summons.DespawnAll() over the registry instance_blackwing_lair.lua
-- records (instanceData.razorSummons, guidLow set). GetCreatureByGUID
-- resolves each low GUID on this map+instance (lua_creature_events.go:
-- 4050); Despawn is DespawnOrUnsummon — permanent for the temp-summon
-- wave adds, normal respawn timer for DB creatures. The registry is
-- cleared first so a re-entrant despawn cannot double-run it.
local function despawnSummons(creature)
    local data = instanceDataOf(creature)
    if not data or not data.razorSummons then
        return
    end
    local registry = data.razorSummons
    data.razorSummons = nil
    for guidLow in pairs(registry) do
        local target = creature:GetCreatureByGUID(guidLow)
        if target then
            target:Despawn()
        end
    end
end

local SPELL_CLEAVE = 22540
local SPELL_WARSTOMP = 24375
local SPELL_FIREBALLVOLLEY = 22425
local SPELL_CONFLAGRATION = 23023

local timers = {}
local secondPhase = {}

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

-- C++ EVENT_CLEAVE: non-triggered DoCastVictim(22540); re-arm {7s,10s}.
local function onCleave(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function()
            onCleave(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", math.random(7000, 10000), function()
        onCleave(creature, guid)
    end)
end

-- C++ EVENT_STOMP: non-triggered DoCastVictim(24375); re-arm {15s,25s}.
local function onWarstomp(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "stomp", 500, function()
            onWarstomp(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_WARSTOMP)
    end
    schedule(guid, "stomp", math.random(15000, 25000), function()
        onWarstomp(creature, guid)
    end)
end

-- C++ EVENT_FIREBALL: non-triggered DoCastVictim(22425); re-arm {12s,15s}.
local function onFireballVolley(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "fireball", 500, function()
            onFireballVolley(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIREBALLVOLLEY)
    end
    schedule(guid, "fireball", math.random(12000, 15000), function()
        onFireballVolley(creature, guid)
    end)
end

-- C++ EVENT_CONFLAGRATION: non-triggered DoCastVictim(23023); re-arm 30s.
local function onConflagration(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "conflag", 500, function()
            onConflagration(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CONFLAGRATION)
    end
    schedule(guid, "conflag", 30000, function()
        onConflagration(creature, guid)
    end)
end

-- C++ DoChangePhase: arms the four phase-two timers, sets secondPhase,
-- RemoveAllAuras + SetFullHealth (both bridged now: RemoveAllAuras
-- is live, SetFullHealth rides SetHealth(GetMaxHealth())). Registered
-- on ev38 below; the sender is the ported instance egg machine
-- (instance_blackwing_lair.lua), which fires once 15 eggs are
-- counted.
local function doChangePhase(creature, guid)
    secondPhase[guid] = true
    schedule(guid, "cleave", 15000, function()
        onCleave(creature, guid)
    end)
    schedule(guid, "stomp", 35000, function()
        onWarstomp(creature, guid)
    end)
    schedule(guid, "fireball", 7000, function()
        onFireballVolley(creature, guid)
    end)
    schedule(guid, "conflag", 12000, function()
        onConflagration(creature, guid)
    end)
    creature:RemoveAllAuras()
    creature:SetHealth(creature:GetMaxHealth())
end

local function razorgoreResetState(guid)
    cancelTimers(guid)
    secondPhase[guid] = nil
end

-- C++ DoAction(ACTION_PHASE_TWO) (boss_razorgore.cpp:112-115): the
-- instance egg machine sends it once the 15th counted egg is destroyed
-- (instance_blackwing_lair.cpp:262-265); ev38 dispatch is bridged
-- (lua_creature_events.go:802) and fires (event, creature, action).
local function razorgoreDoAction(event, creature, action)
    if action == 1 then -- ACTION_PHASE_TWO
        doChangePhase(creature, creature:GetGUIDLow())
    end
end

local function razorgoreEnterCombat(event, creature, target)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537): the base call
    -- sets the boss state IN_PROGRESS; DATA_RAZORGORE_THE_UNTAMED=0
    -- (blackwing_lair.h:31). No JustEngagedWith override in C++ — the
    -- base runs by virtual dispatch (ScriptedCreature.h:329).
    creature:SetBossState(0, 1)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    secondPhase[guid] = nil
    -- C++ phase one schedules nothing: the mind-controlled orb
    -- handler drives the fight, not the AI. Combat AI starts only on
    -- DoAction(ACTION_PHASE_TWO), registered on ev38 below; its sender
    -- is the ported instance egg machine
    -- (instance_blackwing_lair.lua), which fires once 15 eggs are
    -- counted (in-game: after the orb gossip lands).
end

-- C++ DamageTaken: pre-damage hook; zeroes all damage until phase two
-- (thekal convention — return false, 0). No adjustment needed: the
-- zeroing is C++-exact as shipped (the @todo is noted in the header).
local function razorgoreDamageTaken(event, creature, attacker, damage)
    if not secondPhase[creature:GetGUIDLow()] then
        return false, 0
    end
end

local function razorgoreLeaveCombat(event, creature)
    razorgoreResetState(creature:GetGUIDLow())
end

local function razorgoreDied(event, creature, killer)
    -- C++ JustDied (boss_razorgore.cpp:92-97): _JustDied() writes
    -- SetBossState(DATA_RAZORGORE_THE_UNTAMED 0, DONE) first, then
    -- Talk(SAY_DEATH), then SetData(DATA_EGG_EVENT, NOT_STARTED) (:97)
    -- via the instance bearer (instance_blackwing_lair.lua). _JustDied
    -- despawns the summons registry BEFORE the DONE write
    -- (ScriptedCreature.cpp:513-517), so despawnSummons runs first.
    -- The instance OnUnitDeath DONE path
    -- (instance_blackwing_lair.cpp:233-237) stays instance-side — the
    -- boss's own ev4 leg covers a normal kill.
    despawnSummons(creature)
    creature:SetBossState(0, 3)
    creature:Talk(3) -- SAY_DEATH
    local data = instanceDataOf(creature)
    if data then
        data:SetData(DATA_EGG_EVENT, 0) -- NOT_STARTED
    end
    razorgoreResetState(creature:GetGUIDLow())
end

local function razorgoreReset(event, creature)
    -- C++ Reset (boss_razorgore.cpp:84-89): _Reset() writes
    -- SetBossState(DATA_RAZORGORE_THE_UNTAMED 0, NOT_STARTED), alive-
    -- gated and skipped when already DONE (ScriptedCreature.cpp:499-511);
    -- Go fires ev23 on the death path too, hence the gate. The
    -- SetData(DATA_EGG_EVENT, NOT_STARTED) arm (:89) rides the instance
    -- bearer (instance_blackwing_lair.lua) and is
    -- unconditional, mirroring C++ Reset; on the death path ev4 already
    -- wrote the same value, so the duplicate write is a no-op.
    if creature:IsAlive() and creature:GetBossState(0) ~= 3 then
        creature:SetBossState(0, 0)
    end
    local data = instanceDataOf(creature)
    if data then
        data:SetData(DATA_EGG_EVENT, 0) -- NOT_STARTED
    end
    razorgoreResetState(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_RAZORGORE, 1, razorgoreEnterCombat)
RegisterCreatureEvent(ENTRY_RAZORGORE, 2, razorgoreLeaveCombat)
RegisterCreatureEvent(ENTRY_RAZORGORE, 4, razorgoreDied)
RegisterCreatureEvent(ENTRY_RAZORGORE, 9, razorgoreDamageTaken)
RegisterCreatureEvent(ENTRY_RAZORGORE, 23, razorgoreReset)
RegisterCreatureEvent(ENTRY_RAZORGORE, 38, razorgoreDoAction)
