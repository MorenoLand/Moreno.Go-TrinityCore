-- The Lich King escape (Halls of Reflection) — Lua port of the
-- DamageTaken arm of npc_the_lich_king_escape_hor in
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/halls_of_reflection.cpp
-- (C++ :1183-1390; part of the 25 scripts registered from inside
-- AddSC_halls_of_reflection(); loader decl 167 / call 362 per
-- northrend_script_loader.cpp — the SECOND group of the "// Halls of
-- Reflection" block in AddNorthrendScripts(), immediately after
-- AddSC_instance_halls_of_reflection() (call 361), before
-- AddSC_boss_falric() (call 363)).
-- Entry: 36954 NPC_THE_LICH_KING_ESCAPE (halls_of_reflection.h line 84)
-- — the CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_halls_of_reflection" hits halls_of_reflection.cpp only (+ the
-- loader decl/call lines).
-- Eluna creature events: 2 OnLeaveCombat, 3 OnTargetDied,
-- 5 OnSpawn, 9 OnDamageTaken. The handler returns the
-- rewritten damage as the second return (the
-- engine/world/lua_creature_events.go fireCreatureDamageTaken
-- convention); the boolean first return only gates the empty
-- ScriptedAI::DamageTaken base, a provable no-op.
--
-- PORTED ARMS (C++ line refs; C++-exact for the modeled arm):
-- Ctor (:1188-1195) — _instance->SetBossState
--   (DATA_THE_LICH_KING_ESCAPE 2, NOT_STARTED) PORTED 2026-11-05 via
--   creature:SetBossState in an ev5 OnSpawn handler (the ctor runs
--   when the AI is created, i.e. at summon; the escape creature is
--   re-summoned fresh on each escape attempt, so the spawn-time write
--   is C++-exact — it also clears a stale FAIL after a previous
--   evade, which is what lets at_hor_shadow_throne re-arm
--   IN_PROGRESS). The old file had no spawn handler at all.
-- DamageTaken (:1198-1202) — ungated clamp: if damage >= current
--   health, damage = health - 1 (the Lich King cannot be killed during
--   the escape chase). Event 9 fires pre-application, so
--   creature:GetHealth() is the pre-damage value, exactly like C++'s
--   me->GetHealth() in the hook.
-- EnterEvadeMode (:1305-1316) — the _despawn once-latch (GUID-keyed, the
--   svalna captain precedent), instance SetBossState
--   (DATA_THE_LICH_KING_ESCAPE 2, FAIL) PORTED 2026-11-05 via
--   creature:SetBossState FIRST, in C++ order (lua_creature_events.go:
--   5917, the garfrost precedent — the old "no bridge" claim for this
--   leg is retracted; the bridge writes the same store the C++
--   override reads), then DoPlaySoundToSet(SOUND_LK_FURY_OF_FROSTMOURNE
--   17224) = creature:PlayDirectSound (ScriptedAI::DoPlaySoundToSet calls
--   source->PlayDirectSound — ScriptedCreature.cpp:188-200, broadcast-to-set
--   SMSG_PLAY_SOUND, the engine/world/lua_objects.go PlayDirectSound bridge),
--   and DoCastAOE(SPELL_FURY_OF_FROSTMOURNE 70063) = self-cast (the file's
--   DoCastAOE idiom), in C++ order. StopMoving + the 12s delayed Despawn are
--   PORTED below.
-- KilledUnit (:1251-1255) — player-gated
--   (victim:GetObjectType() == "Player", the razuvious precedent)
--   DoPlaySoundToSet(RAND(SOUND_LK_SLAY_1 17214, SOUND_LK_SLAY_2 17215)) =
--   creature:PlayDirectSound(math.random(SOUND_LK_SLAY_1, SOUND_LK_SLAY_2)).
--   The old "no sound bridge" note was wrong: the bridge is PlayDirectSound.
-- SetData(DATA_ICEWALL) machine (:1257-1298) — PORTED 2026-11-10 via
--   the Go-only DoAction signal (ACTION_LK_ICEWALL -668010) the leader
--   sends from SummonIceWall after writing SetData(DATA_ICEWALL 11,
--   _icewall): case 0 (6 ghouls + 1 witch doctor) also DoZoneInCombat
--   + EVENT_REMORSELESS_WINTER at once; the summon schedules are the
--   C++-exact delays per case (0: 8s/14s, 1: 8s/13s/15s/18s,
--   2: 9s/14s/15s/19s/39s, 3: 9s/15s/19s/40s/45s/55s/62s/65s/14s)
--   with the matching Talk(SAY_LK_ESCAPE_ICEWALL_SUMMONED_1..4).
--   One dispatch function (no forward references); each pump defers
--   500ms on not-in-combat (the C++ SelectVictim pre-loop gate) or
--   IsCasting (the C++ CASTING pre-loop return), the mal_ganis idiom.
--   The old "rides instance SetData legs with no instance bridge"
--   note was wrong twice over: the caller is the leader AI's
--   SetData, not the instance, and the Go-only DoAction signal
--   (the HOR_SETDATA_NEXT_WAVE pattern) carries it.
-- UpdateAI summon events (:1321-1358) — EVENT_REMORSELESS_WINTER
--   (StopMoving + Talk WINTER + self-cast 69780),
--   EVENT_ESCAPE_SUMMON_GHOULS (StopMoving + Talk GHOULS + self-cast
--   69818), EVENT_ESCAPE_SUMMON_WITCH_DOCTOR (self-cast 69836),
--   EVENT_ESCAPE_SUMMON_LUMBERING_ABOMINATION (Talk ABOMINATION +
--   self-cast 69835). The FURY_OF_FROSTMOURNE AoE (:1311-1314)
--   stays out: it fires only from EnterEvadeMode, whose legs are
--   ported above (the ev2 handler), not from this machine.
-- MovementInform (:1204-1231) — PORTED 2026-11-10 via ev28 (the
--   nether drake precedent): point 1 MoveChase(the escape leader
--   from instanceData.jainaOrSylvanasEscape); point 2 Talk
--   HARVEST_SOUL + DoCast HARVEST_SOUL 69866 on the leader. The
--   gunship EnableMovement leg stays out (no transport bridge).
--   The old "no-MovementInform / no-instance-GetGuidData bridges"
--   note is retracted: ev28 + the instance bind carry both legs.
-- SelectVictim passive-evade (:1349-1361) — PORTED 2026-11-10 as a
--   1s poller (started on spawn, cancelled on ev2/ev4): passive +
--   REMORSELESS_WINTER aura up + no alive player on the threat
--   list runs the evade legs. The C++ gate is PvE combat refs < 2;
--   the player-count gate is its conservative subset (it can only
--   miss a fire, never false-fire: with a living player on threat
--   the encounter is alive by definition). The non-passive
--   AttackStart-victim leg is engine-driven in Go.
-- EnterEvadeMode (:1305-1316) — the _despawn once-latch, SetBossState
--   (FAIL), sound, cast, StopMoving and 12s delayed Despawn are all
--   ported above, shared with the poller via runLkEvadeLegs; the
--   machine-timer + poller teardown and the shared-counter reset
--   (lkPendingSummons / escapeSummonCount zeroed so a failed attempt
--   cannot trip the next attempt's wall-broken check) are ported
--   2026-11-10.
-- JustSummoned / SummonedCreatureDies (:1233-1250) — the
--   _summonsCount bookkeeping is ported WITHOUT a summon-list
--   bridge: the summoned trash increment instanceData
--   .escapeSummonCount on spawn (ev5, npc_hor_escape_trash.lua)
--   and decrement it on death (ev4); the LK machine counts
--   scheduled-but-unfired summon events in
--   instanceData.lkPendingSummons; the trash death handler fires
--   leader DoAction(ACTION_WALL_BROKEN) when the count hits 0
--   with no summon events pending and the escape boss state
--   IN_PROGRESS — the C++ `!_summonsCount && _events.Empty()`
--   check, read at summon death exactly as C++ reads it.
--   PORTED 2026-11-10 (trash side); see npc_hor_escape_trash.lua.
--
-- DOCUMENTED-ONLY: (none — every arm above is ported or has its
--   stay-out leg noted inline.)

local ENTRY_LICH_KING_ESCAPE = 36954
local DATA_THE_LICH_KING_ESCAPE = 2 -- halls_of_reflection.h:39
local DATA_ICEWALL = 11 -- halls_of_reflection.h:46
local ACTION_LK_ICEWALL = -668010 -- Go-only: leader's SummonIceWall signal
local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_FAIL = 2 -- InstanceScript.h:68-76

local POINT_MOTION_TYPE = 8 -- MovementDefines.h (nether drake precedent)
local REACT_PASSIVE = 0 -- UnitDefines.h:340 (falric precedent)

local SPELL_FURY_OF_FROSTMOURNE = 70063
local SPELL_REMORSELESS_WINTER = 69780 -- halls_of_reflection.cpp:247
local SPELL_RAISE_DEAD = 69818
local SPELL_SUMMON_RISEN_WITCH_DOCTOR = 69836
local SPELL_SUMMON_LUMBERING_ABOMINATION = 69835
local SPELL_HARVEST_SOUL = 69866

local SAY_LK_ESCAPE_ICEWALL_SUMMONED_1 = 2 -- halls_of_reflection.cpp:125
local SAY_LK_ESCAPE_ICEWALL_SUMMONED_2 = 3
local SAY_LK_ESCAPE_ICEWALL_SUMMONED_3 = 4
local SAY_LK_ESCAPE_ICEWALL_SUMMONED_4 = 5
local SAY_LK_ESCAPE_GHOULS = 6
local SAY_LK_ESCAPE_ABOMINATION = 7
local SAY_LK_ESCAPE_WINTER = 8
local SAY_LK_ESCAPE_HARVEST_SOUL = 9

local SOUND_LK_FURY_OF_FROSTMOURNE = 17224
local SOUND_LK_SLAY_1 = 17214
local SOUND_LK_SLAY_2 = 17215

-- C++ _despawn latch (member, ctor-false, set on first evade-fire, never
-- reset): GUID-keyed, never cleared on 2/23 — a respawn mints a fresh GUID,
-- matching the ctor (the svalna captain precedent).
local evadeFired = {}

local lkTimers = {}
local lkSeq = {}
local lkEvadePolls = {}

local function cancelLkTimers(guid)
    local per = lkTimers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        lkTimers[guid] = nil
    end
end

local function lkSchedule(guid, key, delay, fn)
    local per = lkTimers[guid]
    if not per then
        per = {}
        lkTimers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function lkAlive(guid)
    return lkTimers[guid] ~= nil
end

local function getInstanceData(creature)
    local mapObj = creature:GetMap()
    if not mapObj then
        return nil
    end
    return mapObj:GetInstanceData()
end

local function getLeader(creature)
    local data = getInstanceData(creature)
    if not data then
        return nil
    end
    return data.jainaOrSylvanasEscape
end

-- Shared summon counters (instanceData.lkPendingSummons counts
-- scheduled-but-unfired LK summon events; .escapeSummonCount counts
-- live LK-summoned trash — the C++ _events.Empty() / _summonsCount
-- pair, split across the two files because Lua has no per-AI
-- members). Zeroed on a fresh attempt (spawn) and on a failed one
-- (evade) so a stale count cannot trip the next attempt's
-- wall-broken check.
local function resetLkCounters(creature)
    local data = getInstanceData(creature)
    if data then
        data.lkPendingSummons = 0
        data.escapeSummonCount = 0
    end
end

-- C++ UpdateAI summon events (:1321-1358). One dispatch function (no
-- forward references); kind "winter" is the only non-summon event and
-- does not touch the pending-summon counter. The not-in-combat gate
-- is the C++ SelectVictim pre-loop return; the IsCasting gate is the
-- C++ CASTING pre-loop return — both defer 500ms without consuming
-- the event (the mal_ganis idiom).
local function runLkEvent(creature, guid, kind, key)
    if not lkAlive(guid) then
        return
    end
    if not creature:IsInCombat() or creature:IsCasting() then
        lkSchedule(guid, key, 500, function() runLkEvent(creature, guid, kind, key) end)
        return
    end
    if kind ~= "winter" then
        local data = getInstanceData(creature)
        if data then
            data.lkPendingSummons = math.max(0, (data.lkPendingSummons or 0) - 1)
        end
    end
    if kind == "winter" then -- EVENT_REMORSELESS_WINTER :1330-1336
        creature:StopMoving()
        creature:Talk(SAY_LK_ESCAPE_WINTER)
        creature:CastSpell(creature, SPELL_REMORSELESS_WINTER)
    elseif kind == "ghouls" then -- EVENT_ESCAPE_SUMMON_GHOULS :1337-1342
        creature:StopMoving()
        creature:Talk(SAY_LK_ESCAPE_GHOULS)
        creature:CastSpell(creature, SPELL_RAISE_DEAD)
    elseif kind == "witchdoctor" then -- EVENT_ESCAPE_SUMMON_WITCH_DOCTOR :1343-1347
        creature:CastSpell(creature, SPELL_SUMMON_RISEN_WITCH_DOCTOR)
    elseif kind == "abomination" then -- EVENT_ESCAPE_SUMMON_LUMBERING_ABOMINATION :1348-1358
        creature:Talk(SAY_LK_ESCAPE_ABOMINATION)
        creature:CastSpell(creature, SPELL_SUMMON_LUMBERING_ABOMINATION)
    end
end

-- Unique key per scheduled event: case 3 schedules two witch-doctor
-- and two ghouls events, and the C++ EventMap holds both.
local function lkScheduleKind(creature, guid, kind, delay)
    lkSeq[guid] = (lkSeq[guid] or 0) + 1
    local key = kind .. "#" .. lkSeq[guid]
    lkSchedule(guid, key, delay, function() runLkEvent(creature, guid, kind, key) end)
end

-- C++ SetData(DATA_ICEWALL) (:1257-1298): the per-wall summon
-- schedules, in C++ order, Talk last. Summon kinds increment the
-- shared pending-summon counter (read by the trash death handler).
local function lkIcewallMachine(creature, guid, icewall)
    local data = getInstanceData(creature)
    local function addPending(n)
        if data then
            data.lkPendingSummons = (data.lkPendingSummons or 0) + n
        end
    end
    if icewall == 0 then -- 6 Ghouls, 1 Witch Doctor
        creature:DoZoneInCombat()
        lkScheduleKind(creature, guid, "winter", 1)
        lkScheduleKind(creature, guid, "ghouls", 8000)
        lkScheduleKind(creature, guid, "witchdoctor", 14000)
        addPending(2)
        creature:Talk(SAY_LK_ESCAPE_ICEWALL_SUMMONED_1)
    elseif icewall == 1 then -- 6 Ghouls, 2 Witch Doctor, 1 Lumbering Abomination
        lkScheduleKind(creature, guid, "ghouls", 8000)
        lkScheduleKind(creature, guid, "abomination", 13000)
        lkScheduleKind(creature, guid, "witchdoctor", 15000)
        lkScheduleKind(creature, guid, "witchdoctor", 18000)
        addPending(4)
        creature:Talk(SAY_LK_ESCAPE_ICEWALL_SUMMONED_2)
    elseif icewall == 2 then -- 6 Ghouls, 2 Witch Doctor, 2 Lumbering Abomination
        lkScheduleKind(creature, guid, "ghouls", 9000)
        lkScheduleKind(creature, guid, "abomination", 14000)
        lkScheduleKind(creature, guid, "witchdoctor", 15000)
        lkScheduleKind(creature, guid, "abomination", 19000)
        lkScheduleKind(creature, guid, "witchdoctor", 39000)
        addPending(5)
        creature:Talk(SAY_LK_ESCAPE_ICEWALL_SUMMONED_3)
    elseif icewall == 3 then -- 12 Ghouls, 4 Witch Doctor, 3 Lumbering Abomination
        lkScheduleKind(creature, guid, "ghouls", 9000)
        lkScheduleKind(creature, guid, "witchdoctor", 15000)
        lkScheduleKind(creature, guid, "witchdoctor", 19000)
        lkScheduleKind(creature, guid, "abomination", 40000)
        lkScheduleKind(creature, guid, "abomination", 45000)
        lkScheduleKind(creature, guid, "ghouls", 55000)
        lkScheduleKind(creature, guid, "witchdoctor", 62000)
        lkScheduleKind(creature, guid, "witchdoctor", 65000)
        lkScheduleKind(creature, guid, "abomination", 14000)
        addPending(9)
        creature:Talk(SAY_LK_ESCAPE_ICEWALL_SUMMONED_4)
    end
end

local function cancelLkEvadePoll(guid)
    local id = lkEvadePolls[guid]
    if id then
        RemoveEventById(id)
        lkEvadePolls[guid] = nil
    end
end

-- C++ EnterEvadeMode (:1305-1316), C++ order: instance SetBossState
-- (DATA_THE_LICH_KING_ESCAPE 2, FAIL) -> StopMoving ->
-- DoPlaySoundToSet(SOUND_LK_FURY_OF_FROSTMOURNE 17224) — PlayDirectSound
-- (the DoPlaySoundToSet ≡ PlayDirectSound identity above) -> DoCastAOE
-- (SPELL_FURY_OF_FROSTMOURNE 70063) non-triggered — self-cast (the file's
-- DoCastAOE idiom) -> DespawnOrUnsummon(12s), modeled as a 12s
-- CreateLuaEvent one-shot calling creature:Despawn (immediate bridge).
-- All ungated apart from the _despawn once-latch. Shared by the ev2
-- handler and the SelectVictim poller below; teardown (machine timers,
-- poller, shared counters) runs here so both callers get it.
local function runLkEvadeLegs(creature, guid)
    if evadeFired[guid] then
        return
    end
    evadeFired[guid] = true
    cancelLkTimers(guid)
    cancelLkEvadePoll(guid)
    resetLkCounters(creature)
    creature:SetBossState(DATA_THE_LICH_KING_ESCAPE, BOSS_STATE_FAIL)
    creature:StopMoving()
    creature:PlayDirectSound(SOUND_LK_FURY_OF_FROSTMOURNE)
    creature:CastSpell(creature, SPELL_FURY_OF_FROSTMOURNE)
    CreateLuaEvent(function()
        creature:Despawn()
    end, 12000)
end

-- C++ SelectVictim passive branch (:1349-1361): passive + PvE combat
-- refs < 2 + REMORSELESS_WINTER aura up -> EnterEvadeMode. The 1s
-- poller is the UpdateAI twin; the alive-player-on-threat gate is the
-- conservative subset of the combat-refs gate (it can only miss a
-- fire, never false-fire: with a living player on threat the
-- encounter is alive by definition). The non-passive
-- AttackStart-victim leg is engine-driven in Go.
local function lkEvadePoll(creature, guid)
    if lkEvadePolls[guid] == nil then
        return
    end
    if creature:GetReactState() == REACT_PASSIVE and creature:HasAura(SPELL_REMORSELESS_WINTER) then
        local alivePlayer = false
        for _, t in ipairs(creature:GetAITargets()) do
            if t:GetObjectType() == "Player" and not t:IsDead() then
                alivePlayer = true
                break
            end
        end
        if not alivePlayer then
            runLkEvadeLegs(creature, guid)
            return
        end
    end
    lkEvadePolls[guid] = CreateLuaEvent(function() lkEvadePoll(creature, guid) end, 1000)
end

local function startLkEvadePoll(creature, guid)
    cancelLkEvadePoll(guid)
    lkEvadePolls[guid] = CreateLuaEvent(function() lkEvadePoll(creature, guid) end, 1000)
end

-- C++ DamageTaken :1198-1202 — the clamp keeps the Lich King alive
-- through the escape; return the replacement damage as the second
-- return per the fireCreatureDamageTaken convention.
local function lichKingEscapeDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if damage >= health then
        return true, health - 1
    end
end

-- C++ npc_the_lich_king_escape_horAI::EnterEvadeMode (:1305-1316) —
-- the ev2 twin; legs live in runLkEvadeLegs above (shared with the
-- SelectVictim poller). PORTED 2026-11-05 via creature:SetBossState
-- (garfrost precedent); the old "no bridge" claim is retracted.
local function lichKingEscapeLeaveCombat(event, creature)
    runLkEvadeLegs(creature, creature:GetGUIDLow())
end

-- C++ npc_the_lich_king_escape_horAI ctor (:1188-1195) — the AI is
-- created at summon, so the SetBossState(NOT_STARTED) leg rides ev5
-- OnSpawn (fireCreatureSpawned, lua_creature_events.go:3719; the
-- ghoul/witch-doctor/abomination ev5 precedent in
-- npc_hor_escape_trash.lua). PORTED 2026-11-05.
local function lichKingEscapeSpawn(event, creature)
    local guid = creature:GetGUIDLow()
    resetLkCounters(creature)
    creature:SetBossState(DATA_THE_LICH_KING_ESCAPE, BOSS_STATE_NOT_STARTED)
    startLkEvadePoll(creature, guid)
end

-- C++ SetData(DATA_ICEWALL) entry: the leader's SummonIceWall writes
-- the wall index into the LK's SetData store and fires the Go-only
-- DoAction signal (ACTION_LK_ICEWALL -668010, the HOR_SETDATA_NEXT_WAVE
-- pattern); the ev38 receiver reads the value back with GetData and
-- runs the machine. Other actions are ignored.
local function lichKingEscapeDoAction(event, creature, action)
    if action ~= ACTION_LK_ICEWALL then
        return
    end
    lkIcewallMachine(creature, creature:GetGUIDLow(), creature:GetData(DATA_ICEWALL))
end

-- C++ MovementInform (:1204-1231) — POINT_MOTION_TYPE only (the
-- nether drake precedent): point 1 MoveChase the escape leader;
-- point 2 Talk HARVEST_SOUL + DoCast HARVEST_SOUL 69866 on the
-- leader. The gunship EnableMovement leg stays out (no transport
-- bridge).
local function lichKingEscapeMovementInform(event, creature, moveType, pointId)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    local leader = getLeader(creature)
    if pointId == 1 then
        if leader then
            creature:MoveChase(leader)
        end
    elseif pointId == 2 then
        creature:Talk(SAY_LK_ESCAPE_HARVEST_SOUL)
        if leader then
            creature:CastSpell(leader, SPELL_HARVEST_SOUL)
        end
    end
end

-- Death teardown (the LK cannot die through the damage clamp, but a
-- forced kill must not leave timers or counters behind): machine
-- timers, poller and shared counters only — no evade legs.
local function lichKingEscapeDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelLkTimers(guid)
    cancelLkEvadePoll(guid)
    resetLkCounters(creature)
end

-- C++ npc_the_lich_king_escape_horAI::KilledUnit (:1251-1255):
-- player-gated DoPlaySoundToSet(RAND(SOUND_LK_SLAY_1 17214,
-- SOUND_LK_SLAY_2 17215)) — PlayDirectSound; math.random(17214, 17215)
-- matches RAND's two-sound pick. Event 3 is CREATURE_EVENT_ON_TARGET_DIED,
-- live in Go (fireCreatureTargetDied in engine/world/lua_creature_events.go).
local function lichKingEscapeTargetDied(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:PlayDirectSound(math.random(SOUND_LK_SLAY_1, SOUND_LK_SLAY_2))
    end
end

RegisterCreatureEvent(ENTRY_LICH_KING_ESCAPE, 2, lichKingEscapeLeaveCombat)
RegisterCreatureEvent(ENTRY_LICH_KING_ESCAPE, 3, lichKingEscapeTargetDied)
RegisterCreatureEvent(ENTRY_LICH_KING_ESCAPE, 4, lichKingEscapeDied)
RegisterCreatureEvent(ENTRY_LICH_KING_ESCAPE, 5, lichKingEscapeSpawn)
RegisterCreatureEvent(ENTRY_LICH_KING_ESCAPE, 9, lichKingEscapeDamageTaken)
RegisterCreatureEvent(ENTRY_LICH_KING_ESCAPE, 28, lichKingEscapeMovementInform)
RegisterCreatureEvent(ENTRY_LICH_KING_ESCAPE, 38, lichKingEscapeDoAction)
