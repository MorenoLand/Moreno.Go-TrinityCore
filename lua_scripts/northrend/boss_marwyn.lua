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
-- JustEngagedWith (:65-74) — Talk(SAY_AGGRO 0) (:67); DoZoneInCombat
--   (:68) PORTED; instance->SetBossState (DATA_MARWYN 1, IN_PROGRESS)
--   (:69) PORTED 2026-11-05 via creature:SetBossState
--   (lua_creature_events.go:5917, the garfrost precedent) — in C++
--   arm order, after Talk + DoZoneInCombat and before the timer
--   seeds; seeds EVENT_OBLITERATE {8000,13000}ms (:71),
--   EVENT_WELL_OF_CORRUPTION 12000ms (:72), EVENT_CORRUPTED_FLESH
--   20000ms (:73), EVENT_SHARED_SUFFERING {14000,15000}ms (:74).
--   The old "no instance-state bridge" claim for :69 is retracted.
-- EVENT_OBLITERATE (:102-105) — DoCastVictim(SPELL_OBLITERATE 72360)
--   non-triggered; CastSpell(nil, ...) is the victim-cast convention;
--   re-arm {8000,13000}ms (:104).
-- EVENT_WELL_OF_CORRUPTION (:106-110) — SelectTarget(Random, 0, 100.0f,
--   true): random alive threat-list player WITHIN 100 yd
--   (creature:IsWithinDist is the bridged distance check — the maiden /
--   kiljaeden precedent); if found, DoCast(target,
--   SPELL_WELL_OF_CORRUPTION 72362) non-triggered; re-arm 13000ms
--   unconditional (:109, outside the target-if).
-- EVENT_CORRUPTED_FLESH (:111-115) — Talk(SAY_CORRUPTED_FLESH 3) (:112)
--   fires UNCONDITIONALLY in C++; DoCastAOE(SPELL_CORRUPTED_FLESH
--   72363) is self-cast non-triggered (:113); re-arm 20000ms (:114).
-- EVENT_SHARED_SUFFERING (:116-120) — SelectTarget(Random, 0, 0.0f,
--   true): random alive threat-list player, 0.0f = no distance filter;
--   if found, DoCast(target, SPELL_SHARED_SUFFERING 72368)
--   non-triggered; re-arm {14000,15000}ms unconditional (:119, outside
--   the target-if).
-- DoAction (boss_horAI.cpp:36-48) — ACTION_ENTER_COMBAT (-668001,
--   halls_of_reflection.h:130) PORTED 2026-11-08 (tick 491) via the
--   ev38 receiver: SetImmuneToAll(false), REACT_AGGRESSIVE,
--   DoZoneInCombat. The sender is the instance wave machine (ADD_WAVE
--   at wave 5/10); the apply half lives in marwynReset below, so the
--   toggle pair is complete (the 2026-10-23 SetImmuneToAll sweep bar
--   is met — the old "unported instance wave machine" note is stale).
-- KilledUnit (:84-88) — Talk(SAY_SLAY 1) player-gated (:87), the
--   razuvious player-gated variant precedent.
-- JustDied (:77-82) — Talk(SAY_DEATH 2) (:79) unconditional, the
--   sjonnir JustDied-Talk precedent; events.Reset (:80) = timer
--   cancellation; instance->SetBossState (DATA_MARWYN 1, DONE) (:81)
--   PORTED 2026-11-05 via creature:SetBossState, in C++ order AFTER
--   Talk (unlike garfrost, whose _JustDied runs first); the
--   instance SetBossState-override DONE legs
--   (instance_halls_of_reflection.cpp:349-371) that are bridgeable
--   are PORTED below in C++ order, right after the DONE write (the
--   garfrost tyrannus-Talk precedent): bunny self-cast 72900, the
--   Koreln/Loralen questgiver flag, DoUpdateWorldState(4884, 0), the
--   Frostsworn General phase-mask. The old "no instance-state
--   bridge" claim for :81 is retracted.
-- Reset (:26-34 + boss_horAI.cpp:26-34) — _Reset() first:
--   SetBossState(DATA_MARWYN 1, NOT_STARTED) only when the stored
--   state != DONE (the bronjahm / cyanigosa precedent: ev23 fires on
--   spawn too, and a DONE boss must stay DONE), PORTED 2026-11-05
--   via creature:SetBossState in the ev23 leg (split from ev2 —
--   leave-combat does not call Reset in C++). The SetVisible(false) /
--   SetImmuneToAll(true) / REACT_PASSIVE legs (:30-32) are PORTED
--   2026-11-08 (tick 491) — all three bridges are live
--   (lua_creature_events.go:5548/:5966/:4229) and the clear half
--   (DoAction ACTION_ENTER_COMBAT, see above) is ported, so the
--   toggle pair is complete. The GetData(DATA_WAVE_COUNT) /
--   ProcessEvent(EVENT_DO_WIPE) leg (:33-34) is PORTED 2026-11-08
--   (tick 491) via instanceData:SetData(DATA_WAVE_COUNT, 0) when the
--   stored wave count is non-zero (the instance event-8 handler
--   runs the wipe).
-- DOCUMENTED-ONLY (no registration beyond entry 38113):
-- boss_horAI base (boss_horAI.h:27-33 / boss_horAI.cpp) — the
--   ctor/Reset/DoAction arms are PORTED (see the PORTED ARMS notes
--   above, tick 491); the toggle pair is complete because the clear
--   (DoAction ACTION_ENTER_COMBAT, :41, driven by the ported
--   instance wave machine) is ported too. The old "both halves stay
--   out" note (2026-10-23 SetImmuneToAll sweep) is retracted.
--   JustSummoned (boss_horAI.cpp:50-53, summons.Summon) has no Lua
--   twin — neither boss reads its summon list, so nothing to port.
-- JustEngagedWith :68 DoZoneInCombat — PORTED (creature:DoZoneInCombat, event-1 handler, C++ arm order — live bridge engine/world/lua_creature_events.go, 2026-10-23).
-- JustEngagedWith :69 / JustDied :81 instance->SetBossState
--   (DATA_MARWYN, IN_PROGRESS / DONE) — PORTED 2026-11-05 via
--   creature:SetBossState (see the PORTED ARMS note above); the old
--   "no instance-state bridge" claim is retracted.
-- Instance SetBossState-override DONE legs that stay out
--   (instance_halls_of_reflection.cpp:349-371): the HandleGameObject
--   door legs (no door model in the Go boss-state store),
--   SpawnGunship() (CreateTransport, no bridge) and SpawnEscapeEvent()
--   (instance SummonCreature + team latch, no bridge).
-- UpdateAI :97 me->HasUnitState(UNIT_STATE_CASTING) gate — PORTED: the
--   gate sits before ExecuteEvent (:100), so all four timer ticks
--   defer 500ms via creature:IsCasting() (lua_creature_events.go:1950)
--   without consuming their re-arm (the mal_ganis idiom); Talk hooks
--   stay ungated per C++ as separate hooks. The old "no unit-state
--   bridge; timers fire unconditionally" claim is retracted.
-- UpdateAI :125 DoMeleeAttackIfReady — engine-driven in Go, no bridge
--   needed.
-- spell_marwyn_shared_suffering (:141-168, SpellScriptLoader:
--   AfterEffectRemove on EFFECT_0 SPELL_AURA_PERIODIC_DAMAGE
--   (:164) — on AURA_REMOVE_BY_ENEMY_SPELL the remaining
--   damage (amount * remainingTicks) is forwarded as a TRIGGERED cast
--   of SPELL_SHARED_SUFFERING_DISPEL 72373 with
--   SPELLVALUE_BASE_POINT1 = remainingDamage (:150-159)) — PORTED
--   below (RegisterAuraEvent(72368/72369, 3), CastCustomSpell bp1 =
--   remainingDamage; the "no spell-mod bridge" claim is stale —
--   creature CastCustomSpell is live, lua_creature_events.go:977,
--   leeching-swarm precedent; GetRemainingTicks is
--   GetTotalTicks - GetTickNumber, SpellAuraEffects.h:79).
--   DORMANT until Spell.dbc ships 72368/72369/72373.
--
-- DEVIATIONS FROM C++ (documented; every other value C++-exact):
-- Random-target helper: threat-list players via GetAITargets
--   (SelectTarget draws the threat list, UnitAI.h); the old
--   GetPlayersInWorld pool could pick instance players outside combat.
-- No-qualifier corner: with zero qualifying players the C++ target-if
--   skips Talk + cast but still re-arms (:109/:119 outside the if) —
--   replicated: re-arm runs even when no target qualifies.

