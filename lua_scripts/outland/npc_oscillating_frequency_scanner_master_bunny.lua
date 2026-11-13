-- Outland: Oscillating Frequency Scanner Master Bunny — Lua port of
-- src/server/scripts/Outland/zone_blades_edge_mountains.cpp
-- (class npc_oscillating_frequency_scanner_master_bunny :
-- CreatureScript("npc_oscillating_frequency_scanner_master_bunny");
-- struct npc_oscillating_frequency_scanner_master_bunnyAI : public ScriptedAI).
-- Entry: 21760 (Wowhead TBC Classic npc=21760, verified this tick; no
-- C++ entry constant exists — DB-side ScriptName binding).
-- Eluna creature events: 4 OnDied, 22 OnSummoned, 23 OnReset.
-- Ported arms (C++ order, zone_blades_edge_mountains.cpp:941-982):
-- - Reset: if a top bunny (NPC_OSCILLATING_FREQUENCY_SCANNER_TOP_BUNNY
--   21759) is within 25.0 yards, despawn self immediately; else summon
--   the top bunny at self pos +0.5z (TEMPSUMMON_TIMED_OR_DEAD_DESPAWN,
--   50s), summon the scanner GO (184926, 50s), then despawn self after
--   50s. The IsDead guard is the marli ev23 idiom: Go fires ev23 on
--   spawn, evade and death, while C++ Reset runs on spawn + evade only.
--   The 50s self-despawn rides a Lua event that cancels the pump
--   first, then despawns: C++ DespawnOrUnsummon(50s) is engine-side,
--   but the Lua pump is a runtime-global CreateLuaEvent (not
--   creature-lifetime-bound) and would orphan past an engine-side
--   despawn, casting from a detached motion — same observable 50s
--   lifetime, no orphan tick.
-- - IsSummonedBy (ev22): if the summoner is a player, latch its GUID.
--   The old "ev22 is pet-funnel-only" header claim is stale: ev22
--   fires on the summon path for creature/player summons with the
--   summoner as the extra argument (lua_creature_events.go — the
--   blackheart-dummy / hodir precedent).
-- - UpdateAI 500ms/3000ms: if the latched player resolves via
--   GetPlayerByGUID (the C++ ObjectAccessor::GetPlayer twin —
--   dawnforge/calvin precedent), DoCast(player, SPELL_OSCILLATION_
--   FIELD 37408) non-triggered. No UpdateVictim gate in C++ — the
--   pump runs for the bunny's whole 50s life, combat or not. A nil
--   player (summoned by a creature, or the player gone) skips the
--   cast but still re-arms 3000ms, like C++.
-- spell_oscillating_field SpellScript (zone_blades_edge_mountains.cpp:
-- 988-1013) PORTED below (RegisterSpellEvent 37408 ev5): hit player
-- with aura 37408 at 5 stacks and quest 10594 INCOMPLETE gets
-- CompleteQuest(10594). Dormant until Spell.dbc ships 37408.

local ENTRY_MASTER_BUNNY = 21760
local NPC_TOP_BUNNY = 21759
local GO_SCANNER = 184926

local SPELL_OSCILLATION_FIELD = 37408

local QUEST_GAUGING_THE_RESONANT_FREQUENCY = 10594 -- zone_blades_edge_mountains.cpp:928
local QUEST_STATUS_INCOMPLETE = 3 -- lua_player.go:4902

local timers = {}
local playerGuid = {}

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

-- C++ UpdateAI (:973-982): 500ms initial / 3000ms repeat; cast the
-- oscillation field on the captured player when it resolves.
local function oscillationTick(creature, guid)
    if creature:IsDead() then
        return
    end
    local player = GetPlayerByGUID(playerGuid[guid])
    if player ~= nil then
        creature:CastSpell(player, SPELL_OSCILLATION_FIELD)
    end
    schedule(guid, "oscillation", 3000, function()
        oscillationTick(creature, guid)
    end)
end

-- C++ npc_oscillating_frequency_scanner_master_bunnyAI::Reset
-- (zone_blades_edge_mountains.cpp:941-957): timer = 500 (feeds the
-- UpdateAI above) -> top-bunny check -> summon branch.
local function scannerReset(event, creature)
    if creature:IsDead() then
        return
    end
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    if creature:FindNearestCreature(NPC_TOP_BUNNY, 25.0) then
        creature:Despawn()
        return
    end
    local x, y, z, o = creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO()
    creature:SummonCreature(NPC_TOP_BUNNY, x, y, z + 0.5, o, 0, 50000)
    creature:SummonGameObject(GO_SCANNER, x, y, z, o, 50)
    schedule(guid, "oscillation", 500, function()
        oscillationTick(creature, guid)
    end)
    schedule(guid, "despawn", 50000, function()
        cancelTimers(guid)
        playerGuid[guid] = nil
        creature:Despawn()
    end)
end

-- C++ IsSummonedBy (:959-963): latch the summoner when it is a player.
local function scannerSummoned(event, creature, summoner)
    if not summoner then
        return
    end
    local player = summoner:ToPlayer()
    if player ~= nil then
        playerGuid[creature:GetGUIDLow()] = player:GetGUID()
    end
end

local function scannerDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    playerGuid[guid] = nil
end

RegisterCreatureEvent(ENTRY_MASTER_BUNNY, 4, scannerDied)
RegisterCreatureEvent(ENTRY_MASTER_BUNNY, 22, scannerSummoned)
RegisterCreatureEvent(ENTRY_MASTER_BUNNY, 23, scannerReset)

-- C++ spell_oscillating_field::HandleEffect
-- (zone_blades_edge_mountains.cpp:996-1001): OnEffectHitTarget,
-- EFFECT_0, SPELL_EFFECT_APPLY_AURA — hit player with aura 37408
-- at 5 stacks and quest 10594 INCOMPLETE completes the quest.
local function oscillatingField(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then return end
    if hitTarget == nil or hitTarget:GetObjectType() ~= "Player" then return end
    local aura = hitTarget:GetAura(SPELL_OSCILLATION_FIELD)
    if aura == nil or aura:GetStackAmount() ~= 5 then return end
    if hitTarget:GetQuestStatus(QUEST_GAUGING_THE_RESONANT_FREQUENCY) == QUEST_STATUS_INCOMPLETE then
        hitTarget:CompleteQuest(QUEST_GAUGING_THE_RESONANT_FREQUENCY)
    end
end
RegisterSpellEvent(SPELL_OSCILLATION_FIELD, 5, oscillatingField)
