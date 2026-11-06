-- King Ymiron (Utgarde Pinnacle) — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardePinnacle/boss_ymiron.cpp
-- (355 lines; 3 scripts — boss_ymiron (CreatureScript via
-- GetUtgardePinnacleAI (BossAI), DATA_KING_YMIRON = 3);
-- spell_dark_slash (bare SpellScript via RegisterSpellScript, SpellScript
-- OnHit -> CalculateDamage half-target-health); achievement_kings_bane
-- (AchievementCriteriaScript, OnCheck GetData(DATA_KINGS_BANE));
-- all registered from inside AddSC_boss_ymiron(); loader decl 135 /
-- call 330 per northrend_script_loader.cpp — the FOURTH group of the "//
-- Utgarde Keep - Utgarde Pinnacle" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_skadi() (call 329); the checkpoint
-- sequence (boss_skadi -> boss_ymiron) is followed).
-- Entries: 26861 King Ymiron (utgarde_pinnacle.h NPC_KING_YMIRON,
-- line 56) — entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- DATA_KING_YMIRON = 3 (utgarde_pinnacle.h line 34).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_ymiron"
-- hits boss_ymiron.cpp only (+ the loader decl/call lines); this clone
-- carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction. The lua was ported Oct 2 (Talk-only, stale
-- "no-timer-bridge" doctrine) and UPGRADED this run under the
-- CreateLuaEvent doctrine (engine/scripting/runtime.go:798 — the
-- maiden_of_virtue / keleseth / yogg_saron precedent): module-local
-- GUID-keyed timers table, wipe on events 2 (leave combat), 4 (death),
-- 23 (evade). creature:CastSpell(nil, spell) = DoCastVictim;
-- creature:CastSpell(creature, spell) = DoCastSelf / DoCastAOE (the
-- selin_fireheart / mr_smite / boss_ai.go DoCastAOE lowering precedent);
-- the 3rd CastSpell arg is the C++ DoCast 3rd arg (2-arg / 1-arg
-- overloads = non-triggered); {minMs, maxMs} table delay = randtime
-- range. The 2026-10-02 "no-timer-bridge" header doctrine is RETRACTED
-- for this file: CreateLuaEvent is a real registered bridge, so the
-- ungated timer arms below are ported rather than parked.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDied, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_ymiron.cpp):
-- JustEngagedWith (:141-149, base-first) — BossAI::JustEngagedWith
-- arms SetBossState(DATA_KING_YMIRON 3, IN_PROGRESS) PORTED tick 261 via
-- creature:SetBossState (lua_creature_events.go:5917) + SetCombatPulseDelay(5)
-- + me-form DoZoneInCombat (ScriptedCreature.cpp:537/540/542) ported
-- before Talk(SAY_AGGRO 0) (event 1; instance/setActive/ScheduleTasks
-- legs out) — plus the four EVENT_GROUP_BASE_SPELLS timer inits
-- carried on event 1 on the xt002 precedent (C++ schedules on
-- JustEngagedWith, Go on event 1, same inits). All four arms run
-- under BossAI::UpdateAI base gates (ScriptedCreature.cpp pre-loop
-- + in-loop CASTING returns; no UpdateAI override in the file), so
-- all four carry the 500ms IsCasting defer (no phase conditions on
-- the base spells; the event-group delay legs ride the unbridged
-- boat machine — see DEVIATION):
--   EVENT_BANE (arm :236-238): DoCast(SPELL_BANE 48294), 1-arg ->
--   DoCastAOE/self-cast -> CastSpell(creature, 48294), non-triggered;
--   init {18s,23s}, re-arm {20s,25s}.
--   EVENT_FETID_ROT (arm :240-242): DoCastVictim(SPELL_FETID_ROT 48291),
--   1-arg = non-triggered -> CastSpell(nil, 48291); init {8s,13s},
--   re-arm {10s,15s}.
--   EVENT_DARK_SLASH (arm :244-246): DoCastVictim(SPELL_DARK_SLASH
--   48292), 1-arg = non-triggered -> CastSpell(nil, 48292); init
--   {28s,33s}, re-arm {30s,35s}.
--   EVENT_ANCESTORS_VENGEANCE (arm :248-250): DoCast(me,
--   SPELL_ANCESTORS_VENGEANCE 16939), 2-arg = non-triggered ->
--   CastSpell(creature, 16939); init DUNGEON_MODE(60s,45s), re-arm
--   DUNGEON_MODE({60s,65s},{45s,50s}) — difficulty-branched via
--   creature:IsHeroic() (lua_creature_events.go:260, per-instance
--   difficulty; the old "no difficulty bridge" claim was stale).
-- JustDied (:277) — _JustDied() SetBossState(DATA_KING_YMIRON 3, DONE)
-- PORTED tick 261 via creature:SetBossState (lua_creature_events.go:5917),
-- first per C++ :279; Talk(SAY_DEATH 2) — unconditional (the sjonnir
-- JustDied-Talk precedent; the events.Reset / scheduler.CancelAll legs
-- are internal).
-- Reset (:134-141) — _Reset() SetBossState(DATA_KING_YMIRON 3, NOT_STARTED)
-- unless DONE + SetReactState(REACT_AGGRESSIVE) PORTED tick 261 on a NEW
-- ev23 handler split from ev2 (the sjonnir precedent); Initialize() legs
-- have no Lua mirrors (see ymironReset).
-- KilledUnit (:283) — Talk(SAY_SLAY 1) player-gated (event 3 — the
-- razuvious player-gated variant precedent).
-- DOCUMENTED DEVIATION: the ancestor phase machine is unbridged — the
-- DamageTaken health-threshold latch (100 - DUNGEON_MODE(20,25) *
-- HealthAmountModifier pct, :208-230): InterruptNonMeleeSpells +
-- DoCast(me, SPELL_SCREAMS_OF_THE_DEAD 51750) + AttackStop +
-- REACT_PASSIVE + MovePoint(POINT_BOAT) + boat-ghost despawns +
-- events.CancelEvent(prev ancestor event) +
-- events.DelayEvents(10s, EVENT_GROUP_BASE_SPELLS) have no MotionMaster /
-- MovementInform bridge (the health-threshold latch itself is bridgeable
-- via event 9 + GetHealthPct — boss_nethekurse.lua uses GetHealthPct()
-- <= 20 at event 9 — but the blocker is the MovePoint(POINT_BOAT) leg,
-- :218 (InterruptNonMeleeSpells / SetReactState are bridged;
-- the blocker is the MovePoint leg — Despawn IS bridged, but the arm is
-- DespawnBoatGhosts — it despawns OTHER creatures, not
-- the handler; calling creature:Despawn() on Ymiron would
-- target the wrong creature); the MovementInform(POINT_BOAT) ->
-- Talk(summon say 3-6) -> SummonCreature(ancestor visual) ->
-- DoCast(ancestor, SPELL_CHANNEL_YMIRON_TO_SPIRIT 48307) +
-- ancestor->CastSpell(me, SPELL_CHANNEL_SPIRIT_TO_YMIRON 48316, true) ->
-- EVENT_RESUME_COMBAT (REACT_AGGRESSIVE + ancestor event 5s later,
-- :252-255; MovementInform :165-184, DamageTaken latch :208-230)
-- chain has no bridges — so timers keep their combat-start
-- schedules through the ancestor phases (the assembly_of_iron
-- DoAction-reschedule deviation precedent).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 26861):
-- The four ancestor events — EVENT_BJORN_SPIRIT_FOUNT
-- (SPELL_SUMMON_SPIRIT_FOUNT 48386), EVENT_HALDOR_SPIRIT_STRIKE
-- (DoCastVictim SPELL_SPIRIT_STRIKE 48423, 5s), EVENT_RANULF_SPIRIT_
-- BURST (DoCast(me, SPELL_SPIRIT_BURST 48529), 10s), EVENT_TORGYN_
-- SUMMON_AVENGING_SPIRITS (4x DoCast(SPELL_SUMMON_AVENGING_SPIRIT
-- 48592), 15s) (:256-271) — fire only after the unbridged
-- MovementInform/EVENT_RESUME_COMBAT machine (the gothik precedent).
-- Stay out.
-- Talk(SAY_SUMMON_BJORN 3 / SAY_SUMMON_HALDOR 4 / SAY_SUMMON_RANULF 5 /
-- SAY_SUMMON_TORGYN 6) ride the MovementInform POINT_BOAT leg, fused
-- with the ancestor summon — no MovementInform / summon bridges (the
-- zarithrian Talk-fused-with-summon precedent). Stay out.
-- The JustSummoned legs (NPC_SPIRIT_FOUNT 48380 self-cast + SetDisplayId
-- 11686; NPC_AVENGING_SPIRIT random-target AddThreat/AttackStart) stay out —
-- the methods ARE bridged (SetDisplayId lua_creature_events.go:4170,
-- AddThreat :2058, AttackStart :6125, tick-261 re-check) but the legs can
-- never fire in the Lua model: their trigger summons come from the ancestor
-- events, which ride the unbridged boat machine. A JustSummoned handler
-- would be dead code.
-- spell_dark_slash — SpellScript OnHit damage hook only; joins the
-- no-SpellScript-bridge queue.
-- The kingsBane SpellHitTarget latch (:153-157): if (target->GetTypeId()
-- == TYPEID_PLAYER && spellInfo->Id == SPELL_BANE_HIT 59203) kingsBane =
-- false — pure achievement bookkeeping feeding GetData(DATA_KINGS_BANE
-- 2157). GetData/SetData exist as a per-motion ScriptData store
-- (lua_creature_events.go:771-800, tick-261 re-check) and event 15 fires
-- (boss_ai.go:4003), but the achievement OnCheck consumer has no Lua hook
-- and nothing reads the latch — a write-only local would be dead code.
-- Documented only.
-- achievement_kings_bane (OnCheck: GetData(DATA_KINGS_BANE 2157) == 1) —
-- the achievement OnCheck consumer has no Lua hook (GetData itself exists
-- as a ScriptData store, lua_creature_events.go:771-800); joins the
-- unmodeled-achievement queue.