local ENTRY_MARWYN = 38113
local DATA_MARWYN = 1 -- halls_of_reflection.h:38
local DATA_WAVE_COUNT = 6 -- halls_of_reflection.h:45
local BOSS_STATE_DONE = 3 -- InstanceScript.h:68-76

local ACTION_ENTER_COMBAT = -668001 -- halls_of_reflection.h:130

local REACT_PASSIVE = 0 -- UnitDefines.h:340
local REACT_AGGRESSIVE = 2 -- UnitDefines.h:342

local SPELL_START_HALLS_OF_REFLECTION_QUEST_AE = 72900 -- halls_of_reflection.h:158
local SPELL_START_HALLS_OF_REFLECTION_QUEST_A = 71351 -- halls_of_reflection.h:156
local SPELL_START_HALLS_OF_REFLECTION_QUEST_H = 71542 -- halls_of_reflection.h:157
local WORLD_STATE_HOR_WAVES_ENABLED = 4884 -- halls_of_reflection.h:170
local UNIT_NPC_FLAGS = 82 -- UpdateFields.h:139 (baltharus precedent)
local UNIT_NPC_FLAG_QUESTGIVER = 0x00000002 -- UnitDefines.h:187

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2
local SAY_CORRUPTED_FLESH = 3

local SPELL_OBLITERATE = 72360
local SPELL_WELL_OF_CORRUPTION = 72362
local SPELL_CORRUPTED_FLESH = 72363
local SPELL_SHARED_SUFFERING = 72368
local SPELL_SHARED_SUFFERING_HEROIC = 72369 -- boss_marwyn.cpp:135, same loader
local SPELL_SHARED_SUFFERING_DISPEL = 72373 -- boss_marwyn.cpp:39

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
-- alive threat-list player; no distance filter (0.0f) — SelectTarget
-- draws the threat list (UnitAI.h), not the instance player list.
local function randomPlayer(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- Same threat-list pool with a distance filter (IsWithinCombatRange
-- twin — the maiden / kiljaeden precedent, now reach-exact);
-- used for the 100.0f WELL_OF_CORRUPTION target.
local function randomPlayerInRange(creature, dist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < dist then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            candidates[#candidates + 1] = t
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
    -- C++ UpdateAI :95/:97: pre-ExecuteEvent CASTING return — 500ms
    -- IsCasting defer, no re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "obliterate", 500, function() onObliterate(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_OBLITERATE)
    schedule(guid, "obliterate", {8000, 13000}, function() onObliterate(creature, guid) end)
end

-- C++ :106-110 EVENT_WELL_OF_CORRUPTION: random alive player within 100
-- yd (:107 SelectTarget Random, 0, 100.0f, true); DoCast(target, 72362)
-- non-triggered; re-arm 13000ms unconditional (:109, outside the target-if).
local function onWellOfCorruption(creature, guid)
    -- C++ UpdateAI :95/:97: pre-ExecuteEvent CASTING return — 500ms
    -- IsCasting defer precedes the target pick, no re-arm consumed
    -- (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "well", 500, function() onWellOfCorruption(creature, guid) end)
        return
    end
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
    -- C++ UpdateAI :95/:97: pre-ExecuteEvent CASTING return — 500ms
    -- IsCasting defer precedes Talk + cast, no re-arm consumed
    -- (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "corruptedflesh", 500, function() onCorruptedFlesh(creature, guid) end)
        return
    end
    creature:Talk(SAY_CORRUPTED_FLESH)
    creature:CastSpell(creature, SPELL_CORRUPTED_FLESH)
    schedule(guid, "corruptedflesh", 20000, function() onCorruptedFlesh(creature, guid) end)
end

-- C++ :116-120 EVENT_SHARED_SUFFERING: random alive player, no distance
-- filter (:117 SelectTarget Random, 0, 0.0f, true); DoCast(target,
-- 72368) non-triggered; re-arm {14000,15000}ms unconditional (:119,
-- outside the target-if).
local function onSharedSuffering(creature, guid)
    -- C++ UpdateAI :95/:97: pre-ExecuteEvent CASTING return — 500ms
    -- IsCasting defer precedes the target pick, no re-arm consumed
    -- (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "sharedsuffering", 500, function() onSharedSuffering(creature, guid) end)
        return
    end
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_SHARED_SUFFERING)
    end
    schedule(guid, "sharedsuffering", {14000, 15000}, function() onSharedSuffering(creature, guid) end)
end

-- C++ :65-74 JustEngagedWith: Talk(SAY_AGGRO) (:67); DoZoneInCombat
-- (:68) PORTED below (live bridge engine/world/lua_creature_events.go,
-- 2026-10-23); instance->SetBossState IN_PROGRESS (:69) PORTED
-- 2026-11-05 via creature:SetBossState, in C++ arm order after
-- DoZoneInCombat and before the seeds (garfrost precedent).
local function marwynJustEngagedWith(event, creature, target)
    local guid = creature:GetGUIDLow()
    initState(guid)
    creature:Talk(SAY_AGGRO)
    creature:DoZoneInCombat() -- C++ :68
    creature:SetBossState(DATA_MARWYN, 1) -- C++ :69 IN_PROGRESS
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
-- instance->SetBossState DONE (:81) PORTED 2026-11-05 via
-- creature:SetBossState, in C++ order AFTER Talk. The bridgeable
-- instance SetBossState-override DONE legs
-- (instance_halls_of_reflection.cpp:349-371) run right after the DONE
-- write, in C++ order (the garfrost tyrannus-Talk precedent):
-- Frostmourne Altar Bunny self-cast 72900 (:352-354), Koreln/Loralen
-- questgiver flag (:356-358), DoUpdateWorldState(4884, 0) (:362),
-- Frostsworn General SetPhaseMask(1) (:363-364); the door /
-- SpawnGunship / SpawnEscapeEvent legs stay out (see header).
-- All cross-creature reads go through the instanceData mirror
-- (instance_halls_of_reflection.lua binds) and are nil-guarded like
-- the C++ GetCreature checks.
local function marwynJustDied(event, creature, killer)
    initState(creature:GetGUIDLow())
    creature:Talk(SAY_DEATH)
    creature:SetBossState(DATA_MARWYN, BOSS_STATE_DONE)
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            if data.frostmourneAltarBunny then
                data.frostmourneAltarBunny:CastSpell(data.frostmourneAltarBunny, SPELL_START_HALLS_OF_REFLECTION_QUEST_AE, true)
            end
            if data.korelnOrLoralen then
                data.korelnOrLoralen:SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_QUESTGIVER)
            end
            creature:UpdateWorldState(WORLD_STATE_HOR_WAVES_ENABLED, 0)
            if data.frostswornGeneral then
                data.frostswornGeneral:SetPhaseMask(1, true)
            end
        end
    end
end

-- C++ boss_horAI::DoAction (boss_horAI.cpp:36-48): ACTION_ENTER_COMBAT
-- — SetImmuneToAll(false), REACT_AGGRESSIVE, DoZoneInCombat. The
-- sender is the instance wave machine (ADD_WAVE at wave 5/10); the
-- apply half lives in marwynReset below, so the toggle pair is
-- complete (tick 491).
local function marwynDoAction(event, creature, action)
    if action ~= ACTION_ENTER_COMBAT then
        return
    end
    creature:SetImmuneToAll(false)
    creature:SetReactState(REACT_AGGRESSIVE)
    creature:DoZoneInCombat()
end

local function marwynLeaveCombat(event, creature)
    initState(creature:GetGUIDLow())
end

-- C++ Reset (:26-34 + boss_horAI.cpp:26-34): _Reset() first —
-- SetBossState(DATA_MARWYN 1, NOT_STARTED) only when the stored state
-- != DONE (the bronjahm / cyanigosa precedent: ev23 fires on spawn
-- too, and a DONE boss must stay DONE). ev23 is split from ev2
-- (leave-combat does not call Reset in C++).
local function marwynReset(event, creature)
    if creature:GetBossState(DATA_MARWYN) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_MARWYN, 0)
    end
    -- C++ boss_horAI.cpp:30-32: SetVisible(false), SetImmuneToAll(true),
    -- REACT_PASSIVE (tick 491; the clear half is marwynDoAction above).
    creature:SetVisible(false)
    creature:SetImmuneToAll(true)
    creature:SetReactState(REACT_PASSIVE)
    -- C++ boss_horAI.cpp:33-34: GetData(DATA_WAVE_COUNT) != NOT_STARTED
    -- -> ProcessEvent(EVENT_DO_WIPE); the instance event-8 handler runs
    -- the wipe when it receives DATA_WAVE_COUNT NOT_STARTED with a
    -- non-zero count (instance_halls_of_reflection.lua).
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data ~= nil and (data.waveCount or 0) ~= 0 then
            data:SetData(DATA_WAVE_COUNT, 0)
        end
    end
    initState(creature:GetGUIDLow())
