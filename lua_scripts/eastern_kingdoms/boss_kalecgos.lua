-- boss_kalecgos (Sunwell Plateau) --
-- Lua port of boss_kalecgos.cpp (AddSC_boss_kalecgos):
--   boss_kalecgos (dragon), boss_sathrovarr, boss_kalecgos_human,
--   go_kalecgos_spectral_rift OnGossipHello.
-- C++ owners: src/server/scripts/EasternKingdoms/SunwellPlateau/
-- boss_kalecgos.cpp (797 lines; AddSC registers the 3 creature AIs
-- via RegisterSunwellPlateauCreatureAI, the GO script, and 5
-- SpellScript/AuraScript loaders). Whole-server-tree grep confirms
-- the cpp (plus loader lines 141-142/319-320) as the sole boss-AI
-- source of "boss_kalecgos"/"boss_sathrovarr" (boss_kiljaeden.cpp
-- only carries the SAY_KALECGOS_* yell enum for the Kil'jaeden
-- encounter's helper, NPC_KALECGOS_KJ = 25319 — unrelated).
-- Entries (verifiable from C++ sources — kalecgos precedent):
-- sunwell_plateau.h SWPCreatureIds names NPC_KALECGOS = 24850 /
-- NPC_KALECGOS_HUMAN = 24891 / NPC_SATHROVARR = 24892, and
-- instance_sunwell_plateau.cpp:48-50 tracks exactly those three
-- entries as DATA_KALECGOS_DRAGON / DATA_KALECGOS_HUMAN /
-- DATA_SATHROVARR (OnCreatureCreate tie, ramstein-strength).
-- NOTE: the existing npc_kalecgos.lua (entry 24844) is the
-- Magister's Terrace zone NPC gossip script — a different creature.
-- Verifiable numbers (boss_kalecgos.cpp enums):
-- Kalecgos dragon: ARCANE_BUFFET 45018 / FROST_BREATH 44799 /
-- TAIL_LASH 45122 / WILD_MAGIC {45001,45002,45004,45006,45010,
-- 44978} / SPECTRAL_BLAST 44869 / BANISH 44836 / ENRAGE 44807;
-- yells: EVIL_AGGRO 0 / EVIL_SLAY 1 / ARCANE_BUFFET 6 /
-- EMOTE_ENRAGE 4.
-- Sathrovarr: SHADOW_BOLT 45031 / AGONY_CURSE 45032 /
-- CORRUPTION_STRIKE 45029; yells: SATH_AGGRO 0 / SATH_SLAY 1 /
-- SATH_SPELL1 3 / SATH_SPELL2 4 / SATH_DEATH 2.
-- Kalecgos human: REVITALIZE 45027 / HEROIC_STRIKE 45026; yells:
-- GOOD_NEAR_DEATH 0/1/2 / GOOD_DEATH 3.
-- Spectral rift GO: GO_SPECTRAL_RIFT = 187055 (SWPGameObjectIds; corroborated by boss_kalecgos.cpp
-- DespawnPortals' GetGameObjectListWithEntryInGrid use);
-- SPECTRAL_EXHAUSTION 44867 / SPECTRAL_REALM_TRIGGER 44811.
-- Ported arms (C++ cadences, maiden/mr_smite/moroes conventions):
-- boss_kalecgos (24850), OnEnterCombat(1):
-- - Talk(0) aggro (arugal Talk convention).
-- - Arcane Buffet: 8s init -> Repeat 8s; roll_chance_i(20)
--   (~math.random(100) <= 20) Talk(6); DoCastAOE(45018)
--   -> creature:CastSpell(creature, 45018) (mr_smite AoE
--   convention).
-- - Frost Breath: 15s init -> Repeat 15s; DoCastAOE(44799).
-- - Tail Lash: 25s init -> Repeat 15s; DoCastAOE(45122).
-- - Wild Magic: 10s init -> Repeat 20s; DoCastAOE(
--   WildMagicSpells[urand(0,5)], triggered) -> CastSpell(creature,
--   WILD_MAGIC[math.random(1,6)], true).
-- - Spectral Blast: 20-25s init -> Repeat {20s,25s} ->
--   math.random(20000,25000); DoCastAOE(44869, triggered).
-- - 1s check: !_isEnraged && HealthBelowPct(10) ->
--   Talk(EMOTE_ENRAGE 4) + DoCastSelf(44807, triggered);
--   HealthBelowPct(1) && !_isBanished -> DoCastSelf(44836,
--   triggered) (banish) + all combat timers cancelled, the 1s
--   check survives (C++ events.Reset() then Repeat(1s)).
-- boss_sathrovarr (24892), OnEnterCombat(1):
-- - Talk(0) aggro; Shadowbolt: 7-10s init -> Repeat {7s,10s};
--   20% Talk(3); DoCastAOE(45031).
-- - Agony Curse: 20s init -> Repeat 20s; random non-tank target
--   (CurseAgonySelector) else victim; DoCast(45032, triggered).
-- - Corruption Strike: 13s init -> Repeat 13s; 20% Talk(4);
--   DoCastVictim(45029) -> GetVictim + CastSpell.
-- - 1s check: <10% -> enrage latch (the cross-AI DoAction(
--   ACTION_ENRAGE) on the dragon is instance-leg-blocked — the
--   dragon's own <10% check produces the same outcome; documented
--   below); <1% && !_isBanished -> DoCastSelf(44836, triggered).
-- boss_kalecgos_human (24891), OnEnterCombat(1):
-- - Revitalize: 5s init -> Repeat 5s; DoCastSelf(45027).
-- - Heroic Strike: 3s init -> Repeat 2s; DoCastVictim(45026).
-- - JustDied(4): Talk(3) (moroes onDied convention).
-- - DamageTaken(9) near-death say phases (moroes damage-taken
--   convention): pct <= 75 (phase 1 -> Talk(0)), <= 50 (phase 2
--   -> Talk(1)), <= 10 (phase 3 -> Talk(2)), one-shot latches
--   reset on 23.
-- go_kalecgos_spectral_rift (GO 187055), GossipHello(1):
-- - if !player:HasAura(44867) then player:CastSpell(player,
--   44811, true) (C++-exact OnGossipHello body; Eluna GO gossip
--   event ids 1=hello, handler args (event, player, object)).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - instance_sunwell_plateau.cpp (AddSC_instance_sunwell_plateau)
--   — instance-script model blocked (standing): DATA_KALECGOS_*
--   guid tracking, SendEncounterUnit frame legs, the
--   DoRemoveAurasDueToSpellOnPlayers legs, encounter data legs.
-- - Both bosses' DamageTaken no-lethal-damage latch (only sath's
--   TAP_CHECK/Unit::Kill sequence kills) and the banish/outro
--   resolution: sathrovarr's <1% TAP_CHECK leg (needs dragon's
--   banish-aura check + SpellHit(46733) -> Unit::Kill leg — no
--   SpellHit bridge), the dragon's ACTION_START_OUTRO chain
--   (SetRegenerateHealth/REACT_PASSIVE/Faction/SetVisible/
--   MovePoint/KillSelf/InterruptNonMelee/RemoveAllAttackers —
--   no MovementInform/MotionMaster bridges), sathrovarr's
--   EnterEvadeMode cross-AI leg, the KilledUnit cross-leg when
--   the human dies (EVADE_REASON_OTHER leg), and the
--   DamageTaken-nullifier without its resolution partners would
--   strand an unkillable boss, so it stays documented-only.
-- - JustEngagedWith's SummonCreature(NPC_KALECGOS_HUMAN,
--   KalecgosSummonPos, TEMPSUMMON_CORPSE_TIMED_DESPAWN) +
--   SetInCombatWith legs (no summon bridge — gurtogg precedent
--   class); the human registers independently for 24891.
-- - KilledUnit Talk legs (dragon 50% Talk(1), sathrovarr Talk(1)):
--   no KilledUnit bridge in engine/scripting.
-- - The 5 SpellScript/AuraScript loaders
--   (spell_kalecgos_tap_check 46732 / spell_kalecgos_spectral_
--   blast 44869 / spell_kalecgos_spectral_realm_trigger 44811 /
--   spell_kalecgos_spectral_realm_aura 46021 /
--   spell_kalecgos_curse_of_boundless_agony 45032): SpellScript/
--   AuraScript-check-handler-blocked class (standing).
-- - AgonyCurseSelector's no-SPECTRAL_REALM_AURA (46021) and
--   SPECTRAL_EXHAUSTION (44867) filters and SpectralBlast's own
--   target filter: the aura-side filter legs have no Go model;
--   the port targets a random in-range enemy (maiden
--   convention), noted as the approximation.
-- - Dragon's DoAction(ACTION_ENRAGE) / DoAction(ACTION_START_
--   OUTRO): cross-AI action legs, no bridge.