local ENTRY_KING_YMIRON = 26861
local DATA_KING_YMIRON = 3 -- utgarde_pinnacle.h:34

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local SPELL_BANE = 48294
local SPELL_FETID_ROT = 48291
local SPELL_DARK_SLASH = 48292
local SPELL_ANCESTORS_VENGEANCE = 16939

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

-- Shared timer wipe for events 2/23/4.
local function cancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ EVENT_BANE arm (:236-238): DoCast(SPELL_BANE 48294), 1-arg =
-- non-triggered -> self-cast; init {18s,23s}, re-arm {20s,25s}.
local function baneTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "bane", 500, function()
            baneTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_BANE)
    schedule(guid, "bane", {20000, 25000}, function()
        baneTick(creature, guid)
    end)
end

-- C++ EVENT_FETID_ROT arm (:240-242): DoCastVictim(SPELL_FETID_ROT
-- 48291), 1-arg = non-triggered; init {8s,13s}, re-arm {10s,15s}.
local function fetidRotTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "fetidrot", 500, function()
            fetidRotTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_FETID_ROT)
    schedule(guid, "fetidrot", {10000, 15000}, function()
        fetidRotTick(creature, guid)
    end)
end

-- C++ EVENT_DARK_SLASH arm (:244-246): DoCastVictim(SPELL_DARK_SLASH
-- 48292), 1-arg = non-triggered; init {28s,33s}, re-arm {30s,35s}.
local function darkSlashTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "darkslash", 500, function()
            darkSlashTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_DARK_SLASH)
    schedule(guid, "darkslash", {30000, 35000}, function()
        darkSlashTick(creature, guid)
    end)
