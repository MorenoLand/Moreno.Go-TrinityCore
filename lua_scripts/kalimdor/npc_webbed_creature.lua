-- Webbed Creature (Bloodmyst Isle) --
-- Lua port of src/server/scripts/Kalimdor/zone_bloodmyst_isle.cpp
-- (npc_webbed_creatureAI : public NullCreatureAI; registered by
-- AddSC_bloodmyst_isle in kalimdor loader).
-- Entry: 17680 (TDB-verified: 'Webbed Creature', faction 14, 39
-- spawns map 530 Bloodmyst Isle, both TDB dumps 25101/26091;
-- ScriptName is '' in this DB so the C++ AI never binds C++-side,
-- but the entry is unambiguous by name + spawns + the file's
-- ContentData naming npc_webbed_creature).
-- Ported arm: JustDied (zone_bloodmyst_isle.cpp:68-91) — killer
-- guard; urand(0,2): case 0 -> a player killer gets
-- KilledMonsterCredit(NPC_EXPEDITION_RESEARCHER = 17681) and the
-- researcher is summoned; cases 1-2 -> one entry from
-- possibleSpawns[31] is summoned. Summon at own position (0,0,0
-- close-point twin), own orientation (o 0 twin),
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT (3), 1min.
-- Vacuous overrides (NullCreatureAI, empty bodies): Reset,
-- JustEngagedWith, AttackStart, MoveInLineOfSight — nothing to
-- register.
-- Bridges: SummonCreature (lua_creature_events.go:4460, all-zero
-- close-point + o==0 orientation twins), KilledMonsterCredit
-- (lua_player.go:5297; killer:GetTypeId() == 4 torloth precedent),
-- math.random (urand twin). No aggro leg: C++ summons with no
-- DoZoneInCombat/AttackStart, and the bridge does not auto-aggro.

local ENTRY_WEBBED_CREATURE = 17680

local NPC_EXPEDITION_RESEARCHER = 17681

local SUMMON_TYPE_TIMED_DESPAWN_OUT_OF_COMBAT = 3

local SUMMON_DESPAWN_MS = 60000

local TYPEID_PLAYER = 4

-- zone_bloodmyst_isle.cpp:44, verbatim order.
local POSSIBLE_SPAWNS = {17322, 17661, 17496, 17522, 17340, 17352, 17333, 17524, 17654, 17348, 17339, 17345, 17359, 17353, 17336, 17550, 17330, 17701, 17321, 17325, 17320, 17683, 17342, 17715, 17334, 17341, 17338, 17337, 17346, 17344, 17327}

local function onDied(_, creature, killer)
    if not killer then
        return
    end
    local spawnEntry
    if math.random(0, 2) == 0 then
        if killer:GetTypeId() == TYPEID_PLAYER then
            killer:KilledMonsterCredit(NPC_EXPEDITION_RESEARCHER)
        end
        spawnEntry = NPC_EXPEDITION_RESEARCHER
    else
        spawnEntry = POSSIBLE_SPAWNS[math.random(1, 31)]
    end
    creature:SummonCreature(spawnEntry, 0, 0, 0, 0, SUMMON_TYPE_TIMED_DESPAWN_OUT_OF_COMBAT, SUMMON_DESPAWN_MS)
end

RegisterCreatureEvent(ENTRY_WEBBED_CREATURE, 4, onDied)