local ENTRY_KALECGOS_DRAGON = 24850
local ENTRY_SATHROVARR = 24892
local ENTRY_KALECGOS_HUMAN = 24891
local ENTRY_SPECTRAL_RIFT = 187055

local SPELL_ARCANE_BUFFET = 45018
local SPELL_FROST_BREATH = 44799
local SPELL_TAIL_LASH = 45122
local SPELL_WILD_MAGIC_1 = 45001
local SPELL_WILD_MAGIC_2 = 45002
local SPELL_WILD_MAGIC_3 = 45004
local SPELL_WILD_MAGIC_4 = 45006
local SPELL_WILD_MAGIC_5 = 45010
local SPELL_WILD_MAGIC_6 = 44978
local SPELL_SPECTRAL_BLAST = 44869
local SPELL_BANISH = 44836
local SPELL_ENRAGE = 44807
local SPELL_SHADOW_BOLT = 45031
local SPELL_AGONY_CURSE = 45032
local SPELL_CORRUPTION_STRIKE = 45029
local SPELL_REVITALIZE = 45027
local SPELL_HEROIC_STRIKE = 45026
local SPELL_SPECTRAL_EXHAUSTION = 44867
local SPELL_SPECTRAL_REALM_TRIGGER = 44811

local WILD_MAGIC = {
    SPELL_WILD_MAGIC_1, SPELL_WILD_MAGIC_2, SPELL_WILD_MAGIC_3,
    SPELL_WILD_MAGIC_4, SPELL_WILD_MAGIC_5, SPELL_WILD_MAGIC_6,
}

