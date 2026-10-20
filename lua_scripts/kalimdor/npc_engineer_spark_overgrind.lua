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
-- - Reset's UNIT_NPC_FLAGS restore (zone_azuremyst_isle.cpp:224):
--   constructor-captured value cached per GUID, restored via
--   creature:SetNPCFlags() (engine/world/lua_objects.go:657) — C++
--   arm order after Initialize(); the old "no Lua bridge" claim was
--   stale for NPC flags (SetFaction is now bridged too, but the
--   gossip-select flip still needs Attack — zero Lua usage — so the
--   arm stays documented-only).
-- Documented-only (no bridges):
-- - IsTreeEvent gate (GetAreaId() == 3579 cove / 3639 isle returns
--   before combat entirely): GetAreaId has no Lua bridge (talbot
--   precedent, npc_blessed_banner.lua comment). The geezle-event
--   Spark therefore still attacks on the live wire; tree-event area
--   gating is unmodeled.
-- - OnGossipSelect (CloseGossipMenuFor + SetFaction(FACTION_MONSTER)
--   + Attack): gossip IS bridged (RegisterCreatureGossipEvent — the
--   "gossip bridge absent" claim was stale; live at
--   engine/world/gossip.go:129/369); SetFaction is now bridged on the
--   event surface but Attack has zero Lua usage (ouro), so the arm
--   stays documented-only.
-- - Reset's NormFaction restore: SetFaction is now bridged, but the
--   only runtime flip is the gossip-select arm above (still Attack-
--   blocked), so the restore stays documented-only.
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

-- guid -> AI-constructor-captured UNIT_NPC_FLAGS
-- (zone_azuremyst_isle.cpp:205: NpcFlags = creature->GetUInt32Value(UNIT_NPC_FLAGS))
local npcFlags = {}

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
    -- zone_azuremyst_isle.cpp:224 — Reset restores the constructor-captured
    -- UNIT_NPC_FLAGS via SetUInt32Value; the SetFaction(NormFaction) leg
    -- (:223) stays documented-only (bridge exists; the only runtime flip
    -- is the Attack-blocked gossip-select arm). creature:SetNPCFlags()
    -- (engine/world/lua_objects.go:657); Go fires event 23 only on
    -- respawn/evade (lua_creature_events.go:539-549), so the capture
    -- lands at first reset — equal to the C++ constructor capture,
    -- since nothing mutates this creature's NPC flags before then.
    if npcFlags[guid] == nil then
        npcFlags[guid] = creature:GetNPCFlags()
    end
    creature:SetNPCFlags(npcFlags[guid])
    -- C++ UpdateAI never runs while dead (zone_azuremyst_isle.cpp:238-261);
    -- a death-reset (23 fires before 4 on the kill path) must not re-arm
    -- the emote timer, or the corpse Talks every 120-150s until respawn.
    if not creature:IsDead() then
        schedule(guid, "emote", math.random(EMOTE_MIN_MS, EMOTE_MAX_MS), function()
            onEmote(creature, guid)
        end)
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 23, onReset)
