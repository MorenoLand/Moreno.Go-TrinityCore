-- Exarch Maladaar (Auchenai Crypts, Auchindoun) — Lua port of
-- src/server/scripts/Outland/Auchindoun/AuchenaiCrypts/boss_exarch_maladaar.cpp
-- (2 CreatureScripts: boss_exarch_maladaar (ScriptedAI via
-- GetAuchenaiCryptsAI) + npc_stolen_soul (ScriptedAI); both
-- registered from inside AddSC_boss_exarch_maladaar()
-- (boss_exarch_maladaar.cpp:313-317); loader decl 21 / call 144 per
-- outland_script_loader.cpp — the SECOND call of the
-- "// Auchindoun - Auchenai Crypts" sub-block in
-- AddOutlandScripts(), immediately after
-- AddSC_boss_shirrak_the_dead_watcher() (call 143) — verified from
-- the loader this run; sole-source verified: whole-server-tree
-- grep for "AddSC_boss_exarch_maladaar" hits
-- boss_exarch_maladaar.cpp (+ the loader decl/call lines) only).
-- Entry: 18373 — DB-side (no NPC_ constant for the boss in the
-- file); verified this run against the wowhead TBC Classic NPC
-- page (/tbc/npc=18373/exarch-maladaar — page title "Exarch
-- Maladaar - NPC - TBC Classic", second and final boss of
-- Auchenai Crypts).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 23 OnReset, 27 OnMoveInLOS.
-- Ported arms (C++-exact for all modeled arms):
-- boss_exarch_maladaarAI::JustEngagedWith (:185-188) —
-- Talk(SAY_AGGRO 2) (event 1 — the auriaya precedent); the combat
-- timers start here mirroring the C++ UpdateVictim-gated block:
-- Fear_timer 15000+rand(5000), Ribbon_of_Souls_timer 5000,
-- StolenSoul_Timer 25000+rand(10000).
-- Avatar latch (:230-240) — the C++ checks
-- !Avatar_summoned && HealthBelowPct(25) every UpdateAI tick:
-- ported as a 1s repeating checker (the per-tick check has no
-- bridge). On latch: the IsNonMeleeSpellCast /
-- InterruptNonMeleeSpells legs have no bridge (the shaffar blink
-- precedent); Talk(SAY_SUMMON 1); DoCast(me, SPELL_SUMMON_AVATAR
-- 32424) 2-arg (non-triggered) self-cast — CastSpell(creature,
-- spell); Avatar_summoned latches; StolenSoul_Timer re-armed
-- 15000+rand(15000). HealthBelowPct via creature:GetHealthPct()
-- (the sindragosa generic-fallback precedent — object.go).
-- StolenSoul_Timer (:242-262) — SelectTarget(Random, 0): UnitAI.h
-- :162-174 — offset 0 = random over the full threat list in
-- MaxThreat order (no skip). playerOnly defaults false, so the
-- C++ then gates target->GetTypeId() == TYPEID_PLAYER explicitly:
-- ported via GetAITargets (the bridged threat list; midnight
-- precedent) with NO player pre-filter; a non-"Player" pick
-- re-arms 1000ms (the C++ retry leg — the illidan
-- victim:GetObjectType() == "Player" convention); an empty pool
-- also re-arms 1000ms (the C++ leaves the timer expired so it
-- retries on the next UpdateAI tick). On a player pick: the
-- interrupt legs have no bridge; Talk(SAY_ROAR 3);
-- DoCast(target, SPELL_STOLEN_SOUL 32346) 2-arg (non-triggered) —
-- CastSpell(target, spell); re-arm 20000+rand(10000).
-- Ribbon_of_Souls (:264-270) — SelectTarget(Random, 0) with NO
-- player gate in C++: random threat target, DoCast(target,
-- SPELL_RIBBON_OF_SOULS 32422) 2-arg; re-arm
-- 5000 + rand(0,19)*1000 (the C++ 5000 + (rand32() % 20 * 1000)).
-- Fear (:272-276) — DoCast(me, SPELL_SOUL_SCREAM 32421) 2-arg
-- self-cast; re-arm 15000+rand(15000).
-- boss_exarch_maladaarAI::KilledUnit (:211-217) — rand32() % 2
-- gate: Talk(SAY_SLAY 4) on math.random(2) == 1 (the curator
-- precedent) (event 3).
-- boss_exarch_maladaarAI::JustDied (:219-224) — Talk(SAY_DEATH 5)
-- (event 4 — the sjonnir precedent).
-- boss_exarch_maladaarAI::MoveInLineOfSight (:192-202) — the
-- HasTaunted one-shot Talk(SAY_INTRO 0) when a unit is within
-- 150.0f: PORTED on event 27 (CREATURE_EVENT_ON_MOVE_IN_LOS) with
-- a per-GUID latch (latch cleared on 2/4/23 — C++ Reset()
-- re-latches via Initialize). The handler returns nothing (no
-- veto): the C++ body calls ScriptedAI::MoveInLineOfSight(who)
-- after the taunt check, so the default aggro engage proceeds
-- (the inverse of the shaffar depth_charge veto precedent).
-- Fire-site note: Go fires event 27 from the aggro-engage branch
-- (dist <= aggro range < 150, who always a player), a SUBSET of
-- the C++ trigger — the intro fires on first engage rather than
-- at 150y sight, both C++ gates holding at fire time.
-- boss_exarch_maladaarAI::Reset — Initialize() re-latch: the Lua
-- per-GUID timer tables reset on 2/4/23 (the gargolmar
-- precedent); avatarLatched re-latches too.
-- DOCUMENTED-ONLY (in this header; only entry 18373 registered):
-- boss_exarch_maladaarAI::JustSummoned (:204-209, ENTRY_STOLEN_SOUL
-- 18441) — summoned:CastSpell(summoned, SPELL_STOLEN_SOUL_VISUAL
-- 32395, false) (triggered self-cast — the visual's shapeshift is
-- unimplemented in TC per the file comment) +
-- SetDisplayId(soulmodel) + SetFaction(me->GetFaction()) +
-- ENSURE_AI SetMyClass(soulclass) + AI()->AttackStart(soulholder
-- target) — no cross-creature AI-method / AttackStart bridges
-- (the maladaar JustSummoned-coordination precedent cited by the
-- shaffar port).
-- StolenSoul_Timer summon leg (:256) — me->SummonCreature(
-- ENTRY_STOLEN_SOUL 18441, 0.0f, 0.0f, 0.0f, 0.0f,
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT, 10s) — the SummonCreature
-- bridge exists, but the leg stays out on the unbridged AttackStart
-- + cross-AI SetMyClass arms (shirrak precedent); the captured
-- soulmodel/soulholder/soulclass exist only for this leg.
-- boss_exarch_maladaarAI::JustDied D'ore leg (:248-253) —
-- me->SummonCreature(19412, -4.40722f, -387.277f, 40.6294f,
-- 6.26573f, TEMPSUMMON_MANUAL_DESPAWN) — the bridge exists, but the
-- leg stays out on the unbridged AttackStart + cross-AI arms (same
-- precedent as the stolen-soul leg).
-- DoMeleeAttackIfReady — no melee bridge (the alar precedent).
-- npc_stolen_soul (18441) — NOT registered (nothing can summon it
-- without the summon bridge; the yor precedent). Transcribed from
-- boss_exarch_maladaar.cpp:60-133 for the record: Class_Timer 1s
-- init; the myClass switch (set via SetMyClass from the
-- summoner): WARRIOR -> DoCastVictim(SPELL_MORTAL_STRIKE 37335)
-- 6s; PALADIN -> SPELL_HAMMER_OF_JUSTICE 37369 6s; HUNTER ->
-- SPELL_FREEZING_TRAP 37368 20s; ROGUE -> SPELL_HEMORRHAGE 37331
-- 10s; PRIEST -> SPELL_MIND_FLAY 37330 5s; SHAMAN ->
-- SPELL_FROSTSHOCK 37332 8s; MAGE -> SPELL_FIREBALL 37329 5s;
-- WARLOCK -> SPELL_CURSE_OF_AGONY 37334 20s; DRUID ->
-- SPELL_MOONFIRE 37328 10s — all 2-arg (non-triggered)
-- victim casts + DoMeleeAttackIfReady.

