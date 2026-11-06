-- Ignis the Furnace Master (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_ignis.cpp
-- (boss_ignis (CreatureScript) via GetUlduarAI<boss_ignis_AI>
-- (BossAI), BOSS_IGNIS = 1 — registered from inside
-- AddSC_boss_ignis(); loader decl 112 / call 307 per
-- northrend_script_loader.cpp — the THIRD group of the "// Ulduar"
-- block in AddNorthrendScripts(), immediately after
-- AddSC_boss_flame_leviathan() (decl 111 / call 306); the call
-- after it is AddSC_boss_razorscale() (decl 113 / call 308) —
-- loader order confirmed this run; the checkpoint sequence
-- (boss_flame_leviathan -> boss_ignis) is followed).
-- Entry: 33118 Ignis (ulduar.h NPC_IGNIS 33118, line 62;
-- BOSS_IGNIS = 1, line 37) — entry-verifiable, registration
-- proceeds (the nexus_commanders / malygos / sartharion kalecgos
-- precedent); the CreatureScript ScriptName binding is DB-side as
-- usual.
-- Sole-source verified: whole-server-tree grep for "boss_ignis"
-- hits boss_ignis.cpp (+ the loader decl/call lines for
-- AddSC_boss_ignis) only; whole-tree grep for
-- "npc_iron_construct", "npc_scorch_ground",
-- "spell_ignis_slag_pot" and "achievement_ignis_shattered"
-- hits boss_ignis.cpp only; zero sql/ hits for all five —
-- the audited artifact this run. No second ignis lua exists.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timer arms ride the
-- CreateLuaEvent convention (sartharion / baltharus / maiden
-- precedent): GUID-keyed timers table, cancel on events 2/4/23;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (:147, event 1) + the
-- BossAI::JustEngagedWith base arms ported into ignisEnterCombat:
-- SetBossState(DATA_IGNIS 1, IN_PROGRESS) PORTED 2026-11-05 via
-- creature:SetBossState (lua_creature_events.go:5917; the maiden
-- precedent) — the "no instance bridge" claim for the passthrough was
-- stale — plus SetCombatPulseDelay(5) + me-form DoZoneInCombat, base-first
-- before Talk. The six ScheduleEvent legs: JET/SCORCH/CONSTRUCT/BERSERK
-- scheduled below (CONSTRUCT ported 2026-11-05, see below); SLAG_POT +
-- END_POT stay out — the slag vehicle chain has no bridge. The
-- DoStartTimedAchievement leg has no timed-achievement bridge. The
-- Oct-12/13 "no timer bridge" doctrine is STALE: CreateLuaEvent
-- (engine/scripting/runtime.go:798) is the timer bridge, already used below.
-- EVENT_JET — Talk(EMOTE_JETS 7) (:222) + DoCast(me,
-- SPELL_FLAME_JETS 62680) 2-arg non-triggered self-cast (:223);
-- 30s init (:148), re-arm randtime 35-40s (:224).
-- EVENT_BERSERK — DoCast(me, SPELL_BERSERK 47008, true) 3-arg
-- triggered self-cast (:278) + Talk(SAY_BERSERK 5) (:279); 480s
-- init (:153), one-shot (header fix this run: C++ is triggered,
-- the old header omitted the flag).
-- KilledUnit — player-gated Talk(SAY_SLAY 4) (:175, event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent).
-- JustDied — Talk(SAY_DEATH 6) (:161, event 4) + _JustDied()
-- SetBossState(DATA_IGNIS 1, DONE) PORTED 2026-11-05 via
-- creature:SetBossState (lua_creature_events.go:5917; the maiden
-- precedent), base-first before Talk — the "no bridge" claim for the
-- _JustDied() passthrough was stale.
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 33118):
-- Reset / Initialize — _Reset() SetBossState(DATA_IGNIS 1, NOT_STARTED)
-- PORTED 2026-11-05 via creature:SetBossState (lua_creature_events.go:5917;
-- the maiden precedent) into ignisReset (event 23); vehicle passenger
-- removal (no vehicle bridge) + DoStopTimedAchievement(
-- ACHIEVEMENT_IGNIS_START_EVENT 20951) (no timed-achievement bridge) stay
-- out. Initialize() only clears Lua-absent state (_slagPotGUID / _shattered /
-- _firstConstructKill) — nothing to port.
-- GetData(DATA_SHATTERED 29252926) — creature:GetData IS bridged
-- (ScriptData store, 2026-10-24) but the leg stays out: the producer
-- is the DoAction(20) cross-AI relay (dispatch IS bridged — ev38 —
-- but the sender is the unregistered iron construct's DamageTaken,
-- boss_ignis.cpp:333) and the
-- sole consumer is achievement_ignis_shattered below (no achievement
-- bridge), so a Lua-side latch would be dead code.
-- JustSummoned — PORTED 2026-11-03 (ignisJustSummoned, event 19):
-- construct setup (SetFaction/SetReactState/RemoveFlag x4) +
-- EngageWithTarget(victim) (the AttackStart twin) + DoZoneInCombat +
-- summons latch, despawned from died (before Talk)/reset/leave.
-- Deltas: SetImmuneToPC has no bridge and stays out; SetControlled(false,
-- ROOT) is PORTED below as SetRooted(false) (bridged, tick 236);
-- constructs now spawn live via the ported EVENT_CONSTRUCT arm
-- (constructTick, summon type 3), as does the scorch ground via the ported
-- EVENT_SCORCH summon site.
-- DoAction(ACTION_REMOVE_BUFF 20) — RemoveAuraFromStack(SPELL_
-- STRENGHT 64473) + the GameTime <5s shattered window (producer:
-- npc_iron_construct DamageTaken, boss_ignis.cpp:333 — the construct
-- is unregistered, so no Lua sender exists; dispatch itself IS
-- bridged, ev38) — no aura-stack bridge.
-- EVENT_SLAG_POT (:227-:236) — Talk(SAY_SLAG_POT 2) +
-- SelectTarget(Random,1,100,true) + DoCast(target, SPELL_GRAB
-- 62707) 2-arg non-triggered + DelayEvents(3s) -> EVENT_GRAB_POT
-- (EnterVehicle seat 0, 500ms) -> EVENT_CHANGE_POT (DoCast
-- SPELL_SLAG_POT 62717 triggered + seat swap, 1s) -> EVENT_END_POT
-- (ExitVehicle, 10s); 35s init, re-arm RAID_MODE(30s, 15s) —
-- random-target-search is bridgeable via the GetPlayersInWorld idiom (6
-- northrend precedents: gal_darah, eck, elder_nadox, eadric, lord_jaraxxus,
-- nexus_commanders) — the vehicle (EnterVehicle/ExitVehicle) legs have no
-- bridge: arm blocked.
-- EVENT_SCORCH (:262-:268) — PORTED 2026-10-23 (scorchTick, event 1;
-- 25s init / 25s re-arm): Talk(SAY_SCORCH 3) + SummonCreature(
-- NPC_GROUND_SCORCH 33221) at victim + non-triggered DoCast(
-- SPELL_SCORCH 62546) self (C++ arm order); the stale "the SummonCreature
-- leg has no bridge" claim was retracted — creature:SummonCreature is
-- bridged (lua_creature_events.go:728).
-- EVENT_CONSTRUCT (:270-:275) — PORTED 2026-11-05 (constructTick):
-- Talk(SAY_SUMMON 1) + DoSummon(NPC_IRON_CONSTRUCT 33121, random spawn
-- point, 30s, TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT) + DoCast(
-- SPELL_STRENGHT 64473) self + DoCast(me, SPELL_ACTIVATE_CONSTRUCT 62488)
-- self; 15s init, re-arm RAID_MODE(40s, 30s). The "DoSummon has no bridge"
-- claim was stale: DoSummon is CreatureAI's thin SummonCreature wrapper
-- and creature:SummonCreature is bridged (lua_creature_events.go:728) with
-- summonType 3 = TIMED_DESPAWN_OUT_OF_COMBAT (the vancleef DoSummon
-- precedent); RAID_MODE resolves via creature:Is25ManRaid() (bridged,
-- lua_creature_events.go:3651; 25-man 30s, 10-man 40s). The summoned
-- construct fires the ported ev19 JustSummoned arm. Its own AI stays
-- unregistered (DamageTaken shatter leg blocked, see below), so the
-- construct melees engine-driven without the HEAT/BRITTLE machine.
-- npc_iron_construct (NPC_IRON_CONSTRUCT 33121 local enum —
-- entry-verifiable but bridge-blocked; the npc_spark_of_ionar
-- no-bridgeable-arms precedent — no registration): DamageTaken
-- shatter leg (HasAura(SPELL_BRITTLE 62382 / 67114) && damage
-- >= 5000 -> DoCast(SPELL_SHATTER 62383) self + DoAction(
-- ACTION_REMOVE_BUFF) to Ignis + DespawnOrUnsummon(1s)) — damage-taken IS
-- bridged (creature event 9, engine/world/lua_creature_events.go:286); the
-- leg is blocked by the DoAction cross-AI arm (RemoveAuraFromStack also
-- unbridged) and DespawnOrUnsummon; the NPC now spawns live via the ported
-- EVENT_CONSTRUCT arm but stays unregistered (arms blocked as above).
-- UpdateAI HEAT-stack / MOLTEN / BRITTLE /
-- IsInWater machine — aura-stack leg: GetStackAmount is bridged
-- but self-reads only engine-applied auras (a Lua AddAura reads 0)
-- and the HEAT source machine is unported; water-state (IsInWater)
-- still has no bridge; no Talk arms anywhere.
-- npc_scorch_ground (NPC_GROUND_SCORCH 33221 — now spawned by the
-- ported EVENT_SCORCH arm): MoveInLineOfSight heat tracking + UpdateAI
-- AddAura(SPELL_HEAT 65667) 1s machine — proximity IS bridged (creature
-- event 27) and HasAura/AddAura are bridged, so the old "no proximity /
-- aura bridge" claim is stale; the stale "the real blocker is the spawn:
-- SummonCreature has no bridge" claim was retracted 2026-10-23 (the
-- summon bridge is live) — the heat machine stays out on the unported
-- scorch-ground AI (MoveInLineOfSight heat tracking + UpdateAI AddAura
-- machine); the SetControlled(true, ROOT) leg has the SetRooted(true)
-- twin and PACIFIED is a plain SetFlag (boss_ignis.cpp:387) — neither
-- blocks it. Documented-only. Reset DoCast(me, SPELL_GROUND 62548) would
-- be bridgeable alone; no Talk arms anywhere.
-- spell_ignis_slag_pot (62717) — periodic-dummy AuraScript
-- (SPELL_SLAG_POT_DAMAGE 65722 on tick; OnRemove casts SPELL_
-- SLAG_IMBUED 62836 when target alive) — no AuraScript bridge;
-- joins the no-AuraScript-bridge queue.
-- achievement_ignis_shattered — OnCheck: target AI GetData
-- (DATA_SHATTERED) != 0 — no GetData / achievement bridges;
-- joins the unmodeled-achievement queue.