end

-- C++ spell_hor_start_halls_of_reflection_quest_ae (72900,
-- halls_of_reflection.cpp:2753-2784): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT — the hit player casts the faction quest
-- starter (71351 Alliance / 71542 Horde) on self, triggered. The
-- caster is the frostmourne altar bunny (creature path, so the ev5
-- hook fires). DORMANT until Spell.dbc ships 72900.
RegisterSpellEvent(SPELL_START_HALLS_OF_REFLECTION_QUEST_AE, 5, function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if not hitTarget or hitTarget:GetObjectType() ~= "Player" then
        return
    end
    if hitTarget:GetTeam() == 0 then
        hitTarget:CastSpell(hitTarget, SPELL_START_HALLS_OF_REFLECTION_QUEST_A, true)
    else
        hitTarget:CastSpell(hitTarget, SPELL_START_HALLS_OF_REFLECTION_QUEST_H, true)
    end
end)

RegisterCreatureEvent(ENTRY_MARWYN, 1, marwynJustEngagedWith)
RegisterCreatureEvent(ENTRY_MARWYN, 2, marwynLeaveCombat)
RegisterCreatureEvent(ENTRY_MARWYN, 3, marwynKilledUnit)
RegisterCreatureEvent(ENTRY_MARWYN, 4, marwynJustDied)
RegisterCreatureEvent(ENTRY_MARWYN, 23, marwynReset)
RegisterCreatureEvent(ENTRY_MARWYN, 38, marwynDoAction)

