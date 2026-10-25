-- Elder Nadox (Ahn'kahet) — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/Ahnkahet/boss_elder_nadox.cpp
-- (boss_elder_nadox). Ahn'kahet dungeon-script unit per
-- northrend_script_loader.cpp order (decl 32 / call 222, immediately
-- after AddSC_instance_gundrak(); next: boss_taldaram).
-- Entry: 29309 Elder Nadox (ahnkahet.h NPC_ELDER_NADOX — kalecgos
-- pass; the GetAhnKahetAI ScriptName binding is instance-shimmed, the
-- creature_template binding DB-side as usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- C++ DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(creature, spell) = DoCastSelf (moroes precedent).
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler (gargolmar
-- precedent); BossAI _Reset/_JustDied instance bookkeeping has no bridge.
-- Ported arms (C++-exact for all modeled arms):
-- boss_elder_nadox: JustEngagedWith Talk(SAY_AGGRO 0) (event 1);
-- EVENT_PLAGUE DoCast(SelectTarget(Random, 0, 100, true),
-- SPELL_BROOD_PLAGUE 56130, TRIGGERED), 13s init, 15s repeat — the old
-- "random-target SelectTarget bridge absent (cairne/kazzak)" header
-- claim is RETRACTED (randomPlayer is the in-tree bridge: anomalus /
-- eadric / keleseth precedent); the pool is the THREAT list
-- (GetAITargets, tharon_ja precedent — 2026-10-25 fix, old
-- GetPlayersInWorld pool was instance-wide); no cast on an empty candidate pool
-- (C++ SelectTarget null leg — eadric hammer precedent);
-- EVENT_RAGE DoCast(H_SPELL_BROOD_RAGE 59465) self-cast non-triggered,
-- 12s init, 10-50s repeat (math.random — eck spring precedent),
-- IsHeroic()-gated at engage — the old "no difficulty bridge
-- (kelidan)" header claim is RETRACTED (creature:IsHeroic() bridged —
-- engine/world/lua_creature_events.go:260, AGENTS.md 2026-10-17);
-- EVENT_CHECK_ENRAGE, 5s init, 5s repeat: HasAura(26662) returns WITHOUT
-- re-arming (exact C++ :191-192); GetZ() < 24.0 -> DoCast(me,
-- SPELL_ENRAGE 26662, true) TRIGGERED; unconditional 5s re-arm
-- otherwise — the old "no map-height bridge for the Z check" header
-- claim is RETRACTED (GetZ is in the generic scripting fallback —
-- engine/scripting/object.go:94);
-- EVENT_SUMMON_SWARMER DoCastSelf(Summon Swarmers 56119) + 33% Talk
-- (SAY_EGG_SAC 3), 10s init, 10s repeat (math.random for the C++ urand —
-- slad_ran precedent); the guardian-summon latch
-- (!GuardianSummoned && HealthBelowPct(50) -> Talk(EMOTE_HATCHES 4, me)
-- + DoCast(me, SPELL_SUMMON_SWARM_GUARD 56120)) modeled in the
-- pre-application DamageTaken handler (event 9) with the C++-exact
-- HealthBelowPctDamaged(50, damage) strict-< (Unit.h:872) and a
-- per-GUID latch — the old "no health-pct bridge (doomwalker)" header
-- claim is RETRACTED (creature:GetHealthPct()/GetHealth() are bridged —
-- engine/scripting/object.go:107);
-- KilledUnit Talk(SAY_SLAY 1) player-gated (event 3,
-- victim:GetObjectType()=="Player" — nalorakk precedent); JustDied
-- Talk(SAY_DEATH 2) (event 4).
-- Unmodeled (no bridges — documented, not wired):
-- SummonedCreatureDies(NPC_AHNKAHAR_GUARDIAN 30176) -> GuardianDied latch:
-- the event-21 dispatch IS bridged now (2026-10-24), but GuardianDied's
-- only C++ consumer is GetData(DATA_RESPECT_YOUR_ELDERS) for the
-- achievement (boss_elder_nadox.cpp:105) behind the absent
-- achievement-criteria/cross-AI-GetData bridges — the latch would be
-- write-only, so the arm stays documented-only.
-- npc_ahnkahar_nerubian — ENTRY UNVERIFIABLE (no NPC_AHNKAHAR_
-- NERUBIAN constant in ahnkahet.h or anywhere in the C++ sources;
-- the file enums name only the guardian 30176 / swarmer 30178 —
-- belnistrasz/willix precedent, no invented identifiers): zero
-- registration. The EVENT_SPRINT arm is PORT-PATTERN-READY but
-- entry-blocked: Reset-scheduled 13s init, DoCastSelf(Sprint 56354),
-- 20s repeat (the exact slad_ran RegisterCreatureEvent 1/23 pattern);
-- the HasUnitState(UNIT_STATE_CASTING) guard has no bridge but the
-- tick shape is unaffected.
-- spell_ahn_kahet_swarm (56159 — aura-stack bookkeeping around
-- SPELL_SWARM_BUFF 56281) — no SpellScript binding bridge on the
-- Lua surface (razelikh precedent) — documented-only.
-- achievement_respect_your_elders — AchievementCriteriaScript
-- (GetAI()->GetData(DATA_RESPECT_YOUR_ELDERS)) — no
-- achievement-criteria bridge on the Lua surface (snakes precedent)
-- + cross-AI GetData bridge absent — documented-only.

local ENTRY_ELDER_NADOX = 29309

local SAY_AGGRO   = 0
local SAY_SLAY    = 1
local SAY_DEATH   = 2
local SAY_EGG_SAC = 3
local EMOTE_HATCHES = 4

local SPELL_BROOD_PLAGUE       = 56130
local H_SPELL_BROOD_RAGE       = 59465
local SPELL_ENRAGE             = 26662 -- Enraged if too far away from home
local SPELL_SUMMON_SWARMERS    = 56119
local SPELL_SUMMON_SWARM_GUARD = 56120

local timers = {}
local nadoxState = {}

local function clearState(guid)
    nadoxState[guid] = nil
end

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

-- randomPlayer: C++ SelectTarget(Random, 0, 100, true) draws the threat
-- list (UnitAI.h), not instance players — GetAITargets + Player + alive
-- + 100yd (tharon_ja / violet-hold threat-pool fix class, 2026-10-25).
local function randomPlayer(creature, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive()
                and creature:GetDistance(t) <= maxDist then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_SUMMON_SWARMER: DoCastSelf; 10s init, 10s repeat; 33%
-- Talk(SAY_EGG_SAC) on the same tick.
local function swarmerTick(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_SWARMERS)
    if math.random(1, 3) == 3 then
        creature:Talk(SAY_EGG_SAC)
    end
    schedule(guid, "swarmer", 10000, function()
        swarmerTick(creature, guid)
    end)
end

-- C++ EVENT_PLAGUE: DoCast(SelectTarget(Random, 0, 100, true),
-- SPELL_BROOD_PLAGUE, true) TRIGGERED, 13s init, 15s repeat. No cast
-- when no candidate (C++ SelectTarget returns null on empty pool —
-- eadric hammer precedent).
local function plagueTick(creature, guid)
    local target = randomPlayer(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_BROOD_PLAGUE, true)
    end
    schedule(guid, "plague", 15000, function()
        plagueTick(creature, guid)
    end)
end

-- C++ EVENT_RAGE (IsHeroic-gated): DoCast(H_SPELL_BROOD_RAGE)
-- non-triggered self-cast, 12s init, 10-50s repeat (urand — eck
-- spring math.random(min,max) precedent).
local function rageTick(creature, guid)
    creature:CastSpell(creature, H_SPELL_BROOD_RAGE)
    schedule(guid, "rage", math.random(10000, 50000), function()
        rageTick(creature, guid)
    end)
end

-- C++ EVENT_CHECK_ENRAGE (IsHeroic-gated), 5s init, 5s repeat: the
-- HasAura(SPELL_ENRAGE) arm returns WITHOUT re-arming (exact C++
-- :191-192); GetZ() < 24.0 -> DoCast(me, SPELL_ENRAGE, true)
-- TRIGGERED; the re-arm is unconditional otherwise.
local function enrageCheckTick(creature, guid)
    if not creature:HasAura(SPELL_ENRAGE) then
        if creature:GetZ() < 24.0 then
            creature:CastSpell(creature, SPELL_ENRAGE, true)
        end
        schedule(guid, "enrageCheck", 5000, function()
            enrageCheckTick(creature, guid)
        end)
    end
end

local function nadoxEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    nadoxState[guid] = { guardianSummoned = false }
    creature:Talk(SAY_AGGRO)
    schedule(guid, "swarmer", 10000, function()
        swarmerTick(creature, guid)
    end)
    schedule(guid, "plague", 13000, function()
        plagueTick(creature, guid)
    end)
    if creature:IsHeroic() then
        schedule(guid, "rage", 12000, function()
            rageTick(creature, guid)
        end)
        schedule(guid, "enrageCheck", 5000, function()
            enrageCheckTick(creature, guid)
        end)
    end
end

local function nadoxLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    clearState(guid)
end

local function nadoxTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function nadoxDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    clearState(guid)
    creature:Talk(SAY_DEATH)
end

local function nadoxReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    clearState(guid)
end

-- C++ UpdateAI GuardianSummoned latch (every-tick HealthBelowPct(50)
-- check) — modeled in the pre-application DamageTaken handler (event
-- 9): C++-exact HealthBelowPctDamaged(50, damage) = GetHealth()-damage
-- < CountPctFromMaxHealth(50), strict (Unit.h:872). Talk(EMOTE_HATCHES
-- 4, me) + DoCast(me, SPELL_SUMMON_SWARM_GUARD) + latch; the per-GUID
-- latch replaces the C++ bool.
local function nadoxDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = nadoxState[guid]
    if st and not st.guardianSummoned then
        local health = creature:GetHealth()
        local maxHealth = creature:GetMaxHealth()
        if maxHealth ~= 0 and (health - damage) > 0 and health - damage < math.floor(maxHealth * 50 / 100) then
            st.guardianSummoned = true
            creature:Talk(EMOTE_HATCHES, creature)
            creature:CastSpell(creature, SPELL_SUMMON_SWARM_GUARD)
        end
    end
end

RegisterCreatureEvent(ENTRY_ELDER_NADOX, 1, nadoxEnterCombat)
RegisterCreatureEvent(ENTRY_ELDER_NADOX, 2, nadoxLeaveCombat)
RegisterCreatureEvent(ENTRY_ELDER_NADOX, 3, nadoxTargetDied)
RegisterCreatureEvent(ENTRY_ELDER_NADOX, 4, nadoxDied)
RegisterCreatureEvent(ENTRY_ELDER_NADOX, 9, nadoxDamageTaken)
RegisterCreatureEvent(ENTRY_ELDER_NADOX, 23, nadoxReset)