local ENTRY_IGNIS = 33118

-- DATA_IGNIS = 1 (ulduar.h:37).
local DATA_IGNIS = 1

local SAY_AGGRO = 0
local SAY_SUMMON = 1
local SAY_SLAY = 4
local SAY_DEATH = 6
local SAY_BERSERK = 5
local SAY_SCORCH = 3 -- boss_ignis.cpp:264
local EMOTE_JETS = 7

local SPELL_FLAME_JETS = 62680
local SPELL_BERSERK = 47008
local SPELL_SCORCH = 62546 -- boss_ignis.cpp:267
local SPELL_STRENGHT = 64473 -- boss_ignis.cpp:273 (C++ spelling)
local SPELL_ACTIVATE_CONSTRUCT = 62488 -- boss_ignis.cpp:274

local NPC_GROUND_SCORCH = 33221 -- boss_ignis.cpp:266
local NPC_IRON_CONSTRUCT = 33121 -- boss_ignis.cpp:272
local FACTION_MONSTER_2 = 16
local REACT_AGGRESSIVE = 2
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local UNIT_FLAG_PACIFIED = 0x00020000
local UNIT_FLAG_STUNNED = 0x00040000

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

-- Boss UpdateAI CASTING gates (boss_ignis.cpp pre-loop :214 +
-- in-loop :283) PORTED as a 500ms IsCasting defer in jet,
-- scorch and berserk ticks (mal_ganis idiom; IsCasting live,
-- lua_creature_events.go:1950); Talk/DamageTaken/JustSummoned
-- arms stay ungated per C++ (separate hooks).
-- C++ EVENT_JET (:222-:224): Talk(EMOTE_JETS 7) + DoCast(me,
-- SPELL_FLAME_JETS 62680) 2-arg non-triggered self-cast; 30s
-- init, re-arm {35s,40s}.
local function jetTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "jet", 500, function()
            jetTick(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_JETS)
    creature:CastSpell(creature, SPELL_FLAME_JETS)
    schedule(guid, "jet", {35000, 40000}, function()
        jetTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK (:278-:279): DoCast(me, SPELL_BERSERK 47008,
-- true) 3-arg triggered self-cast + Talk(SAY_BERSERK 5); 480s
-- init, one-shot.
local function berserkTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function()
            berserkTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_BERSERK, true)
    creature:Talk(SAY_BERSERK)
