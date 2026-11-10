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
-- DoMeleeAttackIfReady. C++ has no UNIT_STATE_CASTING gate (zero
-- HasUnitState hits in boss_archaedas.cpp, all three AIs), so no
-- defer applies and timers fire unconditionally, matching C++ (the
-- "no bridge" phrasing is stale — IsCasting is live at
-- lua_creature_events.go:2275); the C++ UpdateAI timer arms map onto
-- OnEnterCombat timers with the C++-exact init delays (maiden
-- convention).
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
-- - Archaedas KilledUnit (event 3, CREATURE_EVENT_ON_TARGET_DIED —
--   live in Go via fireCreatureTargetDied in
--   engine/world/lua_creature_events.go): Talk(SAY_KILL = 3),
--   unconditional (the C++ ignores the victim). The old
--   "no KilledUnit bridge" note was stale.
-- - Archaedas JustEngagedWith faction/flag/root legs (ev1): SetFaction
--   (14) + RemoveFlag(59, 0x02000000) + SetRooted(false).
-- - Wall-minion 10s pump (ev1-scheduled, instance_uldaman.cpp
--   ActivateWallMinions): first alive non-MONSTER wall minion
--   (7309/7077) via GetCreaturesInRange, same unfreeze legs as
--   ActivateMinion + archaedas CastSpell(target, 10258, true) +
--   target self-cast 10347; re-arms every 10s in combat even when
--   none remains (C++ SetData leg is unconditional).
-- - Guardian/vault-walker awaken arms (1s health poll):
--   <=66% awakens all 7076 (flag true except the last of the
--   range list) + Talk(1); <=33% awakens all 10120 + Talk(2);
--   per-guid latches. Health comparison is integer-exact
--   (health <= floor(max*pct/100), !HealthAbovePct twin).
-- - Archaedas SpellHit(10347) (ev14): Talk(0) + 4s timer ->
--   AttackStart(whoWoke player); the attack leg runs once the
--   altar has set whoWokeGuid (altar ported, see below).
-- - Archaedas JustDied (ev4): opens the temple door (141869) and
--   the ancient vault door (124369) via SetGoState(0) + despawns
--   every alive MONSTER-faction minion (DeActivateMinions twin).
-- - Stone Keeper JustDied continuation: after the 9874 self-cast,
--   unfreeze the first alive stone keeper (4857); if none is
--   alive, open the keeper temple door (124367) + the archaedas
--   temple door (141869).
-- - Minions (7076/10120/7309/7077) + stone keepers (4857):
--   Reset (ev23) freeze legs, JustEngagedWith (ev1) unfreeze
--   legs, minion SpellHit(10347) (ev14) 5s -> AttackStart
--   (whoWoke-guarded, same as the boss arm).
-- - go_altar_of_archaedas (133234): OnGossipHello -> player
--   self-cast 11206 + ActivateArchaedas (RemoveAura 16245 +
--   self-cast 10347 non-triggered + SetFaction 415 + RemoveFlag
--   NOT_SELECTABLE + whoWoke = player + close temple door
--   141869); returns false (native GO use arms proceed).
-- - Archaedas Reset (ev23) freeze legs: FACTION_FRIENDLY +
--   SetFlag NOT_SELECTABLE + SetRooted(true) + AddAura(16245);
--   pairs with the altar awaken path (re-freeze on evade, altar
--   re-triggers the fight).
-- Fidelity deltas (range-query emulation, no instance script):
-- GetCreaturesInRange returns distance order, C++ walks spawn-
-- order GUID vectors — which guardian is the flag=false one and
-- which wall minion wakes first can differ from C++; counts,
-- timers, and spells are exact.
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - Archaedas Reset's instance->SetData(0, 5) respawn-minions
--   leg: C++ NO-OP, deliberately not ported — the args are
--   swapped (type 0 has no SetData case; the intended
--   SetData(DATA_MINIONS, NOT_STARTED) is never called anywhere
--   in the tree), so RespawnMinions never fires from Reset in
--   this C++ tree.
-- - The instance script itself (instance_uldaman.cpp): SetData /
--   GetData / SetGuidData / ProcessEvent / OnCreatureCreate /
--   Load / SaveToDB have no instance-script bridge — the script
--   as a unit stays DOCUMENTED-BLOCKED; the legs above are
--   Lua-side per-leg emulation, not a port of the instance model.
--   Encounter persistence (doors re-opening on instance load)
--   has no Lua equivalent.
-- - First stone-keeper activation: ProcessEvent(EVENT_SUB_BOSS_
--   AGGRO = 2228) is a DB-driven instance event with no Lua
--   instance-event bridge — the chain cannot start; only the
--   JustDied continuation above is ported.
-- - GetUldamanAI -> GetInstanceAI leg: instance-script model
--   blocked (standing).
-- - The old "flag/aura-strip still bridgeless" claim is STALE:
--   SetFlag (lua_creature_events.go:6022), RemoveFlag (:6087),
--   RemoveAura (:1571), AddAura (:2652) are all live on the
--   creature surface; the stay-outs above are machine/entry
--   grounds, not bridge grounds.

