-- Battle for Mount Hyjal: Rage Winterchill — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/BattleForMountHyjal/
-- boss_rage_winterchill.cpp (171 lines; boss_rage_winterchillAI : public
-- hyjal_trashAI : public EscortAI; AddSC_boss_rage_winterchill at end
-- registers the one script; kalimdor loader decl 30 / call 143 per
-- kalimdor_script_loader.cpp).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "boss_rage_winterchill" (loader lines only otherwise).
-- Entry: hyjal.h HYCreaturesIds names RAGE_WINTERCHILL = 17767 under the
-- "Bosses summoned after every 8 waves" comment AND instance_hyjal.cpp
-- cases it in OnCreatureCreate (RageWinterchill GUID capture, :117) — the
-- name-to-entry tie is C++-verified (ramstein strength); the
-- creature_template ScriptName binding stays DB-side. GetAI uses
-- GetHyjalAI<boss_rage_winterchillAI>, same as jaina/thrall.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; the IsEvent escort
-- machine needs an EscortAI + instance bridge and has none — see below):
-- - Constructor Initialize() arms the four timers at fixed values, so
--   engage arms each timer at its C++ init cooldown; expiry re-arms at the
--   C++ repeat ranges (jeklik convention): Frost Armor 31256 self-cast
--   init 37s -> [40000,59999]; Death and Decay 31258 on victim init 45s ->
--   [60000,79999] + Talk SAY_DECAY (2); Frost Nova 31250 on victim init 15s ->
--   [30000,44999] + Talk SAY_NOVA (3); Icebolt 31249 on a random alive player
--   within 40 (C++ SelectTargetMethod::Random, 0, 40, true —
--   threat-list player pool, ras_frostwhisper precedent) init 10s -> [11000,30999]. All four are
--   DoCast without the triggered flag (non-triggered, jeklik convention);
--   every arm re-arms unconditionally on the firing tick — Icebolt's
--   DoCast(SelectTarget(...)) is null-safe and its timer re-arms even on
--   a nil target (boss_rage_winterchill.cpp UpdateAI); Talk(SAY_*) fires
--   after the cast unconditionally.
-- - Engage (C++ JustEngagedWith): SetBossState(DATA_RAGEWINTERCHILL 10,
--   IN_PROGRESS) before Talk SAY_ONAGGRO (4)
--   (boss_rage_winterchill.cpp:84-88).
-- - KilledUnit: Talk SAY_ONSLAY (1), no target gate in C++ (shade_of_aran
--   precedent) — event 3.
-- - Death (C++ JustDied): SetBossState(DATA_RAGEWINTERCHILL 10, DONE)
--   after hyjal_trashAI::JustDied, before Talk SAY_ONDEATH (0) (:106-111).
-- Unmodeled (documented-only, no bridges):
-- - The IsEvent escort machine: the go-flagged first-UpdateAI block adds
--   8 C++-verbatim waypoints (4896.08,-1576.35,1333.65 /
--   4898.68,-1615.02,1329.48 / 4907.12,-1667.08,1321.00 /
--   4963.18,-1699.35,1340.51 / 4989.16,-1716.67,1335.74 /
--   5026.27,-1736.89,1323.02 / 5037.77,-1770.56,1324.36 /
--   5067.23,-1789.95,1321.17), Start(false, true),
--   SetDespawnAtEnd(false); WaypointReached(7) AddThreat(0) on the
--   instance DATA_JAINAPROUDMOORE GUID — EscortAI movement plus
--   instance-script model, both blocked (standing; the whole hyjal_trashAI
--   escort/wave machine is blocked the same way, see hyjal_trash.lua).
-- - SetBossState legs PORTED this tick (bridge live
--   lua_creature_events.go:5917; the DATA_*EVENT slot 11 is the C++
--   encounter-array id, the DATA id 10 is the Go boss-state key —
--   ruby-sanctum convention): Reset writes NOT_STARTED (:78-81,
--   alive-gated, NO != DONE gate — these AIs are hyjal_trashAI /
--   EscortAI, not BossAI, so no _Reset gate applies; EscortAI::
--   EnterEvadeMode calls Reset(), ScriptedEscortAI.cpp:114, so evade
--   writes NOT_STARTED too, never FAIL — no ev2/ev24 leg). The C++
--   `if (IsEvent)` gate collapses: the entry only ever fights as the
--   event boss (summoned by hyjalAI::SummonCreature with IsEvent=true,
--   hyjalAI.cpp:520-523), same as the combat arms ported unconditionally.
--   The constructor Initialize() timer reset collapses into the engage
--   re-arm (hyjal.lua convention).
-- - hyjal_trashAI::JustDied: instance->SetData(DATA_TRASH, 0) wave signal
--   plus the MINRAIDDAMAGE lootable-flag gate — blocked (standing).