end

-- C++ JustEngagedWith: BossAI passthrough + ScheduleEvent legs
-- + DoStartTimedAchievement — the Talk, the SetBossState base leg and the
-- bridgeable timer casts ride here; the slag vehicle chain and the
-- timed-achievement leg have no bridges.
-- C++ EVENT_SCORCH (boss_ignis.cpp:262-268): Talk(SAY_SCORCH 3) +
-- SummonCreature(NPC_GROUND_SCORCH 33221) at the victim's position,
-- TEMPSUMMON_TIMED_DESPAWN 45s + non-triggered DoCast(SPELL_SCORCH
-- 62546) self; 25s init / 25s re-arm (C++-exact). The stale "the
-- SummonCreature leg has no bridge" claim was retracted 2026-10-23 —
-- creature:SummonCreature is bridged (lua_creature_events.go:728).
local function scorchTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "scorch", 500, function()
            scorchTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_SCORCH)
    local victim = creature:GetVictim()
    if victim then
        creature:SummonCreature(NPC_GROUND_SCORCH, victim:GetX(),
            victim:GetY(), victim:GetZ(), 0, 2, 45000)
    end
    creature:CastSpell(creature, SPELL_SCORCH)
    schedule(guid, "scorch", 25000, function()
        scorchTick(creature, guid)
    end)
