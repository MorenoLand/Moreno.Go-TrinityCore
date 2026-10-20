-- Culling of Stratholme: Mal'Ganis — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/CullingOfStratholme/
-- boss_mal_ganis.cpp (197 lines; class boss_mal_ganis :
-- public CreatureScript { boss_mal_ganisAI : public BossAI(creature,
-- DATA_MAL_GANIS), _defeated / _hadYell30 / _hadYell15 }; GetAI gate is
-- InstanceHasScript(creature, CoSScriptName) plus
-- DATA_INSTANCE_PROGRESS >= MALGANIS_IN_PROGRESS else NullCreatureAI;
-- AddSC_boss_mal_ganis at end registers the one script;
-- kalimdor loader decl 51 / call 164 per kalimdor_script_loader.cpp —
-- the fifth "// CoT Culling Of Stratholme" loader group, right after
-- AddSC_boss_salramm()). Whole-server-tree quoted-name grep confirms
-- the cpp as the sole source of "boss_mal_ganis" (loader decl/call
-- lines only otherwise). Entry: npc_arthas.cpp:60 names
-- NPC_MALGANIS = 26533 and :698 / :1122 have
-- instance->instance->SummonCreature(NPC_MALGANIS, ...) — the
-- name-to-entry tie is C++-verified at summon strength (:698 is the
-- RP5_MALGANIS_POS real-fight summon, :1122 the RP2 intro summon;
-- not GUID-bound in OnCreatureCreate). BossAI's ctor leg
-- DATA_MAL_GANIS is culling_of_stratholme.h:119; MALGANIS_IN_PROGRESS
-- is :106. Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnKilledUnit, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; instance/outro legs
-- below):
-- - Engage (C++ JustEngagedWith): no Talk in C++; latch flags reset
--   (fresh per-GUID state here) + arm each timer at its C++
--   ScheduleEvent cooldown (5s / {6s,8s} / 4s / {17s,21s}); the
--   BossAI::JustEngagedWith instance leg is blocked (standing).
-- - Carrion Swarm (EVENT_CARRION_SWARM): DoCastAOE(SPELL_CARRION_SWARM
--   52720), non-triggered (DoCastAOE resolves to self-cast —
--   kazrogal/illidan precedent), init 5s -> repeat 6s.
-- - Mind Blast (EVENT_MIND_BLAST): SelectTarget(Random, 1, 100.0f,
--   true, -GetSpellIdForDifficulty(SPELL_SLEEP, me)) -> random alive
--   player within 100m excluding the victim (azgalor position-1
--   convention — no threat-list bridge) AND not carrying the Sleep
--   aura (the negative spell-id exception filter; resolves to 52721,
--   no heroic difficulty leg on this spell) — HasAura precedent in
--   blackfathom_deeps.lua; DoCast(target, 52722) non-triggered, else
--   DoCastVictim; init {6s,8s} -> repeat {8s,12s} unconditional (C++
--   Repeat is outside the if/else, nil target -> victim cast).
-- - Vampiric Touch (EVENT_VAMPIRIC_TOUCH): DoCastSelf(52723),
--   non-triggered, init 4s -> repeat 30s.
-- - Sleep (EVENT_SLEEP): SelectTarget(Random, 1, 100.0f) — C++ has no
--   player-only filter, but the Lua API enumerates players only (no
--   nearby-unit enumeration bridge), so this is approximated by
--   random alive player within 100m excluding the victim; DoCast
--   (target, 52721) non-triggered, else DoCastVictim; init {17s,21s}
--   -> repeat {10s,15s} unconditional (nil target -> victim cast).
--   C++ Talks NOTHING here — SAY_SLEEP (5) is never Talked in the
--   file (same class as SAY_KILL (3)); both unused.
-- - DamageTaken (event 9): lethal clamp damage = health-1 +
--   _defeated once-latch (midnight never-die convention; PermBind leg
--   out, no Lua bridge); latch reset on engage.
-- - Health yells (UpdateAI per-tick legs): SAY_30HEALTH (6) at
--   HealthBelowPct(30) and SAY_15HEALTH (7) at HealthBelowPct(15),
--   each latched once (C++ _hadYell30/_hadYell15, reset in
--   JustEngagedWith) — 1s per-GUID pump (sironas convention) using
--   creature:GetHealthPct() (npc_arthas precedent); an approximation
--   of C++'s per-tick check, like old_hillsbrad.
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): C++ gates
--   on !_defeated && victim->GetTypeId() == TYPEID_PLAYER before Talk
--   SAY_SLAY (4) (terestian_illhoof victim:GetObjectType() ==
--   "Player" convention); the !_defeated gate now rides the
--   DamageTaken latch ported above (clamp + latch); the UpdateAI
--   _defeated evade/immune legs stay out (no bridges).
-- - Death (no JustDied override in C++ — the defeated machine
--   short-circuits it): cancel only.
-- Unmodeled (documented-only, no bridges):
-- - GetAI's CoSScriptName instance gate + DATA_INSTANCE_PROGRESS <
--   MALGANIS_IN_PROGRESS -> NullCreatureAI — no instance-data /
--   instance-progress bridges (standing); the Lua registration is
--   entry-based and has no CoS-instance gate.
-- - BossAI ctor (DATA_MAL_GANIS) / Reset's
--   SetBossState(DATA_MAL_GANIS, NOT_STARTED) — instance legs
--   blocked (standing).
-- - DamageTaken lethal clamp — PORTED (this run): OnDamageTaken
--   event 9 is bridged (midnight never-die clamp precedent); the
--   damage = health-1 clamp and the _defeated once-latch ride the
--   midnight "returns false, newDamage" convention with the health > 0
--   hook guard. The PermBindAllPlayers leg rides no Lua bridge
--   (instance_binds.go has no Lua surface) — stays out. The UpdateAI
--   _defeated legs (EnterEvadeMode/SetImmuneToAll) still ride no
--   evade/immune bridges (below).
-- - UpdateAI's _defeated leg: me->IsInCombat() -> EnterEvadeMode() +
--   SetImmuneToAll(true) — no evade-mode / immune-to-all bridges
--   (infinite_corruptor DoAction-leave precedent).
-- - JustReachedHome: DespawnOrUnsummon(1s) — PORTED (onReachedHome,
--   event 24): the stale "no despawn bridge" was retracted earlier
--   (creature:Despawn() is bridged, lua_creature_events.go:214), and
--   the stale "absent JustReachedHome event in the Go API" claim is
--   retracted too — the engine fires CREATURE_EVENT_ON_REACH_HOME (24)
--   from the home-move finalization (engine/world/creaturemotion.go).
--   The !_defeated gate rides the DamageTaken latch (defeated[guid])
--   ported above.
-- - The C++ while(ExecuteEvent()) + UNIT_STATE_CASTING gates — no
--   casting-state bridge in the Lua API (aeonus precedent —
--   unmodeled).
-- - SAY_KILL (3) / SAY_SLEEP (5): declared in the C++ yells enum,
--   never Talked anywhere in the file.

