-- Highborne Lamenter (Undercity) --
-- Lua port of src/server/scripts/EasternKingdoms/zone_undercity.cpp
-- (npc_highborne_lamenterAI — UpdateAI EventMove / EventCast
-- one-shot timer arms only). Zone-script unit per
-- eastern_kingdoms_script_loader.cpp order (tirisfal_glades done;
-- undercity closes the EK zone set).
-- Entry (zone_undercity.cpp Sylvanas enum, verifiable from the C++
-- sources): 21628 (NPC_HIGHBORNE_LAMENTER). The creature_template
-- ScriptName binding is DB-side (no TDB in this workspace).
-- npc_lady_sylvanas_windrunner — the combat rotation is bridgeable
-- in principle (omen 1s-pump pattern: JustEngagedWith aggro sound
-- 5886 — no sound bridge; EVENT_SUMMON_SKELETON 20s self-cast
-- 59711, EVENT_BLACK_ARROW 15s DoCastVictim 59712, EVENT_SHOOT 8s
-- DoCastVictim 59710, EVENT_MULTI_SHOT 10s DoCastVictim 59713,
-- EVENT_FADE 30s self-cast 20672 + blink self-cast 29211 with the
-- 10-yd multishot sub-arm — re-arms 30-35s / 20-30s / 15-20s /
-- 8-10s / 10-13s (urand, C++-exact)) but no NPC_ entry constant
-- for the Undercity Sylvanas exists anywhere in the C++ sources
-- (37596 / 38161 / 37223 / 37554 are the Forge-of-Souls and
-- Halls-of-Reflection versions — different scripts) — joins the
-- TDB-dump entry-evidence queue (azuregos / cairne / thrall_
-- warchief / tiger matriarch precedent); the lament machine
-- (OnQuestReward 9180 -> Talk(EMOTE_LAMENT 2) + self-cast 36568
-- + 4x SummonCreature(21628) + lament event chain), the
-- JustSummoned ribbon machine (ObjectAccessor target-GUID /
-- MoveJump / ribbon-of-souls 37099 cross-creature arms) and the
-- EVENT_SUNSORROW_WHISPER arm (FindNearestCreature 16287 +
-- cross-creature Talk(0)) sit behind the quest-reward / sound /
-- summon / world-search / movement / cross-creature-Talk bridges —
-- documented-only.
-- npc_parqual_fintallas is named in the file's ContentData header
-- but AddSC_undercity registers only the two classes above — no
-- invented registration (tirisfal_glades precedent).
-- Eluna creature events: 5 OnSpawn, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset (the C++ overrides no other hooks — JustEngagedWith
-- is an empty override, so no event 1; the Initialize() ctor/Reset
-- arms land on spawn — mushroom precedent).
-- The C++ timers are one-shot (both latch false after firing):
-- EventMoveTimer 10000 -> SetDisableGravity(true) +
-- MonsterMoveWithSpeed/UpdatePosition toward (x, y, -55.50f) —
-- the whole arm sits behind the no-movement bridge (desolace /
-- rinji precedent) — unmodeled; EventCastTimer 17500 ->
-- non-triggered DoCast(me, SPELL_HIGHBORNE_AURA 37090)
-- (vaelastrasz convention: C++ DoCast default triggered=false)
-- self-cast — C++-exact via creature:CastSpell(creature, 37090).
-- Modeled as a single one-shot CreateLuaEvent(17500, 1) armed on
-- spawn: C++-exact timing for the modeled arm (the unmodeled
-- movement arm latches false at 10s with no bridgeable effect,
-- so a 1s UpdateAI pump would be behaviorally identical but pure
-- noise — omitted).
-- Fight shape (C++-exact for the modeled arm):
-- OnSpawn(5): per-GUID reset to the C++ Initialize() values
-- (mushroom precedent) + one-shot 17500ms self-cast of
-- SPELL_HIGHBORNE_AURA 37090 + state drop.
-- OnDied(4) / OnLeaveCombat(2) / OnReset(23): cancel the one-shot
-- + drop per-GUID state (mushroom precedent; the C++ Reset arms
-- are covered by OnSpawn — the lamenter never engages, so
-- evade-Reset is unreachable).
-- Deliberate deviations: the 10s EventMove arm (disable-gravity
-- + MonsterMoveWithSpeed toward z=-55.50) is unmodeled — no
-- movement bridge.

local SPELL_HIGHBORNE_AURA = 37090

local HIGHBORNE_LAMENTER_ENTRY = 21628

local CAST_DELAY_MS = 17500

local timers = {}
local states = {}

local function cancelTimer(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function fullReset(guid)
    cancelTimer(guid)
    states[guid] = nil
end

RegisterCreatureEvent(HIGHBORNE_LAMENTER_ENTRY, 5, function(_, creature)
    local guid = creature:GetGUID()
    fullReset(guid)
    states[guid] = true
    -- C++ UpdateAI EventCast arm — non-triggered self-cast
    -- (vaelastrasz convention), one-shot at 17.5s (C++-exact).
    -- The 10s EventMove arm is unmodeled (no movement bridge).
    timers[guid] = CreateLuaEvent(function()
        creature:CastSpell(creature, SPELL_HIGHBORNE_AURA)
        fullReset(guid)
    end, CAST_DELAY_MS, 1)
end)

RegisterCreatureEvent(HIGHBORNE_LAMENTER_ENTRY, 2, function(_, creature)
    fullReset(creature:GetGUID())
end)

RegisterCreatureEvent(HIGHBORNE_LAMENTER_ENTRY, 4, function(_, creature)
    fullReset(creature:GetGUID())
end)

RegisterCreatureEvent(HIGHBORNE_LAMENTER_ENTRY, 23, function(_, creature)
    fullReset(creature:GetGUID())
end)