local NPC_ARCHAEDAS = 2748
local NPC_STONE_KEEPER = 4857
local NPC_EARTHEN_GUARDIAN = 7076
local NPC_VAULT_WALKER = 10120
local NPC_EARTHEN_CUSTODIAN = 7309
local NPC_EARTHEN_HALLSHAPER = 7077

local SPELL_GROUND_TREMOR = 6524
local SPELL_SELF_DESTRUCT = 9874
local SPELL_ARCHAEDAS_AWAKEN = 10347
local SPELL_AWAKEN_VAULT_WALKER = 10258
local SPELL_MINION_FREEZE_ANIM = 10255
local SPELL_BOSS_OBJECT_VISUAL = 11206
local SPELL_FREEZE_ANIM = 16245

local SAY_AGGRO = 0
local SAY_SUMMON_GUARDIANS = 1
local SAY_SUMMON_VAULT_WALKERS = 2
local SAY_KILL = 3

local FACTION_FRIENDLY = 35
local FACTION_MONSTER = 14
local FACTION_TITAN = 415

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local GO_ALTAR_OF_ARCHAEDAS = 133234
local GO_ARCHAEDAS_TEMPLE_DOOR = 141869
local GO_ALTAR_OF_THE_KEEPER_TEMPLE_DOOR = 124367
local GO_ANCIENT_VAULT_DOOR = 124369

local SEARCH_RANGE = 100.0

local timers = {}
local state = {}

local function getState(guid)
    local st = state[guid]
    if not st then
        st = { guardiansAwake = false, vaultWalkersAwake = false, whoWokeGuid = 0 }
        state[guid] = st
    end
    return st
end

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

-- C++ !HealthAbovePct(pct) = health <= CountPctFromMaxHealth(pct),
-- floor(max*pct/100) (ironaya healthBelowPct convention, inverted).
local function healthNotAbovePct(creature, pct)
    local max = creature:GetMaxHealth()
    if max == 0 then
        return false
    end
    return creature:GetHealth() <= math.floor(max * pct / 100)
end

-- C++ ActivateMinion (boss_archaedas.cpp:112-124): DoCast(minion,
-- 10258, flag) + minion self-cast 10347 triggered + RemoveFlag
-- NOT_SELECTABLE + ROOT off + FACTION_MONSTER + RemoveAura(10255).
-- The minion's own SpellHit(10347) arm needs the whoWoke player, so
-- the archaedas guid's whoWokeGuid rides along into the minion state.
local function activateMinion(archaedas, guid, minion, flag)
    if not minion or minion:IsDead() then
        return
    end
    archaedas:CastSpell(minion, SPELL_AWAKEN_VAULT_WALKER, flag)
    minion:CastSpell(minion, SPELL_ARCHAEDAS_AWAKEN, true)
    minion:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    minion:SetRooted(false)
    minion:SetFaction(FACTION_MONSTER)
    minion:RemoveAura(SPELL_MINION_FREEZE_ANIM)
    local st = getState(guid)
    if st.whoWokeGuid ~= 0 then
        getState(minion:GetGUIDLow()).whoWokeGuid = st.whoWokeGuid
    end
end

