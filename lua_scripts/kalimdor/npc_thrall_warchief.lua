-- Orgrimmar: Thrall, Warchief of the Horde — combat rotation port of
-- src/server/scripts/Kalimdor/zone_orgrimmar.cpp
-- (npc_thrall_warchiefAI : public ScriptedAI; registered in
-- AddSC_orgrimmar(), kalimdor loader decl :113 / call :226).
-- Entry (wowhead WotLK-verified 2026-10-18): 4949 — "Thrall
-- <Warchief>", Orgrimmar (Grommash Hold, Valley of Wisdom);
-- https://www.wowhead.com/wotlk/npc=4949/thrall. The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace).
-- Ported arms (C++-exact):
-- - JustEngagedWith -> DoPlaySoundToSet(me, SOUND_AGGRO 5880):
--   creature:PlayDirectSound(5880) (SMSG_PLAY_SOUND, bosses_opera
--   precedent).
-- - Chain lightning: DoCastVictim(SPELL_CHAIN_LIGHTNING 16033),
--   non-triggered (2-arg DoCast, vaelastrasz convention), init 2000
--   -> 9000. Shock: DoCastVictim(SPELL_SHOCK 16034), non-triggered,
--   init 8000 -> 15000. Both gated on creature:GetVictim() (the C++
--   !UpdateVictim() early return); timers via CreateLuaEvent, armed
--   on enter combat, cancelled on leave combat / died / reset
--   (timers tick only in combat in the C++).
-- - DoMeleeAttackIfReady — engine-driven in Go (ouro precedent).
-- Documented-only (no bridges):
-- - OnGossipHello/OnGossipSelect (quest-6566 "What the Wind
--   Carries" menu chain, AreaExploredOrEventHappens): gossip IS
--   bridged (RegisterCreatureGossipEvent — the "no gossip bridge"
--   claim was stale; live at engine/world/gossip.go:129/369); the
--   arm stays documented-only on the quest machine
--   (GetQuestRewardStatus / AreaExploredOrEventHappens —
--   ashenvale go_naga_brazier precedent).
-- - npc_shenthul (quest-1654 Shattered Salute emote machine:
--   ReceiveEmote / HandleEmoteCommand / FailQuest /
--   AreaExploredOrEventHappens): no emote/quest bridges.
-- - npc_overlord_runthak_orgrimmar (pre-herald citizen scene:
--   MoveInLineOfSight 20yd latch + cross-creature GUID Talk machine):
--   no MoveInLineOfSight-proximity / cross-AI GUID bridges.
-- - npc_thrall_herald_of_war (Wrathgate cinematic: SummonCreature
--   guards/Jaina/portal, MovePoint/MovePath, DoAction scene events,
--   FindNearestCreature): no summon / movement / DoAction bridges.
-- - areatrigger_orgrimmar_herald_of_war: no area-trigger bridge.

local SPELL_CHAIN_LIGHTNING = 16033
local SPELL_SHOCK = 16034

local SOUND_AGGRO = 5880

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

-- C++ UpdateAI arm order: chain lightning before shock.
local function onChainLightning(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CHAIN_LIGHTNING)
    end
    schedule(guid, "chain", 9000, function()
        onChainLightning(creature, guid)
    end)
end

local function onShock(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHOCK)
    end
    schedule(guid, "shock", 15000, function()
        onShock(creature, guid)
    end)
end

local ENTRY = 4949

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- zone_orgrimmar.cpp JustEngagedWith: DoPlaySoundToSet(me, 5880).
    creature:PlayDirectSound(SOUND_AGGRO)
    schedule(guid, "chain", 2000, function()
        onChainLightning(creature, guid)
    end)
    schedule(guid, "shock", 8000, function()
        onShock(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