end

-- C++ EVENT_ANCESTORS_VENGEANCE arm (:248-250): DoCast(me,
-- SPELL_ANCESTORS_VENGEANCE 16939), 2-arg = non-triggered; init
-- DUNGEON_MODE(60s,45s), re-arm DUNGEON_MODE({60s,65s},{45s,50s}),
-- branched on creature:IsHeroic().
local function ancestorsVengeanceTick(creature, guid, heroic)
    if creature:IsCasting() then
        schedule(guid, "ancestorsvengeance", 500, function()
            ancestorsVengeanceTick(creature, guid, heroic)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_ANCESTORS_VENGEANCE)
    local delay = heroic and {45000, 50000} or {60000, 65000}
    schedule(guid, "ancestorsvengeance", delay, function()
        ancestorsVengeanceTick(creature, guid, heroic)
    end)
end

-- C++ boss_ymironAI::JustEngagedWith (:141-149): BossAI::JustEngagedWith
-- base-first (pulse + me-DoZone ported above Talk; instance/setActive
-- legs out); Talk(SAY_AGGRO) — unconditional (the auriaya engage-port
-- precedent); the four base-spell schedule inits are carried here on
-- the xt002 precedent.
local function ymironEnterCombat(event, creature, target)
    -- BossAI::JustEngagedWith (:143, base-first) — SetBossState(DATA_KING_YMIRON,
    -- IN_PROGRESS) PORTED tick 261 via creature:SetBossState
    -- (lua_creature_events.go:5917), first per BossAI::_JustEngagedWith
    -- (ScriptedCreature.cpp:537), before SetCombatPulseDelay(5)
    -- + me-form DoZoneInCombat (ScriptedCreature.cpp:540/542).
    creature:SetBossState(DATA_KING_YMIRON, 1)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    local heroic = creature:IsHeroic()
    cancelTimers(guid)
    schedule(guid, "bane", {18000, 23000}, function()
        baneTick(creature, guid)
    end)
    schedule(guid, "fetidrot", {8000, 13000}, function()
        fetidRotTick(creature, guid)
    end)
    schedule(guid, "darkslash", {28000, 33000}, function()
        darkSlashTick(creature, guid)
    end)
    schedule(guid, "ancestorsvengeance", heroic and 45000 or 60000, function()
        ancestorsVengeanceTick(creature, guid, heroic)
    end)