-- C++ ActivateWallMinions (instance_uldaman.cpp:199-219): first
-- alive wall minion whose faction is not yet MONSTER, in spawn
-- order; the range query returns distance order instead (documented
-- delta — one minion per 10s either way).
local function firstFrozenWallMinion(creature)
    for _, entry in ipairs({ NPC_EARTHEN_CUSTODIAN, NPC_EARTHEN_HALLSHAPER }) do
        local minions = creature:GetCreaturesInRange(SEARCH_RANGE, entry)
        if minions then
            for _, minion in ipairs(minions) do
                if minion:GetFaction() ~= FACTION_MONSTER then
                    return minion
                end
            end
        end
    end
    return nil
end

local function onWallMinion(creature, guid)
    local minion = firstFrozenWallMinion(creature)
    if minion then
        activateMinion(creature, guid, minion, true)
    end
    schedule(guid, "wall", 10000, function() onWallMinion(creature, guid) end)
end

-- C++ UpdateAI guardian/vault-walker arms (boss_archaedas.cpp:
-- 174-194): <66% awakens EarthenGuardian1..6 (flag true except
-- the 6th), <33% awakens VaultWalker1..4 (flag true except the
-- 4th); the flag=false minion is the last of the range list
-- (distance order, not C++ spawn order — documented delta).
local function awakenGroup(creature, guid, entry, lastFlagFalse)
    local minions = creature:GetCreaturesInRange(SEARCH_RANGE, entry)
    if not minions then
        return
    end
    for i, minion in ipairs(minions) do
        local flag = true
        if lastFlagFalse and i == #minions then
            flag = false
        end
        activateMinion(creature, guid, minion, flag)
    end
end

local function onHealthPoll(creature, guid)
    local st = getState(guid)
    if not st.guardiansAwake and healthNotAbovePct(creature, 66) then
        awakenGroup(creature, guid, NPC_EARTHEN_GUARDIAN, true)
        creature:Talk(SAY_SUMMON_GUARDIANS)
        st.guardiansAwake = true
    end
    if not st.vaultWalkersAwake and healthNotAbovePct(creature, 33) then
        awakenGroup(creature, guid, NPC_VAULT_WALKER, true)
        creature:Talk(SAY_SUMMON_VAULT_WALKERS)
        st.vaultWalkersAwake = true
    end
    schedule(guid, "health", 1000, function() onHealthPoll(creature, guid) end)
end

local function onGroundTremor(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_GROUND_TREMOR)
    end
    schedule(guid, "tremor", 45000, function() onGroundTremor(creature, guid) end)
end

