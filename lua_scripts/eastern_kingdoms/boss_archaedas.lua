-- Archaedas (Uldaman)
-- Lua port of src/server/scripts/EasternKingdoms/Uldaman/
-- boss_archaedas.cpp
-- (boss_archaedas : public ScriptedAI (NOT BossAI — constructor
-- Initialize() zeroes uiTremorTimer 60000 / iAwakenTimer 0 /
-- uiWallMinionTimer 10000 + flags + instance = creature->
-- GetInstanceScript(); Reset -> Initialize() + instance->SetData
-- (0, 5) + FACTION_FRIENDLY + NOT_SELECTABLE + ROOT + AddAura
-- (16245 SPELL_FREEZE_ANIM); ActivateMinion(guid, flag) -> DoCast
-- (minion, 10258 AWAKEN_VAULT_WALKER, flag) + minion self-cast
-- 10347 + RemoveFlag NOT_SELECTABLE + ROOT off + FACTION_MONSTER
-- + RemoveAura(10255); JustEngagedWith -> FACTION_MONSTER +
-- RemoveFlag NOT_SELECTABLE + ROOT off; SpellHit(10347 SPELL_
-- ARCHAEDAS_AWAKEN) -> Talk(SAY_AGGRO = 0) + iAwakenTimer 4000 +
-- bWakingUp; KilledUnit -> Talk(SAY_KILL = 3); UpdateAI: waking-
-- up animation gate (4s) -> AttackStart(instance->GetGuidData
-- (0) = whoWokeuiArchaedasGUID); uiWallMinionTimer 10s -> instance
-- ->SetData(DATA_MINIONS, IN_PROGRESS); <66% -> ActivateMinion
-- (GetGuidData 5..10) + Talk(SAY_SUMMON_GUARDIANS = 1); <33% ->
-- ActivateMinion(GetGuidData 1..4) + Talk(SAY_SUMMON_VAULT_
-- WALKERS = 2); uiTremorTimer 60s init -> 45s re-arm: DoCastVictim
-- (6524 SPELL_GROUND_TREMOR); DoMeleeAttackIfReady; JustDied ->
-- instance->SetData(DATA_ANCIENT_DOOR, DONE) + SetData(DATA_
-- MINIONS, SPECIAL); GetAI via GetUldamanAI -> GetInstanceAI;
-- npc_archaedas_minions : public ScriptedAI (Reset -> FRIENDLY +
-- NOT_SELECTABLE + ROOT + RemoveAllAuras + AddAura(10255);
-- JustEngagedWith -> MONSTER + RemoveAllAuras + flag/root off +
-- bAmIAwake; SpellHit(10347) -> 5s awaken animation; UpdateAI
-- awaken gate -> AttackStart(GetGuidData(0)), else melee-only;
-- npc_stonekeepers : public ScriptedAI (Reset -> FRIENDLY +
-- NOT_SELECTABLE + ROOT + RemoveAllAuras + AddAura(10255);
-- JustEngagedWith -> MONSTER + flag/root off; UpdateAI melee-
-- only; JustDied -> DoCast(me, 9874 SPELL_SELF_DESTRUCT, true) +
-- instance->SetData(DATA_STONE_KEEPERS, IN_PROGRESS);
-- go_altar_of_archaedas (GameObjectAI): OnGossipHello -> player
-- self-cast 11206 SPELL_BOSS_OBJECT_VISUAL + instance->SetGuid
-- Data(0, player GUID); loader lines 149/328).
-- Entry (verifiable from the C++ sources): instance_uldaman.cpp
-- OnCreatureCreate switches on the raw entries with
-- name-comments and stores their GUIDs in the vectors the boss
-- AI pulls back through instance->GetGuidData: 2748 "Archaedas"
-- -> archaedasGUID (used by the boss's DATA_MINIONS / DATA_
-- ANCIENT_DOOR handlers); 4857 "Stone Keeper" -> stoneKeepers
-- (the DATA_STONE_KEEPERS list the npc_stonekeepers JustDied
-- SetData leg advances); 7076 "Earthen Guardian" ->
-- earthenGuardians (GetGuidData 5..10 = the boss's
-- EarthenGuardian1..6 arms); 10120 "Vault Walker" ->
-- vaultWalkers (GetGuidData 1..4 = the boss's VaultWalker1..4
-- arms); 7309 "Earthen Custodian" / 7077 "Earthen Hallshaper"
-- -> archaedasWallMinions (the wall minions the DATA_MINIONS
-- SetData leg advances; SDComment: every 10s he awakens one).
-- The name-to-entry tie is C++-verified (kalecgos entry-
-- verifiability check, ramstein strength: sole-source comment
-- ties, not DB).
-- Whole-server-tree grep confirms boss_archaedas.cpp as the
-- sole source of boss_archaedas / npc_archaedas_minions /
-- npc_stonekeepers / go_altar_of_archaedas (loader lines only
-- otherwise).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. There is no UNIT_STATE_CASTING gate in
-- Go, so timers fire unconditionally (maiden precedent); the C++
-- UpdateAI timer arms map onto OnEnterCombat timers with the
-- C++-exact init delays (maiden convention).
-- Verifiable numbers (file's own enums): SPELL_GROUND_TREMOR =
-- 6524 / SPELL_SELF_DESTRUCT = 9874 / SPELL_BOSS_OBJECT_VISUAL
-- = 11206 / SPELL_ARCHAEDAS_AWAKEN = 10347 / SPELL_AWAKEN_
-- VAULT_WALKER = 10258 / SPELL_AWAKEN_EARTHEN_GUARDIAN = 10252
-- / SPELL_FREEZE_ANIM = 16245 / SPELL_MINION_FREEZE_ANIM =
-- 10255; SAY_AGGRO = 0 / SAY_SUMMON_GUARDIANS = 1 / SAY_SUMMON_
-- VAULT_WALKERS = 2 / SAY_KILL = 3.
-- Ported arms:
-- - Archaedas OnEnterCombat(1): Ground Tremor 60s init -> 45s
--   re-arm; DoCastVictim(6524) non-triggered -> GetVictim +
--   CastSpell (mr_smite convention).
-- - Stone Keeper OnDied(4): DoCast(me, 9874, true) triggered ->
--   creature:CastSpell(creature, SPELL_SELF_DESTRUCT, true)
--   (headless_horseman / omen onDied self-cast precedent).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - Archaedas Reset's instance->SetData(0, 5) respawn-minions
--   leg + FACTION_FRIENDLY / NOT_SELECTABLE / ROOT legs +
--   AddAura(16245) freeze leg: instance-model blocked; the
--   freeze-aura self-cast is deferred with the stranded-half
--   convention (npc_captured_rageclaw / npc_phoenix_tk).
-- - JustEngagedWith's faction/flag/root legs: no flag/faction
--   bridges.
-- - SpellHit(10347) on boss and minions (the whole awaken
--   sequence: Talk(SAY_AGGRO), 4s/5s waking-up timers,
--   AttackStart(instance->GetGuidData(0))): CreatureEventOn
--   SpellHitTarget = 15 never fires in the Go engine
--   (stratholme finding) — STRAND; registering the timers
--   without the SpellHit arm would be a firesworn stub.
-- - KilledUnit Talk(SAY_KILL = 3): no KilledUnit bridge in
--   engine/scripting (no CreatureEvent id declared) — queued
--   (standing).
-- - uiWallMinionTimer 10s -> instance->SetData(DATA_MINIONS,
--   IN_PROGRESS): instance-model blocked; the timer alone
--   would be a firesworn stub.
-- - The guardian (<66%) and vault-walker (<33%) awaken arms:
--   every leg runs through instance->GetGuidData(5..10 /
--   1..4) and ActivateMinion's DoCast(minion, 10258/10252) /
--   self-cast(10347) / flag/faction/aura legs — instance-model
--   blocked plus no flag/faction/aurastrip bridges.
-- - JustDied's SetData(DATA_ANCIENT_DOOR, DONE) /
--   SetData(DATA_MINIONS, SPECIAL) legs: instance-model
--   blocked (standing).
-- - GetUldamanAI -> GetInstanceAI leg: instance-script model
--   blocked (standing).
-- - npc_archaedas_minions (7309/7077/7076/10120): no bridgeable
--   arms at all — Reset/JustEngagedWith are faction/flag/root/
--   freeze-aura state legs with no bridges, the SpellHit(10347)
--   awaken sequence never fires, and the remainder is engine-
--   driven melee. Unregistered (registering a hook with zero
--   bridgeable arms would be a firesworn stub), documented
--   here.
-- - npc_stonekeepers Reset/JustEngagedWith faction/flag/root
--   legs: no bridges; JustDied's instance->SetData(DATA_
--   STONE_KEEPERS, IN_PROGRESS) leg: instance-model blocked.
-- - go_altar_of_archaedas: entry-unverifiable (no GO entry for
--   the altar is named anywhere in src/server — fails the
--   kalecgos check); its OnGossipHello arms (player self-cast
--   11206 + instance->SetGuidData(0, ...)) are bridgeable-in-
--   principle but unregistrable + instance-blocked. QUEUED with
--   the entry-unverifiable list, pending entry evidence.

local NPC_ARCHAEDAS = 2748
local NPC_STONE_KEEPER = 4857

local SPELL_GROUND_TREMOR = 6524
local SPELL_SELF_DESTRUCT = 9874

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

local function onGroundTremor(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_GROUND_TREMOR)
    end
    schedule(guid, "tremor", 45000, function() onGroundTremor(creature, guid) end)
end

local function onArchaedasCombatStart(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "tremor", 60000, function() onGroundTremor(creature, guid) end)
end

local function onArchaedasCombatEnd(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onStoneKeeperDied(event, creature)
    creature:CastSpell(creature, SPELL_SELF_DESTRUCT, true)
end

RegisterCreatureEvent(NPC_ARCHAEDAS, 1, onArchaedasCombatStart)
RegisterCreatureEvent(NPC_ARCHAEDAS, 2, onArchaedasCombatEnd)
RegisterCreatureEvent(NPC_ARCHAEDAS, 4, onArchaedasCombatEnd)
RegisterCreatureEvent(NPC_ARCHAEDAS, 23, onArchaedasCombatEnd)
RegisterCreatureEvent(NPC_STONE_KEEPER, 4, onStoneKeeperDied)