-- C++ spell_marwyn_shared_suffering (boss_marwyn.cpp:141-168):
-- AuraScript AfterEffectRemove EFFECT_0 SPELL_AURA_PERIODIC_DAMAGE,
-- registered for 72368 + 72369 (boss_marwyn.cpp:135). On remove by an
-- enemy spell only (GetRemoveMode() == 3, AURA_REMOVE_BY_ENEMY_SPELL),
-- the caster forwards the remaining damage (amount * remainingTicks,
-- SpellAuraEffects.h:79: GetRemainingTicks = GetTotalTicks - _ticksDone)
-- as a TRIGGERED cast of SPELL_SHARED_SUFFERING_DISPEL 72373 on the
-- target with SPELLVALUE_BASE_POINT1 = remainingDamage. CastCustomSpell
-- bp1 is the BASE_POINT1 slot (sparse map, nil bp0 leaves effect 0 at
-- its DBC value; leeching-swarm precedent). DORMANT until Spell.dbc
-- ships 72368/72369/72373.
local AURA_REMOVE_BY_ENEMY_SPELL = 3 -- SpellAuraDefines.h:61-69

local function sharedSufferingRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_ENEMY_SPELL then
        return
    end
    local caster = aura:GetCaster()
    if caster == nil or target == nil then
        return
    end
    local remainingDamage = aura:GetAmount() * (aura:GetTotalTicks() - aura:GetTickNumber())
    if remainingDamage > 0 then
        caster:CastCustomSpell(target, SPELL_SHARED_SUFFERING_DISPEL, true, nil, remainingDamage)
    end
end
RegisterAuraEvent(SPELL_SHARED_SUFFERING, 3, sharedSufferingRemove)
RegisterAuraEvent(SPELL_SHARED_SUFFERING_HEROIC, 3, sharedSufferingRemove)