end

-- C++ EVENT_CONSTRUCT (boss_ignis.cpp:270-275): Talk(SAY_SUMMON 1) +
-- DoSummon(NPC_IRON_CONSTRUCT 33121, random ConstructSpawnPosition,
-- 30s, TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT) + DoCast(SPELL_STRENGHT
-- 64473) self + DoCast(me, SPELL_ACTIVATE_CONSTRUCT 62488) self; 15s
-- init (:151) / re-arm RAID_MODE(40s, 30s) (:275). DoSummon is
-- CreatureAI's SummonCreature wrapper — creature:SummonCreature with
-- summonType 3 (TIMED_DESPAWN_OUT_OF_COMBAT) is the twin (vancleef
-- precedent); RAID_MODE resolves via creature:Is25ManRaid() (bridged,
-- lua_creature_events.go:3651). The summon fires the ported ev19
-- JustSummoned arm (faction/react/flags + EngageWithTarget).
-- ConstructSpawnPosition table, boss_ignis.cpp:92-113 (20 entries).
local CONSTRUCT_SPAWN_POINTS = {
    {630.366, 216.772, 360.891, 3.001970},
    {630.594, 231.846, 360.891, 3.124140},
    {630.435, 337.246, 360.886, 3.211410},
    {630.493, 313.349, 360.886, 3.054330},
    {630.444, 321.406, 360.886, 3.124140},
    {630.366, 247.307, 360.888, 3.211410},
    {630.698, 305.311, 360.886, 3.001970},
    {630.500, 224.559, 360.891, 3.054330},
    {630.668, 239.840, 360.890, 3.159050},
    {630.384, 329.585, 360.886, 3.159050},
    {543.220, 313.451, 360.886, 0.104720},
    {543.356, 329.408, 360.886, 6.248280},
    {543.076, 247.458, 360.888, 6.213370},
    {543.117, 232.082, 360.891, 0.069813},
    {543.161, 305.956, 360.886, 0.157080},
    {543.277, 321.482, 360.886, 0.052360},
    {543.316, 337.468, 360.886, 6.195920},
    {543.280, 239.674, 360.890, 6.265730},
    {543.265, 217.147, 360.891, 0.174533},
    {543.256, 224.831, 360.891, 0.122173},
}

