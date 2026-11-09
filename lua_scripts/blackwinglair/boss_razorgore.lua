-- Razorgore the Untamed (Blackwing Lair) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackwingLair/
-- boss_razorgore.cpp (boss_razorgoreAI); blackwing_lair.h:31
-- (DATA_RAZORGORE_THE_UNTAMED = 0, first boss), :88 (ACTION_PHASE_TWO =
-- 1), :89 (DATA_EGG_EVENT), :49 (NPC_RAZORGORE = 12435). Creature entry:
-- 12435 Razorgore the Untamed (C++ ScriptName "boss_razorgore" per
-- AddSC_boss_razorgore). The go_orb_of_domination GameObjectScript and
-- the spell_egg_event SpellScript from the same C++ file are not
-- registered: the orb arm blocks on instance GetData/GetCreature legs
-- (GO gossip hello itself is live, gameobject_hooks.go:167) and there
-- is no spellscript model.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. OnDied: Talk(SAY_DEATH=3). The SAY_EGGS_BROKEN
-- talk lines (0-2) are never used by the boss AI and are not modeled.
-- Fight shape (C++-exact for the modeled arms): phase one has no
-- events scheduled — the boss takes no part in combat until
-- DoAction(ACTION_PHASE_TWO). DamageTaken zeroes all damage while
-- secondPhase is false (the C++ @todo admits this is wrong — real
-- behavior nukes the room and respawns him — but the damage-zeroing is
-- the shipped logic). The phase-two DoAction trigger comes from the
-- instance script once the egg event completes; ev38 dispatch is bridged
-- (lua_creature_events.go:802) but the sender is the unported instance
-- script, so it can never fire and secondPhase stays false: phase-two timers (cleave 22540 15s then {7s,10s} /
-- warstomp 24375 35s then {15s,25s} / fireball volley 22425 7s then
-- {12s,15s} / conflagration 23023 12s then 30s, all non-triggered
-- DoCastVictim with unconditional re-arms) are modeled as code but
-- unreachable, and Razorgore stays unkillable doing melee only — which
-- is exactly what C++ phase one does in combat. Reset cancels timers
-- and clears the phase flag.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gates
-- (boss_razorgore.cpp pre-loop :132 + in-loop :157) are ported as a
-- 500ms IsCasting defer on every ported phase-two timer tick
-- (mal_ganis idiom; IsCasting live, lua_creature_events.go:1950;
-- defer precedes target picks, casts and re-arms, no re-arm
-- consumed); no instance-script model — the instance SetData /
-- SetBossState-override arms have no bearer; the boss's own
-- _Reset/_JustDied/BossAI::JustEngagedWith bookkeeping writes ARE
-- ported (creature:SetBossState, bridge live lua_creature_events.go:5917):
-- ev1 IN_PROGRESS (no JustEngagedWith override — base runs,
-- ScriptedCreature.h:329), ev23 NOT_STARTED (Reset :84-89 calls _Reset();
-- alive + != DONE gate, ScriptedCreature.cpp:499-511), ev4 DONE (JustDied
-- :92-97 calls _JustDied()). The Reset/JustDied
-- instance->SetData(DATA_EGG_EVENT, NOT_STARTED) arms have no bearer; RemoveAllAuras/SetFullHealth in DoChangePhase are PORTED
-- (RemoveAllAuras live, lua_creature_events.go:1523; SetFullHealth rides
-- SetHealth(GetMaxHealth())) — see doChangePhase below; the
-- go_orb_of_domination OnGossipHello (instance
-- GetCreature(DATA_RAZORGORE_THE_UNTAMED) -> Attack(player, true) +
-- CastSpell SPELL_MINDCONTROL 42013) rides instance legs with no bridge
-- (GO gossip hello itself is live) and the spell_egg_event
-- SpellScript (OnHit -> instance->SetData(DATA_EGG_EVENT, SPECIAL))
-- has no bridge — the mind-control/orb/egg-destruction gameplay
-- has no bearer, so the fight can never reach phase two (ev38 dispatch is
-- bridged, lua_creature_events.go:802; the blocker is the unported
-- instance-script sender, instance_blackwing_lair.cpp:262-265).

local ENTRY_RAZORGORE = 12435

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
-- is live, SetFullHealth rides SetHealth(GetMaxHealth())). Unreachable today: ev38 dispatch bridged, but the sender is the unported
-- instance script — instance_blackwing_lair.cpp:265 (EVENT_RAZOR_PHASE_TWO, fed
-- by egg-count SetData at :217-227) — so ACTION_PHASE_TWO (1) can never fire.
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
    -- DoAction(ACTION_PHASE_TWO), whose ev38 dispatch is bridged
    -- (lua_creature_events.go:802) but whose sender is the unported instance
    -- script (instance_blackwing_lair.cpp:262-265), so it never fires.
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
    -- Talk(SAY_DEATH); the SetData(DATA_EGG_EVENT, NOT_STARTED) arm
    -- has no bearer. The instance OnUnitDeath DONE path
    -- (instance_blackwing_lair.cpp:233-237) stays instance-side — the
    -- boss's own ev4 leg covers a normal kill.
    creature:SetBossState(0, 3)
    creature:Talk(3) -- SAY_DEATH
    razorgoreResetState(creature:GetGUIDLow())
end

local function razorgoreReset(event, creature)
    -- C++ Reset (boss_razorgore.cpp:84-89): _Reset() writes
    -- SetBossState(DATA_RAZORGORE_THE_UNTAMED 0, NOT_STARTED), alive-
    -- gated and skipped when already DONE (ScriptedCreature.cpp:499-511);
    -- Go fires ev23 on the death path too, hence the gate. The
    -- SetData(DATA_EGG_EVENT, NOT_STARTED) arm has no bearer.
    if creature:IsAlive() and creature:GetBossState(0) ~= 3 then
        creature:SetBossState(0, 0)
    end
    razorgoreResetState(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_RAZORGORE, 1, razorgoreEnterCombat)
RegisterCreatureEvent(ENTRY_RAZORGORE, 2, razorgoreLeaveCombat)
RegisterCreatureEvent(ENTRY_RAZORGORE, 4, razorgoreDied)
RegisterCreatureEvent(ENTRY_RAZORGORE, 9, razorgoreDamageTaken)
RegisterCreatureEvent(ENTRY_RAZORGORE, 23, razorgoreReset)
