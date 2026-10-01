-- Apothecary Baxter (Shadowfang Keep)
-- Lua port of src/server/scripts/EasternKingdoms/ShadowfangKeep/
-- boss_apothecary_hummel.cpp
-- (npc_apothecary_baxterAI : public npc_apothecary_genericAI —
-- ScriptedAI via GetShadowfangKeepAI -> GetInstanceAI; registered by
-- AddSC_boss_apothecary_hummel in eastern_kingdoms_script_loader.cpp
-- (declaration line 125, call line 303)). The Shadowfang Keep block
-- is OPEN.
-- Entry (verifiable from the C++ sources): NPC_APOTHECARY_BAXTER =
-- 36565 in the ApothecaryMisc enum in boss_apothecary_hummel.cpp
-- (kalecgos precedent). Whole-server-tree grep confirms
-- boss_apothecary_hummel.cpp as the only source of
-- "apothecary_hummel"/"ApothecaryHummel" for the script (loader
-- lines only otherwise).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee-
-- AttackIfReady. The C++ UNIT_STATE_CASTING gate has no bridge
-- (maiden precedent), so timers fire unconditionally. The C++
-- events.Repeat arms re-arm from execution; the Lua one-shot timer
-- chains match that cadence.
-- Verifiable numbers (file's own enums): SPELL_CHAIN_REACTION =
-- 68821 / SPELL_SUMMON_TABLE = 69218 / SPELL_COLOGNE_SPRAY =
-- 68948; SAY_BAXTER_DEATH = 0.
-- Init-delay nuance: the C++ arms both events in Reset() (spawn
-- time), but the ExecuteEvent loop is gated on UpdateVictim(), so
-- the pumps only execute once in combat. The Lua timers arm on
-- OnEnterCombat with the C++-exact init delays, matching the
-- observable cadence (maiden convention).
-- Ported arms (C++ Reset + UpdateAI + JustDied):
-- - EVENT_COLOGNE_SPRAY: 7s init -> 4s re-arm; DoCastVictim(
--   68948) -> GetVictim + CastSpell (mr_smite convention).
-- - EVENT_CHAIN_REACTION: 12s init -> 25s re-arm; C++-exact —
--   DoCastVictim(SPELL_SUMMON_TABLE) non-triggered followed by
--   DoCastVictim(SPELL_CHAIN_REACTION) non-triggered (Baxter's
--   leg has no triggered flag, unlike Hummel's summon-table leg).
-- - JustDied: _events.Reset() + Talk(SAY_BAXTER_DEATH=0) ->
--   OnDied Talk(0) with timer cancellation (najentus convention).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - The npc_apothecary_genericAI DoAction legs: ACTION_START_EVENT
--   MovePoint(BaxterMovePos -221.4115, 2206.825, 79.93151) with
--   EMOTE_STATE_USE_STANDING on MovementInform — no MotionMaster
--   bridge (selin_fireheart precedent); ACTION_START_FIGHT
--   SetImmuneToAll(false) + DoZoneInCombat — no immunity/combat
--   bridges; SetImmuneToPC/SetFaction(FACTION_MONSTER) — no
--   bridges.
-- - The GetShadowfangKeepAI -> GetInstanceAI leg (instance-script
--   model blocked, standing).

local NPC_APOTHECARY_BAXTER = 36565

local SAY_BAXTER_DEATH = 0

local SPELL_CHAIN_REACTION = 68821
local SPELL_SUMMON_TABLE = 69218
local SPELL_COLOGNE_SPRAY = 68948

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
    per[key] = CreateLuaEvent(fn, delay)
end

local function onCologneSpray(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_COLOGNE_SPRAY)
    end
    schedule(guid, "cologne", 4000, function() onCologneSpray(creature, guid) end)
end

local function onChainReaction(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SUMMON_TABLE)
        creature:CastSpell(victim, SPELL_CHAIN_REACTION)
    end
    schedule(guid, "chain", 25000, function() onChainReaction(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "cologne", 7000, function() onCologneSpray(creature, guid) end)
    schedule(guid, "chain", 12000, function() onChainReaction(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(_, creature)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_BAXTER_DEATH)
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(NPC_APOTHECARY_BAXTER, 1, onEnterCombat)
RegisterCreatureEvent(NPC_APOTHECARY_BAXTER, 2, onCombatEnd)
RegisterCreatureEvent(NPC_APOTHECARY_BAXTER, 4, onDied)
RegisterCreatureEvent(NPC_APOTHECARY_BAXTER, 23, onReset)
