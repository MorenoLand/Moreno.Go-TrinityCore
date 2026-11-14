-- npc_dkc1_gothik (The Scarlet Enclave, Quest 12698 "The Gift
-- That Keeps On Giving") -- Lua port of the npc_dkc1_gothik
-- struct in src/server/scripts/EasternKingdoms/ScarletEnclave/
-- chapter1.cpp (struct npc_dkc1_gothik line 958 : public
-- CreatureScript; AddSC_the_scarlet_enclave_c1 registers it with
-- RegisterCreatureAI; EK loader decl :95 / call :273).
-- Entry: 28658 ONLY (TDB 26091 creature_template, verified against
-- the dump's own CREATE TABLE column order: 28658 is the sole
-- Gothik the Harvester row carrying ScriptName 'npc_dkc1_gothik',
-- faction 2084). MoveInLineOfSight is the AI's ONLY arm
-- (chapter1.cpp:972-999); there is no Reset/UpdateAI/JustDied.
-- Ported arm (chapter1.cpp:972-999), in C++ order:
-- - who is a Scarlet Ghoul (entry 28845) within 10yd
--   (IsWithinDistInMap) whose owner is a player:
--   quest 12698 INCOMPLETE -> the ghoul casts 52517 on its owner,
--   triggered; the ghoul despawns unconditionally (the C++ @todo
--   says creatures must instead stand next to Gothik and dig into
--   the pit -- unmodeled, the C++ still despawns); quest 12698
--   COMPLETE -> the owner removes all minions by entry 28846
--   (NPC_GHOSTS).
-- - The handler returns nothing, so no veto: ScriptedAI::
--   MoveInLineOfSight runs first and the engage branch proceeds
--   exactly as C++ (Gothik is faction 2084, friendly to the
--   ghouls, so the engage branch never fires for them -- this arm
--   rides the ev27 proximity dispatch, stepCreatureMoveInLOSProximity
--   in creaturemotion.go: 50yd MonsterSight, CanSeeOrDetect gates).
-- - Spell 52517 is an INFERRED seed row (tools/spelldbc_seed/
--   main.go, DUMMY cast on the owner like 52994 Duel Victory);
--   replace with a real DBC row when client data ships.
-- - Bridges live since 2026-11-14: creature:GetOwner,
--   creature:CastSpell(target, id, triggered), player
--   RemoveAllMinionsByEntry, player:GetQuestStatus,
--   IsWithinDistInMap (scripting/object.go generic fallback).

local ENTRY_GOTHIK = 28658
local ENTRY_GHOUL = 28845
local ENTRY_GHOST = 28846
local QUEST_THE_GIFT_THAT_KEEPS_ON_GIVING = 12698
local QUEST_STATUS_INCOMPLETE = 3
local QUEST_STATUS_COMPLETE = 1
local SPELL_GOTHIK_COLLECT = 52517

local function onMoveInLOS(_, creature, who)
    if who:GetEntry() ~= ENTRY_GHOUL then
        return
    end
    if not creature:IsWithinDistInMap(who, 10.0) then
        return
    end
    local owner = who:GetOwner()
    if not owner then
        return
    end
    if owner:GetObjectType() ~= "Player" then
        return
    end
    if owner:GetQuestStatus(QUEST_THE_GIFT_THAT_KEEPS_ON_GIVING) == QUEST_STATUS_INCOMPLETE then
        who:CastSpell(owner, SPELL_GOTHIK_COLLECT, true)
    end
    who:DespawnOrUnsummon()
    if owner:GetQuestStatus(QUEST_THE_GIFT_THAT_KEEPS_ON_GIVING) == QUEST_STATUS_COMPLETE then
        owner:RemoveAllMinionsByEntry(ENTRY_GHOST)
    end
end

RegisterCreatureEvent(ENTRY_GOTHIK, 27, onMoveInLOS)