local function onArchaedasCombatStart(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ boss_archaedas.cpp:126-130: JustEngagedWith -> FACTION_
    -- MONSTER + RemoveFlag NOT_SELECTABLE + SetControlled(false,
    -- UNIT_STATE_ROOT).
    creature:SetFaction(FACTION_MONSTER)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetRooted(false)
    schedule(guid, "tremor", 60000, function() onGroundTremor(creature, guid) end)
    schedule(guid, "wall", 10000, function() onWallMinion(creature, guid) end)
    schedule(guid, "health", 1000, function() onHealthPoll(creature, guid) end)
end

-- C++ SpellHit (boss_archaedas.cpp:131-140): 10347 -> Talk(AGGRO)
-- + 4s waking-up timer -> AttackStart(whoWokeArchaedasGUID). The
-- whoWoke GUID is set by the altar handler (instance_uldaman.cpp
-- SetGuidData(0)); without it the attack leg is skipped — the boss
-- still engages directly via ev1.
local function onArchaedasSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_ARCHAEDAS_AWAKEN then
        return
    end
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
    schedule(guid, "awaken", 4000, function()
        local st = state[guid]
        if st and st.whoWokeGuid ~= 0 then
            local player = GetPlayerByGUID(st.whoWokeGuid)
            if player then
                creature:AttackStart(player)
            end
        end
    end)
end

-- C++ boss_archaedas.cpp:102-108: Reset -> Initialize() +
-- instance->SetData(0, 5) (a C++ no-op — swapped args, see the
-- header) + SetFaction(FACTION_FRIENDLY) + SetFlag(UNIT_FIELD_
-- FLAGS, UNIT_FLAG_NOT_SELECTABLE) + SetControlled(true,
-- UNIT_STATE_ROOT) + AddAura(SPELL_FREEZE_ANIM). Ev23 only:
-- ev2/ev4 stay on onArchaedasCombatEnd (C++ JustDied never
-- Resets, so a corpse must not re-root). The freeze legs pair
-- with the altar awaken path — after an evade the boss re-freezes
-- and the altar re-triggers the fight.
local function onArchaedasReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
    creature:SetFaction(FACTION_FRIENDLY)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetRooted(true)
    creature:AddAura(SPELL_FREEZE_ANIM)
end

local function onArchaedasCombatEnd(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ JustDied (boss_archaedas.cpp:209-213) -> SetData(DATA_
-- ANCIENT_DOOR, DONE) opens the temple door (141869) and the
-- ancient vault door (124369); SetData(DATA_MINIONS, SPECIAL) ->
-- DeActivateMinions despawns every alive MONSTER-faction minion.
local function onArchaedasDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
    for _, entry in ipairs({ GO_ARCHAEDAS_TEMPLE_DOOR, GO_ANCIENT_VAULT_DOOR }) do
        local door = creature:GetNearestGameObject(SEARCH_RANGE, entry)
        if door then
            door:SetGoState(0)
        end
    end
    for _, entry in ipairs({ NPC_EARTHEN_CUSTODIAN, NPC_EARTHEN_HALLSHAPER, NPC_EARTHEN_GUARDIAN, NPC_VAULT_WALKER }) do
        local minions = creature:GetCreaturesInRange(SEARCH_RANGE, entry)
        if minions then
            for _, minion in ipairs(minions) do
                if minion:GetFaction() == FACTION_MONSTER then
                    minion:DespawnOrUnsummon()
                end
            end
        end
    end
end

-- C++ npc_stonekeepers::JustDied (boss_archaedas.cpp:378-381):
-- DoCast(me, 9874, true) + instance->SetData(DATA_STONE_KEEPERS,
-- IN_PROGRESS) -> ActivateStoneKeepers (instance_uldaman.cpp:
-- 177-197): unfreeze the first alive stone keeper; if none is
-- alive, open the altar-of-the-keeper temple door (124367) and
-- the archaedas temple door (141869). The chain's first
-- activation (ProcessEvent 2228, DB-driven) has no Lua bridge,
-- so the continuation only runs once a keeper has died.
local function onStoneKeeperDied(event, creature)
    creature:CastSpell(creature, SPELL_SELF_DESTRUCT, true)
    local keepers = creature:GetCreaturesInRange(SEARCH_RANGE, NPC_STONE_KEEPER)
    local nextKeeper = nil
    if keepers then
        for _, keeper in ipairs(keepers) do
            if keeper:GetGUIDLow() ~= creature:GetGUIDLow() then
                nextKeeper = keeper
                break
            end
        end
    end
    if nextKeeper then
        nextKeeper:SetRooted(false)
        nextKeeper:SetFaction(FACTION_MONSTER)
        nextKeeper:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        nextKeeper:RemoveAura(SPELL_MINION_FREEZE_ANIM)
        return
    end
    for _, entry in ipairs({ GO_ALTAR_OF_THE_KEEPER_TEMPLE_DOOR, GO_ARCHAEDAS_TEMPLE_DOOR }) do
        local door = creature:GetNearestGameObject(SEARCH_RANGE, entry)
        if door then
            door:SetGoState(0)
        end
    end
end

-- C++ boss_archaedas::KilledUnit — Talk(SAY_KILL), unconditional
-- (the C++ body ignores the victim).
local function onArchaedasTargetDied(event, creature, victim)
    creature:Talk(SAY_KILL)
end

-- C++ npc_archaedas_minions::Reset (boss_archaedas.cpp:240-247)
-- and npc_stonekeepers::Reset (:345-352): FRIENDLY + NOT_SELECTABLE
-- + ROOT + RemoveAllAuras + AddAura(10255). JustEngagedWith
-- (minions :249-256, stonekeepers :354-359): MONSTER + flag/root
-- off (minions also RemoveAllAuras). SpellHit(10347) on minions
-- (:259-265): 5s waking-up timer -> AttackStart(whoWokeGUID);
-- the whoWoke GUID rides in the minion's state from activateMinion
-- and is skipped when unknown (no altar click yet).
local function onMinionReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
    creature:SetFaction(FACTION_FRIENDLY)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetRooted(true)
    creature:RemoveAllAuras()
    creature:AddAura(SPELL_MINION_FREEZE_ANIM)
end

local function onMinionCombatStart(event, creature)
    creature:SetFaction(FACTION_MONSTER)
    creature:RemoveAllAuras()
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetRooted(false)
end

local function onStoneKeeperReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
    creature:SetFaction(FACTION_FRIENDLY)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetRooted(true)
    creature:RemoveAllAuras()
    creature:AddAura(SPELL_MINION_FREEZE_ANIM)
end

local function onStoneKeeperCombatStart(event, creature)
    creature:SetFaction(FACTION_MONSTER)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetRooted(false)
end

local function onMinionSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_ARCHAEDAS_AWAKEN then
        return
    end
    local guid = creature:GetGUIDLow()
    schedule(guid, "awaken", 5000, function()
        local st = state[guid]
        if st and st.whoWokeGuid ~= 0 then
            local player = GetPlayerByGUID(st.whoWokeGuid)
            if player then
                creature:AttackStart(player)
            end
        end
    end)
end

RegisterCreatureEvent(NPC_ARCHAEDAS, 1, onArchaedasCombatStart)
RegisterCreatureEvent(NPC_ARCHAEDAS, 2, onArchaedasCombatEnd)
RegisterCreatureEvent(NPC_ARCHAEDAS, 3, onArchaedasTargetDied)
RegisterCreatureEvent(NPC_ARCHAEDAS, 4, onArchaedasDied)
RegisterCreatureEvent(NPC_ARCHAEDAS, 14, onArchaedasSpellHit)
RegisterCreatureEvent(NPC_ARCHAEDAS, 23, onArchaedasReset)
RegisterCreatureEvent(NPC_STONE_KEEPER, 1, onStoneKeeperCombatStart)
RegisterCreatureEvent(NPC_STONE_KEEPER, 4, onStoneKeeperDied)
RegisterCreatureEvent(NPC_STONE_KEEPER, 23, onStoneKeeperReset)
for _, entry in ipairs({ NPC_EARTHEN_GUARDIAN, NPC_VAULT_WALKER, NPC_EARTHEN_CUSTODIAN, NPC_EARTHEN_HALLSHAPER }) do
    RegisterCreatureEvent(entry, 1, onMinionCombatStart)
    RegisterCreatureEvent(entry, 14, onMinionSpellHit)
    RegisterCreatureEvent(entry, 23, onMinionReset)
end

-- C++ go_altar_of_archaedasAI::OnGossipHello (boss_archaedas.cpp:
-- 392-400): player self-cast 11206 SPELL_BOSS_OBJECT_VISUAL +
-- instance->SetGuidData(0, player GUID) -> ActivateArchaedas
-- (instance_uldaman.cpp:256-268): RemoveAura(16245) + self-cast
-- 10347 non-triggered + SetFaction(415 FACTION_TITAN) + RemoveFlag
-- NOT_SELECTABLE + whoWokeArchaedasGUID = player; SetDoor(
-- archaedasTempleDoor, false) closes the temple door (141869).
-- Returns false in C++ (native GO use arms proceed). Entry 133234
-- triple-confirmed 2026-11-10 (Evowow WotLK DB, ClassicDB, NFU
-- WotLK DB — all "Altar of Archaedas", Uldaman, Khaz'goroth's
-- Seat, related spells "Uldaman Boss Agro" + "Uldaman Boss Object
-- Visual"; distinct from the Altar of the Keepers, 130511).
local function onAltarGossip(event, player, go)
    if not player then
        return false
    end
    player:CastSpell(player, SPELL_BOSS_OBJECT_VISUAL)
    local archaedas = player:FindNearestCreature(NPC_ARCHAEDAS, SEARCH_RANGE)
    if archaedas then
        archaedas:RemoveAura(SPELL_FREEZE_ANIM)
        archaedas:CastSpell(archaedas, SPELL_ARCHAEDAS_AWAKEN)
        archaedas:SetFaction(FACTION_TITAN)
        archaedas:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        getState(archaedas:GetGUIDLow()).whoWokeGuid = player:GetGUID()
        local door = archaedas:GetNearestGameObject(SEARCH_RANGE, GO_ARCHAEDAS_TEMPLE_DOOR)
        if door then
            door:SetGoState(1)
        end
    end
    return false
end

RegisterGameObjectGossipEvent(GO_ALTAR_OF_ARCHAEDAS, 1, onAltarGossip)
