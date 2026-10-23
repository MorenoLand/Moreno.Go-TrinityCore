-- Marwyn (Halls of Reflection) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/boss_marwyn.cpp
-- (178 lines incl. license; 2 scripts — boss_marwyn (CreatureScript via
-- boss_horAI, DATA_MARWYN = 1) + spell_marwyn_shared_suffering
-- (SpellScriptLoader, AuraScript AfterEffectRemove); both registered
-- from inside AddSC_boss_marwyn(); loader decl 169 / call 364 per
-- northrend_script_loader.cpp — the FOURTH and LAST group of the
-- "// Halls of Reflection" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_falric() (call 363); the next call is
-- AddSC_boss_lord_marrowgar() (the "// Icecrown Citadel" block) — the
-- block CLOSES here).
-- Entry: 38113 Marwyn (halls_of_reflection.h NPC_MARWYN, line 72;
-- DATA_MARWYN = 1, halls_of_reflection.h line 38; instance binding is
-- DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_marwyn"
-- hits boss_marwyn.cpp only (+ the loader decl/call lines); this clone
-- carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent ({min,max} table =
-- randtime range); melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady (:125).
--
-- DOCTRINE RETRACTION (2026-10-13): the Oct-13 port of this file treated
-- the whole EventMap machine as unbridgeable under the "no-timer-bridge"
-- doctrine and ported Talk arms only. That doctrine is disproved and
-- stale — CreateLuaEvent is a real registered bridge
-- (engine/scripting/runtime.go:1101), and timer-driven cast arms port on
-- the maiden_of_virtue / moroes / curator / nightbane convention:
-- timers table keyed by creature GUID, cancelled on Eluna events 2
-- (leave combat), 4 (death), 23 (reset); CastSpell(nil, spellId) =
-- victim-cast (bridge falls back to TargetGUID); CastSpell(creature,
-- spellId) = self-cast (the DoCastAOE convention); CastSpell(target,
-- spellId) = explicit-target cast; {min,max} delay = randtime range;
-- TRIGGERED casts pass true as the third CastSpell arg. None of the
-- ported arms here is a TRIGGERED cast (every C++ DoCast is 1-2 arg,
-- non-triggered).
--
-- PORTED ARMS (C++ line refs; all C++-exact for the modeled arms):
-- JustEngagedWith (:65-74) — Talk(SAY_AGGRO 0) (:67); seeds
--   EVENT_OBLITERATE {8000,13000}ms (:71), EVENT_WELL_OF_CORRUPTION
--   12000ms (:72), EVENT_CORRUPTED_FLESH 20000ms (:73),
--   EVENT_SHARED_SUFFERING {14000,15000}ms (:74).
-- EVENT_OBLITERATE (:102-105) — DoCastVictim(SPELL_OBLITERATE 72360)
--   non-triggered; CastSpell(nil, ...) is the victim-cast convention;
--   re-arm {8000,13000}ms (:104).
-- EVENT_WELL_OF_CORRUPTION (:106-110) — SelectTarget(Random, 0, 100.0f,
--   true): random alive player WITHIN 100 yd (creature:IsWithinDist is
--   the bridged distance check — the maiden / kiljaeden precedent);
--   if found, DoCast(target, SPELL_WELL_OF_CORRUPTION 72362)
--   non-triggered; re-arm 13000ms unconditional (:109, outside the
--   target-if).
-- EVENT_CORRUPTED_FLESH (:111-115) — Talk(SAY_CORRUPTED_FLESH 3) (:112)
--   fires UNCONDITIONALLY in C++; DoCastAOE(SPELL_CORRUPTED_FLESH
--   72363) is self-cast non-triggered (:113); re-arm 20000ms (:114).
-- EVENT_SHARED_SUFFERING (:116-120) — SelectTarget(Random, 0, 0.0f,
--   true): random alive player, 0.0f = no distance filter (the
--   keleseth / thorim / garfrost GetPlayersInWorld precedent); if
--   found, DoCast(target, SPELL_SHARED_SUFFERING 72368) non-triggered;
--   re-arm {14000,15000}ms unconditional (:119, outside the target-if).
-- KilledUnit (:84-88) — Talk(SAY_SLAY 1) player-gated (:87), the
--   razuvious player-gated variant precedent.
-- JustDied (:77-82) — Talk(SAY_DEATH 2) (:79) unconditional, the
--   sjonnir JustDied-Talk precedent; events.Reset (:80) = timer
--   cancellation (initState).
--
-- DOCUMENTED-ONLY (no registration beyond entry 38113):
-- Reset (:26-34) — boss_horAI::Reset passthrough (see below).
-- boss_horAI base (boss_horAI.h:27-33 / boss_horAI.cpp) — the
--   ctor/Reset/DoAction/JustSummoned arms are documented-only: the flag
--   twin (creature:SetFlag(59, 0x300)) IS bridged, but Reset's
--   SetImmuneToAll(true) (:30) is the apply half of an
--   instance-sequencer-driven toggle whose clear (DoAction
--   ACTION_ENTER_COMBAT, :41, unported instance wave machine) has no
--   DoAction bridge — porting the apply alone fabricates a
--   permanently-immune boss (the falric header carries the full note,
--   2026-10-23 SetImmuneToAll sweep).
-- JustEngagedWith :68 DoZoneInCombat — PORTED (creature:DoZoneInCombat, event-1 handler, C++ arm order — live bridge engine/world/lua_creature_events.go, 2026-10-23).
-- JustEngagedWith :69 / JustDied :81 instance->SetBossState
--   (DATA_MARWYN, IN_PROGRESS / DONE) — no instance-state bridge.
-- UpdateAI :97 me->HasUnitState(UNIT_STATE_CASTING) gate — no
--   unit-state bridge; timers fire unconditionally (the maiden
--   precedent; creature casts are packet-visual in Go).
-- UpdateAI :125 DoMeleeAttackIfReady — engine-driven in Go, no bridge
--   needed.
-- spell_marwyn_shared_suffering (:141-168, SpellScriptLoader:
--   AfterEffectRemove on EFFECT_0 SPELL_AURA_PERIODIC_DAMAGE
--   (:164) — on AURA_REMOVE_BY_ENEMY_SPELL the remaining
--   damage (amount * remainingTicks) is forwarded as a TRIGGERED cast
--   of SPELL_SHARED_SUFFERING_DISPEL 72373 with
--   SPELLVALUE_BASE_POINT1 = remainingDamage (:150-159)) — no
--   AuraScript bridge (the boss_moragg optic-link precedent), stays in
--   the standing queue.
--
-- DEVIATIONS FROM C++ (documented; every other value C++-exact):
-- Random-target helper: GetPlayersInWorld alive-player scan on the same
--   map/instance (the keleseth / thorim precedent); C++ applies no
--   threat-order weighting and playerOnly = true, so the pool is every
--   alive player — no approximation beyond the C++ select rules.
-- No-qualifier corner: with zero qualifying players the C++ target-if
--   skips Talk + cast but still re-arms (:109/:119 outside the if) —
--   replicated: re-arm runs even when no target qualifies.
-- UNIT_STATE_CASTING gate (see documented-only): timers fire
--   unconditionally like the maiden precedent; two arms firing on the
--   same tick both cast, where C++ would serialize them.

local ENTRY_MARWYN = 38113

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2
local SAY_CORRUPTED_FLESH = 3

local SPELL_OBLITERATE = 72360
local SPELL_WELL_OF_CORRUPTION = 72362
local SPELL_CORRUPTED_FLESH = 72363
local SPELL_SHARED_SUFFERING = 72368

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

-- bronjahm-style: re-scheduling a key replaces the pending timer, so each
-- named arm has exactly one live timer at a time.
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

local function initState(guid)
    cancelTimers(guid)
end

-- C++ SelectTarget(SelectTargetMethod::Random, 0, 0.0f, true) — random
-- alive player on the same map/instance; no distance filter (0.0f) —
-- the keleseth / thorim / garfrost GetPlayersInWorld precedent.
local function randomPlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- Same pool with a distance filter (creature:IsWithinDist is the
-- bridged distance check — the maiden / kiljaeden precedent); used for
-- the 100.0f WELL_OF_CORRUPTION target.
local function randomPlayerInRange(creature, dist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:IsWithinDist(p, dist) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ :102-105 EVENT_OBLITERATE: DoCastVictim(72360) non-triggered;
-- re-arm {8000,13000}ms.
local function onObliterate(creature, guid)
    creature:CastSpell(nil, SPELL_OBLITERATE)
    schedule(guid, "obliterate", {8000, 13000}, function() onObliterate(creature, guid) end)
end

-- C++ :106-110 EVENT_WELL_OF_CORRUPTION: random alive player within 100
-- yd (:107 SelectTarget Random, 0, 100.0f, true); DoCast(target, 72362)
-- non-triggered; re-arm 13000ms unconditional (:109, outside the target-if).
local function onWellOfCorruption(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_WELL_OF_CORRUPTION)
    end
    schedule(guid, "well", 13000, function() onWellOfCorruption(creature, guid) end)
end

-- C++ :111-115 EVENT_CORRUPTED_FLESH: Talk(SAY_CORRUPTED_FLESH)
-- unconditional (:112); DoCastAOE(72363) is self-cast non-triggered
-- (the DoCastAOE convention); re-arm 20000ms (:114).
local function onCorruptedFlesh(creature, guid)
    creature:Talk(SAY_CORRUPTED_FLESH)
    creature:CastSpell(creature, SPELL_CORRUPTED_FLESH)
    schedule(guid, "corruptedflesh", 20000, function() onCorruptedFlesh(creature, guid) end)
end

-- C++ :116-120 EVENT_SHARED_SUFFERING: random alive player, no distance
-- filter (:117 SelectTarget Random, 0, 0.0f, true); DoCast(target,
-- 72368) non-triggered; re-arm {14000,15000}ms unconditional (:119,
-- outside the target-if).
local function onSharedSuffering(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_SHARED_SUFFERING)
    end
    schedule(guid, "sharedsuffering", {14000, 15000}, function() onSharedSuffering(creature, guid) end)
end

-- C++ :65-74 JustEngagedWith: Talk(SAY_AGGRO) (:67); DoZoneInCombat
-- (:68) PORTED below (live bridge engine/world/lua_creature_events.go,
-- 2026-10-23); seeds the four timers (:71-74).
-- instance->SetBossState IN_PROGRESS (:69) has no bridge (see header).
local function marwynJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    initState(guid)
    creature:Talk(SAY_AGGRO)
    creature:DoZoneInCombat() -- C++ :68
    schedule(guid, "obliterate", {8000, 13000}, function() onObliterate(creature, guid) end)
    schedule(guid, "well", 12000, function() onWellOfCorruption(creature, guid) end)
    schedule(guid, "corruptedflesh", 20000, function() onCorruptedFlesh(creature, guid) end)
    schedule(guid, "sharedsuffering", {14000, 15000}, function() onSharedSuffering(creature, guid) end)
end

-- C++ :84-88 KilledUnit: if (who->GetTypeId() != TYPEID_PLAYER)
-- return; Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function marwynKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ :77-82 JustDied: Talk(SAY_DEATH) (:79) — the sjonnir
-- JustDied-Talk precedent; events.Reset (:80) = timer cancellation;
-- instance->SetBossState DONE (:81) has no bridge (see header).
local function marwynJustDied(event, creature, killer)
    initState(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function marwynLeaveCombat(event, creature)
    initState(creature:GetGUID())
end

local function marwynReset(event, creature)
    initState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_MARWYN, 1, marwynJustEngagedWith)
RegisterCreatureEvent(ENTRY_MARWYN, 2, marwynLeaveCombat)
RegisterCreatureEvent(ENTRY_MARWYN, 3, marwynKilledUnit)
RegisterCreatureEvent(ENTRY_MARWYN, 4, marwynJustDied)
RegisterCreatureEvent(ENTRY_MARWYN, 23, marwynReset)
