-- Vaelastrasz the Corrupt (Blackwing Lair) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackwingLair/boss_vaelastrasz.cpp
-- (boss_vaelastrasz — BossAI combat scheduler + 15%-HP yell latch +
-- KilledUnit talk). Boss-script unit per eastern_kingdoms_script_loader.cpp
-- order (instance_blackrock_spire blocked on the instance-script model;
-- boss_razorgore blocked: P2 machine driven by instance DoAction +
-- gameobject gossip + SpellScript — no bridges; AddSC_boss_vaelastrasz
-- next in the Blackwing Lair block).
-- Entry (verifiable from the C++ sources): blackwing_lair.h:54 names
-- NPC_VAELASTRAZ = 13020 in the BWL creatures enum; the AI runs
-- under BossAI(DATA_VAELASTRAZ_THE_CORRUPT) via GetBlackwingLairAI
-- (a GetInstanceAI retrieval wrapper only; the combat arms have no
-- instance state). The creature_template ScriptName binding is DB-side
-- (no TDB in this workspace), but the entry itself is C++-verifiable so
-- the port is registered (boss_urok_doomhowl precedent).
-- Verifiable numbers (file's own enums/header): SPELL_ESSENCEOFTHERED =
-- 23513 / SPELL_FLAMEBREATH = 23461 / SPELL_FIRENOVA = 23462 /
-- SPELL_TAILSWIPE = 15847 (cast body fully commented out in C++ — no
-- arm) / SPELL_BURNINGADRENALINE = 18173 / SPELL_BURNINGADRENALINE_
-- EXPLOSION = 23478 (AuraScript only) / SPELL_CLEAVE = 19983 /
-- SAY_LINE1 = 0 .. SAY_LINE3 = 2 (gossip speech chain — no bridge) /
-- SAY_HALFLIFE = 3 / SAY_KILLTARGET = 4 / GOSSIP_ID = 6101
-- (OnGossipSelect — no bridge) / EVENT_CLEAVE = 9 / EVENT_FLAMEBREATH
-- = 6 / EVENT_FIRENOVA = 7 / EVENT_TAILSWIPE = 8 (no arm) /
-- EVENT_BURNINGADRENALINE_CASTER = 10 (SelectTarget-gated — no bridge)
-- / EVENT_BURNINGADRENALINE_TANK = 11.
-- Reset() _Reset() / constructor gossip/faction flags are internal
-- machinery covered here by the cancel on combat events (halycon
-- precedent); Reset's SetStandState(DEAD) is pre-fight cinematic state
-- with no bridge. JustDied has no C++ arms (no override).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied (C++ Eluna::KilledUnit — lua_creature_events.go fires it
-- with (event, creature, victim) from Unit::Kill), 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Driven by CreateLuaEvent timers; melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- Combat entry: self-cast SPELL_ESSENCEOFTHERED 23513 (creature:
-- CastSpell(creature, spell) — me->CastSpell self-cast) + SetHealth(30%
-- of max) (creature:SetHealth is bridged in lua_objects.go:172;
-- C++-exact SetHealth(me->CountPctFromMaxHealth(30))).
-- SPELL_CLEAVE: victim-cast 19983, 10s init -> 15s loop.
-- SPELL_FLAMEBREATH: victim-cast 23461, 15s init -> 8s loop
-- (C++ urand(8s,14s) loop — ranges use the lower bound, halycon
-- recurring-timer convention).
-- SPELL_FIRENOVA: victim-cast 23462 (C++ DoCastVictim), 20s init ->
-- 15s loop.
-- SPELL_BURNINGADRENALINE (tank): victim-cast 18173, 45s init -> 45s
-- loop (C++ me->CastSpell(me->GetVictim(), 18173, true)).
-- Victim casts are GetVictim nil-guarded (incarcerator convention).
-- Timers start on OnEnterCombat(1), cancelled on OnLeaveCombat(2)/
-- OnDied(4)/OnReset(23).
-- 15%-HP yell latch via OnDamageTaken(9) with a per-guid one-shot flag
-- reset on OnEnterCombat(1) (gyth convention — C++-exact HealthBelowPct
-- (15) condition, creature:GetHealth() - damage < 0.15*GetMaxHealth()):
-- Talk(SAY_HALFLIFE) once (maiden-of-virtue convention; creature:Talk
-- is bridged in lua_creature_events.go). C++ checks the latch in
-- UpdateAI even out of combat, but the only state transition into
-- sub-15% is damage, so the damage-event latch is faithful.
-- KilledUnit: 20% chance (C++-exact rand32() % 5) -> Talk(SAY_KILLTARGET)
-- via OnTargetDied(3); creature:Talk takes only the text id — the C++
-- victim targeting of the talk is dropped (deviation, documented).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (casts are
-- packet-visual), so timers fire unconditionally and both in-loop
-- casting gates are dropped (maiden precedent). The C++ scheduler is
-- driven from JustEngagedWith and drained by UpdateAI; the port
-- schedules from OnEnterCombat(1) and cancels on 2/4/23 (maiden
-- convention). me->ResetPlayerDamageReq() has no bridge (documented).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - the whole gossip pre-fight: constructor SetFlag(UNIT_NPC_FLAGS,
--   UNIT_NPC_FLAG_GOSSIP) + SetFaction(FACTION_FRIENDLY), BeginSpeech
--   via OnGossipSelect(GOSSIP_ID 6101), EVENT_SPEECH_1..4 chain
--   (Talk SAY_LINE1..3, SetStandState(STAND), HandleEmoteCommand,
--   SetFaction(FACTION_DRAGONFLIGHT_BLACK), AttackStart(PlayerGUID))
--   — no creature-gossip bridge.
-- - EVENT_TAILSWIPE: the cast is fully commented out in C++; the event
--   only re-schedules itself — nothing ported (rend wave-table
--   precedent).
-- - EVENT_BURNINGADRENALINE_CASTER: gates on SelectTarget(Random, 1,
--   mana-user non-pet without the aura) — no SelectTarget bridge
--   (the_beast / doomwalker precedent); no timer ported.
-- - spell_vael_burning_adrenaline AuraScript (AfterEffectRemove ->
--   self-cast SPELL_BURNINGADRENALINE_EXPLOSION 23478) — AuraScript not
--   modeled (standing blocker).

local SPELL_ESSENCEOFTHERED = 23513
local SPELL_FLAMEBREATH = 23461
local SPELL_FIRENOVA = 23462
local SPELL_BURNINGADRENALINE = 18173
local SPELL_CLEAVE = 19983

local SAY_HALFLIFE = 3
local SAY_KILLTARGET = 4

local ENTRY_VAELASTRAZ = 13020

local timers = {}
local yellFired = {}

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

local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", 15000, function() onCleave(creature, guid) end)
end

local function onFlamebreath(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FLAMEBREATH)
    end
    schedule(guid, "flamebreath", 8000, function() onFlamebreath(creature, guid) end)
end

local function onFirenova(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIRENOVA)
    end
    schedule(guid, "firenova", 15000, function() onFirenova(creature, guid) end)
end

local function onAdrenalineTank(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BURNINGADRENALINE)
    end
    schedule(guid, "adrenaline_tank", 45000, function() onAdrenalineTank(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    yellFired[guid] = nil
    creature:CastSpell(creature, SPELL_ESSENCEOFTHERED)
    creature:SetHealth(math.floor(creature:GetMaxHealth() * 0.3))
    schedule(guid, "cleave", 10000, function() onCleave(creature, guid) end)
    schedule(guid, "flamebreath", 15000, function() onFlamebreath(creature, guid) end)
    schedule(guid, "firenova", 20000, function() onFirenova(creature, guid) end)
    schedule(guid, "adrenaline_tank", 45000, function() onAdrenalineTank(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    if yellFired[guid] then
        return
    end
    if creature:GetHealth() - damage < creature:GetMaxHealth() * 0.15 then
        yellFired[guid] = true
        creature:Talk(SAY_HALFLIFE)
    end
end

local function onTargetDied(_, creature)
    if math.random(5) == 1 then
        creature:Talk(SAY_KILLTARGET)
    end
end

RegisterCreatureEvent(ENTRY_VAELASTRAZ, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_VAELASTRAZ, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_VAELASTRAZ, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_VAELASTRAZ, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_VAELASTRAZ, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_VAELASTRAZ, 23, onCombatEnd)