local timers = {}
local enraged = {}
local banished = {}
local sayStage = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function initState(guid)
    enraged[guid] = false
    banished[guid] = false
    sayStage[guid] = 1
end

local function clearState(guid)
    enraged[guid] = nil
    banished[guid] = nil
    sayStage[guid] = nil
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function healthPct(creature)
    local max = creature:GetMaxHealth()
    if max == 0 then
        return 100
    end
    return creature:GetHealth() * 100 / max
end

-- boss_kalecgos (24850) --

local function onKalecgosBuffet(creature, guid)
    if math.random(100) <= 20 then
        creature:Talk(6)
    end
    creature:CastSpell(creature, SPELL_ARCANE_BUFFET)
    schedule(guid, "buffet", 8000, function() onKalecgosBuffet(creature, guid) end)
end

local function onKalecgosFrost(creature, guid)
    creature:CastSpell(creature, SPELL_FROST_BREATH)
    schedule(guid, "frost", 15000, function() onKalecgosFrost(creature, guid) end)
end

local function onKalecgosTail(creature, guid)
    creature:CastSpell(creature, SPELL_TAIL_LASH)
    schedule(guid, "tail", 15000, function() onKalecgosTail(creature, guid) end)
end

local function onKalecgosWild(creature, guid)
    creature:CastSpell(creature, WILD_MAGIC[math.random(1, 6)], true)
    schedule(guid, "wild", 20000, function() onKalecgosWild(creature, guid) end)
end

local function onKalecgosBlast(creature, guid)
    creature:CastSpell(creature, SPELL_SPECTRAL_BLAST, true)
    schedule(guid, "blast", math.random(20000, 25000), function() onKalecgosBlast(creature, guid) end)
end

local function onKalecgosCheck(creature, guid)
    if not enraged[guid] and healthPct(creature) <= 10 then
        enraged[guid] = true
        creature:Talk(4)
        creature:CastSpell(creature, SPELL_ENRAGE, true)
    end
    if healthPct(creature) <= 1 and not banished[guid] then
        banished[guid] = true
        creature:CastSpell(creature, SPELL_BANISH, true)
        local per = timers[guid]
        if per then
            for key, id in pairs(per) do
                if key ~= "check" then
                    RemoveEventById(id)
                    per[key] = nil
                end
            end
        end
    end
    schedule(guid, "check", 1000, function() onKalecgosCheck(creature, guid) end)
end

local function onKalecgosCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    initState(guid)
    creature:Talk(0)
    schedule(guid, "buffet", 8000, function() onKalecgosBuffet(creature, guid) end)
    schedule(guid, "frost", 15000, function() onKalecgosFrost(creature, guid) end)
    schedule(guid, "wild", 10000, function() onKalecgosWild(creature, guid) end)
    schedule(guid, "tail", 25000, function() onKalecgosTail(creature, guid) end)
    schedule(guid, "blast", math.random(20000, 25000), function() onKalecgosBlast(creature, guid) end)
    schedule(guid, "check", 1000, function() onKalecgosCheck(creature, guid) end)
end

