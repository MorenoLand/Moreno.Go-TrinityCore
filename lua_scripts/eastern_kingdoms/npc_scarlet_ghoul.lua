-- npc_scarlet_ghoul (The Scarlet Enclave, Quest 12698 "The Gift
-- That Keeps On Giving") -- Lua port of the npc_scarlet_ghoul
-- struct in src/server/scripts/EasternKingdoms/ScarletEnclave/
-- chapter1.cpp (struct npc_scarlet_ghoul line 1002 : public
-- ScriptedAI; AddSC_the_scarlet_enclave_c1 registers it with
-- RegisterCreatureAI; EK loader decl :95 / call :273).
-- Entry: 28845 ONLY (TDB 26091 creature_template, verified against
-- the dump's own CREATE TABLE column order: 28845 is the sole
-- Scarlet Ghoul row carrying ScriptName 'npc_scarlet_ghoul',
-- lvl 44-46, faction 2084, npcflag 0). The ghouls are TempSummons
-- owned by the player (summoned by the harvester spells 52490 /
-- 52505, spell_gift_of_the_harvester -- SpellScript, no bridge).
-- Verifiable numbers (chapter1.cpp:950-955 TheGiftThatKeepsOnGiving
-- enum): NPC_GHOULS = 28845 / NPC_GHOSTS = 28846 / SAY_LINE_0 = 0.
-- REACT_DEFENSIVE = 1 (UnitDefines.h: REACT_PASSIVE 0, DEFENSIVE 1,
-- AGGRESSIVE 2 -- lua_creature_events.go SetReactState comment).
-- Eluna creature events: 5 OnSpawn (constructor twin), 22
-- OnSummoned (JustAppeared twin for a TempSummon -- dark rider
-- precedent: ev22 fires after CreatedByGUID is stamped), 7
-- OnAIUpdate (fires every 100ms world tick, creaturemotion.go
-- :5566-5571 -- the true UpdateAI twin; the handler returns
-- nothing, so no veto: the engine's UpdateVictim + melee step
-- below still runs, matching C++ DoMeleeAttackIfReady).
-- Ported arms (chapter1.cpp:1002-1073):
-- - Constructor (chapter1.cpp:1004-1007): SetReactState
--   (REACT_DEFENSIVE) -- rides ev5, spawn-time like the C++
--   constructor. The ghoul must not auto-aggro: all of its
--   attacking goes through the UpdateAI legs below.
-- - JustAppeared (chapter1.cpp:1010-1017): urand(0, 1) -> if the
--   ghoul has an owner, Talk(SAY_LINE_0 0, owner) -- rides ev22,
--   math.random() < 0.5 is the urand twin (maiden/moroes
--   precedent). Talk group 0 has NO creature_text row for 28845
--   in TDB 26091, so the whisper is silent in the current DB
--   state; the call is faithful and speaks when rows exist.
-- - FindMinions (chapter1.cpp:1019-1037): the owner's minions by
--   entry 28845 (player GetAllMinionsByEntry, live on both
--   surfaces); for each minion whose owner GUID equals my owner
--   GUID, if it is in combat and has a victim (GetVictim is the
--   getAttackerForHelper twin -- creaturemotion.go:3980, "the
--   engaged victim"), AttackStart that victim. No break: C++
--   AttackStarts each qualifying minion's attacker in turn.
-- - UpdateAI (chapter1.cpp:1039-1073): out of combat, if the owner
--   is a player in combat, AttackStart the owner's victim when it
--   is a ghost (entry 28846, victim type-gated -- a player victim
--   has no entry, so it falls through to FindMinions exactly like
--   C++ GetEntry() == 0 != 28846), else FindMinions(owner). The
--   in-combat ghost leg (EnsureVictim is a ghost + isAttackReady +
--   IsWithinMeleeRange -> AttackerStateUpdate + resetAttackTimer)
--   is NOT ported: the Go engine combat tick covers melee
--   (initiate header precedent) and porting AttackerStateUpdate
--   would double-swing.

local ENTRY_SCARLET_GHOUL = 28845
local NPC_GHOULS = 28845
local NPC_GHOSTS = 28846
local SAY_LINE_0 = 0
local REACT_DEFENSIVE = 1

-- C++ npc_scarlet_ghoul constructor (chapter1.cpp:1004-1007).
local function onSpawn(_, creature)
    creature:SetReactState(REACT_DEFENSIVE)
end

-- C++ npc_scarlet_ghoul::JustAppeared (chapter1.cpp:1010-1017).
local function onSummoned(_, creature)
    if math.random() < 0.5 then
        local owner = creature:GetOwner()
        if owner then
            creature:Talk(SAY_LINE_0, owner)
        end
    end
end

-- C++ npc_scarlet_ghoul::FindMinions (chapter1.cpp:1019-1037).
local function findMinions(creature, owner)
    local minions = owner:GetAllMinionsByEntry(NPC_GHOULS)
    if minions == nil then
        return
    end
    local myOwnerGUID = creature:GetOwnerGUID()
    for _, minion in ipairs(minions) do
        if minion:GetOwnerGUID() == myOwnerGUID and minion:IsInCombat() then
            local target = minion:GetVictim()
            if target ~= nil then
                creature:AttackStart(target)
            end
        end
    end
end

-- C++ npc_scarlet_ghoul::UpdateAI (chapter1.cpp:1039-1073), minus
-- the engine-covered in-combat ghost melee leg (see header).
local function onAIUpdate(_, creature)
    if creature:IsInCombat() then
        return
    end
    local owner = creature:GetOwner()
    if owner == nil or not owner:IsInCombat() then
        return
    end
    local attacker = owner:GetVictim()
    if attacker ~= nil and attacker:GetObjectType() == "Creature" and attacker:GetEntry() == NPC_GHOSTS then
        creature:AttackStart(attacker)
        return
    end
    findMinions(creature, owner)
end

RegisterCreatureEvent(ENTRY_SCARLET_GHOUL, 5, onSpawn)
RegisterCreatureEvent(ENTRY_SCARLET_GHOUL, 22, onSummoned)
RegisterCreatureEvent(ENTRY_SCARLET_GHOUL, 7, onAIUpdate)