local ENTRY_MALADAAR = 18373 -- DB-side; wowhead-TBC-verified this run

local SAY_INTRO  = 0
local SAY_SUMMON = 1
local SAY_AGGRO  = 2
local SAY_ROAR   = 3
local SAY_SLAY   = 4
local SAY_DEATH  = 5

local SPELL_RIBBON_OF_SOULS = 32422 -- boss_exarch_maladaar.cpp:128
local SPELL_SOUL_SCREAM     = 32421 -- boss_exarch_maladaar.cpp:129
local SPELL_STOLEN_SOUL     = 32346 -- boss_exarch_maladaar.cpp:130
local SPELL_SUMMON_AVATAR   = 32424 -- boss_exarch_maladaar.cpp:132

local timers = {}
local taunted = {}       -- per-GUID HasTaunted latch (C++ :181)
local avatarLatched = {} -- per-GUID Avatar_summoned (C++ :179)

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function clearState(guid)
    cancelTimers(guid)
    taunted[guid] = nil
    avatarLatched[guid] = nil
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

-- C++ SelectTarget(Random, 0): random over the full threat list
-- (no offset skip). No player pre-filter — the C++ gates the pick
-- itself for the stolen-soul arm.
local function randomThreatTarget(creature)
    local pool = creature:GetAITargets()
    if #pool == 0 then
        return nil
    end
    return pool[math.random(#pool)]
end

-- C++ StolenSoul_Timer (:242-262).
local function stolenSoulTick(creature, guid)
    local target = randomThreatTarget(creature)
    if target and target:GetObjectType() == "Player" then
        -- The interrupt legs have no bridge (header).
        creature:Talk(SAY_ROAR)
        -- soulmodel/soulholder/soulclass exist only for the
        -- no-bridge summon leg (documented in the header).
        creature:CastSpell(target, SPELL_STOLEN_SOUL)
        schedule(guid, "stolen_soul", {20000, 29999}, function()
            stolenSoulTick(creature, guid)
        end)
    else
        -- C++ non-player pick: StolenSoul_Timer = 1000 retry; empty
        -- pool: timer stays expired -> next-tick retry.
        schedule(guid, "stolen_soul", 1000, function()
            stolenSoulTick(creature, guid)
        end)
    end
end

-- C++ Ribbon_of_Souls (:264-270): no player gate on the pick.
local function ribbonTick(creature, guid)
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_RIBBON_OF_SOULS)
    end
    schedule(guid, "ribbon", 5000 + math.random(0, 19) * 1000, function()
        ribbonTick(creature, guid)
    end)
end

-- C++ Fear (:272-276).
local function fearTick(creature, guid)
    creature:CastSpell(creature, SPELL_SOUL_SCREAM)
    schedule(guid, "fear", {15000, 29999}, function()
        fearTick(creature, guid)
    end)
end

-- C++ Avatar latch (:230-240): checked every UpdateAI tick — the
-- 1s checker is the bridgeable approximation.
local function avatarCheck(creature, guid)
    if not avatarLatched[guid] then
        local maxHealth = creature:GetMaxHealth()
        if maxHealth > 0
                and creature:GetHealth() < math.floor(maxHealth * 25 / 100) then
            -- The interrupt legs have no bridge (header).
            creature:Talk(SAY_SUMMON)
            creature:CastSpell(creature, SPELL_SUMMON_AVATAR)
            avatarLatched[guid] = true
            schedule(guid, "stolen_soul", {15000, 29999}, function()
                stolenSoulTick(creature, guid)
            end)
        end
    end
    schedule(guid, "avatar_check", 1000, function()
        avatarCheck(creature, guid)
    end)
end

-- C++ boss_exarch_maladaarAI::JustEngagedWith (:185-188).
local function maladaarEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    clearState(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "fear", {15000, 19999}, function()
        fearTick(creature, guid)
    end)
    schedule(guid, "ribbon", 5000, function()
        ribbonTick(creature, guid)
    end)
    schedule(guid, "stolen_soul", {25000, 34999}, function()
        stolenSoulTick(creature, guid)
    end)
    schedule(guid, "avatar_check", 1000, function()
        avatarCheck(creature, guid)
    end)
end

local function maladaarLeaveCombat(event, creature)
    clearState(creature:GetGUID())
end

local function maladaarReset(event, creature)
    clearState(creature:GetGUID())
end

-- C++ boss_exarch_maladaarAI::KilledUnit (:211-217): 50% gate.
local function maladaarTargetDied(event, creature, victim)
    if math.random(2) == 1 then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ boss_exarch_maladaarAI::JustDied (:219-224): the D'ore
-- summon leg has no bridge (header).
local function maladaarDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    clearState(creature:GetGUID())
end

-- C++ boss_exarch_maladaarAI::MoveInLineOfSight (:192-202).
-- No veto: the C++ calls the base AI, so aggro proceeds.
local function maladaarMoveInLOS(event, creature, player)
    local guid = creature:GetGUID()
    if not taunted[guid]
            and creature:IsWithinDistInMap(player, 150.0) then
        taunted[guid] = true
        creature:Talk(SAY_INTRO)
    end
end

RegisterCreatureEvent(ENTRY_MALADAAR, 1, maladaarEnterCombat)
RegisterCreatureEvent(ENTRY_MALADAAR, 2, maladaarLeaveCombat)
RegisterCreatureEvent(ENTRY_MALADAAR, 3, maladaarTargetDied)
RegisterCreatureEvent(ENTRY_MALADAAR, 4, maladaarDied)
RegisterCreatureEvent(ENTRY_MALADAAR, 23, maladaarReset)
RegisterCreatureEvent(ENTRY_MALADAAR, 27, maladaarMoveInLOS)