local function onCombatEnd(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    clearState(guid)
end

-- boss_sathrovarr (24892) --

local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

local function randomTargetInRange(creature, range)
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if creature:IsWithinDist(p, range) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onSathShadowbolt(creature, guid)
    if math.random(100) <= 20 then
        creature:Talk(3)
    end
    creature:CastSpell(creature, SPELL_SHADOW_BOLT)
    schedule(guid, "bolt", math.random(7000, 10000), function() onSathShadowbolt(creature, guid) end)
end

local function onSathAgony(creature, guid)
    local target = randomTargetInRange(creature, 100)
    if not target then
        target = creature:GetVictim()
    end
    if target then
        creature:CastSpell(target, SPELL_AGONY_CURSE, true)
    end
    schedule(guid, "agony", 20000, function() onSathAgony(creature, guid) end)
end

local function onSathCorruption(creature, guid)
    if math.random(100) <= 20 then
        creature:Talk(4)
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CORRUPTION_STRIKE)
    end
    schedule(guid, "corrupt", 13000, function() onSathCorruption(creature, guid) end)
end

local function onSathCheck(creature, guid)
    if healthPct(creature) <= 10 and not enraged[guid] then
        enraged[guid] = true
    end
    if healthPct(creature) <= 1 and not banished[guid] then
        banished[guid] = true
        creature:CastSpell(creature, SPELL_BANISH, true)
    end
    schedule(guid, "check", 1000, function() onSathCheck(creature, guid) end)
end

local function onSathCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    initState(guid)
    creature:Talk(0)
    schedule(guid, "bolt", math.random(7000, 10000), function() onSathShadowbolt(creature, guid) end)
    schedule(guid, "agony", 20000, function() onSathAgony(creature, guid) end)
    schedule(guid, "corrupt", 13000, function() onSathCorruption(creature, guid) end)
    schedule(guid, "check", 1000, function() onSathCheck(creature, guid) end)
end

-- boss_kalecgos_human (24891) --

local function onHumanRevitalize(creature, guid)
    creature:CastSpell(creature, SPELL_REVITALIZE)
    schedule(guid, "revitalize", 5000, function() onHumanRevitalize(creature, guid) end)
end

local function onHumanStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_HEROIC_STRIKE)
    end
    schedule(guid, "strike", 2000, function() onHumanStrike(creature, guid) end)
end

local function onHumanCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    initState(guid)
    schedule(guid, "revitalize", 5000, function() onHumanRevitalize(creature, guid) end)
    schedule(guid, "strike", 3000, function() onHumanStrike(creature, guid) end)
end

local function onHumanDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    local stage = sayStage[guid]
    if stage == nil or stage >= 4 then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local pct = (creature:GetHealth() - damage) * 100 / maxHealth
    if stage == 1 and pct <= 75 then
        sayStage[guid] = 2
        creature:Talk(0)
    elseif stage == 2 and pct <= 50 then
        sayStage[guid] = 3
        creature:Talk(1)
    elseif stage == 3 and pct <= 10 then
        sayStage[guid] = 4
        creature:Talk(2)
    end
end

local function onHumanDied(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    clearState(guid)
    creature:Talk(3)
end

RegisterCreatureEvent(ENTRY_KALECGOS_DRAGON, 1, onKalecgosCombat)
RegisterCreatureEvent(ENTRY_KALECGOS_DRAGON, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_KALECGOS_DRAGON, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_KALECGOS_DRAGON, 23, onCombatEnd)

RegisterCreatureEvent(ENTRY_SATHROVARR, 1, onSathCombat)
RegisterCreatureEvent(ENTRY_SATHROVARR, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_SATHROVARR, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_SATHROVARR, 23, onCombatEnd)

RegisterCreatureEvent(ENTRY_KALECGOS_HUMAN, 1, onHumanCombat)
RegisterCreatureEvent(ENTRY_KALECGOS_HUMAN, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_KALECGOS_HUMAN, 4, onHumanDied)
RegisterCreatureEvent(ENTRY_KALECGOS_HUMAN, 23, onCombatEnd)
RegisterCreatureEvent(ENTRY_KALECGOS_HUMAN, 9, onHumanDamageTaken)

-- go_kalecgos_spectral_rift (GO 187055) --

local function onSpectralRiftGossip(_, player, _)
    if not player:HasAura(SPELL_SPECTRAL_EXHAUSTION) then
        player:CastSpell(player, SPELL_SPECTRAL_REALM_TRIGGER, true)
    end
    return true
end

RegisterGameObjectGossipEvent(ENTRY_SPECTRAL_RIFT, 1, onSpectralRiftGossip)
