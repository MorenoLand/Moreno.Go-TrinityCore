-- Blood-Queen Lana'thel (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_blood_queen_lana_thel.cpp
-- (843 lines incl. license; 1 CreatureScript
-- (boss_blood_queen_lana_thel (BossAI, DATA_BLOOD_QUEEN_LANA_THEL = 8))
-- + 5 SpellScripts (spell_blood_queen_vampiric_bite,
-- spell_blood_queen_bloodbolt, spell_blood_queen_pact_of_the_darkfallen,
-- spell_blood_queen_pact_of_the_darkfallen_dmg_target,
-- spell_blood_queen_twilight_bloodbolt) + 3 AuraScripts
-- (spell_blood_queen_frenzied_bloodthirst,
-- spell_blood_queen_essence_of_the_blood_queen,
-- spell_blood_queen_pact_of_the_darkfallen_dmg) + 2
-- AchievementCriteriaScripts (achievement_once_bitten_twice_shy_n,
-- achievement_once_bitten_twice_shy_v); all registered from inside
-- AddSC_boss_blood_queen_lana_thel(); loader decl 179 / call 374 per
-- northrend_script_loader.cpp — the NINTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_blood_prince_council() (call 373); the call after
-- it is AddSC_boss_sister_svalna() (call 375) — verified from the
-- loader this run; the checkpoint sequence (blood_prince_council ->
-- blood_queen_lana_thel) is followed).
-- Entry: 37955 Blood-Queen Lana'thel (icecrown_citadel.h
-- NPC_BLOOD_QUEEN_LANA_THEL, line 266; icecrown_citadel.h
-- DATA_BLOOD_QUEEN_LANA_THEL = 8, line 82;
-- instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_BLOOD_QUEEN_LANA_THEL, line 270) — entry-verifiable,
-- registration proceeds (the nexus_commanders kalecgos precedent);
-- the CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_blood_queen_lana_thel" hits
-- boss_blood_queen_lana_thel.cpp (+ the loader decl/call lines)
-- only; this clone carries no sql/ tree, so ScriptName bindings
-- are DB-side by construction. No lana'thel lua existed.
-- (The npc_blood_queen_lana_thel PassiveAI referenced by the
-- council file's intro legs lives in boss_blood_prince_council.cpp
-- and was documented there, not here.)
--
-- DOCUMENTED-BLOCKED (no registration beyond entry 37955):
-- boss_blood_queen_lana_thel JustEngagedWith — instance legs
-- (instance->CheckRequiredBosses / DoCastSpellOnPlayers(
-- LIGHT_S_HAMMER_TELEPORT)) have no instance bridge; SetBossState(
-- DATA_BLOOD_QUEEN_LANA_THEL, IN_PROGRESS) PORTED 2026-11-06 via
-- creature:SetBossState (lua_creature_events.go:5917). PORTED
-- (2026-10-23): DoZoneInCombat (:184,
-- me-form) via the live creature:DoZoneInCombat bridge
-- (engine/world/lua_creature_events.go:426; the mandokir/algalon
-- event-1 precedent) — C++ arm order (before Talk(SAY_AGGRO));
-- PORTED (2026-10-13): the three triggered
-- self-cast auras (:188-190) — curator engage-cast precedent.
-- boss_blood_queen_lana_thel JustDied — DoCastAOE(
-- SPELL_BLOOD_INFUSION_CREDIT, true) leg PORTED (Is25ManRaid() +
-- HasAura(SPELL_SHADOWS_FATE) gate, creature:Is25ManRaid() bridged
-- lua_creature_events.go; the old "no 25-man bridge" claim retracted);
-- instance->SetData(DATA_BLOOD_QUICKENING_STATE, DONE) /
-- RewardPlayerAndGroupAtEvent / FindNearestCreature minchar legs
-- have no instance bridge. PORTED (2026-10-23): CleanAuras()
-- (:221-233) — the ten DoRemoveAurasDueToSpellOnPlayers strips —
-- via GetPlayersInWorld + mapId/instanceId filter +
-- player:RemoveAura (the moroes precedent; the stale "no
-- instance-aura bridge" claim is retracted). The :716
-- spell_blood_queen_pact_of_the_darkfallen FilterTargets pact strip
-- rides the SpellScript bridge gap (see below) and stays out.
-- boss_blood_queen_lana_thel JustReachedHome — the Talk(SAY_WIPE 9) leg
-- SetDisableGravity(false) / SetReactState(2) / Talk(SAY_WIPE 9) ported
-- on event 24 in C++ arm order (bridges live, lua_creature_events.go);
-- the instance->SetBossState(FAIL) leg (:275) PORTED 2026-11-06 via
-- creature:SetBossState (the old "no instance bridge" note was stale).
-- boss_blood_queen_lana_thel MovementInform — Talk(SAY_AIR_PHASE
-- 7) rides the POINT_AIR movement-inform leg (no MovementInform
-- bridge); the air-phase machine (MovePoint / SetDisableGravity /
-- hover legs, plus a SetReactState leg that is bridged in isolation)
-- rides the unported movement machine.
-- PORTED (2026-10-13 retraction re-audit): EVENT_BERSERK —
-- single-fire 330s — Talk(EMOTE_BERSERK_RAID 12) +
-- Talk(SAY_BERSERK 10) + DoCastSelf(SPELL_BERSERK 26662)
-- non-triggered (:160/:351-354); the "no-timer-bridge" block for
-- this arm was stale doctrine (the boss_toravon precedent).
-- boss_blood_queen_lana_thel UpdateAI — 2026-10-14 retraction
-- re-audit: the stale "no-timer-bridge" claim for the remaining
-- scheduler arms is RETRACTED (CreateLuaEvent is a real bridge,
-- engine/scripting/runtime.go:798); each arm is classified against
-- the C++ individually (gothik precedent). Talk(SAY_VAMPIRIC_BITE 1)
-- rides EVENT_VAMPIRIC_BITE (:356-368) — PORTED (2026-10-25, seeded
-- once 15s in Reset, NO re-arm): front pick of the threat-sorted
-- GetAITargets pool minus the victim (GetAITargets IS the threat-list
-- bridge, anomalus precedent; offtank exclusion unmodeled, Blood
-- Mirror unported) + DoCast(target, 71726) + DoCastAOE dummy 71837
-- triggered (self-cast twin) + Talk(1) in C++ order + target self-casts
-- PRESENCE 70994 + PRESENCE_2 71952 triggered (player:CastSpell
-- bridged, freya/vezax precedent). The SpellScript IsVampire
-- CheckTarget fizzle leg has no bridge (a re-bite applies where C++
-- would fizzle the bite cast only; Talk/presence legs still fire in
-- C++ on that path). The _vampires set (GuidSet, :508) is PORTED
-- tick 417 as per-player "vampire_<guidLow>" keys in the shared
-- instanceData table (see recordVampire / clearVampireMarks below)
-- for the achievement_once_bitten_twice_shy_n/_v gates
-- (WasVampire, :466-469, feeds those scripts at :793-826) — the old
-- "no ported consumer" note was wrong: the achievement scripts ARE
-- consumers. The player-cast-bite SetGUID feed (:561,
-- spell_blood_queen_vampiric_bite::OnCast, caster must be a player)
-- has no Go path — players cannot cast 71726 in Go (the Frenzied
-- Bloodthirst bite-or-die chain is unported) — so only boss-cast
-- bites are recorded.
-- Talk(SAY_PACT_OF_THE_DARKFALLEN 6) rides
-- EVENT_PACT_OF_THE_DARKFALLEN (:407-420) — PORTED (2026-10-25, seed
-- 15s / re-arm 30500ms unconditional): RandomResize(2/3 by
-- Is25ManRaid) over the threat-sorted GetAITargets pool minus the
-- victim (offtank exclusion unmodeled); Talk(6) + DoCast(71340) each
-- only if picked > 1, C++ order.
-- PORTED this tick: EVENT_SWARMING_SHADOWS (:421-428) —
-- Talk(EMOTE_SWARMING_SHADOWS 5, target) + Talk(SAY_SWARMING_SHADOWS 4)
-- + DoCast(target, SPELL_SWARMING_SHADOWS 71264) (2-arg =
-- non-triggered, :426) in C++ order; 30500ms seed (:166) /
-- unconditional 30500ms re-arm (:427). Targeted Talk is bridged
-- (creature:Talk(id, target); emote rows broadcast, only whisper
-- rows go target-only — creature_text.go), so the old "no
-- targeted-Talk bridge" drop is retracted. Pool (2026-10-25):
-- threat-sorted GetAITargets minus the victim (C++ :478-507 threat
-- list minus maintank minus offtank; offtank exclusion still
-- unmodeled).
-- PORTED (2026-10-25): EVENT_TWILIGHT_BLOODBOLT (:430-438, seed
-- {20,25}s / re-arm {10,15}s) — RandomResize(2/4 by Is25ManRaid)
-- casts of 71446 + self-cast 71445 (TARGET), threat pool minus
-- victim. The UpdateAI case has NO air-phase gate (NORMAL group,
-- only delayed 10s at air phase) — the header's Fly-gate note is
-- retracted; the TARGET spell's SpellScript filter leg stays out
-- (no SpellScript bridge). OUT: Blood Mirror (CANCELLABLE group —
-- cancelled at air phase, and the port has no air-phase cancel
-- signal — plus the quest-status GUSHING_WOUND leg, no quest
-- bridge) and Delirious Slash (gated on Blood Mirror's _offtankGUID,
-- never set in this port, + Fly tier).
-- boss_blood_queen_lana_thel DoAction — ACTION_KILL_MINCHAR leg
-- (ev38 dispatch bridged; sender instance EVENT_UPDATE_EXECUTION_TIME (instance_icecrown_citadel.cpp:1387) unported — instance event machine; krick relay N/A); the
-- EnterEvadeMode / SetGUID vampire-bloodbolt GUID-set bookkeeping
-- rides GUID / cross-AI bridges that do not exist; the quest-credit
-- legs ride the absent quest bridge.
-- spell_blood_queen_frenzied_bloodthirst (AuraScript) —
-- bloodQueen->AI()->Talk(EMOTE_BLOODTHIRST 3, GetTarget()) OnApply
-- and bloodQueen->AI()->Talk(SAY_MIND_CONTROL 2) OnRemove are
-- cross-AI Talk (no cross-AI-Talk bridge); the
-- spell_blood_queen_pact_of_the_darkfallen_dmg PeriodicTick is
-- PORTED (tick 193, RegisterAuraEvent — see below); the remaining
-- AuraScript (essence_of_the_blood_queen) + 5 SpellScript legs join
-- the no-AuraScript-bridge / no-SpellScript-bridge queues
-- (the boss_moragg optic-link precedent). The 2
-- AchievementCriteriaScripts are PORTED tick 417 (see the
-- _vampires note above).
-- All 13 Talk() calls in the file accounted for (13 =
-- SAY_AGGRO 0 (1) + SAY_VAMPIRIC_BITE 1 (1, scheduler) +
-- SAY_MIND_CONTROL 2 (1, cross-AI aura leg) +
-- EMOTE_BLOODTHIRST 3 (1, cross-AI aura leg) +
-- SAY_SWARMING_SHADOWS 4 (1, scheduler) + EMOTE_SWARMING_SHADOWS
-- 5 (1, scheduler) + SAY_PACT_OF_THE_DARKFALLEN 6 (1, scheduler)
-- + SAY_AIR_PHASE 7 (1, MovementInform) + SAY_KILL 8 (1) +
-- SAY_WIPE 9 (1, JustReachedHome) + SAY_BERSERK 10 (1, scheduler)
-- + SAY_DEATH 11 (1) + EMOTE_BERSERK_RAID 12 (1, scheduler)).

local ENTRY_BLOOD_QUEEN_LANA_THEL = 37955

local DATA_BLOOD_QUEEN_LANA_THEL = 8 -- icecrown_citadel.h:82
local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3

local SAY_AGGRO = 0
local SAY_KILL = 8
local SAY_WIPE = 9
local SAY_DEATH = 11
local SAY_BERSERK = 10
local EMOTE_BERSERK_RAID = 12
local SAY_SWARMING_SHADOWS = 4
local SAY_VAMPIRIC_BITE = 1
local EMOTE_SWARMING_SHADOWS = 5
local SAY_PACT_OF_THE_DARKFALLEN = 6

local SPELL_BERSERK = 26662
local SPELL_SWARMING_SHADOWS = 71264
local SPELL_SHADOWS_FATE = 71169
local SPELL_BLOOD_INFUSION_CREDIT = 72934

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

-- C++ _vampires GuidSet (boss_blood_queen_lana_thel.cpp:508) as
-- per-player keys in the shared instanceData table (the
-- boss_northrend_beasts snoboldCount precedent — one real Lua table
-- per (map, instance), engine/scripting/object.go): the Go gate
-- reads "vampire_<guidLow>" via InstanceDataNumber
-- (engine/world/achievement_criteria_scripts.go). GetGUIDLow is the
-- low 32 bits of the player GUID (lua_player.go:105); the Go side
-- formats the same value, so the key agrees exactly. The marks are
-- cleared at ENGAGE (clearVampireMarks, the C++ Reset :169
-- _vampires.clear() twin — Reset precedes engage on every attempt),
-- NEVER in the ev23 Reset handler: ev23 pre-fires on death
-- (fireCreatureDied, lua_creature_events.go) ahead of the
-- kill-credit achievement read (kill.go), so a Reset clear would
-- wipe the fight's marks before the criteria check reads them.
local function vampireKey(guidLow)
    return "vampire_" .. guidLow
end

local function recordVampire(creature, target)
    local mapObj = creature:GetMap()
    if mapObj == nil then
        return
    end
    local data = mapObj:GetInstanceData()
    if data == nil then
        return
    end
    data[vampireKey(target:GetGUIDLow())] = 1
end

local function clearVampireMarks(creature)
    local mapObj = creature:GetMap()
    if mapObj == nil then
        return
    end
    local data = mapObj:GetInstanceData()
    if data == nil then
        return
    end
    for key in pairs(data) do
        if type(key) == "string" and string.sub(key, 1, 8) == "vampire_" then
            data[key] = nil
        end
    end
end

-- C++ SelectRandomTarget(false) pool shape
-- (boss_blood_queen_lana_thel.cpp:478-507): the sorted threat list
-- minus the maintank (victim); GetAITargets is threat-sorted
-- (SortedEntries, rotface/anomalus precedent). The offtank exclusion
-- is unmodeled (Blood Mirror unported, _offtankGUID never set here).
local function threatPlayersMinusVictim(creature)
    local victim = creature:GetVictim()
    local victimGuid = victim ~= nil and victim:GetGUID() or nil
    local pool = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and unit:GetGUID() ~= victimGuid then
            pool[#pool + 1] = unit
        end
    end
    return pool
end

-- RandomResize twin: up to count distinct random picks from pool
-- (pool is consumed; C++ RandomResize keeps min(size, count)).
local function takeRandom(pool, count)
    local picked = {}
    while #picked < count and #pool > 0 do
        local i = math.random(#pool)
        picked[#picked + 1] = pool[i]
        table.remove(pool, i)
    end
    return picked
end

-- C++ :421-428 EVENT_SWARMING_SHADOWS: if (target =
-- SelectRandomTarget(false)) { Talk(EMOTE_SWARMING_SHADOWS, target);
-- Talk(SAY_SWARMING_SHADOWS); DoCast(target, SPELL_SWARMING_SHADOWS)
-- (2-arg = non-triggered) }; ScheduleEvent(EVENT_SWARMING_SHADOWS,
-- 30500ms) — unconditional re-arm. Talk(5, target) + Talk(4) +
-- the cast port, in C++ order (targeted Talk bridged).
local function swarmingShadowsTick(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :344 (+ in-loop :459, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "swarming", 500, function() swarmingShadowsTick(creature, guid) end)
        return
    end
    local pool = threatPlayersMinusVictim(creature)
    local target = nil
    if #pool > 0 then
        target = pool[math.random(#pool)]
    end
    if target ~= nil then
        creature:Talk(EMOTE_SWARMING_SHADOWS, target)
        creature:Talk(SAY_SWARMING_SHADOWS)
        creature:CastSpell(target, SPELL_SWARMING_SHADOWS)
    end
    schedule(guid, "swarming", 30500, function() swarmingShadowsTick(creature, guid) end)
end

-- C++ EVENT_BERSERK: 330s single-fire — Talk(EMOTE_BERSERK_RAID) +
-- Talk(SAY_BERSERK) + DoCastSelf(SPELL_BERSERK) non-triggered
-- (:160/:351-354), UpdateAI CASTING-gated (:344/:459) one-shot —
-- the gate re-schedules a fresh 500ms entry, never drops the cast.
local function onBerserk(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :344 (+ in-loop :459, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function() onBerserk(creature, guid) end)
        return
    end
    creature:Talk(EMOTE_BERSERK_RAID)
    creature:Talk(SAY_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK)
end

local SPELL_SHROUD_OF_SORROW = 70986
local SPELL_FRENZIED_BLOODTHIRST_VISUAL = 71949
local SPELL_CLEAR_ALL_STATUS_AILMENTS = 70939

-- C++ CleanAuras RAID_MODE pools (boss_blood_queen_lana_thel.cpp:94-95):
-- only one variant can be present per instance, so the strip pool
-- checks all four (the twin_valkyr essence-pool convention).
local ESSENCE_OF_BLOOD_QUEEN_AURAS = { 70867, 71473, 71532, 71533 }
local ESSENCE_OF_BLOOD_QUEEN_PLR_AURAS = { 70879, 71525, 71530, 71531 }
local SPELL_FRENZIED_BLOODTHIRST = 70877
local SPELL_UNCONTROLLABLE_FRENZY = 70923
local SPELL_BLOOD_MIRROR_DAMAGE = 70821
local SPELL_BLOOD_MIRROR_VISUAL = 71510
local SPELL_BLOOD_MIRROR_DUMMY = 70838
local SPELL_DELIRIOUS_SLASH = 71623
local SPELL_PACT_OF_THE_DARKFALLEN = 71340
local SPELL_PRESENCE_OF_THE_DARKFALLEN = 70994
local SPELL_VAMPIRIC_BITE = 71726
local SPELL_VAMPIRIC_BITE_DUMMY = 71837
local SPELL_PRESENCE_OF_THE_DARKFALLEN_2 = 71952
local SPELL_TWILIGHT_BLOODBOLT = 71446
local SPELL_TWILIGHT_BLOODBOLT_TARGET = 71445

-- C++ RAID_MODE macros (boss_blood_queen_lana_thel.cpp:94-98):
-- FRENZIED_BLOODTHIRST (70877, 71474), DELIRIOUS_SLASH (71623-71626),
-- PRESENCE_OF_THE_DARKFALLEN (70994, 71962-71964) — expanded like the
-- essence pools below; the single-variant strips missed 25-man /
-- heroic auras on Reset/Engage/JustDied wipes.
local FRENZIED_BLOODTHIRST_AURAS = { 70877, 71474 }
local DELIRIOUS_SLASH_AURAS = { 71623, 71624, 71625, 71626 }
local PRESENCE_OF_THE_DARKFALLEN_AURAS = { 70994, 71962, 71963, 71964 }

-- C++ EVENT_VAMPIRIC_BITE (:356-368, one-shot, no re-arm): front
-- pick of the threat pool (targets.front() after the maintank/offtank
-- strip) + DoCast bite + DoCastAOE dummy triggered + Talk(1) +
-- target self-casts PRESENCE + PRESENCE_2 triggered, C++ order.
local function onVampiricBite(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :344 (+ in-loop :459, single-tick
-- no-op per the marrowgar precedent): 500ms IsCasting defer, no
-- re-arm consumed (the mal_ganis idiom) — one-shot, so the defer
-- re-schedules a fresh 500ms entry rather than dropping the bite.
    if creature:IsCasting() then
        schedule(guid, "bite", 500, function() onVampiricBite(creature, guid) end)
        return
    end
    local pool = threatPlayersMinusVictim(creature)
    local target = pool[1]
    if target ~= nil then
        creature:CastSpell(target, SPELL_VAMPIRIC_BITE)
        creature:CastSpell(creature, SPELL_VAMPIRIC_BITE_DUMMY, true)
        creature:Talk(SAY_VAMPIRIC_BITE)
        target:CastSpell(target, SPELL_PRESENCE_OF_THE_DARKFALLEN, true)
        target:CastSpell(target, SPELL_PRESENCE_OF_THE_DARKFALLEN_2, true)
        -- C++ EVENT_VAMPIRIC_BITE (:366): _vampires.insert(target GUID).
        recordVampire(creature, target)
    end
end

-- C++ EVENT_PACT_OF_THE_DARKFALLEN (:407-420): RandomResize(2/3 by
-- Is25ManRaid); Talk(6) + DoCast each only if picked > 1; the
-- 30500ms re-arm is unconditional (outside the size check).
local function pactTick(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :344 (+ in-loop :459, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "pact", 500, function() pactTick(creature, guid) end)
        return
    end
    local pool = threatPlayersMinusVictim(creature)
    local count = 2
    if creature:Is25ManRaid() then
        count = 3
    end
    local picked = takeRandom(pool, count)
    if #picked > 1 then
        creature:Talk(SAY_PACT_OF_THE_DARKFALLEN)
        for _, target in ipairs(picked) do
            creature:CastSpell(target, SPELL_PACT_OF_THE_DARKFALLEN)
        end
    end
    schedule(guid, "pact", 30500, function() pactTick(creature, guid) end)
end

-- C++ EVENT_TWILIGHT_BLOODBOLT (:430-438): RandomResize(2/4 by
-- Is25ManRaid) casts of 71446 + unconditional self-cast 71445.
-- The 71446 resist leg runs Go-side (twilightBloodboltResist,
-- boss_blood_queen_lana_thel.cpp:772-791: shadow + arcane resists
-- summed on the full damage); the 71445 TARGET filter leg stays out.
local function bloodboltTick(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :344 (+ in-loop :459, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "bloodbolt", 500, function() bloodboltTick(creature, guid) end)
        return
    end
    local pool = threatPlayersMinusVictim(creature)
    local count = 2
    if creature:Is25ManRaid() then
        count = 4
    end
    local picked = takeRandom(pool, count)
    for _, target in ipairs(picked) do
        creature:CastSpell(target, SPELL_TWILIGHT_BLOODBOLT)
    end
    creature:CastSpell(creature, SPELL_TWILIGHT_BLOODBOLT_TARGET)
    schedule(guid, "bloodbolt", {10000, 15000}, function() bloodboltTick(creature, guid) end)
end

-- C++ boss_blood_queen_lana_thel::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent; the
-- CheckRequiredBosses evade branch (:174-180) has no bridge (same
-- accepted deviation as the Talk).
-- C++ CleanAuras (boss_blood_queen_lana_thel.cpp:221-233): the ten
-- DoRemoveAurasDueToSpellOnPlayers strips, in C++ call order (the two
-- RAID_MODE essence pools expand to all four variants). The
-- instance-player set is the mapId/instanceId-filtered
-- GetPlayersInWorld() pool (the moroes precedent) and
-- player:RemoveAura is bridged (lua_player.go:219).
local function cleanAuras(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local pools = {
        ESSENCE_OF_BLOOD_QUEEN_AURAS,
        ESSENCE_OF_BLOOD_QUEEN_PLR_AURAS,
        FRENZIED_BLOODTHIRST_AURAS,
        { SPELL_UNCONTROLLABLE_FRENZY },
        { SPELL_BLOOD_MIRROR_DAMAGE },
        { SPELL_BLOOD_MIRROR_VISUAL },
        { SPELL_BLOOD_MIRROR_DUMMY },
        DELIRIOUS_SLASH_AURAS,
        { SPELL_PACT_OF_THE_DARKFALLEN },
        PRESENCE_OF_THE_DARKFALLEN_AURAS,
    }
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId then
            for _, pool in ipairs(pools) do
                for _, aura in ipairs(pool) do
                    p:RemoveAura(aura)
                end
            end
        end
    end
end

local function lanaThelJustEngagedWith(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ Reset (:169): _vampires.clear() (engage is the clear site —
    -- see the vampireKey note above; never in ev23 Reset).
    clearVampireMarks(creature)
    -- C++ :184 — me-form DoZoneInCombat after me->setActive(true), before
    -- Talk(SAY_AGGRO) (C++ arm order; the mandokir/algalon event-1 precedent).
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    -- C++ JustEngagedWith (:185): SetBossState(IN_PROGRESS) after
    -- Talk(SAY_AGGRO), before CleanAuras() (C++ arm order). PORTED
    -- 2026-11-06 via creature:SetBossState.
    creature:SetBossState(DATA_BLOOD_QUEEN_LANA_THEL, BOSS_STATE_IN_PROGRESS)
    cleanAuras(creature)
    -- C++ :188-190 — triggered self-cast auras (curator engage-cast
    -- precedent): DoCast(me, SPELL_SHROUD_OF_SORROW, true) +
    -- DoCast(me, SPELL_FRENZIED_BLOODTHIRST_VISUAL, true) +
    -- DoCastSelf(SPELL_CLEAR_ALL_STATUS_AILMENTS, true).
    creature:CastSpell(creature, SPELL_SHROUD_OF_SORROW, true)
    creature:CastSpell(creature, SPELL_FRENZIED_BLOODTHIRST_VISUAL, true)
    creature:CastSpell(creature, SPELL_CLEAR_ALL_STATUS_AILMENTS, true)
    schedule(guid, "berserk", 330000, function() onBerserk(creature, guid) end)
    schedule(guid, "swarming", 30500, function() swarmingShadowsTick(creature, guid) end)
    schedule(guid, "bite", 15000, function() onVampiricBite(creature, guid) end)
    schedule(guid, "pact", 15000, function() pactTick(creature, guid) end)
    schedule(guid, "bloodbolt", {20000, 25000}, function() bloodboltTick(creature, guid) end)
end

-- C++ Reset() timer machinery — cancel/re-arm on combat events
-- (the maiden convention).
local function lanaThelLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function lanaThelReset(event, creature)
    -- C++ Reset() :159 — _Reset() first: SetBossState NOT_STARTED
    -- only when the stored state != DONE (BossAI::_Reset,
    -- ScriptedCreature.cpp:499-509; the falric precedent). PORTED
    -- 2026-11-06 via creature:SetBossState.
    if creature:GetBossState(DATA_BLOOD_QUEEN_LANA_THEL) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_BLOOD_QUEEN_LANA_THEL, BOSS_STATE_NOT_STARTED)
    end
    cancelTimers(creature:GetGUIDLow())
    -- C++ Reset() ends with CleanAuras() (boss_blood_queen_lana_thel.cpp:168),
    -- after the timer arms; the _vampires.clear() leg (:169) ports at
    -- ENGAGE (clearVampireMarks — see the vampireKey note; a Reset
    -- clear would be wiped by the death pre-fire before the criteria
    -- read) and Initialize() has no Lua latch.
    cleanAuras(creature)
end

-- C++ boss_blood_queen_lana_thel::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function lanaThelKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_blood_queen_lana_thel::JustDied:
-- Talk(SAY_DEATH) — the sjonnir JustDied-Talk precedent.

local function lanaThelJustDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    -- C++ JustDied :196 — _JustDied() sets DONE, first arm (before
    -- Talk). PORTED 2026-11-06 via creature:SetBossState.
    creature:SetBossState(DATA_BLOOD_QUEEN_LANA_THEL, BOSS_STATE_DONE)
    creature:Talk(SAY_DEATH)
    -- C++ JustDied (:195-218): the Is25ManRaid() + HasAura(SPELL_SHADOWS_FATE)-gated
    -- DoCastAOE(SPELL_BLOOD_INFUSION_CREDIT, true) leg (:199-200) ports
    -- via self-cast (DoCastAOE = self-cast convention); CleanAuras()
    -- (:221-233) ports via cleanAuras above (C++ order kept); the
    -- blood-quickening SetData/RewardPlayerAndGroupAtEvent/
    -- FindNearestCreature minchar legs have no instance bridge.
    if creature:Is25ManRaid() and creature:HasAura(SPELL_SHADOWS_FATE) then
        creature:CastSpell(creature, SPELL_BLOOD_INFUSION_CREDIT, true)
    end
    cleanAuras(creature)
end

-- C++ boss_blood_queen_lana_thel::JustReachedHome: SetDisableGravity(false)
-- (bridge live), then SetReactState(REACT_AGGRESSIVE 2) in C++ arm order
-- ahead of the Talk(SAY_WIPE 9); the instance->SetBossState(FAIL)
-- leg (:275, after Talk) is PORTED 2026-11-06 via creature:SetBossState.
-- The old "no-gravity bridge" note was stale.
local function lanaThelReachHome(event, creature)
    creature:SetDisableGravity(false)
    creature:SetReactState(2)
    creature:Talk(SAY_WIPE)
    creature:SetBossState(DATA_BLOOD_QUEEN_LANA_THEL, BOSS_STATE_FAIL)
end

RegisterCreatureEvent(ENTRY_BLOOD_QUEEN_LANA_THEL, 1, lanaThelJustEngagedWith)
RegisterCreatureEvent(ENTRY_BLOOD_QUEEN_LANA_THEL, 2, lanaThelLeaveCombat)
RegisterCreatureEvent(ENTRY_BLOOD_QUEEN_LANA_THEL, 23, lanaThelReset)
RegisterCreatureEvent(ENTRY_BLOOD_QUEEN_LANA_THEL, 24, lanaThelReachHome)
RegisterCreatureEvent(ENTRY_BLOOD_QUEEN_LANA_THEL, 3, lanaThelKilledUnit)
RegisterCreatureEvent(ENTRY_BLOOD_QUEEN_LANA_THEL, 4, lanaThelJustDied)

-- C++ spell_blood_queen_pact_of_the_darkfallen_dmg::PeriodicTick
-- (boss_blood_queen_lana_thel.cpp:737-749, EFFECT_1): NO
-- PreventDefaultAction — this is an additional effect, so the Go
-- default trigger cast still fires alongside it, exactly like C++
-- (the handler return is void, and the Go hook fires once per aura
-- tick, so the cadence matches whichever effect Go models as the
-- aura's trigger: default casts that effect's DBC trigger, the
-- handler casts the damage spell with the scaled BP0). Damage is
-- the DBC CalcValue of the damage spell (71341, EFFECT_0) scaled by
-- 0.3375 + 0.1 * (tickNumber / 10) with integer division, then cast
-- triggered on the target with the scaled value as BP0
-- (CastSpellExtraArgs AddSpellBP0). GetTickNumber is the tick-191
-- bridge; the DBC value read is the new aura:GetSpellEffectCalcValue
-- bridge (lua_aura.go), the CalcValue() no-caster twin
-- (SpellInfo.cpp:402-440: base points + DieSides roll, no level
-- scaling). The nonzero gate is the GetTriggerSpell convention: the
-- bridge reads 0 with no Spell.dbc loaded, so a zero value means
-- "no data", never a real 0-damage cast.
local SPELL_PACT_OF_THE_DARKFALLEN_DAMAGE = 71341

local function pactOfTheDarkfallenDmgTick(event, aura, target)
    local damage = aura:GetSpellEffectCalcValue(SPELL_PACT_OF_THE_DARKFALLEN_DAMAGE, 0)
    if damage ~= 0 then
        local multiplier = 0.3375 + 0.1 * math.floor(aura:GetTickNumber() / 10)
        damage = math.floor(damage * multiplier)
        target:CastCustomSpell(target, SPELL_PACT_OF_THE_DARKFALLEN_DAMAGE, true, damage)
    end
end

RegisterAuraEvent(SPELL_PACT_OF_THE_DARKFALLEN, 1, pactOfTheDarkfallenDmgTick)
