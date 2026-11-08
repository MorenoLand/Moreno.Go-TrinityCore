-- Lord Marrowgar (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls-relative path
-- src/server/scripts/Northrend/IcecrownCitadel/boss_lord_marrowgar.cpp
-- (703 lines incl. license; 10 scripts — 3 CreatureScripts
-- (boss_lord_marrowgar (BossAI, DATA_LORD_MARROWGAR = 0),
-- npc_coldflame (ScriptedAI summon, SPELL_IMPALED vehicle legs),
-- npc_bone_spike (ScriptedAI trap summon, vehicle passenger legs)) +
-- 6 SpellScripts (spell_marrowgar_coldflame,
-- spell_marrowgar_coldflame_bonestorm, spell_marrowgar_coldflame_damage
-- (AuraScript), spell_marrowgar_bone_spike_graveyard,
-- spell_marrowgar_bone_storm, spell_marrowgar_bone_slice) + 1
-- AreaTriggerScript (at_lord_marrowgar_entrance); all registered from
-- inside AddSC_boss_lord_marrowgar(); loader decl 171 / call 366 per
-- northrend_script_loader.cpp — the FIRST group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_marwyn() (call 364 — closes the "// Halls of
-- Reflection" block); the call after it is
-- AddSC_boss_lady_deathwhisper() — verified from the loader this
-- run; the checkpoint sequence (boss_marwyn ->
-- boss_lord_marrowgar) is followed).
-- Entry: 36612 Lord Marrowgar (icecrown_citadel.h NPC_LORD_MARROWGAR,
-- line 169; instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_LORD_MARROWGAR, line 221) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_lord_marrowgar" hits boss_lord_marrowgar.cpp only (+
-- the loader decl/call lines); this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction. No marrowgar lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill
-- (player-gated), 4 OnDied, 23 OnReset, 24 OnReachHome. Timers via CreateLuaEvent
-- (GUID-keyed, cancelled on events 2/4/23); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady (:276).
--
-- DOCTRINE RETRACTION (2026-10-13): the earlier port of this file
-- treated the whole EventMap machine as unbridgeable under the
-- "no-timer-bridge" doctrine and ported Talk arms only. That doctrine is
-- disproved and stale — CreateLuaEvent is a real registered bridge
-- (engine/scripting/runtime.go:798), and timer-driven cast arms port on
-- the maiden_of_virtue / moroes / curator / nightbane convention:
-- timers table keyed by creature GUID, cancelled on Eluna events 2
-- (leave combat), 4 (death), 23 (reset); CastSpell(nil, spellId) =
-- victim-cast (bridge falls back to TargetGUID); CastSpell(creature,
-- spellId) = self-cast (the DoCastAOE convention, boss_ai.go DoCastAOE);
-- CastSpell(target, spellId) = explicit-target cast; TRIGGERED casts pass
-- true as the third CastSpell arg. Phase/machine gating is the real
-- blocker (the gothik precedent), not timers.
--
-- Ported arms (C++-exact for all modeled arms):
-- boss_lord_marrowgar JustEngagedWith — Talk(SAY_AGGRO 1) (event 1 —
-- the auriaya engage-port precedent; PORTED 2026-10-23:
-- DoZoneInCombat (:162, me-form) via the live bridge — C++ arm
-- order, after Talk; instance->SetBossState IN_PROGRESS (:163)
-- PORTED 2026-11-05 via creature:SetBossState (lua_creature_events.go:
-- 5917) — the old "no bridge" claim is retracted. The
-- me->setActive(true) leg and the five ScheduleEvent legs have no
-- bridge (only EVENT_ENRAGE ports, below).
-- boss_lord_marrowgar KilledUnit — Talk(SAY_KILL 4) player-gated
-- (event 3 — the razuvious player-gated variant precedent; the
-- twelfth ported variant, tenth identical to
-- nalorakk/kelthuzad/gothik/thaddius/garfrost/krick/tyrannus/falric/
-- marwyn).
-- boss_lord_marrowgar JustDied — Talk(SAY_DEATH 5) (event 4 — the
-- sjonnir JustDied-Talk precedent); the _JustDied() DONE leg (:170)
-- PORTED 2026-11-05 via creature:SetBossState, in C++ order AFTER
-- Talk. Melee engine-driven.
-- boss_lord_marrowgar UpdateAI EVENT_ENRAGE (:261-263) — DoCast(me,
-- SPELL_BERSERK 26662, true) + Talk(SAY_BERSERK 6), cast-before-Talk
-- kept; 10min one-shot seeded on engage, no phase/machine/random-
-- target gates (the festergut/rotface/putricide/sindragosa berserk
-- precedent; TRIGGERED self-cast).
-- DOCUMENTED-ONLY (in this header; entries 36612 + 36619 registered):
-- npc_coldflame (36672) exposes zero own Talk lines; its IsSummonedBy
-- legs are cross-AI position (GetLastColdflamePosition /
-- DATA_COLDFLAME_GUID player lookup) + GetNearPoint2D — Teleport and
-- DoCast ARE bridged but the computed position is not — plus the
-- UpdateAI EVENT_COLDFLAME_TRIGGER machine; not registered (the
-- bronjahm npc_corrupted_soul_fragment precedent; ev22 re-checked
-- 2026-11-05, no-port stands — GetAbsoluteAngle / GetNearPoint2D /
-- GetPosition absent from the creature surface, only GetZ exists).
-- The vehicle legs
-- (RIDE_VEHICLE cast + PassengerBoarded spline) belong to bone_spike's
-- IsSummonedBy below, not to coldflame. npc_bone_spike (36619): the JustDied/KilledUnit
-- DespawnOrUnsummon legs PORT (event 4 / event 3, below); the JustDied
-- trapped-unit aura-removal leg PORTS via creature:GetSummonerUnit()
-- (bridged 2026-11-05, below); the IsSummonedBy DoCast(summoner,
-- SPELL_IMPALED) + summoner->CastSpell(me, SPELL_RIDE_VEHICLE, true)
-- legs PORT (event 22, below — player:CastSpell live, lua_player.go:
-- 2752); the 8s EVENT_FAIL_BONED timer leg PORTS via the shared
-- instanceData bonedEligible key (below, Go 12775/12962/13393/13394
-- read it); PassengerBoarded spline leg; UpdateAI machine)
-- has no bridges.
-- boss_lord_marrowgar Reset scheduler legs (:148-152) — only
-- EVENT_ENRAGE ports (above); EVENT_ENABLE_BONE_SLICE /
-- EVENT_BONE_SPIKE_GRAVEYARD / EVENT_COLDFLAME / EVENT_WARN_BONE_STORM
-- stay out (see UpdateAI). The RemoveAurasDueToSpell(SPELL_BONE_STORM
-- 69076 / SPELL_BERSERK 26662) legs (:145-146) PORT (creature:RemoveAura
-- on event 23, C++ order kept); the SetSpeedRate legs ride the
-- unbridged bone-storm motion machine (3x speed :226 accompanies the
-- unported MovePoint chase; the :252 _baseSpeed restore rides the
-- unported MoveChase return) — stays out, verdict unchanged (the
-- old "no SetSpeedRate bridge" claim is stale: the bridge is live,
-- lua_creature_events.go:735 — the motion machine is the blocker).
-- boss_lord_marrowgar UpdateAI — the EventMap machine: arms with
-- unbridged phase/machine legs stay out (the gothik precedent).
-- EVENT_WARN_BONE_STORM legs carry Talk(EMOTE_BONE_STORM 7)
-- and the SPELL_BONE_STORM self-cast but enter the bone-storm phase
-- machine (DelayEvents, schedules BEGIN, speed/move legs);
-- EVENT_BONE_STORM_BEGIN carries
-- Talk(SAY_BONE_STORM 2), aura-duration and 3x speed legs;
-- EVENT_BONE_STORM_MOVE carries random-NonTank SelectTarget +
-- MovePoint legs (no-random-target-SelectTarget / no-motion
-- queues); EVENT_BONE_STORM_END carries movement removal,
-- MoveChase and event-reschedule legs; EVENT_BONE_SPIKE_GRAVEYARD /
-- EVENT_COLDFLAME carry DoCast / DoCastAOE legs but both summon
-- npc_bone_spike / npc_coldflame (vehicle/passenger/spline machines,
-- no summon bridge); EVENT_ENRAGE is PORTED with its pre-loop
-- UNIT_STATE_CASTING gate (:193, 500ms IsCasting defer — the old
-- "ungated one-shot / gate has no bridge" claims are retracted,
-- IsCasting live at lua_creature_events.go:1950);
-- bone-storm-aura gate, bone-slice DoCastVictim legs and
-- DoMeleeAttackIfReady have no bridge. JustReachedHome
-- instance-SetBossState(FAIL) (:175) PORTED 2026-11-05 via
-- creature:SetBossState on ev24 OnReachHome (the opera ev24
-- precedent); the DATA_BONED_ACHIEVEMENT-reset leg PORTS via the
-- shared instanceData bonedEligible key (marrowgarReset, below).
-- DoAction(ACTION_CLEAR_SPIKE_IMMUNITIES) and MovementInform have
-- no bridge.
-- Talk(SAY_ENTER_ZONE 0) rides DoAction(ACTION_TALK_ENTER_ZONE),
-- which only fires from at_lord_marrowgar_entrance — ev38 dispatch bridged;
-- sender area-trigger TryHandleOnce (boss_lord_marrowgar.cpp:675-681) unported,
-- no area-trigger bridge (krick relay N/A — no ported sender hook); the area trigger joins
-- the unbridged-area-trigger queue (the pit_of_saron cavern-triggers
-- precedent).
-- Talk(SAY_BONESPIKE 3) is called by marrowgarAI->Talk from inside
-- spell_marrowgar_bone_spike_graveyard SpellScript HandleSpikes —
-- no-SpellScript bridge (the boss_moragg optic-link precedent);
-- spell_marrowgar_coldflame / spell_marrowgar_coldflame_bonestorm /
-- spell_marrowgar_bone_storm / spell_marrowgar_bone_slice join the
-- no-SpellScript-bridge queue, and spell_marrowgar_coldflame_damage
-- joins the no-AuraScript-bridge queue.
-- All 8 Talk() calls in the file accounted for (8 = ported: aggro 1
-- + kill 4 + death 5 + berserk 6; documented-only: enter-zone 0 +
-- bone-storm 2 + bonespike 3 + emote-bone-storm 7).

local ENTRY_LORD_MARROWGAR = 36612
local ENTRY_BONE_SPIKE = 36619 -- icecrown_citadel.h:171

local DATA_LORD_MARROWGAR = 0 -- icecrown_citadel.h:74
local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3

local SPELL_IMPALED = 69065 -- boss_lord_marrowgar.cpp:54
local SPELL_RIDE_VEHICLE = 46598 -- boss_lord_marrowgar.cpp:55

local SAY_AGGRO = 1
local SAY_KILL = 4
local SAY_DEATH = 5
local SAY_BERSERK = 6

local SPELL_BERSERK = 26662 -- icecrown_citadel.h:33
local SPELL_BONE_STORM = 69076 -- boss_lord_marrowgar.cpp spells enum

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

-- C++ :261-263 EVENT_ENRAGE: DoCast(me, SPELL_BERSERK, true) +
-- Talk(SAY_BERSERK 6) — 10min one-shot, no phase/machine/random-target
-- gates (the festergut/rotface/putricide/sindragosa berserk precedent);
-- TRIGGERED self-cast, cast-before-Talk kept.
local function onEnrage(creature, guid)
    -- C++ UpdateAI pre-loop gate :193 sits BEFORE ExecuteEvent (:196),
    -- so a due enrage stays queued while casting — 500ms IsCasting
    -- defer, no re-arm consumed (the mal_ganis idiom; the in-loop
    -- gate :267 covers subsequent queued events in the same C++ tick,
    -- nothing to port for this single Lua tick).
    if creature:IsCasting() then
        schedule(guid, "enrage", 500, function() onEnrage(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_BERSERK, true)
    creature:Talk(SAY_BERSERK)
end

-- C++ boss_lord_marrowgar::JustEngagedWith: Talk(SAY_AGGRO) — the
-- auriaya engage-port precedent. The EventMap seeds (:148-152) —
-- only EVENT_ENRAGE ports (the rest are summon/phase-machine legs).
local function marrowgarJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    -- C++ :162 — me-form DoZoneInCombat after Talk(SAY_AGGRO) and
    -- me->setActive(true) (C++ arm order); :163 SetBossState
    -- IN_PROGRESS PORTED 2026-11-05 via creature:SetBossState
    -- (lua_creature_events.go:5917).
    creature:DoZoneInCombat()
    creature:SetBossState(DATA_LORD_MARROWGAR, BOSS_STATE_IN_PROGRESS)
    schedule(guid, "enrage", 600000, function() onEnrage(creature, guid) end)
end

-- C++ Reset() event machinery — cancel/re-arm on combat events (the
-- maiden convention).
local function marrowgarLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function marrowgarReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ Reset() :144 — _Reset() first: SetBossState NOT_STARTED
    -- only when the stored state != DONE (BossAI::_Reset,
    -- ScriptedCreature.cpp:499-509; the falric precedent — ev23
    -- fires on spawn/death/evade too, and a DONE boss must stay
    -- DONE). PORTED 2026-11-05 via creature:SetBossState.
    if creature:GetBossState(DATA_LORD_MARROWGAR) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_LORD_MARROWGAR, BOSS_STATE_NOT_STARTED)
    end
    -- C++ Reset() :144-146: SetSpeedRate(MOVE_RUN, _baseSpeed) restores the
    -- template rate (_baseSpeed is GetSpeedRate(MOVE_RUN) captured in the AI
    -- constructor); rate <= 0 clears the script override, restoring the
    -- template rate. Then RemoveAurasDueToSpell(SPELL_BONE_STORM) and
    -- RemoveAurasDueToSpell(SPELL_BERSERK) — order kept (RemoveAura bridge,
    -- boss_lady_deathwhisper.lua precedent). Without this an evade during
    -- bone storm re-seeds the storm phase with the aura still on.
    creature:SetSpeedRate(1, 0)
    creature:RemoveAura(SPELL_BONE_STORM)
    creature:RemoveAura(SPELL_BERSERK)
    -- C++ Reset() :177: instance->SetData(DATA_BONED_ACHIEVEMENT 14,
    -- true) — re-arms the Boned eligibility latch on every reset
    -- (wipe or evade); the Go 12775/12962/13393/13394 check reads
    -- bonedEligible from the shared instanceData table (default 1,
    -- the C++ ctor :156 value), cleared by the bone-spike 8s fail
    -- timer below.
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.bonedEligible = true
        end
    end
end

-- C++ boss_lord_marrowgar::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function marrowgarKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_lord_marrowgar::JustDied: Talk(SAY_DEATH) — the sjonnir
-- JustDied-Talk precedent; _JustDied() (:170) PORTED 2026-11-05 via
-- creature:SetBossState DONE, in C++ order AFTER Talk. cancelTimers
-- stops the 10-min berserk timer from firing on the corpse.
local function marrowgarJustDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
    creature:SetBossState(DATA_LORD_MARROWGAR, BOSS_STATE_DONE)
end

-- C++ boss_lord_marrowgar::JustReachedHome (:174-177): _JustReachedHome()
-- (setActive(false), unmodeled) then SetBossState(FAIL) PORTED
-- 2026-11-05 via creature:SetBossState on ev24 OnReachHome
-- (creaturemotion.go home-move finalize, the opera ev24 precedent).
-- The SetData(DATA_BONED_ACHIEVEMENT 14, true) reset leg PORTS via
-- the shared instanceData bonedEligible key (marrowgarReset, above).
local function marrowgarReachHome(event, creature)
    creature:SetBossState(DATA_LORD_MARROWGAR, BOSS_STATE_FAIL)
end

RegisterCreatureEvent(ENTRY_LORD_MARROWGAR, 1, marrowgarJustEngagedWith)
RegisterCreatureEvent(ENTRY_LORD_MARROWGAR, 2, marrowgarLeaveCombat)
RegisterCreatureEvent(ENTRY_LORD_MARROWGAR, 3, marrowgarKilledUnit)
RegisterCreatureEvent(ENTRY_LORD_MARROWGAR, 4, marrowgarJustDied)
RegisterCreatureEvent(ENTRY_LORD_MARROWGAR, 23, marrowgarReset)
RegisterCreatureEvent(ENTRY_LORD_MARROWGAR, 24, marrowgarReachHome)

-- C++ npc_bone_spike::JustDied (boss_lord_marrowgar.cpp:424-431): the
-- trapped-unit aura-removal leg rides ToTempSummon()->GetSummonerUnit()
-- (creature:GetSummonerUnit, bridged 2026-11-05 — TemporarySummon.cpp:
-- 48-53 over the summoner's CreatedByGUID; the summoner is the trapped
-- player); me->DespawnOrUnsummon() ports (creature:Despawn, bridged).
-- C++ order kept: aura removal, then despawn.
local function boneSpikeJustDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    local summoner = creature:GetSummonerUnit()
    if summoner then
        summoner:RemoveAura(SPELL_IMPALED)
    end
    creature:Despawn()
end

-- C++ npc_bone_spike::KilledUnit (:433-437): me->DespawnOrUnsummon() +
-- victim->RemoveAurasDueToSpell(SPELL_IMPALED), C++ order kept;
-- victim:RemoveAura is the player/creature RemoveAura bridge.
local function boneSpikeKilledUnit(event, creature, victim)
    cancelTimers(creature:GetGUID())
    creature:Despawn()
    victim:RemoveAura(SPELL_IMPALED)
end

-- C++ npc_bone_spike::IsSummonedBy (boss_lord_marrowgar.cpp:439-447):
-- DoCast(summoner, SPELL_IMPALED 69065) + summoner->CastSpell(me,
-- SPELL_RIDE_VEHICLE 46598, true); the summoner is the trapped player
-- (the graveyard SpellScript has the target cast the summon spell on
-- itself, :590) and ev22 passes it as the second arg (the
-- summonTempCreature player-summoner fire site,
-- lua_creature_events.go:200-201; player:CastSpell live at
-- lua_player.go:2752, inside luaPlayer()). C++ order kept: impaled,
-- then ride-vehicle (triggered). The 8s EVENT_FAIL_BONED timer PORTS
-- (:444-445 schedule, :470-475 fire while a unit is trapped, only
-- effect instance SetData(DATA_BONED_ACHIEVEMENT 14, false) — the
-- shared instanceData bonedEligible key is the Go-side twin; the
-- timer is cancelled on spike death/kill above because in C++ the
-- EventMap dies with the spike AI. Dormant until the graveyard
-- summon chain lands: no 69057/69062 handling in Go and
-- SPELL_EFFECT_SUMMON has no session-path executor yet.
local function boneSpikeIsSummonedBy(event, creature, summoner)
    if not summoner then
        return
    end
    creature:CastSpell(summoner, SPELL_IMPALED)
    summoner:CastSpell(creature, SPELL_RIDE_VEHICLE, true)
    schedule(creature:GetGUID(), "failBoned", 8000, function()
        local mapObj = creature:GetMap()
        if mapObj then
            local data = mapObj:GetInstanceData()
            if data then
                data.bonedEligible = false
            end
        end
    end)
end

RegisterCreatureEvent(ENTRY_BONE_SPIKE, 3, boneSpikeKilledUnit)
RegisterCreatureEvent(ENTRY_BONE_SPIKE, 4, boneSpikeJustDied)
RegisterCreatureEvent(ENTRY_BONE_SPIKE, 22, boneSpikeIsSummonedBy)
