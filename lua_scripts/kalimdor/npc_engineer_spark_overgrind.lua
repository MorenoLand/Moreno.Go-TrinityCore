-- Azuremyst Isle: Engineer Spark Overgrind — Lua port of
-- src/server/scripts/Kalimdor/zone_azuremyst_isle.cpp
-- (npc_engineer_spark_overgrindAI : public ScriptedAI; registered in
-- AddSC_azuremyst_isle(), kalimdor loader decl :108).
-- Entry: 17243 (NPC_SPARK, in-enum in the Geezle enum block).
-- Ported arms:
-- - JustEngagedWith -> Talk(ATTACK_YELL = 2) on the engager.
-- - Dynamite_Timer: DoCastVictim(SPELL_DYNAMITE = 7978), init 8000
--   -> 8000, gated on creature:GetVictim() (C++ UpdateVictim gate);
--   timers via CreateLuaEvent, scheduled on enter combat, cancelled
--   on leave combat / reset (boss_ouro precedent).
-- - Emote_Timer (out-of-combat only): Talk(SAY_TEXT = 0) +
--   Talk(SAY_EMOTE = 1), init urand(120000,150000) -> same range;
--   armed at spawn (event 23 fires on spawn), cancelled on enter
--   combat, re-armed on leave combat. C++ resumes a partially
--   elapsed timer after combat; the Lua port re-arms full delay.
-- - DoMeleeAttackIfReady — engine-driven in Go (ouro precedent).
-- Documented-only (no bridges):
-- - IsTreeEvent gate (GetAreaId() == 3579 cove / 3639 isle returns
--   before combat entirely): GetAreaId has no Lua bridge (talbot
--   precedent, npc_blessed_banner.lua comment). The geezle-event
--   Spark therefore still attacks on the live wire; tree-event area
--   gating is unmodeled.
-- - OnGossipSelect (CloseGossipMenuFor + SetFaction(FACTION_MONSTER)
--   + Attack): gossip bridge absent (ashenvale go_naga_brazier
--   precedent); SetFaction has zero Lua usage (ouro).
-- - Reset's NormFaction / NpcFlags restore: SetFaction and
--   SetUInt32Value(UNIT_NPC_FLAGS) have no Lua bridge.
-- - The rest of zone_azuremyst_isle.cpp, documented-only:
--   npc_draenei_survivor (MoveInLineOfSight help-call +
--   SpellHit heal leg + timed thanks/run-away) — entry ID not
--   C++-verifiable (no NPC constant; azshara wailing_caverns /
--   ashenvale cannot-verify precedent), SpellFamilyFlags / MovePoint /
--   stand-state legs have no bridge.
--   npc_injured_draenei — cosmetic only (IN_COMBAT flag + 15% health
--   + sit/sleep standstate); unit-flag bridge absent.
--   npc_magwin — EscortAI (OnQuestAccept / WaypointReached /
--   events): no escort / quest bridge (ashenvale npc_muglash
--   precedent).
--   npc_geezle — SummonCreature has zero Lua usage (zulfarrak
--   go_shallow_grave precedent); MotionMaster, quest-status,
--   gameobject-respawn legs absent bridges.
--   spell_inoculate_nestlewood — AuraScript/PeriodicTick:
--   no SpellScript bridge (ashenvale spell_destroy_karangs_banner
--   precedent).
local ENTRY = 17243

local SAY_TEXT = 0
local SAY_EMOTE = 1
local ATTACK_YELL = 2

local SPELL_DYNAMITE = 7978

local DYNAMITE_DELAY_MS = 8000
local EMOTE_MIN_MS = 120000
local EMOTE_MAX_MS = 150000

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

local function onDynamite(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_DYNAMITE)
    end
    schedule(guid, "dynamite", DYNAMITE_DELAY_MS, function()
        onDynamite(creature, guid)
    end)
end

local function onEmote(creature, guid)
    if not creature:GetVictim() then
        creature:Talk(SAY_TEXT)
        creature:Talk(SAY_EMOTE)
    end
    schedule(guid, "emote", math.random(EMOTE_MIN_MS, EMOTE_MAX_MS), function()
        onEmote(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(ATTACK_YELL, target)
    schedule(guid, "dynamite", DYNAMITE_DELAY_MS, function()
        onDynamite(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "emote", math.random(EMOTE_MIN_MS, EMOTE_MAX_MS), function()
        onEmote(creature, guid)
    end)
end

local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "emote", math.random(EMOTE_MIN_MS, EMOTE_MAX_MS), function()
        onEmote(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 23, onReset)