local ENTRY = 26533

local SAY_SLAY      = 4
local SAY_30HEALTH  = 6
local SAY_15HEALTH  = 7

local SPELL_CARRION_SWARM = 52720
local SPELL_MIND_BLAST    = 52722
local SPELL_SLEEP         = 52721
local SPELL_VAMPIRIC_TOUCH = 52723

local timers = {}
local pumps = {}
local defeated = {} -- per-GUID C++ _defeated latch, reset on engage

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    local pump = pumps[guid]
    if pump then
        RemoveEventById(pump)
        pumps[guid] = nil
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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- nefarian convention: random alive player in the instance within
-- maxDist; excludeVictim approximates C++ position 1 ("never on
-- tank") where no threat-list bridge exists (azgalor Doom precedent);
-- excludeAura models C++'s negative spell-id exception filter
-- (units not affected by that aura — here Sleep 52721).
local function randomPlayerInRange(creature, maxDist, excludeVictim, excludeAura)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local victim = creature:GetVictim()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist
                and not (excludeVictim and victim and p == victim)
                and not (excludeAura and p:HasAura(excludeAura)) then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ EVENT_CARRION_SWARM: DoCastAOE(52720), non-triggered, self-cast
-- (kazrogal/illidan DoCastAOE precedent); init 5s -> repeat 6s.
local function onCarrionSwarm(creature, guid)
    creature:CastSpell(creature, SPELL_CARRION_SWARM)
    schedule(guid, "carrionswarm", 6000, function()
        onCarrionSwarm(creature, guid)
    end)
end

-- C++ EVENT_MIND_BLAST: DoCast(SelectTarget(Random, 1, 100.0f, true,
-- -sleep), 52722) non-triggered else DoCastVictim(52722); init
-- {6s,8s} -> repeat {8s,12s} unconditional (C++ Repeat sits outside
-- the if/else; nil target -> victim cast).
local function onMindBlast(creature, guid)
    local target = randomPlayerInRange(creature, 100, true, SPELL_SLEEP)
    if target then
        creature:CastSpell(target, SPELL_MIND_BLAST)
    else
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_MIND_BLAST)
        end
    end
    schedule(guid, "mindblast", math.random(8000, 12000), function()
        onMindBlast(creature, guid)
    end)