local function constructTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "construct", 500, function()
            constructTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_SUMMON)
    local pos = CONSTRUCT_SPAWN_POINTS[math.random(#CONSTRUCT_SPAWN_POINTS)]
    creature:SummonCreature(NPC_IRON_CONSTRUCT, pos[1], pos[2], pos[3], pos[4], 3, 30000)
    creature:CastSpell(creature, SPELL_STRENGHT)
    creature:CastSpell(creature, SPELL_ACTIVATE_CONSTRUCT)
    local delay = 40000
    if creature:Is25ManRaid() then
        delay = 30000
    end
    schedule(guid, "construct", delay, function()
        constructTick(creature, guid)
    end)
end

-- C++ JustSummoned (boss_ignis.cpp:178-192): construct setup +
-- AttackStart(victim) + DoZoneInCombat + summons.Summon latch.
-- Table defined above its first caller (halazzi/skadi idiom); the
-- latch feeds Reset _Reset (:135-137) + JustDied _JustDied
-- (:158-160) + leave (evade reaches Reset) DespawnAll.
local ignisSummons = {}

local function despawnIgnisSummons(guid)
    local latched = ignisSummons[guid]
    if latched then
        for _, summon in ipairs(latched) do
            if summon then
                summon:Despawn()
            end
        end
        ignisSummons[guid] = nil
    end
end

local function ignisJustSummoned(event, creature, summon)
    if not summon then
        return
    end
    local guid = creature:GetGUID()
    local latched = ignisSummons[guid]
    if not latched then
        latched = {}
        ignisSummons[guid] = latched
    end
    latched[#latched + 1] = summon
    if summon:GetEntry() == NPC_IRON_CONSTRUCT then
        summon:SetFaction(FACTION_MONSTER_2)
        summon:SetReactState(REACT_AGGRESSIVE)
        summon:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        summon:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        summon:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_PACIFIED)
        summon:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED)
        summon:SetRooted(false) -- boss_ignis.cpp:186 SetControlled(false, ROOT) twin (tick 236)
        -- SetImmuneToPC(false) stays out: no bridge (the construct arm
        -- fires only on a direct summon; constructs still never spawn
        -- in Lua, DoSummon leg blocked).
    end
    local victim = creature:GetVictim()
    if victim then
        summon:EngageWithTarget(victim) -- AttackStart twin
    end
    summon:DoZoneInCombat()
end

local function ignisEnterCombat(event, creature, target)
    -- C++ JustEngagedWith: BossAI::JustEngagedWith first (boss_ignis.cpp:146) —
    -- SetBossState(DATA_IGNIS, IN_PROGRESS) (ScriptedCreature.cpp base leg;
    -- the maiden precedent), then SetCombatPulseDelay + DoZoneInCombat,
    -- before Talk(SAY_AGGRO).
    creature:SetBossState(DATA_IGNIS, 1)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540) -- base-first (boss_ignis.cpp:146)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "jet", 30000, function()
        jetTick(creature, guid)
    end)
    -- C++ JustEngagedWith: events.ScheduleEvent(EVENT_SCORCH, 25s).
    schedule(guid, "scorch", 25000, function()
        scorchTick(creature, guid)
    end)
    -- C++ JustEngagedWith: events.ScheduleEvent(EVENT_CONSTRUCT, 15s).
    schedule(guid, "construct", 15000, function()
        constructTick(creature, guid)
    end)
    schedule(guid, "berserk", 480000, function()
        berserkTick(creature, guid)
    end)
end

local function ignisLeaveCombat(event, creature)
    despawnIgnisSummons(creature:GetGUID())
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then Talk
-- (SAY_SLAY) — the razuvious player-gated variant precedent.
local function ignisTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: _JustDied() (SetBossState DATA_IGNIS, DONE — ported
-- 2026-11-05, base-first) + Talk(SAY_DEATH).
local function ignisDied(event, creature, killer)
    creature:SetBossState(DATA_IGNIS, 3) -- C++ JustDied: _JustDied() first (boss_ignis.cpp:160), before Talk
    despawnIgnisSummons(creature:GetGUID())
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function ignisReset(event, creature)
    creature:SetBossState(DATA_IGNIS, 0) -- C++ Reset: _Reset() first (boss_ignis.cpp:137)
    despawnIgnisSummons(creature:GetGUID())
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_IGNIS, 1, ignisEnterCombat)
RegisterCreatureEvent(ENTRY_IGNIS, 2, ignisLeaveCombat)
RegisterCreatureEvent(ENTRY_IGNIS, 3, ignisTargetDied)
RegisterCreatureEvent(ENTRY_IGNIS, 4, ignisDied)
RegisterCreatureEvent(ENTRY_IGNIS, 19, ignisJustSummoned)
RegisterCreatureEvent(ENTRY_IGNIS, 23, ignisReset)
