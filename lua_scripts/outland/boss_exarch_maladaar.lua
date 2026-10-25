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
-- bridge). On latch: the InterruptNonMeleeSpells leg is correctly
-- skipped (IsNonMeleeSpellCast reads constant-false under the
-- caststate axiom; the method itself is a bridged no-op twin — the
-- old "no bridge" note was stale); Talk(SAY_SUMMON 1); DoCast(me, SPELL_SUMMON_AVATAR
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
-- interrupt legs ported (InterruptNonMeleeSpells(true), C++ :279; avatar arm :263 also ported); Talk(SAY_ROAR 3);
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
-- a per-GUID never-cleared latch (HasTaunted is ctor-only in
-- C++ — Initialize() resets timers/avatar but never touches it,
-- ossirian precedent). The handler returns nothing (no
-- veto): the C++ body calls ScriptedAI::MoveInLineOfSight(who)
-- after the taunt check, so the default aggro engage proceeds
-- (the inverse of the shaffar depth_charge veto precedent).
-- Fire-site note: Go fires event 27 from the aggro-engage branch
-- (dist <= aggro range < 150, who always a player), a SUBSET of
-- the C++ trigger — the intro fires on first engage rather than
-- at 150y sight, both C++ gates holding at fire time.
-- boss_exarch_maladaarAI::Reset — Initialize() re-latch: the Lua
-- per-GUID timer tables reset on 2/4/23 (the gargolmar
-- precedent); avatarLatched re-latches too. HasTaunted does NOT
-- re-latch (ctor-only) and is never cleared.
-- DOCUMENTED-ONLY (in this header; only entry 18373 registered):
-- boss_exarch_maladaarAI::JustSummoned (:204-209, ENTRY_STOLEN_SOUL
-- 18441) — summoned:CastSpell(summoned, SPELL_STOLEN_SOUL_VISUAL
-- 32395, false) (triggered self-cast — the visual's shapeshift is
-- unimplemented in TC per the file comment) +
-- SetDisplayId(soulmodel) + SetFaction(me->GetFaction()) +
-- ENSURE_AI SetMyClass(soulclass) + AI()->AttackStart(soulholder
-- target) — PORTED 2026-10-24 (see stolenSoulTick): the summon
-- returns the soul object, so SetMyClass is a soulClass table store,
-- AttackStart is EngageWithTarget, and SetDisplayId(soulmodel) is
-- PORTED 2026-10-24 via the player:GetDisplayId bridge (lua_player.go).
-- StolenSoul_Timer summon leg (:256) — PORTED 2026-10-24 in
-- stolenSoulTick (SummonCreature type 3, 10s + JustSummoned arms).
-- boss_exarch_maladaarAI::JustDied D'ore leg (:248-253) — PORTED
-- (me->SummonCreature(19412, TEMPSUMMON_MANUAL_DESPAWN) on the live
-- summon bridge; the arm carries no AttackStart/cross-AI follow-ups —
-- those belong to the stolen-soul JustSummoned leg above).
-- DoMeleeAttackIfReady — no melee bridge (the alar precedent).
-- npc_stolen_soul (18441) — REGISTERED 2026-10-24 (soulEnterCombat /
-- soulCleanup below): the myClass switch runs off soulClass, and
-- evade/reset clears it (C++ Reset re-inits myClass to NONE).
-- Transcribed from
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
local ENTRY_DORE = 19412 -- D'ore (boss_exarch_maladaar.cpp:252)

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

local ENTRY_STOLEN_SOUL = 18441 -- boss_exarch_maladaar.cpp:156
local SPELL_STOLEN_SOUL_VISUAL = 32395 -- boss_exarch_maladaar.cpp:153

-- C++ npc_stolen_soulAI myClass switch (boss_exarch_maladaar.cpp:94-132):
-- class id (TrinityCore Classes enum) -> {spell, re-arm ms}. Set via
-- SetMyClass from the summoner; the soul's own handlers read it from
-- soulClass below (no cross-AI bridge is needed — the summon returns
-- the soul object and the class is stored at summon time).
local CLASS_SPELLS = {
    [1] = {37335, 6000},  -- warrior: mortal strike
    [2] = {37369, 6000},  -- paladin: hammer of justice
    [3] = {37368, 20000}, -- hunter: freezing trap
    [4] = {37331, 10000}, -- rogue: hemorrhage
    [5] = {37330, 5000},  -- priest: mind flay
    [7] = {37332, 8000},  -- shaman: frostshock
    [8] = {37329, 5000},  -- mage: fireball
    [9] = {37334, 20000}, -- warlock: curse of agony
    [11] = {37328, 10000}, -- druid: moonfire
}
local soulClass = {} -- per-soul-GUID myClass (C++ npc_stolen_soulAI:70)

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

-- Avatar_summoned IS re-latched by C++ Reset()->Initialize(),
-- so avatarLatched clears here; HasTaunted is ctor-only and is
-- never cleared (see MoveInLineOfSight note above).
local function clearState(guid)
    cancelTimers(guid)
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
-- itself for the stolen-soul arm. The pool is alive-filtered: C++
-- threat refs go offline on death (ThreatManager.cpp
-- ShouldBeOffline / _IsTargetAcceptable) and SelectTargetList skips
-- offline refs, while Go's SortedEntries returns dead entries too.
local function randomThreatTarget(creature)
    local pool = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t and t:IsAlive() then
            pool[#pool + 1] = t
        end
    end
    if #pool == 0 then
        return nil
    end
    return pool[math.random(#pool)]
end

-- C++ StolenSoul_Timer (:242-262).
local function stolenSoulTick(creature, guid)
    local target = randomThreatTarget(creature)
    if target and target:GetObjectType() == "Player" then
        -- C++ :279: InterruptNonMeleeSpells(true) before Talk; the
        -- IsNonMeleeSpellCast gate has no bridge but is moot for the no-op twin.
        creature:InterruptNonMeleeSpells(true)
        creature:Talk(SAY_ROAR)
        creature:CastSpell(target, SPELL_STOLEN_SOUL)
        -- C++ StolenSoul_Timer summon leg (:256) + JustSummoned
        -- (:204-209): SummonCreature(18441, 0,0,0,0,
        -- TIMED_DESPAWN_OUT_OF_COMBAT type 3, 10s) then the
        -- JustSummoned arms — visual self-cast, faction copy,
        -- SetMyClass (stored in soulClass), AttackStart(soulholder).
        -- SetDisplayId(soulmodel) PORTED 2026-10-24 via the
        -- player:GetDisplayId bridge (lua_player.go).
        local soul = creature:SummonCreature(ENTRY_STOLEN_SOUL, 0, 0, 0, 0, 3, 10000)
        if soul then
            soul:CastSpell(soul, SPELL_STOLEN_SOUL_VISUAL, false)
            soul:SetDisplayId(target:GetDisplayId())
            soul:SetFaction(creature:GetFaction())
            soulClass[soul:GetGUID()] = target:GetClass()
            soul:EngageWithTarget(target)
        end
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
            -- C++ :263: InterruptNonMeleeSpells(true) before Talk; the
            -- IsNonMeleeSpellCast gate has no bridge but is moot for the no-op twin.
            creature:InterruptNonMeleeSpells(true)
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

-- C++ boss_exarch_maladaarAI::JustDied (:219-224, Talk(SAY_DEATH 5)) +
-- the D'ore leg (:248-253): me->SummonCreature(19412, -4.40722f,
-- -387.277f, 40.6294f, 6.26573f, TEMPSUMMON_MANUAL_DESPAWN) — the arm
-- carries no follow-up legs (the AttackStart/SetMyClass arms are
-- ported in the stolen-soul leg above), so it ports whole on the
-- live summon bridge (lua_creature_events.go:877).
local function maladaarDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    creature:SummonCreature(ENTRY_DORE, -4.40722, -387.277, 40.6294, 6.26573, 7)
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

-- C++ npc_stolen_soulAI (boss_exarch_maladaar.cpp:62-137): Class_Timer
-- 1000 init, then the myClass victim cast + class re-arm; melee is
-- engine-driven. JustEngagedWith is empty in C++ — the schedule
-- starts on event 1 (the soul engages its soulholder at summon).
-- C++ Reset re-inits myClass to NONE, so evade/reset clears the
-- class: a re-engaged soul casts nothing, as in C++.
local function stolenSoulClassTick(creature, guid)
    local arm = CLASS_SPELLS[soulClass[guid]]
    if arm then
        creature:CastSpell(nil, arm[1], false) -- DoCastVictim
    end
    schedule(guid, "class", arm and arm[2] or 1000, function()
        stolenSoulClassTick(creature, guid)
    end)
end

local function stolenSoulEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "class", 1000, function()
        stolenSoulClassTick(creature, guid)
    end)
end

local function stolenSoulCleanup(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    soulClass[guid] = nil
end

RegisterCreatureEvent(ENTRY_STOLEN_SOUL, 1, stolenSoulEnterCombat)
RegisterCreatureEvent(ENTRY_STOLEN_SOUL, 2, stolenSoulCleanup)
RegisterCreatureEvent(ENTRY_STOLEN_SOUL, 4, stolenSoulCleanup)
RegisterCreatureEvent(ENTRY_STOLEN_SOUL, 23, stolenSoulCleanup)