end

-- C++ EVENT_VAMPIRIC_TOUCH: DoCastSelf(52723), non-triggered;
-- init 4s -> repeat 30s.
local function onVampiricTouch(creature, guid)
    creature:CastSpell(creature, SPELL_VAMPIRIC_TOUCH)
    schedule(guid, "vampirictouch", 30000, function()
        onVampiricTouch(creature, guid)
    end)
end

-- C++ DamageTaken (boss_mal_ganis.cpp:80-92): if (damage >= health)
--   { damage = health - 1; if (_defeated) return; _defeated = true;
--   PermBindAllPlayers }. Clamp + latch ported via event 9
--   (fireCreatureDamageTaken, midnight never-die-clamp precedent); the
--   PermBind leg rides no Lua bridge. health > 0 guards the hook on an
--   already-dead creature.
local function malganisDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local health = creature:GetHealth()
    local newDamage = damage
    if health > 0 and damage >= health then
        newDamage = health - 1
        defeated[guid] = true
    end
    return false, newDamage
end

-- C++ EVENT_SLEEP: DoCast(SelectTarget(Random, 1, 100.0f), 52721)
-- non-triggered else DoCastVictim(52721); init {17s,21s} ->
-- repeat {10s,15s} unconditional (nil target -> victim cast).
local function onSleep(creature, guid)
    local target = randomPlayerInRange(creature, 100, true)
    if target then
        creature:CastSpell(target, SPELL_SLEEP)
    else
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_SLEEP)
        end
    end
    schedule(guid, "sleep", math.random(10000, 15000), function()
        onSleep(creature, guid)
    end)
end

-- C++ UpdateAI health-yell legs (per-tick in C++; 1s pump here,
-- sironas convention): HealthBelowPct(30) -> Talk(6) and
-- HealthBelowPct(15) -> Talk(7), each latched once (C++
-- _hadYell30/_hadYell15); the floored healthBelowPct helper is the
-- C++-exact comparison (ironaya convention).
-- C++ HealthBelowPct(pct) = GetHealth() < floor(maxHealth*pct/100)
-- (ironaya convention) — GetHealthPct() fires up to 1 HP early on the
-- float edge.
local function healthBelowPct(creature, pct)
    local max = creature:GetMaxHealth()
    if max == 0 then
        return false
    end
    return creature:GetHealth() < math.floor(max * pct / 100)
end

local function startHealthYellPump(creature, guid)
    local hadYell30, hadYell15 = false, false
    pumps[guid] = CreateLuaEvent(function()
        if not hadYell30 and healthBelowPct(creature, 30) then
            creature:Talk(SAY_30HEALTH)
            hadYell30 = true
        end
        if not hadYell15 and healthBelowPct(creature, 15) then
            creature:Talk(SAY_15HEALTH)
            hadYell15 = true
        end
        if hadYell30 and hadYell15 then
            RemoveEventById(pumps[guid])
            pumps[guid] = nil
        end
    end, 1000, 0)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    defeated[guid] = nil -- C++ JustEngagedWith resets _defeated
    startHealthYellPump(creature, guid)
    schedule(guid, "carrionswarm", 5000, function()
        onCarrionSwarm(creature, guid)
    end)
    schedule(guid, "mindblast", math.random(6000, 8000), function()
        onMindBlast(creature, guid)
    end)
    schedule(guid, "vampirictouch", 4000, function()
        onVampiricTouch(creature, guid)
    end)
    schedule(guid, "sleep", math.random(17000, 21000), function()
        onSleep(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ JustReachedHome (boss_mal_ganis.cpp:106-110): if (!_defeated)
-- me->DespawnOrUnsummon(Seconds(1)). The _defeated gate rides the
-- DamageTaken latch (defeated[guid]) ported above. The 1s delay is a
-- fire-and-forget one-shot CreateLuaEvent — C++-exact: C++
-- DespawnOrUnsummon rides the creature's own despawn timer, not the
-- EventMap, so a re-engage inside the 1s window does not cancel it
-- (the tracked schedule idiom would wrongly die with cancelTimers
-- on ev1 — fixed 2026-10-20).
local function onReachedHome(event, creature)
    if not defeated[creature:GetGUID()] then
        CreateLuaEvent(function()
            creature:Despawn()
        end, 1000)
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, malganisDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
RegisterCreatureEvent(ENTRY, 24, onReachedHome)
