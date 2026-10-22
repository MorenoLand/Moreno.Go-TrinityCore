-- Amanitar (Ahn'kahet) — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/Ahnkahet/boss_amanitar.cpp
-- (boss_amanitar + npc_amanitar_mushrooms +
-- spell_amanitar_potent_fungus).
-- Ahn'kahet dungeon-script unit per northrend_script_loader.cpp order
-- (decl 34 / call 224, immediately after AddSC_boss_taldaram();
-- next: AddSC_boss_jedoga_shadowseeker() (decl 35 / call 225), then
-- AddSC_boss_volazj() (decl 36 / call 226, script boss_herald_volazj)).
-- Entry: 30258 Amanitar (ahnkahet.h NPC_AMANITAR — kalecgos pass; the
-- RegisterAhnKahetCreatureAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as
-- usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim, creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell[, triggered]) = DoCastSelf
-- (phase_hunter / apothecary_hanes precedent — the trailing true is
-- the triggered port).
-- Ported arms (C++-exact for all modeled arms):
-- boss_amanitar: EVENT_BASH — DoCastVictim(Bash 57094);
-- JustEngagedWith-scheduled 10-14s init, rescheduled 7-12s;
-- EVENT_MINI — SelectTarget(Random, 0, 0.0f, true, true,
-- -SPELL_MINI 57055): random alive player WITHOUT the Mini aura
-- (the negative-aura filter is a player:HasAura(57055) exclusion
-- in the GetAITargets pool — netherspite/vaelastrasz precedent;
-- the old "random-target SelectTarget bridge absent" header claim
-- was stale, retracted 2026-10-18) -> DoCastAOE(Mini 57055)
-- self-cast non-triggered (arugal DoCastAOE convention); 12-18s
-- init, 30s repeat on cast, 1s fail-fast repeat when no valid
-- target (C++ :141-147 — the else branch re-arms 1s, not 30s);
-- EVENT_ROOT — SelectTarget(Random, 1, 100.0f, true):
-- random non-victim (position-1 offset skips the tank —
-- malchezaar/netherspite convention) alive threat target within
-- 100yd, playerOnly=true (the 4th arg, murmur convention) -> DoCast(target, Entangling Roots 57095) TRIGGERED
-- (trailing true — apothecary_hanes convention); 5-9s init,
-- 10-15s repeat (the re-arm sits outside the target check —
-- C++ :150-152 exact); EVENT_BOLT — SelectTarget(Random, 0,
-- 100.0f, true): playerOnly=true (the 4th arg), random alive
-- player within 100yd (position 0 = no offset) -> DoCast(target,
-- Venom Bolt Volley 57088) TRIGGERED; 15-20s init, 18-22s
-- repeat (C++ :156-160).
-- JustDied — DoCastAOE(Remove Mushroom Power 57283), self-cast
-- (arugal precedent).
-- Unmodeled (no bridges — documented, not wired):
-- EVENT_SPAWN — SummonCreature(healthy 30391 40% / poisonous
-- 30435, TEMPSUMMON_CORPSE_TIMED_DESPAWN, 4s) over the 32
-- MushroomPositions (1s): summon STRAND absent (standing).
-- EVENT_RESPAWN — _mushroomsDeque dead-mushroom position re-spawn
-- machine (40-60s repeat): summon STRAND absent.
-- JustSummoned summons.Summon leg — no summon bridge.
-- SummonedCreatureDies mushroom-position bookkeeping (into the
-- deque the EVENT_RESPAWN leg reads) — cross-AI / summon bridges
-- absent.
-- EnterEvadeMode — summons.DespawnAll (summon-lifecycle STRAND
-- absent — the old "no despawn bridge (terestian precedent)" was
-- stale: creature:Despawn() IS bridged (lua_creature_events.go:236),
-- but the arm needs the whole summon list), instance
-- DoRemoveAurasDueToSpellOnPlayers(Mini) (instance-script model
-- absent — standing blocker), _DespawnAtEvade.
-- JustDied — _JustDied() instance bookkeeping only (no bridge);
-- DoCastAOE(Remove Mushroom Power 57283) ported as self-cast
-- (creature:CastSpell(creature, 57283) — the Go creature-cast path
-- carries no alive-gate on the caster, so the OnDied cast is
-- faithful); instance DoRemoveAurasDueToSpellOnPlayers(Mini)
-- (instance model absent).
-- npc_amanitar_mushrooms — ENTRY-VERIFIABLE BUT BRIDGE-BLOCKED
-- (30391 NPC_HEALTHY_MUSHROOM / 30435 NPC_POISONOUS_MUSHROOM —
-- ahnkahet.h kalecgos pass): zero registration. Reset leg —
-- SetReactState(REACT_PASSIVE) (SetReactState IS bridged —
-- frozen_core precedent, lua_creature_events.go:524) +
-- SetDisplayId(Modelid2) (display bridge live 2026-10-20 — the
-- blocker is the unbridged Modelid2 VALUE bearer: no
-- GetCreatureTemplate().Modelid2 in Lua) + DoCastSelf triplet
-- (Putrid Mushroom 31690 / Shrink 31691 triggered / Grow 57059
-- triggered) + the healthy/poisonous visual-aura self-cast
-- (56740 / 56741) + the 800ms re-Grow scheduler (CreateLuaEvent
-- one-shot): the Reset leg stays out — the mushroom entries
-- (30391/30435) have no summon source (the boss EVENT_SPAWN leg
-- rides the summon STRAND), so the whole arm would be dead code
-- until the strand exists. The
-- MoveInLineOfSight player-proximity poison leg (2.0f, _active
-- latch): target RemoveAurasDueToSpell(Potent Fungus 56648)
-- (player:RemoveAura IS bridged — engine/world/lua_player.go:219; the
-- old "no aura-removal bridge (aeranas precedent)" claim is stale),
-- DoCastAOE(Poisonous
-- Mushroom Poison Cloud 57061) (stays out: summon STRAND absent),
-- SetObjectScale(0.1f) (stays out: summon STRAND absent — the
-- scale bridge itself exists, creature:SetScale), DespawnOrUnsummon(4s)
-- (stale "no despawn bridge" retracted — creature:Despawn() is
-- bridged, lua_creature_events.go:214; the proximity + summon legs
-- are the true blockers). JustDied healthy
-- leg: DoCastAOE(Potent Fungus 56648, triggered) (stays out:
-- behind the unbridged summon STRAND). Joins the
-- entry-verifiable-but-bridge-blocked queue.
-- spell_amanitar_potent_fungus — AuraScript (56648 Potent Fungus:
-- on-apply, if the target HasAura(Mini 57055) remove it and drop
-- the aura): no SpellScript/AuraScript binding bridge (razelikh
-- precedent) — documented-only.
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler
-- (gargolmar precedent); BossAI _Reset/_JustDied instance
-- bookkeeping has no bridge.

local ENTRY_AMANITAR = 30258

local SPELL_BASH = 57094
local SPELL_MINI = 57055
local SPELL_ENTANGLING_ROOTS = 57095
local SPELL_VENOM_BOLT_VOLLEY = 57088
local SPELL_REMOVE_MUSHROOM_POWER = 57283

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

-- Threat-list random pick (the murmur / brundir idiom — the old
-- "no random-target SelectTarget bridge" claim is stale):
-- playersOnly honors C++ playerOnly via the GetObjectType
-- "Player" filter (illidan/murmur convention); maxDist <= 0 skips
-- the band (C++ dist 0.0f = any range). hasAuraExclude honors
-- the negative-aura SelectTarget filter (-SPELL_MINI, C++ :143).
local function randomThreatTarget(creature, maxDist, playersOnly, hasAuraExclude)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if (not playersOnly) or t:GetObjectType() == "Player" then
            if maxDist <= 0 or creature:GetDistance(t) <= maxDist then
                if hasAuraExclude == nil or not t:HasAura(hasAuraExclude) then
                    candidates[#candidates + 1] = t
                end
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ SelectTarget(Random, 1, dist, true, ...): position-1 offset
-- skips the first (victim) entry — malchezaar/netherspite
-- convention; playerOnly=true honored by the GetObjectType
-- "Player" filter (murmur convention).
local function randomNonVictimTarget(creature, maxDist)
    local victim = creature:GetVictim()
    local victimGuid = victim ~= nil and victim:GetGUID() or nil
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:GetGUID() ~= victimGuid
                and (maxDist <= 0 or creature:GetDistance(t) <= maxDist) then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_MINI (:141-148): SelectTarget(Random, 0, 0.0f, true,
-- true, -SPELL_MINI) — random alive player WITHOUT the Mini aura
-- (playerOnly=true, any range); on hit DoCastAOE(SPELL_MINI)
-- self-cast non-triggered (vaelastrasz/arugal convention) and
-- Repeat(30s); on miss Repeat(1s) fail-fast (the else branch, NOT
-- the 30s cycle).
local function miniTick(creature, guid)
    if randomThreatTarget(creature, 0, true, SPELL_MINI) ~= nil then
        creature:CastSpell(creature, SPELL_MINI)
        schedule(guid, "mini", 30000, function()
            miniTick(creature, guid)
        end)
    else
        schedule(guid, "mini", 1000, function()
            miniTick(creature, guid)
        end)
    end
end

-- C++ EVENT_ROOT (:150-153): SelectTarget(Random, 1, 100.0f, true)
-- -> DoCast(target, SPELL_ENTANGLING_ROOTS, TRIGGERED); the
-- Repeat(10s, 15s) sits outside the target check — fires even on
-- nil target (C++-exact).
local function rootTick(creature, guid)
    local target = randomNonVictimTarget(creature, 100.0)
    if target then
        creature:CastSpell(target, SPELL_ENTANGLING_ROOTS, true)
    end
    schedule(guid, "root", math.random(10000, 15000), function()
        rootTick(creature, guid)
    end)
end

-- C++ EVENT_BOLT (:155-161): SelectTarget(Random, 0, 100.0f,
-- true) — playerOnly=true (the 4th arg, not the 5th; the offset
-- sits at 0 so no tank skip) -> DoCast(target,
-- SPELL_VENOM_BOLT_VOLLEY, TRIGGERED); Repeat(18s, 22s) outside
-- the target check (C++-exact).
local function boltTick(creature, guid)
    local target = randomThreatTarget(creature, 100.0, true, nil)
    if target then
        creature:CastSpell(target, SPELL_VENOM_BOLT_VOLLEY, true)
    end
    schedule(guid, "bolt", math.random(18000, 22000), function()
        boltTick(creature, guid)
    end)
end

-- C++ EVENT_BASH: JustEngagedWith-scheduled 10-14s init,
-- rescheduled 7-12s; DoCastVictim. UpdateAI returns early while
-- casting — the Go engine handles cast-gating, the timer just
-- re-arms.
local function bashTick(creature, guid)
    creature:CastSpell(nil, SPELL_BASH)
    schedule(guid, "bash", math.random(7000, 12000), function()
        bashTick(creature, guid)
    end)
end

local function amanitarEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ JustEngagedWith (boss_amanitar.cpp:96-101): EVENT_ROOT
    -- 5-9s, EVENT_BASH 10-14s, EVENT_BOLT 15-20s, EVENT_MINI
    -- 12-18s, EVENT_SPAWN 1s (summon machine — unbridged),
    -- EVENT_RESPAWN 40-60s (summon machine — unbridged).
    schedule(guid, "root", math.random(5000, 9000), function()
        rootTick(creature, guid)
    end)
    schedule(guid, "bash", math.random(10000, 14000), function()
        bashTick(creature, guid)
    end)
    schedule(guid, "bolt", math.random(15000, 20000), function()
        boltTick(creature, guid)
    end)
    schedule(guid, "mini", math.random(12000, 18000), function()
        miniTick(creature, guid)
    end)
end

local function amanitarLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ EnterEvadeMode (boss_amanitar.cpp:103-109): summons.DespawnAll()
    -- (moot — nothing modeled) + instance DoRemoveAurasDueToSpellOnPlayers
    -- (no instance bridge) + _DespawnAtEvade() — DespawnOrUnsummon is
    -- immediate at evade (ScriptedCreature.cpp:596-616); drakkari precedent.
    -- Stale "no despawn bridge" retracted.
    creature:Despawn()
end

local function amanitarDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_REMOVE_MUSHROOM_POWER)
end

local function amanitarReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_AMANITAR, 1, amanitarEnterCombat)
RegisterCreatureEvent(ENTRY_AMANITAR, 2, amanitarLeaveCombat)
RegisterCreatureEvent(ENTRY_AMANITAR, 4, amanitarDied)
RegisterCreatureEvent(ENTRY_AMANITAR, 23, amanitarReset)
