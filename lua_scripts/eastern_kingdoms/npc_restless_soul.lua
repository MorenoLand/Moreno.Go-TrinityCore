-- Restless Soul (11122, Stratholme) -- Lua port of the
-- npc_restless_soulAI arms in
-- src/server/scripts/EasternKingdoms/Stratholme/stratholme.cpp
-- (one script "npc_restless_soul", ScriptedAI; EK loader decl :138 /
-- call :316 via AddSC_stratholme; DB-side ScriptName binding, entry
-- 11122 from the file's own RestlessSoul enum). Quest 5282
-- (QUEST_RESTLESS_SOUL): the player tags the soul with Egan Blaster
-- 17368; 5s later the soul dies, credits the tagger, and frees a
-- Freed Soul (11136). The freed soul itself has no script (default
-- AI) and is NOT registered here.
-- Eluna creature events: 2 OnLeaveCombat, 4 OnDied, 14 OnHitBySpell,
-- 19 OnJustSummonedCreature, 23 OnReset. No EnterCombat arm: C++
-- JustEngagedWith is empty and the die timer runs behind no
-- UpdateVictim gate, so it is armed from the tag (event 14), not
-- from combat. Melee is engine-driven (DoMeleeAttackIfReady never
-- reached: the soul dies 5s after tagging).
--
-- Ported arms (C++-exact for the modeled arms):
-- - SpellHit (stratholme.cpp:149-160): already-tagged, wrong-spell,
--   non-player-caster, and quest-not-incomplete all return early;
--   else Tagged = true, Tagger = caster GUID. Caster gate via the
--   GetObjectType() == "Player" convention (beryl-sorcerer
--   precedent); quest gate is INCOMPLETE (3, QuestDef.h:106).
-- - UpdateAI Die_Timer (stratholme.cpp:176-188): 5s after the tag,
--   resolve the tagger and credit + kill. C++ resolves map-wide via
--   ObjectAccessor::GetUnit(Tagger) and re-checks every AI tick while
--   unresolved; the Lua surface has no player-by-GUID bridge, so
--   the tagger is found by GUID among creature:GetPlayersInGrid
--   (100yd scan). The 5s movement bound makes 100yd effectively
--   exact (mounted speed covers < 75yd in 5s); a miss re-arms 1s
--   (jeklik gate-retry precedent), matching C++'s per-tick
--   re-check. KilledMonsterCredit(11122) is the player bridge
--   (lua_player.go); Kill() is the Creature::KillSelf twin.
-- - JustDied (stratholme.cpp:170-174): if tagged, SummonCreature
--   (11136 at own position, C++ TEMPSUMMON_TIMED_DESPAWN = 3 ->
--   Go type 2, 300000ms, willey/ramstein numbering precedent).
-- - JustSummoned (stratholme.cpp:162-168): the freed soul self-casts
--   SPELL_SOUL_FREED 17370 non-triggered (ev19 fires on the
--   summoner via the Lua SummonCreature funnel, hydross
--   precedent).
--
-- Documented-unmodeled (no bridges):
-- - JustSummoned MoveFollow(player, 0, 0) leg (no MoveFollow bridge,
--   vorpil-traveler precedent); the self-cast leg above is ported.
-- - Reset() latch clear: C++ Reset calls Initialize() (Tagged =
--   false), but the Go kill funnel fires event 23 (Reset) BEFORE
--   event 4 (Died) — clearing the latch on Reset would make onDied
--   see an untagged soul and skip the freed summon (ghostly-citizen
--   precedent). onReset cancels timers only; evade clears the latch
--   via onLeaveCombat; death consumes it in onDied.
-- - GetPlayersInGrid filters dead/ghost players, so a tagger who
--   died or ghosted within the 5s window is missed until they
--   revive (C++ would credit + kill immediately); the 1s re-arm
--   covers the revive case.
-- - GetStratholmeAI factory template (no logic); the C++ header's
--   "summoned along with three other elites" note is spawn data,
--   not scripted arms.

local ENTRY_RESTLESS_SOUL = 11122
local ENTRY_FREED_SOUL = 11136

local SPELL_EGAN_BLASTER = 17368
local SPELL_SOUL_FREED = 17370

local QUEST_RESTLESS_SOUL = 5282
local QUEST_STATUS_INCOMPLETE = 3

local tagged = {}
local tagger = {}
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

-- C++ UpdateAI Die_Timer: 5s after the tag, credit the tagger and
-- KillSelf; unresolved tagger re-checks every AI tick (1s re-arm
-- here). The tagger is the only player in range with the stored
-- GUID; GetPlayersInGrid is the only player-lookup bridge.
local function onDieTimer(creature, guid)
    local taggerGuid = tagger[guid]
    local taggerPlayer = nil
    for _, player in ipairs(creature:GetPlayersInGrid(100)) do
        if player:GetGUID() == taggerGuid then
            taggerPlayer = player
            break
        end
    end
    if taggerPlayer then
        taggerPlayer:KilledMonsterCredit(ENTRY_RESTLESS_SOUL)
        creature:Kill()
    else
        schedule(guid, "die", 1000, function()
            onDieTimer(creature, guid)
        end)
    end
end

-- C++ SpellHit: tag only on EGAN_BLASTER from a player whose quest
-- 5282 is incomplete; first tag wins (Tagged latch).
local function onSpellHit(event, creature, caster, spellId)
    local guid = creature:GetGUID()
    if tagged[guid] or spellId ~= SPELL_EGAN_BLASTER then
        return
    end
    if not caster or caster:GetObjectType() ~= "Player" then
        return
    end
    if caster:GetQuestStatus(QUEST_RESTLESS_SOUL) ~= QUEST_STATUS_INCOMPLETE then
        return
    end
    tagged[guid] = true
    tagger[guid] = caster:GetGUID()
    schedule(guid, "die", 5000, function()
        onDieTimer(creature, guid)
    end)
end

-- C++ JustSummoned: freed soul self-casts SOUL_FREED; the
-- MoveFollow leg stays out (no bridge).
local function onJustSummoned(event, creature, summoned)
    if summoned and summoned:GetEntry() == ENTRY_FREED_SOUL then
        summoned:CastSpell(summoned, SPELL_SOUL_FREED)
    end
end

-- C++ JustDied: tagged souls free one soul at their own position.
local function onDied(event, creature)
    local guid = creature:GetGUID()
    if tagged[guid] then
        local x, y, z, o = creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO()
        creature:SummonCreature(ENTRY_FREED_SOUL, x, y, z, o, 2, 300000)
    end
    tagged[guid] = nil
    tagger[guid] = nil
    cancelTimers(guid)
end

-- Evade clears the latch (C++ Reset semantics); Reset itself must
-- NOT (kill funnel fires 23 before 4 — see header).
local function onLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    tagged[guid] = nil
    tagger[guid] = nil
    cancelTimers(guid)
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_RESTLESS_SOUL, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_RESTLESS_SOUL, 4, onDied)
RegisterCreatureEvent(ENTRY_RESTLESS_SOUL, 14, onSpellHit)
RegisterCreatureEvent(ENTRY_RESTLESS_SOUL, 19, onJustSummoned)
RegisterCreatureEvent(ENTRY_RESTLESS_SOUL, 23, onReset)