end

-- C++ KilledUnit (:274-278): if (who->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function ymironKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied (:277-280): _JustDied() (SetBossState DONE,
-- ScriptedCreature.cpp:519) then Talk (:280) — the DONE leg PORTED
-- tick 261 via creature:SetBossState (lua_creature_events.go:5917);
-- Talk(SAY_DEATH) — unconditional — the sjonnir JustDied-Talk precedent.
local function ymironJustDied(event, creature)
    creature:SetBossState(DATA_KING_YMIRON, 3) -- DONE
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_ymironAI::Reset (:134-141): _Reset() — SetBossState(DATA_KING_YMIRON 3,
-- NOT_STARTED) unless DONE (BossAI::_Reset, ScriptedCreature.cpp:509-510) PORTED
-- tick 261 via creature:SetBossState (lua_creature_events.go:5917), gated on
-- GetBossState (lua_creature_events.go:5936); the IsDead guard mirrors _Reset's
-- !IsAlive return (:501) and covers the ev23 death-path pre-fire (23 then 4);
-- then SetReactState(REACT_AGGRESSIVE) :140. Split from ev2 this tick (the
-- sjonnir precedent): ev2 leave-combat does not call Reset in C++, so it stays
-- cancel-only. Initialize() legs (kingsBane / ActivedNumber /
-- HealthAmountModifier / GUID clears) have no Lua mirrors — the kingsBane latch
-- is documented-only (see header) and the boat-machine counters ride the
-- unbridged machine.
local function ymironReset(event, creature)
    cancelTimers(creature:GetGUID())
    if creature:IsDead() then
        return
    end
    if creature:GetBossState(DATA_KING_YMIRON) ~= 3 then
        creature:SetBossState(DATA_KING_YMIRON, 0)
    end
    creature:SetReactState(2) -- REACT_AGGRESSIVE
end

RegisterCreatureEvent(ENTRY_KING_YMIRON, 1, ymironEnterCombat)
RegisterCreatureEvent(ENTRY_KING_YMIRON, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_KING_YMIRON, 3, ymironKilledUnit)
RegisterCreatureEvent(ENTRY_KING_YMIRON, 4, ymironJustDied)
RegisterCreatureEvent(ENTRY_KING_YMIRON, 23, ymironReset)
