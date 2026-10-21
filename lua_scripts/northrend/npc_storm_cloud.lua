-- Storm Cloud (Zul'Drak) --
-- Lua port of src/server/scripts/Northrend/zone_zuldrak.cpp
-- (npc_storm_cloudAI — Reset()/JustAppeared() arm only).
-- Zone-script unit per northrend_script_loader.cpp order
-- (wintergrasp done; zuldrak: npc_drakuru_shackles, npc_
-- captured_rageclaw, npc_released_offspring_harkoa, npc_
-- crusade_recruit, go_scourge_enclosure, npc_alchemist_
-- finklestein, go_finklesteins_cauldron, spell_random_
-- ingredient_aura, spell_random_ingredient, spell_pot_check,
-- spell_fetch_ingredient_aura, the four scourge_disguise
-- SpellScripts documented-only, npc_storm_cloud ported).
-- Entry (file's own StormCloud enum, verifiable from the C++
-- sources): 29939 (STORM_COULD — the file's own spelling). The
-- creature_template ScriptName binding is DB-side (no TDB in
-- this workspace).
-- Eluna creature events: 5 OnSpawn, 23 OnReset.
-- Ported arm (C++ Reset() / JustAppeared()): CastSpell(me,
-- STORM_VISUAL 55708, true) — the triggered non-targeted
-- self-cast — creature:CastSpell(creature, spell, true) is the
-- vaelastrasz self-cast convention; the triggered flag is
-- accepted but not threaded by the binding (gruul reverberation
-- precedent — documented deviation, same as every other
-- triggered port). The cloud never engages, so OnReset(23)
-- re-arms the same Reset fragment (C++ Reset semantics —
-- mushroom precedent); no timers, no per-GUID state (the
-- blessed_banner shape — no Talk arm on this one).
-- Unmodeled: the SpellHit arm (spell 55516 GYMERS_GRAB: unit-
-- caster gate -> vehicle-kit seat-count gate -> me->CastSpell(
-- caster, RIDE_VEHICLE 43671, true) + me->CastSpell(caster,
-- HEALING_WINDS 55549, true)) has no vehicle / passenger bridges (SpellHit IS bridged — event 14
-- — fizzule / wintergrasp vehicle_teleporter precedent; the seat-count
-- + ride-vehicle legs are the blockers) — documented-only.
-- npc_drakuru_shackles — the Reset arm (SetFlag NOT_SELECTABLE,
-- GetClosePoint + SummonCreature NPC_RAGECLAW 29686, SetFacing
-- ToObject pair — the LockRageclaw facing pair (C++ :74-75) is
-- bridgeable via the atan2 twin, devourer_of_souls precedent,
-- retracting "facing" from the blocker list) sits behind the flag /
-- summon bridges;
-- the SpellHit(55083 UNLOCK_SHACKLE) machine (player-caster +
-- quest-12861 INCOMPLETE gates -> DoCast(rageclaw, 55223)
-- triggered + setDeathState(DEAD) + KilledMonster + RemoveAura
-- (54990) + DespawnOrUnsummon) behind the quest / kill-credit / despawn bridges (SpellHit IS
-- bridged — event 14; player:KilledMonsterCredit is the blocker)
-- — documented-only. 29686 +
-- 12861 + 54990/55009/55083/55223 verifiable.
-- npc_captured_rageclaw — the Reset arm (SetFaction(FACTION_
-- FRIENDLY) + triggered self-cast 54990) is bridgeable in
-- principle (entry 29686 verifiable from the file's own enum),
-- but the release machine (SpellHit 55223: RemoveAura(55009) +
-- SetStandState(STAND) + SetFaction(template) + DoCast(me,
-- 55085) + Talk(0) + MoveRandom(10) + DespawnOrUnsummon(10s))
-- has the faction / stand-state bridges (SetFaction + SetStandState
-- both live) but not movement / despawn — the release machine's
-- MoveRandom + DespawnOrUnsummon legs block it, so the Reset arm stays
-- deferred rather than ported as a stranded half (a permanently-
-- chained NPC is worse than no registration); MoveInLineOfSight overridden empty —
-- documented-only. 55085 verifiable.
-- npc_released_offspring_harkoa — the Reset arm (GetClosePoint
-- + MovePoint(0)) and the MovementInform despawn arm sit behind
-- the movement / despawn bridges — documented-only.
-- npc_crusade_recruit — the UpdateAI event machine (EVENT_
-- RECRUIT_1: RemoveFlag gossip + Talk(0) + 3s->RECRUIT_2;
-- EVENT_RECRUIT_2: SetWalk + MovePoint 10yd forward + Despawn
-- OrUnsummon(5s)) and the OnGossipSelect arm (schedule 100ms +
-- CloseGossip + CastSpell(player, 50633) + SetFacingToObject):
-- gossip itself IS bridged (RegisterCreatureGossipEvent — the
-- "no gossip bridge" claim was stale; live at
-- engine/world/gossip.go:129/369), creature:CastSpell(player) and
-- Despawn are bridged too — the real blockers are the flag /
-- emotestate writes (no UNIT_NPC_FLAGS / UNIT_FIELD_EMOTESTATE
-- write bridge), SetWalk / MovePoint (no movement bridges), and the
-- entry, which is not C++-verifiable (DB-side ScriptName bind), so
-- nothing can be registered — documented-only. The OnGossipSelect
-- me->SetFacingToObject(player) leg (zone_zuldrak.cpp :276) is
-- bridgeable via the atan2 twin (devourer_of_souls precedent) —
-- the faced target IS the Lua-available gossip player — but the
-- blockers above keep the verdict. 50633/12509 verifiable.
-- go_scourge_enclosure (GameObjectAI) — the OnGossipHello arm
-- (UseDoorOrButton + quest-12916 INCOMPLETE gate -> FindNearest
-- Creature(29928, 20yd) -> KilledMonsterCredit + dummy CastSpell
-- (dummy, 55529) + DespawnOrUnsummon(4s)) sits behind the GO /
-- quest / world-search / kill-credit / despawn bridges —
-- documented-only. 12916/29928/55529 verifiable.
-- npc_alchemist_finklestein — the facing/emote loop (EVENT_TURN_TO
-- _POT 15-26s SetFacingTo(6.230825) + EMOTE_STATE_USE_STANDING
-- _NO_SHEATHE -> EVENT_TURN_BACK 11s SetFacingTo(4.886922) +
-- EMOTE_STATE_NONE -> 25-41s->TURN_TO_POT), the MEDIUM_4 / MEDIUM_5 /
-- HARD_6 ingredient events (player GUID lookup -> Talk(say, player) +
-- DoCast(player, 51154/51157) + ++try, dispatched by the cauldron's
-- SetData(1,1)), and the cauldron GO arm (player->CastSpell(player,
-- SPELL_POT_CHECK 51046)) stay documented-only: no facing/emote/
-- GO/player-self-cast bridges. OnGossipSelect (zone_zuldrak.cpp:
-- 575-583: CloseGossipMenuFor + DoCast(player,
-- SPELL_ALCHEMIST_APPRENTICE_INVISBUFF 51216) + ScheduleEvent
-- (EVENT_EASY_123, 100ms) -> Talk(SAY_EASY_123 0, player) +
-- DoCast(player, SPELL_RANDOM_INGREDIENT_EASY_AURA 51015)) is
-- PORTED below — entry NPC_FINKLESTEIN 28205 (zone_zuldrak.cpp:
-- 341); all three calls are bridged (creature:CastSpell(player,
-- ...), Talk whisper arm); the 100ms timer rides the per-GUID
-- schedule table with cancel on 2/4/23.
-- go_finklesteins_cauldron (GameObjectAI) — OnGossipHello:
-- player->CastSpell(player, SPELL_POT_CHECK 51046) behind the
-- GO / player-self-cast bridges — documented-only.
-- spell_random_ingredient_aura — AuraScript periodic (51015->
-- 51134 / 51154->51105 / 51157->51107 self-casts): no
-- AuraScript bridge (blasted_lands / gordunni precedents) —
-- documented-only.
-- spell_random_ingredient — SpellScript (hit-player urand 0-10
-- / 11-15 / 16-20 -> closest 28205 25yd CastSpell(player,
-- FetchIngredients[ingredient][0]) + Talk(say, player)): no
-- SpellScript / world-search bridges — documented-only.
-- spell_pot_check — SpellScript (21-slot Have/Fetch aura scan
-- -> SPELL_THROW_INGREDIENT 51025 + Have-aura / item-count
-- gates -> DestroyItemCount / SetData(1,1) ingredient-advance
-- vs RemoveItems + RemoveAura(51216) + Talk(4, player) ruin
-- path; hard-tier completion -> quest-12541 INCOMPLETE gate ->
-- RemoveAura(51216) + CastSpell(player, 51111)): no SpellScript
-- / quest bridges — documented-only. The full FetchIngredients
-- table (21 ingredients: 51018-51104 fetch/have pairs, item
-- ids 38336-38398, SAY 5-25) and 51015/51154/51157 + 51134/
-- 51105/51107 + 51046 + 51025 + 51111 + 51216 + 12541
-- verifiable.
-- spell_fetch_ingredient_aura — AuraScript OnRemove (expire +
-- HasAura(51216) + closest 28205 100yd -> RemoveAura(51216) +
-- Talk(4, target)): no AuraScript bridge — documented-only.
-- spell_scourge_disguise / spell_scourge_disguise_instability /
-- spell_scourge_disguise_expiring / spell_drop_disguise —
-- AuraScript/SpellScript (51966 transform apply/remove ->
-- 51971 instability periodic with irand(30,240)s calc + 52010
-- expiring -> whisper TEXT_DISGUISE_WARNING 28891; 54089 hit-
-- unit -> 52010): no AuraScript / SpellScript bridges —
-- documented-only. 51966/51971/52010/54089/28891 verifiable.
-- npc_storm_cloud — SpellHit(55516) vehicle arm (see Unmodeled
-- above) behind the SpellHit / vehicle bridges; entries
-- 55549/55516/43671 + 55708 + 29939 verifiable.

local SPELL_STORM_VISUAL = 55708

local STORM_CLOUD_ENTRY = 29939

-- C++ Reset()/JustAppeared() arm: triggered non-targeted
-- self-cast of the storm visual — one-shot per spawn / reset,
-- no timers, no per-GUID state.
local function onSpawnOrReset(_, creature)
    -- Alive-gated: the death-leg ev23 fire would be a phantom cast; the
    -- ev5 (spawn) leg still fires since a fresh spawn is always alive.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_STORM_VISUAL, true)
    end
end

RegisterCreatureEvent(STORM_CLOUD_ENTRY, 5, onSpawnOrReset)
RegisterCreatureEvent(STORM_CLOUD_ENTRY, 23, onSpawnOrReset)

-- npc_alchemist_finklestein gossip (zone_zuldrak.cpp:575-583) — entry
-- NPC_FINKLESTEIN 28205 (zone_zuldrak.cpp:341).
local NPC_FINKLESTEIN = 28205
local SPELL_ALCHEMIST_APPRENTICE_INVISBUFF = 51216
local SPELL_RANDOM_INGREDIENT_EASY_AURA = 51015
local SAY_EASY_123 = 0

local finklesteinTimers = {}

local function finklesteinCancelTimers(guid)
    local per = finklesteinTimers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        finklesteinTimers[guid] = nil
    end
end

local function finklesteinSchedule(guid, key, delay, fn)
    local per = finklesteinTimers[guid]
    if not per then
        per = {}
        finklesteinTimers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ EVENT_EASY_123 (zone_zuldrak.cpp:528-533): Talk(SAY_EASY_123 0,
-- player) + DoCast(player, SPELL_RANDOM_INGREDIENT_EASY_AURA 51015) +
-- ++try. The player is captured in the closure (the C++ _playerGUID
-- stash); the try counter gates only the unbridged cauldron SetData
-- dispatch, so it is not modeled.
local function finklesteinEasy123(creature, player)
    creature:Talk(SAY_EASY_123, player)
    creature:CastSpell(player, SPELL_RANDOM_INGREDIENT_EASY_AURA)
end

-- C++ OnGossipSelect (zone_zuldrak.cpp:575-583): CloseGossipMenuFor +
-- DoCast(player, SPELL_ALCHEMIST_APPRENTICE_INVISBUFF 51216) +
-- ScheduleEvent(EVENT_EASY_123, 100ms). No menuId/gossipListId gate.
local function finklesteinGossipSelect(event, player, creature, sender, action)
    player:GossipComplete()
    creature:CastSpell(player, SPELL_ALCHEMIST_APPRENTICE_INVISBUFF)
    local guid = creature:GetGUID()
    finklesteinSchedule(guid, "easy123", 100, function()
        finklesteinEasy123(creature, player)
    end)
    -- C++ OnGossipSelect returns false: the AI/native select arms
    -- proceed (Eluna GOSSIP_EVENT_ON_SELECT: a Lua false lets them
    -- proceed; true/nothing would veto them).
    return false
end

local function finklesteinCancel(event, creature)
    finklesteinCancelTimers(creature:GetGUID())
end

RegisterCreatureGossipEvent(NPC_FINKLESTEIN, 2, finklesteinGossipSelect)
RegisterCreatureEvent(NPC_FINKLESTEIN, 2, finklesteinCancel)
RegisterCreatureEvent(NPC_FINKLESTEIN, 4, finklesteinCancel)
RegisterCreatureEvent(NPC_FINKLESTEIN, 23, finklesteinCancel)