local ENTRY = 17767

local DATA_RAGEWINTERCHILL = 10 -- hyjal.h:38

-- EncounterState (InstanceScript.h:68-76): NOT_STARTED 0, IN_PROGRESS 1, DONE 3.
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

local SAY_ONDEATH = 0
local SAY_ONSLAY  = 1
local SAY_DECAY   = 2
local SAY_NOVA    = 3
local SAY_ONAGGRO = 4

-- C++-verbatim timer table: spell, lo/hi engage-arm range,
-- rlo/rhi repeat range, target "self"/"victim"/"random",
-- say (Talk on cast), randomDist for the random-player arm.
local SPELLS = {
    { spell = 31256, lo = 37000, hi = 37000, rlo = 40000, rhi = 59999,
      target = "self" },
    { spell = 31258, lo = 45000, hi = 45000, rlo = 60000, rhi = 79999,
      target = "victim", say = SAY_DECAY },
    { spell = 31250, lo = 15000, hi = 15000, rlo = 30000, rhi = 44999,
      target = "victim", say = SAY_NOVA },
    { spell = 31249, lo = 10000, hi = 10000, rlo = 11000, rhi = 30999,
      target = "random", randomDist = 40 },
}

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

-- C++ SelectTarget(Random, 0, maxDist, true): random threat-list
-- player within maxDist yards, victim included (UnitAI.h:
-- SelectTarget draws from the threat list; offset 0, no skip;
-- ras_frostwhisper precedent).
local function randomPlayerInRange(creature, maxDist)
    local found = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < maxDist then
            found[#found + 1] = t
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

local function onSpell(creature, guid, idx)
    local spec = SPELLS[idx]
    local target = nil
    if spec.target == "self" then
        target = creature
    elseif spec.target == "victim" then
        target = creature:GetVictim()
    else
        target = randomPlayerInRange(creature, spec.randomDist)
    end
    if target then
        creature:CastSpell(target, spec.spell)
    end
    if spec.say then
        creature:Talk(spec.say)
    end
    schedule(guid, "spell" .. idx,
        math.random(spec.rlo, spec.rhi), function()
            onSpell(creature, guid, idx)
        end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ JustEngagedWith (:84-88): SetData(DATA_RAGEWINTERCHILLEVENT,
    -- IN_PROGRESS) first, then Talk.
    creature:SetBossState(DATA_RAGEWINTERCHILL, BOSS_STATE_IN_PROGRESS)
    creature:Talk(SAY_ONAGGRO)
    for i, spec in ipairs(SPELLS) do
        local id = i
        schedule(guid, "spell" .. i, math.random(spec.lo, spec.hi),
            function()
                onSpell(creature, guid, id)
            end)
    end
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ Reset (:78-81): Initialize() then SetData(NOT_STARTED).
    -- Alive-gated: Go fires ev23 on the death path before ev4
    -- (moroes/algalon precedent) and C++ Reset never runs on death.
    -- NO != DONE gate (see header): this AI is hyjal_trashAI /
    -- EscortAI, not BossAI.
    if creature:IsAlive() then
        creature:SetBossState(DATA_RAGEWINTERCHILL, BOSS_STATE_NOT_STARTED)
    end
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    -- C++ JustDied (:106-111): hyjal_trashAI::JustDied first, then
    -- SetData(DONE), then Talk.
    creature:SetBossState(DATA_RAGEWINTERCHILL, BOSS_STATE_DONE)
    creature:Talk(SAY_ONDEATH)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    creature:Talk(SAY_ONSLAY)
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
